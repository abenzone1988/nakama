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
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ConsoleServer) AddVipAccount(ctx context.Context, in *console.AddVipAccountRequest) (*console.AddVipAccountResponse, error) {
	// 参数验证
	if len(in.GetUsernames()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "用户名不能为空")
	}

	// 获取用户名到用户ID的映射
	userMap, err := fetchUserIDsByUsernames(ctx, s.db, in.GetUsernames())
	if err != nil {
		s.logger.Error("获取用户ID失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取用户ID失败")
	}

	// 设置过期时间，如果没有提供则默认为当前时间+1年
	var expireTime time.Time
	if in.GetExpireTime() != nil {
		expireTime = in.GetExpireTime().AsTime()
	} else {
		expireTime = time.Now().AddDate(1, 0, 0) // 添加1年
	}

	var successAccounts []*console.VipAccount
	var failedAccounts []*console.VipAccountError

	// 处理每个用户名
	for _, username := range in.GetUsernames() {
		userInfo, exists := userMap[username]
		if !exists {
			// 用户名不存在
			failedAccounts = append(failedAccounts, &console.VipAccountError{
				Username:     username,
				ErrorMessage: "用户名不存在",
				ErrorCode:    "USER_NOT_FOUND",
			})
			continue
		}

		// 尝试添加VIP
		vipAccount, err := VipAccountAdd(ctx, s.logger, s.db, userInfo.UserID, username, expireTime)
		if err != nil {
			var errorMsg, errorCode string
			if err == ErrVipAccountAlreadyExists {
				errorMsg = "用户已经是VIP"
				errorCode = "ALREADY_VIP"
			} else {
				errorMsg = "添加VIP失败"
				errorCode = "ADD_FAILED"
				s.logger.Error("添加VIP账户失败", zap.String("username", username), zap.Error(err))
			}

			failedAccounts = append(failedAccounts, &console.VipAccountError{
				Username:     username,
				ErrorMessage: errorMsg,
				ErrorCode:    errorCode,
			})
			continue
		}

		successAccounts = append(successAccounts, vipAccount)
	}

	response := &console.AddVipAccountResponse{
		SuccessAccounts: successAccounts,
		FailedAccounts:  failedAccounts,
		TotalCount:      int32(len(in.GetUsernames())),
		SuccessCount:    int32(len(successAccounts)),
		FailedCount:     int32(len(failedAccounts)),
	}

	return response, nil
}

func (s *ConsoleServer) ListVipAccounts(ctx context.Context, in *console.ListVipAccountsRequest) (*console.VipAccountList, error) {
	// 参数验证
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 100
	}

	vipAccounts, err := VipAccountList(ctx, s.logger, s.db, int(in.Limit), in.Cursor, in.Filter)
	if err != nil {
		s.logger.Error("获取VIP账户列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取VIP账户列表失败")
	}

	return vipAccounts, nil
}

func (s *ConsoleServer) RemoveVipAccount(ctx context.Context, in *console.VipAccountId) (*emptypb.Empty, error) {
	// 参数验证
	if in.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "用户ID不能为空")
	}

	// 验证用户ID格式
	userID, err := uuid.FromString(in.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "用户ID格式无效")
	}

	err = VipAccountRemove(ctx, s.logger, s.db, userID)
	if err != nil {
		if err == ErrVipAccountNotFound {
			return nil, status.Error(codes.NotFound, "VIP账户不存在")
		}
		s.logger.Error("移除VIP账户失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "移除VIP账户失败")
	}

	return &emptypb.Empty{}, nil
}

func (s *ConsoleServer) CheckVipStatus(ctx context.Context, in *console.VipAccountId) (*console.VipStatusResponse, error) {
	// 参数验证
	if in.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "用户ID不能为空")
	}

	// 验证用户ID格式
	userID, err := uuid.FromString(in.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "用户ID格式无效")
	}

	vipAccount, isVip, err := VipAccountCheck(ctx, s.logger, s.db, userID)
	if err != nil {
		s.logger.Error("检查VIP状态失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "检查VIP状态失败")
	}

	response := &console.VipStatusResponse{
		IsVip: isVip,
	}

	if vipAccount != nil {
		response.VipAccount = vipAccount
	}

	return response, nil
}
