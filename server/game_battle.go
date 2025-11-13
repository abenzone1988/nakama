package server

import (
	"context"

	"github.com/heroiclabs/nakama/v3/game"
)

func (s *ApiServer) EndBattle(ctx context.Context, in *game.EndBattleRequest) (*game.EndBattleResponse, error) {
	//扣除体力值
	if err := ConsumeStamina(ctx, s.logger, 1); err != nil {
		return &game.EndBattleResponse{
			Code: -1,
			Msg:  "体力扣除失败: " + err.Error(),
		}, nil
	}

	return &game.EndBattleResponse{
		Code: 0,
		Msg:  "体力扣除成功",
	}, nil
}
