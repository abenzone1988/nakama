package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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

// ChallengeTournamentMetadata 挑战赛竞标赛元数据
type ChallengeTournamentMetadata struct {
	ChallengeID     int32  `json:"challenge_id"`     // 所属挑战赛ID
	ChallengeType   string `json:"challenge_type"`   // 挑战赛类型
	CreatedBy       string `json:"created_by"`       // 创建者（固定为"server"）
	BatchNumber     int    `json:"batch_number"`     // 批次编号（同一时间段内的第几个竞标赛）
	StartTimestamp  int64  `json:"start_timestamp"`  // 开始时间戳
	EndTimestamp    int64  `json:"end_timestamp"`    // 结束时间戳 结束之后无法提交成绩-可以手动领取奖励
	CloseTimestamp  int64  `json:"close_timestamp"`  // 关闭时间戳 关闭之后没有领奖的-变成邮件发送
	MaxParticipants int32  `json:"max_participants"` // 最大参与人数
}

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

// ChallengeRewards 擂台赛奖励凭证
type ChallengeRewards struct {
	Rewards      map[string]int32            `json:"rewards"`       // key: 商品ID(如"60000"), value: 历史总数量
	DailyRewards map[string]map[string]int32 `json:"daily_rewards"` // key1: 商品ID, key2: 日期(YYYY-MM-DD), value: 当日数量
}

func (f *ChallengeRewards) GetCollection() string {
	return "challenge"
}

func (f *ChallengeRewards) GetKey() string {
	return "rewards"
}

func (f *ChallengeRewards) Init() {
	f.Rewards = make(map[string]int32)
	f.DailyRewards = make(map[string]map[string]int32)
}

// GetTodayCount 获取今天的奖励数量
func (f *ChallengeRewards) GetTodayCount(itemID string) int32 {
	today := time.Now().Format("2006-01-02")
	if dailyMap, exists := f.DailyRewards[itemID]; exists {
		if count, ok := dailyMap[today]; ok {
			return count
		}
	}
	return 0
}

// GetTotalCount 获取历史总数量
func (f *ChallengeRewards) GetTotalCount(itemID string) int32 {
	if count, exists := f.Rewards[itemID]; exists {
		return count
	}
	return 0
}

// AddReward 添加奖励
func (f *ChallengeRewards) AddReward(itemID string, count int32) {
	// 累加历史总数
	f.Rewards[itemID] += count

	// 累加今日数量
	today := time.Now().Format("2006-01-02")
	if f.DailyRewards[itemID] == nil {
		f.DailyRewards[itemID] = make(map[string]int32)
	}
	f.DailyRewards[itemID][today] += count
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

// challengeTimeWindow 挑战赛在某一轮中的时间窗口。
type challengeTimeWindow struct {
	Open  time.Time // 开始时间
	Close time.Time // 关闭时间
	End   time.Time // 结束时间
}

// annualTime 将模板时间（忽略其年份）落到指定年份的同一“月-日 时:分”。
func annualTime(t time.Time, year int) time.Time {
	return time.Date(year, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
}

// parseChallengeTimes 解析挑战赛模板时间，并忽略模板中配置的年份。
// 挑战赛表按年循环使用，因此以 ref（通常是 now 或玩家加入时间）为基准，
// 返回与 ref 对应的一轮时间窗口；跨年赛程（如 12-31 开始、1-1 结束）会落到下一年。
func parseChallengeTimes(tplChallenge template.TplChallenge, ref time.Time) (challengeTimeWindow, error) {
	openTpl, err := parseDateTime(tplChallenge.OpenTime)
	if err != nil {
		return challengeTimeWindow{}, fmt.Errorf("解析开始时间失败: %w", err)
	}
	closeTpl, err := parseDateTime(tplChallenge.CloseTime)
	if err != nil {
		return challengeTimeWindow{}, fmt.Errorf("解析关闭时间失败: %w", err)
	}
	endTpl, err := parseDateTime(tplChallenge.EndTime)
	if err != nil {
		return challengeTimeWindow{}, fmt.Errorf("解析结束时间失败: %w", err)
	}

	open := annualTime(openTpl, ref.Year())
	end := annualTime(endTpl, ref.Year())
	// 结束时间早于开始时间说明赛程跨年（如 12-31 开始、1-1 结束）。
	if !end.After(open) {
		end = end.AddDate(1, 0, 0)
	}

	// ref 已在本轮结束之后，说明这一轮过期了，改用下一年的一轮。
	if !ref.Before(end) {
		open = open.AddDate(1, 0, 0)
		end = end.AddDate(1, 0, 0)
	}

	// 跨年场景：ref 处于上一年开始、今年结束的那一轮中（如 1 月 1 日仍在 12-31 开始的比赛中）。
	prevOpen := open.AddDate(-1, 0, 0)
	prevEnd := end.AddDate(-1, 0, 0)
	if !ref.Before(prevOpen) && ref.Before(prevEnd) {
		open, end = prevOpen, prevEnd
	}

	close := annualTime(closeTpl, open.Year())
	if close.Before(open) {
		close = close.AddDate(1, 0, 0)
	}

	return challengeTimeWindow{Open: open, Close: close, End: end}, nil
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

		// 解析时间：模板年份会被忽略，按“月-日 时:分”循环到当前这一轮。
		window, err := parseChallengeTimes(tplChallenge, now)
		if err != nil {
			s.logger.Error("解析挑战赛时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		// 判断挑战赛状态
		isOngoing := !now.Before(window.Open) && now.Before(window.End)
		isFuture := window.Open.After(now)

		allChallenges = append(allChallenges, challengeInfo{
			TplChallenge: &tplChallenge,
			StartTime:    window.Open,
			EndTime:      window.End,
			CloseTime:    window.Close,
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

		tplChallenge, found := s.template.GetTplChallenge().FindByKey(strconv.Itoa(int(challengeStatus.ID)))
		if !found {
			s.logger.Error("模板挑战赛不存在", zap.Int32("challenge_id", challengeStatus.ID))
			continue
		}

		// 已参加的挑战赛按加入时间定位到对应的那一轮，避免循环到下一年的同日期。
		ref := challengeStatus.Joined
		if ref.IsZero() {
			ref = now
		}
		window, err := parseChallengeTimes(tplChallenge, ref)
		if err != nil {
			s.logger.Error("解析挑战赛时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
				zap.Error(err))
			continue
		}

		overTime := window.End.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

		if overTime.Before(now) { //已经结束的比赛不再显示
			continue
		}

		joinedStatus := &game.JoinChallengeStatus{
			Id:              challengeStatus.ID,
			TournamentId:    challengeStatus.TournamentID,
			ActivityId:      challengeStatus.ActivityID,
			Joined:          timestamppb.New(challengeStatus.Joined),
			Open:            timestamppb.New(window.Open),
			Close:           timestamppb.New(window.Close),
			End:             timestamppb.New(window.End),
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
	tplChallenge, found := s.template.GetTplChallenge().FindByKey(strconv.Itoa(int(in.ChallengeId)))
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

	if challengeData, exists := userMatch.Challenges[in.ChallengeId]; exists && challengeData != nil {
		return &game.JoinChallengeResponse{
			Code: 2,
			Msg:  "挑战赛不可重复参加",
		}, nil
	}

	// 获取当前时间
	now := time.Now()

	// 解析时间：忽略模板年份，按“月-日 时:分”取当前这一轮。
	window, err := parseChallengeTimes(tplChallenge, now)
	if err != nil {
		s.logger.Error("解析挑战赛时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("tplChallenge", fmt.Sprintf("%+v", tplChallenge)),
			zap.Error(err))
		return &game.JoinChallengeResponse{
			Code: 3,
			Msg:  "配置错误",
		}, nil
	}

	// 时间过滤逻辑：只允许参加正在进行的比赛
	isOngoing := !now.Before(window.Open) && now.Before(window.End)
	if !isOngoing {
		return &game.JoinChallengeResponse{
			Code: 4,
			Msg:  "挑战赛未开放或已结束",
		}, nil
	}

	overTime := window.End.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

	// 获取或分配玩家的竞标赛ID
	tournamentID, err := s.assignPlayerToChallengeTournament(ctx, userID, &tplChallenge, window.Open, window.End)
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
			Open:         timestamppb.New(window.Open),
			End:          timestamppb.New(window.End),
			Close:        timestamppb.New(window.Close),
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

	tplChallenge, found := s.template.GetTplChallenge().FindByKey(strconv.Itoa(int(in.ChallengeId)))
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
			tplChallenge, found := s.template.GetTplChallenge().FindByKey(strconv.Itoa(int(challenge.ID)))
			if !found {
				s.logger.Error("模板挑战赛不存在", zap.Int32("challenge_id", challenge.ID))
				continue
			}

			// 按玩家加入时间定位到对应的那一轮（表按年循环，忽略模板年份）。
			ref := challenge.Joined
			if ref.IsZero() {
				ref = now
			}
			window, err := parseChallengeTimes(tplChallenge, ref)
			if err != nil {
				s.logger.Error("解析挑战赛时间失败", zap.Int32("id", tplChallenge.ID), zap.Error(err))
				continue
			}

			overTime := window.End.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

			if now.After(overTime) {
				// 获取用户在竞标赛中的记录
				records, err := TournamentRecordsList(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, challenge.TournamentID, []string{userID.String()}, nil, "", 0)
				if err != nil || len(records.OwnerRecords) == 0 {
					s.logger.Info("没有提交成绩没有奖励", zap.String("user_id", userID.String()), zap.String("tournament_id", challenge.TournamentID))
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
		s.logger.Info("排名奖励不存在", zap.Int32("rank", int32(ownerRecord.Rank)), zap.String("activity_id", challenge.ActivityID))
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

// 为玩家分配到挑战赛竞标赛
func (s *ApiServer) assignPlayerToChallengeTournament(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {

	// 使用优化的数据库查询，直接查找第一个可加入的竞标赛
	// 快速查找第一个可加入的竞标赛，避免遍历所有竞标赛
	tournament, err := TournamentFindFirstAvailable(ctx, s.logger, s.db, s.leaderboardCache,
		int(tplChallenge.ID), int(tplChallenge.MaxPart))
	if err != nil {
		return "", err
	}

	// 找到可用的竞标赛
	if tournament != nil {
		s.logger.Info("找到可用竞标赛",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.String("tournament_id", tournament.Id),
			zap.Uint32("current_players", tournament.Size),
			zap.Uint32("max_players", tournament.MaxSize))

		// 尝试加入该竞标赛
		username := ctx.Value(ctxUsernameKey{}).(string)
		err := TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, tournament.Id)
		if err == nil {
			// 加入成功
			s.logger.Info("玩家成功加入现有竞标赛",
				zap.String("user_id", userID.String()),
				zap.String("tournament_id", tournament.Id),
				zap.Int32("challenge_id", tplChallenge.ID),
				zap.Uint32("tournament_size", tournament.Size),
				zap.Uint32("max_size", tournament.MaxSize))

			return tournament.Id, nil
		} else {
			s.logger.Warn("加入现有竞标赛失败",
				zap.String("tournament_id", tournament.Id),
				zap.Error(err))
		}
	}

	// 没有可用的竞标赛，创建新的
	// 批次号每次从存储读取最新值并立即保存，保证停服/重启后继续递增
	batchNumber, err := s.GetNextChallengeBatch(tplChallenge.ID)
	if err != nil {
		return "", fmt.Errorf("分配竞标赛批次号失败: %v", err)
	}

	s.logger.Info("创建新竞标赛",
		zap.String("user_id", userID.String()),
		zap.Int32("challenge_id", tplChallenge.ID),
		zap.Int32("batch_number", batchNumber),
		zap.String("reason", "没有找到可用的竞标赛"))

	newTournamentID, err := s.createNewChallengeTournament(ctx, tplChallenge, startTime, endTime, int(batchNumber))
	if err != nil {
		return "", fmt.Errorf("创建新竞标赛失败: %v", err)
	}

	// 玩家加入新创建的竞标赛
	username := ctx.Value(ctxUsernameKey{}).(string)
	err = TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, newTournamentID)
	if err != nil {
		return "", fmt.Errorf("加入新竞标赛失败: %v", err)
	}

	s.logger.Info("为玩家创建并加入新竞标赛",
		zap.String("user_id", userID.String()),
		zap.String("tournament_id", newTournamentID),
		zap.Int32("challenge_id", tplChallenge.ID),
		zap.Int32("batch_number", batchNumber))

	return newTournamentID, nil
}

// 创建新的挑战赛竞标赛
func (s *ApiServer) createNewChallengeTournament(ctx context.Context, tplChallenge *template.TplChallenge, startTime, endTime time.Time, batchNumber int) (string, error) {
	// 构造标准化的竞标赛ID
	// 格式: challenge_{challengeID}_{startTimestamp}_{batchNumber}
	// batchNumber 使用持久化的递增序号确保唯一性，不限制位数以支持超过999的batch
	tournamentID := fmt.Sprintf("challenge_%d_%d_%d",
		tplChallenge.ID,
		startTime.Unix(),
		batchNumber)

	// 创建竞标赛元数据
	metadata := ChallengeTournamentMetadata{
		ChallengeID:     tplChallenge.ID,
		ChallengeType:   "server_managed", // 标识为服务器管理的竞标赛
		CreatedBy:       "server",         // 明确标识创建者为服务器
		BatchNumber:     batchNumber,      // 持久化的递增批次号
		StartTimestamp:  startTime.Unix(),
		EndTimestamp:    endTime.Unix(),
		CloseTimestamp:  endTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute).Unix(),
		MaxParticipants: tplChallenge.MaxPart,
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("序列化元数据失败: %v", err)
	}

	// Tournament配置
	operator := LeaderboardOperatorBest
	sortOrder := LeaderboardSortOrderDescending
	duration := int(endTime.Sub(startTime).Seconds())

	// 构造竞标赛标题和描述
	title := fmt.Sprintf("%s - 第%d轮", tplChallenge.Name, batchNumber)
	description := fmt.Sprintf("挑战赛 %s 的竞标赛，由服务器自动创建和管理", tplChallenge.Name)

	// 创建Tournament（服务器作为创建者，无玩家拥有者）
	err = TournamentCreate(ctx, s.logger, s.leaderboardCache, s.leaderboardScheduler, // 使用正确的scheduler
		tournamentID,              // id
		false,                     //非权威模式 玩家可以自主提交
		sortOrder,                 // sortOrder
		operator,                  // operator
		"",                        // resetSchedule (空表示不重置)
		string(metadataBytes),     // metadata - 包含完整的挑战赛信息
		title,                     // title
		description,               // description
		int(tplChallenge.ID),      // category - 修复：使用int类型 使用挑战赛id
		int(startTime.Unix()),     // startTime
		int(endTime.Unix()),       // endTime
		duration,                  // duration
		int(tplChallenge.MaxPart), // maxSize
		1000,                      // maxNumScore - 允许的最大提交次数
		true,                      // joinRequired - 修复：必须设为true以启用maxSize检查
		true,                      // enableRanks - 启用排名
	)

	if err != nil {
		return "", err
	}

	s.logger.Info("成功创建新的挑战赛竞标赛",
		zap.String("tournament_id", tournamentID),
		zap.Int32("challenge_id", tplChallenge.ID),
		zap.String("title", title),
		zap.Int("batch_number", batchNumber),
		zap.Int32("max_participants", tplChallenge.MaxPart),
		zap.String("created_by", "server"))

	return tournamentID, nil
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

	s.logger.Info("开始初始化用户前三名统计数据", zap.String("user_id", userID.String()))

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

// 获取用户前三名统计数据
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

	// 初始化前三名统计数据（如果需要）
	err = s.initializeTopThreeStats(ctx, userID, userMatch)
	if err != nil {
		s.logger.Error("初始化前三名统计数据失败", zap.String("user_id", userID.String()), zap.Error(err))
		return &game.GetChallengeTopStatsResponse{
			Code: 2,
			Msg:  "初始化统计数据失败",
		}, nil
	}

	s.logger.Info("获取前三名统计数据",
		zap.String("user_id", userID.String()),
		zap.Int32("first_place", userMatch.TopThreeData.GetFirstPlaceCount()),
		zap.Int32("second_place", userMatch.TopThreeData.GetSecondPlaceCount()),
		zap.Int32("third_place", userMatch.TopThreeData.GetThirdPlaceCount()))

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
