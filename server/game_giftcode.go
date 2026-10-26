package server

import (
	"context"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"

	"go.uber.org/zap"
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

	// 客户端输入可能带首尾空格，先归一化再查表
	giftCode := strings.TrimSpace(in.GetGiftCode())
	if giftCode == "" {
		return &game.RedeemGiftResponse{Code: 2, Msg: "兑换码无效"}, nil
	}

	tplRedemption, found := s.template.GetTplRedemption().FindByKey(giftCode)
	if !found {
		s.logger.Info("兑换码无效", zap.String("gift_code", giftCode))
		return &game.RedeemGiftResponse{Code: 2, Msg: "兑换码无效"}, nil
	}

	// 解析兑换码配置的奖励，随响应下发给客户端。
	// 奖励内容全部来自 TplReward 表，新增兑换码无需客户端发版。
	rewards, err := ParseRewards(s.logger, s.template.GetTplReward(), []string{tplRedemption.RewardID})
	if err != nil {
		s.logger.Error("解析兑换码奖励失败",
			zap.String("gift_code", giftCode),
			zap.String("reward_id", tplRedemption.RewardID),
			zap.Error(err))
		return nil, err
	}
	if len(rewards) == 0 {
		s.logger.Error("兑换码奖励配置不存在",
			zap.String("gift_code", giftCode),
			zap.String("reward_id", tplRedemption.RewardID))
		return &game.RedeemGiftResponse{Code: 3, Msg: "兑换码奖励配置错误"}, nil
	}

	redeemHistory := &RedeemHistory{}
	if err := LoadData(ctx, s.logger, s.db, userID, redeemHistory); err != nil {
		return nil, err
	}
	// 存储中若是 {"records":null}，反序列化后 Records 仍为 nil，直接赋值会 panic
	if redeemHistory.Records == nil {
		redeemHistory.Init()
	}

	// 检查兑换码是否已经领取
	if _, exists := redeemHistory.Records[giftCode]; exists {
		s.logger.Info("兑换码已被领取",
			zap.String("gift_code", giftCode),
			zap.String("user_id", userID.String()))
		return &game.RedeemGiftResponse{Code: 1, Msg: "兑换码已被领取"}, nil
	}

	// 添加新的领取记录
	redeemHistory.Records[giftCode] = &RedeemRecord{
		Code:       giftCode,
		RedeemTime: time.Now(),
	}

	// 保存更新后的领取历史记录
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, redeemHistory); err != nil {
		return nil, err
	}

	s.logger.Info("兑换码领取成功",
		zap.String("gift_code", giftCode),
		zap.String("user_id", userID.String()),
		zap.String("reward_id", tplRedemption.RewardID))

	return &game.RedeemGiftResponse{Code: 0, Msg: "领取成功", Reward: rewards}, nil
}
