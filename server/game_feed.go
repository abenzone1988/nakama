package server

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

const (
	ErrNoSuccess         = 0        // 成功
	ErrNoInvalidParam    = 28001007 // 参数错误
	ErrNoSignatureFailed = 28006009 // 校验签名失败

	// 场景类型
	SceneOfflineIncome = 1 // 离线收益场景
	SceneStaminaFull   = 2 // 体力恢复场景
	SceneImportant     = 3 //重要场景

	// 重要场景内容ID
	ContentImportantGift       = "CONTENT12567553538" // 超值赠礼可以领取
	ContentImportantSignIn     = "CONTENT12555269890" // 签到奖励
	ContentImportantChallenge  = "CONTENT12581991426" // 挑战赛开始
	ContentImportantRankChange = "CONTENT12611329282" // 挑战赛排名变化
	ContentImportantMainGame   = "CONTENT12595815426" // 主线关卡中途退出

	// 测试模式
	TestModeEnabled = false // 是否启用测试模式，启用后将返回所有场景

	// 场景缓存时间限制配置
	SceneCacheExpireHours = 8 // 默认场景缓存过期时间（小时）
)

// 不同场景ID的时间限制配置
var SceneExpireHoursConfig = map[string]int{
	ContentImportantGift:       24, // 超值赠礼：24小时
	ContentImportantSignIn:     24, // 签到奖励：8小时
	ContentImportantChallenge:  24, // 挑战赛开始：12小时
	ContentImportantRankChange: 8,  // 挑战赛排名变化：6小时
	ContentImportantMainGame:   8,  // 主线关卡中途退出：4小时
}

const TestSecret = "Fql12512872962aRv"

// Username 白名单配置
// 开启后，仅当用户名在白名单内时才返回场景；否则返回空场景列表
var UsernameWhitelistEnabled = false

// 通过将需要放行的用户名加入该表来管理测试玩家
var UsernameWhitelist = map[string]struct{}{
	// 示例："tester001": {}, "dev_user": {},
	"cINwZBvRGp": {},
	"eDgvzuwMmz": {},
}

func isUsernameAllowed(username string) bool {
	if !UsernameWhitelistEnabled {
		return true
	}
	_, ok := UsernameWhitelist[username]
	return ok
}

// Scene 场景信息
type Scene struct {
	Scene      int64    `json:"scene"`           // 场景类型: 1-离线收益场景 2-体力恢复场景 3-重要事件掉落
	ContentIDs []string `json:"content_ids"`     // 内容ID列表
	Extra      string   `json:"extra,omitempty"` // 额外信息,可选
}

// DataStruct 数据结构
type DataStruct struct {
	Scenes   []*Scene `json:"scenes"`   // 场景列表
	Username string   `json:"username"` // 用户名
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

// NetStarterPackData 超值赠礼数据
type NetStarterPackData struct {
	DateGot     []int  `json:"dateGot"`     // 已领取的日期列表
	LastGotTime string `json:"lastGotTime"` // 最后领取时间
}

// NetSignInData 登录奖励数据
type NetSignInData struct {
	SignInData     []int  `json:"m_SignInData"`     // 已领取的奖励ID列表
	LastSignInTime string `json:"m_LastSignInTime"` // 最后签到时间
	Group          int32  `json:"group"`            // 当前组别
}

// SceneCacheInfo 场景缓存信息
type SceneCacheInfo struct {
	LastReturnTime string `json:"last_return_time"` // 最后返回true的时间
	ExpireHours    int    `json:"expire_hours"`     // 过期时间（小时）
}

// ByteGameDirectPlay 场景缓存数据
type ByteGameDirectPlay struct {
	SceneCache map[string]SceneCacheInfo `json:"scene_cache"` // 场景ID -> 场景缓存信息
}

// RankChangeData 排名变化数据
type RankChangeData struct {
	OvertakenTournaments map[string]bool `json:"overtaken_tournaments"` // 被超越的竞标赛ID集合
	LastUpdated          int64           `json:"last_updated"`          // 最后更新时间戳
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

func (s *NetStarterPackData) GetCollection() string {
	return "Home"
}

func (s *NetStarterPackData) GetKey() string {
	return "StarterPack"
}

func (s *NetStarterPackData) Init() {
	s.DateGot = make([]int, 0)
	s.LastGotTime = time.Now().UTC().Format(time.RFC3339)
}

func (s *NetSignInData) GetCollection() string {
	return "Home"
}

func (s *NetSignInData) GetKey() string {
	return "SignIn"
}

func (s *NetSignInData) Init() {
	s.SignInData = make([]int, 0)
	s.LastSignInTime = time.Now().UTC().Format(time.RFC3339)
	s.Group = 1
}

func (s *ByteGameDirectPlay) GetCollection() string {
	return "ByteGame"
}

func (s *ByteGameDirectPlay) GetKey() string {
	return "DirectPlay"
}

func (s *ByteGameDirectPlay) Init() {
	s.SceneCache = make(map[string]SceneCacheInfo)
}

func (s *RankChangeData) GetCollection() string {
	return "ByteGame"
}

func (s *RankChangeData) GetKey() string {
	return "RankChange"
}

func (s *RankChangeData) Init() {
	s.OvertakenTournaments = make(map[string]bool)
	s.LastUpdated = 0
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

// checkStarterPackAvailable 检查超值赠礼是否可以领取
func (s *ApiServer) checkStarterPackAvailable(ctx context.Context, userID uuid.UUID) bool {
	// 1. 从存储中读取数据
	var starterPackData NetStarterPackData
	err := LoadData(ctx, s.logger, s.db, userID, &starterPackData)
	if err != nil {
		s.logger.Error("读取超值赠礼数据失败", zap.Error(err))
		return false
	}

	// 2. 检查最后领取时间是否是今天
	lastGotTime, err := time.Parse(time.RFC3339, starterPackData.LastGotTime)
	if err != nil {
		s.logger.Error("解析最后领取时间失败", zap.Error(err))
		return false
	}

	now := time.Now().UTC()
	if lastGotTime.Year() == now.Year() && lastGotTime.YearDay() == now.YearDay() {
		// 今天已经领取过了
		return false
	}

	// 4. 计算下一个可领取的ID
	var nextID int
	if len(starterPackData.DateGot) == 0 {
		nextID = 1
	} else {
		// 找到最大值并+1
		maxID := 0
		for _, id := range starterPackData.DateGot {
			if id > maxID {
				maxID = id
			}
		}
		nextID = maxID + 1
	}

	// 5. 检查这个ID是否在模板中存在
	_, exists := s.template.GetTplStarterPack().FindByKey(strconv.Itoa(nextID))
	return exists
}

// checkSignInAvailable 检查登录奖励是否可以领取
func (s *ApiServer) checkSignInAvailable(ctx context.Context, userID uuid.UUID) bool {
	// 1. 从存储中读取数据
	var signInData NetSignInData
	err := LoadData(ctx, s.logger, s.db, userID, &signInData)
	if err != nil {
		s.logger.Error("读取登录奖励数据失败", zap.Error(err))
		return false
	}

	// 2. 检查最后签到时间是否是今天
	lastSignInTime, err := time.Parse(time.RFC3339, signInData.LastSignInTime)
	if err != nil {
		s.logger.Error("解析最后签到时间失败", zap.Error(err))
		return false
	}

	now := time.Now().UTC()
	if lastSignInTime.Year() == now.Year() && lastSignInTime.YearDay() == now.YearDay() {
		// 今天已经签到过了
		return false
	}

	// 3. 获取当前组的奖励配置
	currentGroup := signInData.Group
	groupRewards := s.template.GetTplDailySignIn().FindByFilter(func(item template.TplDailySignIn) bool {
		return item.Group == currentGroup
	})

	if len(groupRewards) == 0 {
		s.logger.Warn("当前组没有奖励配置", zap.Int32("group", currentGroup))
		return false
	}

	// 4. 检查当前组是否已经领取完所有奖励
	if len(signInData.SignInData) >= len(groupRewards) {
		// 当前组已领取完，检查下一组是否存在
		nextGroup := currentGroup + 1
		nextGroupRewards := s.template.GetTplDailySignIn().FindByFilter(func(item template.TplDailySignIn) bool {
			return item.Group == nextGroup
		})

		// 如果下一组不存在，直接返回false
		if len(nextGroupRewards) == 0 {
			return false
		}

		// 下一组存在，可以切换到下一组
		return true
	}

	// 5. 当前组还有未领取的奖励
	return true
}

// checkChallengeAvailable 检查挑战赛开战订阅是否可以触发
func (s *ApiServer) checkChallengeAvailable(ctx context.Context, userID uuid.UUID) bool {
	// 1. 从存储中读取用户挑战赛数据
	var userMatch UserMatch
	err := LoadData(ctx, s.logger, s.db, userID, &userMatch)
	if err != nil {
		s.logger.Error("读取用户挑战赛数据失败", zap.Error(err))
		return false // 读取失败，不触发订阅
	}

	// 2. 获取所有挑战赛模板
	tplChallenges := s.template.GetTplChallenge().FindAll()
	if len(tplChallenges) == 0 {
		return false // 没有挑战赛配置
	}

	// 3. 获取当前时间
	now := time.Now()

	// 4. 检查是否有正在进行的比赛且用户未参加
	for _, tplChallenge := range tplChallenges {
		// 解析开始时间
		startTime, err := parseDateTime(tplChallenge.OpenTime)
		if err != nil {
			s.logger.Error("解析挑战赛开始时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("OpenTime", tplChallenge.OpenTime),
				zap.Error(err))
			continue
		}

		// 解析结束时间
		endTime, err := parseDateTime(tplChallenge.EndTime)
		if err != nil {
			s.logger.Error("解析挑战赛结束时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("EndTime", tplChallenge.EndTime),
				zap.Error(err))
			continue
		}

		// 判断是否正在进行中
		isOngoing := (now.After(startTime) || now.Equal(startTime)) && now.Before(endTime)
		if !isOngoing {
			continue // 不是正在进行的比赛，跳过
		}

		// 检查用户是否已经参加
		if challengeData, exists := userMatch.Challenges[tplChallenge.ID]; exists && challengeData != nil {
			continue // 用户已经参加，跳过
		}

		// 找到正在进行的比赛且用户未参加，可以触发订阅
		s.logger.Info("发现用户可参加的挑战赛",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.String("challenge_name", tplChallenge.Name),
			zap.Time("start_time", startTime),
			zap.Time("end_time", endTime))

		return true
	}

	// 5. 没有找到符合条件的挑战赛
	return false
}

// checkRankChangeAvailable 检查排名变化订阅是否可以触发
func (s *ApiServer) checkRankChangeAvailable(ctx context.Context, userID uuid.UUID) bool {
	// 1. 从存储中读取排名变化数据
	var rankChangeData RankChangeData
	err := LoadData(ctx, s.logger, s.db, userID, &rankChangeData)
	if err != nil {
		s.logger.Error("读取排名变化数据失败", zap.Error(err))
		return false // 读取失败，不触发订阅
	}

	// 2. 检查是否有被超越的竞标赛
	if len(rankChangeData.OvertakenTournaments) == 0 {
		return false // 没有被超越的竞标赛
	}

	// 3. 获取当前时间
	now := time.Now()

	// 4. 检查被超越的竞标赛是否还在进行中
	for tournamentID := range rankChangeData.OvertakenTournaments {
		// 获取竞标赛信息
		tournament := s.leaderboardCache.Get(tournamentID)
		if tournament == nil {
			// 竞标赛不存在，移除该记录
			delete(rankChangeData.OvertakenTournaments, tournamentID)
			continue
		}

		// 检查竞标赛是否还在进行中
		if tournament.EndTime > 0 && tournament.EndTime <= now.Unix() {
			// 竞标赛已结束，移除该记录
			delete(rankChangeData.OvertakenTournaments, tournamentID)
			continue
		}

		// 找到正在进行的被超越竞标赛，可以触发订阅
		s.logger.Info("发现用户被超越的竞标赛",
			zap.String("user_id", userID.String()),
			zap.String("tournament_id", tournamentID),
			zap.Time("tournament_end", time.Unix(tournament.EndTime, 0)))

		return true
	}

	// 5. 如果没有正在进行的被超越竞标赛，清空数据
	if len(rankChangeData.OvertakenTournaments) == 0 {
		rankChangeData.LastUpdated = now.Unix()
		SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, &rankChangeData)
	}

	return false
}

// checkSceneCache 检查场景缓存，判断是否在时间限制内
func (s *ApiServer) checkSceneCache(sceneID string, directPlayData *ByteGameDirectPlay) bool {
	// 1. 检查场景ID是否在缓存中
	cacheInfo, exists := directPlayData.SceneCache[sceneID]
	if !exists {
		return false // 不在缓存中，允许检查
	}

	// 2. 解析缓存时间
	cachedTime, err := time.Parse(time.RFC3339, cacheInfo.LastReturnTime)
	if err != nil {
		s.logger.Error("解析场景缓存时间失败", zap.Error(err), zap.String("scene_id", sceneID))
		return false // 解析失败，允许检查
	}

	// 3. 获取该场景的时间限制配置
	expireHours := cacheInfo.ExpireHours
	if expireHours <= 0 {
		// 如果缓存中没有配置，使用默认配置
		if configHours, exists := SceneExpireHoursConfig[sceneID]; exists {
			expireHours = configHours
		} else {
			expireHours = SceneCacheExpireHours // 使用默认值
		}
	}

	// 4. 检查是否在时间限制内
	now := time.Now().UTC()
	expireTime := cachedTime.Add(time.Duration(expireHours) * time.Hour)

	if now.Before(expireTime) {
		s.logger.Info("场景在缓存时间限制内，跳过检查",
			zap.String("scene_id", sceneID),
			zap.Time("cached_time", cachedTime),
			zap.Time("expire_time", expireTime),
			zap.Int("expire_hours", expireHours))
		return true // 在时间限制内，跳过检查
	}

	// 5. 已过期，允许检查
	return false
}

// updateSceneCache 更新场景缓存
func (s *ApiServer) updateSceneCache(ctx context.Context, userID uuid.UUID, sceneID string, directPlayData *ByteGameDirectPlay) error {
	// 1. 获取该场景的时间限制配置
	expireHours := SceneCacheExpireHours // 默认值
	if configHours, exists := SceneExpireHoursConfig[sceneID]; exists {
		expireHours = configHours
	}

	// 2. 更新场景缓存信息
	now := time.Now().UTC()
	directPlayData.SceneCache[sceneID] = SceneCacheInfo{
		LastReturnTime: now.Format(time.RFC3339),
		ExpireHours:    expireHours,
	}

	// 3. 保存更新后的缓存数据
	err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, directPlayData)
	if err != nil {
		s.logger.Error("保存场景缓存数据失败", zap.Error(err))
		return err
	}

	s.logger.Info("更新场景缓存成功",
		zap.String("scene_id", sceneID),
		zap.Time("cache_time", now),
		zap.Int("expire_hours", expireHours))

	return nil
}

// queryUserScenes 查询用户场景列表
func (s *ApiServer) queryUserScenes(ctx context.Context, openid string) ([]*Scene, error) {
	// 1. 通过openid查询userId
	userID, err := FindUserByDeviceID(ctx, s.logger, s.db, openid)
	if err != nil {
		s.logger.Error("查询用户ID失败", zap.Error(err), zap.String("openid", openid))
		return nil, err
	}

	// 1.1 获取用户名
	//account, err := GetAccount(ctx, s.logger, s.db, s.statusRegistry, userID)
	//if err != nil {
	//	s.logger.Error("查询用户信息失败", zap.Error(err), zap.String("user_id", userID.String()))
	//	return nil, err
	//}
	//username := account.User.Username

	// 1.2 白名单校验：不在白名单内则直接返回空场景
	//if !isUsernameAllowed(username) {
	//	s.logger.Info("用户名未在白名单内，跳过返回场景", zap.String("username", username), zap.String("user_id", userID.String()))
	//	return []*Scene{}, nil
	//}

	// 2. 只加载一次场景缓存数据
	var directPlayData ByteGameDirectPlay
	err = LoadData(ctx, s.logger, s.db, userID, &directPlayData)
	if err != nil {
		s.logger.Error("读取场景缓存数据失败", zap.Error(err))
		// 如果读取失败，初始化一个空的数据结构
		directPlayData.Init()
	}

	scenes := make([]*Scene, 0)

	// 如果启用测试模式，直接返回所有场景
	if TestModeEnabled {
		s.logger.Info("测试模式：返回所有场景")
		return []*Scene{
			{
				Scene: SceneImportant,
				ContentIDs: []string{
					ContentImportantGift,
					ContentImportantSignIn,
					ContentImportantChallenge,
					ContentImportantRankChange,
					ContentImportantMainGame,
				},
				Extra: "",
			},
		}, nil
	}

	needUpdate := false

	// 检查超值赠礼是否可以领取
	if !s.checkSceneCache(ContentImportantGift, &directPlayData) && s.checkStarterPackAvailable(ctx, userID) {
		scenes = append(scenes, &Scene{
			Scene:      SceneImportant,
			ContentIDs: []string{ContentImportantGift},
			Extra:      ContentImportantGift,
		})
		needUpdate = true
	}

	// 检查登录奖励是否可以领取
	if !s.checkSceneCache(ContentImportantSignIn, &directPlayData) && s.checkSignInAvailable(ctx, userID) {
		scenes = append(scenes, &Scene{
			Scene:      SceneImportant,
			ContentIDs: []string{ContentImportantSignIn},
			Extra:      ContentImportantSignIn,
		})
		needUpdate = true
	}

	// 检查挑战赛开战订阅
	if !s.checkSceneCache(ContentImportantChallenge, &directPlayData) && s.checkChallengeAvailable(ctx, userID) {
		scenes = append(scenes, &Scene{
			Scene:      SceneImportant,
			ContentIDs: []string{ContentImportantChallenge},
			Extra:      ContentImportantChallenge,
		})
		needUpdate = true
	}

	// 检查主线关卡中途退出订阅
	if !s.checkSceneCache(ContentImportantMainGame, &directPlayData) {
		scenes = append(scenes, &Scene{
			Scene:      SceneImportant,
			ContentIDs: []string{ContentImportantMainGame},
			Extra:      ContentImportantMainGame,
		})
		needUpdate = true
	}

	// 检查排名变化订阅
	if !s.checkSceneCache(ContentImportantRankChange, &directPlayData) && s.checkRankChangeAvailable(ctx, userID) {
		scenes = append(scenes, &Scene{
			Scene:      SceneImportant,
			ContentIDs: []string{ContentImportantRankChange},
			Extra:      ContentImportantRankChange,
		})
		needUpdate = true
	}

	if needUpdate {
		_ = s.updateSceneCache(ctx, userID, ContentImportantMainGame, &directPlayData)
	}

	return scenes, nil
}
