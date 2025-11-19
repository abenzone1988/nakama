package server

import (
	"context"
	"errors"
	"time"

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
	return GetCurrentStamina(ctx, s.logger)
}

// GetCurrentStamina 获取当前体力值（自动计算恢复）
func GetCurrentStamina(ctx context.Context, logger *zap.Logger) (*game.StaminaData, error) {
	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		logger.Error("Failed to get UserMeta", zap.Error(err))
		return nil, err
	}

	// 首次访问：初始化为满体力
	if userMeta.Stamina == nil {
		initializeStamina(userMeta)
		_ = UpdateUserMeta(ctx, func(meta *game.UserMeta) error { return nil })
		logger.Info("Stamina initialized", zap.Int32("stamina", MaxStamina))
		return userMeta.Stamina, nil
	}

	// 计算自然恢复
	stamina := calculateRecoveredStamina(userMeta.Stamina)

	// 更新体力（如果有变化）
	if stamina.Stamina != userMeta.Stamina.Stamina {
		oldStamina := userMeta.Stamina.Stamina
		_ = UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
			meta.Stamina = stamina
			return nil
		})
		logger.Info("Stamina auto-recovered",
			zap.Int32("old_stamina", oldStamina),
			zap.Int32("new_stamina", stamina.Stamina))
	}

	return stamina, nil
}

// ConsumeStamina 消耗体力
func ConsumeStamina(ctx context.Context, logger *zap.Logger, amount int32) error {
	if amount <= 0 {
		return errors.New("invalid stamina amount")
	}

	// 先检查体力是否已初始化
	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		return err
	}

	// 体力未初始化，先初始化但不扣除本次消耗
	if userMeta.Stamina == nil {
		return UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
			initializeStamina(meta)
			logger.Info("体力首次初始化，本次不扣除",
				zap.Int32("stamina", MaxStamina),
				zap.Int32("skip_amount", amount))
			return nil
		})
	}

	// 计算自然恢复后的体力
	currentStamina := calculateRecoveredStamina(userMeta.Stamina)

	// 检查体力是否足够
	if currentStamina.Stamina < amount {
		logger.Warn("Insufficient stamina", zap.Int32("required", amount), zap.Int32("current", currentStamina.Stamina))
		return errors.New("insufficient stamina")
	}

	// 扣除体力
	return UpdateUserMeta(ctx, func(userMeta *game.UserMeta) error {
		// 先应用自然恢复
		userMeta.Stamina = currentStamina
		// 再扣除体力
		userMeta.Stamina.Stamina -= amount
		logger.Info("Stamina consumed", zap.Int32("amount", amount), zap.Int32("remaining", userMeta.Stamina.Stamina))
		return nil
	})
}

// RefillStamina 补充体力（通过道具或购买）
func RefillStamina(ctx context.Context, logger *zap.Logger, amount int32) error {
	if amount <= 0 {
		return errors.New("invalid stamina amount")
	}

	return UpdateUserMeta(ctx, func(userMeta *game.UserMeta) error {
		if userMeta.Stamina == nil {
			initializeStamina(userMeta)
		}
		oldStamina := userMeta.Stamina.Stamina
		userMeta.Stamina.Stamina = min(userMeta.Stamina.Stamina+amount, MaxStamina)
		logger.Info("Stamina refilled", zap.Int32("amount", amount), zap.Int32("old", oldStamina), zap.Int32("new", userMeta.Stamina.Stamina))
		return nil
	})
}

// ResetStamina 重置体力到最大值（用于测试或管理员操作）
func ResetStamina(ctx context.Context, logger *zap.Logger) error {
	return UpdateUserMeta(ctx, func(userMeta *game.UserMeta) error {
		if userMeta.Stamina == nil {
			userMeta.Stamina = &game.StaminaData{}
		}
		userMeta.Stamina.Stamina = MaxStamina
		userMeta.Stamina.LastRefreshTime = time.Now().Format(time.RFC3339)
		logger.Info("Stamina reset", zap.Int32("stamina", MaxStamina))
		return nil
	})
}

// GetTimeToFullStamina 计算体力恢复到满需要的时间
func GetTimeToFullStamina(ctx context.Context, logger *zap.Logger) (time.Duration, error) {
	stamina, err := GetCurrentStamina(ctx, logger)
	if err != nil || stamina.Stamina >= MaxStamina {
		return 0, err
	}
	return time.Duration(MaxStamina-stamina.Stamina) * StaminaRecoveryRate, nil
}

// initializeStamina 初始化体力数据
func initializeStamina(userMeta *game.UserMeta) {
	userMeta.Stamina = &game.StaminaData{
		Stamina:         MaxStamina,
		LastRefreshTime: time.Now().Format(time.RFC3339),
	}
}

// calculateRecoveredStamina 计算自然恢复后的体力值
func calculateRecoveredStamina(stamina *game.StaminaData) *game.StaminaData {
	if stamina == nil || stamina.Stamina >= MaxStamina {
		return stamina
	}

	lastRefresh, err := time.Parse(time.RFC3339, stamina.LastRefreshTime)
	if err != nil {
		return &game.StaminaData{Stamina: stamina.Stamina, LastRefreshTime: time.Now().Format(time.RFC3339)}
	}

	recoveryPeriods := int32(time.Since(lastRefresh) / StaminaRecoveryRate)
	if recoveryPeriods <= 0 {
		return stamina
	}

	newStamina := min(stamina.Stamina+(recoveryPeriods*StaminaRecoveryAmount), MaxStamina)
	// LastRefreshTime 保存当前恢复时间，用于下次计算间隔，避免重复计算
	return &game.StaminaData{
		Stamina:         newStamina,
		LastRefreshTime: time.Now().Format(time.RFC3339),
	}
}
