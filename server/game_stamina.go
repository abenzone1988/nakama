package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	MaxStamina            = 30               // 最大体力值
	StaminaRecoveryRate   = 20 * time.Minute // 每20分钟恢复1点体力
	StaminaRecoveryAmount = 1                // 每次恢复的体力点数
)

func (s *ApiServer) GetCurrentStamina(ctx context.Context, in *emptypb.Empty) (*game.StaminaData, error) {
	return GetCurrentStamina(ctx, s.logger, s.db, s.statusRegistry)
}

// GetCurrentStamina 获取当前体力值（自动计算恢复）
func GetCurrentStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (*game.StaminaData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从钱包读取体力值
	wallet, exists, err := GetWallet(ctx, logger, db, userID)
	if err != nil {
		logger.Error("读取钱包数据失败", zap.Error(err))
		return nil, err
	}
	if !exists {
		return nil, errors.New("用户不存在")
	}

	// 从 UserMeta 读取刷新时间
	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		logger.Error("读取 UserMeta 失败", zap.Error(err))
		return nil, err
	}

	currentStamina := int32(wallet["stamina"])
	lastRefreshTime := userMeta.StaminaLastRefreshTime

	// 如果体力为0且刷新时间为空，初始化
	if currentStamina == 0 && lastRefreshTime == "" {
		currentStamina = MaxStamina
		lastRefreshTime = time.Now().Format(time.RFC3339)

		// 初始化钱包体力
		metadata, _ := json.Marshal(map[string]interface{}{
			"source": "stamina_init",
		})
		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: map[string]int64{"stamina": int64(MaxStamina)},
			Metadata:  string(metadata),
		}}
		if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
			logger.Error("初始化钱包体力失败", zap.Error(err))
			return nil, err
		}

		// 初始化 UserMeta 刷新时间
		userMeta.StaminaLastRefreshTime = lastRefreshTime
		if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
			logger.Error("保存初始化的刷新时间失败", zap.Error(err))
			return nil, err
		}

		logger.Info("Stamina initialized", zap.Int32("stamina", MaxStamina))
		return &game.StaminaData{
			Stamina:         currentStamina,
			LastRefreshTime: lastRefreshTime,
		}, nil
	}

	// 计算自然恢复
	oldStamina := currentStamina
	newStamina, newRefreshTime := calculateRecoveredStamina(currentStamina, lastRefreshTime)

	// 更新体力（如果有变化）
	if newStamina != oldStamina {
		metadata, _ := json.Marshal(map[string]interface{}{
			"source": "stamina_recovery",
		})
		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: map[string]int64{"stamina": int64(newStamina - oldStamina)},
			Metadata:  string(metadata),
		}}
		if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
			logger.Error("更新恢复后的体力失败", zap.Error(err))
			return nil, err
		}

		// 更新 UserMeta 刷新时间
		userMeta.StaminaLastRefreshTime = newRefreshTime
		if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
			logger.Error("保存恢复后的刷新时间失败", zap.Error(err))
			return nil, err
		}

		logger.Info("Stamina auto-recovered",
			zap.Int32("old_stamina", oldStamina),
			zap.Int32("new_stamina", newStamina))
		currentStamina = newStamina
		lastRefreshTime = newRefreshTime
	}

	return &game.StaminaData{
		Stamina:         currentStamina,
		LastRefreshTime: lastRefreshTime,
	}, nil
}

// ConsumeStamina 消耗体力
func ConsumeStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, amount int32) error {
	if amount <= 0 {
		return errors.New("invalid stamina amount")
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从钱包读取体力值
	wallet, exists, err := GetWallet(ctx, logger, db, userID)
	if err != nil {
		logger.Error("读取钱包数据失败", zap.Error(err))
		return err
	}
	if !exists {
		return errors.New("用户不存在")
	}

	// 从 UserMeta 读取刷新时间
	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		logger.Error("读取 UserMeta 失败", zap.Error(err))
		return err
	}

	currentStamina := int32(wallet["stamina"])
	lastRefreshTime := userMeta.StaminaLastRefreshTime

	// 如果体力为0且刷新时间为空，初始化但不扣除本次消耗
	if currentStamina == 0 && lastRefreshTime == "" {
		currentStamina = MaxStamina
		lastRefreshTime = time.Now().Format(time.RFC3339)

		metadata, _ := json.Marshal(map[string]interface{}{
			"source": "stamina_init",
		})
		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: map[string]int64{"stamina": int64(MaxStamina)},
			Metadata:  string(metadata),
		}}
		if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
			logger.Error("初始化钱包体力失败", zap.Error(err))
			return err
		}

		userMeta.StaminaLastRefreshTime = lastRefreshTime
		if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
			logger.Error("保存初始化的刷新时间失败", zap.Error(err))
			return err
		}

		logger.Info("体力首次初始化，本次不扣除",
			zap.Int32("stamina", MaxStamina),
			zap.Int32("skip_amount", amount))
		return nil
	}

	// 计算自然恢复后的体力
	oldStamina := currentStamina
	newStamina, newRefreshTime := calculateRecoveredStamina(currentStamina, lastRefreshTime)

	// 检查体力是否足够
	if newStamina < amount {
		logger.Warn("Insufficient stamina", zap.Int32("required", amount), zap.Int32("current", newStamina))
		return errors.New("insufficient stamina")
	}

	// 扣除体力（如果有恢复，先更新恢复的体力，再扣除）
	changesetAmount := -int64(amount)
	if newStamina != oldStamina {
		// 体力有恢复，需要先加上恢复的体力，再扣除消耗的体力
		changesetAmount = int64(newStamina - oldStamina - amount)
	}

	metadata, _ := json.Marshal(map[string]interface{}{
		"source": "stamina_consume",
	})
	walletUpdates := []*walletUpdate{{
		UserID:    userID,
		Changeset: map[string]int64{"stamina": changesetAmount},
		Metadata:  string(metadata),
	}}
	if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
		logger.Error("扣除体力失败", zap.Error(err))
		return err
	}

	// 更新刷新时间（如果体力有恢复）
	if newStamina != oldStamina {
		userMeta.StaminaLastRefreshTime = newRefreshTime
		if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
			logger.Error("保存刷新时间失败", zap.Error(err))
			return err
		}
	}

	logger.Info("Stamina consumed", zap.Int32("amount", amount), zap.Int32("remaining", newStamina-amount))
	return nil
}

// RefillStamina 补充体力（通过道具或购买）
func RefillStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, amount int32) error {
	if amount <= 0 {
		return errors.New("invalid stamina amount")
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从钱包读取体力值
	wallet, exists, err := GetWallet(ctx, logger, db, userID)
	if err != nil {
		logger.Error("读取钱包数据失败", zap.Error(err))
		return err
	}
	if !exists {
		return errors.New("用户不存在")
	}

	oldStamina := int32(wallet["stamina"])

	// 补充体力（不设上限，可以超过最大值）
	metadata, _ := json.Marshal(map[string]interface{}{
		"source": "stamina_refill",
	})
	walletUpdates := []*walletUpdate{{
		UserID:    userID,
		Changeset: map[string]int64{"stamina": int64(amount)},
		Metadata:  string(metadata),
	}}
	if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
		logger.Error("补充体力失败", zap.Error(err))
		return err
	}

	logger.Info("Stamina refilled", zap.Int32("amount", amount), zap.Int32("old", oldStamina), zap.Int32("new", oldStamina+amount))
	return nil
}

// ResetStamina 重置体力到最大值（用于测试或管理员操作）
func ResetStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从钱包读取当前体力值
	wallet, exists, err := GetWallet(ctx, logger, db, userID)
	if err != nil {
		logger.Error("读取钱包数据失败", zap.Error(err))
		return err
	}
	if !exists {
		return errors.New("用户不存在")
	}

	currentStamina := int32(wallet["stamina"])
	resetAmount := MaxStamina - currentStamina

	// 重置体力到最大值
	metadata, _ := json.Marshal(map[string]interface{}{
		"source": "stamina_reset",
	})
	walletUpdates := []*walletUpdate{{
		UserID:    userID,
		Changeset: map[string]int64{"stamina": int64(resetAmount)},
		Metadata:  string(metadata),
	}}
	if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
		logger.Error("重置体力失败", zap.Error(err))
		return err
	}

	// 更新 UserMeta 刷新时间
	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		logger.Error("读取 UserMeta 失败", zap.Error(err))
		return err
	}
	userMeta.StaminaLastRefreshTime = time.Now().Format(time.RFC3339)
	if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
		logger.Error("保存重置后的刷新时间失败", zap.Error(err))
		return err
	}

	logger.Info("Stamina reset", zap.Int32("stamina", MaxStamina))
	return nil
}

// GetTimeToFullStamina 计算体力恢复到满需要的时间
func GetTimeToFullStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (time.Duration, error) {
	stamina, err := GetCurrentStamina(ctx, logger, db, statusRegistry)
	if err != nil || stamina.Stamina >= MaxStamina {
		return 0, err
	}
	return time.Duration(MaxStamina-stamina.Stamina) * StaminaRecoveryRate, nil
}

// calculateRecoveredStamina 计算自然恢复后的体力值
// 返回：新体力值、新刷新时间
func calculateRecoveredStamina(currentStamina int32, lastRefreshTime string) (int32, string) {
	if currentStamina >= MaxStamina {
		return currentStamina, lastRefreshTime
	}

	if lastRefreshTime == "" {
		return currentStamina, time.Now().Format(time.RFC3339)
	}

	lastRefresh, err := time.Parse(time.RFC3339, lastRefreshTime)
	if err != nil {
		return currentStamina, time.Now().Format(time.RFC3339)
	}

	recoveryPeriods := int32(time.Since(lastRefresh) / StaminaRecoveryRate)
	if recoveryPeriods <= 0 {
		return currentStamina, lastRefreshTime
	}

	newStamina := min(currentStamina+(recoveryPeriods*StaminaRecoveryAmount), MaxStamina)
	// 保存当前恢复时间，用于下次计算间隔，避免重复计算
	newRefreshTime := time.Now().Format(time.RFC3339)
	return newStamina, newRefreshTime
}
