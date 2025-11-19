package server

import (
	"context"
	"strconv"
	"strings"

	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

func (s *ApiServer) EndBattle(ctx context.Context, in *game.EndBattleRequest) (*game.EndBattleResponse, error) {
	if in.GetProgress() > 100 || in.GetProgress() < 0 {
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
	if err := ConsumeStamina(ctx, s.logger, BattleCostStamina); err != nil {
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
		walletUpdateResult, inventoryUpdateResult, err = GrantReward(ctx, s.logger, s.db, s.template, reward, source)

		if err != nil {
			return &game.EndBattleResponse{
				Code: 4,
				Msg:  "奖励发放失败: " + err.Error(),
			}, nil
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
