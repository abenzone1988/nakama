package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
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
	if err := ConsumeStamina(ctx, s.logger, 5); err != nil {
		return &game.EndBattleResponse{
			Code: -1,
			Msg:  "体力扣除失败: " + err.Error(),
		}, nil
	}

	reward := GetReward(rewardId, s.template.GetTplReward(), s.logger)
	if reward != nil {
		// 发放奖励到钱包和背包
		applyProgressToReward(reward, in.GetProgress())
		var err error
		walletUpdateResult, inventoryUpdateResult, err = grantReward(ctx, s.logger, s.db, reward, source)
		if err != nil {
			return &game.EndBattleResponse{
				Code: -1,
				Msg:  "奖励发放失败: " + err.Error(),
			}, nil
		}
	}
	return &game.EndBattleResponse{
		Code:            0,
		Msg:             "通过成功",
		Reward:          reward,
		WalletUpdate:    walletUpdateResult,
		InventoryUpdate: inventoryUpdateResult,
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

func grantReward(ctx context.Context, logger *zap.Logger, db *sql.DB, reward *game.Reward, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	if reward == nil {
		return nil, nil, nil
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	// 处理钱包奖励
	if reward.Wallet != nil {
		walletChangeset := make(map[string]int64)
		if reward.Wallet.Coin > 0 {
			walletChangeset["coin"] = int64(reward.Wallet.Coin)
		}
		if reward.Wallet.Gem > 0 {
			walletChangeset["gem"] = int64(reward.Wallet.Gem)
		}
		if reward.Wallet.Ad > 0 {
			walletChangeset["ad"] = int64(reward.Wallet.Ad)
		}

		if len(walletChangeset) > 0 {
			metadata, _ := json.Marshal(map[string]interface{}{
				"source": source,
			})
			walletUpdates := []*walletUpdate{
				{
					UserID:    userID,
					Changeset: walletChangeset,
					Metadata:  string(metadata),
				},
			}
			results, err := UpdateWallets(ctx, logger, db, walletUpdates, true)
			if err != nil {
				logger.Error("发放钱包奖励失败", zap.Error(err))
				return nil, nil, err
			}

			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToInt32(results[0].Previous),
				Updated:  convertMapInt64ToInt32(results[0].Updated),
			}
		}
	}

	// 处理道具奖励
	if reward.Items != nil && len(reward.Items) > 0 {
		inventoryChangeset := make(map[string]int64)
		for _, item := range reward.Items {
			if item != nil && item.Num > 0 {
				inventoryChangeset[item.Id] = int64(item.Num)
			}
		}

		if len(inventoryChangeset) > 0 {
			metadata, _ := json.Marshal(map[string]interface{}{
				"source": source,
			})
			inventoryUpdates := []*inventoryUpdate{
				{
					UserID:    userID,
					Changeset: inventoryChangeset,
					Metadata:  string(metadata),
				},
			}
			results, err := UpdateInventories(ctx, logger, db, inventoryUpdates, true)
			if err != nil {
				logger.Error("发放道具奖励失败", zap.Error(err))
				return walletUpdateResult, nil, err
			}
			inventoryUpdateResult = &game.InventoryUpdateResult{
				Previous: convertMapInt64ToInt32(results[0].Previous),
				Updated:  convertMapInt64ToInt32(results[0].Updated),
			}
		}
	}

	return walletUpdateResult, inventoryUpdateResult, nil
}
