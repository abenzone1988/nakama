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
	"errors"
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrSystemNotificationNotFound = errors.New("系统通知不存在")
)

func SystemNotificationDelete(ctx context.Context, db *sql.DB, logger *zap.Logger, notificationId string) error {
	query := "DELETE FROM system_notification WHERE id = $1"
	result, err := db.ExecContext(ctx, query, notificationId)
	if err != nil {
		logger.Error("删除系统通知失败", zap.Error(err))
		return err
	}

	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
		return ErrSystemNotificationNotFound
	}

	return nil
}

func SystemNotificationCreate(ctx context.Context, db *sql.DB, logger *zap.Logger, subject string, content string, effectiveTime *timestamppb.Timestamp, expiryTime *timestamppb.Timestamp, code int32) (*console.SystemNotice, error) {
	var effectiveTimeVal, expiryTimeVal interface{}
	now := time.Now().UTC()

	if effectiveTime != nil {
		effectiveTimeVal = effectiveTime.AsTime()
	}

	if expiryTime != nil {
		expiryTimeVal = expiryTime.AsTime()
	}

	query := `
		INSERT INTO system_notification (
			subject,
			content,
			create_time,
			effective_time,
			expiry_time,
			code
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, subject, content, create_time, effective_time, expiry_time, code`

	var (
		id              string
		createTime      pgtype.Timestamptz
		dbEffectiveTime pgtype.Timestamptz
		dbExpiryTime    pgtype.Timestamptz
		contentStr      string
	)

	err := db.QueryRowContext(ctx, query, subject, content, now, effectiveTimeVal, expiryTimeVal, code).
		Scan(&id, &subject, &contentStr, &createTime, &dbEffectiveTime, &dbExpiryTime, &code)

	if err != nil {
		logger.Error("创建系统通知失败", zap.Error(err))
		return nil, err
	}

	var noticeContent console.NoticeContent
	if err := json.Unmarshal([]byte(contentStr), &noticeContent); err != nil {
		logger.Error("解析通知内容失败", zap.Error(err))
		return nil, err
	}

	notification := &console.SystemNotice{
		Id:         id,
		Subject:    subject,
		Content:    &noticeContent,
		CreateTime: timestamppb.New(createTime.Time),
	}

	if dbEffectiveTime.Valid {
		notification.EffectiveTime = timestamppb.New(dbEffectiveTime.Time)
	}
	if dbExpiryTime.Valid {
		notification.ExpiryTime = timestamppb.New(dbExpiryTime.Time)
	}

	return notification, nil
}

func SystemNotificationList(ctx context.Context, logger *zap.Logger, db *sql.DB, limit int, cursor string) (*console.ListSystemNoticeResponse, error) {
	conditions := "WHERE 1=1"
	params := make([]interface{}, 0)
	paramCount := 0

	// 先查询总数
	countQuery := "SELECT COUNT(*) FROM system_notification " + conditions
	var totalCount int32
	err := db.QueryRowContext(ctx, countQuery, params...).Scan(&totalCount)
	if err != nil {
		logger.Error("Error counting announcements", zap.Error(err))
		return nil, err
	}

	query := `
		SELECT id, subject, content, create_time, effective_time, expiry_time, code
		FROM system_notification
		` + conditions

	if cursor != "" {
		paramCount++
		params = append(params, cursor)
		query += " AND id < $" + strconv.Itoa(paramCount)
	}

	paramCount++
	params = append(params, limit+1)
	query += " ORDER BY id DESC LIMIT $" + strconv.Itoa(paramCount)

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	notifications := make([]*console.SystemNotice, 0, limit)
	var nextCursor string

	for rows.Next() {
		var (
			id            string
			subject       string
			contentStr    string
			createTime    pgtype.Timestamptz
			effectiveTime pgtype.Timestamptz
			expiryTime    pgtype.Timestamptz
			code          int32
		)

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
			Id:         id,
			Subject:    subject,
			Content:    &content,
			CreateTime: timestamppb.New(createTime.Time),
		}

		if effectiveTime.Valid {
			notification.EffectiveTime = timestamppb.New(effectiveTime.Time)
		}
		if expiryTime.Valid {
			notification.ExpiryTime = timestamppb.New(expiryTime.Time)
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
		TotalCount:    totalCount,
	}, nil
}

func SystemNotificationUpdate(ctx context.Context, db *sql.DB, logger *zap.Logger, id string, subject string, content string, effectiveTime *timestamppb.Timestamp, expiryTime *timestamppb.Timestamp, code int32) (*console.SystemNotice, error) {
	var effectiveTimeVal, expiryTimeVal interface{}

	if effectiveTime != nil {
		effectiveTimeVal = effectiveTime.AsTime()
	}

	if expiryTime != nil {
		expiryTimeVal = expiryTime.AsTime()
	}

	query := `
		UPDATE system_notification
		SET subject = $2,
			content = $3,
			effective_time = $4,
			expiry_time = $5,
			code = $6
		WHERE id = $1
		RETURNING id, subject, content, create_time, effective_time, expiry_time, code`

	var (
		notificationId  string
		createTime      pgtype.Timestamptz
		dbEffectiveTime pgtype.Timestamptz
		dbExpiryTime    pgtype.Timestamptz
		contentStr      string
	)

	err := db.QueryRowContext(ctx, query, id, subject, content, effectiveTimeVal, expiryTimeVal, code).
		Scan(&notificationId, &subject, &contentStr, &createTime, &dbEffectiveTime, &dbExpiryTime, &code)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSystemNotificationNotFound
		}
		logger.Error("更新系统通知失败", zap.Error(err))
		return nil, err
	}

	var noticeContent console.NoticeContent
	if err := json.Unmarshal([]byte(contentStr), &noticeContent); err != nil {
		logger.Error("解析通知内容失败", zap.Error(err))
		return nil, err
	}

	notification := &console.SystemNotice{
		Id:         notificationId,
		Subject:    subject,
		Content:    &noticeContent,
		CreateTime: timestamppb.New(createTime.Time),
	}

	if dbEffectiveTime.Valid {
		notification.EffectiveTime = timestamppb.New(dbEffectiveTime.Time)
	}
	if dbExpiryTime.Valid {
		notification.ExpiryTime = timestamppb.New(dbExpiryTime.Time)
	}

	return notification, nil
}

func SystemNotificationGet(ctx context.Context, db *sql.DB, logger *zap.Logger, id string) (*console.SystemNotice, error) {
	query := `
		SELECT id, subject, content, create_time, effective_time, expiry_time, code
		FROM system_notification
		WHERE id = $1`

	var (
		notificationId string
		subject        string
		contentStr     string
		createTime     pgtype.Timestamptz
		effectiveTime  pgtype.Timestamptz
		expiryTime     pgtype.Timestamptz
		code           int32
	)

	err := db.QueryRowContext(ctx, query, id).Scan(
		&notificationId,
		&subject,
		&contentStr,
		&createTime,
		&effectiveTime,
		&expiryTime,
		&code,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSystemNotificationNotFound
		}
		logger.Error("查询系统通知失败", zap.Error(err))
		return nil, err
	}

	var content console.NoticeContent
	if err := json.Unmarshal([]byte(contentStr), &content); err != nil {
		logger.Error("解析通知内容失败", zap.Error(err))
		return nil, err
	}

	notification := &console.SystemNotice{
		Id:         notificationId,
		Subject:    subject,
		Content:    &content,
		CreateTime: timestamppb.New(createTime.Time),
	}

	if effectiveTime.Valid {
		notification.EffectiveTime = timestamppb.New(effectiveTime.Time)
	}
	if expiryTime.Valid {
		notification.ExpiryTime = timestamppb.New(expiryTime.Time)
	}

	return notification, nil
}

func SyncSystemNotifications(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, id uuid.UUID) error {
	// 获取用户元数据
	userMeta, _, err := GetUserMeta(ctx, logger, db, statusRegistry)
	if err != nil {
		logger.Error("获取用户元数据失败", zap.Error(err))
		return status.Error(codes.Internal, "获取用户元数据失败")
	}

	// 查询新的系统通知
	notices, err := QuerySystemNotifications(ctx, db, logger, userMeta.LastSyncNotice)
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

		createTime := notice.GetCreateTime().AsTime().UTC().Unix()
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
			CreateTime: notice.GetCreateTime(),
			ExpiryTime: notice.GetExpiryTime(),
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

func QuerySystemNotifications(ctx context.Context, db *sql.DB, logger *zap.Logger, lastSyncTime int64) ([]*console.SystemNotice, error) {
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
			Id:         id,
			Subject:    subject,
			Content:    &content,
			CreateTime: timestamppb.New(createTime.Time),
		}

		if effectiveTime.Valid {
			notification.EffectiveTime = timestamppb.New(effectiveTime.Time)
		}
		if expiryTime.Valid {
			notification.ExpiryTime = timestamppb.New(expiryTime.Time)
		}

		notifications = append(notifications, notification)
	}

	if err = rows.Err(); err != nil {
		logger.Error("遍历系统通知数据失败", zap.Error(err))
		return nil, err
	}

	return notifications, nil
}
