package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// GetTask 获取任务状态
func (s *ApiServer) GetTask(ctx context.Context, in *game.GetTaskRequest) (*game.GetTaskResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载任务数据
	taskData := &TaskData{}
	if err := LoadUserData(ctx, s.logger, s.db, taskData); err != nil {
		s.logger.Warn("加载任务数据失败，初始化新数据", zap.Error(err))
		return nil, fmt.Errorf("加载任务数据失败")
	}

	// 检查是否需要每日重置
	if needsDailyReset(taskData.DateTime) {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		taskData.Init()

		// 保存重置后的数据
		err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, taskData)
		if err != nil {
			s.logger.Error("保存任务数据失败", zap.Error(err))
		}
	}

	return &game.GetTaskResponse{
		Code:                   0,
		Msg:                    "获取成功",
		CurrentLiveness:        taskData.CurrentLiveness,
		ClaimedTasks:           taskData.ClaimedTasks,
		ClaimedLivenessRewards: taskData.ClaimedLivenessRewards,
	}, nil
}

// ClaimTaskReward 领取任务奖励（支持批量领取）
func (s *ApiServer) ClaimTaskReward(ctx context.Context, in *game.ClaimTaskRewardRequest) (*game.ClaimTaskRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证参数
	taskIDs := in.GetTaskIds()
	if len(taskIDs) == 0 {
		return &game.ClaimTaskRewardResponse{
			Code: 1,
			Msg:  "任务ID列表不能为空",
		}, nil
	}

	// 加载任务数据
	taskData := &TaskData{}
	if err := LoadUserData(ctx, s.logger, s.db, taskData); err != nil {
		s.logger.Warn("加载任务数据失败，初始化新数据", zap.Error(err))
		taskData.Init()
	}

	// 检查是否需要每日重置
	if needsDailyReset(taskData.DateTime) {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		taskData.Init()
	}

	// 分离已领取和未领取的任务
	var toClaimTaskIDs []string
	var alreadyClaimedTaskIDs []string

	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if containsString(taskData.ClaimedTasks, taskID) {
			alreadyClaimedTaskIDs = append(alreadyClaimedTaskIDs, taskID)
		} else {
			toClaimTaskIDs = append(toClaimTaskIDs, taskID)
		}
	}

	// 如果所有任务都已领取
	if len(toClaimTaskIDs) == 0 {
		return &game.ClaimTaskRewardResponse{
			Code: 3,
			Msg:  "所有任务奖励已经领取过了",
		}, nil
	}

	// 验证所有任务配置是否存在
	var allRewards []*game.Reward
	var totalLiveness int32
	var validTaskIDs []string

	for _, taskID := range toClaimTaskIDs {
		taskConfig, exist := s.template.GetTplTasks().FindByKey(taskID)
		if !exist {
			s.logger.Warn("任务配置不存在", zap.String("task_id", taskID))
			continue
		}

		validTaskIDs = append(validTaskIDs, taskID)
		totalLiveness += taskConfig.Liveness

		// 获取任务奖励
		if taskConfig.Reward != "" {
			reward := GetReward(taskConfig.Reward, s.template.GetTplReward(), s.logger)
			if reward != nil {
				allRewards = append(allRewards, reward)
			}
		}
	}

	// 如果没有有效的任务
	if len(validTaskIDs) == 0 {
		return &game.ClaimTaskRewardResponse{
			Code: 2,
			Msg:  "没有有效的任务",
		}, nil
	}

	// 合并所有奖励
	var mergedReward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if len(allRewards) > 0 {
		mergedReward = MergeRewards(allRewards)
		if mergedReward != nil {
			// 发放奖励
			source := "task_merged:" + fmt.Sprintf("%v", validTaskIDs)
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, mergedReward, source)
			if err != nil {
				s.logger.Error("发放任务奖励失败", zap.Error(err))
				return &game.ClaimTaskRewardResponse{
					Code: 4,
					Msg:  "发放奖励失败: " + err.Error(),
				}, nil
			}
			walletUpdateResult = wResult
			inventoryUpdateResult = iResult
		}
	}

	// 增加活跃度
	taskData.CurrentLiveness += totalLiveness

	// 标记所有任务已领取
	taskData.ClaimedTasks = append(taskData.ClaimedTasks, validTaskIDs...)

	// 保存任务数据
	err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, taskData)
	if err != nil {
		s.logger.Error("保存任务数据失败", zap.Error(err))
		return &game.ClaimTaskRewardResponse{
			Code: 5,
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

	s.logger.Info("任务奖励领取成功",
		zap.Strings("task_ids", validTaskIDs),
		zap.Int32("total_liveness", totalLiveness),
		zap.Int32("current_liveness", taskData.CurrentLiveness))

	return &game.ClaimTaskRewardResponse{
		Code:             0,
		Msg:              "领取成功",
		Reward:           mergedReward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

// ClaimLivenessReward 领取活跃度奖励（支持批量领取）
func (s *ApiServer) ClaimLivenessReward(ctx context.Context, in *game.ClaimLivenessRewardRequest) (*game.ClaimLivenessRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证参数
	rewardIDs := in.GetRewardIds()
	if len(rewardIDs) == 0 {
		return &game.ClaimLivenessRewardResponse{
			Code: 1,
			Msg:  "奖励ID列表不能为空",
		}, nil
	}

	// 加载任务数据
	taskData := &TaskData{}
	if err := LoadUserData(ctx, s.logger, s.db, taskData); err != nil {
		s.logger.Warn("加载任务数据失败", zap.Error(err))
		return &game.ClaimLivenessRewardResponse{
			Code: 3,
			Msg:  "获取任务数据失败",
		}, nil
	}

	// 检查是否需要每日重置
	if needsDailyReset(taskData.DateTime) {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		taskData.Init()
		// 重置后活跃度为0，无法领取奖励
		return &game.ClaimLivenessRewardResponse{
			Code: 4,
			Msg:  "活跃度不足",
		}, nil
	}

	// 分离已领取和未领取的奖励
	var toClaimRewardIDs []string
	var alreadyClaimedRewardIDs []string

	for _, rewardID := range rewardIDs {
		if rewardID == "" {
			continue
		}
		if containsString(taskData.ClaimedLivenessRewards, rewardID) {
			alreadyClaimedRewardIDs = append(alreadyClaimedRewardIDs, rewardID)
		} else {
			toClaimRewardIDs = append(toClaimRewardIDs, rewardID)
		}
	}

	// 如果所有奖励都已领取
	if len(toClaimRewardIDs) == 0 {
		return &game.ClaimLivenessRewardResponse{
			Code: 6,
			Msg:  "所有活跃度奖励已经领取过了",
		}, nil
	}

	// 验证所有奖励配置并检查活跃度要求
	var allRewards []*game.Reward
	var validRewardIDs []string

	for _, rewardID := range toClaimRewardIDs {
		rewardConfig, exist := s.template.GetTplProgressReward().FindByKey(rewardID)
		if !exist {
			s.logger.Warn("活跃度奖励配置不存在", zap.String("reward_id", rewardID))
			continue
		}

		// 检查活跃度是否足够
		if taskData.CurrentLiveness < rewardConfig.NeedValue {
			s.logger.Warn("活跃度不足，跳过奖励", zap.String("reward_id", rewardID), zap.Int32("need_liveness", rewardConfig.NeedValue), zap.Int32("current_liveness", taskData.CurrentLiveness))
			continue
		}

		validRewardIDs = append(validRewardIDs, rewardID)

		// 获取活跃度奖励
		if rewardConfig.Reward != "" {
			reward := GetReward(rewardConfig.Reward, s.template.GetTplReward(), s.logger)
			if reward != nil {
				allRewards = append(allRewards, reward)
			}
		}
	}

	// 如果没有有效的奖励
	if len(validRewardIDs) == 0 {
		return &game.ClaimLivenessRewardResponse{
			Code: 5,
			Msg:  "没有可领取的活跃度奖励（活跃度不足或配置不存在）",
		}, nil
	}

	// 合并所有奖励
	var mergedReward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if len(allRewards) > 0 {
		mergedReward = MergeRewards(allRewards)
		if mergedReward != nil {
			// 发放奖励
			source := "liveness_merged:" + fmt.Sprintf("%v", validRewardIDs)
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, mergedReward, source)
			if err != nil {
				s.logger.Error("发放活跃度奖励失败", zap.Error(err))
				return &game.ClaimLivenessRewardResponse{
					Code: 7,
					Msg:  "发放奖励失败: " + err.Error(),
				}, nil
			}
			walletUpdateResult = wResult
			inventoryUpdateResult = iResult
		}
	}

	// 标记所有奖励已领取
	taskData.ClaimedLivenessRewards = append(taskData.ClaimedLivenessRewards, validRewardIDs...)

	// 保存任务数据
	err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, taskData)
	if err != nil {
		s.logger.Error("保存任务数据失败", zap.Error(err))
		return &game.ClaimLivenessRewardResponse{
			Code: 8,
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

	s.logger.Info("活跃度奖励领取成功",
		zap.Strings("reward_ids", validRewardIDs),
		zap.Int32("current_liveness", taskData.CurrentLiveness))

	return &game.ClaimLivenessRewardResponse{
		Code:             0,
		Msg:              "领取成功",
		Reward:           mergedReward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

// needsDailyReset 检查是否需要每日重置
func needsDailyReset(lastUpdateTime time.Time) bool {
	if lastUpdateTime.IsZero() {
		return true
	}

	now := time.Now().UTC()
	lastDay := lastUpdateTime.Truncate(24 * time.Hour)
	today := now.Truncate(24 * time.Hour)

	return today.After(lastDay)
}
