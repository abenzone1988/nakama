package server

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

// compareLevelId 比较两个关卡 ID 的大小，返回 true 表示 id1 > id2
func compareLevelId(id1, id2 string) bool {
	// 解析两个 ID 的数字部分进行比较
	num1, err1 := strconv.ParseInt(id1[1:], 10, 32)
	num2, err2 := strconv.ParseInt(id2[1:], 10, 32)

	if err1 != nil || err2 != nil {
		// 如果解析失败，使用字符串比较
		return id1 > id2
	}

	return num1 > num2
}

func (s *ApiServer) EndBattle(ctx context.Context, in *game.EndBattleRequest) (*game.EndBattleResponse, error) {
	if in.GetProgress() > 100 || in.GetProgress() <= 0 {
		return &game.EndBattleResponse{
			Code: 1,
			Msg:  "进度错误",
		}, nil
	}

	var rewardId string
	var source string
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	switch in.GetType() {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(in.GetLevelId())
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		rewardId = levelInfo.WinRewards
		source = "battle_normal_" + in.GetLevelId()

	case game.BattleType_BATTLE_TYPE_GOLDEN:
		activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(in.GetLevelId())
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		rewardId = activityInfo.RewardID
		source = "battle_golden_" + in.GetLevelId()

	default:
		return &game.EndBattleResponse{
			Code: 2,
			Msg:  "关卡不存在",
		}, nil
	}

	//扣除体力值
	if err := ConsumeStamina(ctx, s.logger, s.db, s.metrics, s.storageIndex, BattleCostStamina); err != nil {
		return &game.EndBattleResponse{
			Code: 3,
			Msg:  "体力扣除失败: " + err.Error(),
		}, nil
	}

	reward := GetReward(rewardId, s.template.GetTplReward(), s.logger)
	if reward != nil {
		// 应用进度折扣
		applyProgressToReward(reward, in.GetProgress())

		// 使用统一的奖励发放函数（支持特殊道具处理）
		var err error
		walletUpdateResult, inventoryUpdateResult, err = GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)

		if err != nil {
			return &game.EndBattleResponse{
				Code: 4,
				Msg:  "奖励发放失败: " + err.Error(),
			}, nil
		}
	}

	// 更新关卡进度（仅对普通关卡）
	if in.GetType() == game.BattleType_BATTLE_TYPE_NORMAL {
		if err := updateLevelProgress(ctx, s.logger, s.db, s.metrics, s.storageIndex, in.GetLevelId(), in.GetProgress()); err != nil {
			s.logger.Error("更新关卡进度失败",
				zap.Error(err),
				zap.String("level_id", in.GetLevelId()),
				zap.Int32("progress", in.GetProgress()))
			// 不影响战斗结算，仅记录错误
		}
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

	return &game.EndBattleResponse{
		Code:             0,
		Msg:              "通过成功",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

func GetReward(rewardId string, tplReward *template.TableTplReward, logger *zap.Logger) *game.Reward {
	reward, exist := tplReward.FindByKey(rewardId)
	if !exist {
		return nil
	}

	items := parseItems(reward.Items, logger)

	return &game.Reward{
		Wallet: &game.Wallet{
			Coin: reward.Coin,
			Gem:  reward.Gem,
			Ad:   reward.Coupon,
		},
		Items: items,
	}
}

func parseItems(itemsString string, logger *zap.Logger) []*game.Item {
	if itemsString == "" {
		return nil
	}

	pairs := strings.Split(itemsString, ",")
	result := make([]*game.Item, 0, len(pairs))

	for _, pair := range pairs {
		trimmedPair := strings.TrimSpace(pair)
		if trimmedPair == "" {
			continue
		}

		keyValue := strings.Split(trimmedPair, "_")
		if len(keyValue) != 2 {
			if logger != nil {
				logger.Error("Invalid key-value pair", zap.String("pair", trimmedPair))
			}
			continue
		}

		id := strings.TrimSpace(keyValue[0])
		numStr := strings.TrimSpace(keyValue[1])

		num, err := strconv.ParseInt(numStr, 10, 32)
		if err != nil {
			if logger != nil {
				logger.Error("Warning: Invalid key-value pair", zap.String("pair", trimmedPair), zap.Error(err))
			}
			continue
		}

		result = append(result, &game.Item{
			Id:  id,
			Num: int32(num),
		})
	}

	return result
}

func applyProgressToReward(reward *game.Reward, progress int32) {
	if reward == nil || progress <= 0 {
		return
	}
	if progress > 100 {
		progress = 100
	}

	if reward.Wallet != nil {
		reward.Wallet.Coin = reward.Wallet.Coin * progress / 100
		reward.Wallet.Gem = reward.Wallet.Gem * progress / 100
		reward.Wallet.Ad = reward.Wallet.Ad * progress / 100
	}

	if reward.Items != nil {
		for _, item := range reward.Items {
			if item != nil {
				item.Num = item.Num * progress / 100
			}
		}
	}
}

// updateLevelProgress 更新关卡进度到 storage
func updateLevelProgress(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, levelId string, progress int32) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载关卡数据
	levelData, err := GetLevelData(ctx, logger, db, metrics, storageIndex)
	if err != nil {
		logger.Error("加载关卡数据失败", zap.Error(err))
		return err
	}

	// 更新最大关卡
	needUpdate := false
	if compareLevelId(levelId, levelData.CurLevelId) {
		// 新关卡更大，更新关卡和进度
		oldLevelId := levelData.CurLevelId
		levelData.CurLevelId = levelId
		levelData.BestProgress = progress
		needUpdate = true
		logger.Info("更新最大关卡",
			zap.String("old_level_id", oldLevelId),
			zap.String("new_level_id", levelId),
			zap.Int32("progress", progress))
	} else if levelId == levelData.CurLevelId && progress > levelData.BestProgress {
		// 同一关卡，更新最好进度
		oldProgress := levelData.BestProgress
		levelData.BestProgress = progress
		needUpdate = true
		logger.Info("更新最好进度",
			zap.String("level_id", levelId),
			zap.Int32("old_progress", oldProgress),
			zap.Int32("new_progress", progress))
	}

	if needUpdate {
		// 保存更新的关卡数据
		if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, levelData); err != nil {
			logger.Error("保存关卡数据失败", zap.Error(err))
			return err
		}
		logger.Info("关卡进度已更新",
			zap.String("cur_level_id", levelData.CurLevelId),
			zap.Int32("best_progress", levelData.BestProgress))
	}

	return nil
}
