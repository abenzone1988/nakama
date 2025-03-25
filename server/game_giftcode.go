package server

import (
	"context"
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"

	"go.uber.org/zap"
	"time"
)

type RedeemRecord struct {
	Code       string    `json:"code"`
	RedeemTime time.Time `json:"time"`
}

type RedeemHistory struct {
	Records map[string]*RedeemRecord `json:"records"`
}

func (f *RedeemHistory) GetCollection() string {
	return "user_data"
}
func (f *RedeemHistory) GetKey() string {
	return "redeem"
}

func (f *RedeemHistory) Init() {
	f.Records = make(map[string]*RedeemRecord)
}

func (s *ApiServer) RedeemGift(ctx context.Context, in *game.RedeemGiftRequest) (*game.RedeemGiftResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	redeemHistory := &RedeemHistory{}
	err := LoadData(ctx, s.logger, s.db, userID, redeemHistory)
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
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, redeemHistory)
	if err != nil {
		return nil, err
	}

	return &game.RedeemGiftResponse{Code: 0, Msg: "领取成功"}, nil
}
