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

	// 场景内容ID
	ContentOfflineIncome = "CONTENT963547906" // 离线收益场景内容ID
	ContentReFight       = "CONTENT586019586" // 重新进入关卡

	// 测试模式
	TestModeEnabled = false // 是否启用测试模式，启用后将返回所有场景
)

const TestSecret = "Mkj586019074Fdh"

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

type ByteDirectPlay struct {
	SceneTimestamps map[int64]string `json:"scene_timestamps"` // 记录每个场景的最后返回时间
}

func (f *ByteDirectPlay) GetCollection() string {
	return "ByteGame"
}

func (f *ByteDirectPlay) GetKey() string {
	return "DirectPlay"
}

func (f *ByteDirectPlay) Init() {
	if f.SceneTimestamps == nil {
		f.SceneTimestamps = make(map[int64]string)
	}
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
				Scene:      1,
				ContentIDs: []string{ContentOfflineIncome},
				Extra:      "",
			},
			{
				Scene:      3,
				ContentIDs: []string{ContentReFight},
				Extra:      "",
			},
		}, nil
	}

	// 读取 ByteDirectPlay 数据
	directPlay := &ByteDirectPlay{}
	if err := LoadData(ctx, s.logger, s.db, userID, directPlay); err != nil {
		s.logger.Error("读取 ByteDirectPlay 数据失败", zap.Error(err))
		directPlay.Init()
	}

	currentTime := time.Now().UTC()
	currentDate := currentTime.Format("2006-01-02")

	// 2. 查询离线收益数据
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

	// 判断是否超过8小时且今天未返回过该场景
	if time.Since(lastGetTime).Hours() >= 8 {
		lastSceneTime := directPlay.SceneTimestamps[1]
		if lastSceneTime == "" || !strings.HasPrefix(lastSceneTime, currentDate) {
			scenes = append(scenes, &Scene{
				Scene:      1,
				ContentIDs: []string{ContentOfflineIncome},
				Extra:      "",
			})
			directPlay.SceneTimestamps[1] = currentTime.Format(time.RFC3339)
		}
	}

	// 检查重新进入关卡场景
	lastReFightTime := directPlay.SceneTimestamps[3]
	if lastReFightTime == "" || !strings.HasPrefix(lastReFightTime, currentDate) {
		scenes = append(scenes, &Scene{
			Scene:      3,
			ContentIDs: []string{ContentReFight},
			Extra:      "",
		})
		directPlay.SceneTimestamps[3] = currentTime.Format(time.RFC3339)
	}

	// 保存更新后的 ByteDirectPlay 数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, directPlay); err != nil {
		s.logger.Error("保存 ByteDirectPlay 数据失败", zap.Error(err))
	}

	return scenes, nil
}
