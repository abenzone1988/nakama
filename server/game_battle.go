package server

import (
	"context"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	unlockConditionTypeLevel      = 1
	maxChallengeTimesPerDay       = 3  // 基础挑战次数
	maxChallengeAdBuyTimesPerDay  = 2  // 广告购买次数上限
	maxChallengeGemBuyTimesPerDay = 5  // 钻石购买次数上限
	challengeGemBuyPrice          = 50 // 钻石购买单价
	dateLayout                    = "2006-01-02"
)

// getCurrentDate 获取当前日期（YYYY-MM-DD）
func getCurrentDate() string {
	return time.Now().Format(dateLayout)
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

	case game.BattleType_BATTLE_TYPE_CHALLENGE:
		challengeInfo, exist := s.template.GetTplChallengeInfo().FindByKey(in.GetLevelId())
		if !exist {
			return &game.StartBattleResponse{
				Code: 2,
				Msg:  "挑战关卡不存在",
			}, nil
		}
		staminaCost = challengeInfo.Stamina

		// 检查每日挑战次数
		currentDate := getCurrentDate()
		if battleData.LastChallengeDate != currentDate {
			// 新的一天，重置挑战次数和购买次数
			battleData.ChallengeTimes = 0
			battleData.ChallengeAdBuyTimes = 0
			battleData.ChallengeGemBuyTimes = 0
			battleData.LastChallengeDate = currentDate
		}

		// 计算总可用次数 = 基础次数 + 广告购买次数 + 钻石购买次数
		totalAvailableTimes := maxChallengeTimesPerDay + battleData.ChallengeAdBuyTimes + battleData.ChallengeGemBuyTimes
		if battleData.ChallengeTimes >= totalAvailableTimes {
			return &game.StartBattleResponse{
				Code: 6,
				Msg:  "今日挑战次数已用完",
			}, nil
		}

		// 增加挑战次数
		battleData.ChallengeTimes++

		// 记录挑战赛参与次数（用于次数型奖励）
		if userID, ok := ctx.Value(ctxUserIDKey{}).(uuid.UUID); ok {
			userMatch := &UserMatch{}
			if err := LoadData(ctx, s.logger, s.db, userID, userMatch); err == nil && userMatch != nil {
				for _, ch := range userMatch.Challenges {
					if ch == nil {
						continue
					}
					// StartBattle 的 level_id 对应挑战活动ID（TplChallengeInfo.id）
					if ch.ActivityID == in.GetLevelId() && ch.TournamentID != "" {
						ch.BattleTimes++
						_ = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
						break
					}
				}
			}
		}

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

	// 统一奖励获取逻辑：先获取奖励配置，再应用进度，最后发放
	var finalReward *game.Reward
	var source string

	// 第一步：根据战斗类型获取奖励配置
	switch battleData.BattleType {
	case game.BattleType_BATTLE_TYPE_NORMAL:
		levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(battleData.CurLevelId)
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		finalReward = GetReward(levelInfo.WinRewards, s.template.GetTplReward(), s.logger)
		source = "battle_normal_" + battleData.CurLevelId

	case game.BattleType_BATTLE_TYPE_GOLDEN:
		activityInfo, exist := s.template.GetTplActivityLevelInfo().FindByKey(battleData.CurLevelId)
		if !exist {
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "关卡不存在",
			}, nil
		}
		finalReward = GetReward(activityInfo.RewardID, s.template.GetTplReward(), s.logger)
		source = "battle_golden_" + battleData.CurLevelId

	case game.BattleType_BATTLE_TYPE_CHALLENGE:
		// 挑战模式：根据怪物数量计算得分
		monsters := in.GetMonsters()
		totalScore := int32(0)

		for monsterId, num := range monsters {
			monsterInfo, exist := s.template.GetTplMonster().FindByKey(monsterId)
			if !exist {
				s.logger.Warn("怪物配置不存在",
					zap.String("monster_id", monsterId))
				continue
			}
			totalScore += monsterInfo.Score * num
		}

		s.logger.Info("挑战模式得分计算",
			zap.String("level_id", battleData.CurLevelId),
			zap.Int32("total_score", totalScore),
			zap.Any("monsters", monsters))

		// 挑战模式：计算奖励 = （最佳主线关卡奖励 + 当前挑战赛关卡奖励）* 进度
		var rewards []*game.Reward

		// 1. 获取最佳主线关卡的胜利奖励
		if battleData.MaxLevelId != "" {
			maxLevelInfo, exist := s.template.GetTplLevelInfo().FindByKey(battleData.MaxLevelId)
			if exist && maxLevelInfo.WinRewards != "" {
				maxLevelReward := GetReward(maxLevelInfo.WinRewards, s.template.GetTplReward(), s.logger)
				if maxLevelReward != nil {
					rewards = append(rewards, maxLevelReward)
					s.logger.Info("挑战赛添加最佳主线关卡奖励",
						zap.String("max_level_id", battleData.MaxLevelId),
						zap.String("reward_id", maxLevelInfo.WinRewards))
				}
			}
		}

		// 2. 获取当前挑战赛关卡的胜利奖励
		challengeInfo, exist := s.template.GetTplChallengeInfo().FindByKey(battleData.CurLevelId)
		if !exist {
			s.logger.Error("挑战赛关卡配置不存在", zap.String("level_id", battleData.CurLevelId))
			return &game.EndBattleResponse{
				Code: 2,
				Msg:  "挑战赛关卡不存在",
			}, nil
		}

		if challengeInfo.WinRewards != "" {
			challengeLevelReward := GetReward(challengeInfo.WinRewards, s.template.GetTplReward(), s.logger)
			if challengeLevelReward != nil {
				rewards = append(rewards, challengeLevelReward)
				s.logger.Info("挑战赛添加当前关卡奖励",
					zap.String("challenge_level_id", battleData.CurLevelId),
					zap.String("reward_id", challengeInfo.WinRewards))
			}
		}

		// 合并奖励
		finalReward = MergeRewards(rewards)
		source = "battle_challenge_" + battleData.CurLevelId

	default:
		return &game.EndBattleResponse{
			Code: 2,
			Msg:  "无效的战斗类型",
		}, nil
	}

	// 第二步：统一处理进度为0的情况（不发放奖励）
	if progress == 0 {
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

	// 第三步：统一应用进度折扣并发放奖励
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if finalReward != nil {
		// 应用进度折扣
		applyProgressToReward(finalReward, progress)

		// 发放奖励
		var err error
		walletUpdateResult, inventoryUpdateResult, err = GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, finalReward, source)
		if err != nil {
			s.logger.Error("奖励发放失败", zap.Error(err))
			return &game.EndBattleResponse{
				Code: 4,
				Msg:  "奖励发放失败: " + err.Error(),
			}, nil
		}

		// 保存奖励JSON（用于分享后再次领取相同奖励）
		rewardJSON, err := protojson.Marshal(finalReward)
		if err != nil {
			s.logger.Error("序列化奖励失败", zap.Error(err))
		} else {
			battleData.RewardJSON = string(rewardJSON)
		}
	}

	// 第四步：重置分享奖励状态，标记战斗已结束
	battleData.ShareRewardClaimed = false
	battleData.BattleEnded = true

	// 第五步：更新关卡进度（仅对普通关卡）
	if battleData.BattleType == game.BattleType_BATTLE_TYPE_NORMAL {
		// 章节满进度时尝试解锁对应炮台
		if progress >= 100 {
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
		// 更新最大关卡ID（如果新关卡更大）
		if battleData.MaxLevelId == "" || compareLevelId(battleData.CurLevelId, battleData.MaxLevelId) {
			oldLevelId := battleData.MaxLevelId
			battleData.MaxLevelId = battleData.CurLevelId
			s.logger.Info("更新最大关卡",
				zap.String("old_level_id", oldLevelId),
				zap.String("new_level_id", battleData.CurLevelId))
		}
	}

	// 第六步：统一保存战斗数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, battleData); err != nil {
		s.logger.Error("保存战斗数据失败", zap.Error(err))
	}

	// 第七步：提取更新后的数据并返回
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
		Reward:           finalReward,
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
	case game.BattleType_BATTLE_TYPE_CHALLENGE:
		source = "battle_challenge_share_" + battleData.CurLevelId
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
