// Copyright 2018 The Nakama Authors
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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/console"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ApiServer) ListNotifications(ctx context.Context, in *api.ListNotificationsRequest) (*api.NotificationList, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// Before hook.
	if fn := s.runtime.BeforeListNotifications(); fn != nil {
		beforeFn := func(clientIP, clientPort string) error {
			result, err, code := fn(ctx, s.logger, userID.String(), ctx.Value(ctxUsernameKey{}).(string), ctx.Value(ctxVarsKey{}).(map[string]string), ctx.Value(ctxExpiryKey{}).(int64), clientIP, clientPort, in)
			if err != nil {
				return status.Error(code, err.Error())
			}
			if result == nil {
				// If result is nil, requested resource is disabled.
				s.logger.Warn("Intercepted a disabled resource.", zap.Any("resource", ctx.Value(ctxFullMethodKey{}).(string)), zap.String("uid", userID.String()))
				return status.Error(codes.NotFound, "Requested resource was not found.")
			}
			in = result
			return nil
		}

		// Execute the before function lambda wrapped in a trace for stats measurement.
		err := traceApiBefore(ctx, s.logger, s.metrics, ctx.Value(ctxFullMethodKey{}).(string), beforeFn)
		if err != nil {
			return nil, err
		}
	}

	//同步系统通知
	if err := SyncSystemNotifications(ctx, s.logger, s.db, s.statusRegistry, userID); err != nil {
		s.logger.Error("Sync System Notice error ", zap.Error(err))
	}

	limit := 1
	if in.GetLimit() != nil {
		if in.GetLimit().Value < 1 || in.GetLimit().Value > 100 {
			return nil, status.Error(codes.InvalidArgument, "Invalid limit - limit must be between 1 and 100.")
		}
		limit = int(in.GetLimit().Value)
	}

	notificationList, err := NotificationList(ctx, s.logger, s.db, userID, limit, in.CacheableCursor, true)
	if err != nil {
		return nil, status.Error(codes.Internal, "Error retrieving notifications.")
	}

	// After hook.
	if fn := s.runtime.AfterListNotifications(); fn != nil {
		afterFn := func(clientIP, clientPort string) error {
			return fn(ctx, s.logger, userID.String(), ctx.Value(ctxUsernameKey{}).(string), ctx.Value(ctxVarsKey{}).(map[string]string), ctx.Value(ctxExpiryKey{}).(int64), clientIP, clientPort, notificationList, in)
		}

		// Execute the after function lambda wrapped in a trace for stats measurement.
		traceApiAfter(ctx, s.logger, s.metrics, ctx.Value(ctxFullMethodKey{}).(string), afterFn)
	}

	return notificationList, nil
}

func (s *ApiServer) DeleteNotifications(ctx context.Context, in *api.DeleteNotificationsRequest) (*emptypb.Empty, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// Before hook.
	if fn := s.runtime.BeforeDeleteNotifications(); fn != nil {
		beforeFn := func(clientIP, clientPort string) error {
			result, err, code := fn(ctx, s.logger, ctx.Value(ctxUserIDKey{}).(uuid.UUID).String(), ctx.Value(ctxUsernameKey{}).(string), ctx.Value(ctxVarsKey{}).(map[string]string), ctx.Value(ctxExpiryKey{}).(int64), clientIP, clientPort, in)
			if err != nil {
				return status.Error(code, err.Error())
			}
			if result == nil {
				// If result is nil, requested resource is disabled.
				s.logger.Warn("Intercepted a disabled resource.", zap.Any("resource", ctx.Value(ctxFullMethodKey{}).(string)), zap.String("uid", userID.String()))
				return status.Error(codes.NotFound, "Requested resource was not found.")
			}
			in = result
			return nil
		}

		// Execute the before function lambda wrapped in a trace for stats measurement.
		err := traceApiBefore(ctx, s.logger, s.metrics, ctx.Value(ctxFullMethodKey{}).(string), beforeFn)
		if err != nil {
			return nil, err
		}
	}

	if len(in.GetIds()) == 0 {
		return &emptypb.Empty{}, nil
	}

	if err := NotificationDelete(ctx, s.logger, s.db, userID, in.GetIds()); err != nil {
		return nil, status.Error(codes.Internal, "Error while deleting notifications.")
	}

	// After hook.
	if fn := s.runtime.AfterDeleteNotifications(); fn != nil {
		afterFn := func(clientIP, clientPort string) error {
			return fn(ctx, s.logger, ctx.Value(ctxUserIDKey{}).(uuid.UUID).String(), ctx.Value(ctxUsernameKey{}).(string), ctx.Value(ctxVarsKey{}).(map[string]string), ctx.Value(ctxExpiryKey{}).(int64), clientIP, clientPort, in)
		}

		// Execute the after function lambda wrapped in a trace for stats measurement.
		traceApiAfter(ctx, s.logger, s.metrics, ctx.Value(ctxFullMethodKey{}).(string), afterFn)
	}

	return &emptypb.Empty{}, nil
}

func (s *ApiServer) MarkNotificationsRead(ctx context.Context, in *game.MarkNotificationsReadRequest) (*game.MarkNotificationsReadResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	if len(in.GetIds()) == 0 {
		return &game.MarkNotificationsReadResponse{Markedcount: 0}, nil
	}

	markedCount, err := NotificationMarkRead(ctx, s.logger, s.db, userID, in.GetIds())
	if err != nil {
		return nil, status.Error(codes.Internal, "Error while marking notifications as read.")
	}

	return &game.MarkNotificationsReadResponse{Markedcount: markedCount}, nil
}

func (s *ApiServer) ClaimNotificationAttachments(ctx context.Context, in *game.ClaimNotificationAttachmentsRequest) (*game.ClaimNotificationAttachmentsResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	if len(in.GetIds()) == 0 {
		return &game.ClaimNotificationAttachmentsResponse{Claimedcount: 0}, nil
	}

	contents, claimedCount, err := NotificationClaimAttachments(ctx, s.logger, s.db, userID, in.GetIds())
	if err != nil {
		if errors.Is(err, ErrNotificationAlreadyClaimed) {
			return nil, status.Error(codes.FailedPrecondition, "Some notifications are already claimed.")
		}
		return nil, status.Error(codes.Internal, "Error while claiming notification attachments.")
	}

	var walletSnapshot *game.Wallet
	inventoryChanges := make(map[string]int32)

	for id, contentStr := range contents {
		if contentStr == "" {
			continue
		}
		var noticeContent console.NoticeContent
		if err := json.Unmarshal([]byte(contentStr), &noticeContent); err != nil {
			s.logger.Error("解析通知内容失败", zap.String("notification_id", id), zap.Error(err))
			return nil, status.Error(codes.Internal, "Error while parsing notification attachments.")
		}

		for _, reward := range noticeContent.GetRewards() {
			if reward == nil {
				continue
			}
			source := fmt.Sprintf("notification:%s", id)
			walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
			if err != nil {
				s.logger.Error("发放通知奖励失败", zap.String("notification_id", id), zap.Error(err))
				return nil, status.Error(codes.Internal, "Error while claiming notification attachments.")
			}

			if walletResult != nil && walletResult.Updated != nil {
				walletSnapshot = convertGameWalletToAPI(walletResult.Updated)
			}
			if inventoryResult != nil && len(inventoryResult.Updated) > 0 {
				for _, item := range inventoryResult.Updated {
					if item == nil || item.Id == "" || item.Num == 0 {
						continue
					}
					inventoryChanges[item.Id] += item.Num
				}
			}
		}
	}

	var inventoryUpdated []*game.Item
	if len(inventoryChanges) > 0 {
		inventoryUpdated = make([]*game.Item, 0, len(inventoryChanges))
		for id, num := range inventoryChanges {
			inventoryUpdated = append(inventoryUpdated, &game.Item{
				Id:  id,
				Num: num,
			})
		}
	}

	return &game.ClaimNotificationAttachmentsResponse{
		Claimedcount:     claimedCount,
		WalletUpdated:    walletSnapshot,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

func convertGameWalletToAPI(wallet *game.Wallet) *game.Wallet {
	if wallet == nil {
		return nil
	}
	return &game.Wallet{
		Coin: wallet.Coin,
		Gem:  wallet.Gem,
		Ad:   wallet.Ad,
	}
}
