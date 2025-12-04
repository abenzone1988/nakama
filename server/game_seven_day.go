package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/console"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

// GetSevenDayStatus 获取七日购买状态
func (s *ApiServer) GetSevenDayStatus(ctx context.Context, in *game.GetSevenDayStatusRequest) (*game.GetSevenDayStatusResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 从 storage 加载七日购买数据
	sevenDayData := &SevenDayData{}
	if err := LoadUserData(ctx, s.logger, s.db, sevenDayData); err != nil {
		s.logger.Error("加载七日购买数据失败", zap.Error(err))
		sevenDayData.Init()
	}

	if _, err := s.handleSevenDayExpiration(ctx, userID, sevenDayData); err != nil {
		s.logger.Error("处理七日过期奖励失败", zap.Error(err))
	}

	// 检查是否在有效购买周期内
	isPurchased := false
	availableDays := int32(0)

	if sevenDayData.LastPurchaseTime != "" {
		purchaseTime, err := time.Parse(time.RFC3339, sevenDayData.LastPurchaseTime)
		if err == nil {
			// 计算从购买到现在经过了多少天
			daysPassed := int32(time.Since(purchaseTime).Hours() / 24)

			// 如果在7天内，说明是有效购买
			if daysPassed < 7 {
				isPurchased = true
				// 可领取的天数 = 经过的天数 + 1（购买当天可以领 day 0）
				// 最多到 day 7
				availableDays = daysPassed + 1
				if availableDays > 7 {
					availableDays = 7
				}
			}
		}
	}

	tplPays := s.template.GetTplPay().FindByFilter(func(tp template.TplPay) bool {
		return tp.ID == SevenDayProductID
	})
	if tplPays == nil {
		return nil, fmt.Errorf("商品不存在: %s", SevenDayProductID)
	}
	tplPay := tplPays.Get(0)

	return &game.GetSevenDayStatusResponse{
		Code:           0,
		Msg:            "Success",
		IsPurchased:    isPurchased,
		PurchaseTime:   sevenDayData.LastPurchaseTime,
		ClaimedDays:    sevenDayData.ClaimedDays,
		AvailableDays:  availableDays,
		TotalPurchases: sevenDayData.TotalPurchases,
		Price:          tplPay.Money,
	}, nil
}

// ClaimSevenDayReward 领取七日奖励（支持批量领取）
func (s *ApiServer) ClaimSevenDayReward(ctx context.Context, in *game.ClaimSevenDayRewardRequest) (*game.ClaimSevenDayRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证天数参数数组
	if len(in.Days) == 0 {
		return &game.ClaimSevenDayRewardResponse{
			Code: 1,
			Msg:  "天数参数不能为空",
		}, nil
	}

	// 验证所有天数参数 (0-7) 并去重
	daySet := make(map[int32]struct{})
	var validDays []int32
	for _, day := range in.Days {
		if day < 0 || day > 7 {
			return &game.ClaimSevenDayRewardResponse{
				Code: 1,
				Msg:  fmt.Sprintf("无效的天数参数: %d（有效范围：0-7）", day),
			}, nil
		}
		if _, exists := daySet[day]; !exists {
			daySet[day] = struct{}{}
			validDays = append(validDays, day)
		}
	}

	// 从 storage 加载七日购买数据
	sevenDayData := &SevenDayData{}
	if err := LoadUserData(ctx, s.logger, s.db, sevenDayData); err != nil {
		s.logger.Error("加载七日购买数据失败", zap.Error(err))
		sevenDayData.Init()
	}

	// 检查是否已购买
	if sevenDayData.LastPurchaseTime == "" {
		return &game.ClaimSevenDayRewardResponse{
			Code: 2,
			Msg:  "尚未购买七日奖励",
		}, nil
	}

	if handled, err := s.handleSevenDayExpiration(ctx, userID, sevenDayData); err != nil {
		s.logger.Error("处理七日过期奖励失败", zap.Error(err))
	} else if handled {
		return &game.ClaimSevenDayRewardResponse{
			Code: 4,
			Msg:  "购买周期已过期，奖励已通过邮件补发",
		}, nil
	}

	// 检查是否在有效周期内
	purchaseTime, err := time.Parse(time.RFC3339, sevenDayData.LastPurchaseTime)
	if err != nil {
		s.logger.Error("解析购买时间失败", zap.Error(err))
		return &game.ClaimSevenDayRewardResponse{
			Code: 3,
			Msg:  "购买数据异常",
		}, nil
	}

	daysPassed := int32(time.Since(purchaseTime).Hours() / 24)

	// 计算当前可领取的天数
	availableDays := daysPassed + 1
	if availableDays > 7 {
		availableDays = 7
	}

	// 构建已领取天数的集合，用于快速查找
	claimedSet := make(map[int32]struct{}, len(sevenDayData.ClaimedDays))
	for _, claimedDay := range sevenDayData.ClaimedDays {
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

		// day 0 是购买后立即可领取，day 1-7 需要等相应天数
		if day > 0 && day > availableDays {
			return &game.ClaimSevenDayRewardResponse{
				Code: 6,
				Msg:  fmt.Sprintf("第%d天的奖励尚未解锁，当前只能领取第%d天及之前的奖励", day, availableDays),
			}, nil
		}

		toClaimDays = append(toClaimDays, day)
	}

	// 如果所有天数都已领取
	if len(toClaimDays) == 0 {
		return &game.ClaimSevenDayRewardResponse{
			Code: 5,
			Msg:  fmt.Sprintf("这些奖励已领取: %v", alreadyClaimedDays),
		}, nil
	}

	// 收集所有有效的奖励配置
	var allRewards []*game.Reward
	for _, day := range toClaimDays {
		reward, err := s.getSevenDayReward(day)
		if err != nil {
			s.logger.Error("获取七日奖励配置失败", zap.Error(err), zap.Int32("day", day))
			return &game.ClaimSevenDayRewardResponse{
				Code: 7,
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
			source := fmt.Sprintf("seven_day_reward_merged:%v", toClaimDays)
			wResult, iResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, mergedReward, source)
			if err != nil {
				s.logger.Error("发放七日奖励失败", zap.Error(err), zap.Int32s("days", toClaimDays))
				return &game.ClaimSevenDayRewardResponse{
					Code: 8,
					Msg:  "发放奖励失败",
				}, nil
			}
			walletUpdateResult = wResult
			inventoryUpdateResult = iResult
		}
	}

	// 更新已领取天数
	sevenDayData.ClaimedDays = append(sevenDayData.ClaimedDays, toClaimDays...)
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, sevenDayData); err != nil {
		s.logger.Error("保存七日购买数据失败", zap.Error(err))
		return &game.ClaimSevenDayRewardResponse{
			Code: 9,
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

	s.logger.Info("七日奖励领取成功",
		zap.String("user_id", userID.String()),
		zap.Int32s("days", toClaimDays))

	return &game.ClaimSevenDayRewardResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           mergedReward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ClaimedDays:      sevenDayData.ClaimedDays,
	}, nil
}

// getSevenDayReward 获取七日奖励配置
func (s *ApiServer) getSevenDayReward(day int32) (*game.Reward, error) {
	// 根据天数查找配置
	configs := s.template.GetTplSevenDay().FindByFilter(func(sd template.TplSevenDay) bool {
		return sd.Day == day
	})

	if configs.Len() == 0 {
		return nil, fmt.Errorf("找不到第%d天的七日奖励配置", day)
	}

	config := configs.Get(0)

	// 使用通用奖励配置表
	reward := GetReward(config.Reward, s.template.GetTplReward(), s.logger)
	if reward == nil {
		return nil, fmt.Errorf("解析奖励配置失败，reward_id=%s", config.Reward)
	}

	return reward, nil
}

// RecordSevenDayPurchase 记录七日购买（在充值回调中调用）
func (s *ApiServer) RecordSevenDayPurchase(ctx context.Context, userID uuid.UUID) error {
	// 从 storage 加载七日购买数据
	sevenDayData := &SevenDayData{}
	if err := LoadUserData(ctx, s.logger, s.db, sevenDayData); err != nil {
		s.logger.Error("加载七日购买数据失败", zap.Error(err))
		sevenDayData.Init()
	}

	if _, err := s.handleSevenDayExpiration(ctx, userID, sevenDayData); err != nil {
		s.logger.Error("记录七日购买时处理过期奖励失败", zap.Error(err))
	}

	// 记录购买（可以重复购买）
	sevenDayData.LastPurchaseTime = time.Now().Format(time.RFC3339)
	sevenDayData.ClaimedDays = []int32{} // 重置领取记录
	sevenDayData.TotalPurchases++        // 增加购买次数

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, sevenDayData); err != nil {
		return fmt.Errorf("保存七日购买数据失败: %w", err)
	}

	s.logger.Info("七日购买记录成功",
		zap.String("user_id", userID.String()),
		zap.String("purchase_time", sevenDayData.LastPurchaseTime),
		zap.Int32("total_purchases", sevenDayData.TotalPurchases))
	return nil
}

// handleSevenDayExpiration 检测并处理七日购买过期补偿
func (s *ApiServer) handleSevenDayExpiration(ctx context.Context, userID uuid.UUID, sevenDayData *SevenDayData) (bool, error) {
	if sevenDayData == nil || sevenDayData.LastPurchaseTime == "" {
		return false, nil
	}

	purchaseTime, err := time.Parse(time.RFC3339, sevenDayData.LastPurchaseTime)
	if err != nil {
		sevenDayData.Init()
		return false, SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, sevenDayData)
	}

	if time.Since(purchaseTime) < 7*24*time.Hour {
		return false, nil
	}

	pendingDays, pendingRewards := s.collectUnclaimedSevenDayRewards(sevenDayData.ClaimedDays)
	if len(pendingRewards) > 0 {
		if err := s.sendSevenDayExpiredRewardMail(ctx, userID, pendingDays, pendingRewards); err != nil {
			return false, err
		}
	}

	sevenDayData.LastPurchaseTime = ""
	sevenDayData.ClaimedDays = []int32{}
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, sevenDayData); err != nil {
		return false, err
	}

	return true, nil
}

func (s *ApiServer) collectUnclaimedSevenDayRewards(claimedDays []int32) ([]int32, []*game.Reward) {
	claimedSet := make(map[int32]struct{}, len(claimedDays))
	for _, day := range claimedDays {
		claimedSet[day] = struct{}{}
	}

	allEntries := s.template.GetTplSevenDay().FindAll().ToSlice()
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Day < allEntries[j].Day
	})

	var pendingDays []int32
	var pendingRewards []*game.Reward
	for _, entry := range allEntries {
		if _, ok := claimedSet[entry.Day]; ok {
			continue
		}
		reward := GetReward(entry.Reward, s.template.GetTplReward(), s.logger)
		if reward == nil {
			s.logger.Warn("七日奖励配置缺失", zap.Int32("day", entry.Day), zap.String("reward_id", entry.Reward))
			continue
		}
		pendingDays = append(pendingDays, entry.Day)
		pendingRewards = append(pendingRewards, reward)
	}
	return pendingDays, pendingRewards
}

func (s *ApiServer) sendSevenDayExpiredRewardMail(ctx context.Context, userID uuid.UUID, pendingDays []int32, rewards []*game.Reward) error {
	if len(rewards) == 0 {
		return nil
	}

	content := &console.NoticeContent{
		Description: fmt.Sprintf("您还有%d天的七日奖励未领取，系统已通过邮件补发，请注意查收。", len(pendingDays)),
		Rewards:     rewards,
	}
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("序列化七日补发邮件失败: %w", err)
	}

	notification := &api.Notification{
		Id:         uuid.Must(uuid.NewV4()).String(),
		Subject:    "七日奖励补发",
		Content:    string(contentBytes),
		Code:       NotificationSystemNotice,
		SenderId:   uuid.Nil.String(),
		Persistent: true,
	}

	if err := NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, map[uuid.UUID][]*api.Notification{
		userID: {notification},
	}); err != nil {
		return fmt.Errorf("发送七日补发邮件失败: %w", err)
	}

	s.logger.Info("七日奖励通过邮件补发成功",
		zap.String("user_id", userID.String()),
		zap.Int("pending_reward_count", len(rewards)))
	return nil
}
