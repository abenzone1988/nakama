package server

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"go.uber.org/zap"
)

const (
	// 应用ID和密钥 - 这些应该从配置文件中读取
	DefaultSiteID  = "cjyx_cn"
	DefaultSiteKey = "11b18290a34e03da78900824fa59b140"
)

// PurchaseNotifyRequest 支付通知请求结构
type PurchaseNotifyRequest struct {
	Site       string `json:"site"`        // 应用 ID
	OrderID    string `json:"order_id"`    // 商户订单号
	UID        string `json:"uid"`         // 用户ID
	SID        string `json:"sid"`         // 服务器ID
	CPOrderID  string `json:"cp_order_id"` // CP订单号
	RoleID     string `json:"roleid"`      // 角色ID
	RoleName   string `json:"rolename"`    // 角色名称
	OrderMoney string `json:"order_money"` // 实际总价
	ProductID  string `json:"productid"`   // 商品ID
	Time       string `json:"time"`        // 时间戳
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

	// 读取请求体
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("读取购买通知请求失败", zap.Error(err))
		s.writePurchaseError(w, "READ_ERROR", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 解析请求参数
	var req PurchaseNotifyRequest

	// 尝试JSON格式解析
	if err := json.Unmarshal(body, &req); err != nil {
		// 如果JSON解析失败，尝试从POST表单解析
		if parseErr := r.ParseForm(); parseErr != nil {
			s.logger.Error("解析购买通知请求失败", zap.Error(err), zap.String("body", string(body)))
			s.writePurchaseError(w, "PARSE_ERROR", http.StatusBadRequest)
			return
		}

		// 从表单获取参数
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
			Time:       r.FormValue("time"),
			Sign:       r.FormValue("sign"),
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
	// 从配置中获取应用ID（这里使用默认值，实际应该从配置文件读取）
	expectedSite := DefaultSiteID

	// TODO: 从配置文件中读取site配置
	// if s.config.GetSocial().Tiktok != nil {
	//     expectedSite = s.config.GetSocial().Tiktok.SiteID
	// }

	return site == expectedSite
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

	return true
}

// verifyPurchaseSignature 验证购买通知签名
func (s *ApiServer) verifyPurchaseSignature(req PurchaseNotifyRequest) bool {
	// 获取密钥（这里使用默认值，实际应该从配置文件读取）
	key := DefaultSiteKey

	// TODO: 从配置文件中读取key配置
	// if s.config.GetSocial().Tiktok != nil {
	//     key = s.config.GetSocial().Tiktok.SiteKey
	// }

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
		zap.String("sign_str", signStr),
		zap.String("calculated_sign", calculatedSign),
		zap.String("received_sign", req.Sign))

	return calculatedSign == req.Sign
}

// processPurchaseDelivery 处理购买发货
func (s *ApiServer) processPurchaseDelivery(ctx context.Context, req *PurchaseNotifyRequest) error {
	// TODO: 实现实际的发货逻辑
	// 1. 根据UID查找用户
	// 2. 验证订单是否已处理（防止重复发货）
	// 3. 发放商品到用户账户
	// 4. 记录订单状态

	s.logger.Info("开始处理发货",
		zap.String("uid", req.UID),
		zap.String("product_id", req.ProductID),
		zap.String("order_id", req.OrderID))

	// 示例：这里应该调用实际的发货逻辑
	// userID, err := FindUserByDeviceID(ctx, s.logger, s.db, req.UID)
	// if err != nil {
	//     return fmt.Errorf("查找用户失败: %v", err)
	// }

	// 发送通知或更新用户数据
	// ...

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
