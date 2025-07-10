package examples

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/heroiclabs/nakama/v3/server"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

// ChallengeService 挑战赛服务
type ChallengeService struct {
	logger          *zap.Logger
	db              *sql.DB
	templateManager server.TemplateManager
	cache           server.LeaderboardCache
	scheduler       server.LeaderboardScheduler
}

// NewChallengeService 创建挑战赛服务
func NewChallengeService(logger *zap.Logger, db *sql.DB, templateManager server.TemplateManager,
	cache server.LeaderboardCache, scheduler server.LeaderboardScheduler) *ChallengeService {
	return &ChallengeService{
		logger:          logger,
		db:              db,
		templateManager: templateManager,
		cache:           cache,
		scheduler:       scheduler,
	}
}

// InitializeChallenges 初始化所有挑战赛
func (cs *ChallengeService) InitializeChallenges(ctx context.Context) error {
	challenges := cs.templateManager.GetTplChallenge().FindAll()

	for _, challenge := range challenges {
		if challenge.Status == 1 { // 只初始化启用的挑战赛
			err := cs.CreateChallenge(ctx, challenge)
			if err != nil {
				cs.logger.Error("创建挑战赛失败",
					zap.String("challenge_id", challenge.ID),
					zap.Error(err))
				continue
			}
			cs.logger.Info("成功创建挑战赛",
				zap.String("challenge_id", challenge.ID),
				zap.String("name", challenge.Name))
		}
	}

	return nil
}

// CreateChallenge 创建单个挑战赛
func (cs *ChallengeService) CreateChallenge(ctx context.Context, challenge template.TplChallenge) error {
	// 操作符映射
	operatorMap := map[string]int{
		"best": server.LeaderboardOperatorBest,
		"set":  server.LeaderboardOperatorSet,
		"incr": server.LeaderboardOperatorIncrement,
		"decr": server.LeaderboardOperatorDecrement,
	}

	operator, exists := operatorMap[challenge.Operator]
	if !exists {
		operator = server.LeaderboardOperatorBest
	}

	// 创建Tournament
	err := server.TournamentCreate(ctx, cs.logger, cs.cache, cs.scheduler,
		challenge.ID,                   // id
		true,                           // authoritative
		challenge.SortOrder,            // sortOrder
		operator,                       // operator
		"",                             // resetSchedule (空表示不重置)
		challenge.RewardConfig,         // metadata
		challenge.Name,                 // title
		challenge.Description,          // description
		challenge.Category,             // category
		int(challenge.StartTimeUnix),   // startTime
		int(challenge.EndTimeUnix),     // endTime
		int(challenge.DurationSeconds), // duration
		challenge.MaxPart,              // maxSize
		1000000,                        // maxNumScore
		false,                          // joinRequired
		true,                           // enableRanks
	)

	return err
}

// GetActiveChallenges 获取指定活动的所有活跃挑战赛
func (cs *ChallengeService) GetActiveChallenges(activityID string) []template.TplChallenge {
	return cs.templateManager.GetTplChallenge().FindActiveByActivityID(activityID)
}

// GetChallengeByID 根据ID获取挑战赛
func (cs *ChallengeService) GetChallengeByID(challengeID string) (template.TplChallenge, bool) {
	return cs.templateManager.GetTplChallenge().FindByKey(challengeID)
}

// CanUserJoinChallenge 检查用户是否可以参与挑战赛
func (cs *ChallengeService) CanUserJoinChallenge(challenge template.TplChallenge, userLevel int, isVIP bool) bool {
	// 检查挑战赛是否可以参与
	if !challenge.CanJoin() {
		return false
	}

	// 解析参与要求
	if challenge.Requirements == "" {
		return true
	}

	var requirements map[string]interface{}
	if err := json.Unmarshal([]byte(challenge.Requirements), &requirements); err != nil {
		cs.logger.Error("解析挑战赛参与要求失败",
			zap.String("challenge_id", challenge.ID),
			zap.Error(err))
		return false
	}

	// 检查等级要求
	if levelReq, exists := requirements["level"]; exists {
		if level, ok := levelReq.(float64); ok && userLevel < int(level) {
			return false
		}
	}

	// 检查VIP要求
	if vipReq, exists := requirements["vip"]; exists {
		if vip, ok := vipReq.(bool); ok && vip && !isVIP {
			return false
		}
	}

	return true
}

// ProcessChallengeRewards 处理挑战赛奖励
func (cs *ChallengeService) ProcessChallengeRewards(challengeID string, userRank int) (map[string]interface{}, error) {
	challenge, found := cs.GetChallengeByID(challengeID)
	if !found {
		return nil, fmt.Errorf("挑战赛不存在: %s", challengeID)
	}

	if challenge.RewardConfig == "" {
		return nil, fmt.Errorf("挑战赛没有配置奖励")
	}

	var rewards map[string]interface{}
	if err := json.Unmarshal([]byte(challenge.RewardConfig), &rewards); err != nil {
		return nil, fmt.Errorf("解析奖励配置失败: %v", err)
	}

	// 根据排名获取奖励
	var userReward map[string]interface{}

	// 尝试精确匹配排名
	if reward, exists := rewards[strconv.Itoa(userRank)]; exists {
		if r, ok := reward.(map[string]interface{}); ok {
			userReward = r
		}
	}

	// 尝试匹配排名范围奖励
	if userReward == nil {
		if userRank == 1 {
			if reward, exists := rewards["first"]; exists {
				if r, ok := reward.(map[string]interface{}); ok {
					userReward = r
				}
			}
		} else if userRank == 2 {
			if reward, exists := rewards["second"]; exists {
				if r, ok := reward.(map[string]interface{}); ok {
					userReward = r
				}
			}
		} else if userRank == 3 {
			if reward, exists := rewards["third"]; exists {
				if r, ok := reward.(map[string]interface{}); ok {
					userReward = r
				}
			}
		}
	}

	// 检查完成奖励
	if userReward == nil {
		if reward, exists := rewards["completion"]; exists {
			if r, ok := reward.(map[string]interface{}); ok {
				userReward = r
			}
		}
	}

	return userReward, nil
}

// 定时任务：检查挑战赛状态
func (cs *ChallengeService) CheckChallengeStatus(ctx context.Context) {
	challenges := cs.templateManager.GetTplChallenge().FindAll()

	for _, challenge := range challenges {
		if challenge.Status == 1 {
			remainingTime := challenge.GetRemainingTime()

			if remainingTime <= 0 && challenge.EndTimeUnix <= time.Now().Unix() {
				// 挑战赛已结束，处理结算
				cs.logger.Info("挑战赛已结束，开始结算",
					zap.String("challenge_id", challenge.ID),
					zap.String("name", challenge.Name))

				// 这里可以添加结算逻辑
				// 例如：发放奖励、更新状态等
			} else if remainingTime <= 3600 { // 1小时内结束
				cs.logger.Info("挑战赛即将结束",
					zap.String("challenge_id", challenge.ID),
					zap.String("name", challenge.Name),
					zap.Int64("remaining_seconds", remainingTime))
			}
		}
	}
}

// RPC函数示例：获取用户可参与的挑战赛
func GetUserChallenges(ctx context.Context, logger *zap.Logger, db *sql.DB,
	nk runtime.NakamaModule, payload string) (string, error) {

	// 解析请求参数
	var request struct {
		ActivityID string `json:"activity_id"`
		UserLevel  int    `json:"user_level"`
		IsVIP      bool   `json:"is_vip"`
	}

	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		return "", fmt.Errorf("解析请求参数失败: %v", err)
	}

	// 这里需要获取templateManager实例
	// 在实际项目中，应该通过依赖注入或全局变量获取
	// templateManager := GetTemplateManager()

	// 获取活动的挑战赛
	// challenges := templateManager.GetTplChallenge().FindActiveByActivityID(request.ActivityID)

	// 过滤用户可参与的挑战赛
	var availableChallenges []template.TplChallenge
	// for _, challenge := range challenges {
	//     if CanUserJoinChallenge(challenge, request.UserLevel, request.IsVIP) {
	//         availableChallenges = append(availableChallenges, challenge)
	//     }
	// }

	// 返回结果
	response := map[string]interface{}{
		"challenges": availableChallenges,
		"count":      len(availableChallenges),
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("序列化响应失败: %v", err)
	}

	return string(responseBytes), nil
}

// RPC函数示例：提交挑战赛分数
func SubmitChallengeScore(ctx context.Context, logger *zap.Logger, db *sql.DB,
	nk runtime.NakamaModule, payload string) (string, error) {

	// 解析请求参数
	var request struct {
		ChallengeID string `json:"challenge_id"`
		Score       int64  `json:"score"`
		Subscore    int64  `json:"subscore"`
		Metadata    string `json:"metadata"`
	}

	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		return "", fmt.Errorf("解析请求参数失败: %v", err)
	}

	// 获取用户信息
	userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	username := ctx.Value(runtime.RUNTIME_CTX_USERNAME).(string)

	// 解析metadata
	var metadata map[string]interface{}
	if request.Metadata != "" {
		if err := json.Unmarshal([]byte(request.Metadata), &metadata); err != nil {
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	// 提交分数到Tournament
	record, err := nk.TournamentRecordWrite(ctx, request.ChallengeID, userID, username,
		request.Score, request.Subscore, metadata, nil)
	if err != nil {
		return "", fmt.Errorf("提交分数失败: %v", err)
	}

	// 返回结果
	response := map[string]interface{}{
		"success": true,
		"record":  record,
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("序列化响应失败: %v", err)
	}

	return string(responseBytes), nil
}
