package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

// GetMonthlyCardStatus 获取月卡状态
func (s *ApiServer) GetMonthlyCardStatus(ctx context.Context, in *game.GetMonthlyCardStatusRequest) (*game.GetMonthlyCardStatusResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	vipAccount, isVip, err := VipAccountCheck(ctx, s.logger, s.db, userID)
	if err != nil {
		s.logger.Error("检查VIP状态失败", zap.Error(err))
		return &game.GetMonthlyCardStatusResponse{
			Code: 1,
			Msg:  "检查VIP状态失败",
		}, nil
	}

	monthlyCardData := &MonthlyCardData{}
	if err := LoadUserData(ctx, s.logger, s.db, monthlyCardData); err != nil {
		s.logger.Error("加载月卡数据失败", zap.Error(err))
		monthlyCardData.Init()
	}

	var expiryTime int64
	if isVip && vipAccount.ExpiryTime != nil {
		expiryTime = vipAccount.ExpiryTime.AsTime().Unix()
	}

	tplPays := s.template.GetTplPay().FindByFilter(func(tp template.TplPay) bool {
		return tp.ID == MonthlyCardID
	})
	if tplPays == nil {
		return &game.GetMonthlyCardStatusResponse{
			Code: 2,
			Msg:  fmt.Sprintf("商品不存在: %s", MonthlyCardID),
		}, nil
	}
	canClaimDailyReward := false
	if isVip {
		now := time.Now().UTC()
		if monthlyCardData.LastClaimTime.IsZero() {
			canClaimDailyReward = true
		} else {
			canClaimDailyReward = !IsSameDay(ctx, monthlyCardData.LastClaimTime, now)
		}
	}

	return &game.GetMonthlyCardStatusResponse{
		Code:                  0,
		Msg:                   "Success",
		IsActive:              isVip,
		ExpiryTime:            expiryTime,
		CanClaimDailyReward:   canClaimDailyReward,
		PurchaseRewardClaimed: monthlyCardData.PurchaseRewardClaimed,
	}, nil
}

// ClaimMonthlyCardReward 领取月卡奖励（购买奖励或每日奖励）
func (s *ApiServer) ClaimMonthlyCardReward(ctx context.Context, in *game.ClaimMonthlyCardRewardRequest) (*game.ClaimMonthlyCardRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	monthlyCardData := &MonthlyCardData{}
	if err := LoadUserData(ctx, s.logger, s.db, monthlyCardData); err != nil {
		s.logger.Error("加载月卡数据失败", zap.Error(err))
		monthlyCardData.Init()
	}

	if in.IsPurchaseReward {
		// 领取购买奖励
		if monthlyCardData.PurchaseRewardClaimed {
			return &game.ClaimMonthlyCardRewardResponse{
				Code: 1,
				Msg:  "购买奖励已领取",
			}, nil
		}

		if monthlyCardData.PurchaseTime == "" {
			return &game.ClaimMonthlyCardRewardResponse{
				Code: 2,
				Msg:  "尚未购买月卡",
			}, nil
		}

		reward := GetReward(MonthlyPassReward00ID, s.template.GetTplReward(), s.logger)
		if reward == nil {
			s.logger.Error("月卡购买奖励配置不存在", zap.String("reward_id", MonthlyPassReward00ID))
			return &game.ClaimMonthlyCardRewardResponse{
				Code: 3,
				Msg:  "奖励配置不存在",
			}, nil
		}

		source := "monthly_card_purchase_reward"
		walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
		if err != nil {
			s.logger.Error("发放月卡购买奖励失败", zap.Error(err))
			return &game.ClaimMonthlyCardRewardResponse{
				Code: 4,
				Msg:  "发放奖励失败",
			}, nil
		}

		monthlyCardData.PurchaseRewardClaimed = true
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, monthlyCardData); err != nil {
			s.logger.Error("保存月卡数据失败", zap.Error(err))
			return &game.ClaimMonthlyCardRewardResponse{
				Code: 5,
				Msg:  "更新数据失败",
			}, nil
		}

		var walletUpdated *game.Wallet
		if walletResult != nil {
			walletUpdated = walletResult.Updated
		}
		var inventoryUpdated []*game.Item
		if inventoryResult != nil {
			inventoryUpdated = inventoryResult.Updated
		}

		s.logger.Info("月卡购买奖励领取成功",
			zap.String("user_id", userID.String()))

		return &game.ClaimMonthlyCardRewardResponse{
			Code:             0,
			Msg:              "Success",
			Reward:           reward,
			WalletUpdated:    walletUpdated,
			InventoryUpdated: inventoryUpdated,
		}, nil
	}

	// 领取每日奖励（IsPurchaseReward = false）
	vipAccount, isVip, err := VipAccountCheck(ctx, s.logger, s.db, userID)
	if err != nil {
		s.logger.Error("检查VIP状态失败", zap.Error(err))
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 1,
			Msg:  "检查VIP状态失败",
		}, nil
	}

	if !isVip {
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 2,
			Msg:  "月卡已过期或未购买",
		}, nil
	}

	if vipAccount.ExpiryTime == nil {
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 3,
			Msg:  "VIP数据异常",
		}, nil
	}

	now := time.Now().UTC()
	if !monthlyCardData.LastClaimTime.IsZero() && IsSameDay(ctx, monthlyCardData.LastClaimTime, now) {
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 4,
			Msg:  "今日奖励已领取",
		}, nil
	}

	reward := GetReward(MonthlyPassReward01ID, s.template.GetTplReward(), s.logger)
	if reward == nil {
		s.logger.Error("月卡每日奖励配置不存在", zap.String("reward_id", MonthlyPassReward01ID))
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 5,
			Msg:  "奖励配置不存在",
		}, nil
	}

	source := "monthly_card_daily_reward"
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		s.logger.Error("发放月卡每日奖励失败", zap.Error(err))
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 6,
			Msg:  "发放奖励失败",
		}, nil
	}

	monthlyCardData.LastClaimTime = now
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, monthlyCardData); err != nil {
		s.logger.Error("保存月卡数据失败", zap.Error(err))
		return &game.ClaimMonthlyCardRewardResponse{
			Code: 7,
			Msg:  "更新数据失败",
		}, nil
	}

	var walletUpdated *game.Wallet
	if walletResult != nil {
		walletUpdated = walletResult.Updated
	}
	var inventoryUpdated []*game.Item
	if inventoryResult != nil {
		inventoryUpdated = inventoryResult.Updated
	}

	todayStr := GetDateString(ctx, now)
	s.logger.Info("月卡每日奖励领取成功",
		zap.String("user_id", userID.String()),
		zap.String("date", todayStr))

	return &game.ClaimMonthlyCardRewardResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}
