package server

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"

	"go.uber.org/zap"
)

const (
	push_token = "sparkinfi"
)

// WechatResponse 微信接口统一返回格式
type WechatResponse struct {
	ErrCode int    `json:"ErrCode"`
	ErrMsg  string `json:"ErrMsg"`
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

// HandleWechatVerify 处理微信服务器发来的验证请求
// 当配置消息推送URL时，微信服务器会发送一个验证请求
func (s *ApiServer) HandleWechatVerify(w http.ResponseWriter, r *http.Request) {
	// 处理GET请求（微信服务器验证）
	if r.Method == "GET" {
		// 获取请求参数
		signature := r.URL.Query().Get("signature")
		timestamp := r.URL.Query().Get("timestamp")
		nonce := r.URL.Query().Get("nonce")
		echostr := r.URL.Query().Get("echostr")

		// 验证签名
		valid, err := s.verifyWechatSignature(signature, timestamp, nonce)
		if err != nil {
			s.logger.Warn("微信消息推送Token验证失败", zap.Error(err))
			http.Error(w, "Token configuration missing", http.StatusInternalServerError)
			return
		}

		if valid {
			// 验证成功，返回echostr
			s.logger.Info("微信消息推送验证成功")
			w.Write([]byte(echostr))
		} else {
			// 验证失败
			s.logger.Warn("微信消息推送验证失败")
			http.Error(w, "Signature verification failed", http.StatusBadRequest)
		}
		return
	}

	// 处理POST请求（微信消息推送）
	if r.Method == "POST" {
		// 首先验证签名
		signature := r.URL.Query().Get("signature")
		timestamp := r.URL.Query().Get("timestamp")
		nonce := r.URL.Query().Get("nonce")

		valid, err := s.verifyWechatSignature(signature, timestamp, nonce)
		if err != nil {
			s.logger.Warn("微信消息推送Token验证失败", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Token configuration missing",
			})
			return
		}

		if !valid {
			s.logger.Warn("微信消息推送验证失败")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Signature verification failed",
			})
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("读取微信消息推送请求失败", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Failed to read request body",
			})
			return
		}
		defer r.Body.Close()

		// 解析推送消息
		var pushMsg WechatPushMessage
		if err := json.Unmarshal(body, &pushMsg); err != nil {
			s.logger.Error("解析微信消息推送失败", zap.Error(err), zap.String("body", string(body)))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Failed to parse message",
			})
			return
		}

		// 处理不同类型的消息
		switch pushMsg.Event {
		case "minigame_deliver_goods":
			// 处理发货请求
			s.logger.Info("收到小游戏发货请求",
				zap.String("orderId", pushMsg.MiniGame.OrderId),
				zap.String("openId", pushMsg.MiniGame.ToUserOpenid),
				zap.Int("zone", pushMsg.MiniGame.Zone),
				zap.Any("goods", pushMsg.MiniGame.GoodsList))

			// TODO: 在这里处理实际的发货逻辑
			// 1. 验证订单是否已经处理过
			// 2. 发放物品到用户账户
			// 3. 记录发货日志

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(WechatResponse{
				ErrCode: 0,
				ErrMsg:  "Success",
			})
			return
		default:
			s.logger.Info("收到其他类型的微信消息推送",
				zap.String("event", pushMsg.Event),
				zap.String("msgType", pushMsg.MsgType))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(WechatResponse{
				ErrCode: 0,
				ErrMsg:  "Success",
			})
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WechatResponse{
			ErrCode: -1,
			ErrMsg:  "Method not allowed",
		})
	}
}
