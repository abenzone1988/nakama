package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestChallengeTimeScenarioImpl 具体的时间场景测试实现示例
func TestChallengeTimeScenarioImpl(t *testing.T) {
	// 模拟服务器设置
	server := setupTestServer(t)
	defer teardownTestServer(t, server)

	// 设置测试时间
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	// 创建测试挑战赛模板
	challengeTemplate := &template.TplChallenge{
		ID:            1001,
		Name:          "测试挑战赛",
		ActivityID:    "test_activity_001",
		OpenTime:      baseTime.Format("2006-01-02 15:04:05"),
		CloseTime:     baseTime.Add(1 * time.Hour).Format("2006-01-02 15:04:05"),
		EndTime:       baseTime.Add(2 * time.Hour).Format("2006-01-02 15:04:05"),
		RewardRemains: 60, // 1小时奖励保留时间
		MaxPart:       100,
	}

	// 创建测试用户
	userID := uuid.Must(uuid.NewV4())
	username := "test_user"
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userID)
	ctx = context.WithValue(ctx, ctxUsernameKey{}, username)

	// 测试场景1：比赛开始前
	t.Run("比赛开始前", func(t *testing.T) {
		// 设置当前时间为比赛开始前
		mockTime := baseTime.Add(-1 * time.Hour)
		server.SetMockTime(mockTime)

		// 测试GetChallenge - 应该能看到未来的比赛
		resp, err := server.GetChallenge(ctx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.NotNil(t, resp)

		// 应该能在下一场比赛中看到这个挑战赛
		found := false
		for _, challenge := range resp.Challenges {
			if challenge.Id == challengeTemplate.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "应该能在GetChallenge中看到未来的比赛")

		// 测试JoinChallenge - 应该失败
		joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(4), joinResp.Code, "比赛未开始，应该无法参加")
		assert.Equal(t, "挑战赛未开放或已结束", joinResp.Msg)
	})

	// 测试场景2：比赛进行中
	t.Run("比赛进行中", func(t *testing.T) {
		// 设置当前时间为比赛进行中
		mockTime := baseTime.Add(30 * time.Minute)
		server.SetMockTime(mockTime)

		// 测试JoinChallenge - 应该成功
		joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), joinResp.Code, "比赛进行中，应该能够参加")
		assert.NotNil(t, joinResp.Challenge)
		assert.NotEmpty(t, joinResp.Challenge.TournamentId)

		// 测试GetChallenge - 应该能看到已参加的比赛
		resp, err := server.GetChallenge(ctx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.NotNil(t, resp)

		// 应该在已参加列表中看到这个挑战赛
		found := false
		for _, joined := range resp.Joined {
			if joined.Id == challengeTemplate.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "应该能在已参加列表中看到这个挑战赛")

		// 模拟提交成绩
		tournamentID := joinResp.Challenge.TournamentId
		score := int64(8000)
		err = server.SubmitTournamentScore(ctx, tournamentID, score)
		require.NoError(t, err)
	})

	// 测试场景3：比赛结束后，结算前
	t.Run("比赛结束后结算前", func(t *testing.T) {
		// 设置当前时间为比赛结束后，但结算前
		mockTime := baseTime.Add(2*time.Hour + 30*time.Minute)
		server.SetMockTime(mockTime)

		// 测试JoinChallenge - 应该失败
		newUserID := uuid.Must(uuid.NewV4())
		newCtx := context.WithValue(context.Background(), ctxUserIDKey{}, newUserID)
		newCtx = context.WithValue(newCtx, ctxUsernameKey{}, "new_user")

		joinResp, err := server.JoinChallenge(newCtx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(4), joinResp.Code, "比赛已结束，应该无法参加")

		// 测试GainChallengeReward - 应该能领取奖励
		// 1. 测试积分奖励
		rewardResp, err := server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
			ChallengeId: challengeTemplate.ID,
			RankReward:  false, // 积分奖励
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), rewardResp.Code, "应该能够领取积分奖励")
		assert.NotNil(t, rewardResp.Reward)
		assert.Greater(t, len(rewardResp.Reward), 0, "应该有积分奖励")

		// 2. 测试排名奖励
		rewardResp, err = server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
			ChallengeId: challengeTemplate.ID,
			RankReward:  true, // 排名奖励
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), rewardResp.Code, "应该能够领取排名奖励")
		assert.NotNil(t, rewardResp.Reward)
		assert.Greater(t, len(rewardResp.Reward), 0, "应该有排名奖励")
	})

	// 测试场景4：比赛结算后
	t.Run("比赛结算后", func(t *testing.T) {
		// 设置当前时间为比赛结算后
		mockTime := baseTime.Add(4 * time.Hour)
		server.SetMockTime(mockTime)

		// 创建新用户测试邮件奖励
		newUserID := uuid.Must(uuid.NewV4())
		newCtx := context.WithValue(context.Background(), ctxUserIDKey{}, newUserID)
		newCtx = context.WithValue(newCtx, ctxUsernameKey{}, "email_test_user")

		// 首先让这个用户参加比赛（需要回到比赛时间）
		server.SetMockTime(baseTime.Add(30 * time.Minute))
		joinResp, err := server.JoinChallenge(newCtx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), joinResp.Code)

		// 提交成绩
		err = server.SubmitTournamentScore(newCtx, joinResp.Challenge.TournamentId, 9000)
		require.NoError(t, err)

		// 回到结算时间后
		server.SetMockTime(baseTime.Add(4 * time.Hour))

		// 测试GetChallenge - 应该触发邮件奖励发送
		resp, err := server.GetChallenge(newCtx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.True(t, resp.GetReward, "应该有邮件奖励被发送")

		// 验证邮件是否被发送
		emails := server.GetSentEmails(newUserID)
		assert.Greater(t, len(emails), 0, "应该有邮件被发送")

		// 验证邮件内容
		found := false
		for _, email := range emails {
			if email.Subject == "挑战赛排名奖励" || email.Subject == "挑战赛个人战绩奖励" {
				found = true
				break
			}
		}
		assert.True(t, found, "应该收到挑战赛奖励邮件")
	})
}

// TestChallengeRewardScenarioImpl 具体的奖励场景测试实现示例
func TestChallengeRewardScenarioImpl(t *testing.T) {
	server := setupTestServer(t)
	defer teardownTestServer(t, server)

	// 设置测试时间
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	server.SetMockTime(baseTime.Add(30 * time.Minute)) // 比赛进行中

	// 创建测试活动模板
	activityTemplate := &template.TplActivityInfo{
		ID:          "test_activity_001",
		Name:        "测试活动",
		Condition01: 1000,  // 低分条件
		Condition02: 5000,  // 中分条件
		Condition03: 10000, // 高分条件
		Reward01:    "low_score_reward",
		Reward02:    "mid_score_reward",
		Reward03:    "high_score_reward",
	}

	// 创建测试挑战赛模板
	challengeTemplate := &template.TplChallenge{
		ID:            1002,
		Name:          "奖励测试挑战赛",
		ActivityID:    activityTemplate.ID,
		OpenTime:      baseTime.Format("2006-01-02 15:04:05"),
		CloseTime:     baseTime.Add(1 * time.Hour).Format("2006-01-02 15:04:05"),
		EndTime:       baseTime.Add(2 * time.Hour).Format("2006-01-02 15:04:05"),
		RewardRemains: 60,
		MaxPart:       100,
	}

	// 测试不同分数的奖励
	testCases := []struct {
		name            string
		score           int64
		expectedRewards []string
		description     string
	}{
		{
			name:            "无成绩玩家",
			score:           0,
			expectedRewards: []string{},
			description:     "没有提交成绩，不应该获得任何奖励",
		},
		{
			name:            "低分玩家",
			score:           1500,
			expectedRewards: []string{"low_score_reward"},
			description:     "达到低分条件，应该获得低分奖励",
		},
		{
			name:            "中分玩家",
			score:           6000,
			expectedRewards: []string{"low_score_reward", "mid_score_reward"},
			description:     "达到中分条件，应该获得低分和中分奖励",
		},
		{
			name:            "高分玩家",
			score:           12000,
			expectedRewards: []string{"low_score_reward", "mid_score_reward", "high_score_reward"},
			description:     "达到高分条件，应该获得所有积分奖励",
		},
		{
			name:            "边界分数玩家",
			score:           5000,
			expectedRewards: []string{"low_score_reward", "mid_score_reward"},
			description:     "刚好达到中分条件边界",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建测试用户
			userID := uuid.Must(uuid.NewV4())
			username := fmt.Sprintf("test_user_%s", tc.name)
			ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userID)
			ctx = context.WithValue(ctx, ctxUsernameKey{}, username)

			// 用户参加挑战赛
			joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
				ChallengeId: challengeTemplate.ID,
			})
			require.NoError(t, err)
			assert.Equal(t, int32(0), joinResp.Code)

			// 如果有成绩，提交成绩
			if tc.score > 0 {
				err = server.SubmitTournamentScore(ctx, joinResp.Challenge.TournamentId, tc.score)
				require.NoError(t, err)
			}

			// 等待比赛结束
			server.SetMockTime(baseTime.Add(2*time.Hour + 30*time.Minute))

			// 测试积分奖励
			if len(tc.expectedRewards) > 0 {
				rewardResp, err := server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
					ChallengeId: challengeTemplate.ID,
					RankReward:  false, // 积分奖励
				})
				require.NoError(t, err)
				assert.Equal(t, int32(0), rewardResp.Code, tc.description)
				assert.NotNil(t, rewardResp.Reward)
				assert.Equal(t, len(tc.expectedRewards), len(rewardResp.Reward), "奖励数量应该匹配")
			} else {
				// 无成绩玩家不应该获得奖励
				rewardResp, err := server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
					ChallengeId: challengeTemplate.ID,
					RankReward:  false,
				})
				require.NoError(t, err)
				assert.Equal(t, int32(5), rewardResp.Code, "没有提交成绩")
				assert.Equal(t, "没有提交成绩", rewardResp.Msg)
			}
		})
	}
}

// TestChallengeBoundaryConditionsImpl 具体的边界条件测试实现示例
func TestChallengeBoundaryConditionsImpl(t *testing.T) {
	server := setupTestServer(t)
	defer teardownTestServer(t, server)

	// 设置测试时间
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	server.SetMockTime(baseTime.Add(30 * time.Minute)) // 比赛进行中

	// 创建测试挑战赛模板
	challengeTemplate := &template.TplChallenge{
		ID:            1003,
		Name:          "边界条件测试挑战赛",
		ActivityID:    "test_activity_001",
		OpenTime:      baseTime.Format("2006-01-02 15:04:05"),
		CloseTime:     baseTime.Add(1 * time.Hour).Format("2006-01-02 15:04:05"),
		EndTime:       baseTime.Add(2 * time.Hour).Format("2006-01-02 15:04:05"),
		RewardRemains: 60,
		MaxPart:       5, // 小容量用于测试满员情况
	}

	// 创建测试用户
	userID := uuid.Must(uuid.NewV4())
	username := "test_user"
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userID)
	ctx = context.WithValue(ctx, ctxUsernameKey{}, username)

	// 测试重复参加挑战赛
	t.Run("重复参加挑战赛", func(t *testing.T) {
		// 第一次参加应该成功
		joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), joinResp.Code)

		// 第二次参加应该失败
		joinResp, err = server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(2), joinResp.Code)
		assert.Equal(t, "挑战赛不可重复参加", joinResp.Msg)
	})

	// 测试竞标赛满员情况
	t.Run("竞标赛满员情况", func(t *testing.T) {
		// 创建多个用户填满竞标赛
		for i := 1; i < int(challengeTemplate.MaxPart); i++ {
			userID := uuid.Must(uuid.NewV4())
			username := fmt.Sprintf("user_%d", i)
			ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userID)
			ctx = context.WithValue(ctx, ctxUsernameKey{}, username)

			joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
				ChallengeId: challengeTemplate.ID,
			})
			require.NoError(t, err)
			assert.Equal(t, int32(0), joinResp.Code)
		}

		// 再加入一个用户应该创建新的竞标赛
		lastUserID := uuid.Must(uuid.NewV4())
		lastUsername := "last_user"
		lastCtx := context.WithValue(context.Background(), ctxUserIDKey{}, lastUserID)
		lastCtx = context.WithValue(lastCtx, ctxUsernameKey{}, lastUsername)

		joinResp, err := server.JoinChallenge(lastCtx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), joinResp.Code, "应该创建新的竞标赛")
		assert.NotEmpty(t, joinResp.Challenge.TournamentId)
	})

	// 测试重复领取奖励
	t.Run("重复领取奖励", func(t *testing.T) {
		// 提交成绩
		err := server.SubmitTournamentScore(ctx, "test_tournament", 8000)
		require.NoError(t, err)

		// 等待比赛结束
		server.SetMockTime(baseTime.Add(2*time.Hour + 30*time.Minute))

		// 第一次领取积分奖励应该成功
		rewardResp, err := server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
			ChallengeId: challengeTemplate.ID,
			RankReward:  false,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), rewardResp.Code)

		// 第二次领取应该失败
		rewardResp, err = server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
			ChallengeId: challengeTemplate.ID,
			RankReward:  false,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(6), rewardResp.Code)
		assert.Equal(t, "没有可领取的积分奖励", rewardResp.Msg)
	})

	// 测试无效挑战赛ID
	t.Run("无效挑战赛ID", func(t *testing.T) {
		// 测试不存在的挑战赛ID
		joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: 99999, // 不存在的ID
		})
		require.NoError(t, err)
		assert.Equal(t, int32(1), joinResp.Code)
		assert.Equal(t, "挑战赛Id不存在", joinResp.Msg)

		// 测试获取不存在的挑战赛奖励
		rewardResp, err := server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
			ChallengeId: 99999,
			RankReward:  false,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(1), rewardResp.Code)
		assert.Equal(t, "没有参加挑战赛", rewardResp.Msg)
	})
}

// TestChallengeTimeBoundaryImpl 具体的时间边界测试实现示例
func TestChallengeTimeBoundaryImpl(t *testing.T) {
	server := setupTestServer(t)
	defer teardownTestServer(t, server)

	// 设置测试时间
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	// 创建测试挑战赛模板
	challengeTemplate := &template.TplChallenge{
		ID:            1004,
		Name:          "时间边界测试挑战赛",
		ActivityID:    "test_activity_001",
		OpenTime:      baseTime.Format("2006-01-02 15:04:05"),
		CloseTime:     baseTime.Add(1 * time.Hour).Format("2006-01-02 15:04:05"),
		EndTime:       baseTime.Add(2 * time.Hour).Format("2006-01-02 15:04:05"),
		RewardRemains: 60,
		MaxPart:       100,
	}

	// 创建测试用户
	userID := uuid.Must(uuid.NewV4())
	username := "test_user"
	ctx := context.WithValue(context.Background(), ctxUserIDKey{}, userID)
	ctx = context.WithValue(ctx, ctxUsernameKey{}, username)

	// 测试开始时间边界
	t.Run("开始时间边界", func(t *testing.T) {
		// 开始时间前1秒
		server.SetMockTime(baseTime.Add(-1 * time.Second))
		joinResp, err := server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(4), joinResp.Code, "开始前1秒应该无法参加")

		// 开始时间精确时刻
		server.SetMockTime(baseTime)
		joinResp, err = server.JoinChallenge(ctx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), joinResp.Code, "开始时间精确时刻应该可以参加")
	})

	// 测试结束时间边界
	t.Run("结束时间边界", func(t *testing.T) {
		// 结束时间前1秒
		server.SetMockTime(baseTime.Add(2*time.Hour - 1*time.Second))

		// 创建新用户测试
		newUserID := uuid.Must(uuid.NewV4())
		newCtx := context.WithValue(context.Background(), ctxUserIDKey{}, newUserID)
		newCtx = context.WithValue(newCtx, ctxUsernameKey{}, "new_user")

		joinResp, err := server.JoinChallenge(newCtx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), joinResp.Code, "结束前1秒应该可以参加")

		// 结束时间精确时刻
		server.SetMockTime(baseTime.Add(2 * time.Hour))

		anotherUserID := uuid.Must(uuid.NewV4())
		anotherCtx := context.WithValue(context.Background(), ctxUserIDKey{}, anotherUserID)
		anotherCtx = context.WithValue(anotherCtx, ctxUsernameKey{}, "another_user")

		joinResp, err = server.JoinChallenge(anotherCtx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(4), joinResp.Code, "结束时间精确时刻应该无法参加")
	})

	// 测试结算时间边界
	t.Run("结算时间边界", func(t *testing.T) {
		// 提交成绩
		err := server.SubmitTournamentScore(ctx, "test_tournament", 8000)
		require.NoError(t, err)

		// 结算时间前1秒 - 应该可以手动领取奖励
		server.SetMockTime(baseTime.Add(3*time.Hour - 1*time.Second))

		rewardResp, err := server.GainChallengeReward(ctx, &game.GainChallengeRewardRequest{
			ChallengeId: challengeTemplate.ID,
			RankReward:  false,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), rewardResp.Code, "结算前1秒应该可以手动领取奖励")

		// 结算时间后 - 应该触发邮件发送
		server.SetMockTime(baseTime.Add(3*time.Hour + 1*time.Second))

		// 创建新用户测试邮件发送
		emailUserID := uuid.Must(uuid.NewV4())
		emailCtx := context.WithValue(context.Background(), ctxUserIDKey{}, emailUserID)
		emailCtx = context.WithValue(emailCtx, ctxUsernameKey{}, "email_user")

		// 让这个用户参加比赛并提交成绩（回到比赛时间）
		server.SetMockTime(baseTime.Add(30 * time.Minute))
		joinResp, err := server.JoinChallenge(emailCtx, &game.JoinChallengeRequest{
			ChallengeId: challengeTemplate.ID,
		})
		require.NoError(t, err)

		err = server.SubmitTournamentScore(emailCtx, joinResp.Challenge.TournamentId, 7000)
		require.NoError(t, err)

		// 回到结算时间后
		server.SetMockTime(baseTime.Add(3*time.Hour + 1*time.Second))

		// 调用GetChallenge触发邮件检查
		resp, err := server.GetChallenge(emailCtx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.True(t, resp.GetReward, "结算时间后应该触发邮件奖励发送")
	})
}

// 测试辅助函数
func setupTestServer(t *testing.T) *TestApiServer {
	// 这里应该初始化一个模拟的ApiServer
	// 包括数据库、缓存、模板等
	return &TestApiServer{
		// 初始化测试服务器
	}
}

func teardownTestServer(t *testing.T, server *TestApiServer) {
	// 清理测试资源
	server.Cleanup()
}

// TestApiServer 模拟的测试服务器
type TestApiServer struct {
	*ApiServer
	mockTime   time.Time
	sentEmails map[uuid.UUID][]Email
}

type Email struct {
	Subject string
	Content string
}

func (s *TestApiServer) SetMockTime(t time.Time) {
	s.mockTime = t
}

func (s *TestApiServer) GetSentEmails(userID uuid.UUID) []Email {
	return s.sentEmails[userID]
}

func (s *TestApiServer) SubmitTournamentScore(ctx context.Context, tournamentID string, score int64) error {
	// 模拟提交成绩的逻辑
	return nil
}

func (s *TestApiServer) Cleanup() {
	// 清理测试资源
}
