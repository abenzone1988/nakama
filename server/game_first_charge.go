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
	// 从 storage 加载首冲数据
	firstChargeData := &FirstChargeData{}
	if err := LoadUserData(ctx, s.logger, s.db, firstChargeData); err != nil {
		s.logger.Error("加载首冲数据失败", zap.Error(err))
		firstChargeData.Init()
	}

	// 计算可领取的天数
	availableDays := int32(0)
	if firstChargeData.IsCharged {
		availableDays = calculateAvailableDays(firstChargeData.ChargeTime)
	}

	tplPays := s.template.GetTplPay().FindByFilter(func(tp template.TplPay) bool {
		return tp.ID == FirstChargeProductID
	})
	if tplPays == nil {
		return nil, fmt.Errorf("商品不存在: %s", FirstChargeProductID)
	}
	tplPay := tplPays.Get(0)

	return &game.GetFirstChargeStatusResponse{
		Code:          0,
		Msg:           "Success",
		IsCharged:     firstChargeData.IsCharged,
		ChargeTime:    firstChargeData.ChargeTime,
		ClaimedDays:   firstChargeData.ClaimedDays,
		AvailableDays: availableDays,
		Price:         tplPay.Money,
	}, nil
}

// ClaimFirstChargeReward 领取首冲奖励（支持批量领取）
func (s *ApiServer) ClaimFirstChargeReward(ctx context.Context, in *game.ClaimFirstChargeRewardRequest) (*game.ClaimFirstChargeRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证天数参数数组
	if len(in.Days) == 0 {
		return &game.ClaimFirstChargeRewardResponse{
			Code: 1,
			Msg:  "天数参数不能为空",
		}, nil
	}

	// 验证所有天数参数 (1-3) 并去重
	daySet := make(map[int32]struct{})
	var validDays []int32
	for _, day := range in.Days {
		if day < 1 || day > 3 {
			return &game.ClaimFirstChargeRewardResponse{
				Code: 1,
				Msg:  fmt.Sprintf("无效的天数参数: %d（有效范围：1-3）", day),
			}, nil
		}
		if _, exists := daySet[day]; !exists {
			daySet[day] = struct{}{}
			validDays = append(validDays, day)
		}
	}

	// 从 storage 加载首冲数据
	firstChargeData := &FirstChargeData{}
	if err := LoadUserData(ctx, s.logger, s.db, firstChargeData); err != nil {
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

	// 计算当前可领取的天数
	availableDays := calculateAvailableDays(firstChargeData.ChargeTime)

	// 构建已领取天数的集合，用于快速查找
	claimedSet := make(map[int32]struct{}, len(firstChargeData.ClaimedDays))
	for _, claimedDay := range firstChargeData.ClaimedDays {
		claimedSet[claimedDay] = struct{}{}
	}

	// 验证所有天数是否可领取
	var toClaimDays []int32
	var alreadyClaimedDays []int32
	for _, day := range validDays {
		// 检查是否已领取
		if _, claimed := claimedSet[day]; claimed {
			alreadyClaimedDays = append(alreadyClaimedDays, day)
			continue
		}

		// 检查是否在可领取范围内
		if day > availableDays {
			return &game.ClaimFirstChargeRewardResponse{
				Code: 4,
				Msg:  fmt.Sprintf("第%d天的奖励尚未解锁，当前只能领取第%d天及之前的奖励", day, availableDays),
			}, nil
		}

		toClaimDays = append(toClaimDays, day)
	}

	// 如果所有天数都已领取
	if len(toClaimDays) == 0 {
		return &game.ClaimFirstChargeRewardResponse{
			Code: 3,
			Msg:  fmt.Sprintf("这些奖励已领取: %v", alreadyClaimedDays),
		}, nil
	}

	// 收集所有有效的奖励配置
	var allRewards []*game.Reward
	for _, day := range toClaimDays {
		reward, err := s.getFirstChargeReward(day)
		if err != nil {
			s.logger.Error("获取首冲奖励配置失败", zap.Error(err), zap.Int32("day", day))
			return &game.ClaimFirstChargeRewardResponse{
				Code: 5,
				Msg:  fmt.Sprintf("第%d天的奖励配置不存在", day),
			}, nil
		}
		allRewards = append(allRewards, reward)
	}

	// 合并所有奖励
	var mergedReward *game.Reward
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if len(allRewards) > 0 {
		mergedReward = MergeRewards(allRewards)
		if mergedReward != nil {
			// 发放奖励
			source := fmt.Sprintf("first_charge_merged:%v", toClaimDays)
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, mergedReward, source)
			if err != nil {
				s.logger.Error("发放首冲奖励失败", zap.Error(err), zap.Int32s("days", toClaimDays))
				return &game.ClaimFirstChargeRewardResponse{
					Code: 6,
					Msg:  "发放奖励失败",
				}, nil
			}
			walletUpdateResult = wResult
			inventoryUpdateResult = iResult
		}
	}

	// 更新已领取天数
	firstChargeData.ClaimedDays = append(firstChargeData.ClaimedDays, toClaimDays...)
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, firstChargeData); err != nil {
		s.logger.Error("保存首冲数据失败", zap.Error(err))
		return &game.ClaimFirstChargeRewardResponse{
			Code: 7,
			Msg:  "更新数据失败",
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

	s.logger.Info("首冲奖励领取成功",
		zap.String("user_id", userID.String()),
		zap.Int32s("days", toClaimDays))

	return &game.ClaimFirstChargeRewardResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           mergedReward,
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
	firstChargeData.ChargeTime = time.Now().UTC().Format(time.RFC3339)
	firstChargeData.ClaimedDays = []int32{}

	if err := SaveData(ctx, logger, db, metrics, storageIndex, userID, firstChargeData); err != nil {
		return fmt.Errorf("保存首冲数据失败: %w", err)
	}

	logger.Info("首冲记录成功", zap.String("charge_time", firstChargeData.ChargeTime))
	return nil
}
