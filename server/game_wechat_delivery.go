package server

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 微信发货
const (
	push_token                  = "sparkinfi"
	sub_err_code_user_not_found = 172935494
	NotificationCodeWechatGift  = 1
)

// WechatResponse 微信接口统一返回格式
type WechatResponse struct {
	ErrCode    int    `json:"ErrCode"`
	ErrMsg     string `json:"ErrMsg"`
	SubErrCode int    `json:"SubErrCode"`
}

// WechatGoodsItem 物品信息
type WechatGoodsItem struct {
	Id  string `json:"Id"`  // 物品ID
	Num int    `json:"Num"` // 物品数量
}

// WechatMiniGameInfo 小游戏发货信息
type WechatMiniGameInfo struct {
	OrderId      string            `json:"OrderId"`      // 订单ID
	IsPreview    int               `json:"IsPreview"`    // 是否为预览
	ToUserOpenid string            `json:"ToUserOpenid"` // 接收者OpenID
	GoodsList    []WechatGoodsItem `json:"GoodsList"`    // 物品列表
	Zone         int               `json:"Zone"`         // 区服ID
	GiftTypeId   int               `json:"GiftTypeId"`   // 礼包类型ID
	GiftId       string            `json:"GiftId"`       // 礼包ID
	SendTime     int64             `json:"SendTime"`     // 发送时间
}

// WechatPushMessage 微信推送消息
type WechatPushMessage struct {
	ToUserName   string             `json:"ToUserName"`   // 接收者
	FromUserName string             `json:"FromUserName"` // 发送者
	CreateTime   int64              `json:"CreateTime"`   // 创建时间
	MsgType      string             `json:"MsgType"`      // 消息类型
	Event        string             `json:"Event"`        // 事件类型
	MiniGame     WechatMiniGameInfo `json:"MiniGame"`     // 小游戏信息
	Encrypt      string             `json:"Encrypt"`      // 加密消息
}

// verifyWechatSignature 验证请求是否来自微信服务器
func (s *ApiServer) verifyWechatSignature(signature, timestamp, nonce string) (bool, error) {
	// 1. 将token、timestamp、nonce三个参数进行字典序排序
	params := []string{push_token, timestamp, nonce}
	sort.Strings(params)

	// 2. 将三个参数字符串拼接成一个字符串进行sha1计算
	str := strings.Join(params, "")
	hash := sha1.New()
	hash.Write([]byte(str))
	hashcode := fmt.Sprintf("%x", hash.Sum(nil))

	// 3. 验证签名是否正确
	return hashcode == signature, nil
}

// handleWechatGetRequest 处理微信服务器的GET验证请求
func (s *ApiServer) handleWechatGetRequest(w http.ResponseWriter, r *http.Request) {
	signature := r.URL.Query().Get("signature")
	timestamp := r.URL.Query().Get("timestamp")
	nonce := r.URL.Query().Get("nonce")
	echostr := r.URL.Query().Get("echostr")

	valid, err := s.verifyWechatSignature(signature, timestamp, nonce)
	if err != nil {
		s.logger.Warn("微信消息推送Token验证失败", zap.Error(err))
		http.Error(w, "Token configuration missing", http.StatusInternalServerError)
		return
	}

	if valid {
		s.logger.Info("微信消息推送验证成功")
		w.Write([]byte(echostr))
	} else {
		s.logger.Warn("微信消息推送验证失败")
		http.Error(w, "Signature verification failed", http.StatusBadRequest)
	}
}

// verifyPostSignature 验证POST请求的签名
func (s *ApiServer) verifyPostSignature(w http.ResponseWriter, r *http.Request) bool {
	signature := r.URL.Query().Get("signature")
	timestamp := r.URL.Query().Get("timestamp")
	nonce := r.URL.Query().Get("nonce")

	valid, err := s.verifyWechatSignature(signature, timestamp, nonce)
	if err != nil {
		s.logger.Warn("微信消息推送Token验证失败", zap.Error(err))
		s.writeWechatResponse(w, WechatResponse{
			ErrCode: -1,
			ErrMsg:  "Token configuration missing",
		})
		return false
	}

	if !valid {
		s.logger.Warn("微信消息推送验证失败")
		s.writeWechatResponse(w, WechatResponse{
			ErrCode: -1,
			ErrMsg:  "Signature verification failed",
		})
		return false
	}

	return true
}

// writeWechatResponse 写入微信响应
func (s *ApiServer) writeWechatResponse(w http.ResponseWriter, response WechatResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// sendGiftNotification 发送礼物通知
func (s *ApiServer) sendGiftNotification(ctx context.Context, userID uuid.UUID, orderId string, goods []WechatGoodsItem, giftTypeId int, sendTime int64) error {
	notifications := make(map[uuid.UUID][]*api.Notification)
	content := map[string]interface{}{
		"goods":    goods,
		"giftType": giftTypeId,
	}
	contentJson, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to marshal notification content: %v", err)
	}

	// 使用订单ID生成UUID
	notificationId := uuid.NewV5(uuid.NamespaceURL, orderId)

	notifications[userID] = []*api.Notification{{
		Id:         notificationId.String(),
		Subject:    "wechat_gift_delivery",
		Content:    string(contentJson),
		Code:       int32(giftTypeId),
		CreateTime: timestamppb.New(time.Unix(sendTime, 0)),
		Persistent: true,
		SenderId:   uuid.Nil.String(),
	}}

	return NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications)
}

// handleDeliverGoods 处理发货请求
func (s *ApiServer) handleDeliverGoods(w http.ResponseWriter, r *http.Request, miniGame *WechatMiniGameInfo) {
	s.logger.Info("收到小游戏发货请求",
		zap.String("orderId", miniGame.OrderId),
		zap.String("openId", miniGame.ToUserOpenid),
		zap.Int("zone", miniGame.Zone),
		zap.Any("goods", miniGame.GoodsList))

	// 1. 查找用户
	userID, err := FindUserByDeviceID(r.Context(), s.logger, s.db, miniGame.ToUserOpenid)
	if err != nil {
		s.logger.Error("查找用户失败", zap.Error(err))
		if err.Error() == fmt.Sprintf("user not found for openid: %s", miniGame.ToUserOpenid) {
			s.writeWechatResponse(w, WechatResponse{
				ErrCode:    -1,
				ErrMsg:     "User not found",
				SubErrCode: sub_err_code_user_not_found,
			})
		} else {
			s.writeWechatResponse(w, WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Internal error",
			})
		}
		return
	}

	// 2. 发送通知
	if err := s.sendGiftNotification(r.Context(), userID, miniGame.OrderId, miniGame.GoodsList, miniGame.GiftTypeId, miniGame.SendTime); err != nil {
		s.logger.Error("发送通知失败", zap.Error(err))
		s.writeWechatResponse(w, WechatResponse{
			ErrCode: -1,
			ErrMsg:  "Failed to send notification",
		})
		return
	}

	s.writeWechatResponse(w, WechatResponse{
		ErrCode: 0,
		ErrMsg:  "Success",
	})
}

// HandleWechatVerify 处理微信服务器发来的验证请求
func (s *ApiServer) HandleWechatVerify(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.handleWechatGetRequest(w, r)
		return
	case "POST":
		if !s.verifyPostSignature(w, r) {
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("读取微信消息推送请求失败", zap.Error(err))
			s.writeWechatResponse(w, WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Failed to read request body",
			})
			return
		}
		defer r.Body.Close()

		var pushMsg WechatPushMessage
		if err := json.Unmarshal(body, &pushMsg); err != nil {
			s.logger.Error("解析微信消息推送失败", zap.Error(err), zap.String("body", string(body)))
			s.writeWechatResponse(w, WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Failed to parse message",
			})
			return
		}

		switch pushMsg.Event {
		case "minigame_deliver_goods":
			s.handleDeliverGoods(w, r, &pushMsg.MiniGame)
		default:
			s.logger.Info("收到其他类型的微信消息推送",
				zap.String("event", pushMsg.Event),
				zap.String("msgType", pushMsg.MsgType))
			s.writeWechatResponse(w, WechatResponse{
				ErrCode: 0,
				ErrMsg:  "Success",
			})
		}
	default:
		s.writeWechatResponse(w, WechatResponse{
			ErrCode: -1,
			ErrMsg:  "Method not allowed",
		})
	}
}
