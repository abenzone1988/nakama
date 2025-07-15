package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	"google.golang.org/protobuf/types/known/wrapperspb"
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

type UserMatch struct {
	Challenges map[int32]*ChallengeStatus `json:"join_challenges"`
}

func (f *UserMatch) GetCollection() string {
	return "user_data"
}

func (f *UserMatch) GetKey() string {
	return "match"
}

func (f *UserMatch) Init() {
	f.Challenges = map[int32]*ChallengeStatus{}
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
	if tplChallenges == nil {
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

		// 解析关闭时间
		closeTime, err := parseDateTime(tplChallenge.CloseTime)
		if err != nil {
			s.logger.Error("解析挑战赛关闭时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("CloseTime", tplChallenge.CloseTime),
				zap.Error(err))
			continue
		}

		// 解析结束时间
		endTime, err := parseDateTime(tplChallenge.EndTime)
		if err != nil {
			s.logger.Error("解析挑战赛结束时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("end_date", tplChallenge.EndTime),
				zap.Error(err))
			continue
		}

		// 判断挑战赛状态 以overtime
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

	// 2. 找到下一场比赛（最早的未来比赛）
	var nextChallenge *challengeInfo
	for _, challenge := range allChallenges {
		if challenge.IsFuture {
			if nextChallenge == nil || challenge.StartTime.Before(nextChallenge.StartTime) {
				nextChallenge = &challenge
			}
		}
	}

	// 添加下一场比赛（如果存在）
	if nextChallenge != nil {
		selectedChallenges = append(selectedChallenges, *nextChallenge)
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
			s.logger.Error("解析挑战赛开始时间失败", zap.Int32("id", tplChallenge.ID), zap.Error(err))
			continue
		}

		closeTime, err := parseDateTime(tplChallenge.CloseTime)
		if err != nil {
			s.logger.Error("解析挑战赛关闭时间失败", zap.Int32("id", tplChallenge.ID), zap.Error(err))
			continue
		}

		endTime, err := parseDateTime(tplChallenge.EndTime)
		if err != nil {
			s.logger.Error("解析挑战赛结束时间失败", zap.Int32("id", tplChallenge.ID), zap.Error(err))
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

	if challengeData, exists := userMatch.Challenges[in.ChallengeId]; exists && challengeData != nil {
		return &game.JoinChallengeResponse{
			Code: 2,
			Msg:  "挑战赛不可重复参加",
		}, nil
	}

	// 获取当前时间
	now := time.Now()

	// 解析开始时间
	startTime, err := parseDateTime(tplChallenge.OpenTime)
	if err != nil {
		s.logger.Error("解析挑战赛开始时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("start_time", tplChallenge.OpenTime),
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
	tournamentID, err := s.getOrAssignPlayerTournament(ctx, userID, &tplChallenge, startTime, endTime, now)
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
			return acReward.ActiveID == challengeData.ActivityID
		})

		rewardId := ""
		for _, acReward := range acRewards {
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

// ScoreRewardResult 积分奖励检查结果
type ScoreRewardResult struct {
	RewardIds    []string `json:"reward_ids"`
	RewardTypes  []string `json:"reward_types"` // "low", "mid", "high"
	HasNewReward bool     `json:"has_new_reward"`
}

// checkScoreRewards 检查积分等级奖励（公共逻辑）
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

// applyScoreRewards 应用积分等级奖励（更新状态）
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

	// 如果有奖励更新，保存数据
	if hasRewardUpdate {
		err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
		if err != nil {
			s.logger.Error("保存挑战赛状态失败", zap.String("user_id", userID.String()), zap.Error(err))
			return err, false
		}
	}

	return nil, hasRewardUpdate
}

// checkAndSendExpiredChallengeRewards 检查活动结束后是否有奖励没有领取，没有领取的变成邮件
func (s *ApiServer) checkAndSendExpiredChallengeRewards(ctx context.Context, userID uuid.UUID, userMatch *UserMatch, now time.Time) (bool, error) {
	hasRewardUpdate := false

	for _, challenge := range userMatch.Challenges {
		if challenge.TournamentID != "" {
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
					s.logger.Info("没有提交成绩没有奖励")
					continue
				}
				ownerRecord := records.OwnerRecords[0]

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

// sendExpiredRankReward 发送过期的排名奖励
func (s *ApiServer) sendExpiredRankReward(ctx context.Context, userID uuid.UUID, challenge *ChallengeStatus, ownerRecord *api.LeaderboardRecord, activityInfo *template.TplActivityInfo) (bool, error) {
	// 获取排名奖励配置
	acRewards := s.template.GetTplActivityReward().FindByFilter(func(acReward template.TplActivityReward) bool {
		return acReward.ActiveID == challenge.ActivityID
	})

	rewardId := ""
	for _, acReward := range acRewards {
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
		s.logger.Error("排名奖励不存在", zap.Int32("rank", int32(ownerRecord.Rank)), zap.String("activity_id", challenge.ActivityID))
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

// sendExpiredScoreRewards 发送过期的积分等级奖励
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

// sendChallengeRewardNotification 发送挑战赛奖励通知（公共方法）
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
		zap.String("subject", subject),
		zap.String("description", description),
		zap.Int("rewards_count", len(rewards)))

	return true, nil
}

// getOrAssignPlayerTournament 获取或分配玩家的竞标赛ID
func (s *ApiServer) getOrAssignPlayerTournament(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime, now time.Time) (string, error) {
	// 检查玩家是否已经参与了该挑战赛的任何竞标赛
	existingTournamentID, err := s.findPlayerExistingTournament(ctx, userID, tplChallenge.ID, startTime, endTime)
	if err != nil {
		return "", fmt.Errorf("查找玩家现有竞标赛失败: %v", err)
	}

	// 如果玩家已经参与，返回现有竞标赛ID
	if existingTournamentID != "" {
		return existingTournamentID, nil
	}

	// 检查挑战赛是否处于活跃状态
	if !isChallengeActive(now, startTime, endTime) {
		// 挑战赛未开始或已结束，不分配新的竞标赛
		return "", fmt.Errorf("挑战赛未开始或已结束")
	}

	// 为玩家分配竞标赛（查找空位或创建新的）
	tournamentID, err := s.assignPlayerToChallengeTournament(ctx, userID, tplChallenge, startTime, endTime)
	if err != nil {
		return "", fmt.Errorf("分配玩家到竞标赛失败: %v", err)
	}

	return tournamentID, nil
}

// findPlayerExistingTournament 查找玩家已参与的竞标赛ID
func (s *ApiServer) findPlayerExistingTournament(ctx context.Context, userID uuid.UUID, challengeID int32, startTime, endTime time.Time) (string, error) {
	// 获取该挑战赛的所有竞标赛
	tournaments, err := s.getChallengeTournaments(ctx, challengeID, startTime, endTime)
	if err != nil {
		return "", err
	}

	// 检查玩家是否在任何一个竞标赛中
	for _, tournament := range tournaments {
		records, err := TournamentRecordsList(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, tournament.Id, []string{userID.String()}, nil, "", 0)
		if err != nil {
			continue
		}

		if len(records.Records) > 0 {
			// 玩家已在该竞标赛中
			s.logger.Info("找到玩家现有竞标赛",
				zap.String("user_id", userID.String()),
				zap.String("tournament_id", tournament.Id),
				zap.Int32("challenge_id", challengeID))
			return tournament.Id, nil
		}
	}

	return "", nil
}

// getTournamentCurrentPlayerCount 获取竞标赛当前实际玩家数量
func (s *ApiServer) getTournamentCurrentPlayerCount(ctx context.Context, tournamentID string) (int32, error) {
	// 使用TournamentRecordsList获取所有玩家记录，限制数量提高性能
	records, err := TournamentRecordsList(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, tournamentID, nil, wrapperspb.Int32(1000), "", 0)
	if err != nil {
		return 0, fmt.Errorf("获取竞标赛记录失败: %v", err)
	}

	count := int32(len(records.Records))
	s.logger.Debug("获取竞标赛当前玩家数量",
		zap.String("tournament_id", tournamentID),
		zap.Int32("current_player_count", count))

	return count, nil
}

// assignPlayerToChallengeTournament 为玩家分配到挑战赛竞标赛
func (s *ApiServer) assignPlayerToChallengeTournament(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {

	// 获取该挑战赛的所有竞标赛
	tournaments, err := s.getChallengeTournaments(ctx, tplChallenge.ID, startTime, endTime)
	if err != nil {
		return "", err
	}

	s.logger.Info("查找可用竞标赛",
		zap.String("user_id", userID.String()),
		zap.Int32("challenge_id", tplChallenge.ID),
		zap.Int("found_tournaments", len(tournaments)),
		zap.Int32("max_participants", tplChallenge.MaxPart))

	// 寻找有空位的竞标赛
	for _, tournament := range tournaments {
		// 获取竞标赛当前实际玩家数量
		currentPlayerCount, err := s.getTournamentCurrentPlayerCount(ctx, tournament.Id)
		if err != nil {
			s.logger.Warn("获取竞标赛玩家数量失败，跳过此竞标赛",
				zap.String("tournament_id", tournament.Id),
				zap.Error(err))
			continue
		}

		s.logger.Info("检查竞标赛空位情况",
			zap.String("tournament_id", tournament.Id),
			zap.Int32("current_players", currentPlayerCount),
			zap.Int32("max_players", tplChallenge.MaxPart),
			zap.Bool("has_space", currentPlayerCount < tplChallenge.MaxPart))

		if currentPlayerCount < tplChallenge.MaxPart {
			// 尝试加入该竞标赛
			username := ctx.Value(ctxUsernameKey{}).(string)
			err := TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, tournament.Id)
			if err == nil {
				// 加入成功后，再次验证tournament人数，防止超员
				finalPlayerCount, countErr := s.getTournamentCurrentPlayerCount(ctx, tournament.Id)
				if countErr == nil {
					s.logger.Info("玩家成功加入现有竞标赛",
						zap.String("user_id", userID.String()),
						zap.String("tournament_id", tournament.Id),
						zap.Int32("challenge_id", tplChallenge.ID),
						zap.Int32("final_size", finalPlayerCount),
						zap.Int32("max_size", tplChallenge.MaxPart))

					// 如果超员，记录警告但不回滚（因为TournamentJoin已经成功）
					if finalPlayerCount > tplChallenge.MaxPart {
						s.logger.Warn("竞标赛出现超员情况（并发导致）",
							zap.String("tournament_id", tournament.Id),
							zap.Int32("final_size", finalPlayerCount),
							zap.Int32("max_size", tplChallenge.MaxPart))
					}
				}
				return tournament.Id, nil
			} else {
				s.logger.Warn("加入现有竞标赛失败",
					zap.String("tournament_id", tournament.Id),
					zap.Error(err))
			}
		} else {
			s.logger.Info("竞标赛已满员，跳过",
				zap.String("tournament_id", tournament.Id),
				zap.Int32("current_players", currentPlayerCount),
				zap.Int32("max_players", tplChallenge.MaxPart))
		}
	}

	// 没有可用的竞标赛，创建新的
	// 使用持久化的批次号管理系统确保唯一性
	batchNumber := s.GetNextChallengeBatch(tplChallenge.ID)

	s.logger.Info("创建新竞标赛",
		zap.String("user_id", userID.String()),
		zap.Int32("challenge_id", tplChallenge.ID),
		zap.Int32("batch_number", batchNumber),
		zap.String("reason", "没有找到可用的竞标赛"),
		zap.Int("checked_tournaments", len(tournaments)))

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

// createNewChallengeTournament 创建新的挑战赛竞标赛
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
		false,                     // authoritative - 权威模式，服务器管理
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
		100,                       // maxNumScore - 允许的最大提交次数
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

// getChallengeTournaments 获取指定挑战赛的所有竞标赛（使用category和时间范围查询，避免哈希冲突）
func (s *ApiServer) getChallengeTournaments(ctx context.Context, challengeID int32, startTime, endTime time.Time) ([]*api.Tournament, error) {
	// 使用category和时间范围进行查询，避免哈希冲突导致的错误匹配
	// 增加limit以支持更多竞标赛，避免batchNumber超过100时查询不到最新竞标赛的问题
	tournaments, err := TournamentList(ctx, s.logger, s.db, s.leaderboardCache,
		int(challengeID), int(challengeID), // 精确匹配category范围
		int(startTime.Unix()), int(endTime.Unix()), // startTime=0(不限制), endTime=-1(显示进行中和未来的锦标赛)
		5000, // limit - 增加到1000以支持更多竞标赛
		nil)  // cursor
	if err != nil {
		return nil, err
	}

	// 过滤出属于该挑战赛的竞标赛
	var result []*api.Tournament
	challengePrefix := fmt.Sprintf("challenge_%d", challengeID)

	for _, tournament := range tournaments.Tournaments {
		// 验证ID前缀和元数据
		if strings.HasPrefix(tournament.Id, challengePrefix) {
			var metadata ChallengeTournamentMetadata
			if err := json.Unmarshal([]byte(tournament.Metadata), &metadata); err == nil {
				if metadata.ChallengeID == challengeID && metadata.CreatedBy == "server" {
					result = append(result, tournament)
				}
			} else {
				// 如果metadata解析失败，但ID匹配且category正确，也认为是有效的（兼容老数据）
				if strings.HasPrefix(tournament.Id, challengePrefix) {
					result = append(result, tournament)
				}
			}
		}
	}

	s.logger.Debug("通过category快速查找挑战赛所有竞标赛",
		zap.Int32("challenge_id", challengeID),
		zap.String("prefix", challengePrefix),
		zap.Int("total_found", len(tournaments.Tournaments)),
		zap.Int("filtered_count", len(result)))

	// 当竞标赛数量接近limit时记录警告
	if len(tournaments.Tournaments) >= 5000 {
		s.logger.Warn("挑战赛竞标赛数量接近limit上限，可能存在查询截断风险",
			zap.Int32("challenge_id", challengeID),
			zap.Int("total_found", len(tournaments.Tournaments)),
			zap.Int("filtered_count", len(result)))
	}

	return result, nil
}

// isChallengeActive 检查挑战赛是否处于活跃状态（可以参与）
func isChallengeActive(now, startTime, endTime time.Time) bool {
	return (now.After(startTime) || now.Equal(startTime)) && now.Before(endTime)
}

// shouldJoinChallenge 判断是否可以参加挑战赛
func shouldJoinChallenge(now, startTime, endTime time.Time, rewardRemains int32) bool {
	// 条件1：正在进行中的比赛（已开始但未结束）
	if startTime.Before(now) || startTime.Equal(now) {
		if endTime.After(now) {
			return true
		}
	}
	return false
}

// shouldShowChallenge 判断是否应该显示该挑战赛
// 注意：此函数主要用于测试，实际业务逻辑在GetChallenge中实现
// 实际的显示逻辑是：所有正在进行的比赛 + 下一场比赛
func shouldShowChallenge(now, startTime, endTime time.Time, rewardRemains int32) bool {
	// 条件1：正在进行中的比赛（已开始但未结束）
	if startTime.Before(now) || startTime.Equal(now) {
		if endTime.After(now) {
			return true
		}
	}

	// 条件2：未来2天内开始的比赛
	twoDaysLater := now.AddDate(0, 0, 2)
	if startTime.After(now) && (startTime.Before(twoDaysLater) || startTime.Equal(twoDaysLater)) {
		return true
	}

	// 条件3：已经结束但还在奖励领取期内的比赛
	if endTime.Before(now) {
		rewardDeadline := endTime.Add(time.Duration(rewardRemains) * time.Minute)
		if now.Before(rewardDeadline) || now.Equal(rewardDeadline) {
			return true
		}
	}

	return false
}

// GetChallengeTournamentCount 获取指定挑战赛的竞标赛数量（快速统计）
func (s *ApiServer) GetChallengeTournamentCount(ctx context.Context, challengeID int32, startTime, endTime time.Time) (int, error) {
	tournaments, err := s.getChallengeTournaments(ctx, challengeID, startTime, endTime)
	if err != nil {
		return 0, err
	}

	s.logger.Debug("统计挑战赛竞标赛数量",
		zap.Int32("challenge_id", challengeID),
		zap.Int("count", len(tournaments)))

	return len(tournaments), nil
}

// FindAvailableChallengeTournament 查找有空位的挑战赛竞标赛（快速匹配）
func (s *ApiServer) FindAvailableChallengeTournament(ctx context.Context, challengeID int32, maxParticipants int32, startTime, endTime time.Time) (*api.Tournament, error) {
	tournaments, err := s.getChallengeTournaments(ctx, challengeID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	// 按创建时间排序，优先加入较早创建的竞标赛
	for _, tournament := range tournaments {
		// 获取竞标赛当前实际玩家数量
		currentPlayerCount, err := s.getTournamentCurrentPlayerCount(ctx, tournament.Id)
		if err != nil {
			s.logger.Warn("获取竞标赛玩家数量失败，跳过此竞标赛",
				zap.String("tournament_id", tournament.Id),
				zap.Error(err))
			continue
		}

		if currentPlayerCount < maxParticipants {
			s.logger.Debug("找到可用的挑战赛竞标赛",
				zap.Int32("challenge_id", challengeID),
				zap.String("tournament_id", tournament.Id),
				zap.Int32("current_size", currentPlayerCount),
				zap.Int32("max_size", maxParticipants))
			return tournament, nil
		}
	}

	s.logger.Debug("未找到可用的挑战赛竞标赛",
		zap.Int32("challenge_id", challengeID),
		zap.Int("total_tournaments", len(tournaments)))

	return nil, nil // 没有找到可用的竞标赛
}

// GetChallengeFromTournamentID 从竞标赛ID反向查找挑战赛ID
func (s *ApiServer) GetChallengeFromTournamentID(tournamentID string) string {
	// 从竞标赛ID中解析挑战赛ID
	// 格式: challenge_{challengeID}_{startTimestamp}_{batchNumber}
	if !strings.HasPrefix(tournamentID, "challenge_") {
		return ""
	}

	parts := strings.Split(tournamentID, "_")
	if len(parts) < 4 {
		return ""
	}

	// 挑战赛ID是第2个到倒数第3个部分
	challengeID := strings.Join(parts[1:len(parts)-2], "_")

	return challengeID
}

// ListAllChallengeTournaments 列出所有挑战赛竞标赛（管理员功能）
func (s *ApiServer) ListAllChallengeTournaments(ctx context.Context) (map[string][]*api.Tournament, error) {
	// 使用挑战赛category范围查询所有挑战赛竞标赛
	tournaments, err := TournamentList(ctx, s.logger, s.db, s.leaderboardCache,
		1000, 9999, // 挑战赛category范围
		0,    // 任何开始时间
		0,    // 任何结束时间
		5000, // limit - 增加到5000以支持更多竞标赛（管理员功能）
		nil)  // cursor
	if err != nil {
		return nil, err
	}

	// 按挑战赛ID分组
	result := make(map[string][]*api.Tournament)

	for _, tournament := range tournaments.Tournaments {
		if strings.HasPrefix(tournament.Id, "challenge_") {
			challengeID := s.GetChallengeFromTournamentID(tournament.Id)
			if challengeID != "" {
				result[challengeID] = append(result[challengeID], tournament)
			}
		}
	}

	s.logger.Info("列出所有挑战赛竞标赛",
		zap.Int("total_tournaments", len(tournaments.Tournaments)),
		zap.Int("challenge_count", len(result)))

	return result, nil
}
