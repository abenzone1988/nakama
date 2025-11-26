package server

import (
	"context"
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
	if err := LoadData(ctx, s.logger, s.db, userID, taskData); err != nil {
		s.logger.Warn("加载任务数据失败，初始化新数据", zap.Error(err))
		taskData.Init()
	}

	// 检查是否需要每日重置
	if needsDailyReset(taskData.DateTime) {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		taskData.Init()

		// 保存重置后的数据
		err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, taskData)
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

// ClaimTaskReward 领取任务奖励
func (s *ApiServer) ClaimTaskReward(ctx context.Context, in *game.ClaimTaskRewardRequest) (*game.ClaimTaskRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证参数
	if in.GetTaskId() == "" {
		return &game.ClaimTaskRewardResponse{
			Code: 1,
			Msg:  "任务ID不能为空",
		}, nil
	}

	// 检查任务配置是否存在
	taskConfig, exist := s.template.GetTplTasks().FindByKey(in.GetTaskId())
	if !exist {
		return &game.ClaimTaskRewardResponse{
			Code: 2,
			Msg:  "任务不存在",
		}, nil
	}

	// 加载任务数据
	taskData := &TaskData{}
	if err := LoadData(ctx, s.logger, s.db, userID, taskData); err != nil {
		s.logger.Warn("加载任务数据失败，初始化新数据", zap.Error(err))
		taskData.Init()
	}

	// 检查是否需要每日重置
	if needsDailyReset(taskData.DateTime) {
		s.logger.Info("任务数据每日重置", zap.String("user_id", userID.String()))
		taskData.Init()
	}

	// 检查任务是否已领取
	if containsString(taskData.ClaimedTasks, in.GetTaskId()) {
		return &game.ClaimTaskRewardResponse{
			Code: 3,
			Msg:  "任务奖励已经领取过了",
		}, nil
	}

	// TODO: 这里应该检查任务是否完成（需要前端传递进度或后端记录进度）
	// 暂时先允许直接领取

	// 获取任务奖励
	var reward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if taskConfig.Reward != "" {
		reward = GetReward(taskConfig.Reward, s.template.GetTplReward(), s.logger)
		if reward != nil {
			// 发放奖励
			source := "task_" + in.GetTaskId()
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
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
	taskData.CurrentLiveness += taskConfig.Liveness

	// 标记任务已领取
	taskData.ClaimedTasks = append(taskData.ClaimedTasks, in.GetTaskId())

	// 保存任务数据
	err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, taskData)
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
		zap.String("task_id", in.GetTaskId()),
		zap.Int32("liveness", taskConfig.Liveness),
		zap.Int32("total_liveness", taskData.CurrentLiveness))

	return &game.ClaimTaskRewardResponse{
		Code:             0,
		Msg:              "领取成功",
		Liveness:         taskConfig.Liveness,
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

// ClaimLivenessReward 领取活跃度奖励
func (s *ApiServer) ClaimLivenessReward(ctx context.Context, in *game.ClaimLivenessRewardRequest) (*game.ClaimLivenessRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证参数
	if in.GetRewardId() == "" {
		return &game.ClaimLivenessRewardResponse{
			Code: 1,
			Msg:  "奖励ID不能为空",
		}, nil
	}

	// 检查活跃度奖励配置是否存在
	rewardConfig, exist := s.template.GetTplProgressReward().FindByKey(in.GetRewardId())
	if !exist {
		return &game.ClaimLivenessRewardResponse{
			Code: 2,
			Msg:  "活跃度奖励不存在",
		}, nil
	}

	// 加载任务数据
	taskData := &TaskData{}
	if err := LoadData(ctx, s.logger, s.db, userID, taskData); err != nil {
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

	// 检查活跃度是否足够
	if taskData.CurrentLiveness < rewardConfig.NeedValue {
		return &game.ClaimLivenessRewardResponse{
			Code: 5,
			Msg:  "活跃度不足",
		}, nil
	}

	// 检查奖励是否已领取
	if containsString(taskData.ClaimedLivenessRewards, in.GetRewardId()) {
		return &game.ClaimLivenessRewardResponse{
			Code: 6,
			Msg:  "活跃度奖励已经领取过了",
		}, nil
	}

	// 获取活跃度奖励
	var reward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if rewardConfig.Reward != "" {
		reward = GetReward(rewardConfig.Reward, s.template.GetTplReward(), s.logger)
		if reward != nil {
			// 发放奖励
			source := "liveness_" + in.GetRewardId()
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
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

	// 标记奖励已领取
	taskData.ClaimedLivenessRewards = append(taskData.ClaimedLivenessRewards, in.GetRewardId())

	// 保存任务数据
	err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, taskData)
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
		zap.String("reward_id", in.GetRewardId()),
		zap.Int32("need_liveness", rewardConfig.NeedValue),
		zap.Int32("current_liveness", taskData.CurrentLiveness))

	return &game.ClaimLivenessRewardResponse{
		Code:             0,
		Msg:              "领取成功",
		Reward:           reward,
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

// containsString 检查字符串切片中是否包含指定元素
func containsString(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
