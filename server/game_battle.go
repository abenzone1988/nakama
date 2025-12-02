package server

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/gofrs/uuid/v5"
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

// 结束战斗领取奖励 扣除体力
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

		//扣除体力值
		if err := ConsumeStamina(ctx, s.logger, s.db, s.statusRegistry, levelInfo.Cost); err != nil {
			return &game.EndBattleResponse{
				Code: 3,
				Msg:  "体力扣除失败: " + err.Error(),
			}, nil
		}

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

		//扣除体力值
		if err := ConsumeStamina(ctx, s.logger, s.db, s.statusRegistry, activityInfo.Stamina); err != nil {
			return &game.EndBattleResponse{
				Code: 3,
				Msg:  "体力扣除失败: " + err.Error(),
			}, nil
		}

	default:
		return &game.EndBattleResponse{
			Code: 2,
			Msg:  "关卡不存在",
		}, nil
	}

	if in.GetProgress() == 0 {
		return &game.EndBattleResponse{
			Code: 0,
			Msg:  "通过成功",
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

	// 保存战斗信息，用于后续再次领取奖励
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	battleData := &BattleData{}
	if err := LoadData(ctx, s.logger, s.db, userID, battleData); err != nil {
		s.logger.Error("加载战斗奖励数据失败", zap.Error(err))
		// 不影响战斗结算，仅记录错误
	} else {
		battleData.LevelId = in.GetLevelId()
		battleData.Progress = in.GetProgress()
		battleData.BattleType = int32(in.GetType())
		battleData.ShareRewardClaimed = false // 重置再次领取状态

		// 保存奖励 JSON（用于分享后再次领取相同奖励）
		if reward != nil {
			rewardJSON, err := protojson.Marshal(reward)
			if err != nil {
				s.logger.Error("序列化奖励失败", zap.Error(err))
			} else {
				battleData.RewardJSON = string(rewardJSON)
			}
		}

		if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, battleData); err != nil {
			s.logger.Error("保存战斗奖励数据失败", zap.Error(err))
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

// 分享后领取战斗奖励（每场战斗一次）
func (s *ApiServer) ClaimBattleRewardByShare(ctx context.Context, in *game.ClaimBattleRewardByShareRequest) (*game.ClaimBattleRewardByShareResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载战斗奖励数据
	battleData := &BattleData{}
	if err := LoadData(ctx, s.logger, s.db, userID, battleData); err != nil {
		s.logger.Error("加载战斗奖励数据失败", zap.Error(err))
		return &game.ClaimBattleRewardByShareResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	if battleData.LevelId == "" {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 2,
			Msg:  "未完成战斗",
		}, nil
	}

	if battleData.ShareRewardClaimed {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 3,
			Msg:  "奖励已领取",
		}, nil
	}

	// 从保存的奖励 JSON 中恢复奖励
	var reward *game.Reward
	if battleData.RewardJSON != "" {
		reward = &game.Reward{}
		if err := protojson.Unmarshal([]byte(battleData.RewardJSON), reward); err != nil {
			s.logger.Error("反序列化奖励失败", zap.Error(err))
			return &game.ClaimBattleRewardByShareResponse{
				Code: 4,
				Msg:  "奖励数据错误",
			}, nil
		}
	} else {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 4,
			Msg:  "未找到保存的奖励数据",
		}, nil
	}

	// 根据战斗类型确定 source
	var source string
	battleType := game.BattleType(battleData.BattleType)
	switch battleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		source = "battle_normal_share_" + battleData.LevelId
	case game.BattleType_BATTLE_TYPE_GOLDEN:
		source = "battle_golden_share_" + battleData.LevelId
	default:
		return &game.ClaimBattleRewardByShareResponse{
			Code: 4,
			Msg:  "无效的战斗类型",
		}, nil
	}

	// 发放奖励
	walletUpdateResult, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		return &game.ClaimBattleRewardByShareResponse{
			Code: 6,
			Msg:  "奖励发放失败: " + err.Error(),
		}, nil
	}

	// 标记为已领取（全局唯一一次）
	battleData.ShareRewardClaimed = true
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, battleData); err != nil {
		s.logger.Error("保存战斗奖励数据失败", zap.Error(err))
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
