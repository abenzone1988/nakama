package server

import (
	"context"
	"time"

	"github.com/heroiclabs/nakama/v3/game"
	"google.golang.org/protobuf/types/known/emptypb"
)

const MaxFeedbackRecords = 10

func (s *ApiServer) Feedback(ctx context.Context, in *game.FeedbackRequest) (*emptypb.Empty, error) {
	feedbackHistory := &FeedbackHistory{}
	err := LoadUserData(ctx, s.logger, s.db, feedbackHistory)
	if err != nil {
		return nil, err
	}

	// 创建新的反馈记录
	feedbackRecord := &FeedbackRecord{
		Description: in.Description,
		Issues:      in.Issues,
		ReportedAt:  time.Now(),
	}

	// 检查反馈记录的数量，超过 MaxFeedbackRecords 个就移除最早的一个
	if len(feedbackHistory.List) >= MaxFeedbackRecords {
		feedbackHistory.List = feedbackHistory.List[1:]
	}

	// 添加新的反馈记录到列表
	feedbackHistory.List = append(feedbackHistory.List, feedbackRecord)

	// 保存更新的反馈记录
	err = SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, feedbackHistory)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
