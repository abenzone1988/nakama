package server

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

const (
	MoppingStage1MaxTimes = 3 // 第一阶段最大次数（扣体力）
	MoppingStage2MaxTimes = 5 // 第二阶段最大次数（扣体力+广告）
	MoppingCostStamina    = 5 // 扫荡消耗的体力值
	OnHookMaxHours        = 8 // 挂机最大领取小时数
)

// ClaimMoppingReward 领取扫荡奖励
func (s *ApiServer) ClaimMoppingReward(ctx context.Context, in *game.ClaimMoppingRewardRequest) (*game.ClaimMoppingRewardResponse, error) {
	// 检查关卡 ID 是否有效

	// 获取关卡数据
	levelData, err := GetLevelData(ctx, s.logger, s.db, s.metrics, s.storageIndex)
	if err != nil {
		return &game.ClaimMoppingRewardResponse{
			Code: 3,
			Msg:  "获取关卡数据失败",
		}, nil
	}

	levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(levelData.CurLevelId)
	if !exist {
		return &game.ClaimMoppingRewardResponse{
			Code: 1,
			Msg:  "关卡不存在",
		}, nil
	}

	// 检查扫荡奖励配置是否存在
	if levelInfo.MoppingReward == "" {
		return &game.ClaimMoppingRewardResponse{
			Code: 2,
			Msg:  "该关卡没有扫荡奖励",
		}, nil
	}

	// 检查并刷新每日次数
	needsReset, err := checkAndResetDailyMoppingTimes(levelData, s.logger)
	if err != nil {
		s.logger.Error("检查每日重置失败", zap.Error(err))
		return &game.ClaimMoppingRewardResponse{
			Code: 4,
			Msg:  "检查每日重置失败: " + err.Error(),
		}, nil
	}

	if needsReset {
		// 重置每日次数
		resetMoppingTimes(levelData)
		userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
		if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, levelData); err != nil {
			return &game.ClaimMoppingRewardResponse{
				Code: 5,
				Msg:  "重置每日次数失败: " + err.Error(),
			}, nil
		}
	}

	// 判断当前处于哪个阶段
	stage1Times := levelData.HasMoppingTimes
	stage2Times := levelData.HasMoppingTimesForAdv

	var isStage2 bool
	var needsAd bool

	if stage1Times < MoppingStage1MaxTimes {
		// 第一阶段：只扣体力
		isStage2 = false
		needsAd = false
	} else if stage2Times < MoppingStage2MaxTimes {
		// 第二阶段：扣体力 + 广告
		isStage2 = true
		needsAd = !in.GetWatchedAd() // 如果没看广告，需要扣除 ad 值
	} else {
		// 已达到每日上限
		return &game.ClaimMoppingRewardResponse{
			Code: 6,
			Msg:  "今日扫荡次数已用完",
		}, nil
	}

	// 扣除体力
	if err := ConsumeStamina(ctx, s.logger, s.db, s.statusRegistry, MoppingCostStamina); err != nil {
		return &game.ClaimMoppingRewardResponse{
			Code: 7,
			Msg:  "体力不足: " + err.Error(),
		}, nil
	}

	// 第二阶段且未观看广告，需要扣除 ad 值
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if needsAd {
		// 扣除 1 个广告券
		userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
		metadata, _ := json.Marshal(map[string]interface{}{"source": "mopping_ad_" + levelData.CurLevelId})

		walletChangeset := make(map[string]int64)
		walletChangeset["ad"] = -1

		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: walletChangeset,
			Metadata:  string(metadata),
		}}

		results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
		if err != nil {
			return &game.ClaimMoppingRewardResponse{
				Code: 8,
				Msg:  "广告券不足，请观看广告或购买广告券",
			}, nil
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 获取扫荡奖励
	reward := GetReward(levelInfo.MoppingReward, s.template.GetTplReward(), s.logger)
	if reward != nil {
		// 发放奖励
		var err error
		source := "mopping_" + levelData.CurLevelId

		// 如果之前扣除了 ad，需要合并钱包更新结果
		if walletUpdateResult != nil {
			// 先发放奖励
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
			if err != nil {
				return &game.ClaimMoppingRewardResponse{
					Code: 9,
					Msg:  "奖励发放失败: " + err.Error(),
				}, nil
			}
			// 合并钱包更新结果（扣除的 ad 值已经在 walletUpdateResult 中）
			if wResult != nil {
				walletUpdateResult.Updated = wResult.Updated
			}
			inventoryUpdateResult = iResult
		} else {
			walletUpdateResult, inventoryUpdateResult, err = GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
			if err != nil {
				return &game.ClaimMoppingRewardResponse{
					Code: 9,
					Msg:  "奖励发放失败: " + err.Error(),
				}, nil
			}
		}
	}

	// 更新扫荡次数
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	if isStage2 {
		levelData.HasMoppingTimesForAdv++
	} else {
		levelData.HasMoppingTimes++
	}
	levelData.LastMoppingTimestamp = time.Now().Format(time.RFC3339)

	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, levelData); err != nil {
		s.logger.Error("更新扫荡次数失败", zap.Error(err))
		return &game.ClaimMoppingRewardResponse{
			Code: 10,
			Msg:  "更新扫荡次数失败: " + err.Error(),
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

	return &game.ClaimMoppingRewardResponse{
		Code:                  0,
		Msg:                   "扫荡成功",
		Reward:                reward,
		WalletUpdated:         walletUpdated,
		InventoryUpdated:      inventoryUpdated,
		HasMoppingTimes:       levelData.HasMoppingTimes,
		HasMoppingTimesForAdv: levelData.HasMoppingTimesForAdv,
	}, nil
}

// checkAndResetDailyMoppingTimes 检查并重置每日扫荡次数
// 如果跨天了，返回 true 表示需要重置
func checkAndResetDailyMoppingTimes(levelData *LevelData, logger *zap.Logger) (bool, error) {
	if levelData == nil {
		return false, errors.New("levelData is nil")
	}

	// 如果从未扫荡过，不需要重置
	if levelData.LastMoppingTimestamp == "" {
		return false, nil
	}

	// 解析最后扫荡时间
	lastMoppingTime, err := time.Parse(time.RFC3339, levelData.LastMoppingTimestamp)
	if err != nil {
		logger.Warn("解析最后扫荡时间失败，将重置次数",
			zap.Error(err),
			zap.String("timestamp", levelData.LastMoppingTimestamp))
		// 解析失败，重置次数
		resetMoppingTimes(levelData)
		return true, nil
	}

	// 检查是否跨天（使用本地时区）
	now := time.Now()
	lastDay := lastMoppingTime.Truncate(24 * time.Hour)
	today := now.Truncate(24 * time.Hour)

	if today.After(lastDay) {
		// 跨天了，重置次数
		logger.Info("检测到跨天，重置扫荡次数",
			zap.Time("last_mopping_time", lastMoppingTime),
			zap.Time("now", now))
		resetMoppingTimes(levelData)
		return true, nil
	}

	return false, nil
}

// resetMoppingTimes 重置扫荡次数
func resetMoppingTimes(levelData *LevelData) {
	levelData.HasMoppingTimes = 0
	levelData.HasMoppingTimesForAdv = 0
}

// ClaimOnHookReward 领取挂机（巡逻）奖励
func (s *ApiServer) ClaimOnHookReward(ctx context.Context, in *game.ClaimOnHookRewardRequest) (*game.ClaimOnHookRewardResponse, error) {
	// 获取关卡数据
	levelData, err := GetLevelData(ctx, s.logger, s.db, s.metrics, s.storageIndex)
	if err != nil {
		return &game.ClaimOnHookRewardResponse{
			Code: 1,
			Msg:  "获取关卡数据失败",
		}, nil
	}

	// 获取当前关卡配置
	levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(levelData.CurLevelId)
	if !exist {
		return &game.ClaimOnHookRewardResponse{
			Code: 3,
			Msg:  "关卡不存在",
		}, nil
	}

	// 检查是否配置了挂机奖励
	if levelInfo.OnHookRewardID == "" {
		return &game.ClaimOnHookRewardResponse{
			Code: 4,
			Msg:  "当前关卡没有挂机奖励",
		}, nil
	}

	// 解析上次领取时间
	var hours int
	var newLastGetTime time.Time
	now := time.Now()

	if levelData.LastGetOnHookTimestamp == "" {
		// 首次领取，直接给予最大8小时奖励
		hours = OnHookMaxHours
		newLastGetTime = now
		s.logger.Info("首次领取挂机奖励，发放最大时长奖励",
			zap.Int("hours", OnHookMaxHours))
	} else {
		// 非首次领取，计算经过的时间
		lastGetTime, err := time.Parse(time.RFC3339, levelData.LastGetOnHookTimestamp)
		if err != nil {
			s.logger.Error("解析上次领取时间失败", zap.Error(err), zap.String("timestamp", levelData.LastGetOnHookTimestamp))
			return &game.ClaimOnHookRewardResponse{
				Code: 7,
				Msg:  "解析上次领取时间失败",
			}, nil
		}

		// 计算经过的时间（小时数）
		elapsed := now.Sub(lastGetTime)
		totalMinutes := int(elapsed.Minutes())

		// 计算完整的小时数
		hours = totalMinutes / 60

		// 不满1小时，无法领取
		if hours < 1 {
			remainingMinutes := 60 - (totalMinutes % 60)
			return &game.ClaimOnHookRewardResponse{
				Code: 8,
				Msg:  "挂机时间不足1小时，还需" + strconv.Itoa(remainingMinutes) + "分钟",
			}, nil
		}

		// 限制最大领取小时数
		if hours > OnHookMaxHours {
			hours = OnHookMaxHours
			s.logger.Info("挂机时间超过上限，限制为最大值",
				zap.Int("original_hours", totalMinutes/60),
				zap.Int("limited_hours", OnHookMaxHours))
		}

		// 计算新的领取时间（只增加已领取的完整小时数，不满1小时的分钟累加到下次）
		newLastGetTime = lastGetTime.Add(time.Duration(hours) * time.Hour)
	}

	// 获取道具奖励
	var reward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if levelInfo.OnHookRewardID != "" {
		reward = GetReward(levelInfo.OnHookRewardID, s.template.GetTplReward(), s.logger)
		if reward != nil {
			// 奖励乘以小时数
			if reward.Wallet != nil {
				reward.Wallet.Coin *= int32(hours)
				reward.Wallet.Gem *= int32(hours)
				reward.Wallet.Ad *= int32(hours)
			}
			if reward.Items != nil {
				for _, item := range reward.Items {
					if item != nil {
						item.Num *= int32(hours)
					}
				}
			}
		}
	}

	// 发放道具奖励
	if reward != nil {
		source := "onhook_" + levelData.CurLevelId
		wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
		if err != nil {
			s.logger.Error("发放道具奖励失败", zap.Error(err))
			// 不返回错误，金币已经发放成功
		} else {
			// 合并钱包更新结果
			if wResult != nil && walletUpdateResult != nil {
				// 保留金币奖励的钱包状态，只更新道具奖励部分
				walletUpdateResult.Updated = wResult.Updated
			} else if wResult != nil {
				walletUpdateResult = wResult
			}
			inventoryUpdateResult = iResult
		}
	}

	// 更新上次领取时间（只增加完整小时数，不满1小时的分钟累加到下次）
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	levelData.LastGetOnHookTimestamp = newLastGetTime.Format(time.RFC3339)

	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, levelData); err != nil {
		s.logger.Error("更新挂机时间失败", zap.Error(err))
		return &game.ClaimOnHookRewardResponse{
			Code: 10,
			Msg:  "更新挂机时间失败: " + err.Error(),
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

	return &game.ClaimOnHookRewardResponse{
		Code:             0,
		Msg:              "领取成功",
		Hours:            int32(hours),
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}
