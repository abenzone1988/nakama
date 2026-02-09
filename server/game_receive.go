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
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	push_token                  = "sparkinfi"
	sub_err_code_user_not_found = 172935494
	NotificationCodeWechatGift  = 1
	ChallengeRewardItemID       = "60000" // 擂台赛获奖凭证商品ID
)

type GiftNotificationConfig struct {
	Title       string
	Description string
}

var giftNotificationConfigs = map[string]GiftNotificationConfig{
	"CBgAAoXb6-hx2kb4vrq9mMP2tXCYy-CnIDjeL16t6_ZBPJLZZWcCOnnyoNDIGle4ju2SMRwwRdwYevb4": {
		Title:       "每日登录奖励",
		Description: "您的每日登录奖励已送达，请查收！",
	},
	"CBgAAoXb6-hx2kb4vrq9mMP2tXCYy-Cmca2VjoRJHnCv4wtyo9B1YoJmvh14frdIM7Gfwk6sV_dfQ2n4": {
		Title:       "擂台赛胜利",
		Description: "恭喜您在擂台赛中取得胜利！奖励已送达，请查收。",
	},
}

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
	QueryReward  QueryRewardRequest `json:"QueryReward"`  // 查询奖励信息
	Encrypt      string             `json:"Encrypt"`      // 加密消息
}

// QueryRewardRequest 查询奖励请求
type QueryRewardRequest struct {
	ToUserOpenid string `json:"ToUserOpenid"` // 用户OpenID
	ItemID       string `json:"ItemID"`       // 商品ID
}

// QueryRewardResponse 查询奖励响应
type QueryRewardResponse struct {
	ErrCode    int    `json:"ErrCode"`
	ErrMsg     string `json:"ErrMsg"`
	TodayCount int32  `json:"TodayCount"` // 当天获得数量
	TotalCount int32  `json:"TotalCount"` // 历史总数量
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

// 处理微信服务器的GET验证请求
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
func (s *ApiServer) sendGiftNotification(ctx context.Context, userID uuid.UUID, orderId string, goods []WechatGoodsItem, giftTypeId int, giftId string, sendTime int64) error {
	if len(goods) == 0 {
		s.logger.Warn("礼物物品列表为空", zap.String("order_id", orderId))
		return nil
	}

	items := make([]*game.Item, 0, len(goods))
	for _, good := range goods {
		items = append(items, &game.Item{
			Id:  good.Id,
			Num: int32(good.Num),
		})
	}

	reward := &game.Reward{
		Items: items,
	}

	config, exists := giftNotificationConfigs[giftId]
	title := "微信礼包领取"
	description := "微信礼包已送到，请查收。"
	if exists {
		title = config.Title
		description = config.Description
	}

	content := map[string]interface{}{
		"description": description,
		"rewards":     []*game.Reward{reward},
	}

	contentBytes, err := json.Marshal(content)
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return fmt.Errorf("failed to marshal notification content: %v", err)
	}

	notificationId := uuid.NewV5(uuid.NamespaceURL, orderId)

	notification := &api.Notification{
		Id:         notificationId.String(),
		Subject:    title,
		Content:    string(contentBytes),
		Code:       NotificationSystemNotice,
		SenderId:   uuid.Nil.String(),
		CreateTime: timestamppb.New(time.Unix(sendTime, 0)),
		Persistent: true,
	}

	notifications := make(map[uuid.UUID][]*api.Notification)
	notifications[userID] = []*api.Notification{notification}

	err = NotificationSend(ctx, s.logger, s.db, s.tracker, s.router, notifications)
	if err != nil {
		s.logger.Error("发送礼物通知失败",
			zap.String("user_id", userID.String()),
			zap.String("order_id", orderId),
			zap.String("gift_id", giftId),
			zap.Error(err))
		return err
	}

	s.logger.Info("成功发送微信礼包奖励通知",
		zap.String("user_id", userID.String()),
		zap.String("order_id", orderId),
		zap.String("gift_id", giftId))

	return nil
}

// handleQueryReward 处理查询奖励请求
func (s *ApiServer) handleQueryReward(w http.ResponseWriter, r *http.Request, queryReq *QueryRewardRequest) {
	s.logger.Info("收到查询奖励请求",
		zap.String("openId", queryReq.ToUserOpenid),
		zap.String("itemId", queryReq.ItemID))

	// 查找用户
	userID, err := FindUserByDeviceID(r.Context(), s.logger, s.db, queryReq.ToUserOpenid)
	if err != nil {
		s.logger.Error("查找用户失败", zap.Error(err))
		s.writeQueryRewardResponse(w, QueryRewardResponse{
			ErrCode: -1,
			ErrMsg:  "User not found",
		})
		return
	}

	// 加载奖励数据
	challengeRewards := &ChallengeRewards{}
	err = LoadData(r.Context(), s.logger, s.db, userID, challengeRewards)
	if err != nil {
		s.logger.Warn("加载擂台赛奖励数据失败，返回0", zap.Error(err))
		s.writeQueryRewardResponse(w, QueryRewardResponse{
			ErrCode:    0,
			ErrMsg:     "Success",
			TodayCount: 0,
			TotalCount: 0,
		})
		return
	}

	// 获取数量
	todayCount := challengeRewards.GetTodayCount(queryReq.ItemID)
	totalCount := challengeRewards.GetTotalCount(queryReq.ItemID)

	s.logger.Info("查询奖励成功",
		zap.String("userId", userID.String()),
		zap.String("itemId", queryReq.ItemID),
		zap.Int32("todayCount", todayCount),
		zap.Int32("totalCount", totalCount))

	s.writeQueryRewardResponse(w, QueryRewardResponse{
		ErrCode:    0,
		ErrMsg:     "Success",
		TodayCount: todayCount,
		TotalCount: totalCount,
	})
}

// writeQueryRewardResponse 写入查询奖励响应
func (s *ApiServer) writeQueryRewardResponse(w http.ResponseWriter, response QueryRewardResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDeliverGoods 处理发货请求
func (s *ApiServer) handleDeliverGoods(w http.ResponseWriter, r *http.Request, miniGame *WechatMiniGameInfo) {
	s.logger.Info("收到小游戏发货请求",
		zap.String("orderId", miniGame.OrderId),
		zap.String("giftId", miniGame.GiftId),
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

	// 2. 处理特殊商品 ID 60000（擂台赛获奖凭证）
	regularGoods := []WechatGoodsItem{}
	var challengeRewardCount int32 = 0

	for _, good := range miniGame.GoodsList {
		if good.Id == ChallengeRewardItemID {
			challengeRewardCount += int32(good.Num)
		} else {
			regularGoods = append(regularGoods, good)
		}
	}

	// 3. 保存擂台赛奖励凭证到 storage
	if challengeRewardCount > 0 {
		challengeRewards := &ChallengeRewards{}
		err := LoadData(r.Context(), s.logger, s.db, userID, challengeRewards)
		if err != nil {
			s.logger.Warn("首次加载擂台赛奖励数据，初始化新数据", zap.Error(err))
		}

		// 初始化数据结构
		if challengeRewards.Rewards == nil {
			challengeRewards.Init()
		}

		// 添加奖励（自动处理历史总数和当日数量）
		challengeRewards.AddReward(ChallengeRewardItemID, challengeRewardCount)

		// 保存到 storage
		if err := SaveData(r.Context(), s.logger, s.db, s.metrics, s.storageIndex, userID, challengeRewards); err != nil {
			s.logger.Error("保存擂台赛奖励失败", zap.Error(err))
			s.writeWechatResponse(w, WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Failed to save challenge reward",
			})
			return
		}

		s.logger.Info("成功保存擂台赛奖励凭证",
			zap.String("userId", userID.String()),
			zap.Int32("count", challengeRewardCount))
	}

	// 4. 发送剩余商品通知
	if len(regularGoods) > 0 {
		if err := s.sendGiftNotification(r.Context(), userID, miniGame.OrderId, regularGoods, miniGame.GiftTypeId, miniGame.GiftId, miniGame.SendTime); err != nil {
			s.logger.Error("发送通知失败", zap.Error(err))
			s.writeWechatResponse(w, WechatResponse{
				ErrCode: -1,
				ErrMsg:  "Failed to send notification",
			})
			return
		}
	}

	s.writeWechatResponse(w, WechatResponse{
		ErrCode: 0,
		ErrMsg:  "Success",
	})
}

// HandleWechatVerify 处理微信服务器发来的验证请求
func (s *ApiServer) HandleWechatVerify(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("收到微信服务器发来的验证请求", zap.String("method", r.Method))
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
		case "query_challenge_reward":
			s.handleQueryReward(w, r, &pushMsg.QueryReward)
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
