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
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ConsoleServer) ListPersonalNotificationLogs(ctx context.Context, in *console.ListPersonalNotificationLogRequest) (*console.ListPersonalNotificationLogResponse, error) {
	// 参数验证
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 100
	}

	logs, err := PersonalNotificationLogList(ctx, s.db, s.logger, int(in.Limit), in.Cursor, in.Filter, in.DateFrom, in.DateTo)
	if err != nil {
		s.logger.Error("获取个人通知日志失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取个人通知日志失败")
	}

	return logs, nil
}

func (s *ConsoleServer) CreatePersonalNotification(ctx context.Context, in *console.CreatePersonalNotificationRequest) (*console.PersonalNotice, error) {
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
	if len(in.GetTarget()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "个人类型必须指定目标用户")
	}
	contentJson, err := json.Marshal(notice.GetContent())
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "通知内容格式错误")
	}

	userMap, err := fetchUserIDsByUsernames(ctx, s.db, in.GetTarget())
	if err != nil {
		s.logger.Error("获取用户ID失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取用户ID失败")
	}

	if len(userMap) == 0 {
		return nil, status.Error(codes.InvalidArgument, "未找到有效的用户")
	}

	// 获取当前管理员信息
	adminUsername, ok := ctx.Value(ctxConsoleUsernameKey{}).(string)
	s.logger.Info("adminUsername", zap.String("adminUsername", adminUsername))
	if !ok || adminUsername == "" {
		adminUsername = "admin"
	}

	notifications := make(map[uuid.UUID][]*api.Notification)
	successUserIDs := make([]string, 0)

	// 处理每个用户名
	for _, username := range in.GetTarget() {
		userInfo, exists := userMap[username]
		if !exists {
			// 用户名不存在
			continue
		}
		notifications[userInfo.UserID] = []*api.Notification{{
			Id:         uuid.Must(uuid.NewV4()).String(),
			Subject:    notice.GetSubject(),
			Content:    string(contentJson),
			SenderId:   uuid.Nil.String(),
			Code:       0,
			Persistent: true,
		}}
		successUserIDs = append(successUserIDs, userInfo.Username)
	}

	if len(notifications) == 0 {
		return nil, status.Error(codes.InvalidArgument, "没有有效的用户ID")
	}

	if err := NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications); err != nil {
		s.logger.Error("发送个人通知失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "发送个人通知失败")
	}

	// 记录成功发送的日志
	if len(successUserIDs) > 0 {
		logID := uuid.Must(uuid.NewV4())
		err = PersonalNotificationLogCreate(ctx, s.db, s.logger, logID.String(), notice.GetSubject(), string(contentJson), successUserIDs, adminUsername)
		if err != nil {
			s.logger.Error("创建个人通知日志失败", zap.Error(err))
			// 不阻止通知发送，只记录错误
		}
	}
	// 个人类型不返回系统通知，因为直接发送了
	return nil, nil
}
