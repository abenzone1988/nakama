package server

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

// GetFirstChargeStatus 获取首冲状态
func (s *ApiServer) GetFirstChargeStatus(ctx context.Context, in *game.GetFirstChargeStatusRequest) (*game.GetFirstChargeStatusResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从 storage 加载首冲数据
	firstChargeData := &FirstChargeData{}
	if err := LoadData(ctx, s.logger, s.db, userID, firstChargeData); err != nil {
		s.logger.Error("加载首冲数据失败", zap.Error(err))
		firstChargeData.Init()
	}

	// 计算可领取的天数
	availableDays := int32(0)
	if firstChargeData.IsCharged {
		availableDays = calculateAvailableDays(firstChargeData.ChargeTime)
	}

	return &game.GetFirstChargeStatusResponse{
		Code:          0,
		Msg:           "Success",
		IsCharged:     firstChargeData.IsCharged,
		ChargeTime:    firstChargeData.ChargeTime,
		ClaimedDays:   firstChargeData.ClaimedDays,
		AvailableDays: availableDays,
	}, nil
}

// ClaimFirstChargeReward 领取首冲奖励
func (s *ApiServer) ClaimFirstChargeReward(ctx context.Context, in *game.ClaimFirstChargeRewardRequest) (*game.ClaimFirstChargeRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证天数参数
	if in.Day < 1 || in.Day > 3 {
		return &game.ClaimFirstChargeRewardResponse{
			Code: 1,
			Msg:  "无效的天数参数",
		}, nil
	}

	// 从 storage 加载首冲数据
	firstChargeData := &FirstChargeData{}
	if err := LoadData(ctx, s.logger, s.db, userID, firstChargeData); err != nil {
		s.logger.Error("加载首冲数据失败", zap.Error(err))
		firstChargeData.Init()
	}

	// 检查是否已首冲
	if !firstChargeData.IsCharged {
		return &game.ClaimFirstChargeRewardResponse{
			Code: 2,
			Msg:  "尚未首冲",
		}, nil
	}

	// 检查是否已领取过该天的奖励
	for _, claimedDay := range firstChargeData.ClaimedDays {
		if claimedDay == in.Day {
			return &game.ClaimFirstChargeRewardResponse{
				Code: 3,
				Msg:  "该奖励已领取",
			}, nil
		}
	}

	// 计算当前可领取的天数
	availableDays := calculateAvailableDays(firstChargeData.ChargeTime)
	if in.Day > availableDays {
		return &game.ClaimFirstChargeRewardResponse{
			Code: 4,
			Msg:  fmt.Sprintf("当前只能领取第%d天及之前的奖励", availableDays),
		}, nil
	}

	// 获取奖励配置
	reward, err := s.getFirstChargeReward(in.Day)
	if err != nil {
		s.logger.Error("获取首冲奖励配置失败", zap.Error(err), zap.Int32("day", in.Day))
		return &game.ClaimFirstChargeRewardResponse{
			Code: 5,
			Msg:  "奖励配置不存在",
		}, nil
	}

	// 发放奖励
	source := fmt.Sprintf("first_charge_day_%d", in.Day)
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		s.logger.Error("发放首冲奖励失败", zap.Error(err), zap.Int32("day", in.Day))
		return &game.ClaimFirstChargeRewardResponse{
			Code: 6,
			Msg:  "发放奖励失败",
		}, nil
	}

	// 更新已领取天数
	firstChargeData.ClaimedDays = append(firstChargeData.ClaimedDays, in.Day)
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, firstChargeData); err != nil {
		s.logger.Error("保存首冲数据失败", zap.Error(err))
		return &game.ClaimFirstChargeRewardResponse{
			Code: 7,
			Msg:  "更新数据失败",
		}, nil
	}

	// 提取更新后的数据
	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item

	if walletResult != nil {
		walletUpdated = walletResult.Updated
	}
	if inventoryResult != nil {
		inventoryUpdated = inventoryResult.Updated
	}

	s.logger.Info("首冲奖励领取成功",
		zap.String("user_id", userID.String()),
		zap.Int32("day", in.Day))

	return &game.ClaimFirstChargeRewardResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ClaimedDays:      firstChargeData.ClaimedDays,
	}, nil
}

// calculateAvailableDays 计算当前可领取的天数
func calculateAvailableDays(chargeTimeStr string) int32 {
	if chargeTimeStr == "" {
		return 0
	}

	chargeTime, err := time.Parse(time.RFC3339, chargeTimeStr)
	if err != nil {
		return 0
	}

	// 计算从首冲到现在经过了多少天
	daysPassed := int32(time.Since(chargeTime).Hours()/24) + 1 // +1 是因为首冲当天算第一天

	// 最多3天
	if daysPassed > 3 {
		daysPassed = 3
	}

	return daysPassed
}

// getFirstChargeReward 获取首冲奖励配置
func (s *ApiServer) getFirstChargeReward(day int32) (*game.Reward, error) {
	// 根据天数查找配置
	configs := s.template.GetTplFirstCharge().FindByFilter(func(fc template.TplFirstCharge) bool {
		return fc.Day == day
	})

	if configs.Len() == 0 {
		return nil, fmt.Errorf("找不到第%d天的首冲配置", day)
	}

	config := configs.Get(0)

	// 使用通用奖励配置表
	reward := GetReward(config.Reward, s.template.GetTplReward(), s.logger)
	if reward == nil {
		return nil, fmt.Errorf("解析奖励配置失败，reward_id=%s", config.Reward)
	}

	return reward, nil
}

// RecordFirstCharge 记录首冲（在充值回调中调用）
func RecordFirstCharge(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, userID uuid.UUID) error {
	// 从 storage 加载首冲数据
	firstChargeData := &FirstChargeData{}
	if err := LoadData(ctx, logger, db, userID, firstChargeData); err != nil {
		logger.Error("加载首冲数据失败", zap.Error(err))
		firstChargeData.Init()
	}

	// 如果已经首冲过，直接返回
	if firstChargeData.IsCharged {
		logger.Info("用户已经首冲过，跳过记录")
		return nil
	}

	// 记录首冲
	firstChargeData.IsCharged = true
	firstChargeData.ChargeTime = time.Now().Format(time.RFC3339)
	firstChargeData.ClaimedDays = []int32{}

	if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, firstChargeData); err != nil {
		return fmt.Errorf("保存首冲数据失败: %w", err)
	}

	logger.Info("首冲记录成功", zap.String("charge_time", firstChargeData.ChargeTime))
	return nil
}
