package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	EndTimestamp    int64  `json:"end_timestamp"`    // 结束时间戳
	MaxParticipants int32  `json:"max_participants"` // 最大参与人数
}

type Challenge struct {
	ID           int32     `json:"id"`
	TournamentID string    `json:"tournament_id"`
	JoinedAt     time.Time `json:"joined_at"`
}

type UserMatch struct {
	Challenges map[int32]*Challenge `json:"challenges"`
}

func (f *UserMatch) GetCollection() string {
	return "user_data"
}

func (f *UserMatch) GetKey() string {
	return "match"
}

func (f *UserMatch) Init() {
	f.Challenges = map[int32]*Challenge{}
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
		startTime, err := parseDateTime(tplChallenge.StartDate, tplChallenge.StartTime)
		if err != nil {
			s.logger.Error("解析挑战赛开始时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("start_date", tplChallenge.StartDate),
				zap.String("start_time", tplChallenge.StartTime),
				zap.Error(err))
			continue
		}

		// 解析结束时间
		endTime, err := parseDateTime(tplChallenge.EndDate, tplChallenge.EndTime)
		if err != nil {
			s.logger.Error("解析挑战赛结束时间失败",
				zap.Int32("id", tplChallenge.ID),
				zap.String("end_date", tplChallenge.EndDate),
				zap.String("end_time", tplChallenge.EndTime),
				zap.Error(err))
			continue
		}

		// 时间过滤逻辑
		if !shouldShowChallenge(now, startTime, endTime, tplChallenge.RewardRemains) {
			continue
		}

		// 获取或分配玩家的竞标赛ID
		//tournamentID, err := s.getOrAssignPlayerTournament(ctx, userID, &tplChallenge, startTime, endTime, now)
		//if err != nil {
		//	s.logger.Error("获取玩家竞标赛ID失败",
		//		zap.String("challenge_id", tplChallenge.ID),
		//		zap.String("user_id", userID.String()),
		//		zap.Error(err))
		//	continue
		//}

		// 获取用户挑战赛状态中的竞标赛ID
		var tournamentID string
		if challengeData, exists := userMatch.Challenges[tplChallenge.ID]; exists && challengeData != nil {
			tournamentID = challengeData.TournamentID
		}

		challenge := &game.Challenge{
			Id:           tplChallenge.ID,
			ActivityId:   tplChallenge.ActivityID,
			Start:        timestamppb.New(startTime),
			End:          timestamppb.New(endTime),
			MaxPart:      tplChallenge.MaxPart,
			TournamentId: tournamentID,
		}

		challenges = append(challenges, challenge)
	}

	return &game.GetChallengeResponse{
		Challenges: challenges,
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
	startTime, err := parseDateTime(tplChallenge.StartDate, tplChallenge.StartTime)
	if err != nil {
		s.logger.Error("解析挑战赛开始时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("start_date", tplChallenge.StartDate),
			zap.String("start_time", tplChallenge.StartTime),
			zap.Error(err))
		return &game.JoinChallengeResponse{
			Code: 3,
			Msg:  "配置错误",
		}, nil
	}

	// 解析结束时间
	endTime, err := parseDateTime(tplChallenge.EndDate, tplChallenge.EndTime)
	if err != nil {
		s.logger.Error("解析挑战赛结束时间失败",
			zap.Int32("id", tplChallenge.ID),
			zap.String("end_date", tplChallenge.EndDate),
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

	// 获取或分配玩家的竞标赛ID
	tournamentID, err := s.getOrAssignPlayerTournament(ctx, userID, &tplChallenge, startTime, endTime, now)
	if err != nil {
		s.logger.Error("加入挑战赛失败",
			zap.Int32("challenge_id", tplChallenge.ID),
			zap.String("user_id", userID.String()),
			zap.Error(err))

		return &game.JoinChallengeResponse{
			Code: 5,
			Msg:  "无法加入挑战赛，系统错误",
		}, nil
	}

	userMatch.Challenges[tplChallenge.ID] = &Challenge{
		ID:           tplChallenge.ID,
		TournamentID: tournamentID,
		JoinedAt:     now.Truncate(time.Second),
	}

	// 保存更新后的领取历史记录
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, userMatch)
	if err != nil {
		return nil, err
	}
	return &game.JoinChallengeResponse{
		Code: 0,
		Challenge: &game.Challenge{
			Id:           tplChallenge.ID,
			ActivityId:   tplChallenge.ActivityID,
			Start:        timestamppb.New(startTime),
			End:          timestamppb.New(endTime),
			MaxPart:      tplChallenge.MaxPart,
			TournamentId: tournamentID,
		},
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
		return "", nil
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

// assignPlayerToChallengeTournament 为玩家分配到挑战赛竞标赛
func (s *ApiServer) assignPlayerToChallengeTournament(ctx context.Context, userID uuid.UUID, tplChallenge *template.TplChallenge, startTime, endTime time.Time) (string, error) {
	// 获取该挑战赛的所有竞标赛
	tournaments, err := s.getChallengeTournaments(ctx, tplChallenge.ID, startTime, endTime)
	if err != nil {
		return "", err
	}

	// 寻找有空位的竞标赛
	for _, tournament := range tournaments {
		if int32(tournament.Size) < tplChallenge.MaxPart {
			// 尝试加入该竞标赛
			username := ctx.Value(ctxUsernameKey{}).(string)
			err := TournamentJoin(ctx, s.logger, s.db, s.leaderboardCache, s.leaderboardRankCache, userID, username, tournament.Id)
			if err == nil {
				s.logger.Info("玩家成功加入现有竞标赛",
					zap.String("user_id", userID.String()),
					zap.String("tournament_id", tournament.Id),
					zap.Int32("challenge_id", tplChallenge.ID),
					zap.Int32("current_size", int32(tournament.Size)+1),
					zap.Int32("max_size", tplChallenge.MaxPart))
				return tournament.Id, nil
			} else {
				s.logger.Warn("加入现有竞标赛失败",
					zap.String("tournament_id", tournament.Id),
					zap.Error(err))
			}
		}
	}

	// 没有可用的竞标赛，创建新的
	newTournamentID, err := s.createNewChallengeTournament(ctx, tplChallenge, startTime, endTime, len(tournaments)+1)
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
		zap.Int("batch_number", len(tournaments)+1))

	return newTournamentID, nil
}

// createNewChallengeTournament 创建新的挑战赛竞标赛
func (s *ApiServer) createNewChallengeTournament(ctx context.Context, tplChallenge *template.TplChallenge, startTime, endTime time.Time, batchNumber int) (string, error) {
	// 构造标准化的竞标赛ID
	// 格式: challenge_{challengeID}_{startTimestamp}_{batchNumber}
	tournamentID := fmt.Sprintf("challenge_%d_%d_%03d",
		tplChallenge.ID,
		startTime.Unix(),
		batchNumber)

	// 创建竞标赛元数据
	metadata := ChallengeTournamentMetadata{
		ChallengeID:     tplChallenge.ID,
		ChallengeType:   "server_managed", // 标识为服务器管理的竞标赛
		CreatedBy:       "server",         // 明确标识创建者为服务器
		BatchNumber:     batchNumber,
		StartTimestamp:  startTime.Unix(),
		EndTimestamp:    endTime.Unix(),
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
	err = TournamentCreate(ctx, s.logger, s.leaderboardCache, nil, // scheduler参数为nil，由系统管理
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
		false,                     // joinRequired - 设为false，因为我们手动管理加入过程
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
		0, 0, //不用时间过滤
		//int(startTime.Unix()), // 限制开始时间
		//int(endTime.Unix()),   // 限制结束时间
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
		if int32(tournament.Size) < maxParticipants {
			s.logger.Debug("找到可用的挑战赛竞标赛",
				zap.Int32("challenge_id", challengeID),
				zap.String("tournament_id", tournament.Id),
				zap.Int32("current_size", int32(tournament.Size)),
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
