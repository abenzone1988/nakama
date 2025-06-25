package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

func (s *ApiServer) updatePlayerWallet(ctx context.Context, coinChange, gemChange, adChange int64, record string) ([]*runtime.WalletUpdateResult, error) {
	// 如果没有任何货币变化，则不执行任何操作
	if coinChange == 0 && gemChange == 0 && adChange == 0 {
		return nil, nil
	}
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	// 构造钱包更新的变化集
	changeset := make(map[string]int64)
	if coinChange != 0 {
		changeset["coin"] = coinChange
	}
	if gemChange != 0 {
		changeset["gem"] = gemChange
	}
	if adChange != 0 {
		changeset["ad"] = adChange
	}

	// 构造更新对象
	updates := []*walletUpdate{{
		UserID:    userID,
		Changeset: changeset,
		Metadata:  fmt.Sprintf(`{"message": "%s"}`, record),
	}}

	// 调用更新钱包的函数
	results, err := UpdateWallets(ctx, s.logger, s.db, updates, true)
	if err != nil {
		// 直接返回错误，让调用者处理WalletNegativeError
		return nil, err
	}

	return results, nil
}

// UseWallet 使用钱包货币接口
func (s *ApiServer) UseWallet(ctx context.Context, in *game.UseWalletRequest) (*game.UseWalletResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证请求参数
	if in.GetAmount() <= 0 {
		s.logger.Warn("无效的使用数量",
			zap.String("user_id", userID.String()),
			zap.Int32("amount", in.GetAmount()),
		)
		return &game.UseWalletResponse{
			Code: 1,
			Msg:  "使用数量必须大于0",
		}, nil
	}

	// 准备钱包变更参数（转换为负数进行扣减）
	var coinChange, gemChange, adChange int64
	var currencyName string
	useAmount := int64(-in.GetAmount()) // 转换为负数进行扣减

	switch in.GetCurrencyType() {
	case game.CurrencyType_CURRENCY_TYPE_COIN:
		coinChange = useAmount
		currencyName = "硬币"
	case game.CurrencyType_CURRENCY_TYPE_GEM:
		gemChange = useAmount
		currencyName = "宝石"
	case game.CurrencyType_CURRENCY_TYPE_AD:
		adChange = useAmount
		currencyName = "广告币"
	default:
		s.logger.Warn("无效的货币类型",
			zap.String("user_id", userID.String()),
			zap.Int32("currency_type", int32(in.GetCurrencyType())),
		)
		return &game.UseWalletResponse{
			Code: 2,
			Msg:  "无效的货币类型",
		}, nil
	}

	// 构造使用记录信息
	reason := in.GetReason()
	if reason == "" {
		reason = "钱包消费"
	}
	record := fmt.Sprintf("使用%s: %d, 原因: %s", currencyName, in.GetAmount(), reason)

	// 执行钱包更新（core_wallet.go会自动检查余额是否足够）
	results, err := s.updatePlayerWallet(ctx, coinChange, gemChange, adChange, record)
	if err != nil {
		// 检查是否是余额不足错误
		var walletErr *runtime.WalletNegativeError
		if errors.As(err, &walletErr) {
			s.logger.Warn("余额不足",
				zap.String("user_id", userID.String()),
				zap.String("currency", currencyName),
				zap.Int64("current_balance", walletErr.Current),
				zap.Int32("required_amount", in.GetAmount()),
			)
			return &game.UseWalletResponse{
				Code: 3,
				Msg:  fmt.Sprintf("%s余额不足", currencyName),
			}, nil
		}

		s.logger.Error("钱包扣减失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.UseWalletResponse{
			Code: 4,
			Msg:  "钱包扣减失败",
		}, nil
	}

	// 从结果中获取钱包数据
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果", zap.String("user_id", userID.String()))
		return &game.UseWalletResponse{
			Code: 5,
			Msg:  "未找到钱包更新结果",
		}, nil
	}

	result := results[0] // 只有一个用户的更新结果

	// 构造返回的钱包数据
	previousWallet := &game.Wallet{
		Coin: int32(result.Previous["coin"]),
		Gem:  int32(result.Previous["gem"]),
		Ad:   int32(result.Previous["ad"]),
	}

	updatedWallet := &game.Wallet{
		Coin: int32(result.Updated["coin"]),
		Gem:  int32(result.Updated["gem"]),
		Ad:   int32(result.Updated["ad"]),
	}

	currencyKey := getCurrencyKey(in.GetCurrencyType())
	s.logger.Info("钱包使用成功",
		zap.String("user_id", userID.String()),
		zap.String("currency_type", currencyName),
		zap.Int32("used_amount", in.GetAmount()),
		zap.String("reason", reason),
		zap.Int64("balance_before", result.Previous[currencyKey]),
		zap.Int64("balance_after", result.Updated[currencyKey]),
	)

	return &game.UseWalletResponse{
		Code:           0,
		Msg:            "使用成功",
		WalletUpdated:  updatedWallet,
		WalletPrevious: previousWallet,
	}, nil
}

// 辅助函数：获取货币类型对应的键名
func getCurrencyKey(currencyType game.CurrencyType) string {
	switch currencyType {
	case game.CurrencyType_CURRENCY_TYPE_COIN:
		return "coin"
	case game.CurrencyType_CURRENCY_TYPE_GEM:
		return "gem"
	case game.CurrencyType_CURRENCY_TYPE_AD:
		return "ad"
	default:
		return ""
	}
}

// GetReward 获取奖励接口
func (s *ApiServer) GetReward(ctx context.Context, in *game.GetRewardRequest) (*game.GetRewardResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证请求参数
	if in.GetRewardId() == "" {
		s.logger.Warn("奖励ID不能为空",
			zap.String("user_id", userID.String()),
		)
		return &game.GetRewardResponse{
			Code: 1,
			Msg:  "奖励ID不能为空",
		}, nil
	}

	// 通过奖励ID查找奖励模板
	rewardTemplate, found := s.template.GetTplReward().FindByKey(in.GetRewardId())
	if !found {
		s.logger.Warn("未找到指定的奖励配置",
			zap.String("user_id", userID.String()),
			zap.String("reward_id", in.GetRewardId()),
		)
		return &game.GetRewardResponse{
			Code: 2,
			Msg:  "未找到指定的奖励配置",
		}, nil
	}

	// 检查奖励是否有效（至少有一种货币大于0）
	if rewardTemplate.Coin <= 0 && rewardTemplate.Gem <= 0 && rewardTemplate.Coupon <= 0 {
		s.logger.Warn("奖励配置无效，所有货币数量都为0",
			zap.String("user_id", userID.String()),
			zap.String("reward_id", in.GetRewardId()),
		)
		return &game.GetRewardResponse{
			Code: 0, // 奖励没有配置钱包 可能有背包物品 不算错误 目前
			Msg:  "钱包没有更新",
		}, nil
	}

	// 准备钱包变更参数（正数表示增加）
	coinChange := int64(rewardTemplate.Coin)
	gemChange := int64(rewardTemplate.Gem)
	adChange := int64(rewardTemplate.Coupon) // Coupon对应ad广告券

	// 构造奖励记录信息
	record := fmt.Sprintf("获取奖励: %s, 硬币+%d, 宝石+%d, 广告券+%d",
		in.GetRewardId(), rewardTemplate.Coin, rewardTemplate.Gem, rewardTemplate.Coupon)

	// 执行钱包更新
	results, err := s.updatePlayerWallet(ctx, coinChange, gemChange, adChange, record)
	if err != nil {
		s.logger.Error("钱包更新失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.GetRewardResponse{
			Code: 4,
			Msg:  "钱包更新失败",
		}, nil
	}

	// 从结果中获取钱包数据
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果", zap.String("user_id", userID.String()))
		return &game.GetRewardResponse{
			Code: 5,
			Msg:  "未找到钱包更新结果",
		}, nil
	}

	result := results[0] // 只有一个用户的更新结果

	// 构造返回的钱包数据
	previousWallet := &game.Wallet{
		Coin: int32(result.Previous["coin"]),
		Gem:  int32(result.Previous["gem"]),
		Ad:   int32(result.Previous["ad"]),
	}

	updatedWallet := &game.Wallet{
		Coin: int32(result.Updated["coin"]),
		Gem:  int32(result.Updated["gem"]),
		Ad:   int32(result.Updated["ad"]),
	}

	s.logger.Info("奖励获取成功",
		zap.String("user_id", userID.String()),
		zap.String("reward_id", in.GetRewardId()),
		zap.String("reward_name", rewardTemplate.Name),
		zap.Int32("coin_gained", rewardTemplate.Coin),
		zap.Int32("gem_gained", rewardTemplate.Gem),
		zap.Int32("coupon_gained", rewardTemplate.Coupon),
		zap.Int64("coin_before", result.Previous["coin"]),
		zap.Int64("coin_after", result.Updated["coin"]),
		zap.Int64("gem_before", result.Previous["gem"]),
		zap.Int64("gem_after", result.Updated["gem"]),
		zap.Int64("ad_before", result.Previous["ad"]),
		zap.Int64("ad_after", result.Updated["ad"]),
	)

	return &game.GetRewardResponse{
		Code:           0,
		Msg:            "获取奖励成功",
		WalletUpdated:  updatedWallet,
		WalletPrevious: previousWallet,
	}, nil
}

func (s *ApiServer) AddWallet(ctx context.Context, in *game.AddWalletRequest) (*game.AddWalletResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 准备钱包变更参数（转换为负数进行扣减）
	var coinChange, gemChange, adChange int64

	coinChange = int64(in.Coin)
	gemChange = int64(in.Gem)
	adChange = int64(in.Ad)

	record := fmt.Sprintf("加钱包: 金币 %d 钻石 %d 广告券 %d, 原因: %s", coinChange, gemChange, adChange, in.GetReason())
	s.logger.Info("addWallet", zap.String("更新", record))

	// 执行钱包更新（core_wallet.go会自动检查余额是否足够）
	results, err := s.updatePlayerWallet(ctx, coinChange, gemChange, adChange, record)
	if err != nil {
		// 检查是否是余额不足错误
		var walletErr *runtime.WalletNegativeError
		if errors.As(err, &walletErr) {
			s.logger.Warn("钱包更新失败",
				zap.String("user_id", userID.String()),
				zap.String("原因", in.GetReason()),
				zap.Int64("金币", coinChange),
				zap.Int64("钻石", gemChange),
				zap.Int64("广告券", adChange),
			)
			return &game.AddWalletResponse{
				Code: 3,
				Msg:  fmt.Sprintf("钱包更新失败"),
			}, nil
		}

		s.logger.Error("钱包更新失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.AddWalletResponse{
			Code: 4,
			Msg:  "钱包更新失败",
		}, nil
	}

	// 从结果中获取钱包数据
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果", zap.String("user_id", userID.String()))
		return &game.AddWalletResponse{
			Code: 5,
			Msg:  "未找到钱包更新结果",
		}, nil
	}

	result := results[0] // 只有一个用户的更新结果

	// 构造返回的钱包数据
	previousWallet := &game.Wallet{
		Coin: int32(result.Previous["coin"]),
		Gem:  int32(result.Previous["gem"]),
		Ad:   int32(result.Previous["ad"]),
	}

	updatedWallet := &game.Wallet{
		Coin: int32(result.Updated["coin"]),
		Gem:  int32(result.Updated["gem"]),
		Ad:   int32(result.Updated["ad"]),
	}

	return &game.AddWalletResponse{
		Code:           0,
		Msg:            "更新成功",
		WalletUpdated:  updatedWallet,
		WalletPrevious: previousWallet,
	}, nil
}
