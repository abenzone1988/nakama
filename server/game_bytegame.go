package server

import (
	"context"
	"fmt"
	"time"

	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// ClaimByteReward 领取抖音奖励
func (s *ApiServer) ClaimByteReward(ctx context.Context, in *game.ClaimByteRewardRequest) (*game.ClaimByteRewardResponse, error) {
	rewardID := in.GetRewardId()

	// 验证奖励ID
	if rewardID != TTShortcutRewardID && rewardID != TTEntryRewardID && rewardID != TTDirectPlayRewardID {
		return &game.ClaimByteRewardResponse{
			Code: 1,
			Msg:  "无效的奖励ID",
		}, nil
	}

	// 加载抖音奖励数据
	rewardData := &ByteRewardData{}
	if err := LoadUserData(ctx, s.logger, s.db, rewardData); err != nil {
		s.logger.Warn("加载抖音奖励数据失败，初始化新数据", zap.Error(err))
		rewardData.Init()
	}

	// 判断奖励类型并校验
	if rewardID == TTShortcutRewardID {
		// 添加到桌面奖励：仅一次机会
		if rewardData.ShortcutRewardClaimed {
			return &game.ClaimByteRewardResponse{
				Code: 2,
				Msg:  "该奖励已经领取过了",
			}, nil
		}
		rewardData.ShortcutRewardClaimed = true
	} else if rewardID == TTEntryRewardID {
		// 入口有奖：每天一次机会
		now := time.Now()
		today := now.Format("2006-01-02")

		if rewardData.EntryRewardLastDate == today {
			return &game.ClaimByteRewardResponse{
				Code: 3,
				Msg:  "今天已经领取过了，请明天再来",
			}, nil
		}
		rewardData.EntryRewardLastDate = today
	}

	if rewardID == TTDirectPlayRewardID && rewardData.GetDirectPlayReward {
		return &game.ClaimByteRewardResponse{
			Code: 4,
			Msg:  "今天已经领取过了，请明天再来领取",
		}, nil
	} else if rewardID == TTDirectPlayRewardID {
		rewardData.GetDirectPlayReward = true
	}

	// 获取奖励配置
	reward := GetReward(rewardID, s.template.GetTplReward(), s.logger)
	if reward == nil {
		s.logger.Warn("奖励配置不存在", zap.String("reward_id", rewardID))
		return &game.ClaimByteRewardResponse{
			Code: 4,
			Msg:  "奖励配置不存在",
		}, nil
	}

	// 发放奖励
	source := fmt.Sprintf("byte_reward:%s", rewardID)
	walletUpdateResult, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		s.logger.Error("发放抖音奖励失败", zap.Error(err), zap.String("reward_id", rewardID))
		return &game.ClaimByteRewardResponse{
			Code: 5,
			Msg:  fmt.Sprintf("发放奖励失败: %v", err),
		}, nil
	}

	// 保存数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, rewardData); err != nil {
		s.logger.Error("保存抖音奖励数据失败", zap.Error(err))
		return &game.ClaimByteRewardResponse{
			Code: 6,
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

	s.logger.Info("抖音奖励领取成功", zap.String("reward_id", rewardID))

	return &game.ClaimByteRewardResponse{
		Code:             0,
		Msg:              "领取成功",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}
