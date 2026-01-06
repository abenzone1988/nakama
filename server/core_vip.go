// Copyright 2024 The Nakama Authors
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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrVipAccountNotFound      = errors.New("VIP账户不存在")
	ErrVipAccountAlreadyExists = errors.New("VIP账户已存在")
)

// VipAccountAdd 添加VIP账户
func VipAccountAdd(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, username string, expiryTime time.Time) (*console.VipAccount, error) {
	// 检查用户是否已经是VIP
	var existingID string
	err := db.QueryRowContext(ctx, "SELECT id FROM vip_accounts WHERE user_id = $1 AND expiry_time > NOW()", userID).Scan(&existingID)
	if err == nil {
		return nil, ErrVipAccountAlreadyExists
	} else if err != sql.ErrNoRows {
		logger.Error("检查VIP账户失败", zap.Error(err))
		return nil, err
	}

	// 生成新的VIP记录ID
	vipID := uuid.Must(uuid.NewV4())
	createTime := time.Now()

	// 插入新的VIP记录
	_, err = db.ExecContext(ctx, `
		INSERT INTO vip_accounts (id, user_id, username, create_time, expiry_time)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			expiry_time = $5,
			create_time = $4
	`, vipID, userID, username, createTime, expiryTime)

	if err != nil {
		logger.Error("插入VIP账户失败", zap.Error(err))
		return nil, err
	}

	return &console.VipAccount{
		Id:         vipID.String(),
		UserId:     userID.String(),
		Username:   username,
		CreateTime: timestamppb.New(createTime),
		ExpiryTime: timestamppb.New(expiryTime),
		IsActive:   expiryTime.After(time.Now()),
	}, nil
}

// VipAccountList 获取VIP账户列表
func VipAccountList(ctx context.Context, logger *zap.Logger, db *sql.DB, limit int, cursor string, filter string) (*console.VipAccountList, error) {
	var query strings.Builder
	var args []interface{}
	argIndex := 1

	query.WriteString(`
		SELECT id, user_id, username, create_time, expiry_time
		FROM vip_accounts
		WHERE 1=1
	`)

	// 添加过滤条件
	if filter != "" {
		query.WriteString(fmt.Sprintf(" AND (username ILIKE $%d OR user_id::text ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter+"%")
		argIndex++
	}

	// 添加游标条件
	if cursor != "" {
		query.WriteString(fmt.Sprintf(" AND create_time < (SELECT create_time FROM vip_accounts WHERE id = $%d)", argIndex))
		args = append(args, cursor)
		argIndex++
	}

	query.WriteString(fmt.Sprintf(" ORDER BY create_time DESC LIMIT $%d", argIndex))
	args = append(args, limit+1) // 多取一个用于判断是否有下一页

	rows, err := db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		logger.Error("查询VIP账户列表失败", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var accounts []*console.VipAccount
	var nextCursor string
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			// 有更多数据，设置下一页游标
			break
		}

		var vipAccount console.VipAccount
		var id, userID, username string
		var createTime, expiryTime time.Time

		err := rows.Scan(&id, &userID, &username, &createTime, &expiryTime)
		if err != nil {
			logger.Error("扫描VIP账户数据失败", zap.Error(err))
			return nil, err
		}

		vipAccount = console.VipAccount{
			Id:         id,
			UserId:     userID,
			Username:   username,
			CreateTime: timestamppb.New(createTime),
			ExpiryTime: timestamppb.New(expiryTime),
			IsActive:   expiryTime.After(time.Now()),
		}

		accounts = append(accounts, &vipAccount)
		nextCursor = id // 使用当前记录的ID作为游标
	}

	if count <= limit {
		nextCursor = ""
	}

	// 获取总数
	var totalCount int32
	countQuery := "SELECT COUNT(*) FROM vip_accounts"
	countArgs := []interface{}{}
	if filter != "" {
		countQuery += " WHERE (username ILIKE $1 OR user_id::text ILIKE $1)"
		countArgs = append(countArgs, "%"+filter+"%")
	}

	err = db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		logger.Error("获取VIP账户总数失败", zap.Error(err))
		return nil, err
	}

	return &console.VipAccountList{
		Accounts:   accounts,
		NextCursor: nextCursor,
		TotalCount: totalCount,
		PrevCursor: "", // 简化实现，不提供上一页游标
	}, nil
}

// VipAccountRemove 移除VIP账户（设置为过期）
func VipAccountRemove(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID) error {
	result, err := db.ExecContext(ctx, `
		UPDATE vip_accounts
		SET expiry_time = NOW()
		WHERE user_id = $1 AND expiry_time > NOW()
	`, userID)

	if err != nil {
		logger.Error("更新VIP账户过期时间失败", zap.Error(err))
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Error("获取受影响行数失败", zap.Error(err))
		return err
	}

	if rowsAffected == 0 {
		return ErrVipAccountNotFound
	}

	return nil
}

// VipAccountCheck 检查用户VIP状态
func VipAccountCheck(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID) (*console.VipAccount, bool, error) {
	var id, username string
	var createTime, expiryTime time.Time

	err := db.QueryRowContext(ctx, `
		SELECT id, username, create_time, expiry_time
		FROM vip_accounts
		WHERE user_id = $1
		ORDER BY create_time DESC
		LIMIT 1
	`, userID).Scan(&id, &username, &createTime, &expiryTime)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}

	if err != nil {
		logger.Error("查询VIP账户状态失败", zap.Error(err))
		return nil, false, err
	}

	isActive := expiryTime.After(time.Now())
	vipAccount := &console.VipAccount{
		Id:         id,
		UserId:     userID.String(),
		Username:   username,
		CreateTime: timestamppb.New(createTime),
		ExpiryTime: timestamppb.New(expiryTime),
		IsActive:   isActive,
	}

	return vipAccount, isActive, nil
}

// IsUserVip 简单检查用户是否为VIP（用于其他服务调用）
func IsUserVip(ctx context.Context, db *sql.DB, userID uuid.UUID) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM vip_accounts
		WHERE user_id = $1 AND expiry_time > NOW()
	`, userID).Scan(&count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// VipAccountExtend 扩展VIP账户有效期
func VipAccountExtend(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, username string, extendDuration time.Duration) (*console.VipAccount, error) {
	var id string
	var existingExpiryTime time.Time
	var createTime time.Time

	err := db.QueryRowContext(ctx, `
		SELECT id, expiry_time, create_time
		FROM vip_accounts
		WHERE user_id = $1
		ORDER BY create_time DESC
		LIMIT 1
	`, userID).Scan(&id, &existingExpiryTime, &createTime)

	now := time.Now()
	var newExpiryTime time.Time

	if err == sql.ErrNoRows {
		newExpiryTime = now.Add(extendDuration)
		vipID := uuid.Must(uuid.NewV4())
		_, err = db.ExecContext(ctx, `
			INSERT INTO vip_accounts (id, user_id, username, create_time, expiry_time)
			VALUES ($1, $2, $3, $4, $5)
		`, vipID, userID, username, now, newExpiryTime)
		if err != nil {
			logger.Error("插入VIP账户失败", zap.Error(err))
			return nil, err
		}
		id = vipID.String()
		createTime = now
	} else if err != nil {
		logger.Error("查询VIP账户失败", zap.Error(err))
		return nil, err
	} else {
		if existingExpiryTime.After(now) {
			newExpiryTime = existingExpiryTime.Add(extendDuration)
		} else {
			newExpiryTime = now.Add(extendDuration)
		}

		_, err = db.ExecContext(ctx, `
			UPDATE vip_accounts
			SET expiry_time = $1, username = $2
			WHERE id = $3
		`, newExpiryTime, username, id)
		if err != nil {
			logger.Error("更新VIP账户过期时间失败", zap.Error(err))
			return nil, err
		}
	}

	return &console.VipAccount{
		Id:         id,
		UserId:     userID.String(),
		Username:   username,
		CreateTime: timestamppb.New(createTime),
		ExpiryTime: timestamppb.New(newExpiryTime),
		IsActive:   newExpiryTime.After(now),
	}, nil
}
