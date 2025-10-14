package server

import "time"

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
