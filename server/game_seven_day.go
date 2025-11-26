package server

import (
	"context"
	"encoding/json"
	"errors"
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

	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		s.logger.Error("获取用户元数据失败", zap.Error(err))
		return &game.GetSevenDayStatusResponse{
			Code: 1,
			Msg:  "获取用户数据失败",
		}, nil
	}

	// 如果七日购买数据为空，初始化
	if userMeta.SevenDay == nil {
		userMeta.SevenDay = &game.SevenDayData{
			LastPurchaseTime: "",
			ClaimedDays:      []int32{},
			TotalPurchases:   0,
		}
	}

	if _, err := s.handleSevenDayExpiration(ctx, userID, userMeta); err != nil {
		s.logger.Error("处理七日过期奖励失败", zap.Error(err))
	}

	// 检查是否在有效购买周期内
	isPurchased := false
	availableDays := int32(0)

	if userMeta.SevenDay.LastPurchaseTime != "" {
		purchaseTime, err := time.Parse(time.RFC3339, userMeta.SevenDay.LastPurchaseTime)
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

	return &game.GetSevenDayStatusResponse{
		Code:           0,
		Msg:            "Success",
		IsPurchased:    isPurchased,
		PurchaseTime:   userMeta.SevenDay.LastPurchaseTime,
		ClaimedDays:    userMeta.SevenDay.ClaimedDays,
		AvailableDays:  availableDays,
		TotalPurchases: userMeta.SevenDay.TotalPurchases,
	}, nil
}

// ClaimSevenDayReward 领取七日奖励
func (s *ApiServer) ClaimSevenDayReward(ctx context.Context, in *game.ClaimSevenDayRewardRequest) (*game.ClaimSevenDayRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证天数参数 (0-7)
	if in.Day < 0 || in.Day > 7 {
		return &game.ClaimSevenDayRewardResponse{
			Code: 1,
			Msg:  "无效的天数参数",
		}, nil
	}

	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		s.logger.Error("获取用户元数据失败", zap.Error(err))
		return &game.ClaimSevenDayRewardResponse{
			Code: 1,
			Msg:  "获取用户数据失败",
		}, nil
	}

	// 检查是否已购买
	if userMeta.SevenDay == nil || userMeta.SevenDay.LastPurchaseTime == "" {
		return &game.ClaimSevenDayRewardResponse{
			Code: 2,
			Msg:  "尚未购买七日奖励",
		}, nil
	}

	if handled, err := s.handleSevenDayExpiration(ctx, userID, userMeta); err != nil {
		s.logger.Error("处理七日过期奖励失败", zap.Error(err))
	} else if handled {
		return &game.ClaimSevenDayRewardResponse{
			Code: 4,
			Msg:  "购买周期已过期，奖励已通过邮件补发",
		}, nil
	}

	// 检查是否在有效周期内
	purchaseTime, err := time.Parse(time.RFC3339, userMeta.SevenDay.LastPurchaseTime)
	if err != nil {
		s.logger.Error("解析购买时间失败", zap.Error(err))
		return &game.ClaimSevenDayRewardResponse{
			Code: 3,
			Msg:  "购买数据异常",
		}, nil
	}

	daysPassed := int32(time.Since(purchaseTime).Hours() / 24)

	// 检查是否已领取过该天的奖励
	for _, claimedDay := range userMeta.SevenDay.ClaimedDays {
		if claimedDay == in.Day {
			return &game.ClaimSevenDayRewardResponse{
				Code: 5,
				Msg:  "该奖励已领取",
			}, nil
		}
	}

	// 计算当前可领取的天数
	availableDays := daysPassed + 1
	if availableDays > 7 {
		availableDays = 7
	}

	// day 0 是购买后立即可领取，day 1-7 需要等相应天数
	if in.Day > 0 && in.Day > availableDays {
		return &game.ClaimSevenDayRewardResponse{
			Code: 6,
			Msg:  fmt.Sprintf("当前只能领取第%d天及之前的奖励", availableDays),
		}, nil
	}

	// 获取奖励配置
	reward, err := s.getSevenDayReward(in.Day)
	if err != nil {
		s.logger.Error("获取七日奖励配置失败", zap.Error(err), zap.Int32("day", in.Day))
		return &game.ClaimSevenDayRewardResponse{
			Code: 7,
			Msg:  "奖励配置不存在",
		}, nil
	}

	// 发放奖励
	source := fmt.Sprintf("seven_day_reward_day_%d", in.Day)
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, source)
	if err != nil {
		s.logger.Error("发放七日奖励失败", zap.Error(err), zap.Int32("day", in.Day))
		return &game.ClaimSevenDayRewardResponse{
			Code: 8,
			Msg:  "发放奖励失败",
		}, nil
	}

	// 更新已领取天数
	if err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		if meta.SevenDay == nil {
			return errors.New("七日购买数据为空")
		}
		meta.SevenDay.ClaimedDays = append(meta.SevenDay.ClaimedDays, in.Day)
		return nil
	}); err != nil {
		s.logger.Error("更新七日购买数据失败", zap.Error(err))
		return &game.ClaimSevenDayRewardResponse{
			Code: 9,
			Msg:  "更新数据失败",
		}, nil
	}

	// 重新获取更新后的数据
	userMeta, _, _ = GetUserMeta(ctx)

	// 提取更新后的数据
	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item

	if walletResult != nil {
		walletUpdated = walletResult.Updated
	}
	if inventoryResult != nil {
		inventoryUpdated = inventoryResult.Updated
	}

	s.logger.Info("七日奖励领取成功",
		zap.String("user_id", userID.String()),
		zap.Int32("day", in.Day))

	return &game.ClaimSevenDayRewardResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ClaimedDays:      userMeta.SevenDay.ClaimedDays,
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
	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		return fmt.Errorf("获取用户元数据失败: %w", err)
	}

	// 如果七日购买数据为空，初始化
	if userMeta.SevenDay == nil {
		userMeta.SevenDay = &game.SevenDayData{
			LastPurchaseTime: "",
			ClaimedDays:      []int32{},
			TotalPurchases:   0,
		}
	}

	if _, err := s.handleSevenDayExpiration(ctx, userID, userMeta); err != nil {
		s.logger.Error("记录七日购买时处理过期奖励失败", zap.Error(err))
	}

	// 记录购买（可以重复购买）
	return UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		if meta.SevenDay == nil {
			meta.SevenDay = &game.SevenDayData{}
		}

		meta.SevenDay.LastPurchaseTime = time.Now().Format(time.RFC3339)
		meta.SevenDay.ClaimedDays = []int32{} // 重置领取记录
		meta.SevenDay.TotalPurchases++        // 增加购买次数

		s.logger.Info("七日购买记录成功",
			zap.String("user_id", userID.String()),
			zap.String("purchase_time", meta.SevenDay.LastPurchaseTime),
			zap.Int32("total_purchases", meta.SevenDay.TotalPurchases))
		return nil
	})
}

// handleSevenDayExpiration 检测并处理七日购买过期补偿
func (s *ApiServer) handleSevenDayExpiration(ctx context.Context, userID uuid.UUID, userMeta *game.UserMeta) (bool, error) {
	if userMeta == nil || userMeta.SevenDay == nil || userMeta.SevenDay.LastPurchaseTime == "" {
		return false, nil
	}

	purchaseTime, err := time.Parse(time.RFC3339, userMeta.SevenDay.LastPurchaseTime)
	if err != nil {
		return false, UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
			if meta.SevenDay == nil {
				meta.SevenDay = &game.SevenDayData{}
			}
			meta.SevenDay.LastPurchaseTime = ""
			meta.SevenDay.ClaimedDays = []int32{}
			return nil
		})
	}

	if time.Since(purchaseTime) < 7*24*time.Hour {
		return false, nil
	}

	pendingDays, pendingRewards := s.collectUnclaimedSevenDayRewards(userMeta.SevenDay.ClaimedDays)
	if len(pendingRewards) > 0 {
		if err := s.sendSevenDayExpiredRewardMail(ctx, userID, pendingDays, pendingRewards); err != nil {
			return false, err
		}
	}

	if err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		if meta.SevenDay == nil {
			meta.SevenDay = &game.SevenDayData{}
		}
		meta.SevenDay.LastPurchaseTime = ""
		meta.SevenDay.ClaimedDays = []int32{}
		return nil
	}); err != nil {
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
