package server

import (
	"context"
	"database/sql"
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
	return GetCurrentStamina(ctx, s.logger, s.db, s.metrics, s.storageIndex)
}

// GetCurrentStamina 获取当前体力值（自动计算恢复）
func GetCurrentStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex) (*game.StaminaData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载体力数据
	staminaData := &StaminaData{}
	if err := LoadData(ctx, logger, db, userID, staminaData); err != nil {
		logger.Error("加载体力数据失败", zap.Error(err))
		return nil, err
	}

	// 如果数据为空，初始化并保存
	if staminaData.Stamina == 0 && staminaData.LastRefreshTime == "" {
		staminaData.Init()
		if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, staminaData); err != nil {
			logger.Error("保存初始化的体力数据失败", zap.Error(err))
			return nil, err
		}
		logger.Info("Stamina initialized", zap.Int32("stamina", MaxStamina))
		return &game.StaminaData{
			Stamina:         staminaData.Stamina,
			LastRefreshTime: staminaData.LastRefreshTime,
		}, nil
	}

	// 计算自然恢复
	oldStamina := staminaData.Stamina
	calculateRecoveredStamina(staminaData)

	// 更新体力（如果有变化）
	if staminaData.Stamina != oldStamina {
		if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, staminaData); err != nil {
			logger.Error("保存恢复后的体力数据失败", zap.Error(err))
			return nil, err
		}
		logger.Info("Stamina auto-recovered",
			zap.Int32("old_stamina", oldStamina),
			zap.Int32("new_stamina", staminaData.Stamina))
	}

	return &game.StaminaData{
		Stamina:         staminaData.Stamina,
		LastRefreshTime: staminaData.LastRefreshTime,
	}, nil
}

// ConsumeStamina 消耗体力
func ConsumeStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, amount int32) error {
	if amount <= 0 {
		return errors.New("invalid stamina amount")
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载体力数据
	staminaData := &StaminaData{}
	if err := LoadData(ctx, logger, db, userID, staminaData); err != nil {
		logger.Error("加载体力数据失败", zap.Error(err))
		return err
	}

	// 如果数据为空，初始化但不扣除本次消耗
	if staminaData.Stamina == 0 && staminaData.LastRefreshTime == "" {
		staminaData.Init()
		if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, staminaData); err != nil {
			logger.Error("保存初始化的体力数据失败", zap.Error(err))
			return err
		}
		logger.Info("体力首次初始化，本次不扣除",
			zap.Int32("stamina", MaxStamina),
			zap.Int32("skip_amount", amount))
		return nil
	}

	// 计算自然恢复后的体力
	calculateRecoveredStamina(staminaData)

	// 检查体力是否足够
	if staminaData.Stamina < amount {
		logger.Warn("Insufficient stamina", zap.Int32("required", amount), zap.Int32("current", staminaData.Stamina))
		return errors.New("insufficient stamina")
	}

	// 扣除体力
	staminaData.Stamina -= amount

	// 保存更新的体力数据
	if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, staminaData); err != nil {
		logger.Error("保存消耗后的体力数据失败", zap.Error(err))
		return err
	}

	logger.Info("Stamina consumed", zap.Int32("amount", amount), zap.Int32("remaining", staminaData.Stamina))
	return nil
}

// RefillStamina 补充体力（通过道具或购买）
func RefillStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, amount int32) error {
	if amount <= 0 {
		return errors.New("invalid stamina amount")
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载体力数据
	staminaData := &StaminaData{}
	if err := LoadData(ctx, logger, db, userID, staminaData); err != nil {
		logger.Error("加载体力数据失败", zap.Error(err))
		return err
	}

	// 如果数据为空，初始化
	if staminaData.Stamina == 0 && staminaData.LastRefreshTime == "" {
		staminaData.Init()
	}

	oldStamina := staminaData.Stamina
	staminaData.Stamina = min(staminaData.Stamina+amount, MaxStamina)

	// 保存更新的体力数据
	if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, staminaData); err != nil {
		logger.Error("保存补充后的体力数据失败", zap.Error(err))
		return err
	}

	logger.Info("Stamina refilled", zap.Int32("amount", amount), zap.Int32("old", oldStamina), zap.Int32("new", staminaData.Stamina))
	return nil
}

// ResetStamina 重置体力到最大值（用于测试或管理员操作）
func ResetStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载体力数据
	staminaData := &StaminaData{}
	if err := LoadData(ctx, logger, db, userID, staminaData); err != nil {
		logger.Error("加载体力数据失败", zap.Error(err))
		return err
	}

	// 重置体力
	staminaData.Stamina = MaxStamina
	staminaData.LastRefreshTime = time.Now().Format(time.RFC3339)

	// 保存重置后的体力数据
	if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, staminaData); err != nil {
		logger.Error("保存重置后的体力数据失败", zap.Error(err))
		return err
	}

	logger.Info("Stamina reset", zap.Int32("stamina", MaxStamina))
	return nil
}

// GetTimeToFullStamina 计算体力恢复到满需要的时间
func GetTimeToFullStamina(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex) (time.Duration, error) {
	stamina, err := GetCurrentStamina(ctx, logger, db, metrics, storageIndex)
	if err != nil || stamina.Stamina >= MaxStamina {
		return 0, err
	}
	return time.Duration(MaxStamina-stamina.Stamina) * StaminaRecoveryRate, nil
}

// calculateRecoveredStamina 计算自然恢复后的体力值（直接修改传入的结构体）
func calculateRecoveredStamina(staminaData *StaminaData) {
	if staminaData == nil || staminaData.Stamina >= MaxStamina {
		return
	}

	lastRefresh, err := time.Parse(time.RFC3339, staminaData.LastRefreshTime)
	if err != nil {
		staminaData.LastRefreshTime = time.Now().Format(time.RFC3339)
		return
	}

	recoveryPeriods := int32(time.Since(lastRefresh) / StaminaRecoveryRate)
	if recoveryPeriods <= 0 {
		return
	}

	newStamina := min(staminaData.Stamina+(recoveryPeriods*StaminaRecoveryAmount), MaxStamina)
	// LastRefreshTime 保存当前恢复时间，用于下次计算间隔，避免重复计算
	staminaData.Stamina = newStamina
	staminaData.LastRefreshTime = time.Now().Format(time.RFC3339)
}
