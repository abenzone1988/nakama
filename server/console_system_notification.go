// Copyright 2019 The Nakama Authors
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
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *ConsoleServer) ListSystemNotifications(ctx context.Context, in *console.ListSystemNoticeRequest) (*console.ListSystemNoticeResponse, error) {
	// 参数验证
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 100
	}

	notifications, err := SystemNotificationList(ctx, s.logger, s.db, int(in.Limit), in.Cursor)
	if err != nil {
		s.logger.Error("获取系统通知列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取系统通知列表失败")
	}

	return notifications, nil
}

func (s *ConsoleServer) CreateSystemNotification(ctx context.Context, in *console.CreateSystemNotificationRequest) (*console.SystemNotice, error) {
	// 参数验证
	notice := in.GetNotice()
	if notice == nil {
		return nil, status.Error(codes.InvalidArgument, "通知内容不能为空")
	}
	if notice.GetSubject() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知标题不能为空")
	}
	if notice.GetContent() == nil {
		return nil, status.Error(codes.InvalidArgument, "通知内容不能为空")
	}

	contentJson, err := json.Marshal(notice.GetContent())
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "通知内容格式错误")
	}

	// 计算生效时间
	var effectiveTime *timestamppb.Timestamp
	if in.Type == 1 { // 比赛类型
		// 从通知对象中获取挑战赛ID
		challengeID := notice.GetChallengeId()
		if challengeID > 0 {
			tplChallenge := s.template.GetTplChallenge()
			_, found := tplChallenge.FindByKey(int(challengeID))
			if !found {
				s.logger.Warn("挑战赛模板不存在", zap.Int32("challenge_id", challengeID))
				return nil, status.Error(codes.InvalidArgument, "挑战赛模板不存在")
			}
		} else {
			s.logger.Error("比赛类型未指定挑战赛ID", zap.Int32("challenge_id", challengeID))
			return nil, status.Error(codes.InvalidArgument, "比赛类型未指定挑战赛ID")
		}
	}

	effectiveTime = in.Notice.GetEffectiveTime()
	// 验证生效时间不能小于当前时间
	now := time.Now()
	if effectiveTime.AsTime().Before(now) {
		return nil, status.Error(codes.InvalidArgument, "生效时间不能小于当前时间")
	}

	// 创建系统通知
	notification, err := SystemNotificationCreate(ctx, s.db, s.logger, notice.GetNoticeType(), notice.GetSubject(), string(contentJson), effectiveTime, notice.GetExpiryTime(), notice.GetChallengeId())
	if err != nil {
		s.logger.Error("创建系统通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "创建系统通知失败")
	}

	// 根据类型处理发送逻辑
	switch in.Type {
	case 0: // 全体
		// 创建系统通知，不立即发送，等待生效时间
		s.logger.Info("创建全体系统通知", zap.String("subject", notice.GetSubject()))

	case 1: // 比赛
		// 创建系统通知，不立即发送，等待生效时间
		s.logger.Info("创建比赛系统通知", zap.String("subject", notice.GetSubject()))

	case 2: // 个人
		// 直接发送给指定用户，不创建系统通知
		if len(in.GetTarget()) == 0 {
			return nil, status.Error(codes.InvalidArgument, "个人类型必须指定目标用户")
		}

		userIDs, err := fetchUserID(ctx, s.db, in.GetTarget())
		if err != nil {
			s.logger.Error("获取用户ID失败", zap.Error(err))
			return nil, status.Error(codes.Internal, "获取用户ID失败")
		}

		if len(userIDs) == 0 {
			return nil, status.Error(codes.InvalidArgument, "未找到有效的用户")
		}

		notifications := make(map[uuid.UUID][]*api.Notification)
		for _, id := range userIDs {
			uid := uuid.FromStringOrNil(id)
			notifications[uid] = []*api.Notification{{
				Id:         uuid.Must(uuid.NewV4()).String(),
				Subject:    notice.GetSubject(),
				Content:    string(contentJson),
				SenderId:   uuid.Nil.String(),
				Code:       0,
				Persistent: true,
			}}
		}

		if err := NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications); err != nil {
			s.logger.Error("发送个人通知失败", zap.Error(err))
			return nil, status.Error(codes.Internal, "发送个人通知失败")
		}

		// 个人类型不返回系统通知，因为直接发送了
		return nil, nil
	}

	return notification, nil
}

func (s *ConsoleServer) DeleteSystemNotification(ctx context.Context, in *console.SystemNotificationId) (*emptypb.Empty, error) {
	// 参数验证
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知ID不能为空")
	}

	if err := SystemNotificationDelete(ctx, s.db, s.logger, in.GetId()); err != nil {
		if err == ErrSystemNotificationNotFound {
			return nil, status.Error(codes.NotFound, "通知不存在")
		}
		s.logger.Error("删除系统通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "删除系统通知失败")
	}

	return &emptypb.Empty{}, nil
}

func (s *ConsoleServer) UpdateSystemNotification(ctx context.Context, in *console.SystemNotice) (*console.SystemNotice, error) {
	// 参数验证
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "请求参数不能为空")
	}
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知ID不能为空")
	}
	if in.GetSubject() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知标题不能为空")
	}
	if in.GetContent() == nil {
		return nil, status.Error(codes.InvalidArgument, "通知内容不能为空")
	}

	contentJson, err := json.Marshal(in.GetContent())
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "通知内容格式错误")
	}

	notification, err := SystemNotificationUpdate(ctx, s.db, s.logger, in.GetId(), in.GetSubject(), string(contentJson), in.GetEffectiveTime(), in.GetExpiryTime())
	if err != nil {
		if err == ErrSystemNotificationNotFound {
			return nil, status.Error(codes.NotFound, "通知不存在")
		}
		s.logger.Error("更新系统通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "更新系统通知失败")
	}

	return notification, nil
}

func (s *ConsoleServer) GetSystemNotification(ctx context.Context, in *console.SystemNotificationId) (*console.SystemNotice, error) {
	// 参数验证
	if in.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "通知ID不能为空")
	}

	notification, err := SystemNotificationGet(ctx, s.db, s.logger, in.GetId())
	if err != nil {
		if err == ErrSystemNotificationNotFound {
			return nil, status.Error(codes.NotFound, "通知不存在")
		}
		s.logger.Error("获取系统通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取系统通知失败")
	}

	return notification, nil
}
