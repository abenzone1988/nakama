package server

import (
	"context"
	"strconv"

	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
)

const unlockConditionTypeLevel = 1

func (s *ApiServer) addExpForBattle(ctx context.Context, battleType game.BattleType, levelId string) int32 {
	var staminaCost int32

	switch battleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		if levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(levelId); exist {
			staminaCost = levelInfo.Cost
		}
	case game.BattleType_BATTLE_TYPE_GOLDEN:
		if activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(levelId); exist {
			staminaCost = activityInfo.Stamina
		}
	case game.BattleType_BATTLE_TYPE_ELITE:
		if eliteInfo, exist := s.template.GetTplEliteLevelInfo().FindByKey(levelId); exist {
			staminaCost = eliteInfo.Cost
		}
	}

	if staminaCost == 0 {
		return 0
	}

	expGained, err := AddExp(ctx, s.logger, s.db, s.metrics, s.storageIndex, s.template, staminaCost)
	if err != nil {
		s.logger.Error("增加经验值失败", zap.Error(err))
		return 0
	}

	return expGained
}

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

func (s *ApiServer) StartBattle(ctx context.Context, in *game.StartBattleRequest) (*game.StartBattleResponse, error) {
	// 保存战斗信息，用于后续再次领取奖励
	battleData := &BattleData{}
	if err := LoadUserData(ctx, s.logger, s.db, battleData); err != nil {
		s.logger.Error("加载战斗数据失败", zap.Error(err))
	}

	battleData.CurLevelId = in.GetLevelId()
	battleData.BattleType = in.GetType()
	battleData.BattleEnded = false

	var staminaCost int32
	var stamina *game.StaminaData

	switch in.GetType() {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(in.GetLevelId())
		if !exist {
			return &game.StartBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		staminaCost = levelInfo.Cost

	case game.BattleType_BATTLE_TYPE_GOLDEN:
		activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(in.GetLevelId())
		if !exist {
			return &game.StartBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		staminaCost = activityInfo.Stamina
	case game.BattleType_BATTLE_TYPE_ELITE:
		eliteInfo, exist := s.template.GetTplEliteLevelInfo().FindByKey(in.GetLevelId())
		if !exist {
			return &game.StartBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		staminaCost = eliteInfo.Cost

	default:
		return &game.StartBattleResponse{
			Code: 2,
			Msg:  "无效的战斗类型",
		}, nil
	}

	// 扣除体力值
	var err error
	stamina, err = ConsumeStamina(ctx, s.logger, s.db, s.statusRegistry, staminaCost)
	if err != nil {
		return &game.StartBattleResponse{
			Code:    3,
			Msg:     "体力扣除失败: " + err.Error(),
			Stamina: stamina,
		}, nil
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
	}
	return &game.StartBattleResponse{
		Code:    0,
		Msg:     "开始成功",
		Stamina: stamina,
	}, nil
}

// 结束战斗领取奖励
func (s *ApiServer) EndBattle(ctx context.Context, in *game.EndBattleRequest) (*game.EndBattleResponse, error) {
	// 验证进度值
	progress := in.GetProgress()
	if progress > 100 || progress < 0 {
		return &game.EndBattleResponse{
			Code: 1,
			Msg:  "进度错误",
		}, nil
	}

	// 从开始战斗时保存的数据中获取关卡ID
	battleData := &BattleData{}
	if err := LoadUserData(ctx, s.logger, s.db, battleData); err != nil {
		s.logger.Error("加载战斗数据失败", zap.Error(err))
		return &game.EndBattleResponse{
			Code: 2,
			Msg:  "加载战斗数据失败",
		}, nil
	}

	// 验证是否有有效的战斗数据
	if battleData.CurLevelId == "" {
		return &game.EndBattleResponse{
			Code: 2,
			Msg:  "未找到战斗记录，请先开始战斗",
		}, nil
	}

	// 检查战斗是否已结束
	if battleData.BattleEnded {
		return &game.EndBattleResponse{
			Code: 5,
			Msg:  "战斗已结束，无法重复领取奖励",
		}, nil
	}

	// 如果进度为0，直接返回成功（不发放奖励，但给经验值）
	if progress == 0 {
		battleData.ShareRewardClaimed = false
		battleData.RewardJSON = ""
		battleData.BattleEnded = true
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
			s.logger.Error("保存战斗数据失败", zap.Error(err))
		}

		expGained := s.addExpForBattle(ctx, battleData.BattleType, battleData.CurLevelId)

		return &game.EndBattleResponse{
			Code:      0,
			Msg:       "通过成功",
			ExpGained: expGained,
		}, nil
	}

	// 根据战斗类型获取奖励ID和source
	var rewardId string
	var source string
	switch battleData.BattleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(battleData.CurLevelId)
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		rewardId = levelInfo.WinRewards
		source = "battle_normal_" + battleData.CurLevelId

	case game.BattleType_BATTLE_TYPE_GOLDEN:
		activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(battleData.CurLevelId)
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		rewardId = activityInfo.RewardID
		source = "battle_golden_" + battleData.CurLevelId

	case game.BattleType_BATTLE_TYPE_ELITE:
		eliteInfo, exist := s.template.GetTplEliteLevelInfo().FindByKey(battleData.CurLevelId)
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		rewardId = eliteInfo.WinRewards
		source = "battle_elite_" + battleData.CurLevelId
	}

	// 收集首充奖励ID
	var firstRewardId string

	// 如果进度 >= 100，检查并更新最大关卡ID，收集首充奖励ID
	if progress >= 100 {
		var levelUpdated bool
		switch battleData.BattleType {
		case game.BattleType_BATTLE_TYPE_NORMAL:
			if battleData.MaxLevelId == "" || compareLevelId(battleData.CurLevelId, battleData.MaxLevelId) {
				oldLevelId := battleData.MaxLevelId
				battleData.MaxLevelId = battleData.CurLevelId
				s.logger.Info("更新最大普通关卡",
					zap.String("old_level_id", oldLevelId),
					zap.String("new_level_id", battleData.CurLevelId))
			}

		case game.BattleType_BATTLE_TYPE_GOLDEN:
			if battleData.MaxActivityLevelId == "" || compareLevelId(battleData.CurLevelId, battleData.MaxActivityLevelId) {
				oldLevelId := battleData.MaxActivityLevelId
				battleData.MaxActivityLevelId = battleData.CurLevelId
				levelUpdated = (oldLevelId != battleData.CurLevelId)
				s.logger.Info("更新最大活动关卡",
					zap.String("old_level_id", oldLevelId),
					zap.String("new_level_id", battleData.CurLevelId))

				// 收集首充奖励ID
				if levelUpdated {
					activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(battleData.CurLevelId)
					if exist && activityInfo.FirstRewards != "" {
						firstRewardId = activityInfo.FirstRewards
					}
				}
			}

		case game.BattleType_BATTLE_TYPE_ELITE:
			if battleData.MaxEliteLevelId == "" || compareLevelId(battleData.CurLevelId, battleData.MaxEliteLevelId) {
				oldLevelId := battleData.MaxEliteLevelId
				battleData.MaxEliteLevelId = battleData.CurLevelId
				levelUpdated = (oldLevelId != battleData.CurLevelId)
				s.logger.Info("更新最大精英关卡",
					zap.String("old_level_id", oldLevelId),
					zap.String("new_level_id", battleData.CurLevelId))

				// 收集首充奖励ID
				if levelUpdated {
					eliteInfo, exist := s.template.GetTplEliteLevelInfo().FindByKey(battleData.CurLevelId)
					if exist && eliteInfo.FirstRewards != "" {
						firstRewardId = eliteInfo.FirstRewards
					}
				}
			}
		}

		// 章节满进度时尝试解锁对应炮台
		if battleData.BattleType == game.BattleType_BATTLE_TYPE_NORMAL {
			unlockEquips := s.template.GetTplUnlock().FindByFilter(func(unlock template.TplUnlock) bool {
				return unlock.Type == UnlockType_Equipment &&
					unlock.ConditionType == unlockConditionTypeLevel &&
					unlock.ConditionParameter == battleData.CurLevelId
			}).ToSlice()

			for _, unlock := range unlockEquips {
				if unlock.Parameter == "" {
					continue
				}
				if _, err := tryUnlockEquipment(ctx, s.logger, s.db, s.metrics, s.storageIndex, unlock.Parameter); err != nil {
					s.logger.Error("章节解锁炮台失败",
						zap.Error(err),
						zap.String("level_id", battleData.CurLevelId),
						zap.String("equip_id", unlock.Parameter))
				}
			}
		}
	}
	// 收集所有奖励
	var rewards []*game.Reward

	// 获取主奖励并应用进度折扣
	mainReward := GetReward(rewardId, s.template.GetTplReward(), s.logger)
	if mainReward != nil {
		applyProgressToReward(mainReward, progress)
		rewards = append(rewards, mainReward)
	}

	// 只有进度100时才发放首充奖励
	if firstRewardId != "" {
		firstReward := GetReward(firstRewardId, s.template.GetTplReward(), s.logger)
		if firstReward != nil {
			rewards = append(rewards, firstReward)
			s.logger.Info("添加首充奖励", zap.String("reward_id", firstRewardId))
		}
	}

	// 使用通用函数合并并发放奖励
	mergedReward, walletUpdateResult, inventoryUpdateResult, err := GrantMergedRewards(
		ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex,
		rewards, source,
	)
	if err != nil {
		s.logger.Error("发放奖励失败", zap.Error(err))
		return &game.EndBattleResponse{
			Code: 4,
			Msg:  "奖励发放失败: " + err.Error(),
		}, nil
	}

	// 保存奖励JSON（用于分享后再次领取相同奖励，包含主奖励和首充奖励）
	if mergedReward != nil {
		rewardJSON, err := protojson.Marshal(mergedReward)
		if err != nil {
			s.logger.Error("序列化奖励失败", zap.Error(err))
		} else {
			battleData.RewardJSON = string(rewardJSON)
		}
	}

	// 重置分享奖励状态，标记战斗已结束
	battleData.ShareRewardClaimed = false
	battleData.BattleEnded = true

	// 统一保存战斗数据（包含进度更新）
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
	}

	expGained := s.addExpForBattle(ctx, battleData.BattleType, battleData.CurLevelId)

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
		Reward:           mergedReward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ExpGained:        expGained,
	}, nil
}

// 快速通过战斗（直接扣体力发放奖励，无分享逻辑）
func (s *ApiServer) QuickBattle(ctx context.Context, in *game.QuickBattleRequest) (*game.QuickBattleResponse, error) {
	levelId := in.GetLevelId()
	battleType := in.GetType()

	// 根据战斗类型获取关卡配置和奖励ID
	var staminaCost int32
	var rewardId string
	var source string

	switch battleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(levelId)
		if !exist {
			return &game.QuickBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		staminaCost = levelInfo.Cost
		rewardId = levelInfo.WinRewards
		source = "quick_battle_normal_" + levelId

	case game.BattleType_BATTLE_TYPE_GOLDEN:
		activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(levelId)
		if !exist {
			return &game.QuickBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		staminaCost = activityInfo.Stamina
		rewardId = activityInfo.RewardID
		source = "quick_battle_golden_" + levelId

	case game.BattleType_BATTLE_TYPE_ELITE:
		eliteInfo, exist := s.template.GetTplEliteLevelInfo().FindByKey(levelId)
		if !exist {
			return &game.QuickBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		staminaCost = eliteInfo.Cost
		rewardId = eliteInfo.WinRewards
		source = "quick_battle_elite_" + levelId

	default:
		return &game.QuickBattleResponse{
			Code: 2,
			Msg:  "无效的战斗类型",
		}, nil
	}

	// 扣除体力值
	stamina, err := ConsumeStamina(ctx, s.logger, s.db, s.statusRegistry, staminaCost)
	if err != nil {
		return &game.QuickBattleResponse{
			Code:    3,
			Msg:     "体力扣除失败: " + err.Error(),
			Stamina: stamina,
		}, nil
	}

	expGained := s.addExpForBattle(ctx, battleType, levelId)

	// 加载战斗数据以检查关卡是否已通关
	battleData := &BattleData{}
	if err := LoadUserData(ctx, s.logger, s.db, battleData); err != nil {
		s.logger.Error("加载战斗数据失败", zap.Error(err))
	}

	// 检查关卡是否已经通关过（只能快速通过未通关的关卡）
	var alreadyCleared bool
	switch battleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		if battleData.MaxLevelId != "" && !compareLevelId(levelId, battleData.MaxLevelId) {
			alreadyCleared = true
		}
	case game.BattleType_BATTLE_TYPE_GOLDEN:
		if battleData.MaxActivityLevelId != "" && !compareLevelId(levelId, battleData.MaxActivityLevelId) {
			alreadyCleared = true
		}
	case game.BattleType_BATTLE_TYPE_ELITE:
		if battleData.MaxEliteLevelId != "" && !compareLevelId(levelId, battleData.MaxEliteLevelId) {
			alreadyCleared = true
		}
	}

	if !alreadyCleared {
		return &game.QuickBattleResponse{
			Code: 5,
			Msg:  "该关卡未通关，不能快速通过",
		}, nil
	}

	// 保存战斗数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
	}

	// 获取主奖励（快速通过不打折扣）
	mainReward := GetReward(rewardId, s.template.GetTplReward(), s.logger)
	if mainReward == nil {
		return &game.QuickBattleResponse{
			Code: 6,
			Msg:  "奖励配置不存在",
		}, nil
	}

	// 发放奖励
	walletUpdateResult, inventoryUpdateResult, err := GrantReward(
		ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex,
		mainReward, source,
	)
	if err != nil {
		s.logger.Error("发放奖励失败", zap.Error(err))
		return &game.QuickBattleResponse{
			Code: 4,
			Msg:  "奖励发放失败: " + err.Error(),
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

	return &game.QuickBattleResponse{
		Code:             0,
		Msg:              "快速通过成功",
		Reward:           mainReward,
		Stamina:          stamina,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ExpGained:        expGained,
	}, nil
}

// 分享后领取战斗奖励（每场战斗一次）
func (s *ApiServer) ClaimBattleRewardByShare(ctx context.Context, in *game.ClaimBattleRewardByShareRequest) (*game.ClaimBattleRewardByShareResponse, error) {
	// 加载战斗数据
	battleData := &BattleData{}
	if err := LoadUserData(ctx, s.logger, s.db, battleData); err != nil {
		s.logger.Error("加载战斗数据失败", zap.Error(err))
		return &game.ClaimBattleRewardByShareResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	// 验证是否有战斗记录
	if battleData.CurLevelId == "" {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 2,
			Msg:  "未完成战斗",
		}, nil
	}

	// 检查战斗是否已结束（只有战斗结束后才能领取分享奖励）
	if !battleData.BattleEnded {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 6,
			Msg:  "战斗尚未结束，无法领取分享奖励",
		}, nil
	}

	// 检查是否已领取
	if battleData.ShareRewardClaimed {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 3,
			Msg:  "奖励已领取",
		}, nil
	}

	// 验证是否有保存的奖励数据
	if battleData.RewardJSON == "" {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 4,
			Msg:  "未找到保存的奖励数据",
		}, nil
	}

	// 从保存的奖励 JSON 中恢复奖励
	reward := &game.Reward{}
	if err := protojson.Unmarshal([]byte(battleData.RewardJSON), reward); err != nil {
		s.logger.Error("反序列化奖励失败", zap.Error(err))
		return &game.ClaimBattleRewardByShareResponse{
			Code: 4,
			Msg:  "奖励数据错误",
		}, nil
	}

	// 根据战斗类型确定 source
	var source string
	switch battleData.BattleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		source = "battle_normal_share_" + battleData.CurLevelId
	case game.BattleType_BATTLE_TYPE_GOLDEN:
		source = "battle_golden_share_" + battleData.CurLevelId
	case game.BattleType_BATTLE_TYPE_ELITE:
		source = "battle_elite_share_" + battleData.CurLevelId
	default:
		return &game.ClaimBattleRewardByShareResponse{
			Code: 4,
			Msg:  "无效的战斗类型",
		}, nil
	}

	// 发放奖励
	walletUpdateResult, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		s.logger.Error("奖励发放失败", zap.Error(err))
		return &game.ClaimBattleRewardByShareResponse{
			Code: 5,
			Msg:  "奖励发放失败: " + err.Error(),
		}, nil
	}

	// 标记为已领取
	battleData.ShareRewardClaimed = true
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
		// 不影响奖励发放，仅记录错误
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

	return &game.ClaimBattleRewardByShareResponse{
		Code:             0,
		Msg:              "领取成功",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
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
