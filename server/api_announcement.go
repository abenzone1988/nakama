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

	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ConsoleServer) CreateAnnouncement(ctx context.Context, in *console.CreateAnnouncementRequest) (*console.Announcement, error) {
	// 参数验证
	if in.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "标题不能为空")
	}
	if in.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "内容不能为空")
	}
	if in.Status < 0 || in.Status > 2 {
		return nil, status.Error(codes.InvalidArgument, "状态值无效")
	}

	announcement, err := AnnouncementCreate(ctx, s.logger, s.db, in)
	if err != nil {
		s.logger.Error("创建公告失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "创建公告失败")
	}

	return announcement, nil
}

func (s *ConsoleServer) UpdateAnnouncement(ctx context.Context, in *console.UpdateAnnouncementRequest) (*console.Announcement, error) {
	// 参数验证
	if in.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "ID不能为空")
	}
	if in.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "标题不能为空")
	}
	if in.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "内容不能为空")
	}
	if in.Status < 0 || in.Status > 2 {
		return nil, status.Error(codes.InvalidArgument, "状态值无效")
	}

	announcement, err := AnnouncementUpdate(ctx, s.logger, s.db, in)
	if err != nil {
		if err == ErrAnnouncementNotFound {
			return nil, status.Error(codes.NotFound, "公告不存在")
		}
		s.logger.Error("更新公告失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "更新公告失败")
	}

	return announcement, nil
}

func (s *ConsoleServer) DeleteAnnouncement(ctx context.Context, in *console.AnnouncementId) (*emptypb.Empty, error) {
	// 参数验证
	if in.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "ID不能为空")
	}

	err := AnnouncementDelete(ctx, s.logger, s.db, in.Id)
	if err != nil {
		if err == ErrAnnouncementNotFound {
			return nil, status.Error(codes.NotFound, "公告不存在")
		}
		s.logger.Error("删除公告失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "删除公告失败")
	}

	return &emptypb.Empty{}, nil
}

func (s *ConsoleServer) GetAnnouncement(ctx context.Context, in *console.AnnouncementId) (*console.Announcement, error) {
	// 参数验证
	if in.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "ID不能为空")
	}

	announcement, err := AnnouncementGet(ctx, s.logger, s.db, in.Id)
	if err != nil {
		if err == ErrAnnouncementNotFound {
			return nil, status.Error(codes.NotFound, "公告不存在")
		}
		s.logger.Error("获取公告失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取公告失败")
	}

	return announcement, nil
}

func (s *ConsoleServer) ListAnnouncements(ctx context.Context, in *console.ListAnnouncementsRequest) (*console.AnnouncementList, error) {
	// 参数验证
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 20
	}

	announcements, err := AnnouncementList(ctx, s.logger, s.db, in.Status, in.Limit, in.Cursor)
	if err != nil {
		s.logger.Error("获取公告列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取公告列表失败")
	}

	return announcements, nil
}
