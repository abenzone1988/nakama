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
	needsDaily := needsDailyReset(taskData.DateTime)
	// 检查是否需要每周重置
	needsWeekly := needsWeeklyReset(taskData.WeeklyResetTime)

	if needsDaily || needsWeekly {
		if needsDaily {
			s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
			resetDailyTasks(taskData)
		}
		if needsWeekly {
			s.logger.Info("任务数据每周重置", zap.String("user_id", userID.String()))
			resetWeeklyTasks(taskData)
		}

		// 保存重置后的数据
		err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, taskData)
		if err != nil {
			s.logger.Error("保存任务数据失败", zap.Error(err))
		}
	}

	return &game.GetTaskResponse{
		Code:                         0,
		Msg:                          "获取成功",
		DailyLiveness:                taskData.DailyLiveness,
		WeeklyLiveness:               taskData.WeeklyLiveness,
		ClaimedDailyTasks:            taskData.ClaimedDailyTasks,
		ClaimedWeeklyTasks:           taskData.ClaimedWeeklyTasks,
		ClaimedMainTasks:             taskData.ClaimedMainTasks,
		ClaimedDailyLivenessRewards:  taskData.ClaimedDailyLivenessRewards,
		ClaimedWeeklyLivenessRewards: taskData.ClaimedWeeklyLivenessRewards,
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
	needsDaily := needsDailyReset(taskData.DateTime)
	// 检查是否需要每周重置
	needsWeekly := needsWeeklyReset(taskData.WeeklyResetTime)

	if needsDaily {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		resetDailyTasks(taskData)
	}
	if needsWeekly {
		s.logger.Info("任务数据每周重置", zap.String("user_id", userID.String()))
		resetWeeklyTasks(taskData)
	}

	// 分离已领取和未领取的任务（根据任务类型判断）
	var toClaimTaskIDs []string

	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}

		// 获取任务配置以确定任务类型
		taskConfig, exist := s.template.GetTplTasks().FindByKey(taskID)
		if !exist {
			continue
		}

		// 根据任务类型判断是否已领取
		isClaimed := false
		switch taskConfig.TaskType {
		case 1: // 主线任务
			isClaimed = containsString(taskData.ClaimedMainTasks, taskID)
		case 2: // 每日任务
			isClaimed = containsString(taskData.ClaimedDailyTasks, taskID)
		case 3: // 每周任务
			isClaimed = containsString(taskData.ClaimedWeeklyTasks, taskID)
		}

		if !isClaimed {
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
	var addedDailyLiveness int32  // 本次增加的每日活跃度
	var addedWeeklyLiveness int32 // 本次增加的每周活跃度
	var validTaskIDs []string

	for _, taskID := range toClaimTaskIDs {
		taskConfig, exist := s.template.GetTplTasks().FindByKey(taskID)
		if !exist {
			s.logger.Warn("任务配置不存在", zap.String("task_id", taskID))
			continue
		}

		validTaskIDs = append(validTaskIDs, taskID)

		// 统计本次增加的活跃度（按任务类型分类）
		switch taskConfig.TaskType {
		case 2: // 每日任务
			addedDailyLiveness += taskConfig.Liveness
		case 3: // 每周任务
			addedWeeklyLiveness += taskConfig.Liveness
		}

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

	// 根据任务类型标记已领取并增加对应活跃度
	for _, taskID := range validTaskIDs {
		taskConfig, exist := s.template.GetTplTasks().FindByKey(taskID)
		if !exist {
			continue
		}

		switch taskConfig.TaskType {
		case 1: // 主线任务（不增加活跃度）
			if !containsString(taskData.ClaimedMainTasks, taskID) {
				taskData.ClaimedMainTasks = append(taskData.ClaimedMainTasks, taskID)
			}
		case 2: // 每日任务（增加每日活跃度）
			if !containsString(taskData.ClaimedDailyTasks, taskID) {
				taskData.ClaimedDailyTasks = append(taskData.ClaimedDailyTasks, taskID)
				taskData.DailyLiveness += taskConfig.Liveness
			}
		case 3: // 每周任务（增加每周活跃度）
			if !containsString(taskData.ClaimedWeeklyTasks, taskID) {
				taskData.ClaimedWeeklyTasks = append(taskData.ClaimedWeeklyTasks, taskID)
				taskData.WeeklyLiveness += taskConfig.Liveness
			}
		}
	}

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
		zap.Int32("added_daily_liveness", addedDailyLiveness),
		zap.Int32("added_weekly_liveness", addedWeeklyLiveness),
		zap.Int32("total_daily_liveness", taskData.DailyLiveness),
		zap.Int32("total_weekly_liveness", taskData.WeeklyLiveness))

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
	needsDaily := needsDailyReset(taskData.DateTime)
	// 检查是否需要每周重置
	needsWeekly := needsWeeklyReset(taskData.WeeklyResetTime)

	if needsDaily {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		resetDailyTasks(taskData)
		// 重置后每日活跃度为0，无法领取每日奖励
		return &game.ClaimLivenessRewardResponse{
			Code: 4,
			Msg:  "活跃度不足",
		}, nil
	}

	if needsWeekly {
		s.logger.Info("任务数据每周重置", zap.String("user_id", userID.String()))
		resetWeeklyTasks(taskData)
	}

	// 分离已领取和未领取的奖励（根据奖励类型判断）
	var toClaimRewardIDs []string

	for _, rewardID := range rewardIDs {
		if rewardID == "" {
			continue
		}

		// 获取奖励配置以确定奖励类型
		rewardConfig, exist := s.template.GetTplProgressReward().FindByKey(rewardID)
		if !exist {
			continue
		}

		// 根据奖励类型判断是否已领取
		isClaimed := false
		switch rewardConfig.Type {
		case 2: // 每日活跃度奖励
			isClaimed = containsString(taskData.ClaimedDailyLivenessRewards, rewardID)
		case 3: // 每周活跃度奖励
			isClaimed = containsString(taskData.ClaimedWeeklyLivenessRewards, rewardID)
		}

		if !isClaimed {
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

		// 根据奖励类型检查对应的活跃度是否足够
		var currentLiveness int32
		switch rewardConfig.Type {
		case 2: // 每日活跃度奖励
			currentLiveness = taskData.DailyLiveness
		case 3: // 每周活跃度奖励
			currentLiveness = taskData.WeeklyLiveness
		default:
			s.logger.Warn("未知的活跃度奖励类型", zap.String("reward_id", rewardID), zap.Int32("type", rewardConfig.Type))
			continue
		}

		if currentLiveness < rewardConfig.NeedValue {
			s.logger.Warn("活跃度不足，跳过奖励",
				zap.String("reward_id", rewardID),
				zap.Int32("type", rewardConfig.Type),
				zap.Int32("need_value", rewardConfig.NeedValue),
				zap.Int32("current_liveness", currentLiveness))
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

	// 根据奖励类型标记已领取
	for _, rewardID := range validRewardIDs {
		rewardConfig, exist := s.template.GetTplProgressReward().FindByKey(rewardID)
		if !exist {
			continue
		}

		switch rewardConfig.Type {
		case 2: // 每日活跃度奖励
			if !containsString(taskData.ClaimedDailyLivenessRewards, rewardID) {
				taskData.ClaimedDailyLivenessRewards = append(taskData.ClaimedDailyLivenessRewards, rewardID)
			}
		case 3: // 每周活跃度奖励
			if !containsString(taskData.ClaimedWeeklyLivenessRewards, rewardID) {
				taskData.ClaimedWeeklyLivenessRewards = append(taskData.ClaimedWeeklyLivenessRewards, rewardID)
			}
		}
	}

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
		zap.Int32("daily_liveness", taskData.DailyLiveness),
		zap.Int32("weekly_liveness", taskData.WeeklyLiveness))

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

// needsWeeklyReset 检查是否需要每周重置
func needsWeeklyReset(lastResetTime time.Time) bool {
	if lastResetTime.IsZero() {
		return true
	}

	now := time.Now().UTC()
	currentWeekStart := getWeekStart(now)
	lastWeekStart := getWeekStart(lastResetTime)

	return currentWeekStart.After(lastWeekStart)
}

// getWeekStart 获取本周开始时间（周一 00:00 UTC）
func getWeekStart(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysFromMonday := weekday - 1
	return t.Truncate(24*time.Hour).AddDate(0, 0, -daysFromMonday)
}

// resetDailyTasks 重置每日任务相关数据
func resetDailyTasks(taskData *TaskData) {
	taskData.DateTime = time.Now().UTC()
	taskData.ClaimedDailyTasks = []string{}
	taskData.ClaimedDailyLivenessRewards = []string{}
	taskData.DailyLiveness = 0
}

// resetWeeklyTasks 重置每周任务相关数据
func resetWeeklyTasks(taskData *TaskData) {
	taskData.WeeklyResetTime = getWeekStart(time.Now().UTC())
	taskData.ClaimedWeeklyTasks = []string{}
	taskData.ClaimedWeeklyLivenessRewards = []string{}
	taskData.WeeklyLiveness = 0
}
