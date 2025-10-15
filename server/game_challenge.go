package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	tournamentID, err := s.assignPlayerToChallengeTournament(ctx, userID, &tplChallenge, startTime, endTime)
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
	// 根据集群配置选择锁类型
	if s.config.GetCluster().Enabled {
		// 集群模式：双层锁策略（内存锁 + Redis锁）
		return s.assignPlayerToChallengeTournamentWithHybridLock(ctx, userID, tplChallenge, startTime, endTime)
	} else {
		// 单节点模式：只使用内存锁（性能最优）
		// 单节点无需Redis锁，内存锁足以保证一致性
		return s.assignPlayerToChallengeTournamentWithMemoryLock(ctx, userID, tplChallenge, startTime, endTime)
	}
}

// 使用内存锁的分配方法
func (s *ApiServer) assignPlayerToChallengeTournamentWithMemoryLock(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {
	// 加锁：按 challenge_id 串行化查找、加入与创建
	lockIface, _ := s.challengeAssignLocks.LoadOrStore(tplChallenge.ID, &sync.Mutex{})
	mu := lockIface.(*sync.Mutex)
	mu.Lock()

	// 【性能监控】记录内存锁持有时间
	lockAcquireTime := time.Now()
	defer func() {
		mu.Unlock()

		lockHoldDuration := time.Since(lockAcquireTime)

		// 记录锁持有时间（用于性能分析和调优）
		s.logger.Info("内存锁持有时间统计",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.Duration("lock_hold_time", lockHoldDuration),
			zap.Int64("lock_hold_ms", lockHoldDuration.Milliseconds()))

		// 如果持有时间过长，发出警告
		if lockHoldDuration > 50*time.Millisecond {
			s.logger.Warn("内存锁持有时间过长",
				zap.String("user_id", userID.String()),
				zap.Int32("challenge_id", tplChallenge.ID),
				zap.Duration("lock_hold_time", lockHoldDuration))
		}
	}()

	return s.performTournamentAssignment(ctx, userID, tplChallenge, startTime, endTime)
}

// 使用Redis分布式锁的分配方法（集群模式专用）
func (s *ApiServer) assignPlayerToChallengeTournamentWithHybridLock(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {
	// 创建Redis客户端（如果还没有的话）
	if s.redisClient == nil {
		s.redisClient = redis.NewClient(&redis.Options{
			Addr:     s.config.GetCluster().RedisAddress,
			Password: s.config.GetCluster().RedisPassword,
			DB:       0,
		})
	}

	// 创建分布式锁实例
	distributedLock := NewRedisDistributedLock(s.redisClient, s.logger)

	// 【集群模式：只用Redis分布式锁】
	// 说明：集群模式下，各节点独立，直接使用Redis锁协调即可
	// 不需要内存锁，因为：
	// 1. 内存锁只能保护单节点，无法跨节点
	// 2. 先内存锁会导致节点内请求阻塞，降低吞吐
	// 3. 先Redis锁会导致所有节点请求都打到Redis（没有过滤）
	// 结论：集群模式接受部分请求失败，依赖客户端重试

	lockKey := strconv.FormatInt(int64(tplChallenge.ID), 10)
	acquired, lockValue, err := distributedLock.TryLockWithSpin(ctx, lockKey, redisLockTTL, redisSpinAttempts, redisSpinInterval)
	if err != nil {
		s.logger.Error("获取Redis自旋锁失败",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.Error(err))
		return "", fmt.Errorf("获取分布式锁失败: %v", err)
	}

	if !acquired {
		// Redis自旋锁失败，降级重试
		s.logger.Debug("Redis自旋锁失败，尝试降级重试",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID))

		acquired, lockValue, err = distributedLock.TryLockWithFallback(ctx, lockKey, redisLockTTL, redisFallbackRetry, redisFallbackDelay)
		if err != nil || !acquired {
			s.logger.Info("无法获取Redis锁，快速失败（客户端应重试）",
				zap.String("user_id", userID.String()),
				zap.Int32("challenge_id", tplChallenge.ID))
			return "", fmt.Errorf("服务器繁忙，请稍后重试")
		}
	}

	// 【性能监控】记录Redis锁持有时间
	lockAcquireTime := time.Now()
	defer func() {
		lockHoldDuration := time.Since(lockAcquireTime)

		// 释放Redis锁
		if err := distributedLock.ReleaseLock(ctx, lockKey, lockValue); err != nil {
			s.logger.Error("释放Redis锁失败",
				zap.String("user_id", userID.String()),
				zap.Int32("challenge_id", tplChallenge.ID),
				zap.Error(err))
		}

		// 记录锁持有时间（用于性能分析和调优）
		s.logger.Info("Redis锁持有时间统计",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.Duration("lock_hold_time", lockHoldDuration),
			zap.Int64("lock_hold_ms", lockHoldDuration.Milliseconds()))

		// 如果持有时间过长，发出警告
		if lockHoldDuration > 50*time.Millisecond {
			s.logger.Warn("Redis锁持有时间过长",
				zap.String("user_id", userID.String()),
				zap.Int32("challenge_id", tplChallenge.ID),
				zap.Duration("lock_hold_time", lockHoldDuration))
		}
	}()

	s.logger.Debug("成功获取Redis分布式锁",
		zap.String("user_id", userID.String()),
		zap.Int32("challenge_id", tplChallenge.ID))

	return s.performTournamentAssignment(ctx, userID, tplChallenge, startTime, endTime)
}

// 执行竞标赛分配的核心逻辑（锁内执行）
func (s *ApiServer) performTournamentAssignment(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {
	// 锁内查找并尝试加入（避免并发加入同一比赛），最多重试2次以避免不必要创建
	for i := 0; i < 2; i++ {
		tournament, err := TournamentFindFirstAvailable(ctx, s.logger, s.db, s.leaderboardCache, int(tplChallenge.ID))
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

		s.logger.Warn("加入可用竞标赛失败，锁内重试",
			zap.String("tournament_id", tournament.Id),
			zap.String("user_id", userID.String()),
			zap.Int("retry", i+1),
			zap.Error(err))
	}

	// 创建新的竞标赛并加入
	batchNumber := GetNextChallengeBatch(ctx, s.logger, s.db, tplChallenge.ID)
	newTournamentID, err := CreateNewChallengeTournament(ctx, s.logger, s.leaderboardCache, s.leaderboardScheduler, tplChallenge, startTime, endTime, int(batchNumber))
	if err != nil {
		s.logger.Warn("创建新竞标赛失败",
			zap.String("user_id", userID.String()),
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.Error(err))
		return "", fmt.Errorf("创建新竞标赛失败: %v", err)
	}

	username := ctx.Value(ctxUsernameKey{}).(string)
	err = TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, newTournamentID)
	if err != nil {
		s.logger.Warn("加入新竞标赛失败",
			zap.String("user_id", userID.String()),
			zap.String("tournament_id", newTournamentID),
			zap.Error(err))
		return "", fmt.Errorf("加入新竞标赛失败: %v", err)
	}

	s.logger.Info("玩家成功加入新创建的竞标赛",
		zap.String("user_id", userID.String()),
		zap.String("tournament_id", newTournamentID),
		zap.Int32("challenge_id", tplChallenge.ID),
		zap.Int32("batch_number", batchNumber))

	return newTournamentID, nil
}

// 创建新的挑战赛竞标赛
func CreateNewChallengeTournament(ctx context.Context, logger *zap.Logger, leaderboardCache LeaderboardCache, leaderboardScheduler LeaderboardScheduler, tplChallenge *template.TplChallenge, startTime, endTime time.Time, batchNumber int) (string, error) {
	// 构造标准化的竞标赛ID
	// 格式: challenge_{challengeID}_{startTimestamp}_{batchNumber}
	// batchNumber 使用持久化的递增序号确保唯一性，不限制位数以支持超过999的batch
	tournamentID := fmt.Sprintf("challenge_%d_%d_%d",
		tplChallenge.ID,
		startTime.Unix(),
		batchNumber)

	// Tournament配置
	operator := LeaderboardOperatorBest
	sortOrder := LeaderboardSortOrderDescending
	duration := int(endTime.Sub(startTime).Seconds())

	// 构造竞标赛标题和描述
	title := fmt.Sprintf("%s - 第%d轮", tplChallenge.Name, batchNumber)
	description := fmt.Sprintf("挑战赛 %s 的竞标赛，由服务器自动创建和管理", tplChallenge.Name)

	// 创建Tournament（服务器作为创建者，无玩家拥有者）
	err := TournamentCreate(ctx, logger, leaderboardCache, leaderboardScheduler, // 使用正确的scheduler
		tournamentID,              // id
		false,                     //非权威模式 玩家可以自主提交
		sortOrder,                 // sortOrder
		operator,                  // operator
		"",                        // resetSchedule (空表示不重置)
		"{}",                      // metadata - 包含完整的挑战赛信息
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

	logger.Info("成功创建新的挑战赛竞标赛",
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

	// 初始化前三名统计数据
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

// isRetryableError 检查错误是否可重试
func isRetryableError(err error, logger *zap.Logger) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 检查常见的可重试错误
	retryableErrors := []string{
		"WriteTooOldError",
		"TransactionRetryWithProtoRefreshError",
		"SerializationFailure",
		"deadlock",
		"lock timeout",
		"connection reset",
		"temporary failure",
		"busy",
		"SQLSTATE 40001", // PostgreSQL serialization failure
		"SQLSTATE 40P01", // PostgreSQL deadlock detected
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			logger.Error("retryable error", zap.String("error", retryableErr))
			return true
		}
	}

	return false
}
