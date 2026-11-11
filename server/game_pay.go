package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// 支付相关常量。
//
// 客户端 CheckUser = NoCheck：SF SDK 不做 Google 订单校验，回调里把 purchaseToken
// 原样交给客户端，由本服务端代校验、写订单、发货。校验手段是 SF 的谷歌校验接口，
// 只认 packageName + productId + token，不需要 Google service account 凭证。
const (
	// sfCheckOrderURL SF 谷歌订单校验地址（香港主体）。
	// 主体迁移到大陆时改为 https://google.sfgames.io/googlepay/checkOrder2
	sfCheckOrderURL = "https://google.sfeverlasting.com/googlepay/checkOrder2"
	// sfCheckTimeout 校验请求超时，超时按校验失败处理（不发货，等补单）
	sfCheckTimeoutSec = 10
	// expectedPackageName 应用包名，仅用于日志告警
	expectedPackageName = "com.hjzx.sfgame"

	// 业务商品键，与 TplPay.id / 客户端 PRODUCT_KEY 一致
	payProductFirstCharge  = "firstCharge"
	payProductSevenDayCard = "sevenDayAdvCard"
	// cardDays 月卡有效天数，与客户端 SevenDayAdvCardModel.CARD_DAYS 一致
	cardDays = 7
	// cardDailyRewardID 月卡每日奖励，与客户端 SevenDayAdvCardSystem.DAILY_REWARD_ID 一致
	cardDailyRewardID = "SevenDayAdvCard"
	// minClaimIntervalSeconds 两次领取的最小间隔，防时区参数篡改
	minClaimIntervalSeconds = 60

	// 存档位置，与客户端 [Storage] 特性一致
	payCollection            = "Home"
	firstChargeKey           = "FirstCharge"
	sevenDayAdvCardKey       = "SevenDayAdvCard"
	purchaseMarkerCollection = "Purchase"

	// 响应码
	payCodeOK             = 0
	payCodePurchased      = 1001 // 首充已购买过（订单已记录，不重复发货）
	payCodeVerifyFailed   = 1002 // 订单校验失败
	payCodeCannotClaim    = 1003 // 当前不可领取
	payCodeProductUnknown = 1004 // 商品未配置
	payCodeInternal       = 1005 // 服务端内部错误
)

var (
	// sfCheckHTTPClient 校验请求客户端，禁止复用长连接，避免域名切换后串号
	sfCheckHTTPClient = &http.Client{Timeout: sfCheckTimeoutSec * time.Second}
)

// ---------------- 对外入口 ----------------

// PayOrder 支付订单统一入口：支付校验发货 / 首充每日领取 / 月卡每日领取。
func (s *ApiServer) PayOrder(ctx context.Context, in *game.PayOrderRequest) (*game.PayOrderResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	if userID == uuid.Nil {
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "未登录"}, nil
	}

	switch in.GetAction() {
	case game.PayOrderRequest_PAY_CLAIM_FIRST_CHARGE:
		return s.payClaimFirstCharge(ctx, userID, in)
	case game.PayOrderRequest_PAY_CLAIM_CARD:
		return s.payClaimCard(ctx, userID, in)
	default:
		return s.payVerify(ctx, userID, in)
	}
}

// ---------------- 支付校验发货 ----------------

func (s *ApiServer) payVerify(ctx context.Context, userID uuid.UUID, in *game.PayOrderRequest) (*game.PayOrderResponse, error) {
	productID := strings.TrimSpace(in.GetProductId())
	token := strings.TrimSpace(in.GetPurchaseToken())
	if productID == "" || token == "" {
		return &game.PayOrderResponse{Code: payCodeVerifyFailed, Msg: "订单参数缺失"}, nil
	}

	productKey, ok := s.findPayProduct(productID)
	if !ok {
		s.logger.Warn("支付商品未配置",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID))
		return &game.PayOrderResponse{Code: payCodeProductUnknown, Msg: "商品未配置"}, nil
	}

	// 幂等：同一 purchaseToken 只发货一次
	markerKey := purchaseMarkerKey(token)
	delivered, err := s.purchaseMarkerExists(ctx, userID, markerKey)
	if err != nil {
		s.logger.Error("读取支付发货标记失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}
	if delivered {
		s.logger.Info("支付订单已发货，忽略重复请求",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID))
		return s.payStateResponse(ctx, userID, payCodeOK, "已发货")
	}

	// 包名不符只告警：校验接口本身绑定 packageName + productId + token，包名不对会校验失败
	if packageName := strings.TrimSpace(in.GetPackageName()); packageName != "" && packageName != expectedPackageName {
		s.logger.Warn("支付回调包名与预期不一致",
			zap.String("user_id", userID.String()),
			zap.String("package_name", packageName),
			zap.String("expected", expectedPackageName))
	}

	// 1. 校验订单：NoCheck 模式下由业务服务器代校验
	checkResp, err := sfCheckGoogleOrder(in.GetPackageName(), productID, token)
	if err != nil {
		s.logger.Error("支付订单校验请求失败",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID),
			zap.String("order_id", in.GetOrderId()),
			zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeVerifyFailed, Msg: "订单校验失败"}, nil
	}
	if checkResp.Code != 0 {
		s.logger.Warn("支付订单校验未通过",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID),
			zap.String("order_id", in.GetOrderId()),
			zap.Int32("code", checkResp.Code),
			zap.String("msg", checkResp.Msg))
		return &game.PayOrderResponse{Code: payCodeVerifyFailed, Msg: "订单校验失败"}, nil
	}

	// 2. 记录订单（purchase 表，transaction_id = purchaseToken，天然幂等）
	rawResponse, _ := json.Marshal(map[string]interface{}{
		"orderId":    in.GetOrderId(),
		"productId":  productID,
		"productKey": productKey,
		"orderCode":  checkResp.Code,
		"checkMsg":   checkResp.Msg,
	})
	environment := api.StoreEnvironment_PRODUCTION
	if in.GetIsTest() {
		environment = api.StoreEnvironment_SANDBOX
	}
	if _, err = upsertPurchases(ctx, s.db, []*storagePurchase{{
		userID:        userID,
		store:         api.StoreProvider_GOOGLE_PLAY_STORE,
		productId:     productID,
		transactionId: token,
		rawResponse:   string(rawResponse),
		purchaseTime:  time.Now().UTC(),
		environment:   environment,
	}}); err != nil {
		s.logger.Error("记录支付订单失败",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID),
			zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "订单记录失败"}, nil
	}

	// 3. 发货
	switch productKey {
	case payProductFirstCharge:
		return s.deliverFirstCharge(ctx, userID, markerKey, token)
	case payProductSevenDayCard:
		return s.deliverSevenDayAdvCard(ctx, userID, markerKey, token, in.GetTzOffsetMinutes())
	default:
		s.logger.Error("支付商品没有发货实现", zap.String("product_key", productKey))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "商品未接入"}, nil
	}
}

// deliverFirstCharge 首充发货：只解锁每日领取，本身不发奖。
func (s *ApiServer) deliverFirstCharge(ctx context.Context, userID uuid.UUID, markerKey, token string) (*game.PayOrderResponse, error) {
	doc, stateVersion, err := s.readPayDoc(ctx, userID, payCollection, firstChargeKey)
	if err != nil {
		s.logger.Error("读取首充存档失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}

	// 首充只能购买一次：已购买则只记录订单，不重复发货
	if purchased, _ := doc["isPurchased"].(bool); purchased {
		s.logger.Warn("首充重复购买，订单已记录但不重复发货",
			zap.String("user_id", userID.String()),
			zap.String("token_hash", markerKey))
		return s.payStateResponse(ctx, userID, payCodePurchased, "已购买过首充")
	}

	doc["isPurchased"] = true
	doc["claimedCount"] = 0
	doc["lastClaimTime"] = formatPayTime(time.Time{})

	// 状态与发货标记必须在同一事务里落库，避免"发了货没记录"或反过来
	stateJSON, err := s.writePayDocWithMarker(ctx, userID, payCollection, firstChargeKey, doc, stateVersion, markerKey, token)
	if err != nil {
		s.logger.Error("首充发货失败",
			zap.String("user_id", userID.String()),
			zap.String("token_hash", markerKey),
			zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "发货失败"}, nil
	}

	resp, err := s.payStateResponse(ctx, userID, payCodeOK, "购买成功")
	if err != nil {
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}
	resp.FirstChargeJson = stateJSON
	return resp, nil
}

// deliverSevenDayAdvCard 月卡发货：按本地自然日对齐，到期时间叠加 7 天。
func (s *ApiServer) deliverSevenDayAdvCard(ctx context.Context, userID uuid.UUID, markerKey, token string, tzOffsetMinutes int32) (*game.PayOrderResponse, error) {
	doc, stateVersion, err := s.readPayDoc(ctx, userID, payCollection, sevenDayAdvCardKey)
	if err != nil {
		s.logger.Error("读取月卡存档失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}

	now := time.Now().UTC()
	tz := s.resolveTzOffset(doc, tzOffsetMinutes)
	current := parsePayTime(doc["expireTime"])
	doc["expireTime"] = formatPayTime(extendCardExpire(now, current, tz))
	doc["lastOrderToken"] = token
	doc["tzOffset"] = tz

	if _, err = s.writePayDocWithMarker(ctx, userID, payCollection, sevenDayAdvCardKey, doc, stateVersion, markerKey, token); err != nil {
		s.logger.Error("月卡发货失败",
			zap.String("user_id", userID.String()),
			zap.String("token_hash", markerKey),
			zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "发货失败"}, nil
	}

	resp, err := s.payStateResponse(ctx, userID, payCodeOK, "购买成功")
	if err != nil {
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}
	resp.CardJson = docJSON(doc)
	return resp, nil
}

// ---------------- 每日领取 ----------------

func (s *ApiServer) payClaimFirstCharge(ctx context.Context, userID uuid.UUID, in *game.PayOrderRequest) (*game.PayOrderResponse, error) {
	doc, stateVersion, err := s.readPayDoc(ctx, userID, payCollection, firstChargeKey)
	if err != nil {
		s.logger.Error("读取首充存档失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}

	if purchased, _ := doc["isPurchased"].(bool); !purchased {
		return s.payStateResponse(ctx, userID, payCodeCannotClaim, "未购买首充")
	}

	tpl := s.template.GetTplFirstCharge()
	totalDays := tpl.MaxDay()
	claimedCount := int32(intValue(doc["claimedCount"]))
	if totalDays <= 0 || claimedCount >= totalDays {
		return s.payStateResponse(ctx, userID, payCodeCannotClaim, "首充奖励已领完")
	}

	now := time.Now().UTC()
	tz := s.resolveTzOffset(doc, in.GetTzOffsetMinutes())
	if !s.canClaimOnDay(doc, now, tz) {
		return s.payStateResponse(ctx, userID, payCodeCannotClaim, "今日已领取")
	}

	dayData, ok := tpl.FindByDay(claimedCount + 1)
	if !ok {
		s.logger.Error("首充天数配置缺失",
			zap.String("user_id", userID.String()),
			zap.Int32("day", claimedCount+1))
		return s.payStateResponse(ctx, userID, payCodeCannotClaim, "配置缺失")
	}

	reward, err := ParseSingleReward(s.logger, s.template.GetTplReward(), dayData.Reward)
	if err != nil {
		s.logger.Error("首充奖励配置缺失",
			zap.String("user_id", userID.String()),
			zap.String("reward_id", dayData.Reward),
			zap.Error(err))
		return s.payStateResponse(ctx, userID, payCodeInternal, "奖励配置缺失")
	}

	doc["claimedCount"] = claimedCount + 1
	doc["lastClaimTime"] = formatPayTime(now)
	doc["tzOffset"] = tz

	return s.grantAndSave(ctx, userID, payCollection, firstChargeKey, doc, stateVersion, reward, dayData.Reward)
}

func (s *ApiServer) payClaimCard(ctx context.Context, userID uuid.UUID, in *game.PayOrderRequest) (*game.PayOrderResponse, error) {
	doc, stateVersion, err := s.readPayDoc(ctx, userID, payCollection, sevenDayAdvCardKey)
	if err != nil {
		s.logger.Error("读取月卡存档失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}

	now := time.Now().UTC()
	if !parsePayTime(doc["expireTime"]).After(now) {
		return s.payStateResponse(ctx, userID, payCodeCannotClaim, "月卡未生效或已过期")
	}

	tz := s.resolveTzOffset(doc, in.GetTzOffsetMinutes())
	if !s.canClaimOnDay(doc, now, tz) {
		return s.payStateResponse(ctx, userID, payCodeCannotClaim, "今日已领取")
	}

	reward, err := ParseSingleReward(s.logger, s.template.GetTplReward(), cardDailyRewardID)
	if err != nil {
		s.logger.Error("月卡每日奖励配置缺失",
			zap.String("user_id", userID.String()),
			zap.String("reward_id", cardDailyRewardID),
			zap.Error(err))
		return s.payStateResponse(ctx, userID, payCodeInternal, "奖励配置缺失")
	}

	doc["lastClaimTime"] = formatPayTime(now)
	doc["tzOffset"] = tz

	return s.grantAndSave(ctx, userID, payCollection, sevenDayAdvCardKey, doc, stateVersion, reward, cardDailyRewardID)
}

// grantAndSave 领取：钱包入账 + 存档写入在同一事务内完成，避免重复领取或吞奖励。
// 存档写入带版本号乐观锁，并发重复领取只有一个能成功。
func (s *ApiServer) grantAndSave(ctx context.Context, userID uuid.UUID, collection, key string, doc map[string]interface{}, stateVersion string, reward *game.Reward, rewardID string) (*game.PayOrderResponse, error) {
	changeset := make(map[string]int64)
	if w := reward.GetWallet(); w != nil {
		if w.GetCoin() != 0 {
			changeset["coin"] = int64(w.GetCoin())
		}
		if w.GetGem() != 0 {
			changeset["gem"] = int64(w.GetGem())
		}
		if w.GetAd() != 0 {
			changeset["ad"] = int64(w.GetAd())
		}
	}

	var walletUpdates []*walletUpdate
	if len(changeset) > 0 {
		metadata, _ := json.Marshal(map[string]string{"message": "支付奖励:" + rewardID})
		walletUpdates = []*walletUpdate{{
			UserID:    userID,
			Changeset: changeset,
			Metadata:  string(metadata),
		}}
	}

	stateJSON, err := json.Marshal(doc)
	if err != nil {
		s.logger.Error("序列化支付存档失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}

	stateOp := CreateStorageOpWrite(collection, key, string(stateJSON), userID.String())
	stateOp.Object.Version = stateVersion
	storageWrites := StorageOpWrites{stateOp}

	_, walletResults, err := MultiUpdate(ctx, s.logger, s.db, s.metrics,
		nil, storageWrites, nil, s.storageIndex, walletUpdates, true)
	if err != nil {
		s.logger.Error("支付奖励领取失败",
			zap.String("user_id", userID.String()),
			zap.String("key", key),
			zap.String("reward_id", rewardID),
			zap.Error(err))
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "领取失败"}, nil
	}

	resp, err := s.payStateResponse(ctx, userID, payCodeOK, "领取成功")
	if err != nil {
		return &game.PayOrderResponse{Code: payCodeInternal, Msg: "服务端错误"}, nil
	}

	if key == firstChargeKey {
		resp.FirstChargeJson = string(stateJSON)
	} else {
		resp.CardJson = string(stateJSON)
	}
	resp.Items = reward.GetItems()
	resp.RewardIds = rewardID

	if len(walletResults) > 0 {
		updated := walletResults[0].Updated
		resp.Wallet = &game.Wallet{
			Coin: int32(updated["coin"]),
			Gem:  int32(updated["gem"]),
			Ad:   int32(updated["ad"]),
		}
	}

	return resp, nil
}

// ---------------- 规则辅助 ----------------

// canClaimOnDay 自然日内只能领一次，另加最小间隔防重放。
func (s *ApiServer) canClaimOnDay(doc map[string]interface{}, now time.Time, tzOffsetMinutes int32) bool {
	last := parsePayTime(doc["lastClaimTime"])
	if last.IsZero() {
		return true
	}
	if now.Sub(last) < minClaimIntervalSeconds*time.Second {
		return false
	}
	return !sameLocalDay(last, now, tzOffsetMinutes)
}

// resolveTzOffset 首次领取时记录客户端时区并固化，之后忽略客户端传入值，
// 避免通过反复修改时区偏移伪造"新的一天"。
func (s *ApiServer) resolveTzOffset(doc map[string]interface{}, clientOffset int32) int32 {
	if stored := int32(intValue(doc["tzOffset"])); stored != 0 {
		return clampTzOffset(stored)
	}
	return clampTzOffset(clientOffset)
}

func clampTzOffset(minutes int32) int32 {
	if minutes > 14*60 {
		return 14 * 60
	}
	if minutes < -12*60 {
		return -12 * 60
	}
	return minutes
}

// extendCardExpire 与原客户端 ActivateCard 一致：按本地自然日对齐，
// 未过期则在原到期日上叠加，已过期则从今天算起，得到第 N 天本地 24 点。
func extendCardExpire(now, current time.Time, tzOffsetMinutes int32) time.Time {
	offset := time.Duration(clampTzOffset(tzOffsetMinutes)) * time.Minute
	base := now
	if current.After(now) {
		base = current
	}
	baseLocal := base.Add(offset)
	localMidnight := time.Date(baseLocal.Year(), baseLocal.Month(), baseLocal.Day(), 0, 0, 0, 0, time.UTC)
	return localMidnight.AddDate(0, 0, cardDays).Add(-offset)
}

func sameLocalDay(a, b time.Time, tzOffsetMinutes int32) bool {
	offset := time.Duration(clampTzOffset(tzOffsetMinutes)) * time.Minute
	la, lb := a.Add(offset), b.Add(offset)
	return la.Year() == lb.Year() && la.YearDay() == lb.YearDay()
}

// ---------------- 存档读写 ----------------

// readPayDoc 读取存档为 map，保留客户端可能新增的未知字段。
// 同时返回存档版本号，写入时用乐观锁（CAS）防止并发重复发货/重复领取；
// 存档不存在时返回 "*"，表示"只能新建"。
func (s *ApiServer) readPayDoc(ctx context.Context, userID uuid.UUID, collection, key string) (map[string]interface{}, string, error) {
	ids := []*api.ReadStorageObjectId{{
		Collection: collection,
		Key:        key,
		UserId:     userID.String(),
	}}
	result, err := StorageReadObjects(ctx, s.logger, s.db, userID, ids)
	if err != nil {
		return nil, "", err
	}

	doc := make(map[string]interface{})
	if result == nil || len(result.Objects) == 0 || result.Objects[0].Value == "" {
		return doc, "*", nil
	}
	if err = json.Unmarshal([]byte(result.Objects[0].Value), &doc); err != nil {
		// 解析失败不能当空档处理，否则会覆盖玩家数据
		return nil, "", err
	}
	return doc, result.Objects[0].Version, nil
}

// writePayDocWithMarker 状态写入与发货标记写入放在同一事务，保证幂等与发货一致。
// 状态写入带版本号乐观锁，并发重复请求只有一个能成功。
func (s *ApiServer) writePayDocWithMarker(ctx context.Context, userID uuid.UUID, collection, key string, doc map[string]interface{}, stateVersion, markerKey, token string) (string, error) {
	value, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}

	marker, err := json.Marshal(map[string]interface{}{
		"token":       token,
		"deliveredAt": formatPayTime(time.Now()),
	})
	if err != nil {
		return "", err
	}

	stateOp := CreateStorageOpWrite(collection, key, string(value), userID.String())
	stateOp.Object.Version = stateVersion
	markerOp := CreateStorageOpWrite(purchaseMarkerCollection, markerKey, string(marker), userID.String())
	markerOp.Object.Version = "*"

	ops := StorageOpWrites{stateOp, markerOp}
	if _, _, err = MultiUpdate(ctx, s.logger, s.db, s.metrics, nil, ops, nil, s.storageIndex, nil, false); err != nil {
		return "", err
	}
	return string(value), nil
}

func (s *ApiServer) purchaseMarkerExists(ctx context.Context, userID uuid.UUID, markerKey string) (bool, error) {
	ids := []*api.ReadStorageObjectId{{
		Collection: purchaseMarkerCollection,
		Key:        markerKey,
		UserId:     userID.String(),
	}}
	result, err := StorageReadObjects(ctx, s.logger, s.db, userID, ids)
	if err != nil {
		return false, err
	}
	return result != nil && len(result.Objects) > 0, nil
}

// purchaseMarkerKey purchaseToken 可能超过存储 key 长度上限，用摘要做 key，原文写进 value。
func purchaseMarkerKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "pay_" + hex.EncodeToString(sum[:])
}

// payStateResponse 读取两份支付存档，组装通用响应。
func (s *ApiServer) payStateResponse(ctx context.Context, userID uuid.UUID, code int32, msg string) (*game.PayOrderResponse, error) {
	firstCharge, _, err := s.readPayDoc(ctx, userID, payCollection, firstChargeKey)
	if err != nil {
		return nil, err
	}
	card, _, err := s.readPayDoc(ctx, userID, payCollection, sevenDayAdvCardKey)
	if err != nil {
		return nil, err
	}
	return &game.PayOrderResponse{
		Code:            code,
		Msg:             msg,
		FirstChargeJson: docJSON(firstCharge),
		CardJson:        docJSON(card),
	}, nil
}

// findPayProduct 用 Google 商品 ID 反查业务商品键（TplPay.id）。
func (s *ApiServer) findPayProduct(googleProductID string) (string, bool) {
	tpl, ok := s.template.GetTplPay().FindByGoogleID(googleProductID)
	if !ok || tpl.ID == "" {
		return "", false
	}
	return tpl.ID, true
}

// ---------------- 时间与序列化 ----------------

// parsePayTime 解析客户端 Newtonsoft 序列化的时间：可能带 Z、可能带本地偏移、
// 可能是 DateTime.MinValue 的 "0001-01-01T00:00:00"（无时区）。
func parsePayTime(value interface{}) time.Time {
	str, ok := value.(string)
	if !ok || str == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.9999999",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, str); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// formatPayTime 统一输出带 Z 的 RFC3339，Newtonsoft 会按 UTC 解析。
func formatPayTime(t time.Time) string {
	if t.IsZero() {
		return "0001-01-01T00:00:00Z"
	}
	return t.UTC().Format(time.RFC3339)
}

func docJSON(doc map[string]interface{}) string {
	if doc == nil {
		return "{}"
	}
	if value, err := json.Marshal(doc); err == nil {
		return string(value)
	}
	return "{}"
}

func intValue(value interface{}) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

// ---------------- SF 谷歌订单校验 ----------------

type sfCheckRequest struct {
	PackageName string `json:"packageName"`
	ProductID   string `json:"productId"`
	Token       string `json:"token"`
}

type sfCheckResponse struct {
	Code int32  `json:"code"`
	Msg  string `json:"msg"`
}

// sfCheckGoogleOrder 调 SF 谷歌校验接口（NoCheck 模式下的服务端校验）。
// 只有 code == 0 才算订单有效；网络错误或非 0 一律不发货。
func sfCheckGoogleOrder(packageName, productID, token string) (*sfCheckResponse, error) {
	payload, err := json.Marshal(sfCheckRequest{
		PackageName: packageName,
		ProductID:   productID,
		Token:       token,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, sfCheckOrderURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := sfCheckHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sf check http status %d, body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var checkResp sfCheckResponse
	if err = json.Unmarshal(body, &checkResp); err != nil {
		return nil, err
	}
	return &checkResp, nil
}
