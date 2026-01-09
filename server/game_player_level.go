package server

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

func AddExp(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, template TemplateManager, staminaAmount int32) (int32, error) {
	if staminaAmount <= 0 {
		return 0, nil
	}

	expGained := staminaAmount * ExpPerStamina

	playerLevelData := &PlayerLevelData{}
	if err := LoadUserData(ctx, logger, db, playerLevelData); err != nil {
		logger.Error("加载玩家等级数据失败", zap.Error(err))
		return 0, err
	}

	playerLevelData.TotalExp += expGained
	if err := SaveUserData(ctx, logger, db, metrics, storageIndex, playerLevelData); err != nil {
		logger.Error("保存玩家等级数据失败", zap.Error(err))
		return 0, err
	}

	logger.Info("玩家获得经验值",
		zap.Int32("stamina_consumed", staminaAmount),
		zap.Int32("exp_gained", expGained),
		zap.Int32("total_exp", playerLevelData.TotalExp))

	return expGained, nil
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
	oldLevel := currentLevel

	rewards := make([]*game.Reward, 0)

	for {
		nextLevel := currentLevel + 1
		levelKey := strconv.FormatInt(int64(nextLevel), 10)
		levelInfo, exist := s.template.GetTplPlayerLevel().FindByKey(levelKey)
		if !exist {
			break
		}

		if totalExp < levelInfo.ExpRequire {
			break
		}

		currentLevel = nextLevel

		if levelInfo.Reward != "" {
			reward := GetReward(levelInfo.Reward, s.template.GetTplReward(), s.logger)
			if reward != nil {
				rewards = append(rewards, reward)
			}
		}
	}

	if currentLevel == oldLevel {
		nextLevel := currentLevel + 1
		levelKey := strconv.FormatInt(int64(nextLevel), 10)
		levelInfo, exist := s.template.GetTplPlayerLevel().FindByKey(levelKey)
		if !exist {
			return &game.UpgradePlayerLevelResponse{
				Code: 2,
				Msg:  "已达到最高等级",
			}, nil
		}

		return &game.UpgradePlayerLevelResponse{
			Code:             3,
			Msg:              "经验值不足，需要 " + strconv.FormatInt(int64(levelInfo.ExpRequire-totalExp), 10) + " 点经验值",
			TotalExp:         playerLevelData.TotalExp,
			Level:            playerLevelData.Level,
			Reward:           nil,
			WalletUpdated:    nil,
			InventoryUpdated: nil,
		}, nil
	}

	playerLevelData.Level = currentLevel

	var mergedReward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if len(rewards) > 0 {
		source := "player_level_up"
		var err error
		mergedReward, walletUpdateResult, inventoryUpdateResult, err = GrantMergedRewards(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, rewards, source)
		if err != nil {
			s.logger.Error("发放升级奖励失败", zap.Error(err))
		}
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, playerLevelData); err != nil {
		s.logger.Error("保存玩家等级数据失败", zap.Error(err))
		return &game.UpgradePlayerLevelResponse{
			Code: 4,
			Msg:  "保存数据失败",
		}, nil
	}

	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item
	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}
	if inventoryUpdateResult != nil {
		inventoryUpdated = inventoryUpdateResult.Updated
	}

	s.logger.Info("玩家升级成功",
		zap.Int32("old_level", oldLevel),
		zap.Int32("new_level", currentLevel),
		zap.Int("levels_upgraded", int(currentLevel-oldLevel)))

	return &game.UpgradePlayerLevelResponse{
		Code:             0,
		Msg:              "升级成功",
		Level:            currentLevel,
		TotalExp:         playerLevelData.TotalExp,
		Reward:           mergedReward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}
