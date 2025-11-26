// Copyright 2024 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CheckVipStatus 检查当前用户的VIP状态
func (s *ApiServer) CheckVipStatus(ctx context.Context, in *emptypb.Empty) (*game.CheckVipStatusResponse, error) {
	// 从上下文中获取用户ID
	userID := ctx.Value(ctxUserIDKey{})
	if userID == nil {
		return nil, status.Error(codes.Unauthenticated, "No user ID found in context.")
	}

	userIDUUID, ok := userID.(uuid.UUID)
	if !ok {
		return nil, status.Error(codes.Internal, "Invalid user ID format.")
	}

	// 检查用户VIP状态
	vipAccount, isVip, err := VipAccountCheck(ctx, s.logger, s.db, userIDUUID)
	if err != nil {
		s.logger.Error("检查用户VIP状态失败", zap.String("user_id", userIDUUID.String()), zap.Error(err))
		return nil, status.Error(codes.Internal, "Failed to check VIP status.")
	}

	response := &game.CheckVipStatusResponse{}

	// 如果用户是VIP，添加签名
	if isVip && vipAccount.ExpiryTime != nil {
		signature, err := GenerateVipSignature(userIDUUID.String(), vipAccount.ExpiryTime.AsTime().Unix())
		if err != nil {
			s.logger.Error("生成VIP签名失败", zap.String("user_id", userIDUUID.String()), zap.Error(err))
			// 签名生成失败不影响VIP状态返回，只是不包含签名
		} else {
			response.Signature = signature
			response.ExpireTime = vipAccount.ExpiryTime.AsTime().Unix()
			vipRewardData := &VipRewardData{}
			if err := LoadData(ctx, s.logger, s.db, userIDUUID, vipRewardData); err != nil {
				return nil, status.Error(codes.Internal, "Failed to load VIP reward data.")
			}
			response.RewardClaimed = vipRewardData.RewardClaimed
		}
	}

	return response, nil
}

// ClaimVipReward 领取VIP购买奖励（一次性）
func (s *ApiServer) ClaimVipReward(ctx context.Context, in *game.ClaimVipRewardRequest) (*game.ClaimVipRewardResponse, error) {
	userIDVal := ctx.Value(ctxUserIDKey{})
	if userIDVal == nil {
		return nil, status.Error(codes.Unauthenticated, "No user ID found in context.")
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		return nil, status.Error(codes.Internal, "Invalid user ID format.")
	}

	// 确认用户当前是VIP
	_, isVip, err := VipAccountCheck(ctx, s.logger, s.db, userID)
	if err != nil {
		s.logger.Error("检查VIP状态失败", zap.String("user_id", userID.String()), zap.Error(err))
		return nil, status.Error(codes.Internal, "Failed to check VIP status.")
	}
	if !isVip {
		return &game.ClaimVipRewardResponse{
			Code: 1,
			Msg:  "尚未购买VIP",
		}, nil
	}

	// 加载VIP奖励领取数据
	vipRewardData := &VipRewardData{}
	if err := LoadData(ctx, s.logger, s.db, userID, vipRewardData); err != nil {
		return nil, status.Error(codes.Internal, "Failed to load VIP reward data.")
	}

	if vipRewardData.RewardClaimed {
		return &game.ClaimVipRewardResponse{
			Code:          2,
			Msg:           "奖励已领取",
			RewardClaimed: true,
		}, nil
	}

	// 获取奖励配置
	reward := GetReward(VipRewardID, s.template.GetTplReward(), s.logger)
	if reward == nil {
		s.logger.Error("VIP奖励配置不存在", zap.String("reward_id", VipRewardID))
		return &game.ClaimVipRewardResponse{
			Code: 3,
			Msg:  "奖励配置不存在",
		}, nil
	}

	// 发放奖励
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "vip_reward")
	if err != nil {
		s.logger.Error("发放VIP奖励失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.ClaimVipRewardResponse{
			Code: 4,
			Msg:  "发放奖励失败",
		}, nil
	}

	// 更新领取状态
	vipRewardData.RewardClaimed = true
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, vipRewardData); err != nil {
		s.logger.Error("保存VIP奖励状态失败", zap.String("user_id", userID.String()), zap.Error(err))
		return nil, status.Error(codes.Internal, "Failed to save VIP reward state.")
	}

	var walletUpdated *game.Wallet
	if walletResult != nil {
		walletUpdated = walletResult.Updated
	}
	var inventoryUpdated []*game.Item
	if inventoryResult != nil {
		inventoryUpdated = inventoryResult.Updated
	}

	return &game.ClaimVipRewardResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		RewardClaimed:    true,
	}, nil
}
