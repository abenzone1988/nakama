package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

const signInDateLayout = "2006-01-02"

// ClaimSignInReward 领取每日签到奖励
func (s *ApiServer) ClaimSignInReward(ctx context.Context, in *game.ClaimSignInRewardRequest) (*game.ClaimSignInRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	signInData := &SignInData{}
	if err := LoadUserData(ctx, s.logger, s.db, signInData); err != nil {
		s.logger.Error("加载签到数据失败", zap.Error(err))
		signInData.Init()
	}

	today := time.Now().Format(signInDateLayout)
	if signInData.LastClaimDate != "" && signInData.LastClaimDate == today {
		return &game.ClaimSignInRewardResponse{
			Code: 2,
			Msg:  "今日已领取",
		}, nil
	}

	// 计算本次要领取的天数
	nextDay := signInData.ClaimedDay + 1
	if nextDay > DailySignInTotalDays {
		nextDay = 1 // 重新开始新一轮
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
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		s.logger.Error("发放每日签到奖励失败", zap.Error(err), zap.Int32("day", nextDay))
		return &game.ClaimSignInRewardResponse{
			Code: 4,
			Msg:  "发放奖励失败",
		}, nil
	}

	// 双重检查，防止并发
	if signInData.LastClaimDate != "" && signInData.LastClaimDate == today {
		return &game.ClaimSignInRewardResponse{
			Code: 2,
			Msg:  "今日已领取",
		}, nil
	}

	// 更新签到数据：保存已领取的天数
	signInData.ClaimedDay = nextDay
	signInData.LastClaimDate = today
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, signInData); err != nil {
		s.logger.Error("保存签到数据失败", zap.Error(err))
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

func (s *ApiServer) GetSignInReward(ctx context.Context, in *emptypb.Empty) (*game.GetSignInRewardResponse, error) {
	signInData := &SignInData{}
	if err := LoadUserData(ctx, s.logger, s.db, signInData); err != nil {
		s.logger.Error("加载签到数据失败", zap.Error(err))
		signInData.Init()
	}

	today := time.Now().Format(signInDateLayout)
	isClaimedToday := signInData.LastClaimDate != "" && signInData.LastClaimDate == today

	// 计算当前应该领取的天数
	var currentDay int32
	if isClaimedToday {
		// 今天已领取，返回已领取的天数
		currentDay = signInData.ClaimedDay
	} else {
		// 今天未领取，返回应该领取的天数
		currentDay = signInData.ClaimedDay + 1
		if currentDay > DailySignInTotalDays {
			currentDay = 1 // 重新开始新一轮
		}
	}

	return &game.GetSignInRewardResponse{
		Code:           0,
		Msg:            "Success",
		CurrentDay:     currentDay,
		IsClaimedToday: isClaimedToday,
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

// GetSevenDaySignIn 获取7天登录奖励信息
func (s *ApiServer) GetSevenDaySignIn(ctx context.Context, in *game.GetSevenDaySignInRequest) (*game.GetSevenDaySignInResponse, error) {
	sevenDayData := &SevenDaySignInData{}
	if err := LoadUserData(ctx, s.logger, s.db, sevenDayData); err != nil {
		s.logger.Error("加载7天登录数据失败", zap.Error(err))
		sevenDayData.Init()
	}

	return &game.GetSevenDaySignInResponse{
		Code:          0,
		Msg:           "Success",
		ClaimedDay:    sevenDayData.ClaimedDay,
		LastClaimDate: sevenDayData.LastClaimDate,
	}, nil
}

// ClaimSevenDaySignIn 领取7天登录奖励
func (s *ApiServer) ClaimSevenDaySignIn(ctx context.Context, in *game.ClaimSevenDaySignInRequest) (*game.ClaimSevenDaySignInResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	sevenDayData := &SevenDaySignInData{}
	if err := LoadUserData(ctx, s.logger, s.db, sevenDayData); err != nil {
		s.logger.Error("加载7天登录数据失败", zap.Error(err))
		sevenDayData.Init()
	}

	today := time.Now().Format(signInDateLayout)

	// 检查今日是否已领取
	if sevenDayData.LastClaimDate != "" && sevenDayData.LastClaimDate == today {
		return &game.ClaimSevenDaySignInResponse{
			Code: 2,
			Msg:  "今日已领取",
		}, nil
	}

	// 检查是否已经领取完7天
	if sevenDayData.ClaimedDay >= SevenDaySignInTotalDays {
		return &game.ClaimSevenDaySignInResponse{
			Code: 3,
			Msg:  "已领取完所有奖励",
		}, nil
	}

	// 计算本次要领取的天数（累加）
	nextDay := sevenDayData.ClaimedDay + 1

	// 获取奖励配置
	reward, err := s.getSevenDaySignInReward(nextDay)
	if err != nil {
		s.logger.Error("获取7天登录奖励失败", zap.Error(err), zap.Int32("day", nextDay))
		return &game.ClaimSevenDaySignInResponse{
			Code: 4,
			Msg:  "奖励配置不存在",
		}, nil
	}

	// 发放奖励
	source := fmt.Sprintf("seven_day_sign_in_day_%d", nextDay)
	walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
	if err != nil {
		s.logger.Error("发放7天登录奖励失败", zap.Error(err), zap.Int32("day", nextDay))
		return &game.ClaimSevenDaySignInResponse{
			Code: 5,
			Msg:  "发放奖励失败",
		}, nil
	}

	// 双重检查，防止并发
	if sevenDayData.LastClaimDate != "" && sevenDayData.LastClaimDate == today {
		return &game.ClaimSevenDaySignInResponse{
			Code: 2,
			Msg:  "今日已领取",
		}, nil
	}

	// 更新数据：累加天数
	sevenDayData.ClaimedDay = nextDay
	sevenDayData.LastClaimDate = today
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, sevenDayData); err != nil {
		s.logger.Error("保存7天登录数据失败", zap.Error(err))
		return &game.ClaimSevenDaySignInResponse{
			Code: 6,
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

	s.logger.Info("7天登录奖励领取成功",
		zap.String("user_id", userID.String()),
		zap.Int32("day", nextDay))

	return &game.ClaimSevenDaySignInResponse{
		Code:             0,
		Msg:              "Success",
		Day:              nextDay,
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

func (s *ApiServer) getSevenDaySignInReward(day int32) (*game.Reward, error) {
	if day < 1 || day > SevenDaySignInTotalDays {
		return nil, fmt.Errorf("invalid day %d", day)
	}

	id := fmt.Sprintf("%d", SevenDaySignInBaseID+day)
	entry, ok := s.template.GetTplSevenDaySignIn().FindByKey(id)
	if !ok {
		return nil, fmt.Errorf("7天登录奖励配置不存在, id=%s", id)
	}

	reward := GetReward(entry.Reward, s.template.GetTplReward(), s.logger)
	if reward == nil {
		return nil, fmt.Errorf("解析7天登录奖励失败, reward_id=%s", entry.Reward)
	}

	return reward, nil
}
