package server

import (
	"context"
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"google.golang.org/protobuf/types/known/emptypb"
	"time"
)

type FeedbackRecord struct {
	Description string    `json:"content"`
	Issues      int32     `json:"issues"`
	ReportedAt  time.Time `json:"time"`
}

type FeedbackHistory struct {
	List []*FeedbackRecord `json:"list"`
}

func (f *FeedbackHistory) GetCollection() string {
	return "manager_data"
}

func (f *FeedbackHistory) GetKey() string {
	return "feedback"
}

func (f *FeedbackHistory) Init() {
	f.List = []*FeedbackRecord{}
}

const MaxFeedbackRecords = 10

func (s *ApiServer) Feedback(ctx context.Context, in *game.FeedbackRequest) (*emptypb.Empty, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	feedbackHistory := &FeedbackHistory{}
	err := LoadData(ctx, s.logger, s.db, userID, feedbackHistory)
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
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, feedbackHistory)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
