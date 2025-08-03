package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 配置
var EnableAutoBan = true          // 是否启用自动封禁
var SignatureFailBanThreshold = 5 // 阈值，5次失败才封禁

// 内存计数器（生产建议用 Redis，防止多实例不一致）
var signatureFailCounter = make(map[string]int)

// 作弊检测阈值
const CheatDetectionThreshold = 300 // 广告券>=300视为作弊

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

// 检测并处理作弊玩家
func (s *ApiServer) detectAndHandleCheatPlayer(ctx context.Context, userID uuid.UUID, walletData map[string]int64) (map[string]int64, bool, error) {
	// 检查广告券是否超过阈值
	if adAmount, exists := walletData["ad"]; exists && adAmount >= CheatDetectionThreshold {
		s.logger.Warn("检测到作弊玩家",
			zap.String("user_id", userID.String()),
			zap.Int64("ad_amount", adAmount),
			zap.Int64("threshold", CheatDetectionThreshold))

		// 清空玩家钱包
		clearChangeset := map[string]int64{
			"coin": -walletData["coin"],
			"gem":  -walletData["gem"],
			"ad":   -walletData["ad"],
		}

		// 执行钱包清零
		clearUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: clearChangeset,
			Metadata:  `{"message": "作弊检测：清空钱包"}`,
		}}

		clearResults, err := UpdateWallets(ctx, s.logger, s.db, clearUpdates, true)
		if err != nil {
			s.logger.Error("清空作弊玩家钱包失败", zap.String("user_id", userID.String()), zap.Error(err))
			return nil, false, err
		}

		// 发送警告邮件
		err = s.sendCheatWarningEmail(ctx, userID)
		if err != nil {
			s.logger.Error("发送作弊警告邮件失败", zap.String("user_id", userID.String()), zap.Error(err))
			return nil, false, err
		}

		s.logger.Info("成功处理作弊玩家",
			zap.String("user_id", userID.String()),
			zap.Int64("ad_amount", adAmount))

		// 返回清空后的钱包数据
		if len(clearResults) > 0 {
			return clearResults[0].Updated, true, nil
		}
	}

	return walletData, false, nil
}

// 发送作弊警告邮件
func (s *ApiServer) sendCheatWarningEmail(ctx context.Context, userID uuid.UUID) error {
	// 邮件内容
	emailContent := map[string]interface{}{
		"description": "经技术排查，您的账号涉及篡改数据，现已将您账号的货币重置。请阁下遵守游戏规则，勿通过任何渠道篡改游戏数据或使用外挂！",
	}

	contentBytes, err := json.Marshal(emailContent)
	if err != nil {
		s.logger.Error("序列化邮件内容失败", zap.Error(err))
		return err
	}

	// 创建通知
	notification := &api.Notification{
		Id:         uuid.Must(uuid.NewV4()).String(),
		Subject:    "篡改数据处理通知",
		Content:    string(contentBytes),
		Code:       0, // 系统通知
		SenderId:   uuid.Nil.String(),
		CreateTime: timestamppb.Now(),
		Persistent: true,
	}

	// 发送通知
	notifications := make(map[uuid.UUID][]*api.Notification)
	notifications[userID] = []*api.Notification{notification}

	err = NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications)
	if err != nil {
		s.logger.Error("发送作弊警告邮件失败",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("成功发送作弊警告邮件",
		zap.String("user_id", userID.String()))

	return nil
}

func (s *ApiServer) OperateWallet(ctx context.Context, in *game.OperateWalletRequest) (*game.WalletResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	username, _ := ctx.Value(ctxUsernameKey{}).(string)

	// 1. 参数校验
	if in.GetSignature() == "" {
		s.logger.Warn("钱包操作缺少签名", zap.String("user_id", userID.String()), zap.String("username", username))
		return &game.WalletResponse{Code: 1, Msg: "缺少签名"}, nil
	}
	if in.GetCoin() < 0 || in.GetGem() < 0 || in.GetAd() < 0 {
		s.logger.Warn("钱包操作参数非法", zap.String("user_id", userID.String()), zap.Int32("coin", in.GetCoin()), zap.Int32("gem", in.GetGem()), zap.Int32("ad", in.GetAd()))
		return &game.WalletResponse{Code: 2, Msg: "数量不能为负"}, nil
	}

	// 可选：最大值校验，防止溢出 金币钻石 10w 广告券 10
	const maxAmount = 100000
	if in.GetCoin() > maxAmount || in.GetGem() > maxAmount {
		s.logger.Warn("钱包操作参数过大", zap.String("user_id", userID.String()), zap.Int32("coin", in.GetCoin()), zap.Int32("gem", in.GetGem()), zap.Int32("ad", in.GetAd()))
		return &game.WalletResponse{Code: 3, Msg: "数量过大"}, nil
	}

	if in.GetAd() > 10 {
		s.logger.Warn("玩家作弊", zap.String("user_id", userID.String()), zap.Int32("ad", in.GetAd()))
		return &game.WalletResponse{Code: 3, Msg: "作弊行为"}, nil
	}

	// 验证walletID是否为有效的UUID
	walletId, err := uuid.FromString(in.GetWalletId())
	if err != nil {
		return &game.WalletResponse{Code: 6, Msg: "id错误"}, nil
	}

	op := in.GetOption()
	coin := int64(in.GetCoin())
	gem := int64(in.GetGem())
	ad := int64(in.GetAd())
	reason := in.GetReason()
	if reason == "" {
		reason = "钱包操作"
	}

	// 2. 操作类型校验
	var coinChange, gemChange, adChange int64
	switch op {
	case game.OperateWalletRequest_GAIN:
		coinChange, gemChange, adChange = coin, gem, ad
	case game.OperateWalletRequest_CONSUME:
		coinChange, gemChange, adChange = -coin, -gem, -ad
	default:
		s.logger.Warn("钱包操作类型未指定", zap.String("user_id", userID.String()))
		return &game.WalletResponse{Code: 4, Msg: "操作类型未指定"}, nil
	}

	// 3. 签名校验
	if !VerifyWalletSignatureV2(op.String(), coin, gem, ad, reason, in.GetSignature(), userID.String(), in.GetWalletId()) {
		s.logger.Error("钱包操作签名验证失败", zap.String("user_id", userID.String()), zap.String("username", username), zap.Int64("coin", coin), zap.Int64("gem", gem), zap.Int64("ad", ad), zap.String("reason", reason), zap.String("signature", in.GetSignature()), zap.String("wallet_id", in.GetWalletId()))

		// 计数+1
		failKey := userID.String()
		signatureFailCounter[failKey]++
		failCount := signatureFailCounter[failKey]

		// 判断是否封禁
		if EnableAutoBan && failCount >= SignatureFailBanThreshold {
			BanUsers(ctx, s.logger, s.db, s.config, s.sessionCache, s.sessionRegistry, s.tracker, []uuid.UUID{userID})
			s.logger.Warn("用户因签名多次失败被封禁", zap.String("user_id", userID.String()), zap.Int("fail_count", failCount))
		}

		return &game.WalletResponse{Code: 5, Msg: "签名验证失败"}, nil
	}

	// 4. 钱包变更
	record := fmt.Sprintf("%s钱包: 金币 %d 钻石 %d 广告券 %d, 原因: %s", op.String(), coinChange, gemChange, adChange, reason)
	results, err := s.updatePlayerWalletWithID(ctx, coinChange, gemChange, adChange, record, &walletId)
	if err != nil {
		var walletErr *runtime.WalletNegativeError
		if errors.As(err, &walletErr) {
			s.logger.Warn("钱包余额不足", zap.String("user_id", userID.String()), zap.String("原因", reason), zap.Int64("金币", coinChange), zap.Int64("钻石", gemChange), zap.Int64("广告券", adChange))
			return &game.WalletResponse{Code: 6, Msg: "余额不足"}, nil
		}
		s.logger.Error("钱包更新失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.WalletResponse{Code: 7, Msg: "钱包更新失败"}, nil
	}
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果", zap.String("user_id", userID.String()))
		return &game.WalletResponse{Code: 8, Msg: "未找到钱包更新结果"}, nil
	}

	result := results[0]
	// 5. 作弊检测和处理
	finalWalletData, isCheatDetected, err := s.detectAndHandleCheatPlayer(ctx, userID, result.Updated)
	if err != nil {
		s.logger.Error("作弊检测处理失败", zap.String("user_id", userID.String()), zap.Error(err))
		// 不返回错误，继续正常流程
	}

	previousWallet := &game.Wallet{
		Coin: int32(result.Previous["coin"]),
		Gem:  int32(result.Previous["gem"]),
		Ad:   int32(result.Previous["ad"]),
	}

	// 使用最终的钱包数据（如果检测到作弊，使用清空后的数据）
	updatedWallet := &game.Wallet{
		Coin: int32(finalWalletData["coin"]),
		Gem:  int32(finalWalletData["gem"]),
		Ad:   int32(finalWalletData["ad"]),
	}

	// 如果检测到作弊，记录日志
	if isCheatDetected {
		s.logger.Warn("钱包操作成功（检测到作弊并已处理）",
			zap.String("user_id", userID.String()),
			zap.String("username", username),
			zap.Int64("coin", coinChange),
			zap.Int64("gem", gemChange),
			zap.Int64("ad", adChange),
			zap.String("reason", reason))
	} else {
		s.logger.Info("钱包操作成功",
			zap.String("user_id", userID.String()),
			zap.String("username", username),
			zap.Int64("coin", coinChange),
			zap.Int64("gem", gemChange),
			zap.Int64("ad", adChange),
			zap.String("reason", reason))
	}

	return &game.WalletResponse{
		Code:           0,
		Msg:            "操作成功",
		WalletUpdated:  updatedWallet,
		WalletPrevious: previousWallet,
	}, nil
}
