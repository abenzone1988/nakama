package server

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// 应用ID和密钥的对应关系 - 支持多个平台
	// 这些应该从配置文件中读取
	SiteKeyMap = map[string]string{
		"xjsmdyapp_android": "214d987374f232af33608e184a666bc9",
		"xjsmdyapp_ios":     "91b31f759862e63834878664a4436b3b",
	}
)

// PurchaseNotifyRequest 支付通知请求结构
type PurchaseNotifyRequest struct {
	Site       string `json:"site"`        // 应用 ID
	OrderID    string `json:"order_id"`    // 商户订单号
	UID        string `json:"uid"`         // 用户ID
	SID        string `json:"sid"`         //服务器 ID
	CPOrderID  string `json:"cp_order_id"` // CP订单号
	RoleID     string `json:"roleid"`      // 角色ID
	RoleName   string `json:"rolename"`    // 角色名称
	OrderMoney string `json:"order_money"` // 实际总价
	ProductID  string `json:"productid"`   // 商品ID
	PayType    int    `json:"pay_type"`    // 支付方式 1：苹果内购, 2：支付宝, 3和11：微信h5支付, 5：微信公众号支付, 13：微信虚拟内购
	Ext        string `json:"ext"`         // 透传字段，由 CP 定义的字段
	Time       string `json:"time"`        // 时间戳 10位
	Sign       string `json:"sign"`        // 签名
}

// PurchaseNotifyResponse 支付通知响应
type PurchaseNotifyResponse struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

// HandlePurchaseNotify 处理购买发货通知请求
func (s *ApiServer) HandlePurchaseNotify(w http.ResponseWriter, r *http.Request) {
	// 只接受POST请求
	if r.Method != http.MethodPost {
		s.logger.Warn("购买通知请求方法错误", zap.String("method", r.Method))
		s.writePurchaseError(w, "METHOD_NOT_ALLOWED", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求参数
	var req PurchaseNotifyRequest

	// 先尝试解析表单数据
	if err := r.ParseForm(); err == nil && len(r.Form) > 0 {
		// 从表单获取参数
		payType, _ := strconv.Atoi(r.FormValue("pay_type"))
		req = PurchaseNotifyRequest{
			Site:       r.FormValue("site"),
			OrderID:    r.FormValue("order_id"),
			UID:        r.FormValue("uid"),
			SID:        r.FormValue("sid"),
			CPOrderID:  r.FormValue("cp_order_id"),
			RoleID:     r.FormValue("roleid"),
			RoleName:   r.FormValue("rolename"),
			OrderMoney: r.FormValue("order_money"),
			ProductID:  r.FormValue("productid"),
			PayType:    payType,
			Ext:        r.FormValue("ext"),
			Time:       r.FormValue("time"),
			Sign:       r.FormValue("sign"),
		}
	} else {
		// 如果表单解析失败，尝试JSON格式
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("读取购买通知请求失败", zap.Error(err))
			s.writePurchaseError(w, "READ_ERROR", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if err := json.Unmarshal(body, &req); err != nil {
			s.logger.Error("解析购买通知请求失败", zap.Error(err), zap.String("body", string(body)))
			s.writePurchaseError(w, "PARSE_ERROR", http.StatusBadRequest)
			return
		}
	}

	s.logger.Info("收到购买通知请求",
		zap.String("site", req.Site),
		zap.String("order_id", req.OrderID),
		zap.String("uid", req.UID),
		zap.String("order_money", req.OrderMoney),
		zap.String("cp_order_id", req.CPOrderID))

	// 验证应用ID
	if !s.verifySiteID(req.Site) {
		s.logger.Warn("应用ID验证失败", zap.String("site", req.Site))
		s.writePurchaseErrorText(w, "site-error")
		return
	}

	// 验证订单金额（可根据实际业务需求完善）
	if !s.verifyOrderAmount(req.OrderMoney, req.ProductID) {
		s.logger.Warn("订单金额验证失败",
			zap.String("order_money", req.OrderMoney),
			zap.String("product_id", req.ProductID))
		s.writePurchaseErrorText(w, "amount-error")
		return
	}

	// 验证签名
	if !s.verifyPurchaseSignature(req) {
		s.logger.Warn("签名验证失败",
			zap.String("order_id", req.OrderID),
			zap.String("sign", req.Sign))
		s.writePurchaseErrorText(w, "sign error")
		return
	}

	// 处理发货逻辑
	if err := s.processPurchaseDelivery(r.Context(), &req); err != nil {
		s.logger.Error("处理发货失败",
			zap.Error(err),
			zap.String("order_id", req.OrderID))
		s.writePurchaseErrorText(w, "delivery-error")
		return
	}

	// 返回成功
	s.logger.Info("购买通知处理成功", zap.String("order_id", req.OrderID))
	s.writePurchaseSuccessText(w)
}

// verifySiteID 验证应用ID
func (s *ApiServer) verifySiteID(site string) bool {
	// TODO: 从配置文件中读取site配置
	// if s.config.GetSocial().Tiktok != nil {
	//     siteKeyMap = s.config.GetSocial().Tiktok.SiteKeyMap
	// }

	// 检查site是否在map中
	if _, exists := SiteKeyMap[site]; exists {
		return true
	}

	// 获取所有允许的site列表用于日志
	allowedSites := make([]string, 0, len(SiteKeyMap))
	for site := range SiteKeyMap {
		allowedSites = append(allowedSites, site)
	}

	s.logger.Warn("应用ID不在允许列表中",
		zap.String("site", site),
		zap.Strings("allowed_sites", allowedSites))

	return false
}

// verifyOrderAmount 验证订单金额
func (s *ApiServer) verifyOrderAmount(orderMoney, productID string) bool {
	// TODO: 根据商品ID验证金额是否匹配
	// 这里需要从数据库或配置中查询商品价格并验证

	// 基本验证：确保金额不为空且为有效数字
	if orderMoney == "" {
		return false
	}

	// 验证金额格式
	if _, err := strconv.ParseFloat(orderMoney, 64); err != nil {
		return false
	}

	s.logger.Info("verifyOrderAmount", zap.String("order_money", orderMoney), zap.String("product_id", productID))

	return true
}

// verifyPurchaseSignature 验证购买通知签名
func (s *ApiServer) verifyPurchaseSignature(req PurchaseNotifyRequest) bool {
	// TODO: 从配置文件中读取key配置
	// if s.config.GetSocial().Tiktok != nil {
	//     siteKeyMap = s.config.GetSocial().Tiktok.SiteKeyMap
	// }

	// 根据site获取对应的密钥
	key, exists := SiteKeyMap[req.Site]
	if !exists {
		s.logger.Warn("找不到对应的应用密钥",
			zap.String("site", req.Site))
		return false
	}

	// 按照API规范拼接字符串: site + time + key + uid + order_money + cp_order_id
	signStr := fmt.Sprintf("%s%s%s%s%s%s",
		req.Site,
		req.Time,
		key,
		req.UID,
		req.OrderMoney,
		req.CPOrderID)

	// 计算MD5
	hash := md5.New()
	hash.Write([]byte(signStr))
	calculatedSign := fmt.Sprintf("%x", hash.Sum(nil))

	s.logger.Debug("验证签名",
		zap.String("site", req.Site),
		zap.String("sign_str", signStr),
		zap.String("calculated_sign", calculatedSign),
		zap.String("received_sign", req.Sign))

	return calculatedSign == req.Sign
}

// processPurchaseDelivery 处理购买发货
func (s *ApiServer) processPurchaseDelivery(ctx context.Context, req *PurchaseNotifyRequest) error {
	s.logger.Info("开始处理发货",
		zap.String("uid", req.UID),
		zap.String("product_id", req.ProductID),
		zap.String("order_id", req.OrderID),
		zap.String("cp_order_id", req.CPOrderID))

	// 1. 根据UID查找用户
	userID, err := FindUserByCustomID(ctx, s.logger, s.db, req.UID)
	if err != nil {
		userID = ctx.Value(ctxUserIDKey{}).(uuid.UUID)
		s.logger.Warn("用户不存在，使用上下文中的用户ID")
	}

	// 2. 检查订单是否已处理（防止重复发货）
	existingPurchase, err := GetPurchaseByTransactionId(ctx, s.logger, s.db, req.CPOrderID)
	if err != nil {
		return fmt.Errorf("查询订单失败: %w", err)
	}

	if existingPurchase != nil {
		s.logger.Warn("订单已存在，可能是重复通知",
			zap.String("transaction_id", req.CPOrderID),
			zap.String("user_id", existingPurchase.UserId),
			zap.Time("create_time", existingPurchase.CreateTime.AsTime()))

		// 订单已存在，返回成功（幂等性处理）
		return nil
	}

	// 4. 记录购买到数据库（复用现有的 upsertPurchases 函数）
	if err := s.recordThirdPartyPurchase(ctx, userID, req); err != nil {
		return fmt.Errorf("记录购买失败: %w", err)
	}

	// 5. 执行实际的发货逻辑
	if err := s.deliverProductToUser(ctx, userID, req.ProductID, req.OrderMoney, req); err != nil {
		// 发货失败，但购买记录已保存，可以通过后台补发
		s.logger.Error("发货失败，需要人工处理",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("transaction_id", req.CPOrderID))
		return fmt.Errorf("发货失败: %w", err)
	}

	s.logger.Info("发货成功",
		zap.String("user_id", userID.String()),
		zap.String("product_id", req.ProductID),
		zap.String("transaction_id", req.CPOrderID))

	return nil
}

// writePurchaseError 写入JSON格式错误响应
func (s *ApiServer) writePurchaseError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(PurchaseNotifyResponse{
		Status:  "error",
		Message: message,
	})
}

// writePurchaseErrorText 写入纯文本错误响应
func (s *ApiServer) writePurchaseErrorText(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(message))
}

// writePurchaseSuccessText 写入纯文本成功响应
func (s *ApiServer) writePurchaseSuccessText(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("SUCCESS"))
}

// recordThirdPartyPurchase 记录第三方购买到数据库（复用现有购买系统）
func (s *ApiServer) recordThirdPartyPurchase(ctx context.Context, userID uuid.UUID, req *PurchaseNotifyRequest) error {
	// 构建原始响应JSON（用于存储完整的通知数据）
	rawResponse, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("序列化原始响应失败: %w", err)
	}

	// 解析时间戳
	timestamp, err := strconv.ParseInt(req.Time, 10, 64)
	if err != nil {
		timestamp = time.Now().Unix()
	}
	purchaseTime := time.Unix(timestamp, 0)

	// 构建存储购买对象
	sPurchase := &storagePurchase{
		userID:        userID,
		store:         api.StoreProvider_PLAY800_SDK,
		productId:     req.ProductID,
		transactionId: req.CPOrderID, // 使用CP订单号作为唯一交易ID
		rawResponse:   string(rawResponse),
		purchaseTime:  purchaseTime,
		environment:   api.StoreEnvironment_PRODUCTION, // 根据实际情况调整
	}

	// 复用现有的 upsertPurchases 函数
	_, err = upsertPurchases(ctx, s.db, []*storagePurchase{sPurchase})
	if err != nil {
		return fmt.Errorf("保存购买记录失败: %w", err)
	}

	s.logger.Info("购买记录已保存",
		zap.String("user_id", userID.String()),
		zap.String("transaction_id", req.CPOrderID),
		zap.String("product_id", req.ProductID),
		zap.Time("purchase_time", purchaseTime))

	return nil
}

// 验证商品是否存在和价格是否匹配
func (s *ApiServer) validateProduct(productID string, price string) (*template.TplPay, error) {
	tplPays := s.template.GetTplPay().FindByFilter(func(tp template.TplPay) bool {
		return tp.ID == productID
	})
	if tplPays == nil {
		return nil, fmt.Errorf("商品不存在: %s", productID)
	}
	tplPay := tplPays.Get(0)
	expectedPrice, err1 := strconv.ParseFloat(tplPay.Money, 64)
	actualPrice, err2 := strconv.ParseFloat(price, 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("商品价格解析失败: %s", productID)
	}
	// 允许浮点误差, 例如1分（0.01）以内认为相同
	if math.Abs(expectedPrice-actualPrice) > 0.01 {
		return nil, fmt.Errorf("商品价格不匹配: %s", productID)
	}
	return &tplPay, nil
}

// deliverFirstChargeProduct 处理首冲商品发货
func (s *ApiServer) deliverFirstChargeProduct(ctx context.Context, userID uuid.UUID, productID string, price string) error {
	if err := RecordFirstCharge(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID); err != nil {
		s.logger.Error("记录首冲失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID))
		// 首冲记录失败不影响发货流程，继续处理
	} else {
		s.logger.Info("首冲记录成功",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID),
			zap.String("price", price))
	}
	return nil
}

// deliverVipProduct 处理VIP购买发货
func (s *ApiServer) deliverVipProduct(ctx context.Context, userID uuid.UUID, productID string) error {
	username := userID.String()
	users, err := GetUsers(ctx, s.logger, s.db, s.statusRegistry, []string{userID.String()}, nil, nil)
	if err == nil && users != nil && len(users.Users) > 0 && users.Users[0] != nil && users.Users[0].Username != "" {
		username = users.Users[0].Username
	}

	expiryTime := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC) // 视为永久
	if _, err := VipAccountAdd(ctx, s.logger, s.db, userID, username, expiryTime); err != nil {
		if err == ErrVipAccountAlreadyExists {
			s.logger.Info("VIP账户已存在，刷新有效期",
				zap.String("user_id", userID.String()),
				zap.String("product_id", productID))
		} else {
			s.logger.Error("记录VIP购买失败",
				zap.Error(err),
				zap.String("user_id", userID.String()),
				zap.String("product_id", productID))
		}
	} else {
		s.logger.Info("VIP购买记录成功",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID),
			zap.Time("vip_expiry", expiryTime))
	}

	// 重置VIP奖励领取状态
	vipRewardData := &VipRewardData{}
	vipRewardData.Init()
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, vipRewardData); err != nil {
		s.logger.Error("重置VIP奖励状态失败",
			zap.Error(err),
			zap.String("user_id", userID.String()))
	}
	return nil
}

// deliverSevenDayProduct 处理七日购买发货
func (s *ApiServer) deliverSevenDayProduct(ctx context.Context, userID uuid.UUID, productID string, price string) error {
	if err := s.RecordSevenDayPurchase(ctx, userID); err != nil {
		s.logger.Error("记录七日购买失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID))
		// 记录失败不影响发货流程，继续处理
	} else {
		s.logger.Info("七日购买记录成功",
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID),
			zap.String("price", price))
	}
	return nil
}

// deliverMonthlyCardProduct 处理月卡购买发货
func (s *ApiServer) deliverMonthlyCardProduct(ctx context.Context, userID uuid.UUID, productID string) error {
	username := userID.String()
	users, err := GetUsers(ctx, s.logger, s.db, s.statusRegistry, []string{userID.String()}, nil, nil)
	if err == nil && users != nil && len(users.Users) > 0 && users.Users[0] != nil && users.Users[0].Username != "" {
		username = users.Users[0].Username
	}

	extendDuration := 30 * 24 * time.Hour
	if _, err := VipAccountExtend(ctx, s.logger, s.db, userID, username, extendDuration); err != nil {
		s.logger.Error("扩展VIP时间失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("product_id", productID))
		return fmt.Errorf("扩展VIP时间失败: %w", err)
	}

	monthlyCardData := &MonthlyCardData{}
	if err := LoadUserData(ctx, s.logger, s.db, monthlyCardData); err != nil {
		s.logger.Error("加载月卡数据失败", zap.Error(err))
		monthlyCardData.Init()
	}

	monthlyCardData.PurchaseTime = time.Now().UTC().Format(time.RFC3339)
	monthlyCardData.PurchaseRewardClaimed = false
	monthlyCardData.TotalPurchases++

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, monthlyCardData); err != nil {
		return fmt.Errorf("保存月卡数据失败: %w", err)
	}

	s.logger.Info("月卡购买记录成功",
		zap.String("user_id", userID.String()),
		zap.String("product_id", productID),
		zap.String("purchase_time", monthlyCardData.PurchaseTime),
		zap.Int32("total_purchases", monthlyCardData.TotalPurchases))
	return nil
}

// deliverShopGemProduct 处理钻石商店商品购买发货
func (s *ApiServer) deliverShopGemProduct(ctx context.Context, userID uuid.UUID, productID string, price string) error {
	// 通过 productID（TplPay.ID）找到对应的商店商品
	gemItems := s.template.GetTplShopGemItem().FindByFilter(func(item template.TplShopGemItem) bool {
		return item.PayID == productID
	})

	if gemItems == nil || gemItems.Len() == 0 {
		s.logger.Warn("未找到对应的钻石商店商品",
			zap.String("product_id", productID),
			zap.String("user_id", userID.String()))
		return fmt.Errorf("未找到对应的钻石商店商品: %s", productID)
	}

	// 获取第一个匹配的商品（通常应该只有一个）
	tplGemItem := gemItems.Get(0)
	shopItemID := tplGemItem.ID

	// 加载钻石商店数据
	gemShopData := &GemShopData{}
	if err := LoadUserData(ctx, s.logger, s.db, gemShopData); err != nil {
		s.logger.Error("加载钻石商店数据失败",
			zap.Error(err),
			zap.String("user_id", userID.String()))
		gemShopData.Init()
	}

	// 确保 BoughtCounts 不为 nil
	if gemShopData.BoughtCounts == nil {
		gemShopData.BoughtCounts = make(map[string]int32)
	}

	// 增加购买次数
	gemShopData.BoughtCounts[shopItemID] = gemShopData.BoughtCounts[shopItemID] + 1

	// 保存商店数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, gemShopData); err != nil {
		s.logger.Error("保存钻石商店数据失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("shop_item_id", shopItemID))
		return fmt.Errorf("保存钻石商店数据失败: %w", err)
	}

	s.logger.Info("钻石商店商品购买记录成功",
		zap.String("user_id", userID.String()),
		zap.String("product_id", productID),
		zap.String("shop_item_id", shopItemID),
		zap.String("price", price),
		zap.Int32("bought_count", gemShopData.BoughtCounts[shopItemID]))

	return nil
}

// deliverShopChapterProduct 处理章节商店商品购买发货
func (s *ApiServer) deliverShopChapterProduct(ctx context.Context, userID uuid.UUID, productID string, price string) error {
	// 通过 productID（TplPay.ID）找到对应的商店商品
	chapterItems := s.template.GetTplShopChapterItem().FindByFilter(func(item template.TplShopChapterItem) bool {
		return item.PayID == productID
	})

	if chapterItems == nil || chapterItems.Len() == 0 {
		s.logger.Warn("未找到对应的章节商店商品",
			zap.String("product_id", productID),
			zap.String("user_id", userID.String()))
		return fmt.Errorf("未找到对应的章节商店商品: %s", productID)
	}

	// 获取第一个匹配的商品（通常应该只有一个）
	tplChapterItem := chapterItems.Get(0)
	shopItemID := tplChapterItem.ID

	// 加载章节商店数据
	chapterShopData := &ChapterShopData{}
	if err := LoadUserData(ctx, s.logger, s.db, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败",
			zap.Error(err),
			zap.String("user_id", userID.String()))
		chapterShopData.Init()
	}

	// 确保 BoughtCounts 不为 nil
	if chapterShopData.BoughtCounts == nil {
		chapterShopData.BoughtCounts = make(map[string]int32)
	}

	// 增加购买次数
	chapterShopData.BoughtCounts[shopItemID] = chapterShopData.BoughtCounts[shopItemID] + 1

	// 保存商店数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, chapterShopData); err != nil {
		s.logger.Error("保存章节商店数据失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("shop_item_id", shopItemID))
		return fmt.Errorf("保存章节商店数据失败: %w", err)
	}

	s.logger.Info("章节商店商品购买记录成功",
		zap.String("user_id", userID.String()),
		zap.String("product_id", productID),
		zap.String("shop_item_id", shopItemID),
		zap.String("price", price),
		zap.Int32("bought_count", chapterShopData.BoughtCounts[shopItemID]))

	return nil
}

// sendPurchaseNotification 发送购买成功通知给用户
func (s *ApiServer) sendPurchaseNotification(ctx context.Context, userID uuid.UUID, productName string, price string, orderID string) error {
	content := map[string]interface{}{
		"description": fmt.Sprintf(`{"商品":"%s", "价格":"%s","订单id":"%s"}`, productName, price, orderID),
	}

	contentBytes, err := json.Marshal(content)
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return err
	}

	// 创建通知
	notification := &api.Notification{
		Id:         uuid.Must(uuid.NewV4()).String(),
		Subject:    "购买成功",
		Content:    string(contentBytes),
		Code:       NotificationSystemNotice,
		SenderId:   uuid.Nil.String(),
		CreateTime: timestamppb.Now(),
		Persistent: true,
	}

	// 发送通知
	notifications := make(map[uuid.UUID][]*api.Notification)
	notifications[userID] = []*api.Notification{notification}

	err = NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications)
	if err != nil {
		s.logger.Error("发送奖励通知失败",
			zap.String("user_id", userID.String()),
			zap.Any("notification", notification),
			zap.Error(err))
		return err
	}

	return nil
}

// deliverProductToUser 向用户发放商品
func (s *ApiServer) deliverProductToUser(ctx context.Context, userID uuid.UUID, productID string, price string, req *PurchaseNotifyRequest) error {
	// 检测是否为特殊商品
	isFirstCharge := productID == FirstChargeProductID
	isVipPurchase := productID == VipProductID
	isSevenDayPurchase := productID == SevenDayProductID
	isMonthlyCardPurchase := productID == MonthlyCardID
	isShopGem := strings.HasPrefix(productID, "shopGem")
	isShopChapter := strings.HasPrefix(productID, "chapter")

	if (isFirstCharge || isSevenDayPurchase || isVipPurchase || isMonthlyCardPurchase || isShopGem || isShopChapter) && ctx.Value(ctxUserIDKey{}) == nil {
		ctx = context.WithValue(ctx, ctxUserIDKey{}, userID)
	}

	// 验证商品
	tplPay, err := s.validateProduct(productID, price)
	if err != nil {
		return err
	}

	// 根据商品类型执行不同的发货逻辑
	if isFirstCharge {
		if err := s.deliverFirstChargeProduct(ctx, userID, productID, price); err != nil {
			// 首冲记录失败不影响发货流程，继续处理
		}
	}

	if isVipPurchase {
		if err := s.deliverVipProduct(ctx, userID, productID); err != nil {
			// VIP购买记录失败不影响发货流程，继续处理
		}
	}

	if isSevenDayPurchase {
		if err := s.deliverSevenDayProduct(ctx, userID, productID, price); err != nil {
			// 七日购买记录失败不影响发货流程，继续处理
		}
	}

	if isMonthlyCardPurchase {
		if err := s.deliverMonthlyCardProduct(ctx, userID, productID); err != nil {
			s.logger.Error("月卡购买记录失败", zap.Error(err), zap.String("user_id", userID.String()), zap.String("product_id", productID))
		}
	}

	if isShopGem {
		if err := s.deliverShopGemProduct(ctx, userID, productID, price); err != nil {
			// 钻石购买记录失败不影响发货流程，继续处理
		}
	}

	if isShopChapter {
		if err := s.deliverShopChapterProduct(ctx, userID, productID, price); err != nil {
			// 章节购买记录失败不影响发货流程，继续处理
		}
	}

	// 发送通知给用户
	if err := s.sendPurchaseNotification(ctx, userID, tplPay.ProductName, price, req.CPOrderID); err != nil {
		// 通知发送失败不影响发货流程，记录日志即可
	}

	s.logger.Info("商品发放完成",
		zap.String("user_id", userID.String()),
		zap.String("product_id", productID),
		zap.String("price", price))

	return nil
}
