package server

import (
	"context"

	"github.com/heroiclabs/nakama/v3/game"

	"time"

	"go.uber.org/zap"
)

func (s *ApiServer) RedeemGift(ctx context.Context, in *game.RedeemGiftRequest) (*game.RedeemGiftResponse, error) {
	redeemHistory := &RedeemHistory{}
	err := LoadUserData(ctx, s.logger, s.db, redeemHistory)
	if err != nil {
		return nil, err
	}

	giftCode := in.GetGiftCode()
	if _, found := s.template.GetTplRedemption().FindByKey(giftCode); !found {
		s.logger.Info("兑换码无效", zap.String("gift_code", in.GiftCode))
		return &game.RedeemGiftResponse{Code: 2, Msg: "兑换码无效"}, nil
	}
	// 检查兑换码是否已经领取
	if _, exists := redeemHistory.Records[giftCode]; exists {
		s.logger.Info("兑换码已被领取", zap.String("gift_code", in.GiftCode))
		return &game.RedeemGiftResponse{Code: 1, Msg: "兑换码已被领取"}, nil
	}

	// 添加新的领取记录
	redeemHistory.Records[giftCode] = &RedeemRecord{
		Code:       giftCode,
		RedeemTime: time.Now(),
	}

	// 保存更新后的领取历史记录
	err = SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, redeemHistory)
	if err != nil {
		return nil, err
	}

	return &game.RedeemGiftResponse{Code: 0, Msg: "领取成功"}, nil
}
