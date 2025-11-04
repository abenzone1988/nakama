package server

import (
	"context"
	"github.com/heroiclabs/nakama/v3/game"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ListPublishedAnnouncements 获取已发布的公告列表
func (s *ApiServer) ListPublishedAnnouncements(ctx context.Context, in *game.ListPublishedAnnouncementsRequest) (*game.ListPublishedAnnouncementsResponse, error) {
	// 参数验证
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 20
	}

	// 调用核心方法获取已发布公告
	announcements, err := AnnouncementList(ctx, s.logger, s.db, 1, in.Limit, in.Cursor) // status=1 表示已发布
	if err != nil {
		s.logger.Error("获取已发布公告列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "获取已发布公告列表失败")
	}

	// 转换为客户端响应格式
	response := &game.ListPublishedAnnouncementsResponse{
		TotalCount: announcements.TotalCount,
		NextCursor: announcements.NextCursor,
	}

	// 转换公告列表
	publishedAnnouncements := make([]*game.AnnouncementInfo, 0, len(announcements.Announcements))
	for _, announcement := range announcements.Announcements {
		publishedAnnouncements = append(publishedAnnouncements, &game.AnnouncementInfo{
			Id:         announcement.Id,
			Title:      announcement.Title,
			Content:    announcement.Content,
			Img:        announcement.Img,
			CreateTime: announcement.CreateTime,
			UpdateTime: announcement.UpdateTime,
		})
	}
	response.Announcements = publishedAnnouncements

	return response, nil
}
