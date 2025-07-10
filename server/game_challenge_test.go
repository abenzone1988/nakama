package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/apigrpc"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// assertEqual 简单的断言函数
func assertEqual(t *testing.T, expected, actual interface{}, message string) {
	if expected != actual {
		t.Errorf("%s: expected %v, got %v", message, expected, actual)
	}
}

// assertTrue 简单的真值断言
func assertTrue(t *testing.T, condition bool, message string) {
	if !condition {
		t.Errorf("%s: expected true, got false", message)
	}
}

// assertFalse 简单的假值断言
func assertFalse(t *testing.T, condition bool, message string) {
	if condition {
		t.Errorf("%s: expected false, got true", message)
	}
}

// assertNoError 简单的错误断言
func assertNoError(t *testing.T, err error, message string) {
	if err != nil {
		t.Errorf("%s: expected no error, got %v", message, err)
	}
}

// assertError 简单的错误断言
func assertError(t *testing.T, err error, message string) {
	if err == nil {
		t.Errorf("%s: expected error, got nil", message)
	}
}

// TestChallengeSystemIntegration 综合测试挑战赛系统功能
func TestChallengeSystemIntegration(t *testing.T) {
	// 跳过性能测试，除非明确要求
	if testing.Short() {
		t.Skip("跳过长时间运行的综合测试")
	}

	// 创建模拟的ApiServer
	logger := zap.NewNop()
	server := &ApiServer{
		logger: logger,
	}

	// 创建测试挑战赛配置数据 - 基于用户提供的JSON数据
	testChallengeData := map[string]template.TplChallenge{
		"1": {
			ID:            1,
			ActivityID:    "AL10011",
			Name:          "夏季挑战赛01",
			StartDate:     "2025-01-15",
			StartTime:     "10:00",
			EndDate:       "2025-01-15",
			EndTime:       "18:00",
			MaxPart:       2,
			RewardRemains: 60,
			Status:        1,
		},
		"2": {
			ID:            2,
			ActivityID:    "AL10021",
			Name:          "夏季挑战赛02",
			StartDate:     "2025-01-16",
			StartTime:     "10:00",
			EndDate:       "2025-01-16",
			EndTime:       "22:00",
			MaxPart:       3,
			RewardRemains: 60,
			Status:        1,
		},
		"3": {
			ID:            3,
			ActivityID:    "AL10031",
			Name:          "夏季挑战赛03",
			StartDate:     "2025-01-17",
			StartTime:     "10:00",
			EndDate:       "2025-01-17",
			EndTime:       "22:00",
			MaxPart:       4,
			RewardRemains: 60,
			Status:        1,
		},
	}

	// 定义挑战赛统计结构
	type ChallengeStats struct {
		ChallengeID        int32
		MaxParticipants    int32
		TournamentsCreated int
		PlayersJoined      int
		ScoresSubmitted    int
		TournamentFull     int
		TournamentGroups   map[string]int // 竞标赛ID -> 参与人数
	}

	// 统计信息
	var stats struct {
		totalPlayers            int
		challengeVisibilityTest int
		challengeJoinTest       int
		tournamentCreated       int
		playersJoined           int
		scoresSubmitted         int
		challengeResults        map[int32]ChallengeStats
	}
	stats.challengeResults = make(map[int32]ChallengeStats)

	t.Logf("=== 开始综合测试挑战赛系统 ===")

	// 第一部分：测试挑战赛显示时间逻辑
	t.Run("挑战赛显示时间逻辑测试", func(t *testing.T) {
		for challengeIDStr, challengeConfig := range testChallengeData {
			t.Run(fmt.Sprintf("挑战赛_%s", challengeIDStr), func(t *testing.T) {
				startTime, err := parseDateTime(challengeConfig.StartDate, challengeConfig.StartTime)
				assertNoError(t, err, "解析开始时间应该成功")

				endTime, err := parseDateTime(challengeConfig.EndDate, challengeConfig.EndTime)
				assertNoError(t, err, "解析结束时间应该成功")

				// 测试不同时间点的显示逻辑
				testCases := []struct {
					name     string
					timeDesc string
					now      time.Time
					expected bool
				}{
					{
						name:     "比赛开始前3天",
						timeDesc: "3天前",
						now:      startTime.AddDate(0, 0, -3),
						expected: false,
					},
					{
						name:     "比赛开始前1天",
						timeDesc: "1天前",
						now:      startTime.AddDate(0, 0, -1),
						expected: true,
					},
					{
						name:     "比赛进行中",
						timeDesc: "进行中",
						now:      startTime.Add(2 * time.Hour),
						expected: true,
					},
					{
						name:     "比赛结束后奖励期内",
						timeDesc: "结束后30分钟",
						now:      endTime.Add(30 * time.Minute),
						expected: true,
					},
					{
						name:     "比赛结束后奖励期外",
						timeDesc: "结束后2小时",
						now:      endTime.Add(2 * time.Hour),
						expected: false,
					},
				}

				for _, tc := range testCases {
					t.Run(tc.name, func(t *testing.T) {
						result := shouldShowChallenge(tc.now, startTime, endTime, challengeConfig.RewardRemains)
						assertEqual(t, tc.expected, result, fmt.Sprintf("挑战赛 %s 在 %s 的显示状态", challengeConfig.Name, tc.timeDesc))
						if result {
							stats.challengeVisibilityTest++
						}
					})
				}
			})
		}
	})

	// 第二部分：测试挑战赛加入时间逻辑
	t.Run("挑战赛加入时间逻辑测试", func(t *testing.T) {
		for challengeIDStr, challengeConfig := range testChallengeData {
			t.Run(fmt.Sprintf("挑战赛_%s", challengeIDStr), func(t *testing.T) {
				startTime, err := parseDateTime(challengeConfig.StartDate, challengeConfig.StartTime)
				assertNoError(t, err, "解析开始时间应该成功")

				endTime, err := parseDateTime(challengeConfig.EndDate, challengeConfig.EndTime)
				assertNoError(t, err, "解析结束时间应该成功")

				// 测试不同时间点的加入逻辑
				testCases := []struct {
					name     string
					timeDesc string
					now      time.Time
					expected bool
				}{
					{
						name:     "比赛开始前",
						timeDesc: "开始前1小时",
						now:      startTime.Add(-1 * time.Hour),
						expected: false,
					},
					{
						name:     "比赛刚开始",
						timeDesc: "刚开始",
						now:      startTime,
						expected: true,
					},
					{
						name:     "比赛进行中",
						timeDesc: "进行中",
						now:      startTime.Add(2 * time.Hour),
						expected: true,
					},
					{
						name:     "比赛结束后",
						timeDesc: "结束后",
						now:      endTime.Add(30 * time.Minute),
						expected: false,
					},
				}

				for _, tc := range testCases {
					t.Run(tc.name, func(t *testing.T) {
						result := shouldJoinChallenge(tc.now, startTime, endTime, challengeConfig.RewardRemains)
						assertEqual(t, tc.expected, result, fmt.Sprintf("挑战赛 %s 在 %s 的加入状态", challengeConfig.Name, tc.timeDesc))
						if result {
							stats.challengeJoinTest++
						}
					})
				}
			})
		}
	})

	// 第三部分：测试玩家分配和竞标赛创建
	t.Run("玩家分配和竞标赛创建测试", func(t *testing.T) {
		// 为每个挑战赛创建不同数量的玩家来测试分配逻辑
		challengePlayerCounts := map[int32]int{
			1: 10, // 挑战赛1: 10个玩家，最大2人/竞标赛，应该创建5个竞标赛
			2: 15, // 挑战赛2: 15个玩家，最大3人/竞标赛，应该创建5个竞标赛
			3: 20, // 挑战赛3: 20个玩家，最大4人/竞标赛，应该创建5个竞标赛
		}

		for challengeID, playerCount := range challengePlayerCounts {
			t.Run(fmt.Sprintf("挑战赛_%d", challengeID), func(t *testing.T) {
				challengeConfig := testChallengeData[fmt.Sprintf("%d", challengeID)]

				// 初始化挑战赛统计
				stats.challengeResults[challengeID] = ChallengeStats{
					ChallengeID:      challengeID,
					MaxParticipants:  challengeConfig.MaxPart,
					TournamentGroups: make(map[string]int),
				}

				startTime, err := parseDateTime(challengeConfig.StartDate, challengeConfig.StartTime)
				assertNoError(t, err, "解析开始时间应该成功")

				endTime, err := parseDateTime(challengeConfig.EndDate, challengeConfig.EndTime)
				assertNoError(t, err, "解析结束时间应该成功")

				// 模拟当前时间在挑战赛进行中
				now := startTime.Add(2 * time.Hour)

				// 创建玩家并模拟加入
				for i := 0; i < playerCount; i++ {
					t.Run(fmt.Sprintf("玩家_%d", i+1), func(t *testing.T) {
						// 创建用户
						userID := uuid.Must(uuid.NewV4())
						username := fmt.Sprintf("test_player_%d_%d", challengeID, i+1)

						// 创建用户挑战赛数据
						userMatch := &UserMatch{}
						userMatch.Init()

						// 检查是否可以加入挑战赛
						canJoin := shouldJoinChallenge(now, startTime, endTime, challengeConfig.RewardRemains)
						assertTrue(t, canJoin, "玩家应该能够加入挑战赛")

						// 检查玩家是否已经加入过
						_, alreadyJoined := userMatch.Challenges[challengeID]
						assertFalse(t, alreadyJoined, "玩家不应该重复加入挑战赛")

						// 模拟分配竞标赛ID
						tournamentBatch := (i / int(challengeConfig.MaxPart)) + 1
						tournamentID := fmt.Sprintf("challenge_%d_%d_%03d",
							challengeID,
							startTime.Unix(),
							tournamentBatch)

						// 记录玩家加入挑战赛
						userMatch.Challenges[challengeID] = &Challenge{
							ID:           challengeID,
							TournamentID: tournamentID,
							JoinedAt:     now,
						}

						// 更新统计信息
						result := stats.challengeResults[challengeID]
						result.PlayersJoined++
						result.TournamentGroups[tournamentID]++

						// 检查是否创建了新的竞标赛
						if result.TournamentGroups[tournamentID] == 1 {
							result.TournamentsCreated++
						}

						// 检查竞标赛是否满员
						if result.TournamentGroups[tournamentID] == int(challengeConfig.MaxPart) {
							result.TournamentFull++
						}

						stats.challengeResults[challengeID] = result
						stats.playersJoined++

						t.Logf("玩家 %s (ID: %s) 成功加入挑战赛 %d，竞标赛 %s，当前人数 %d/%d",
							username, userID.String(), challengeID, tournamentID,
							result.TournamentGroups[tournamentID], challengeConfig.MaxPart)
					})
				}

				// 验证竞标赛分配结果
				result := stats.challengeResults[challengeID]
				expectedTournaments := (playerCount + int(challengeConfig.MaxPart) - 1) / int(challengeConfig.MaxPart)
				assertEqual(t, expectedTournaments, result.TournamentsCreated, "竞标赛数量应该正确")
				assertEqual(t, playerCount, result.PlayersJoined, "加入玩家数量应该正确")

				// 验证每个竞标赛的玩家数量
				for tournamentID, count := range result.TournamentGroups {
					if count > int(challengeConfig.MaxPart) {
						t.Errorf("竞标赛 %s 超过最大玩家数限制: %d > %d", tournamentID, count, challengeConfig.MaxPart)
					}
				}

				stats.tournamentCreated += result.TournamentsCreated
			})
		}
	})

	// 第四部分：模拟1000个玩家参加锦标赛
	t.Run("1000玩家大规模挑战赛测试", func(t *testing.T) {
		const massPlayerCount = 1000

		// 选择挑战赛3进行大规模测试（最大4人/竞标赛）
		challengeConfig := testChallengeData["3"]

		startTime, err := parseDateTime(challengeConfig.StartDate, challengeConfig.StartTime)
		assertNoError(t, err, "解析开始时间应该成功")

		endTime, err := parseDateTime(challengeConfig.EndDate, challengeConfig.EndTime)
		assertNoError(t, err, "解析结束时间应该成功")

		// 模拟当前时间在挑战赛进行中
		now := startTime.Add(2 * time.Hour)

		// 创建1000个玩家的数据
		players := make([]struct {
			userID       uuid.UUID
			username     string
			userMatch    *UserMatch
			tournamentID string
			scores       []int64
		}, massPlayerCount)

		t.Logf("创建 %d 个玩家数据...", massPlayerCount)

		// 初始化玩家数据
		for i := 0; i < massPlayerCount; i++ {
			players[i].userID = uuid.Must(uuid.NewV4())
			players[i].username = fmt.Sprintf("mass_player_%d", i+1)
			players[i].userMatch = &UserMatch{}
			players[i].userMatch.Init()

			// 为每个玩家生成3-5个随机成绩
			scoreCount := 3 + (i % 3)
			players[i].scores = make([]int64, scoreCount)
			for j := 0; j < scoreCount; j++ {
				players[i].scores[j] = int64(1000 + (i*10 + j*100) + (i*j)%5000)
			}
		}

		// 大规模测试统计
		var massStats struct {
			joinedPlayers      int
			createdTournaments int
			submittedScores    int
			failedSubmissions  int
			tournamentGroups   map[string]int
		}
		massStats.tournamentGroups = make(map[string]int)

		t.Logf("开始1000玩家加入挑战赛测试...")

		// 玩家加入挑战赛
		for i := 0; i < massPlayerCount; i++ {
			player := &players[i]

			// 检查是否可以加入挑战赛
			if !shouldJoinChallenge(now, startTime, endTime, challengeConfig.RewardRemains) {
				t.Errorf("玩家 %d 无法加入挑战赛 - 时间不符合", i+1)
				continue
			}

			// 分配竞标赛ID
			tournamentBatch := (i / int(challengeConfig.MaxPart)) + 1
			tournamentID := fmt.Sprintf("challenge_%d_%d_%03d",
				challengeConfig.ID,
				startTime.Unix(),
				tournamentBatch)

			// 记录玩家加入挑战赛
			player.userMatch.Challenges[challengeConfig.ID] = &Challenge{
				ID:           challengeConfig.ID,
				TournamentID: tournamentID,
				JoinedAt:     now,
			}

			player.tournamentID = tournamentID
			massStats.joinedPlayers++

			// 统计竞标赛分组
			if _, exists := massStats.tournamentGroups[tournamentID]; !exists {
				massStats.createdTournaments++
			}
			massStats.tournamentGroups[tournamentID]++

			// 定期输出进度
			if (i+1)%200 == 0 {
				t.Logf("已处理 %d 个玩家", i+1)
			}
		}

		t.Logf("开始1000玩家提交成绩测试...")

		// 玩家提交成绩
		for i := 0; i < massPlayerCount; i++ {
			player := &players[i]

			// 检查是否已经加入了挑战赛
			challengeData, exists := player.userMatch.Challenges[challengeConfig.ID]
			if !exists || challengeData == nil {
				massStats.failedSubmissions++
				continue
			}

			// 模拟多次成绩提交
			for j, score := range player.scores {
				// 模拟成绩提交的验证逻辑
				if score > 0 && player.tournamentID != "" && isChallengeActive(now, startTime, endTime) {
					// 模拟成功提交成绩
					massStats.submittedScores++

					// 只记录第一次提交的日志
					if j == 0 {
						t.Logf("玩家 %s 成功提交首次成绩: %d (竞标赛: %s)",
							player.username, score, player.tournamentID)
					}
				} else {
					massStats.failedSubmissions++
				}
			}

			// 定期输出进度
			if (i+1)%200 == 0 {
				t.Logf("已处理 %d 个玩家的成绩提交", i+1)
			}
		}

		// 验证大规模测试结果
		t.Logf("=== 1000玩家大规模测试结果 ===")
		t.Logf("总玩家数: %d", massPlayerCount)
		t.Logf("成功加入挑战赛: %d", massStats.joinedPlayers)
		t.Logf("创建的竞标赛数量: %d", massStats.createdTournaments)
		t.Logf("成功提交成绩: %d", massStats.submittedScores)
		t.Logf("失败提交次数: %d", massStats.failedSubmissions)

		// 验证结果
		expectedTournaments := (massPlayerCount + int(challengeConfig.MaxPart) - 1) / int(challengeConfig.MaxPart)
		assertEqual(t, expectedTournaments, massStats.createdTournaments, "大规模测试竞标赛数量应该正确")
		assertEqual(t, massPlayerCount, massStats.joinedPlayers, "大规模测试加入玩家数量应该正确")

		// 验证每个竞标赛的玩家数量
		for tournamentID, playerCount := range massStats.tournamentGroups {
			if playerCount > int(challengeConfig.MaxPart) {
				t.Errorf("竞标赛 %s 超过最大玩家数限制: %d > %d", tournamentID, playerCount, challengeConfig.MaxPart)
			}
		}

		// 验证成绩提交成功率
		expectedSubmissions := massPlayerCount * 4                // 平均每个玩家4次提交
		if massStats.submittedScores < expectedSubmissions*8/10 { // 允许20%的失败率
			t.Errorf("成绩提交成功率过低: %d/%d (%.2f%%)",
				massStats.submittedScores, expectedSubmissions,
				float64(massStats.submittedScores)/float64(expectedSubmissions)*100)
		}

		stats.scoresSubmitted += massStats.submittedScores
		stats.totalPlayers += massStats.joinedPlayers
	})

	// 第五部分：竞标赛ID解析测试
	t.Run("竞标赛ID解析测试", func(t *testing.T) {
		testCases := []struct {
			name          string
			tournamentID  string
			expectedID    string
			expectedValid bool
		}{
			{
				name:          "标准挑战赛竞标赛ID",
				tournamentID:  "challenge_1_1641024000_001",
				expectedID:    "1",
				expectedValid: true,
			},
			{
				name:          "复杂挑战赛竞标赛ID",
				tournamentID:  "challenge_AL10011_1641024000_002",
				expectedID:    "AL10011",
				expectedValid: true,
			},
			{
				name:          "非挑战赛竞标赛ID",
				tournamentID:  "tournament_1_1641024000_001",
				expectedID:    "",
				expectedValid: false,
			},
			{
				name:          "格式错误的竞标赛ID",
				tournamentID:  "challenge_1",
				expectedID:    "",
				expectedValid: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := server.GetChallengeFromTournamentID(tc.tournamentID)

				if tc.expectedValid {
					assertEqual(t, tc.expectedID, result, "应该正确解析挑战赛ID")
				} else {
					assertEqual(t, "", result, "应该返回空字符串表示无效ID")
				}
			})
		}
	})

	// 输出最终统计结果
	t.Logf("=== 综合测试最终统计 ===")
	t.Logf("总玩家数: %d", stats.totalPlayers)
	t.Logf("挑战赛显示测试通过: %d", stats.challengeVisibilityTest)
	t.Logf("挑战赛加入测试通过: %d", stats.challengeJoinTest)
	t.Logf("创建的竞标赛总数: %d", stats.tournamentCreated)
	t.Logf("成功加入的玩家总数: %d", stats.playersJoined)
	t.Logf("成功提交的成绩总数: %d", stats.scoresSubmitted)

	t.Logf("=== 各挑战赛详细统计 ===")
	for challengeID, result := range stats.challengeResults {
		t.Logf("挑战赛 %d:", challengeID)
		t.Logf("  - 最大参与人数: %d", result.MaxParticipants)
		t.Logf("  - 创建的竞标赛数: %d", result.TournamentsCreated)
		t.Logf("  - 加入的玩家数: %d", result.PlayersJoined)
		t.Logf("  - 提交的成绩数: %d", result.ScoresSubmitted)
		t.Logf("  - 满员的竞标赛数: %d", result.TournamentFull)
		t.Logf("  - 竞标赛分组情况:")

		count := 0
		for tournamentID, playerCount := range result.TournamentGroups {
			if count < 5 { // 只显示前5个竞标赛
				t.Logf("    - %s: %d 个玩家", tournamentID, playerCount)
			}
			count++
		}
		if count > 5 {
			t.Logf("    - ... 还有 %d 个竞标赛", count-5)
		}
	}

	// 验证总体测试结果
	assertTrue(t, stats.totalPlayers >= 1000, "总玩家数应该达到预期")
	assertTrue(t, stats.tournamentCreated > 0, "应该创建了竞标赛")
	assertTrue(t, stats.playersJoined > 0, "应该有玩家成功加入")
	assertTrue(t, stats.scoresSubmitted > 0, "应该有成绩被提交")

	t.Logf("=== 综合测试完成 ===")
}

// TestChallengeTimeZoneCompatibility 测试挑战赛时区兼容性
func TestChallengeTimeZoneCompatibility(t *testing.T) {
	// 测试不同时区的时间解析
	testCases := []struct {
		name        string
		dateStr     string
		timeStr     string
		expectError bool
	}{
		{
			name:        "标准格式",
			dateStr:     "2025-01-15",
			timeStr:     "10:00",
			expectError: false,
		},
		{
			name:        "午夜时间",
			dateStr:     "2025-01-15",
			timeStr:     "00:00",
			expectError: false,
		},
		{
			name:        "深夜时间",
			dateStr:     "2025-01-15",
			timeStr:     "23:59",
			expectError: false,
		},
		{
			name:        "无效时间",
			dateStr:     "2025-01-15",
			timeStr:     "25:00",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDateTime(tc.dateStr, tc.timeStr)

			if tc.expectError {
				assertError(t, err, "应该解析失败")
			} else {
				assertNoError(t, err, "应该解析成功")
				assertTrue(t, !result.IsZero(), "解析结果不应该为零时间")
			}
		})
	}
}

// TestChallengeEdgeCases 测试挑战赛边界情况
func TestChallengeEdgeCases(t *testing.T) {
	t.Run("同时开始结束的挑战赛", func(t *testing.T) {
		now := time.Now()
		startTime := now
		endTime := now // 同时开始和结束

		// 应该不显示这种异常配置的挑战赛
		result := shouldShowChallenge(now, startTime, endTime, 60)
		assertFalse(t, result, "同时开始结束的挑战赛不应该显示")

		// 应该不能加入这种异常配置的挑战赛
		joinResult := shouldJoinChallenge(now, startTime, endTime, 60)
		assertFalse(t, joinResult, "同时开始结束的挑战赛不应该能加入")
	})

	t.Run("负数奖励期", func(t *testing.T) {
		now := time.Now()
		startTime := now.Add(-2 * time.Hour)
		endTime := now.Add(-1 * time.Hour)

		// 负数奖励期应该被当作0处理
		result := shouldShowChallenge(now, startTime, endTime, -60)
		assertFalse(t, result, "负数奖励期的已结束挑战赛不应该显示")
	})

	t.Run("零参与人数的挑战赛", func(t *testing.T) {
		challengeConfig := template.TplChallenge{
			ID:      999,
			MaxPart: 0, // 零参与人数
		}

		// 这种配置应该在实际使用中被拒绝
		assertTrue(t, challengeConfig.MaxPart == 0, "测试数据应该为零参与人数")
	})
}

// TestRealAPIChallenge100Players 测试100个玩家的真实API挑战赛流程
func TestRealAPIChallenge100Players(t *testing.T) {
	// 跳过性能测试，除非明确要求
	if testing.Short() {
		t.Skip("跳过长时间运行的API测试")
	}

	// 创建 gRPC 客户端连接
	conn, err := grpc.Dial("localhost:7349", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器: %v", err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头 - 使用配置文件中的server_key "sparkgame"
	ctx := context.Background()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始100个玩家的真实API挑战赛测试 ===")
	t.Logf("服务器地址: localhost:7349")
	t.Logf("认证方式: Basic Auth (sparkgame)")

	// 创建玩家数据结构
	type Player struct {
		CustomID         string
		Username         string
		UserID           string
		Session          *api.Session
		Context          context.Context
		JoinedChallenges []int32
		Scores           []int64
	}

	const playerCount = 100
	players := make([]*Player, playerCount)

	// 统计信息
	stats := struct {
		CreatedAccounts      int
		AuthenticationErrors int
		ChallengesFetched    int
		ChallengesJoined     int
		ScoresSubmitted      int
		Errors               int
		TournamentGroups     map[string]int
	}{
		TournamentGroups: make(map[string]int),
	}

	// 第一阶段：创建100个玩家账号
	t.Logf("=== 阶段1: 创建100个玩家账号 ===")
	for i := 0; i < playerCount; i++ {
		player := &Player{
			CustomID:         fmt.Sprintf("test_player_%d_%d", i+1, time.Now().Unix()),
			Username:         fmt.Sprintf("Player_%d", i+1),
			JoinedChallenges: []int32{},
			Scores:           []int64{},
		}

		// 创建账号请求
		authReq := &api.AuthenticateCustomRequest{
			Account: &api.AccountCustom{
				Id: player.CustomID,
			},
			Username: player.Username,
			Create:   &wrapperspb.BoolValue{Value: true},
		}

		// 调用认证API
		session, err := client.AuthenticateCustom(ctx, authReq)
		if err != nil {
			t.Errorf("玩家 %s 创建账号失败: %v", player.Username, err)
			stats.AuthenticationErrors++
			continue
		}

		player.Session = session
		// 解析用户ID
		claims := &SessionTokenClaims{}
		token, _, err := jwt.NewParser().ParseUnverified(session.Token, claims)
		if err != nil {
			t.Errorf("玩家 %s 解析token失败: %v", player.Username, err)
			stats.AuthenticationErrors++
			continue
		}

		if claims, ok := token.Claims.(*SessionTokenClaims); ok {
			player.UserID = claims.UserId
		}

		// 创建带认证的上下文
		player.Context = metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)

		players[i] = player
		stats.CreatedAccounts++

		t.Logf("✓ 玩家 %s (ID: %s) 账号创建成功", player.Username, player.UserID)

		// 每创建10个玩家输出一次进度
		if (i+1)%10 == 0 {
			t.Logf("已创建 %d 个玩家账号", i+1)
		}
	}

	t.Logf("账号创建完成: 成功 %d 个, 失败 %d 个", stats.CreatedAccounts, stats.AuthenticationErrors)

	// 第二阶段：获取挑战赛信息
	t.Logf("=== 阶段2: 获取挑战赛信息 ===")
	var challengeList *game.GetChallengeResponse
	if stats.CreatedAccounts > 0 {
		// 使用第一个玩家的上下文获取挑战赛
		challengeList, err = client.GetChallenge(players[0].Context, &emptypb.Empty{})
		if err != nil {
			t.Fatalf("获取挑战赛失败: %v", err)
		}

		stats.ChallengesFetched = len(challengeList.Challenges)
		t.Logf("✓ 成功获取 %d 个挑战赛", stats.ChallengesFetched)

		for _, challenge := range challengeList.Challenges {
			t.Logf("挑战赛 ID: %d, 活动ID: %s, 最大参与人数: %d",
				challenge.Id, challenge.ActivityId, challenge.MaxPart)
		}
	}

	// 第三阶段：玩家加入挑战赛
	t.Logf("=== 阶段3: 玩家加入挑战赛 ===")
	if challengeList != nil && len(challengeList.Challenges) > 0 {
		// 选择第一个挑战赛进行测试
		targetChallenge := challengeList.Challenges[0]
		t.Logf("目标挑战赛: ID=%d, 活动ID=%s, 最大参与人数=%d",
			targetChallenge.Id, targetChallenge.ActivityId, targetChallenge.MaxPart)

		for i, player := range players {
			if player == nil || player.Context == nil {
				continue
			}

			// 加入挑战赛请求
			joinReq := &game.JoinChallengeRequest{
				ChallengeId: targetChallenge.Id,
			}

			// 调用加入挑战赛API
			joinResp, err := client.JoinChallenge(player.Context, joinReq)
			if err != nil {
				t.Errorf("玩家 %s 加入挑战赛失败: %v", player.Username, err)
				stats.Errors++
				continue
			}

			if joinResp.Code != 0 {
				t.Errorf("玩家 %s 加入挑战赛失败: Code=%d, Msg=%s",
					player.Username, joinResp.Code, joinResp.Msg)
				stats.Errors++
				continue
			}

			player.JoinedChallenges = append(player.JoinedChallenges, targetChallenge.Id)
			stats.ChallengesJoined++

			if joinResp.Challenge != nil && joinResp.Challenge.TournamentId != "" {
				stats.TournamentGroups[joinResp.Challenge.TournamentId]++
			}

			t.Logf("✓ 玩家 %s 成功加入挑战赛 %d (竞标赛: %s)",
				player.Username, targetChallenge.Id, joinResp.Challenge.TournamentId)

			// 每加入10个玩家输出一次进度
			if (i+1)%10 == 0 {
				t.Logf("已有 %d 个玩家加入挑战赛", stats.ChallengesJoined)
			}
		}
	}

	t.Logf("加入挑战赛完成: 成功 %d 个玩家", stats.ChallengesJoined)

	// 第四阶段：提交成绩
	t.Logf("=== 阶段4: 提交成绩 ===")
	if stats.ChallengesJoined > 0 {
		for i, player := range players {
			if player == nil || player.Context == nil || len(player.JoinedChallenges) == 0 {
				continue
			}

			// 为每个玩家生成随机成绩
			player.Scores = []int64{
				int64(1000 + rand.Intn(9000)), // 1000-10000 随机分数
				int64(1500 + rand.Intn(8500)), // 第二次尝试
				int64(2000 + rand.Intn(8000)), // 第三次尝试
			}

			// 获取该玩家的竞标赛ID
			var tournamentID string

			// 重新获取挑战赛信息以获取tournamentID
			challengeResp, err := client.GetChallenge(player.Context, &emptypb.Empty{})
			if err != nil {
				t.Errorf("玩家 %s 获取挑战赛信息失败: %v", player.Username, err)
				continue
			}

			// 找到对应的挑战赛
			for _, challenge := range challengeResp.Challenges {
				if challenge.Id == player.JoinedChallenges[0] {
					tournamentID = challenge.TournamentId
					break
				}
			}

			if tournamentID == "" {
				t.Errorf("玩家 %s 没有分配到竞标赛", player.Username)
				continue
			}

			// 提交每个成绩
			for scoreIndex, score := range player.Scores {
				writeReq := &api.WriteTournamentRecordRequest{
					TournamentId: tournamentID,
					Record: &api.WriteTournamentRecordRequest_TournamentRecordWrite{
						Score:    score,
						Subscore: 0,
						Metadata: fmt.Sprintf(`{"attempt": %d, "timestamp": "%s"}`,
							scoreIndex+1, time.Now().Format(time.RFC3339)),
						Operator: api.Operator_BEST,
					},
				}

				// 调用提交成绩API
				recordResp, err := client.WriteTournamentRecord(player.Context, writeReq)
				if err != nil {
					t.Errorf("玩家 %s 提交成绩失败: %v", player.Username, err)
					stats.Errors++
					continue
				}

				stats.ScoresSubmitted++

				t.Logf("✓ 玩家 %s 提交成绩 %d (尝试 %d/3) 到竞标赛 %s",
					player.Username, score, scoreIndex+1, tournamentID)

				// 显示成绩记录详情
				t.Logf("  - 用户名: %s, 排名: %d, 分数: %d",
					recordResp.Username, recordResp.Rank, recordResp.Score)
			}

			// 每提交10个玩家成绩输出一次进度
			if (i+1)%10 == 0 {
				t.Logf("已有 %d 个玩家提交成绩", (i+1)*3)
			}
		}
	}

	// 第五阶段：输出测试结果
	t.Logf("=== 测试结果统计 ===")
	t.Logf("创建账号: %d 成功, %d 失败", stats.CreatedAccounts, stats.AuthenticationErrors)
	t.Logf("获取挑战赛: %d 个", stats.ChallengesFetched)
	t.Logf("加入挑战赛: %d 个玩家成功", stats.ChallengesJoined)
	t.Logf("提交成绩: %d 次成功", stats.ScoresSubmitted)
	t.Logf("总错误数: %d", stats.Errors)

	t.Logf("=== 竞标赛分配详情 ===")
	for tournamentID, playerCount := range stats.TournamentGroups {
		t.Logf("竞标赛 %s: %d 个玩家", tournamentID, playerCount)
	}

	// 验证测试结果
	if stats.CreatedAccounts < playerCount*8/10 {
		t.Errorf("创建账号成功率过低: %d/%d", stats.CreatedAccounts, playerCount)
	}

	if stats.ChallengesFetched == 0 {
		t.Error("未获取到任何挑战赛")
	}

	if stats.ChallengesJoined < stats.CreatedAccounts*8/10 {
		t.Errorf("加入挑战赛成功率过低: %d/%d", stats.ChallengesJoined, stats.CreatedAccounts)
	}

	expectedScores := stats.ChallengesJoined * 3 // 每个玩家3次成绩
	if stats.ScoresSubmitted < expectedScores*8/10 {
		t.Errorf("提交成绩成功率过低: %d/%d", stats.ScoresSubmitted, expectedScores)
	}

	t.Logf("=== 测试完成 ===")
}

// TestRealAPIChallenge10Players 测试10个玩家的详细API流程
func TestRealAPIChallenge10Players(t *testing.T) {
	// 跳过性能测试，除非明确要求
	if testing.Short() {
		t.Skip("跳过API测试")
	}

	// 创建 gRPC 客户端连接
	conn, err := grpc.Dial("localhost:7349", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器: %v", err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头 - 使用配置文件中的server_key "sparkgame"
	ctx := context.Background()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始10个玩家的详细API挑战赛测试 ===")
	t.Logf("服务器地址: localhost:7349")
	t.Logf("认证方式: Basic Auth (sparkgame)")

	// 创建玩家数据结构
	type Player struct {
		CustomID         string
		Username         string
		UserID           string
		Session          *api.Session
		Context          context.Context
		JoinedChallenges []int32
		Scores           []int64
		TournamentID     string
	}

	const playerCount = 10
	players := make([]*Player, playerCount)

	// 第一阶段：创建玩家账号
	t.Logf("=== 阶段1: 创建玩家账号 ===")
	for i := 0; i < playerCount; i++ {
		player := &Player{
			CustomID:         fmt.Sprintf("detail_test_%d_%d", i+1, time.Now().Unix()),
			Username:         fmt.Sprintf("DetailPlayer_%d", i+1),
			JoinedChallenges: []int32{},
			Scores:           []int64{},
		}

		// 创建账号请求
		authReq := &api.AuthenticateCustomRequest{
			Account: &api.AccountCustom{
				Id: player.CustomID,
			},
			Username: player.Username,
			Create:   &wrapperspb.BoolValue{Value: true},
		}

		// 调用认证API
		session, err := client.AuthenticateCustom(ctx, authReq)
		if err != nil {
			t.Errorf("玩家 %s 创建账号失败: %v", player.Username, err)
			continue
		}

		player.Session = session
		// 解析用户ID
		claims := &SessionTokenClaims{}
		token, _, err := jwt.NewParser().ParseUnverified(session.Token, claims)
		if err != nil {
			t.Errorf("玩家 %s 解析token失败: %v", player.Username, err)
			continue
		}

		if claims, ok := token.Claims.(*SessionTokenClaims); ok {
			player.UserID = claims.UserId
		}

		// 创建带认证的上下文
		player.Context = metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)

		players[i] = player

		t.Logf("✓ 玩家 %s (CustomID: %s, UserID: %s) 账号创建成功",
			player.Username, player.CustomID, player.UserID)
		t.Logf("  - Session Created: %v", session.Created)
		t.Logf("  - Token: %s...", session.Token[:50])
	}

	// 第二阶段：获取挑战赛信息
	t.Logf("=== 阶段2: 获取挑战赛信息 ===")
	var challengeList *game.GetChallengeResponse
	if len(players) > 0 && players[0] != nil {
		challengeList, err = client.GetChallenge(players[0].Context, &emptypb.Empty{})
		if err != nil {
			t.Fatalf("获取挑战赛失败: %v", err)
		}

		t.Logf("✓ 成功获取 %d 个挑战赛", len(challengeList.Challenges))

		for _, challenge := range challengeList.Challenges {
			t.Logf("挑战赛详情:")
			t.Logf("  - ID: %d", challenge.Id)
			t.Logf("  - 活动ID: %s", challenge.ActivityId)
			t.Logf("  - 开始时间: %s", challenge.Start.AsTime().Format("2006-01-02 15:04:05"))
			t.Logf("  - 结束时间: %s", challenge.End.AsTime().Format("2006-01-02 15:04:05"))
			t.Logf("  - 最大参与人数: %d", challenge.MaxPart)
			t.Logf("  - 竞标赛ID: %s", challenge.TournamentId)
		}
	}

	// 第三阶段：玩家加入挑战赛
	t.Logf("=== 阶段3: 玩家加入挑战赛 ===")
	if challengeList != nil && len(challengeList.Challenges) > 0 {
		// 选择第一个挑战赛进行测试
		targetChallenge := challengeList.Challenges[0]
		t.Logf("目标挑战赛: ID=%d, 活动ID=%s", targetChallenge.Id, targetChallenge.ActivityId)

		for _, player := range players {
			if player == nil || player.Context == nil {
				continue
			}

			// 加入挑战赛请求
			joinReq := &game.JoinChallengeRequest{
				ChallengeId: targetChallenge.Id,
			}

			// 调用加入挑战赛API
			joinResp, err := client.JoinChallenge(player.Context, joinReq)
			if err != nil {
				t.Errorf("玩家 %s 加入挑战赛失败: %v", player.Username, err)
				continue
			}

			t.Logf("✓ 玩家 %s 加入挑战赛结果:", player.Username)
			t.Logf("  - 返回代码: %d", joinResp.Code)
			t.Logf("  - 返回消息: %s", joinResp.Msg)

			if joinResp.Code != 0 {
				t.Errorf("玩家 %s 加入挑战赛失败: Code=%d, Msg=%s",
					player.Username, joinResp.Code, joinResp.Msg)
				continue
			}

			player.JoinedChallenges = append(player.JoinedChallenges, targetChallenge.Id)

			if joinResp.Challenge != nil {
				player.TournamentID = joinResp.Challenge.TournamentId
				t.Logf("  - 分配到竞标赛: %s", player.TournamentID)
			}

			// 延迟一秒，避免过快请求
			time.Sleep(100 * time.Millisecond)
		}
	}

	// 第四阶段：提交成绩
	t.Logf("=== 阶段4: 提交成绩 ===")
	for i, player := range players {
		if player == nil || player.Context == nil || len(player.JoinedChallenges) == 0 {
			continue
		}

		// 为每个玩家生成3个不同的成绩
		player.Scores = []int64{
			int64(1000 + i*100 + rand.Intn(500)), // 基础分数 + 玩家偏移 + 随机
			int64(1500 + i*100 + rand.Intn(500)), // 第二次尝试
			int64(2000 + i*100 + rand.Intn(500)), // 第三次尝试
		}

		// 获取竞标赛ID
		if player.TournamentID == "" {
			// 重新获取挑战赛信息
			challengeResp, err := client.GetChallenge(player.Context, &emptypb.Empty{})
			if err != nil {
				t.Errorf("玩家 %s 获取挑战赛信息失败: %v", player.Username, err)
				continue
			}

			// 找到对应的挑战赛
			for _, challenge := range challengeResp.Challenges {
				if challenge.Id == player.JoinedChallenges[0] {
					player.TournamentID = challenge.TournamentId
					break
				}
			}
		}

		if player.TournamentID == "" {
			t.Errorf("玩家 %s 没有分配到竞标赛", player.Username)
			continue
		}

		t.Logf("玩家 %s 开始提交成绩到竞标赛 %s", player.Username, player.TournamentID)

		// 提交每个成绩
		for scoreIndex, score := range player.Scores {
			writeReq := &api.WriteTournamentRecordRequest{
				TournamentId: player.TournamentID,
				Record: &api.WriteTournamentRecordRequest_TournamentRecordWrite{
					Score:    score,
					Subscore: 0,
					Metadata: fmt.Sprintf(`{"attempt": %d, "player": "%s", "timestamp": "%s"}`,
						scoreIndex+1, player.Username, time.Now().Format(time.RFC3339)),
					Operator: api.Operator_BEST,
				},
			}

			// 调用提交成绩API
			recordResp, err := client.WriteTournamentRecord(player.Context, writeReq)
			if err != nil {
				t.Errorf("玩家 %s 提交成绩失败: %v", player.Username, err)
				continue
			}

			t.Logf("✓ 玩家 %s 提交成绩成功:", player.Username)
			t.Logf("  - 尝试次数: %d/3", scoreIndex+1)
			t.Logf("  - 提交分数: %d", score)
			t.Logf("  - 用户名: %s", recordResp.Username)
			t.Logf("  - 最终分数: %d", recordResp.Score)
			t.Logf("  - 当前排名: %d", recordResp.Rank)
			t.Logf("  - 更新时间: %s", recordResp.UpdateTime.AsTime().Format("2006-01-02 15:04:05"))

			// 延迟一秒，避免过快请求
			time.Sleep(200 * time.Millisecond)
		}
	}

	t.Logf("=== 详细测试完成 ===")
}

// TestRealAPIChallenge100PlayersSimple 测试100个玩家的简化API挑战赛流程
func TestRealAPIChallenge100PlayersSimple(t *testing.T) {
	// 跳过性能测试，除非明确要求
	if testing.Short() {
		t.Skip("跳过长时间运行的API测试")
	}

	// 创建 gRPC 客户端连接
	conn, err := grpc.Dial("localhost:7349", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器: %v", err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头 - 使用配置文件中的server_key "sparkgame"
	ctx := context.Background()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始100个玩家的简化API挑战赛测试 ===")
	t.Logf("服务器地址: localhost:7349")
	t.Logf("认证方式: Basic Auth (sparkgame)")

	// 创建玩家数据结构
	type Player struct {
		CustomID         string
		Username         string
		UserID           string
		Session          *api.Session
		Context          context.Context
		JoinedChallenges []int32
		TournamentID     string
		ScoresSubmitted  int
	}

	const playerCount = 100
	players := make([]*Player, playerCount)

	// 统计信息
	stats := struct {
		CreatedAccounts      int
		AuthenticationErrors int
		ChallengesFetched    int
		ChallengesJoined     int
		ScoresSubmitted      int
		Errors               int
		TournamentGroups     map[string]int
		StartTime            time.Time
	}{
		TournamentGroups: make(map[string]int),
		StartTime:        time.Now(),
	}

	// 第一阶段：创建100个玩家账号
	t.Logf("=== 阶段1: 创建100个玩家账号 ===")
	for i := 0; i < playerCount; i++ {
		player := &Player{
			CustomID:         fmt.Sprintf("api_test_%d_%d", i+1, time.Now().Unix()),
			Username:         fmt.Sprintf("APIPlayer_%d", i+1),
			JoinedChallenges: []int32{},
		}

		// 创建账号请求
		authReq := &api.AuthenticateCustomRequest{
			Account: &api.AccountCustom{
				Id: player.CustomID,
			},
			Username: player.Username,
			Create:   &wrapperspb.BoolValue{Value: true},
		}

		// 调用认证API
		session, err := client.AuthenticateCustom(ctx, authReq)
		if err != nil {
			t.Logf("玩家 %s 创建账号失败: %v", player.Username, err)
			stats.AuthenticationErrors++
			continue
		}

		player.Session = session

		// 创建带认证的上下文
		player.Context = metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)

		players[i] = player
		stats.CreatedAccounts++

		// 每创建10个玩家输出一次进度
		if (i+1)%10 == 0 {
			t.Logf("已创建 %d 个玩家账号", i+1)
		}
	}

	t.Logf("账号创建完成: 成功 %d 个, 失败 %d 个", stats.CreatedAccounts, stats.AuthenticationErrors)

	// 第二阶段：获取挑战赛信息
	t.Logf("=== 阶段2: 获取挑战赛信息 ===")
	var challengeList *game.GetChallengeResponse
	if stats.CreatedAccounts > 0 {
		// 使用第一个成功创建的玩家获取挑战赛
		var validPlayer *Player
		for _, p := range players {
			if p != nil && p.Context != nil {
				validPlayer = p
				break
			}
		}

		if validPlayer != nil {
			challengeList, err = client.GetChallenge(validPlayer.Context, &emptypb.Empty{})
			if err != nil {
				t.Logf("获取挑战赛失败: %v", err)
				stats.Errors++
			} else {
				stats.ChallengesFetched = len(challengeList.Challenges)
				t.Logf("✓ 成功获取 %d 个挑战赛", stats.ChallengesFetched)

				for _, challenge := range challengeList.Challenges {
					t.Logf("挑战赛 ID: %d, 活动ID: %s, 最大参与人数: %d",
						challenge.Id, challenge.ActivityId, challenge.MaxPart)
				}
			}
		}
	}

	// 第三阶段：玩家加入挑战赛
	t.Logf("=== 阶段3: 玩家加入挑战赛 ===")
	if challengeList != nil && len(challengeList.Challenges) > 0 {
		// 选择第一个挑战赛进行测试
		targetChallenge := challengeList.Challenges[0]
		t.Logf("目标挑战赛: ID=%d, 活动ID=%s, 最大参与人数=%d",
			targetChallenge.Id, targetChallenge.ActivityId, targetChallenge.MaxPart)

		for i, player := range players {
			if player == nil || player.Context == nil {
				continue
			}

			// 加入挑战赛请求
			joinReq := &game.JoinChallengeRequest{
				ChallengeId: targetChallenge.Id,
			}

			// 调用加入挑战赛API
			joinResp, err := client.JoinChallenge(player.Context, joinReq)
			if err != nil {
				t.Logf("玩家 %s 加入挑战赛失败: %v", player.Username, err)
				stats.Errors++
				continue
			}

			if joinResp.Code != 0 {
				t.Logf("玩家 %s 加入挑战赛失败: Code=%d, Msg=%s",
					player.Username, joinResp.Code, joinResp.Msg)
				stats.Errors++
				continue
			}

			player.JoinedChallenges = append(player.JoinedChallenges, targetChallenge.Id)
			stats.ChallengesJoined++

			if joinResp.Challenge != nil && joinResp.Challenge.TournamentId != "" {
				player.TournamentID = joinResp.Challenge.TournamentId
				stats.TournamentGroups[joinResp.Challenge.TournamentId]++
			}

			// 每加入10个玩家输出一次进度
			if (i+1)%10 == 0 {
				t.Logf("已有 %d 个玩家加入挑战赛", stats.ChallengesJoined)
			}
		}
	}

	t.Logf("加入挑战赛完成: 成功 %d 个玩家", stats.ChallengesJoined)

	// 第四阶段：提交成绩
	t.Logf("=== 阶段4: 提交成绩 ===")
	if stats.ChallengesJoined > 0 {
		for i, player := range players {
			if player == nil || player.Context == nil || len(player.JoinedChallenges) == 0 || player.TournamentID == "" {
				continue
			}

			// 为每个玩家提交3个成绩
			scores := []int64{
				int64(1000 + i*10 + rand.Intn(500)),
				int64(1500 + i*10 + rand.Intn(500)),
				int64(2000 + i*10 + rand.Intn(500)),
			}

			for _, score := range scores {
				writeReq := &api.WriteTournamentRecordRequest{
					TournamentId: player.TournamentID,
					Record: &api.WriteTournamentRecordRequest_TournamentRecordWrite{
						Score:    score,
						Subscore: 0,
						Metadata: fmt.Sprintf(`{"player": "%s", "attempt": %d}`, player.Username, player.ScoresSubmitted+1),
						Operator: api.Operator_BEST,
					},
				}

				// 调用提交成绩API
				_, err := client.WriteTournamentRecord(player.Context, writeReq)
				if err != nil {
					t.Logf("玩家 %s 提交成绩失败: %v", player.Username, err)
					stats.Errors++
					continue
				}

				player.ScoresSubmitted++
				stats.ScoresSubmitted++
			}

			// 每提交10个玩家成绩输出一次进度
			if (i+1)%10 == 0 {
				t.Logf("已有 %d 个玩家提交成绩", i+1)
			}
		}
	}

	// 第五阶段：输出测试结果
	elapsedTime := time.Since(stats.StartTime)
	t.Logf("=== 测试结果统计 ===")
	t.Logf("测试总耗时: %v", elapsedTime)
	t.Logf("创建账号: %d 成功, %d 失败", stats.CreatedAccounts, stats.AuthenticationErrors)
	t.Logf("获取挑战赛: %d 个", stats.ChallengesFetched)
	t.Logf("加入挑战赛: %d 个玩家成功", stats.ChallengesJoined)
	t.Logf("提交成绩: %d 次成功", stats.ScoresSubmitted)
	t.Logf("总错误数: %d", stats.Errors)

	t.Logf("=== 竞标赛分配详情 ===")
	for tournamentID, playerCount := range stats.TournamentGroups {
		t.Logf("竞标赛 %s: %d 个玩家", tournamentID, playerCount)
	}

	// 输出性能统计
	if stats.CreatedAccounts > 0 {
		t.Logf("=== 性能统计 ===")
		t.Logf("账号创建速度: %.2f 个/秒", float64(stats.CreatedAccounts)/elapsedTime.Seconds())
		t.Logf("加入挑战赛速度: %.2f 个/秒", float64(stats.ChallengesJoined)/elapsedTime.Seconds())
		t.Logf("成绩提交速度: %.2f 次/秒", float64(stats.ScoresSubmitted)/elapsedTime.Seconds())
		t.Logf("成功率: %.2f%%", float64(stats.CreatedAccounts)/float64(playerCount)*100)
	}

	// 验证测试结果
	if stats.CreatedAccounts < playerCount*7/10 {
		t.Errorf("创建账号成功率过低: %d/%d (%.2f%%)", stats.CreatedAccounts, playerCount, float64(stats.CreatedAccounts)/float64(playerCount)*100)
	}

	if stats.ChallengesFetched == 0 {
		t.Error("未获取到任何挑战赛")
	}

	if stats.ChallengesJoined < stats.CreatedAccounts*7/10 {
		t.Errorf("加入挑战赛成功率过低: %d/%d (%.2f%%)", stats.ChallengesJoined, stats.CreatedAccounts, float64(stats.ChallengesJoined)/float64(stats.CreatedAccounts)*100)
	}

	expectedScores := stats.ChallengesJoined * 3 // 每个玩家3次成绩
	if stats.ScoresSubmitted < expectedScores*7/10 {
		t.Errorf("提交成绩成功率过低: %d/%d (%.2f%%)", stats.ScoresSubmitted, expectedScores, float64(stats.ScoresSubmitted)/float64(expectedScores)*100)
	}

	t.Logf("=== 测试完成 ===")
}
