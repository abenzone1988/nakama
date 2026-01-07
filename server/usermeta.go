package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"go.uber.org/zap"
)

type UserMeta struct {
	LastSyncNotice           int64     `json:"last_sync_notice"`             // 秒级时间戳（unix seconds）
	StaminaLastRefreshTime   time.Time `json:"stamina_last_refresh_time"`    // 体力最后刷新时间
	RestStationStamina       int32     `json:"rest_station_stamina"`         // 休息站存储的体力数量
	RestStationLastGrantTime time.Time `json:"rest_station_last_grant_time"` // 休息站最后一次补充时间
}

// LoadUserMeta 从数据库加载 UserMeta
func LoadUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, userID uuid.UUID) (*UserMeta, *api.User, error) {
	ids := []string{userID.String()}
	users, err := GetUsers(ctx, logger, db, statusRegistry, ids, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	user := users.Users[0]
	userMeta := &UserMeta{}

	// 如果 metadata 不为空，则解析
	if user.Metadata != "" {
		if err := json.Unmarshal([]byte(user.Metadata), userMeta); err != nil {
			logger.Error("json.Unmarshal user.Metadata",
				zap.Error(err),
				zap.String("metadata", user.Metadata))
			return nil, nil, err
		}
	}

	return userMeta, user, nil
}

// SaveUserMeta 保存 UserMeta 到数据库
func SaveUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, userMeta *UserMeta) error {
	metadataJSON, err := json.Marshal(userMeta)
	if err != nil {
		logger.Error("json.Marshal userMeta failed", zap.Error(err))
		return err
	}

	if err = UpdateAccountMetadata(ctx, logger, db, userID, string(metadataJSON)); err != nil {
		logger.Error("update user Metadata err",
			zap.Error(err),
			zap.String("metadata", string(metadataJSON)))
		return err
	}

	return nil
}

// InitializeNewUserMeta 初始化新用户的 UserMeta（在用户注册时调用）
func InitializeNewUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, templateMgr TemplateManager) error {
	// 创建新的 UserMeta
	userMeta := &UserMeta{}

	// 注意：StaminaData 和 LevelData 已迁移到独立的 storage 存储
	// 它们会在首次访问时自动初始化，不需要在这里初始化

	// TODO: 在这里添加其他模块的初始化（如商店、任务等）
	// 注意：首冲、七日购买、每日签到数据已迁移到独立的 storage 存储

	// 序列化并保存到数据库
	metadataJSON, err := json.Marshal(userMeta)
	if err != nil {
		logger.Error("序列化 UserMeta 失败", zap.Error(err), zap.String("user_id", userID.String()))
		return err
	}

	if err := UpdateAccountMetadata(ctx, logger, db, userID, string(metadataJSON)); err != nil {
		logger.Error("保存新用户 UserMeta 失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("metadata", string(metadataJSON)))
		return err
	}

	logger.Info("成功初始化新用户 UserMeta",
		zap.String("user_id", userID.String()),
		zap.String("metadata", string(metadataJSON)))
	return nil
}
