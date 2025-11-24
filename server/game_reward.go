package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
const CheatDetectionThreshold = 10000  // 广告券>=1000视为作弊
const FrequentClaimIntervalSeconds = 1 // 频繁领取间隔阈值：3秒
const DailyAdCheatThreshold = 100      // 全天ad获取阈值：100个
const DailyGemLimit = 9999             // 每日钻石领取上限：9999
const DailyCoinLimit = 499999          // 每日金币领取上限：499999
const CheatBanThreshold = 5            // 作弊次数阈值：5次作弊封禁

// DailyClaimHistory 每日领取历史跟踪（持久化到数据库）
type DailyClaimHistory struct {
	LastClaimTime time.Time `json:"last_claim_time"`
	DailyAd       int64     `json:"daily_ad"`        // 今日ad获取总量
	DailyGem      int64     `json:"daily_gem"`       // 今日钻石获取总量
	DailyCoin     int64     `json:"daily_coin"`      // 今日金币获取总量
	LastResetDate string    `json:"last_reset_date"` // 格式：2006-01-02，用于判断日期切换
	CheatCount    int       `json:"cheat_count"`     // 作弊检测次数（累计）
}

// GetCollection 实现 Storable 接口
func (d *DailyClaimHistory) GetCollection() string {
	return "AntiCheat"
}

// GetKey 实现 Storable 接口
func (d *DailyClaimHistory) GetKey() string {
	return "DailyClaimHistory"
}

// Init 实现 Storable 接口
func (d *DailyClaimHistory) Init() {
	d.LastClaimTime = time.Time{}
	d.DailyAd = 0
	d.DailyGem = 0
	d.DailyCoin = 0
	d.LastResetDate = ""
	d.CheatCount = 0
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

	// 判断是否需要记录到 ledger
	// 只在以下情况下才记录：ad 增加 > 3 或 gem 增加 > 100 或 coin 增加 > 10000
	updateLedger := (adChange > 3) || (gemChange > 100) || (coinChange > 10000)

	// 构造更新对象
	updates := []*walletUpdate{{
		UserID:     userID,
		Changeset:  changeset,
		Metadata:   fmt.Sprintf(`{"message": "%s"}`, record),
		ExternalID: externalID,
	}}

	// 调用更新钱包的函数
	results, err := UpdateWallets(ctx, s.logger, s.db, updates, updateLedger)
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

// 检测每日领取限制（使用数据库持久化）
func (s *ApiServer) detectDailyClaimLimit(ctx context.Context, userID uuid.UUID, coinChange, gemChange, adChange int64) (isCheat bool, reason string, cheatCount int) {
	// 只检测增加的情况
	if coinChange <= 0 && gemChange <= 0 && adChange <= 0 {
		return false, "", 0
	}

	// 先加载历史记录获取当前作弊次数
	history := &DailyClaimHistory{}
	err := LoadData(ctx, s.logger, s.db, userID, history)
	if err != nil {
		s.logger.Error("加载每日领取历史失败", zap.Error(err), zap.String("user_id", userID.String()))
	}

	// 0. 单次ad检测：单次领取超过5个
	if adChange > 5 {
		isCheat = true
		reason = "单次领取ad过多"
		// 增加作弊计数
		history.CheatCount++
		s.logger.Warn("检测到单次领取ad过多",
			zap.String("user_id", userID.String()),
			zap.Int64("ad_change", adChange),
			zap.Int("cheat_count", history.CheatCount))
		// 保存更新后的作弊计数
		SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, history)
		return isCheat, reason, history.CheatCount
	}

	now := time.Now()
	today := now.Format("2006-01-02")

	// 如果之前没加载，这里重新加载（正常情况已在前面加载）
	if history.LastResetDate == "" && err == nil {
		err = LoadData(ctx, s.logger, s.db, userID, history)
	}

	if err != nil {
		s.logger.Error("加载每日领取历史失败", zap.Error(err), zap.String("user_id", userID.String()))
		// 加载失败时，初始化新记录
		history.LastClaimTime = now
		history.DailyAd = maxInt64(0, adChange)
		history.DailyGem = maxInt64(0, gemChange)
		history.DailyCoin = maxInt64(0, coinChange)
		history.LastResetDate = today
		history.CheatCount = 0
		SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, history)
		return false, "", 0
	}

	// 首次记录（空数据）
	if history.LastResetDate == "" {
		history.LastClaimTime = now
		history.DailyAd = maxInt64(0, adChange)
		history.DailyGem = maxInt64(0, gemChange)
		history.DailyCoin = maxInt64(0, coinChange)
		history.LastResetDate = today
		SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, history)
		return false, "", 0
	}

	// 检查是否需要重置每日计数
	if history.LastResetDate != today {
		history.DailyAd = maxInt64(0, adChange)
		history.DailyGem = maxInt64(0, gemChange)
		history.DailyCoin = maxInt64(0, coinChange)
		history.LastResetDate = today
		history.LastClaimTime = now
		SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, history)
		return false, "", 0
	}

	// 作弊判定
	isCheat = false
	reason = ""

	// 计算与上次领取的时间间隔
	intervalSeconds := now.Sub(history.LastClaimTime).Seconds()
	s.logger.Info("上次领取间隔", zap.String("user_id", userID.String()), zap.Float64("interval_seconds", intervalSeconds))

	// 更新今日总量
	newDailyAd := history.DailyAd + maxInt64(0, adChange)
	newDailyGem := history.DailyGem + maxInt64(0, gemChange)
	newDailyCoin := history.DailyCoin + maxInt64(0, coinChange)

	// 1. 频繁领取ad检测：间隔小于3秒 且 今日总量超过100
	if adChange > 0 && int32(intervalSeconds) < FrequentClaimIntervalSeconds && newDailyAd > DailyAdCheatThreshold {
		isCheat = true
		reason = "频繁领取ad"
		history.CheatCount++
		s.logger.Warn("检测到频繁领取ad作弊",
			zap.String("user_id", userID.String()),
			zap.Float64("interval_seconds", intervalSeconds),
			zap.Int64("daily_ad", newDailyAd),
			zap.Int64("current_claim", adChange),
			zap.Int("cheat_count", history.CheatCount))
	}

	// 2. 每日钻石上限检测
	if gemChange > 0 && newDailyGem > DailyGemLimit {
		isCheat = true
		reason = fmt.Sprintf("每日钻石超限(今日已获得%d，上限%d)", newDailyGem, DailyGemLimit)
		history.CheatCount++
		s.logger.Warn("检测到每日钻石领取超限",
			zap.String("user_id", userID.String()),
			zap.Int64("daily_gem", newDailyGem),
			zap.Int64("limit", DailyGemLimit),
			zap.Int64("current_claim", gemChange),
			zap.Int("cheat_count", history.CheatCount))
	}

	// 3. 每日金币上限检测
	if coinChange > 0 && newDailyCoin > DailyCoinLimit {
		isCheat = true
		reason = fmt.Sprintf("每日金币超限(今日已获得%d，上限%d)", newDailyCoin, DailyCoinLimit)
		history.CheatCount++
		s.logger.Warn("检测到每日金币领取超限",
			zap.String("user_id", userID.String()),
			zap.Int64("daily_coin", newDailyCoin),
			zap.Int64("limit", DailyCoinLimit),
			zap.Int64("current_claim", coinChange),
			zap.Int("cheat_count", history.CheatCount))
	}

	// 更新记录
	history.LastClaimTime = now
	history.DailyAd = newDailyAd
	history.DailyGem = newDailyGem
	history.DailyCoin = newDailyCoin

	// 保存更新后的历史记录
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, history)
	if err != nil {
		s.logger.Error("保存每日领取历史失败", zap.Error(err), zap.String("user_id", userID.String()))
	}

	return isCheat, reason, history.CheatCount
}

// maxInt64 返回两个int64中的较大值
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// 发送作弊警告邮件
func (s *ApiServer) sendCheatWarningEmail(ctx context.Context, userID uuid.UUID) error {
	// 邮件内容
	emailContent := map[string]interface{}{
		"description": "经技术排查，您的账号涉及篡改数据，现已将您账号的货币重置。请阁下遵守游戏规则，勿通过任何渠道篡改游戏数据或使用外挂！如再次发现将会封禁账号！",
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
		return &game.WalletResponse{Code: 2, Msg: "钱包操作参数非法"}, nil
	}

	if in.GetCoin() == 0 && in.GetGem() == 0 && in.GetAd() == 0 {
		return &game.WalletResponse{Code: 2, Msg: "钱包操作参数非法"}, nil
	}

	// 可选：最大值校验，防止溢出 金币钻石 10w 广告券 10
	const maxAmount = 1000000
	if in.GetCoin() > maxAmount || in.GetGem() > maxAmount {
		s.logger.Warn("钱包操作参数过大", zap.String("user_id", userID.String()), zap.Int32("coin", in.GetCoin()), zap.Int32("gem", in.GetGem()), zap.Int32("ad", in.GetAd()))
		return &game.WalletResponse{Code: 3, Msg: "数量过大"}, nil
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

	//3. 签名校验
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

	// 3.5. 每日领取限制检测（仅检测GAIN操作）
	if op == game.OperateWalletRequest_GAIN && (coinChange > 0 || gemChange > 0 || adChange > 0) {
		isCheat, cheatReason, cheatCount := s.detectDailyClaimLimit(ctx, userID, coinChange, gemChange, adChange)
		if isCheat {
			s.logger.Warn("检测到超出每日领取限制",
				zap.String("user_id", userID.String()),
				zap.String("username", username),
				zap.String("reason", cheatReason),
				zap.Int("cheat_count", cheatCount),
				zap.Int64("coin", coinChange),
				zap.Int64("gem", gemChange),
				zap.Int64("ad", adChange))

			// 判断是否封禁（连续5次作弊）
			if EnableAutoBan && cheatCount >= CheatBanThreshold {
				BanUsers(ctx, s.logger, s.db, s.config, s.sessionCache, s.sessionRegistry, s.tracker, []uuid.UUID{userID})
				s.logger.Warn("用户因连续作弊被封禁",
					zap.String("user_id", userID.String()),
					zap.String("username", username),
					zap.Int("cheat_count", cheatCount),
					zap.String("last_cheat_reason", cheatReason))
				return &game.WalletResponse{Code: 11, Msg: "账号已被封禁"}, nil
			}

			// 所有作弊行为统一处理：清空钱包 + 发送警告邮件
			// 1. 清空钱包
			account, err := GetAccount(ctx, s.logger, s.db, s.statusRegistry, userID)
			if err != nil {
				s.logger.Error("获取账户信息失败", zap.String("user_id", userID.String()), zap.Error(err))
			} else if account != nil && account.Wallet != "" {
				var currentWallet map[string]int64
				if err := json.Unmarshal([]byte(account.Wallet), &currentWallet); err == nil {
					// 清空钱包
					clearChangeset := map[string]int64{
						"coin": -currentWallet["coin"],
						"gem":  -currentWallet["gem"],
						"ad":   -currentWallet["ad"],
					}

					clearUpdates := []*walletUpdate{{
						UserID:    userID,
						Changeset: clearChangeset,
						Metadata:  fmt.Sprintf(`{"message": "作弊检测：清空钱包 - %s"}`, cheatReason),
					}}

					_, clearErr := UpdateWallets(ctx, s.logger, s.db, clearUpdates, true)
					if clearErr != nil {
						s.logger.Error("清空作弊玩家钱包失败", zap.String("user_id", userID.String()), zap.Error(clearErr))
					} else {
						s.logger.Info("成功清空作弊玩家钱包",
							zap.String("user_id", userID.String()),
							zap.String("reason", cheatReason),
							zap.Int("cheat_count", cheatCount))
					}
				}
			}

			// 2. 发送警告邮件
			emailErr := s.sendCheatWarningEmail(ctx, userID)
			if emailErr != nil {
				s.logger.Error("发送作弊警告邮件失败",
					zap.String("user_id", userID.String()),
					zap.Error(emailErr))
			}

			// 返回警告信息
			remainingWarnings := CheatBanThreshold - cheatCount
			return &game.WalletResponse{
				Code: 9,
				Msg:  fmt.Sprintf("检测到作弊行为，钱包已清空（第%d次警告，剩余%d次）", cheatCount, remainingWarnings),
			}, nil
		}
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
		s.logger.Error("钱包更新失败", zap.Error(err), zap.String("user_id", userID.String()), zap.String("record", record))
		return &game.WalletResponse{Code: 7, Msg: "钱包更新失败"}, nil
	}
	if len(results) == 0 {
		s.logger.Error("未找到钱包更新结果",
			zap.String("user_id", userID.String()),
			zap.String("username", username),
			zap.String("record", record),
			zap.String("wallet_id", in.GetWalletId()),
			zap.Int64("coin_change", coinChange),
			zap.Int64("gem_change", gemChange),
			zap.Int64("ad_change", adChange),
			zap.String("reason", reason),
			zap.String("operation", op.String()),
			zap.String("signature", in.GetSignature()))
		return &game.WalletResponse{Code: 8, Msg: "未找到钱包更新结果"}, nil
	}

	result := results[0]
	//5. 作弊检测和处理
	finalWalletData, isCheatDetected, err := s.detectAndHandleCheatPlayer(ctx, userID, result.Updated)
	if err != nil {
		s.logger.Error("作弊检测处理失败", zap.String("user_id", userID.String()), zap.Error(err))
		// 不返回错误，继续正常流程
		finalWalletData = result.Updated
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

	//如果检测到作弊，记录日志
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
