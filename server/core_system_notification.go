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
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 系统通知分页cursor结构体
type systemNotificationsCursor struct {
	NotificationID []byte
	EffectiveTime  int64
	IsNext         bool
}

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

func SystemNotificationCreate(ctx context.Context, db *sql.DB, logger *zap.Logger, noticeType int32, subject string, content string, effectiveTime *timestamppb.Timestamp, expiryTime *timestamppb.Timestamp, challengeId int32) (*console.SystemNotice, error) {
	var effectiveTimeVal, expiryTimeVal interface{}
	now := time.Now().UTC()

	if effectiveTime != nil {
		effectiveTimeVal = effectiveTime.AsTime()
	}

	if expiryTime != nil {
		expiryTimeVal = expiryTime.AsTime()
	}

	var challengeIdVal interface{}
	if challengeId > 0 {
		challengeIdVal = challengeId
	}

	query := `
		INSERT INTO system_notification (
			notice_type,
			subject,
			content,
			create_time,
			effective_time,
			expiry_time,
			challenge_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, notice_type, subject, content, create_time, effective_time, expiry_time, challenge_id`

	var (
		id              string
		createTime      pgtype.Timestamptz
		dbEffectiveTime pgtype.Timestamptz
		dbExpiryTime    pgtype.Timestamptz
		dbChallengeId   sql.NullInt32
		contentStr      string
	)

	err := db.QueryRowContext(ctx, query, noticeType, subject, content, now, effectiveTimeVal, expiryTimeVal, challengeIdVal).
		Scan(&id, &noticeType, &subject, &contentStr, &createTime, &dbEffectiveTime, &dbExpiryTime, &dbChallengeId)

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
	if dbChallengeId.Valid {
		notification.ChallengeId = dbChallengeId.Int32
	}

	return notification, nil
}

func SystemNotificationList(ctx context.Context, logger *zap.Logger, db *sql.DB, limit int, cursor string) (*console.ListSystemNoticeResponse, error) {
	var nc *systemNotificationsCursor
	if cursor != "" {
		nc = &systemNotificationsCursor{}
		cb, err := base64.URLEncoding.DecodeString(cursor)
		if err != nil {
			logger.Warn("Could not base64 decode system notification cursor.", zap.String("cursor", cursor))
			return nil, status.Error(codes.InvalidArgument, "Malformed cursor was used.")
		}
		if err = gob.NewDecoder(bytes.NewReader(cb)).Decode(nc); err != nil {
			logger.Warn("Could not decode system notification cursor.", zap.String("cursor", cursor))
			return nil, status.Error(codes.InvalidArgument, "Malformed cursor was used.")
		}
	}

	// 先查询总数
	countQuery := "SELECT COUNT(*) FROM system_notification"
	var totalCount int32
	err := db.QueryRowContext(ctx, countQuery).Scan(&totalCount)
	if err != nil {
		logger.Error("Error counting system notifications", zap.Error(err))
		return nil, err
	}

	query := `
SELECT
    id,
    notice_type,
    subject,
    content,
    create_time,
    effective_time,
    expiry_time,
    challenge_id
FROM
    system_notification
`

	var params []interface{}
	var comparisonOp string
	var sortOrder string

	if nc != nil {
		if nc.IsNext {
			comparisonOp = "<"
			sortOrder = "ORDER BY effective_time DESC, id DESC"
		} else {
			comparisonOp = ">"
			sortOrder = "ORDER BY effective_time ASC, id ASC"
		}
		query += `WHERE (effective_time, id) ` + comparisonOp + ` ($1::TIMESTAMPTZ, $2::UUID)`
		params = append(params, &pgtype.Timestamptz{Time: time.Unix(0, nc.EffectiveTime).UTC(), Valid: true}, uuid.FromBytesOrNil(nc.NotificationID))
	} else {
		sortOrder = "ORDER BY effective_time DESC, id DESC"
	}

	query += " " + sortOrder + " LIMIT $" + strconv.Itoa(len(params)+1)
	params = append(params, limit+1)

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var nextCursor *systemNotificationsCursor
	var prevCursor *systemNotificationsCursor
	notifications := make([]*console.SystemNotice, 0, limit)

	var (
		id            string
		noticeType    sql.NullInt32
		subject       string
		contentStr    string
		createTime    pgtype.Timestamptz
		effectiveTime pgtype.Timestamptz
		expiryTime    pgtype.Timestamptz
		challengeId   sql.NullInt32
	)

	for rows.Next() {
		if len(notifications) >= limit {
			nextCursor = &systemNotificationsCursor{
				NotificationID: uuid.FromStringOrNil(id).Bytes(),
				EffectiveTime:  effectiveTime.Time.UnixNano(),
				IsNext:         true,
			}
			break
		}

		if err = rows.Scan(&id, &noticeType, &subject, &contentStr, &createTime, &effectiveTime, &expiryTime, &challengeId); err != nil {
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
			NoticeType: noticeType.Int32,
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
		if challengeId.Valid {
			notification.ChallengeId = challengeId.Int32
		}

		notifications = append(notifications, notification)

		if nc != nil && prevCursor == nil {
			prevCursor = &systemNotificationsCursor{
				NotificationID: uuid.FromStringOrNil(id).Bytes(),
				EffectiveTime:  effectiveTime.Time.UnixNano(),
				IsNext:         false,
			}
		}
	}

	if err = rows.Err(); err != nil {
		logger.Error("遍历系统通知数据失败", zap.Error(err))
		return nil, err
	}

	// 如果是向前翻页，需要处理cursor和结果顺序
	if nc != nil && !nc.IsNext {
		if nextCursor != nil && prevCursor != nil {
			nextCursor, nextCursor.IsNext, prevCursor, prevCursor.IsNext = prevCursor, prevCursor.IsNext, nextCursor, nextCursor.IsNext
		} else if nextCursor != nil {
			nextCursor, prevCursor = nil, nextCursor
			prevCursor.IsNext = !prevCursor.IsNext
		} else if prevCursor != nil {
			nextCursor, prevCursor = prevCursor, nil
			nextCursor.IsNext = !nextCursor.IsNext
		}

		// 反转结果顺序
		slices.Reverse(notifications)
	}

	var nextCursorStr string
	if nextCursor != nil {
		cursorBuf := new(bytes.Buffer)
		if err := gob.NewEncoder(cursorBuf).Encode(nextCursor); err != nil {
			logger.Error("Error creating system notification list cursor", zap.Error(err))
			return nil, err
		}
		nextCursorStr = base64.URLEncoding.EncodeToString(cursorBuf.Bytes())
	}

	var prevCursorStr string
	if prevCursor != nil {
		cursorBuf := new(bytes.Buffer)
		if err := gob.NewEncoder(cursorBuf).Encode(prevCursor); err != nil {
			logger.Error("Error creating system notification list cursor", zap.Error(err))
			return nil, err
		}
		prevCursorStr = base64.URLEncoding.EncodeToString(cursorBuf.Bytes())
	}

	return &console.ListSystemNoticeResponse{
		Notifications: notifications,
		NextCursor:    nextCursorStr,
		PrevCursor:    prevCursorStr,
		TotalCount:    totalCount,
	}, nil
}

func SystemNotificationUpdate(ctx context.Context, db *sql.DB, logger *zap.Logger, id string, subject string, content string, effectiveTime *timestamppb.Timestamp, expiryTime *timestamppb.Timestamp) (*console.SystemNotice, error) {
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
			expiry_time = $5
		WHERE id = $1
		RETURNING id, subject, content, create_time, effective_time, expiry_time`

	var (
		notificationId  string
		createTime      pgtype.Timestamptz
		dbEffectiveTime pgtype.Timestamptz
		dbExpiryTime    pgtype.Timestamptz
		contentStr      string
	)

	err := db.QueryRowContext(ctx, query, id, subject, content, effectiveTimeVal, expiryTimeVal).
		Scan(&notificationId, &subject, &contentStr, &createTime, &dbEffectiveTime, &dbExpiryTime)

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
		SELECT id, notice_type, subject, content, create_time, effective_time, expiry_time, challenge_id
		FROM system_notification
		WHERE id = $1`

	var (
		notificationId string
		subject        string
		contentStr     string
		createTime     pgtype.Timestamptz
		effectiveTime  pgtype.Timestamptz
		expiryTime     pgtype.Timestamptz
		challengeId    sql.NullInt32
		noticeType     sql.NullInt32
	)

	err := db.QueryRowContext(ctx, query, id).Scan(
		&notificationId,
		&noticeType,
		&subject,
		&contentStr,
		&createTime,
		&effectiveTime,
		&expiryTime,
		&challengeId,
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
		NoticeType: noticeType.Int32,
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
	if challengeId.Valid {
		notification.ChallengeId = challengeId.Int32
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
	serverNow := time.Now().Unix()
	notices, err := QuerySystemNotifications(ctx, db, logger, userMeta.LastSyncNotice, serverNow)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return err
	}

	if len(notices) == 0 {
		return nil
	}

	logger.Info("查询到的系统通知", zap.Any("notices", notices))
	// 构建通知列表
	notifications := make([]*api.Notification, 0, len(notices))
	var latestSyncTime int64 = userMeta.LastSyncNotice

	logger.Info("上次同步的时间", zap.String("last_sync_time", time.Unix(latestSyncTime, 0).Format("2006-01-02 15:04:05")))
	logger.Info("当前系统时间", zap.String("last_sync_time", time.Unix(serverNow, 0).Format("2006-01-02 15:04:05")))

	// 加载用户挑战赛数据，用于检查比赛类型通知
	var userMatch *UserMatch
	var needLoadUserMatch bool

	for _, notice := range notices {
		// 检查是否需要加载用户挑战赛数据
		if notice.GetNoticeType() == 1 && !needLoadUserMatch {
			needLoadUserMatch = true
		}

		contentJson, err := json.Marshal(notice.GetContent())
		if err != nil {
			logger.Error("序列化通知内容失败", zap.Error(err))
			continue
		}

		// 使用 effective_time 作为同步时间戳，但确保只处理已生效的通知
		effectiveTime := notice.GetEffectiveTime().AsTime().UTC().Unix()

		// 只处理已生效的通知

		// 根据通知类型进行不同处理
		shouldAddNotification := false

		switch notice.GetNoticeType() {
		case 0: // 全局邮件，直接添加
			shouldAddNotification = true
			logger.Debug("处理全局系统通知",
				zap.String("notice_id", notice.GetId()),
				zap.String("subject", notice.GetSubject()))

		case 1: // 比赛邮件，需要检查用户是否参加了对应的挑战赛
			if userMatch == nil && needLoadUserMatch {
				userMatch = &UserMatch{}
				err := LoadData(ctx, logger, db, id, userMatch)
				if err != nil {
					logger.Error("加载用户挑战赛数据失败", zap.Error(err))
					// 加载失败时跳过比赛类型通知，避免误发
					continue
				}
			}

			// 检查用户是否参加了对应的挑战赛
			if userMatch != nil {
				challengeID := notice.GetChallengeId()
				if challengeStatus, exists := userMatch.Challenges[challengeID]; exists && challengeStatus != nil {
					shouldAddNotification = true
					logger.Debug("用户参加了挑战赛，添加比赛通知",
						zap.String("notice_id", notice.GetId()),
						zap.Int32("challenge_id", challengeID),
						zap.String("subject", notice.GetSubject()))
				} else {
					logger.Debug("用户未参加挑战赛，跳过比赛通知",
						zap.String("notice_id", notice.GetId()),
						zap.Int32("challenge_id", challengeID),
						zap.String("subject", notice.GetSubject()))
				}
			}

		default:
			logger.Warn("未知的通知类型，跳过",
				zap.String("notice_id", notice.GetId()),
				zap.Int32("notice_type", notice.GetNoticeType()))
			continue
		}

		if effectiveTime >= latestSyncTime {
			latestSyncTime = effectiveTime
		}

		if shouldAddNotification {
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

	}

	// 更新用户的最后同步时间
	// 如果有新通知，使用最新的通知时间；否则使用当前时间
	if len(notices) > 0 {
		userMeta.LastSyncNotice = latestSyncTime
		logger.Info("更新同步的时间", zap.String("last_sync_time", time.Unix(latestSyncTime, 0).Format("2006-01-02 15:04:05")))
		if err := SaveUserMeta(ctx, logger, db, userMeta); err != nil {
			logger.Error("更新用户元数据失败", zap.Error(err))
		}
	}

	// 只有在有有效通知时才保存到用户通知列表
	if len(notifications) > 0 {
		if err := NotificationSave(ctx, logger, db, map[uuid.UUID][]*api.Notification{id: notifications}); err != nil {
			logger.Error("保存用户通知失败", zap.Error(err))
			return err
		}
	}

	return nil
}

func QuerySystemNotifications(ctx context.Context, db *sql.DB, logger *zap.Logger, lastSyncTime, serverNow int64) ([]*console.SystemNotice, error) {
	// 直接使用int64时间戳进行比较，避免类型转换开销
	query := `
		SELECT
			id,
			notice_type,
			subject,
			content,
			create_time,
			effective_time,
			expiry_time,
			challenge_id
		FROM system_notification
		WHERE
			EXTRACT(EPOCH FROM effective_time)::BIGINT > $1
			AND EXTRACT(EPOCH FROM effective_time)::BIGINT <= $2
			AND (expiry_time IS NULL OR EXTRACT(EPOCH FROM expiry_time)::BIGINT > $2)
		ORDER BY effective_time ASC
	`

	rows, err := db.QueryContext(ctx, query, lastSyncTime, serverNow)
	if err != nil {
		logger.Error("查询系统通知失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	notifications := make([]*console.SystemNotice, 0)
	for rows.Next() {
		var id string
		var noticeType sql.NullInt32
		var subject string
		var contentStr string
		var createTime pgtype.Timestamptz
		var effectiveTime pgtype.Timestamptz
		var expiryTime pgtype.Timestamptz
		var challengeId sql.NullInt32

		if err = rows.Scan(&id, &noticeType, &subject, &contentStr, &createTime, &effectiveTime, &expiryTime, &challengeId); err != nil {
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
			NoticeType: noticeType.Int32,
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
		if challengeId.Valid {
			notification.ChallengeId = challengeId.Int32
		}

		notifications = append(notifications, notification)
	}

	if err = rows.Err(); err != nil {
		logger.Error("遍历系统通知数据失败", zap.Error(err))
		return nil, err
	}

	return notifications, nil
}

// PersonalNotificationLogCreate 创建个人通知发送日志
func PersonalNotificationLogCreate(ctx context.Context, db *sql.DB, logger *zap.Logger, logID, subject, content string, targetUserIds []string, sender string) error {
	query := `
		INSERT INTO personal_notification_log (
			id,
			subject,
			content,
			target_ids,
			sender,
			notification_count
		) VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := db.ExecContext(ctx, query, logID, subject, content, targetUserIds, sender, len(targetUserIds))
	if err != nil {
		logger.Error("创建个人通知日志失败", zap.Error(err))
		return err
	}

	return nil
}

// PersonalNotificationLogList 获取个人通知日志列表
func PersonalNotificationLogList(ctx context.Context, db *sql.DB, logger *zap.Logger, limit int, cursor string, filter, dateFrom, dateTo string) (*console.ListPersonalNotificationLogResponse, error) {
	// 构建查询条件
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if filter != "" {
		whereClause += " AND (subject ILIKE $1 OR content::text ILIKE $1)"
		args = append(args, "%"+filter+"%")
		argIndex++
	}

	if dateFrom != "" {
		whereClause += fmt.Sprintf(" AND send_time >= $%d", argIndex)
		args = append(args, dateFrom)
		argIndex++
	}

	if dateTo != "" {
		whereClause += fmt.Sprintf(" AND send_time <= $%d", argIndex)
		args = append(args, dateTo)
		argIndex++
	}

	if cursor != "" {
		whereClause += fmt.Sprintf(" AND send_time < (SELECT send_time FROM personal_notification_log WHERE id = $%d)", argIndex)
		args = append(args, cursor)
		argIndex++
	}

	// 查询总数 (不包含分页限制)
	countQuery := "SELECT COUNT(*) FROM personal_notification_log " + whereClause
	var totalCount int64
	err := db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		logger.Error("查询个人通知日志总数失败", zap.Error(err))
		return nil, err
	}

	// 添加分页限制
	limitClause := fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit+1) // 多查询一条用于判断是否有下一页

	// 查询数据
	query := `
		SELECT
			id,
			subject,
			content,
			target_ids,
			sender,
			send_time,
			notification_count
		FROM personal_notification_log
		` + whereClause + `
		ORDER BY send_time DESC
		` + limitClause

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		logger.Error("查询个人通知日志失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	logs := make([]*console.PersonalNotificationLog, 0, limit)
	var nextCursor string

	for rows.Next() {
		var (
			id                string
			subject           string
			contentStr        string
			targetIds         string
			sender            string
			sendTime          pgtype.Timestamptz
			notificationCount int
		)

		if err := rows.Scan(&id, &subject, &contentStr, &targetIds, &sender, &sendTime, &notificationCount); err != nil {
			logger.Error("扫描个人通知日志数据失败", zap.Error(err))
			continue
		}

		// 如果已经获取了足够的记录，设置下一页cursor并退出
		if len(logs) >= limit {
			nextCursor = id
			break
		}

		var content console.NoticeContent
		if err := json.Unmarshal([]byte(contentStr), &content); err != nil {
			logger.Error("解析通知内容失败", zap.Error(err))
			return nil, err
		}

		log := &console.PersonalNotificationLog{
			Id:                id,
			Subject:           subject,
			Content:           &content,
			TargetIds:         targetIds,
			Sender:            sender,
			SendTime:          timestamppb.New(sendTime.Time),
			NotificationCount: int32(notificationCount),
		}

		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		logger.Error("遍历个人通知日志数据失败", zap.Error(err))
		return nil, err
	}

	// 计算上一页的cursor
	var prevCursor string
	if cursor != "" {
		prevQuery := `
			SELECT id FROM personal_notification_log
			WHERE send_time > (SELECT send_time FROM personal_notification_log WHERE id = $1)
			ORDER BY send_time ASC
			LIMIT 1
		`
		err := db.QueryRowContext(ctx, prevQuery, cursor).Scan(&prevCursor)
		if err != nil && err != sql.ErrNoRows {
			logger.Error("查询上一页cursor失败", zap.Error(err))
		}
	}

	return &console.ListPersonalNotificationLogResponse{
		Logs:       logs,
		TotalCount: int32(totalCount),
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
	}, nil
}
