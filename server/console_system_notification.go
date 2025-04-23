// Copyright 2019 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type globalNotificationsCursor struct {
	ID     int64
	IsNext bool
}

func (s *ConsoleServer) ListSystemNotifications(ctx context.Context, in *console.ListSystemNoticeRequest) (*console.ListSystemNoticeResponse, error) {
	// 参数验证
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 20
	}

	notifications, err := ListSystemNotifications(ctx, s.logger, s.db, int(in.Limit), in.Cursor)
	if err != nil {
		s.logger.Error("获取系统通知列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取系统通知列表失败")
	}

	return notifications, nil
}

func (s *ConsoleServer) CreateSystemNotification(ctx context.Context, in *console.CreateSystemNotificationRequest) (*emptypb.Empty, error) {
	// 参数验证
	notice := in.GetNotice()
	if notice == nil {
		return nil, status.Error(codes.InvalidArgument, "通知内容不能为空")
	}
	if notice.GetSubject() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知标题不能为空")
	}
	if notice.GetContent() == nil {
		return nil, status.Error(codes.InvalidArgument, "通知内容不能为空")
	}

	contentJson, err := json.Marshal(notice.GetContent())
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "通知内容格式错误")
	}

	// 创建系统通知
	if err := CreateSystemNotification(ctx, s.db, s.logger, notice.GetSubject(), string(contentJson), notice.GetEffective(), notice.GetExpiry(), notice.GetCode()); err != nil {
		s.logger.Error("创建系统通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "创建系统通知失败")
	}

	// 如果需要即时推送
	if in.Type == 0 || in.Type == 1 {
		notification := &api.Notification{
			Id:         uuid.Must(uuid.NewV4()).String(),
			Subject:    notice.GetSubject(),
			Content:    string(contentJson),
			SenderId:   uuid.Nil.String(),
			Code:       notice.GetCode(),
			Persistent: false,
		}
		if err := NotificationSendAll(ctx, s.logger, s.db, s.tracker, s.router, notification); err != nil {
			s.logger.Error("发送系统通知失败", zap.Error(err))
			return nil, status.Error(codes.Internal, "发送系统通知失败")
		}
	} else if in.Type == 2 && len(in.GetTarget()) > 0 {
		// 发送给指定用户
		userIDs, err := fetchUserID(ctx, s.db, in.GetTarget())
		if err != nil {
			s.logger.Error("获取用户ID失败", zap.Error(err))
			return nil, status.Error(codes.Internal, "获取用户ID失败")
		}

		if len(userIDs) == 0 {
			return nil, status.Error(codes.InvalidArgument, "未找到有效的用户")
		}

		notifications := make(map[uuid.UUID][]*api.Notification)
		for _, id := range userIDs {
			uid := uuid.FromStringOrNil(id)
			notifications[uid] = []*api.Notification{{
				Id:         uuid.Must(uuid.NewV4()).String(),
				Subject:    notice.GetSubject(),
				Content:    string(contentJson),
				SenderId:   uuid.Nil.String(),
				Code:       notice.GetCode(),
				Persistent: true,
			}}
		}

		if err := NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications); err != nil {
			s.logger.Error("发送系统通知给指定用户失败", zap.Error(err))
			return nil, status.Error(codes.Internal, "发送系统通知失败")
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *ConsoleServer) DeleteSystemNotification(ctx context.Context, in *console.SystemNotificationId) (*emptypb.Empty, error) {
	// 参数验证
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知ID不能为空")
	}

	if err := DeleteSystemNotification(ctx, s.db, s.logger, in.GetId()); err != nil {
		s.logger.Error("删除系统通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "删除系统通知失败")
	}

	return &emptypb.Empty{}, nil
}

// DeleteSystemNotification is the internal implementation
func DeleteSystemNotification(ctx context.Context, db *sql.DB, logger *zap.Logger, notificationId string) error {
	query := "DELETE FROM system_notification WHERE id = $1"
	result, err := db.ExecContext(ctx, query, notificationId)
	if err != nil {
		logger.Error("执行删除系统通知SQL失败", zap.Error(err))
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Error("获取影响行数失败", zap.Error(err))
		return err
	}

	if rowsAffected == 0 {
		return status.Error(codes.NotFound, "通知不存在")
	}

	return nil
}

func CreateSystemNotification(ctx context.Context, db *sql.DB, logger *zap.Logger, subject string, content string, effectiveTime *timestamppb.Timestamp, expiryTime *timestamppb.Timestamp, code int32) error {
	var effectiveTimeVal, expiryTimeVal interface{} // 使用 interface{} 类型来处理可能的 NULL 值

	// 转换 effectiveTime
	if effectiveTime != nil {
		effectiveTimeVal = effectiveTime.AsTime()
	} else {
		effectiveTimeVal = nil // 处理 NULL 值
	}

	// 转换 expiryTime
	if expiryTime != nil {
		expiryTimeVal = expiryTime.AsTime()
	} else {
		expiryTimeVal = nil // 处理 NULL 值
	}

	insertQuery := `
		INSERT INTO system_notification (
			subject,
			content,
			create_time,
			effective_time,
			expiry_time,
			code
		) VALUES ($1, $2, now(), $3, $4, $5)`

	_, err := db.ExecContext(ctx, insertQuery, subject, content, effectiveTimeVal, expiryTimeVal, code)

	if err != nil {
		logger.Error("Error inserting data into system_notification table.", zap.Error(err))
		return err
	}

	return nil
}

func ListSystemNotifications(ctx context.Context, logger *zap.Logger, db *sql.DB, limit int, cursor string) (*console.ListSystemNoticeResponse, error) {
	params := make([]interface{}, 0)
	query := "SELECT id, subject, content, create_time, effective_time, expiry_time, code FROM system_notification"

	if cursor != "" {
		query += " WHERE id < $1"
		params = append(params, cursor)
	}

	query += " ORDER BY id DESC LIMIT $" + fmt.Sprintf("%d", len(params)+1)
	params = append(params, limit+1)

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	notifications := make([]*console.SystemNotice, 0, limit)
	var nextCursor string

	for rows.Next() {
		var id string
		var subject string
		var contentStr string
		var createTime pgtype.Timestamptz
		var effectiveTime pgtype.Timestamptz
		var expiryTime pgtype.Timestamptz
		var code int32

		if err = rows.Scan(&id, &subject, &contentStr, &createTime, &effectiveTime, &expiryTime, &code); err != nil {
			logger.Error("扫描系统通知数据失败", zap.Error(err))
			return nil, err
		}

		if len(notifications) >= limit {
			nextCursor = id
			break
		}

		var content console.NoticeContent
		if err := json.Unmarshal([]byte(contentStr), &content); err != nil {
			logger.Error("解析通知内容失败", zap.Error(err))
			return nil, err
		}

		notification := &console.SystemNotice{
			Id:      id,
			Subject: subject,
			Content: &content,
			Create:  timestamppb.New(createTime.Time),
			Code:    code,
		}

		if effectiveTime.Valid {
			notification.Effective = timestamppb.New(effectiveTime.Time)
		}
		if expiryTime.Valid {
			notification.Expiry = timestamppb.New(expiryTime.Time)
		}

		notifications = append(notifications, notification)
	}

	if err = rows.Err(); err != nil {
		logger.Error("遍历系统通知数据失败", zap.Error(err))
		return nil, err
	}

	return &console.ListSystemNoticeResponse{
		Notifications: notifications,
		NextCursor:    nextCursor,
	}, nil
}

func syncSystemNotifications(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, id uuid.UUID) error {
	// 获取用户元数据
	userMeta, _, err := GetUserMeta(ctx, logger, db, statusRegistry)
	if err != nil {
		logger.Error("获取用户元数据失败", zap.Error(err))
		return status.Error(codes.Internal, "获取用户元数据失败")
	}

	// 查询新的系统通知
	notices, err := querySystemNotifications(ctx, db, logger, userMeta.LastSyncNotice)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return err
	}

	if len(notices) == 0 {
		return nil
	}

	// 构建通知列表
	notifications := make([]*api.Notification, 0, len(notices))
	var latestSyncTime int64 = userMeta.LastSyncNotice

	for _, notice := range notices {
		contentJson, err := json.Marshal(notice.GetContent())
		if err != nil {
			logger.Error("序列化通知内容失败", zap.Error(err))
			continue
		}

		createTime := notice.GetCreate().AsTime().UTC().Unix()
		if createTime > latestSyncTime {
			latestSyncTime = createTime
		}

		notifications = append(notifications, &api.Notification{
			Id:         uuid.Must(uuid.NewV4()).String(),
			Subject:    notice.GetSubject(),
			Content:    string(contentJson),
			SenderId:   uuid.Nil.String(),
			Code:       NotificationSystemNotice,
			Persistent: true,
			CreateTime: notice.GetCreate(),
			ExpiryTime: notice.GetExpiry(),
		})
	}

	// 更新用户的最后同步时间
	userMeta.LastSyncNotice = latestSyncTime
	if err := SaveUserMeta(ctx, logger, db, userMeta); err != nil {
		logger.Error("更新用户元数据失败", zap.Error(err))
	}

	// 保存通知到用户的通知列表
	if err := NotificationSave(ctx, logger, db, map[uuid.UUID][]*api.Notification{id: notifications}); err != nil {
		logger.Error("保存用户通知失败", zap.Error(err))
		return err
	}

	return nil
}

func querySystemNotifications(ctx context.Context, db *sql.DB, logger *zap.Logger, lastSyncTime int64) ([]*console.SystemNotice, error) {
	// 使用时间戳查询，考虑时间精度问题
	query := `
		SELECT
			id,
			subject,
			content,
			create_time,
			effective_time,
			expiry_time,
			code
		FROM system_notification
		WHERE
			EXTRACT(EPOCH FROM date_trunc('second', create_time)) > $1
			AND (effective_time IS NULL OR effective_time <= CURRENT_TIMESTAMP)
			AND (expiry_time IS NULL OR expiry_time > CURRENT_TIMESTAMP)
		ORDER BY create_time ASC
	`

	rows, err := db.QueryContext(ctx, query, lastSyncTime)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	notifications := make([]*console.SystemNotice, 0)
	for rows.Next() {
		var id string
		var subject string
		var contentStr string
		var createTime pgtype.Timestamptz
		var effectiveTime pgtype.Timestamptz
		var expiryTime pgtype.Timestamptz
		var code int32

		if err = rows.Scan(&id, &subject, &contentStr, &createTime, &effectiveTime, &expiryTime, &code); err != nil {
			logger.Error("扫描系统通知数据失败", zap.Error(err))
			return nil, err
		}

		var content console.NoticeContent
		if err := json.Unmarshal([]byte(contentStr), &content); err != nil {
			logger.Error("解析通知内容失败", zap.Error(err))
			return nil, err
		}

		notification := &console.SystemNotice{
			Id:      id,
			Subject: subject,
			Content: &content,
			Create:  timestamppb.New(createTime.Time),
			Code:    code,
		}

		if effectiveTime.Valid {
			notification.Effective = timestamppb.New(effectiveTime.Time)
		}
		if expiryTime.Valid {
			notification.Expiry = timestamppb.New(expiryTime.Time)
		}

		notifications = append(notifications, notification)
	}

	if err = rows.Err(); err != nil {
		logger.Error("遍历系统通知数据失败", zap.Error(err))
		return nil, err
	}

	return notifications, nil
}
