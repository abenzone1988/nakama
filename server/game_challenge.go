package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	JoinedAt        time.Time `json:"joined_at"`
	RankReward      bool      `json:"rank_reward"`       // 排名奖励是否已领取
	LowScoreReward  bool      `json:"low_score_reward"`  // 低分奖励是否已领取
	MidScoreReward  bool      `json:"mid_score_reward"`  // 中分奖励是否已领取
	HighScoreReward bool      `json:"high_score_reward"` // 高分奖励是否已领取
}

type UserMatch struct {
	Challenges map[int32]*ChallengeStatus `json:"challenges"`
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

	// 转换 TplChallenge 数组为 game.Challenge 数组，并进行时间过滤
	challenges := make([]*game.Challenge, 0, len(tplChallenges))

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

		// 解析开始时间
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

		// 时间过滤逻辑
		if !shouldShowChallenge(now, startTime, endTime, tplChallenge.RewardRemains) {
			continue
		}

		// 获取用户挑战赛状态中的竞标赛ID
		var tournamentID string
		if challengeData, exists := userMatch.Challenges[tplChallenge.ID]; exists && challengeData != nil {
			tournamentID = challengeData.TournamentID
		}

		challenge := &game.Challenge{
			Id:           tplChallenge.ID,
			ActivityId:   tplChallenge.ActivityID,
			Open:         timestamppb.New(startTime),
			Close:        timestamppb.New(closeTime),
			End:          timestamppb.New(endTime),
			Over:         timestamppb.New(endTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)),
			MaxPart:      tplChallenge.MaxPart,
			TournamentId: tournamentID,
		}

		challenges = append(challenges, challenge)
	}

	// 转换用户已参与的挑战赛状态为JoinChallengeStatus数组
	joinedChallenges := make([]*game.JoinChallengeStatus, 0, len(userMatch.Challenges))
	for _, challengeStatus := range userMatch.Challenges {
		joinedStatus := &game.JoinChallengeStatus{
			Id:              challengeStatus.ID,
			TournamentId:    challengeStatus.TournamentID,
			JoinedAt:        timestamppb.New(challengeStatus.JoinedAt),
			RankReward:      challengeStatus.RankReward,
			LowScoreReward:  challengeStatus.LowScoreReward,
			MidScoreReward:  challengeStatus.MidScoreReward,
			HighScoreReward: challengeStatus.HighScoreReward,
		}
		joinedChallenges = append(joinedChallenges, joinedStatus)
	}

	return &game.GetChallengeResponse{
		Challenges: challenges,
		Joined:     joinedChallenges,
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

	// 时间过滤逻辑
	if !shouldShowChallenge(now, startTime, endTime, tplChallenge.RewardRemains) {
		return &game.JoinChallengeResponse{
			Code: 4,
			Msg:  "挑战赛未开放或已结束",
		}, nil
	}

	overTime := endTime.Add(time.Duration(tplChallenge.RewardRemains) * time.Minute)

	// 获取或分配玩家的竞标赛ID
	tournamentID, err := s.getOrAssignPlayerTournament(ctx, userID, &tplChallenge, startTime, overTime, now)
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
		ID:           tplChallenge.ID,
		TournamentID: tournamentID,
		JoinedAt:     now.Truncate(time.Second),
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

	var rewardIds []string
	if ownerRecord.Score >= int64(activityInfo.Condition01) && !challengeData.LowScoreReward {
		challengeData.LowScoreReward = true
		rewardIds = append(rewardIds, activityInfo.Reward01)
	}
	if ownerRecord.Score >= int64(activityInfo.Condition02) && !challengeData.MidScoreReward {
		challengeData.MidScoreReward = true
		rewardIds = append(rewardIds, activityInfo.Reward02)
	}
	if ownerRecord.Score >= int64(activityInfo.Condition03) && !challengeData.HighScoreReward {
		challengeData.HighScoreReward = true
		rewardIds = append(rewardIds, activityInfo.Reward03)
	}

	// 解析奖励
	rewards, err := s.parseRewards(rewardIds)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("解析奖励失败: %v", err))
	}

	if len(rewards) == 0 {
		return &game.GainChallengeRewardResponse{
			Code: 6,
			Msg:  "没有可领取的奖励",
		}, nil
	}

	// 保存奖励状态更新
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
	if err != nil {
		return nil, err
	}

	s.logger.Info("发放挑战赛奖励",
		zap.Int32("challenge_id", in.ChallengeId),
		zap.String("user_id", userID.String()),
		zap.Strings("reward_ids", rewardIds),
		zap.Int32("rank", int32(ownerRecord.Rank)))

	return &game.GainChallengeRewardResponse{
		Code:   0,
		Msg:    "领取奖励成功",
		Reward: rewards,
	}, nil
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

// createNewChallengeTournament 创建新的挑战赛竞标赛
func (s *ApiServer) createNewChallengeTournament(ctx context.Context, tplChallenge *template.TplChallenge, startTime, endTime time.Time, batchNumber int) (string, error) {
	// 构造标准化的竞标赛ID
	// 格式: challenge_{challengeID}_{startTimestamp}_{batchNumber}
	// batchNumber 使用持久化的递增序号确保唯一性
	tournamentID := fmt.Sprintf("challenge_%d_%d_%03d",
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
	tournaments, err := TournamentList(ctx, s.logger, s.db, s.leaderboardCache,
		int(challengeID), int(challengeID), // 精确匹配category范围
		int(startTime.Unix()), int(endTime.Unix()), // startTime=0(不限制), endTime=-1(显示进行中和未来的锦标赛)
		100, // limit
		nil) // cursor
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
		1000, // limit
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

// parseRewards 解析奖励ID数组，返回奖励对象数组
func (s *ApiServer) parseRewards(rewardIds []string) ([]*game.Reward, error) {
	if len(rewardIds) == 0 {
		return nil, nil
	}

	rewards := make([]*game.Reward, 0, len(rewardIds))

	for _, rewardId := range rewardIds {
		if rewardId == "" {
			continue
		}

		tplReward, found := s.template.GetTplReward().FindByKey(rewardId)
		if !found {
			s.logger.Warn("奖励配置不存在",
				zap.String("reward_id", rewardId))
			continue
		}

		// 构造钱包奖励
		walletReward := &game.Wallet{
			Gem:  tplReward.Gem,
			Coin: tplReward.Coin,
			Ad:   tplReward.Coupon,
		}

		// 解析物品奖励
		items, err := s.parseItemRewards(tplReward.Items, rewardId)
		if err != nil {
			s.logger.Warn("解析物品奖励失败",
				zap.String("reward_id", rewardId),
				zap.Error(err))
			continue
		}

		// 构造奖励对象
		reward := &game.Reward{
			Wallet: walletReward,
			Items:  items,
		}

		rewards = append(rewards, reward)
	}

	return rewards, nil
}

// parseItemRewards 解析物品奖励字符串 "90000_20,90001_30"
func (s *ApiServer) parseItemRewards(itemsStr, rewardId string) ([]*game.Item, error) {
	if itemsStr == "" || strings.TrimSpace(itemsStr) == "" {
		return nil, nil
	}

	var items []*game.Item
	itemStrings := strings.Split(itemsStr, ",")

	for _, itemStr := range itemStrings {
		itemStr = strings.TrimSpace(itemStr)
		if itemStr == "" {
			continue
		}

		parts := strings.Split(itemStr, "_")
		if len(parts) != 2 {
			s.logger.Warn("物品奖励格式错误",
				zap.String("item_string", itemStr),
				zap.String("reward_id", rewardId))
			continue
		}

		itemID := strings.TrimSpace(parts[0])
		numStr := strings.TrimSpace(parts[1])

		num, err := strconv.ParseInt(numStr, 10, 32)
		if err != nil {
			s.logger.Warn("物品数量解析失败",
				zap.String("item_string", itemStr),
				zap.String("num_string", numStr),
				zap.String("reward_id", rewardId),
				zap.Error(err))
			continue
		}

		item := &game.Item{
			Id:  itemID,
			Num: int32(num),
		}
		items = append(items, item)
	}

	return items, nil
}

// parseSingleReward 解析单个奖励ID，返回奖励对象（兼容性方法）
func (s *ApiServer) parseSingleReward(rewardId string) (*game.Reward, error) {
	rewards, err := s.parseRewards([]string{rewardId})
	if err != nil {
		return nil, err
	}

	if len(rewards) == 0 {
		return nil, fmt.Errorf("奖励不存在: %s", rewardId)
	}

	return rewards[0], nil
}
