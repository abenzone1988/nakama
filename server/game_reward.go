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
	return s.updatePlayerWalletWithID(ctx, coinChange, gemChange, adChange, record, nil)
}

func (s *ApiServer) updatePlayerWalletWithID(ctx context.Context, coinChange, gemChange, adChange int64, record string, externalID *uuid.UUID) ([]*runtime.WalletUpdateResult, error) {
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
		UserID:     userID,
		Changeset:  changeset,
		Metadata:   fmt.Sprintf(`{"message": "%s"}`, record),
		ExternalID: externalID,
	}}

	// 调用更新钱包的函数
	results, err := UpdateWallets(ctx, s.logger, s.db, updates, true)
	if err != nil {
		// 直接返回错误，让调用者处理WalletNegativeError
		return nil, err
	}

	return results, nil
}

func (s *ApiServer) GainWallet(ctx context.Context, in *game.GainWalletRequest) (*game.WalletResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	username := ctx.Value(ctxUsernameKey{}).(string)

	// 验证签名
	if in.GetSignature() == "" {
		s.logger.Warn("钱包增加操作缺少签名",
			zap.String("user_id", userID.String()),
			zap.String("username", username))
		return &game.WalletResponse{
			Code: 1,
			Msg:  "验证失败",
		}, nil
	}

	// 验证签名
	coin := int64(in.GetCoin())
	gem := int64(in.GetGem())
	ad := int64(in.GetAd())
	reason := in.GetReason()

	isValid := VerifyWalletSignature("gain", coin, gem, ad, reason, in.GetSignature())
	if !isValid {
		s.logger.Error("钱包增加操作签名验证失败，自动封禁用户",
			zap.String("user_id", userID.String()),
			zap.String("username", username),
			zap.Int64("coin", coin),
			zap.Int64("gem", gem),
			zap.Int64("ad", ad),
			zap.String("reason", reason),
			zap.String("signature", in.GetSignature()))

		// 签名验证失败，自动BAN用户 暂时不封禁
		if err := BanUsers(ctx, s.logger, s.db, s.config, s.sessionCache, s.sessionRegistry, s.tracker, []uuid.UUID{userID}); err != nil {
			s.logger.Error("自动封禁用户失败", zap.Error(err), zap.String("user_id", userID.String()))
		} else {
			s.logger.Info("用户因签名验证失败已被自动封禁", zap.String("user_id", userID.String()), zap.String("username", username))
		}

		return &game.WalletResponse{
			Code: 2,
			Msg:  "验证失败，账户已被封禁",
		}, nil
	}

	// 准备钱包变更参数
	var coinChange, gemChange, adChange int64

	coinChange = coin
	gemChange = gem
	adChange = ad

	record := fmt.Sprintf("加钱包: 金币 %d 钻石 %d 广告券 %d, 原因: %s", coinChange, gemChange, adChange, reason)
	s.logger.Info("addWallet", zap.String("更新", record))

	// 执行钱包更新（core_wallet.go会自动检查余额是否足够）
	results, err := s.updatePlayerWallet(ctx, coinChange, gemChange, adChange, record)
	if err != nil {
		// 检查是否是余额不足错误
		var walletErr *runtime.WalletNegativeError
		if errors.As(err, &walletErr) {
			s.logger.Warn("钱包更新失败",
				zap.String("user_id", userID.String()),
				zap.String("原因", reason),
				zap.Int64("金币", coinChange),
				zap.Int64("钻石", gemChange),
				zap.Int64("广告券", adChange),
			)
			return &game.WalletResponse{
				Code: 3,
				Msg:  fmt.Sprintf("钱包更新失败"),
			}, nil
		}

		s.logger.Error("钱包更新失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.WalletResponse{
			Code: 4,
			Msg:  "钱包更新失败",
		}, nil
	}

	// 从结果中获取钱包数据
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果", zap.String("user_id", userID.String()))
		return &game.WalletResponse{
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

	s.logger.Info("钱包增加操作成功",
		zap.String("user_id", userID.String()),
		zap.String("username", username),
		zap.Int64("coin_added", coinChange),
		zap.Int64("gem_added", gemChange),
		zap.Int64("ad_added", adChange),
		zap.String("reason", reason))

	return &game.WalletResponse{
		Code:           0,
		Msg:            "更新成功",
		WalletUpdated:  updatedWallet,
		WalletPrevious: previousWallet,
	}, nil
}

func (s *ApiServer) ConsumeWallet(ctx context.Context, in *game.ConsumeWalletRequest) (*game.WalletResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	username := ctx.Value(ctxUsernameKey{}).(string)

	// 验证签名
	if in.GetSignature() == "" {
		s.logger.Warn("钱包消费操作缺少签名",
			zap.String("user_id", userID.String()),
			zap.String("username", username))
		return &game.WalletResponse{
			Code: 1,
			Msg:  "验证失败",
		}, nil
	}

	// 验证请求参数
	if in.GetCoin() < 0 || in.GetGem() < 0 || in.GetAd() < 0 {
		s.logger.Warn("无效的使用数量",
			zap.String("user_id", userID.String()),
			zap.Int32("coin", in.GetCoin()),
			zap.Int32("gem", in.GetGem()),
			zap.Int32("ad", in.GetAd()),
		)
		return &game.WalletResponse{
			Code: 2,
			Msg:  "使用数量不能为负数",
		}, nil
	}

	// 验证签名
	coin := int64(in.GetCoin())
	gem := int64(in.GetGem())
	ad := int64(in.GetAd())
	reason := in.GetReason()

	if reason == "" {
		reason = "钱包消费"
	}

	isValid := VerifyWalletSignature("consume", coin, gem, ad, reason, in.GetSignature())
	if !isValid {
		s.logger.Error("钱包消费操作签名验证失败，自动封禁用户",
			zap.String("user_id", userID.String()),
			zap.String("username", username),
			zap.Int64("coin", coin),
			zap.Int64("gem", gem),
			zap.Int64("ad", ad),
			zap.String("reason", reason),
			zap.String("signature", in.GetSignature()))

		// 签名验证失败，自动BAN用户
		if err := BanUsers(ctx, s.logger, s.db, s.config, s.sessionCache, s.sessionRegistry, s.tracker, []uuid.UUID{userID}); err != nil {
			s.logger.Error("自动封禁用户失败", zap.Error(err), zap.String("user_id", userID.String()))
		} else {
			s.logger.Info("用户因签名验证失败已被自动封禁", zap.String("user_id", userID.String()), zap.String("username", username))
		}

		return &game.WalletResponse{
			Code: 3,
			Msg:  "签名验证失败，账户已被封禁",
		}, nil
	}

	// 准备钱包变更参数（转换为负数进行扣减）
	var coinChange, gemChange, adChange int64

	coinChange = -coin
	gemChange = -gem
	adChange = -ad

	record := fmt.Sprintf("使用 coin:%d gem:%d ad:%d, 原因: %s", coin, gem, ad, reason)

	// 执行钱包更新（core_wallet.go会自动检查余额是否足够）
	results, err := s.updatePlayerWallet(ctx, coinChange, gemChange, adChange, record)
	if err != nil {
		// 检查是否是余额不足错误
		var walletErr *runtime.WalletNegativeError
		if errors.As(err, &walletErr) {
			s.logger.Warn("余额不足",
				zap.String("user_id", userID.String()),
				zap.String("path", walletErr.Path),
				zap.Int64("current_balance", walletErr.Current),
				zap.Int64("required_amount", -walletErr.Amount),
			)
			return &game.WalletResponse{
				Code: 4,
				Msg:  fmt.Sprintf("%s余额不足，当前余额：%d", walletErr.Path, walletErr.Current),
			}, nil
		}

		s.logger.Error("钱包扣减失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.WalletResponse{
			Code: 5,
			Msg:  "钱包扣减失败",
		}, nil
	}

	// 从结果中获取钱包数据
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果", zap.String("user_id", userID.String()))
		return &game.WalletResponse{
			Code: 6,
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

	s.logger.Info("钱包消费操作成功",
		zap.String("user_id", userID.String()),
		zap.String("username", username),
		zap.Int64("coin_consumed", coin),
		zap.Int64("gem_consumed", gem),
		zap.Int64("ad_consumed", ad),
		zap.String("reason", reason))

	return &game.WalletResponse{
		Code:           0,
		Msg:            "使用成功",
		WalletUpdated:  updatedWallet,
		WalletPrevious: previousWallet,
	}, nil
}
