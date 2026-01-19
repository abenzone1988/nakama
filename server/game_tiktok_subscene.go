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

// 订阅场景变化
const (
	ErrNoSuccess         = 0        // 成功
	ErrNoInvalidParam    = 28001007 // 参数错误
	ErrNoSignatureFailed = 28006009 // 校验签名失败
)

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

	// 3. 从配置获取Secret
	socialConfig := s.config.GetSocial()
	if socialConfig == nil || socialConfig.TikTok == nil {
		return false
	}
	secret := socialConfig.TikTok.DirectPlaySecret
	if secret == "" {
		return false
	}

	// 4. 计算签名
	expectedSignature := getSignature(params, bodyStr, secret)

	// 5. 验证签名
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

	// 3. 从配置获取Secret
	socialConfig := s.config.GetSocial()
	if socialConfig == nil || socialConfig.TikTok == nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	secret := socialConfig.TikTok.DirectPlaySecret
	if secret == "" {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 4. 计算响应签名
	signature := getSignature(params, bodyStr, secret)

	// 5. 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-signature", signature)

	// 6. 写入响应体
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
	scenes := make([]*Scene, 0)

	// 从配置获取测试模式设置
	socialConfig := s.config.GetSocial()
	if socialConfig == nil || socialConfig.TikTok == nil {
		return scenes, nil
	}

	// 如果启用测试模式，直接返回所有场景
	if socialConfig.TikTok.DirectPlayTest {
		s.logger.Info("测试模式：返回所有场景")
		testScenes := []*Scene{}
		if scene1ID := socialConfig.TikTok.Scene1ContentID; scene1ID != "" {
			testScenes = append(testScenes, &Scene{
				Scene:      1,
				ContentIDs: []string{scene1ID},
				Extra:      "",
			})
		}
		if scene3ID := socialConfig.TikTok.Scene3ContentID; scene3ID != "" {
			testScenes = append(testScenes, &Scene{
				Scene:      3,
				ContentIDs: []string{scene3ID},
				Extra:      "",
			})
		}
		return testScenes, nil
	}

	// 从 openid 获取 userID
	userID, err := FindUserByDeviceID(ctx, s.logger, s.db, openid)
	if err != nil {
		s.logger.Error("根据 openid 查找用户失败", zap.String("openid", openid), zap.Error(err))
		return scenes, nil
	}

	// 将 userID 设置到 context 中
	ctx = context.WithValue(ctx, ctxUserIDKey{}, userID)

	// 读取 ByteDirectPlay 数据
	directPlay := &ByteRewardData{}
	if err := LoadUserData(ctx, s.logger, s.db, directPlay); err != nil {
		s.logger.Error("读取 ByteDirectPlay 数据失败", zap.Error(err))
		directPlay.Init()
	}

	currentTime := time.Now().UTC()
	currentDate := currentTime.Format("2006-01-02")

	// 2. 查询离线收益数据
	battleData := &BattleData{}
	if err := LoadUserData(ctx, s.logger, s.db, battleData); err != nil {
		s.logger.Error("读取Home数据失败", zap.Error(err))
		return nil, err
	}

	// 解析lastGetOnHookTimestamp
	lastGetTime, err := parseTime(battleData.LastGetOnHookTimestamp)
	if err != nil {
		s.logger.Error("解析时间格式失败", zap.Error(err))
		return nil, err
	}

	scene1ContentID := socialConfig.TikTok.Scene1ContentID
	scene3ContentID := socialConfig.TikTok.Scene3ContentID

	// 判断是否超过8小时且今天未返回过该场景
	if scene1ContentID != "" && time.Since(lastGetTime).Hours() >= 8 {
		lastSceneTime := directPlay.SceneTimestamps[scene1ContentID]
		isSameDay := false
		if lastSceneTime != "" {
			lastTime, err := parseTime(lastSceneTime)
			if err == nil {
				lastDate := lastTime.UTC().Format("2006-01-02")
				isSameDay = lastDate == currentDate
			}
		}
		if !isSameDay {
			scenes = append(scenes, &Scene{
				Scene:      1,
				ContentIDs: []string{scene1ContentID},
				Extra:      "",
			})
			directPlay.SceneTimestamps[scene1ContentID] = currentTime.Format(time.RFC3339)
		}
	}

	// 检查重新进入关卡场景
	if scene3ContentID != "" {
		lastReFightTime := directPlay.SceneTimestamps[scene3ContentID]
		isSameDay := false
		if lastReFightTime != "" {
			lastTime, err := parseTime(lastReFightTime)
			if err == nil {
				lastDate := lastTime.UTC().Format("2006-01-02")
				isSameDay = lastDate == currentDate
			}
		}
		if !isSameDay {
			scenes = append(scenes, &Scene{
				Scene:      3,
				ContentIDs: []string{scene3ContentID},
				Extra:      "",
			})
			directPlay.SceneTimestamps[scene3ContentID] = currentTime.Format(time.RFC3339)
			directPlay.GetDirectPlayReward = false
		}
	}

	// 保存更新后的 ByteDirectPlay 数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, directPlay); err != nil {
		s.logger.Error("保存 ByteDirectPlay 数据失败", zap.Error(err))
	}

	return scenes, nil
}
