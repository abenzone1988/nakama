package server

import (
	"context"
	"github.com/heroiclabs/nakama/v3/game"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func (s *ApiServer) GetGameTime(ctx context.Context, in *emptypb.Empty) (*game.GetGameTimeResponse, error) {
	return &game.GetGameTimeResponse{GameTime: &timestamppb.Timestamp{Seconds: time.Now().Unix()}}, nil
}
