package server

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	ErrNoSuccess         = 0        // 成功
	ErrNoInvalidParam    = 28001007 // 参数错误
	ErrNoSignatureFailed = 28006009 // 校验签名失败

	// 场景类型
	SceneOfflineIncome = 1 // 离线收益场景
	SceneStaminaFull   = 2 // 体力恢复场景

	// 场景内容ID
	ContentOfflineIncome = "CONTENT621161474" // 离线收益场景内容ID
	ContentStaminaFull   = "CONTENT611333890" // 体力恢复场景内容ID
	ContentExitNoLevel   = "CONTENT663576322" // 退出游戏，无中途退出关卡场景内容ID
	ContentExitWithLevel = "CONTENT605195010" // 退出游戏，有中途退出关卡场景内容ID

	// 测试模式
	TestModeEnabled = false // 是否启用测试模式，启用后将返回所有场景
)

const TestSecret = "HpF584757762CNY"

// Scene 场景信息
type Scene struct {
	Scene      int64    `json:"scene"`           // 场景类型: 1-离线收益场景 2-体力恢复场景 3-重要事件掉落
	ContentIDs []string `json:"content_ids"`     // 内容ID列表
	Extra      string   `json:"extra,omitempty"` // 额外信息,可选
}

// DataStruct 数据结构
type DataStruct struct {
	Scenes []*Scene `json:"scenes"` // 场景列表
}

// SceneListResponse 场景列表响应
type SceneListResponse struct {
	ErrNo  int32      `json:"err_no"`  // 错误码
	ErrMsg string     `json:"err_msg"` // 错误信息
	Data   DataStruct `json:"data"`    // 数据
}

// StaminaData 体力数据
type StaminaData struct {
	Stamina         int    `json:"stamina"`
	UpdateTimestamp string `json:"updateTimestamp"`
}

func (s *StaminaData) GetCollection() string {
	return "Home"
}

func (s *StaminaData) GetKey() string {
	return "Stamina"
}

func (s *StaminaData) Init() {
	s.Stamina = 0
	s.UpdateTimestamp = time.Now().UTC().Format(time.RFC3339)
}

// NetReconnectData 重连数据
type NetReconnectData struct {
	BattleType int `json:"battleType"` // 战斗类型
}

func (s *NetReconnectData) GetCollection() string {
	return "Battle"
}

func (s *NetReconnectData) GetKey() string {
	return "Reconnect"
}

func (s *NetReconnectData) Init() {
	s.BattleType = 0
}

// getSignature 计算签名
func getSignature(params map[string]string, bodyStr, secret string) string {
	// 1. 对url的query参数按key字典序排序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 2. 将key-value按顺序连接起来
	kvList := make([]string, 0, len(params))
	for _, k := range keys {
		kvList = append(kvList, k+"="+params[k])
	}
	urlParams := strings.Join(kvList, "&")

	// 3. 拼接body和secret
	rawData := urlParams + bodyStr + secret

	// 4. 计算md5并base64编码
	md5Sum := md5.Sum([]byte(rawData))
	return base64.StdEncoding.EncodeToString(md5Sum[:])
}

// verifySignature 验证签名
func (s *ApiServer) verifySignature(r *http.Request, bodyStr string) bool {
	// 1. 获取请求参数
	params := make(map[string]string)
	params["appid"] = r.URL.Query().Get("appid")
	params["openid"] = r.URL.Query().Get("openid")
	params["nonce"] = r.URL.Query().Get("nonce")
	params["timestamp"] = r.URL.Query().Get("timestamp")

	// 2. 获取签名
	signature := r.Header.Get("x-signature")
	if signature == "" {
		return false
	}

	// 3. 计算签名
	expectedSignature := getSignature(params, bodyStr, TestSecret)

	// 4. 验证签名
	return signature == expectedSignature
}

// writeFeedResponse 写入响应
func (s *ApiServer) writeFeedResponse(w http.ResponseWriter, r *http.Request, response SceneListResponse) {
	// 1. 将响应序列化为JSON字符串
	bodyBytes, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("序列化响应失败", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	bodyStr := string(bodyBytes)

	// 2. 获取请求参数用于签名
	params := make(map[string]string)
	params["appid"] = r.URL.Query().Get("appid")
	params["openid"] = r.URL.Query().Get("openid")
	params["nonce"] = r.URL.Query().Get("nonce")
	params["timestamp"] = r.URL.Query().Get("timestamp")

	// 3. 计算响应签名
	signature := getSignature(params, bodyStr, TestSecret)

	// 4. 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-signature", signature)

	// 5. 写入响应体
	w.Write(bodyBytes)
}

// HandleSceneList 处理场景列表查询请求
func (s *ApiServer) HandleSceneList(w http.ResponseWriter, r *http.Request) {
	// 1. 验证请求方法
	if r.Method != http.MethodGet {
		s.writeFeedResponse(w, r, SceneListResponse{
			ErrNo:  ErrNoInvalidParam,
			ErrMsg: "invalid method",
		})
		return
	}

	// 2. 验证必要参数
	appid := r.URL.Query().Get("appid")
	openid := r.URL.Query().Get("openid")
	nonce := r.URL.Query().Get("nonce")
	timestamp := r.URL.Query().Get("timestamp")

	if appid == "" || openid == "" || nonce == "" || timestamp == "" {
		s.writeFeedResponse(w, r, SceneListResponse{
			ErrNo:  ErrNoInvalidParam,
			ErrMsg: "missing required parameters",
		})
		return
	}

	// 3. 验证签名
	if !s.verifySignature(r, "") {
		s.writeFeedResponse(w, r, SceneListResponse{
			ErrNo:  ErrNoSignatureFailed,
			ErrMsg: "check signature failed",
		})
		return
	}

	// 4. 查询用户场景列表
	scenes, err := s.queryUserScenes(r.Context(), openid)
	if err != nil {
		s.logger.Error("查询用户场景列表失败",
			zap.String("openid", openid),
			zap.Error(err))
		s.writeFeedResponse(w, r, SceneListResponse{
			ErrNo:  ErrNoSuccess,
			ErrMsg: "",
			Data: DataStruct{
				Scenes: []*Scene{},
			},
		})
		return
	}

	// 5. 返回场景列表
	s.writeFeedResponse(w, r, SceneListResponse{
		ErrNo:  ErrNoSuccess,
		ErrMsg: "",
		Data: DataStruct{
			Scenes: scenes,
		},
	})
}

// queryUserScenes 查询用户场景列表
func (s *ApiServer) queryUserScenes(ctx context.Context, openid string) ([]*Scene, error) {
	// 1. 通过openid查询userId
	userID, err := FindUserByDeviceID(ctx, s.logger, s.db, openid)
	if err != nil {
		s.logger.Error("查询用户ID失败", zap.Error(err), zap.String("openid", openid))
		return nil, err
	}

	scenes := make([]*Scene, 0)

	// 如果启用测试模式，直接返回所有场景
	if TestModeEnabled {
		s.logger.Info("测试模式：返回所有场景")
		return []*Scene{
			{
				Scene:      SceneOfflineIncome,
				ContentIDs: []string{ContentOfflineIncome},
				Extra:      "",
			},
		}, nil
	}

	// 2. 查询离线收益数据s
	homeData := &HomeData{}
	if err := LoadData(ctx, s.logger, s.db, userID, homeData); err != nil {
		s.logger.Error("读取Home数据失败", zap.Error(err))
		return nil, err
	}

	// 解析lastGetOnHookTimestamp
	lastGetTime, err := parseTime(homeData.LastGetOnHookTimestamp)
	if err != nil {
		s.logger.Error("解析时间格式失败", zap.Error(err))
		return nil, err
	}

	//s.logger.Info("离线收益时间间隔", zap.Float64("Hours", time.Since(lastGetTime).Hours()))

	// 判断是否超过8小时
	if time.Since(lastGetTime).Hours() >= 8 {
		scenes = append(scenes, &Scene{
			Scene:      SceneOfflineIncome,
			ContentIDs: []string{ContentOfflineIncome},
			Extra:      "",
		})
	}

	// 3. 查询重连数据
	reconnectData := &NetReconnectData{}
	if err := LoadData(ctx, s.logger, s.db, userID, reconnectData); err != nil {
		s.logger.Error("读取Reconnect数据失败", zap.Error(err))
		return nil, err
	}

	if reconnectData.BattleType == 0 {
		// 非关卡退出
		scenes = append(scenes, &Scene{
			Scene:      3,
			ContentIDs: []string{ContentExitNoLevel},
			Extra:      "",
		})
	} else {
		// 关卡退出
		scenes = append(scenes, &Scene{
			Scene:      3,
			ContentIDs: []string{ContentExitWithLevel},
			Extra:      "",
		})
	}

	// 3. 查询体力数据 暂时不用
	// staminaData := &StaminaData{}
	// if err := LoadData(ctx, s.logger, s.db, userID, staminaData); err != nil {
	// 	s.logger.Error("读取Stamina数据失败", zap.Error(err))
	// 	return nil, err
	// }

	// // 解析updateTimestamp
	// updateTime, err := parseTime(staminaData.UpdateTimestamp)
	// if err != nil {
	// 	s.logger.Error("解析时间格式失败", zap.Error(err))
	// 	return nil, err
	// }

	// //s.logger.Info("体力恢复时间间隔", zap.Float64("Minutes", time.Since(updateTime).Minutes()))

	// // 计算体力恢复
	// minutesPassed := time.Since(updateTime).Minutes()
	// recoveredStamina := int(minutesPassed / 20) // 每20分钟恢复1点体力
	// currentStamina := staminaData.Stamina + recoveredStamina

	// // 如果体力达到上限30
	// if currentStamina >= 30 {
	// 	scenes = append(scenes, &Scene{
	// 		Scene:      SceneStaminaFull,
	// 		ContentIDs: []string{ContentStaminaFull},
	// 		Extra:      "",
	// 	})
	// }

	return scenes, nil
}
