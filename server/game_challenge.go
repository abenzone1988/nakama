package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 旧锁已移除：使用数据库事务与顾问锁保证多节点安全
const MaxJoinRetry = 3

// ChallengeStatus 挑战赛状态
type ChallengeStatus struct {
	ID              int32     `json:"id"`
	TournamentID    string    `json:"tournament_id"`
	ActivityID      string    `json:"activity_id"`
	Joined          time.Time `json:"joined"`
	RankReward      bool      `json:"rank_reward"`       // 排名奖励是否已领取
	LowScoreReward  bool      `json:"low_score_reward"`  // 低分奖励是否已领取
	MidScoreReward  bool      `json:"mid_score_reward"`  // 中分奖励是否已领取
	HighScoreReward bool      `json:"high_score_reward"` // 高分奖励是否已领取
}

// ScoreRewardResult 积分奖励检查结果
type ScoreRewardResult struct {
	RewardIds    []string `json:"reward_ids"`
	RewardTypes  []string `json:"reward_types"` // "low", "mid", "high"
	HasNewReward bool     `json:"has_new_reward"`
}
type UserMatch struct {
	Challenges   map[int32]*ChallengeStatus `json:"join_challenges"`
	TopThreeData *TopThreeStats             `json:"top_three_data,omitempty"` // 前三名统计数据
}

func (f *UserMatch) GetCollection() string {
	return "user_data"
}

func (f *UserMatch) GetKey() string {
	return "match"
}

func (f *UserMatch) Init() {
	f.Challenges = map[int32]*ChallengeStatus{}
	f.TopThreeData = &TopThreeStats{
		FirstPlaceTournaments:  make(map[string]bool),
		SecondPlaceTournaments: make(map[string]bool),
		ThirdPlaceTournaments:  make(map[string]bool),
		LastUpdated:            0,
	}
}

// TopThreeStats 前三名统计数据
type TopThreeStats struct {
	FirstPlaceTournaments  map[string]bool `json:"first_place_tournaments"`  // 获得第一名的竞标赛ID集合
	SecondPlaceTournaments map[string]bool `json:"second_place_tournaments"` // 获得第二名的竞标赛ID集合
	ThirdPlaceTournaments  map[string]bool `json:"third_place_tournaments"`  // 获得第三名的竞标赛ID集合
	LastUpdated            int64           `json:"last_updated"`             // 最后更新时间戳，用于数据版本控制
}

// GetFirstPlaceCount 获取第一名次数
func (t *TopThreeStats) GetFirstPlaceCount() int32 {
	return int32(len(t.FirstPlaceTournaments))
}

// GetSecondPlaceCount 获取第二名次数
func (t *TopThreeStats) GetSecondPlaceCount() int32 {
	return int32(len(t.SecondPlaceTournaments))
}

// GetThirdPlaceCount 获取第三名次数
func (t *TopThreeStats) GetThirdPlaceCount() int32 {
	return int32(len(t.ThirdPlaceTournaments))
}

func (s *ApiServer) GetChallenge(ctx context.Context, in *emptypb.Empty) (*game.GetChallengeResponse, error) {
	// 获取用户ID
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	userMatch := &UserMatch{}
	err := LoadData(ctx, s.logger, s.db, userID, userMatch)
	if err != nil {
		return nil, err
	}

	tplChallenges := s.template.GetTplChallenge().FindAll()
	if tplChallenges.Len() == 0 {
		return &game.GetChallengeResponse{
			Challenges: []*game.Challenge{},
		}, nil
	}

	// 获取当前时间
	now := time.Now()

	// 首先解析所有挑战赛的时间信息
	type challengeInfo struct {
		TplChallenge *template.TplChallenge
		StartTime    time.Time
		EndTime      time.Time
		CloseTime    time.Time
		IsOngoing    bool
		IsFuture     bool
	}

	var allChallenges []challengeInfo
	for i := 0; i < tplChallenges.Len(); i++ {
		tplChallenge := tplChallenges.Get(i)
		// 安全检查：确保模板数据有效
		if tplChallenge.ID == 0 {
			s.logger.Warn("跳过无效的挑战赛模板", zap.String("reason", "ID为0"))
			continue
		}

		// 解析开始时间
		startTime, err := parseDateTime(tplChallenge.OpenTime)
		if err != nil {
			s.logger.Error("解析挑战赛开始时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("OpenTime", tplChallenge.OpenTime),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		// 解析关闭时间
		closeTime, err := parseDateTime(tplChallenge.CloseTime)
		if err != nil {
			s.logger.Error("解析挑战赛关闭时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("CloseTime", tplChallenge.CloseTime),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		// 解析结束时间
		endTime, err := parseDateTime(tplChallenge.EndTime)
		if err != nil {
			s.logger.Error("解析挑战赛结束时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("end_date", tplChallenge.EndTime),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		// 时间逻辑验证
		if startTime.After(endTime) {
			s.logger.Warn("挑战赛时间配置错误：开始时间晚于结束时间",
				zap.Int32("id", tplChallenge.ID),
				zap.Time("start_time", startTime),
				zap.Time("end_time", endTime))
			continue
		}

		// 判断挑战赛状态
		isOngoing := (now.After(startTime) || now.Equal(startTime)) && now.Before(endTime)
		isFuture := startTime.After(now)

		allChallenges = append(allChallenges, challengeInfo{
			TplChallenge: &tplChallenge,
			StartTime:    startTime,
			EndTime:      endTime,
			CloseTime:    closeTime,
			IsOngoing:    isOngoing,
			IsFuture:     isFuture,
		})
	}

	// 筛选要显示的挑战赛：正在进行的比赛 + 下一场比赛
	var selectedChallenges []challengeInfo

	// 1. 添加所有正在进行的比赛
	for _, challenge := range allChallenges {
		if challenge.IsOngoing {
			selectedChallenges = append(selectedChallenges, challenge)
		}
	}

	// 2. 找到未来三场比赛（按开始时间排序）
	var futureChallenges []challengeInfo
	for _, challenge := range allChallenges {
		if challenge.IsFuture {
			futureChallenges = append(futureChallenges, challenge)
		}
	}

	// 按开始时间排序，取前三个
	if len(futureChallenges) > 0 {
		// 按开始时间排序
		sort.Slice(futureChallenges, func(i, j int) bool {
			return futureChallenges[i].StartTime.Before(futureChallenges[j].StartTime)
		})

		// 取前三个未来比赛
		maxFutureChallenges := 3
		if len(futureChallenges) < maxFutureChallenges {
			maxFutureChallenges = len(futureChallenges)
		}

		for i := 0; i < maxFutureChallenges; i++ {
			selectedChallenges = append(selectedChallenges, futureChallenges[i])
		}
	}

	// 转换为 game.Challenge 数组
	challenges := make([]*game.Challenge, 0, len(selectedChallenges))
	for _, challengeInfo := range selectedChallenges {
		tplChallenge := challengeInfo.TplChallenge

		// 获取用户挑战赛状态中的竞标赛ID
		var tournamentID string
		if challengeData, exists := userMatch.Challenges[tplChallenge.ID]; exists && challengeData != nil {
			tournamentID = challengeData.TournamentID
		}

		challenge := &game.Challenge{
			Id:           tplChallenge.ID,
			ActivityId:   tplChallenge.ActivityID,
			Open:         timestamppb.New(challengeInfo.StartTime),
			Close:        timestamppb.New(challengeInfo.CloseTime),
			End:          timestamppb.New(challengeInfo.EndTime),
			Over:         timestamppb.New(challengeInfo.EndTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)),
			MaxPart:      tplChallenge.MaxPart,
			TournamentId: tournamentID,
		}

		challenges = append(challenges, challenge)
	}

	// 转换用户已参与的挑战赛状态为JoinChallengeStatus数组
	joinedChallenges := make([]*game.JoinChallengeStatus, 0, len(userMatch.Challenges))
	for _, challengeStatus := range userMatch.Challenges {

		tplChallenge, found := s.template.GetTplChallenge().FindByKey(challengeStatus.ID)
		if !found {
			s.logger.Error("模板挑战赛不存在", zap.Int32("challenge_id", challengeStatus.ID))
			continue
		}

		// 从模板表中读取时间信息
		openTime, err := parseDateTime(tplChallenge.OpenTime)
		if err != nil {
			s.logger.Error("解析挑战赛开始时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		closeTime, err := parseDateTime(tplChallenge.CloseTime)
		if err != nil {
			s.logger.Error("解析挑战赛关闭时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		endTime, err := parseDateTime(tplChallenge.EndTime)
		if err != nil {
			s.logger.Error("解析挑战赛结束时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		overTime := endTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

		if overTime.Before(now) { //已经结束的比赛不再显示
			continue
		}

		joinedStatus := &game.JoinChallengeStatus{
			Id:              challengeStatus.ID,
			TournamentId:    challengeStatus.TournamentID,
			ActivityId:      challengeStatus.ActivityID,
			Joined:          timestamppb.New(challengeStatus.Joined),
			Open:            timestamppb.New(openTime),
			Close:           timestamppb.New(closeTime),
			End:             timestamppb.New(endTime),
			Over:            timestamppb.New(overTime),
			RankReward:      challengeStatus.RankReward,
			LowScoreReward:  challengeStatus.LowScoreReward,
			MidScoreReward:  challengeStatus.MidScoreReward,
			HighScoreReward: challengeStatus.HighScoreReward,
		}
		joinedChallenges = append(joinedChallenges, joinedStatus)
	}
	err, getReward := s.checkAndSendExpiredChallengeRewardsForGetAccount(ctx)
	if err != nil {
		s.logger.Error("检查过期挑战赛奖励失败", zap.Error(err))
	}
	return &game.GetChallengeResponse{
		Challenges: challenges,
		Joined:     joinedChallenges,
		GetReward:  getReward,
	}, nil
}

func (s *ApiServer) JoinChallenge(ctx context.Context, in *game.JoinChallengeRequest) (*game.JoinChallengeResponse, error) {
	tplChallenge, found := s.template.GetTplChallenge().FindByKey(in.ChallengeId)
	if !found {
		return &game.JoinChallengeResponse{
			Code: 1,
			Msg:  "挑战赛Id不存在",
		}, nil
	}

	// 获取用户ID
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	userMatch := &UserMatch{}
	err := LoadData(ctx, s.logger, s.db, userID, userMatch)
	if err != nil {
		return nil, err
	}

	// 确保Challenges map已初始化
	if userMatch.Challenges == nil {
		userMatch.Challenges = map[int32]*ChallengeStatus{}
	}

	//if challengeData, exists := userMatch.Challenges[in.ChallengeId]; exists && challengeData != nil {
	//	return &game.JoinChallengeResponse{
	//		Code: 2,
	//		Msg:  "挑战赛不可重复参加",
	//	}, nil
	//}

	// 获取当前时间
	now := time.Now()

	// 解析开始时间
	startTime, err := parseDateTime(tplChallenge.OpenTime)
	if err != nil {
		s.logger.Error("解析挑战赛开始时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("start_time", tplChallenge.OpenTime),
			zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
			zap.Error(err))
		return &game.JoinChallengeResponse{
			Code: 3,
			Msg:  "配置错误",
		}, nil
	}

	// 解析开始时间
	closeTime, err := parseDateTime(tplChallenge.CloseTime)
	if err != nil {
		s.logger.Error("解析挑战赛关闭时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("close_time", tplChallenge.CloseTime),
			zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
			zap.Error(err))
		return &game.JoinChallengeResponse{
			Code: 3,
			Msg:  "配置错误",
		}, nil
	}

	// 解析结束时间
	endTime, err := parseDateTime(tplChallenge.EndTime)
	if err != nil {
		s.logger.Error("解析挑战赛结束时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("end_time", tplChallenge.EndTime),
			zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
			zap.Error(err))
		return &game.JoinChallengeResponse{
			Code: 3,
			Msg:  "配置错误",
		}, nil
	}

	// 时间过滤逻辑：只允许参加正在进行的比赛
	isOngoing := (now.After(startTime) || now.Equal(startTime)) && now.Before(endTime)
	if !isOngoing {
		return &game.JoinChallengeResponse{
			Code: 4,
			Msg:  "挑战赛未开放或已结束",
		}, nil
	}

	overTime := endTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

	// 获取或分配玩家的竞标赛ID
	tournamentID, err := s.matchPlayerToChallengeTournament(ctx, userID, &tplChallenge, startTime, endTime)
	if err != nil || tournamentID == "" {
		s.logger.Error("加入挑战赛失败",
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.String("user_id", userID.String()),
			zap.Error(err))

		return &game.JoinChallengeResponse{
			Code: 5,
			Msg:  fmt.Sprintf("无法加入挑战赛: %v", err),
		}, nil
	}

	userMatch.Challenges[tplChallenge.ID] = &ChallengeStatus{
		ID:              tplChallenge.ID,
		ActivityID:      tplChallenge.ActivityID,
		TournamentID:    tournamentID,
		Joined:          now.Truncate(time.Second),
		HighScoreReward: false,
		LowScoreReward:  false,
		MidScoreReward:  false,
		RankReward:      false,
	}

	//保存更新后的比赛数据
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
	if err != nil {
		return nil, err
	}

	return &game.JoinChallengeResponse{
		Code: 0,
		Challenge: &game.Challenge{
			Id:           tplChallenge.ID,
			ActivityId:   tplChallenge.ActivityID,
			Open:         timestamppb.New(startTime),
			End:          timestamppb.New(endTime),
			Close:        timestamppb.New(closeTime),
			Over:         timestamppb.New(overTime),
			MaxPart:      tplChallenge.MaxPart,
			TournamentId: tournamentID,
		},
	}, nil

}

func (s *ApiServer) GainChallengeReward(ctx context.Context, in *game.GainChallengeRewardRequest) (*game.GainChallengeRewardResponse, error) {
	// 获取用户ID
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	userMatch := &UserMatch{}
	err := LoadData(ctx, s.logger, s.db, userID, userMatch)
	if err != nil {
		return nil, err
	}

	challengeData, exists := userMatch.Challenges[in.ChallengeId]
	if !exists || challengeData == nil {
		return &game.GainChallengeRewardResponse{
			Code: 1,
			Msg:  "没有参加挑战赛",
		}, nil
	}

	tplChallenge, found := s.template.GetTplChallenge().FindByKey(in.ChallengeId)
	if !found {
		return nil, status.Error(codes.InvalidArgument, "挑战赛不存在")
	}

	tournamentID := userMatch.Challenges[in.ChallengeId].TournamentID
	records, err := TournamentRecordsList(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, tournamentID, []string{userID.String()}, nil, "", 0)
	if err != nil || len(records.OwnerRecords) == 0 {
		return &game.GainChallengeRewardResponse{
			Code: 5,
			Msg:  "没有提交成绩",
		}, nil
	}

	ownerRecord := records.OwnerRecords[0]
	activityInfo, found := s.template.GetTplActivityInfo().FindByKey(tplChallenge.ActivityID)
	if !found {
		return nil, status.Error(codes.InvalidArgument, "活动不存在")
	}

	var rewards []*game.Reward
	var rewardIds []string
	var rewardTypes []string

	if in.RankReward {
		// 处理排名奖励
		if challengeData.RankReward {
			return &game.GainChallengeRewardResponse{
				Code: 7,
				Msg:  "排名奖励已领取",
			}, nil
		}

		// 获取排名奖励配置
		acRewards := s.template.GetTplActivityReward().FindByFilter(func(acReward template.TplActivityReward) bool {
			return acReward.ActiveID == activityInfo.RewardGroupID
		})

		rewardId := ""
		for i := 0; i < acRewards.Len(); i++ {
			acReward := acRewards.Get(i)
			if int64(acReward.MinRank) <= ownerRecord.Rank && int64(acReward.MaxRank) >= ownerRecord.Rank {
				rewardId = acReward.RewardID
				break
			}
		}

		if rewardId == "" {
			return &game.GainChallengeRewardResponse{
				Code: 8,
				Msg:  "排名奖励不存在",
			}, nil
		}

		rewardIds = []string{rewardId}
		rewardTypes = []string{"rank"}

		// 解析排名奖励
		rewards, err = ParseRewards(s.logger, s.template.GetTplReward(), rewardIds)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("解析排名奖励失败: %v", err))
		}

		// 设置排名奖励为已领取
		challengeData.RankReward = true

		s.logger.Info("主动领取挑战赛排名奖励",
			zap.Int32("challenge_id", in.ChallengeId),
			zap.String("user_id", userID.String()),
			zap.String("reward_id", rewardId),
			zap.Strings("reward_types", rewardTypes),
			zap.Int32("rank", int32(ownerRecord.Rank)))

		// 更新前三名统计数据
		s.updateTopThreeStats(userID, userMatch, ownerRecord.Rank, tournamentID)

	} else {
		// 处理积分奖励
		scoreResult := s.checkScoreRewards(challengeData, ownerRecord, &activityInfo)
		if !scoreResult.HasNewReward {
			return &game.GainChallengeRewardResponse{
				Code: 6,
				Msg:  "没有可领取的积分奖励",
			}, nil
		}

		// 应用积分奖励状态更新
		s.applyScoreRewards(challengeData, scoreResult)

		rewardIds = scoreResult.RewardIds
		rewardTypes = scoreResult.RewardTypes

		// 解析积分奖励
		rewards, err = ParseRewards(s.logger, s.template.GetTplReward(), rewardIds)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("解析积分奖励失败: %v", err))
		}

		s.logger.Info("主动领取挑战赛积分奖励",
			zap.Int32("challenge_id", in.ChallengeId),
			zap.String("user_id", userID.String()),
			zap.Strings("reward_ids", rewardIds),
			zap.Strings("reward_types", rewardTypes),
			zap.Int32("rank", int32(ownerRecord.Rank)))
	}

	// 保存奖励状态更新
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
	if err != nil {
		return nil, err
	}

	return &game.GainChallengeRewardResponse{
		Code:   0,
		Msg:    "领取奖励成功",
		Reward: rewards,
	}, nil
}

func (s *ApiServer) GetChallengeTopStats(ctx context.Context, in *emptypb.Empty) (*game.GetChallengeTopStatsResponse, error) {
	// 获取用户ID
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	userMatch := &UserMatch{}
	err := LoadData(ctx, s.logger, s.db, userID, userMatch)
	if err != nil {
		return &game.GetChallengeTopStatsResponse{
			Code: 1,
			Msg:  "加载用户数据失败",
		}, nil
	}

	// 初始化前三名统计数据
	err = s.initializeTopThreeStats(ctx, userID, userMatch)
	if err != nil {
		s.logger.Error("初始化前三名统计数据失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.GetChallengeTopStatsResponse{
			Code: 2,
			Msg:  "初始化统计数据失败",
		}, nil
	}

	return &game.GetChallengeTopStatsResponse{
		Code: 0,
		Msg:  "获取成功",
		Stats: &game.TopThreeStats{
			FirstPlace:  userMatch.TopThreeData.GetFirstPlaceCount(),
			SecondPlace: userMatch.TopThreeData.GetSecondPlaceCount(),
			ThirdPlace:  userMatch.TopThreeData.GetThirdPlaceCount(),
			LastUpdated: userMatch.TopThreeData.LastUpdated,
		},
	}, nil
}

// 检查积分等级奖励（公共逻辑）
func (s *ApiServer) checkScoreRewards(challengeData *ChallengeStatus, ownerRecord *api.LeaderboardRecord, activityInfo *template.TplActivityInfo) *ScoreRewardResult {
	result := &ScoreRewardResult{
		RewardIds:   []string{},
		RewardTypes: []string{},
	}

	if ownerRecord.Score >= int64(activityInfo.Condition01) && !challengeData.LowScoreReward {
		result.RewardIds = append(result.RewardIds, activityInfo.Reward01)
		result.RewardTypes = append(result.RewardTypes, "low")
		result.HasNewReward = true
	}
	if ownerRecord.Score >= int64(activityInfo.Condition02) && !challengeData.MidScoreReward {
		result.RewardIds = append(result.RewardIds, activityInfo.Reward02)
		result.RewardTypes = append(result.RewardTypes, "mid")
		result.HasNewReward = true
	}
	if ownerRecord.Score >= int64(activityInfo.Condition03) && !challengeData.HighScoreReward {
		result.RewardIds = append(result.RewardIds, activityInfo.Reward03)
		result.RewardTypes = append(result.RewardTypes, "high")
		result.HasNewReward = true
	}

	return result
}

// 应用积分等级奖励（更新状态）
func (s *ApiServer) applyScoreRewards(challengeData *ChallengeStatus, scoreResult *ScoreRewardResult) {
	for _, rewardType := range scoreResult.RewardTypes {
		switch rewardType {
		case "low":
			challengeData.LowScoreReward = true
		case "mid":
			challengeData.MidScoreReward = true
		case "high":
			challengeData.HighScoreReward = true
		}
	}
}

// 检查过期挑战赛奖励
func (s *ApiServer) checkAndSendExpiredChallengeRewardsForGetAccount(ctx context.Context) (error, bool) {
	// 获取用户ID
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 创建并加载UserMatch对象
	userMatch := &UserMatch{}
	err := LoadData(ctx, s.logger, s.db, userID, userMatch)
	if err != nil {
		// 如果加载失败，记录日志但不返回错误，避免影响GetAccount的正常流程
		s.logger.Debug("加载用户挑战赛数据失败", zap.String("user_id", userID.String()), zap.Error(err))
		return nil, false
	}

	// 获取当前时间
	now := time.Now()

	// 调用现有的检查函数
	hasRewardUpdate, err := s.checkAndSendExpiredChallengeRewards(ctx, userID, userMatch, now)
	if err != nil {
		s.logger.Error("检查过期挑战赛奖励失败", zap.String("user_id", userID.String()), zap.Error(err))
		return err, false
	}

	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
	if err != nil {
		s.logger.Error("保存挑战赛状态失败", zap.String("user_id", userID.String()), zap.Error(err))
		return err, false
	}

	return nil, hasRewardUpdate
}

// 检查活动结束后是否有奖励没有领取，没有领取的变成邮件
func (s *ApiServer) checkAndSendExpiredChallengeRewards(ctx context.Context, userID uuid.UUID, userMatch *UserMatch, now time.Time) (bool, error) {
	hasRewardUpdate := false

	for _, challenge := range userMatch.Challenges {
		if challenge.TournamentID != "" {

			if challenge.LowScoreReward && challenge.MidScoreReward && challenge.HighScoreReward && challenge.RankReward {
				continue
			}

			// 获取挑战赛模板信息
			tplChallenge, found := s.template.GetTplChallenge().FindByKey(challenge.ID)
			if !found {
				s.logger.Error("模板挑战赛不存在", zap.Int32("challenge_id", challenge.ID))
				continue
			}

			// 从模板表中读取时间信息
			endTime, err := parseDateTime(tplChallenge.EndTime)
			if err != nil {
				s.logger.Error("解析挑战赛结束时间失败", zap.Int32("id", tplChallenge.ID), zap.Error(err))
				continue
			}

			overTime := endTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

			if now.After(overTime) {
				// 获取用户在竞标赛中的记录
				records, err := TournamentRecordsList(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, challenge.TournamentID, []string{userID.String()}, nil, "", 0)
				if err != nil || len(records.OwnerRecords) == 0 {
					continue
				}
				ownerRecord := records.OwnerRecords[0]

				// 更新前三名统计数据
				hasRewardUpdate = s.updateTopThreeStats(userID, userMatch, ownerRecord.Rank, challenge.TournamentID)

				// 获取活动配置
				activityInfo, found := s.template.GetTplActivityInfo().FindByKey(challenge.ActivityID)
				if !found {
					s.logger.Error("活动配置不存在", zap.String("activity_id", challenge.ActivityID))
					continue
				}

				// 检查排名奖励
				if !challenge.RankReward {
					rankRewardSent, err := s.sendExpiredRankReward(ctx, userID, challenge, ownerRecord, &activityInfo)
					if err != nil {
						s.logger.Error("发送过期排名奖励失败", zap.Error(err))
					} else if rankRewardSent {
						challenge.RankReward = true
						hasRewardUpdate = true
					}
				}

				// 检查积分等级奖励
				scoreResult := s.checkScoreRewards(challenge, ownerRecord, &activityInfo)
				if scoreResult.HasNewReward {
					err := s.sendExpiredScoreRewards(ctx, userID, challenge, scoreResult, &activityInfo)
					if err != nil {
						s.logger.Error("发送过期积分奖励失败", zap.Error(err))
					} else {
						s.applyScoreRewards(challenge, scoreResult)
						hasRewardUpdate = true
					}
				}
			}
		}
	}

	return hasRewardUpdate, nil
}

// 发送过期的排名奖励
func (s *ApiServer) sendExpiredRankReward(ctx context.Context, userID uuid.UUID, challenge *ChallengeStatus, ownerRecord *api.LeaderboardRecord, activityInfo *template.TplActivityInfo) (bool, error) {
	// 获取排名奖励配置
	acRewards := s.template.GetTplActivityReward().FindByFilter(func(acReward template.TplActivityReward) bool {
		return acReward.ActiveID == activityInfo.RewardGroupID
	})

	rewardId := ""
	for i := 0; i < acRewards.Len(); i++ {
		acReward := acRewards.Get(i)
		if int64(acReward.MinRank) <= ownerRecord.Rank && int64(acReward.MaxRank) >= ownerRecord.Rank {
			s.logger.Info("找到过期排名奖励",
				zap.Int32("minRank", acReward.MinRank),
				zap.Int32("maxRank", acReward.MaxRank),
				zap.String("reward_id", acReward.RewardID))
			rewardId = acReward.RewardID
			break
		}
	}

	if rewardId == "" {
		return false, nil
	}

	// 解析奖励
	rewards, err := ParseRewards(s.logger, s.template.GetTplReward(), []string{rewardId})
	if err != nil {
		s.logger.Error("解析排名奖励失败",
			zap.String("reward_id", rewardId),
			zap.Error(err))
		return false, err
	}

	if len(rewards) == 0 {
		s.logger.Error("排名奖励为空", zap.String("reward_id", rewardId))
		return false, nil
	}

	// 从 TournamentID 中解析 batchNumber
	// 格式：challenge_{challengeID}_{startTimestamp}_{batchNumber}
	batchNumber := "0"
	if challenge.TournamentID != "" {
		parts := strings.Split(challenge.TournamentID, "_")
		if len(parts) >= 4 {
			batchNumber = parts[len(parts)-1]
		}
	}

	// 构建新的描述
	description := fmt.Sprintf("亲爱的指挥官：您在\"%s\"挑战赛中获得%s号战区排行榜第%d名，邮件中是您的奖励，请查收。",
		activityInfo.Name, batchNumber, ownerRecord.Rank)
	return s.sendChallengeRewardNotification(ctx, userID, "挑战赛排名奖励", description, rewards)
}

// 发送过期的积分等级奖励
func (s *ApiServer) sendExpiredScoreRewards(ctx context.Context, userID uuid.UUID, challenge *ChallengeStatus, scoreResult *ScoreRewardResult, activityInfo *template.TplActivityInfo) error {
	// 解析奖励
	rewards, err := ParseRewards(s.logger, s.template.GetTplReward(), scoreResult.RewardIds)
	if err != nil {
		s.logger.Error("解析积分奖励失败",
			zap.Strings("reward_ids", scoreResult.RewardIds),
			zap.Error(err))
		return err
	}

	if len(rewards) == 0 {
		s.logger.Error("积分奖励为空", zap.Strings("reward_ids", scoreResult.RewardIds))
		return nil
	}

	// 构建新的描述
	description := fmt.Sprintf("亲爱的指挥官：您在\"%s\"挑战赛中有未领取的个人战绩奖励，现通过邮件发放给您，请查收", activityInfo.Name)
	_, err = s.sendChallengeRewardNotification(ctx, userID, "挑战赛个人战绩奖励", description, rewards)
	return err
}

// 发送挑战赛奖励通知（公共方法）
func (s *ApiServer) sendChallengeRewardNotification(ctx context.Context, userID uuid.UUID, subject string, description string, rewards []*game.Reward) (bool, error) {
	// 组合描述和奖励作为通知内容
	content := map[string]interface{}{
		"description": description,
		"rewards":     rewards,
	}

	contentBytes, err := json.Marshal(content)
	if err != nil {
		s.logger.Error("序列化通知内容失败", zap.Error(err))
		return false, err
	}

	// 创建通知
	notification := &api.Notification{
		Id:         uuid.Must(uuid.NewV4()).String(),
		Subject:    subject,
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
			zap.String("subject", subject),
			zap.Error(err))
		return false, err
	}

	s.logger.Info("成功发送挑战赛奖励通知",
		zap.String("user_id", userID.String()),
		zap.String("subject", subject))

	return true, nil
}

// 执行竞标赛分配的核心逻辑并行
func (s *ApiServer) assignPlayerToChallengeTournament(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {
	// 快路径：优先尝试直接加入已存在的可用竞标赛（无跨节点锁，靠 DB 原子判断 size < max_size）

	for i := 0; i < MaxJoinRetry; i++ {
		tournament, err := ChallengeFindAvailable(ctx, s.logger, s.db, s.leaderboardCache, int(tplChallenge.ID))
		if err != nil {
			s.logger.Error("二次查找可用竞标赛失败",
				zap.String("user_id", userID.String()),
				zap.Int32("challenge_id", tplChallenge.ID),
				zap.Error(err))
			return "", err
		}
		if tournament == nil {
			break
		}

		username := ctx.Value(ctxUsernameKey{}).(string)
		err = TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, tournament.Id)
		if err == nil {
			s.logger.Info("玩家成功加入可用竞标赛",
				zap.String("user_id", userID.String()),
				zap.String("tournament_id", tournament.Id),
				zap.Int32("challenge_id", tplChallenge.ID))
			return tournament.Id, nil
		}

		s.logger.Warn("加入可用竞标赛失败，重试",
			zap.String("tournament_id", tournament.Id),
			zap.String("user_id", userID.String()),
			zap.Int("retry", i+1),
			zap.Error(err))
		// 抖动后重试
		time.Sleep(time.Duration(10+rand.Intn(90)) * time.Millisecond)
	}

	// 慢路径：创建新竞标赛（跨节点串行化）封装到 core 层
	newTournamentID, err := ChallengeCreateAndJoin(
		ctx,
		s.logger,
		s.db,
		s.leaderboardCache,
		tplChallenge.ID,
		tplChallenge.Name,
		startTime,
		endTime,
		tplChallenge.MaxPart,
	)
	if err != nil {
		s.logger.Warn("创建新竞标赛失败",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.Error(err))
		return "", fmt.Errorf("创建新竞标赛失败: %v", err)
	}

	// 若锁内发现已有可用竞标赛，则直接用该 ID；否则用新建的 ID
	username := ctx.Value(ctxUsernameKey{}).(string)
	// 加入动作增加小延迟重试，以等待广播同步缓存
	var joinErr error
	for attempt := 0; attempt < MaxJoinRetry; attempt++ {
		joinErr = TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, newTournamentID)
		if joinErr == nil {
			break
		}
		time.Sleep(time.Duration(10+rand.Intn(90)) * time.Millisecond)
	}
	if joinErr != nil {
		s.logger.Warn("加入竞标赛失败",
			zap.String("user_id", userID.String()),
			zap.String("tournament_id", newTournamentID),
			zap.Error(joinErr))
		return "", fmt.Errorf("加入新竞标赛失败: %v", joinErr)
	}

	s.logger.Info("玩家成功加入竞标赛",
		zap.String("user_id", userID.String()),
		zap.String("tournament_id", newTournamentID),
		zap.Int32("challenge_id", tplChallenge.ID))

	return newTournamentID, nil
}

// 执行竞标赛分配的核心逻辑
func (s *ApiServer) matchPlayerToChallengeTournament(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {
	// 单事务 + 顾问锁 串行化“查找/创建 + 加入”，不使用客户端重试
	username := ctx.Value(ctxUsernameKey{}).(string)
	tid, err := ChallengeJoin(
		ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache,
		userID, username,
		tplChallenge.ID, tplChallenge.Name,
		startTime, endTime,
		tplChallenge.MaxPart,
	)
	if err != nil {
		s.logger.Warn("ChallengeJoin 失败",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.Error(err))
		return "", err
	}
	return tid, nil
}

// 初始化前三名统计数据，首次查询历史记录
func (s *ApiServer) initializeTopThreeStats(ctx context.Context, userID uuid.UUID, userMatch *UserMatch) error {
	// 如果已经初始化过，跳过
	if userMatch.TopThreeData != nil && userMatch.TopThreeData.LastUpdated > 0 {
		return nil
	}

	// 确保 TopThreeData 已初始化
	if userMatch.TopThreeData == nil {
		userMatch.TopThreeData = &TopThreeStats{
			FirstPlaceTournaments:  make(map[string]bool),
			SecondPlaceTournaments: make(map[string]bool),
			ThirdPlaceTournaments:  make(map[string]bool),
			LastUpdated:            0,
		}
	}

	// 通过循环查找 userMatch 中参加挑战赛的ID，查询用户的排名成绩
	for challengeID, challenge := range userMatch.Challenges {
		if challenge == nil || challenge.TournamentID == "" {
			s.logger.Debug("跳过无效的挑战赛记录", zap.Int32("challenge_id", challengeID))
			continue
		}

		// 获取该竞标赛的完整排名信息
		records, err := TournamentRecordsList(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, challenge.TournamentID, []string{userID.String()}, nil, "", 0)
		if err != nil || len(records.OwnerRecords) == 0 {
			s.logger.Debug("获取竞标赛排名失败",
				zap.Int32("challenge_id", challengeID),
				zap.String("tournament_id", challenge.TournamentID),
				zap.Error(err))
			continue
		}

		rank := records.OwnerRecords[0].Rank

		// 统计前三名
		switch rank {
		case 1:
			userMatch.TopThreeData.FirstPlaceTournaments[challenge.TournamentID] = true
			s.logger.Debug("用户在该竞标赛中获得第一名",
				zap.String("tournament_id", challenge.TournamentID),
				zap.Int32("challenge_id", challengeID))
		case 2:
			userMatch.TopThreeData.SecondPlaceTournaments[challenge.TournamentID] = true
			s.logger.Debug("用户在该竞标赛中获得第二名",
				zap.String("tournament_id", challenge.TournamentID),
				zap.Int32("challenge_id", challengeID))
		case 3:
			userMatch.TopThreeData.ThirdPlaceTournaments[challenge.TournamentID] = true
			s.logger.Debug("用户在该竞标赛中获得第三名",
				zap.String("tournament_id", challenge.TournamentID),
				zap.Int32("challenge_id", challengeID))
		}

		s.logger.Debug("处理挑战赛记录",
			zap.Int32("challenge_id", challengeID),
			zap.String("tournament_id", challenge.TournamentID),
			zap.Int64("rank", rank))
	}

	userMatch.TopThreeData.LastUpdated = time.Now().Unix()

	s.logger.Info("完成前三名统计数据初始化",
		zap.String("user_id", userID.String()),
		zap.Int32("first_place", userMatch.TopThreeData.GetFirstPlaceCount()),
		zap.Int32("second_place", userMatch.TopThreeData.GetSecondPlaceCount()),
		zap.Int32("third_place", userMatch.TopThreeData.GetThirdPlaceCount()))

	err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
	if err != nil {
		s.logger.Error("保存用户统计数据失败", zap.String("user_id", userID.String()), zap.Error(err))
		// 不影响返回结果，继续执行
	}

	return nil
}

// 更新前三名统计数据（在比赛结束时调用）
func (s *ApiServer) updateTopThreeStats(userID uuid.UUID, userMatch *UserMatch, rank int64, tournamentID string) bool {
	// 确保 TopThreeData 已初始化
	if userMatch.TopThreeData == nil {
		userMatch.TopThreeData = &TopThreeStats{
			FirstPlaceTournaments:  make(map[string]bool),
			SecondPlaceTournaments: make(map[string]bool),
			ThirdPlaceTournaments:  make(map[string]bool),
			LastUpdated:            0,
		}
	}

	// 如果是前三名，更新统计
	var updated bool
	switch rank {
	case 1:
		if !userMatch.TopThreeData.FirstPlaceTournaments[tournamentID] {
			userMatch.TopThreeData.FirstPlaceTournaments[tournamentID] = true
			updated = true
			s.logger.Info("用户获得第一名，更新统计", zap.String("user_id", userID.String()), zap.String("tournament_id", tournamentID), zap.Int32("total_first", userMatch.TopThreeData.GetFirstPlaceCount()))
		}
	case 2:
		if !userMatch.TopThreeData.SecondPlaceTournaments[tournamentID] {
			userMatch.TopThreeData.SecondPlaceTournaments[tournamentID] = true
			updated = true
			s.logger.Info("用户获得第二名，更新统计", zap.String("user_id", userID.String()), zap.String("tournament_id", tournamentID), zap.Int32("total_second", userMatch.TopThreeData.GetSecondPlaceCount()))
		}
	case 3:
		if !userMatch.TopThreeData.ThirdPlaceTournaments[tournamentID] {
			userMatch.TopThreeData.ThirdPlaceTournaments[tournamentID] = true
			updated = true
			s.logger.Info("用户获得第三名，更新统计", zap.String("user_id", userID.String()), zap.String("tournament_id", tournamentID), zap.Int32("total_third", userMatch.TopThreeData.GetThirdPlaceCount()))
		}
	}

	if updated {
		userMatch.TopThreeData.LastUpdated = time.Now().Unix()
	}

	return updated
}
