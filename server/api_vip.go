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
		}
	}

	return response, nil
}
