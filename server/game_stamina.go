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
	if currentStamina == 0 && lastRefreshTime.IsZero() {
		currentStamina = MaxStamina
		lastRefreshTime = time.Now().UTC()

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
			LastRefreshTime: lastRefreshTime.Format(time.RFC3339),
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
		LastRefreshTime: lastRefreshTime.Format(time.RFC3339),
	}, nil
}

// ConsumeStamina 消耗体力
func ConsumeStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, amount int32) (*game.StaminaData, error) {
	if amount <= 0 {
		return nil, errors.New("invalid stamina amount")
	}

	// 先调用 GetCurrentStamina 获取当前体力（已包含恢复计算）
	staminaData, err := GetCurrentStamina(ctx, logger, db, statusRegistry)
	if err != nil {
		return nil, err
	}

	currentStamina := staminaData.Stamina
	lastRefreshTime := staminaData.LastRefreshTime

	// 检查体力是否足够
	if currentStamina < amount {
		logger.Warn("Insufficient stamina", zap.Int32("required", amount), zap.Int32("current", currentStamina))
		return nil, errors.New("insufficient stamina")
	}

	// 扣除体力
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	metadata, _ := json.Marshal(map[string]interface{}{
		"source": "stamina_consume",
	})
	walletUpdates := []*walletUpdate{{
		UserID:    userID,
		Changeset: map[string]int64{"stamina": -int64(amount)},
		Metadata:  string(metadata),
	}}
	if _, err := UpdateWallets(ctx, logger, db, walletUpdates, true); err != nil {
		logger.Error("扣除体力失败", zap.Error(err))
		return nil, err
	}

	logger.Info("Stamina consumed", zap.Int32("amount", amount), zap.Int32("remaining", currentStamina-amount))
	return &game.StaminaData{
		Stamina:         currentStamina - amount,
		LastRefreshTime: lastRefreshTime,
	}, nil
}

// 补充体力（通过道具或购买）
func RefillStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, amount int32) (*game.StaminaData, error) {
	if amount <= 0 {
		return nil, errors.New("invalid stamina amount")
	}

	// 先调用 GetCurrentStamina 获取当前体力（已包含恢复计算）
	staminaData, err := GetCurrentStamina(ctx, logger, db, statusRegistry)
	if err != nil {
		return nil, err
	}

	oldStamina := staminaData.Stamina
	lastRefreshTime := staminaData.LastRefreshTime

	// 补充体力（不设上限，可以超过最大值）
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
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
		return nil, err
	}

	logger.Info("Stamina refilled", zap.Int32("amount", amount), zap.Int32("old", oldStamina), zap.Int32("new", oldStamina+amount))
	return &game.StaminaData{
		Stamina:         oldStamina + amount,
		LastRefreshTime: lastRefreshTime,
	}, nil
}

// 重置体力到最大值（用于测试或管理员操作）
func ResetStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (*game.StaminaData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从钱包读取当前体力值
	wallet, exists, err := GetWallet(ctx, logger, db, userID)
	if err != nil {
		logger.Error("读取钱包数据失败", zap.Error(err))
		return nil, err
	}
	if !exists {
		return nil, errors.New("用户不存在")
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
		return nil, err
	}

	// 更新 UserMeta 刷新时间
	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		logger.Error("读取 UserMeta 失败", zap.Error(err))
		return nil, err
	}
	lastRefreshTime := time.Now().UTC()
	userMeta.StaminaLastRefreshTime = lastRefreshTime
	if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
		logger.Error("保存重置后的刷新时间失败", zap.Error(err))
		return nil, err
	}

	logger.Info("Stamina reset", zap.Int32("stamina", MaxStamina))
	return &game.StaminaData{
		Stamina:         MaxStamina,
		LastRefreshTime: lastRefreshTime.Format(time.RFC3339),
	}, nil
}

// 计算体力恢复到满需要的时间
func GetTimeToFullStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (time.Duration, error) {
	stamina, err := GetCurrentStamina(ctx, logger, db, statusRegistry)
	if err != nil || stamina.Stamina >= MaxStamina {
		return 0, err
	}
	return time.Duration(MaxStamina-stamina.Stamina) * StaminaRecoveryRate, nil
}

// 计算自然恢复后的体力值
// 返回：新体力值、新刷新时间
func calculateRecoveredStamina(currentStamina int32, lastRefreshTime time.Time) (int32, time.Time) {
	if currentStamina >= MaxStamina {
		return currentStamina, lastRefreshTime
	}

	if lastRefreshTime.IsZero() {
		return currentStamina, time.Now().UTC()
	}

	recoveryPeriods := int32(time.Since(lastRefreshTime) / StaminaRecoveryRate)
	if recoveryPeriods <= 0 {
		return currentStamina, lastRefreshTime
	}

	newStamina := min(currentStamina+(recoveryPeriods*StaminaRecoveryAmount), MaxStamina)
	// 保存当前恢复时间，用于下次计算间隔，避免重复计算
	newRefreshTime := time.Now().UTC()
	return newStamina, newRefreshTime
}
