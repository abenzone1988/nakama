package server

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// LoadUserMeta 从数据库加载 UserMeta
func LoadUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, userID uuid.UUID) (*game.UserMeta, *api.User, error) {
	ids := []string{userID.String()}
	users, err := GetUsers(ctx, logger, db, statusRegistry, ids, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	user := users.Users[0]
	userMeta := &game.UserMeta{}

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
func SaveUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, userMeta *game.UserMeta) error {
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
	userMeta := &game.UserMeta{}

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
