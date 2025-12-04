package server

import (
	"context"
	"strconv"

	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
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

	// 更新最大关卡ID（如果新关卡更大）
	if battleData.MaxLevelId == "" || compareLevelId(in.GetLevelId(), battleData.MaxLevelId) {
		oldLevelId := battleData.MaxLevelId
		battleData.MaxLevelId = in.GetLevelId()
		s.logger.Info("更新最大关卡",
			zap.String("old_level_id", oldLevelId),
			zap.String("new_level_id", in.GetLevelId()))
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
	}
	return &game.StartBattleResponse{
		Code:    0,
		Msg:     "通过成功",
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

	// 根据战斗类型获取奖励ID
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

	default:
		return &game.EndBattleResponse{
			Code: 2,
			Msg:  "无效的战斗类型",
		}, nil
	}

	// 如果进度为0，直接返回成功（不发放奖励）
	if progress == 0 {
		// 重置分享奖励状态，清空奖励JSON，标记战斗已结束
		battleData.ShareRewardClaimed = false
		battleData.RewardJSON = ""
		battleData.BattleEnded = true
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
			s.logger.Error("保存战斗数据失败", zap.Error(err))
		}
		return &game.EndBattleResponse{
			Code: 0,
			Msg:  "通过成功",
		}, nil
	}

	// 获取奖励并应用进度折扣
	var reward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	reward = GetReward(rewardId, s.template.GetTplReward(), s.logger)
	if reward != nil {
		// 应用进度折扣
		applyProgressToReward(reward, progress)

		// 发放奖励
		var err error
		walletUpdateResult, inventoryUpdateResult, err = GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
		if err != nil {
			s.logger.Error("奖励发放失败", zap.Error(err))
			return &game.EndBattleResponse{
				Code: 4,
				Msg:  "奖励发放失败: " + err.Error(),
			}, nil
		}
	}

	// 保存奖励JSON（用于分享后再次领取相同奖励）
	if reward != nil {
		rewardJSON, err := protojson.Marshal(reward)
		if err != nil {
			s.logger.Error("序列化奖励失败", zap.Error(err))
		} else {
			battleData.RewardJSON = string(rewardJSON)
		}
	}

	// 重置分享奖励状态，标记战斗已结束
	battleData.ShareRewardClaimed = false
	battleData.BattleEnded = true

	// 更新关卡进度（仅对普通关卡）- 直接内联逻辑，避免多次保存
	if battleData.BattleType == game.BattleType_BATTLE_TYPE_NORMAL {
		// 初始化 Progress map（如果为 nil）
		if battleData.Progress == nil {
			battleData.Progress = make(map[string]int32)
		}

		levelId := battleData.CurLevelId
		currentProgress, exists := battleData.Progress[levelId]

		// 更新 Progress map（如果进度更好）
		if !exists || progress > currentProgress {
			if exists {
				oldProgress := currentProgress
				battleData.Progress[levelId] = progress
				s.logger.Info("更新关卡进度",
					zap.String("level_id", levelId),
					zap.Int32("old_progress", oldProgress),
					zap.Int32("new_progress", progress))
			} else {
				battleData.Progress[levelId] = progress
				s.logger.Info("添加新关卡进度",
					zap.String("level_id", levelId),
					zap.Int32("progress", progress))
			}
		}
	}

	// 统一保存战斗数据（包含进度更新）
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
		// 不影响战斗结算，仅记录错误
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
