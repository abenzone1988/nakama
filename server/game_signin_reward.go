package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

const signInDateLayout = "2006-01-02"

var errSignInAlreadyClaimed = errors.New("daily sign-in reward already claimed")

// ClaimSignInReward 领取每日签到奖励
func (s *ApiServer) ClaimSignInReward(ctx context.Context, in *game.ClaimSignInRewardRequest) (*game.ClaimSignInRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		s.logger.Error("获取用户元数据失败", zap.Error(err))
		return &game.ClaimSignInRewardResponse{
			Code: 1,
			Msg:  "获取用户数据失败",
		}, nil
	}

	if userMeta.SignIn == nil {
		initializeSignInData(userMeta)
	}

	today := time.Now().Format(signInDateLayout)
	if userMeta.SignIn.LastClaimDate == today {
		return &game.ClaimSignInRewardResponse{
			Code: 2,
			Msg:  "今日已领取",
		}, nil
	}

	nextDay := userMeta.SignIn.CurrentDay + 1
	if nextDay > DailySignInTotalDays {
		nextDay = 1
	}

	reward, err := s.getDailySignInReward(nextDay)
	if err != nil {
		s.logger.Error("获取每日签到奖励失败", zap.Error(err), zap.Int32("day", nextDay))
		return &game.ClaimSignInRewardResponse{
			Code: 3,
			Msg:  "奖励配置不存在",
		}, nil
	}

	source := fmt.Sprintf("daily_sign_in_day_%d", nextDay)
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, source)
	if err != nil {
		s.logger.Error("发放每日签到奖励失败", zap.Error(err), zap.Int32("day", nextDay))
		return &game.ClaimSignInRewardResponse{
			Code: 4,
			Msg:  "发放奖励失败",
		}, nil
	}

	if err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		if meta.SignIn == nil {
			meta.SignIn = &game.SignInData{}
		}
		if meta.SignIn.LastClaimDate == today {
			return errSignInAlreadyClaimed
		}
		meta.SignIn.CurrentDay = nextDay
		meta.SignIn.LastClaimDate = today
		return nil
	}); err != nil {
		if errors.Is(err, errSignInAlreadyClaimed) {
			return &game.ClaimSignInRewardResponse{
				Code: 2,
				Msg:  "今日已领取",
			}, nil
		}
		s.logger.Error("更新签到数据失败", zap.Error(err))
		return &game.ClaimSignInRewardResponse{
			Code: 5,
			Msg:  "更新数据失败",
		}, nil
	}

	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item
	if walletResult != nil {
		walletUpdated = walletResult.Updated
	}
	if inventoryResult != nil {
		inventoryUpdated = inventoryResult.Updated
	}

	s.logger.Info("每日签到领取成功",
		zap.String("user_id", userID.String()),
		zap.Int32("day", nextDay))

	return &game.ClaimSignInRewardResponse{
		Code:             0,
		Msg:              "Success",
		Day:              nextDay,
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

func (s *ApiServer) getDailySignInReward(day int32) (*game.Reward, error) {
	if day < 1 || day > DailySignInTotalDays {
		return nil, fmt.Errorf("invalid day %d", day)
	}

	id := fmt.Sprintf("%d", DailySignInBaseID+day)
	entry, ok := s.template.GetTplDailySignIn().FindByKey(id)
	if !ok {
		return nil, fmt.Errorf("签到奖励配置不存在, id=%s", id)
	}

	reward := GetReward(entry.Reward, s.template.GetTplReward(), s.logger)
	if reward == nil {
		return nil, fmt.Errorf("解析签到奖励失败, reward_id=%s", entry.Reward)
	}

	return reward, nil
}
