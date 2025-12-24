package server

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AddExp 增加经验值（消耗体力时调用）
func AddExp(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, template TemplateManager, staminaAmount int32) error {
	if staminaAmount <= 0 {
		return nil
	}

	expGained := staminaAmount * ExpPerStamina

	playerLevelData := &PlayerLevelData{}
	if err := LoadUserData(ctx, logger, db, playerLevelData); err != nil {
		logger.Error("加载玩家等级数据失败", zap.Error(err))
		return err
	}

	playerLevelData.TotalExp += expGained
	if err := SaveUserData(ctx, logger, db, metrics, storageIndex, playerLevelData); err != nil {
		logger.Error("保存玩家等级数据失败", zap.Error(err))
		return err
	}

	logger.Info("玩家获得经验值",
		zap.Int32("stamina_consumed", staminaAmount),
		zap.Int32("exp_gained", expGained),
		zap.Int32("total_exp", playerLevelData.TotalExp),
		zap.Int32("current_exp", playerLevelData.Exp))

	return nil
}

// GetPlayerLevelData 获取玩家等级数据
func (s *ApiServer) GetPlayerLevelData(ctx context.Context, in *emptypb.Empty) (*game.PlayerLevelData, error) {
	playerLevelData := &PlayerLevelData{}
	if err := LoadUserData(ctx, s.logger, s.db, playerLevelData); err != nil {
		s.logger.Error("加载玩家等级数据失败", zap.Error(err))
		return nil, err
	}

	return &game.PlayerLevelData{
		Level:    playerLevelData.Level,
		Exp:      playerLevelData.Exp,
		TotalExp: playerLevelData.TotalExp,
	}, nil
}

// UpgradePlayerLevel 升级玩家等级
func (s *ApiServer) UpgradePlayerLevel(ctx context.Context, in *game.UpgradePlayerLevelRequest) (*game.UpgradePlayerLevelResponse, error) {
	playerLevelData := &PlayerLevelData{}
	if err := LoadUserData(ctx, s.logger, s.db, playerLevelData); err != nil {
		s.logger.Error("加载玩家等级数据失败", zap.Error(err))
		return &game.UpgradePlayerLevelResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	currentLevel := playerLevelData.Level
	totalExp := playerLevelData.TotalExp

	// 查询下一级所需经验值
	nextLevel := currentLevel + 1
	levelKey := strconv.FormatInt(int64(nextLevel), 10)
	levelInfo, exist := s.template.GetTplPlayerLevel().FindByKey(levelKey)
	if !exist {
		return &game.UpgradePlayerLevelResponse{
			Code: 2,
			Msg:  "已达到最高等级",
		}, nil
	}

	// 检查总经验值是否足够（ExpRequire 是升级到该等级所需的总经验值）
	if totalExp < levelInfo.ExpRequire {
		return &game.UpgradePlayerLevelResponse{
			Code: 3,
			Msg:  "经验值不足",
		}, nil
	}

	// 升级（不需要扣除经验值，因为 TotalExp 是累计值）
	playerLevelData.Level = nextLevel
	// Exp 更新为当前等级的经验值（总经验值减去当前等级所需的总经验值）
	playerLevelData.Exp = totalExp - levelInfo.ExpRequire

	// 发放升级奖励
	var reward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if levelInfo.Reward != "" {
		reward = GetReward(levelInfo.Reward, s.template.GetTplReward(), s.logger)
		if reward != nil {
			source := "player_level_up_" + levelKey
			var err error
			walletUpdateResult, inventoryUpdateResult, err = GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
			if err != nil {
				s.logger.Error("发放升级奖励失败", zap.Error(err))
				// 不返回错误，继续升级流程
			}
		}
	}

	// 保存等级数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, playerLevelData); err != nil {
		s.logger.Error("保存玩家等级数据失败", zap.Error(err))
		return &game.UpgradePlayerLevelResponse{
			Code: 4,
			Msg:  "保存数据失败",
		}, nil
	}

	// 提取更新后的数据
	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item
	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}
	if inventoryUpdateResult != nil {
		inventoryUpdated = inventoryUpdateResult.Updated
	}

	s.logger.Info("玩家升级成功",
		zap.Int32("old_level", currentLevel),
		zap.Int32("new_level", nextLevel),
		zap.Int32("remaining_exp", playerLevelData.Exp))

	return &game.UpgradePlayerLevelResponse{
		Code:             0,
		Msg:              "升级成功",
		Level:            nextLevel,
		Exp:              playerLevelData.Exp,
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}
