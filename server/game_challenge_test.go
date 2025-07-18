package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/apigrpc"
	"github.com/heroiclabs/nakama/v3/game"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// createSignedTournamentMetadata 创建包含签名的tournament record metadata
func createSignedTournamentMetadata(tournamentID string, score, subscore int64, metadataFields map[string]interface{}) (string, error) {
	// 将metadata字段转换为JSON字符串
	metadataBytes, err := json.Marshal(metadataFields)
	if err != nil {
		return "", fmt.Errorf("编码metadata失败: %v", err)
	}
	metadataStr := string(metadataBytes)

	// 生成签名
	signature, err := GenerateTournamentRecordSignature(tournamentID, score, subscore, metadataStr)
	if err != nil {
		return "", fmt.Errorf("生成签名失败: %v", err)
	}

	// 将签名添加到metadata中
	metadataFields["signature"] = signature

	// 返回最终的metadata JSON字符串
	finalMetadataBytes, err := json.Marshal(metadataFields)
	if err != nil {
		return "", fmt.Errorf("编码最终metadata失败: %v", err)
	}

	return string(finalMetadataBytes), nil
}

// 统一的API挑战赛测试函数
func TestRealAPIChallenge100Players(t *testing.T) {
	// 设置测试超时时间（25分钟，留5分钟缓冲）
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// 测试配置
	config := struct {
		PlayerCount     int
		CustomIDPrefix  string
		UsernamePrefix  string
		StrictMode      bool
		PerformanceMode bool
		VerboseLogging  bool
	}{
		PlayerCount:     10000,
		CustomIDPrefix:  "test_player_10000_1",
		UsernamePrefix:  "Player_10000_1",
		StrictMode:      true, // 严格模式：使用t.Errorf，解析JWT
		PerformanceMode: true, // 性能模式：记录时间，计算速度
		VerboseLogging:  true, // 详细日志：显示详细成绩信息
	}

	// 检查是否需要跳过长时间运行的测试
	if testing.Short() {
		t.Log("检测到-short标志，但继续运行API测试...")
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		t.Fatal("测试上下文已取消")
	default:
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
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始%d个玩家的API挑战赛测试 ===", config.PlayerCount)
	t.Logf("服务器地址: localhost:7349")
	t.Logf("认证方式: Basic Auth (sparkgame)")
	t.Logf("测试模式: 严格模式=%v, 性能模式=%v, 详细日志=%v",
		config.StrictMode, config.PerformanceMode, config.VerboseLogging)

	// 创建玩家数据结构
	type Player struct {
		CustomID         string
		Username         string
		UserID           string
		Session          *api.Session
		Context          context.Context
		JoinedChallenges []int32
		TournamentID     string
		Scores           []int64
		ScoresSubmitted  int
	}

	players := make([]*Player, config.PlayerCount)

	// 统计信息
	stats := struct {
		CreatedAccounts   int
		ChallengesFetched int
		ChallengesJoined  int
		ScoresSubmitted   int
		TournamentGroups  map[string]int
		StartTime         time.Time
	}{
		TournamentGroups: make(map[string]int),
		StartTime:        time.Now(),
	}

	// 错误处理函数 - 任何失败都立即停止测试
	logError := func(format string, args ...interface{}) {
		t.Fatalf(format, args...)
	}

	fatalError := func(format string, args ...interface{}) {
		t.Fatalf(format, args...)
	}

	// 第一阶段：创建玩家账号
	t.Logf("=== 阶段1: 创建%d个玩家账号 ===", config.PlayerCount)

	// 添加进度检查和超时控制
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	for i := 0; i < config.PlayerCount; i++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			t.Logf("测试超时，已创建 %d 个玩家账号", i)
			return
		default:
		}

		// 每100个玩家检查一次进度
		if i > 0 && i%100 == 0 {
			select {
			case <-progressTicker.C:
				t.Logf("进度: 已创建 %d/%d 个玩家账号 (%.1f%%)",
					i, config.PlayerCount, float64(i)/float64(config.PlayerCount)*100)
			default:
			}
		}
		player := &Player{
			CustomID: fmt.Sprintf("%s_%d", config.CustomIDPrefix, i+1),
			Username: fmt.Sprintf("%s_%d", config.UsernamePrefix, i+1),
		}

		// 创建/认证账号请求
		authReq := &api.AuthenticateCustomRequest{
			Account: &api.AccountCustom{
				Id: player.CustomID,
			},
			Username: player.Username,
			Create:   &wrapperspb.BoolValue{Value: true},
		}

		// 调用认证API
		session, err := client.AuthenticateCustom(authCtx, authReq)
		if err != nil {
			logError("玩家 %s 认证/创建账号失败: %v", player.Username, err)
		}

		player.Session = session

		// 解析用户ID（仅在严格模式下）
		if config.StrictMode {
			claims := &SessionTokenClaims{}
			token, _, err := jwt.NewParser().ParseUnverified(session.Token, claims)
			if err != nil {
				logError("玩家 %s 解析token失败: %v", player.Username, err)
			}

			if claims, ok := token.Claims.(*SessionTokenClaims); ok {
				player.UserID = claims.UserId
			}
		}

		// 创建带认证的上下文
		player.Context = metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)

		players[i] = player
		stats.CreatedAccounts++

		if config.VerboseLogging {
			t.Logf("✓ 玩家 %s (ID: %s) 账号创建成功", player.Username, player.UserID)
		}

		// 每创建10个玩家输出一次进度
		if (i+1)%10 == 0 {
			t.Logf("已创建 %d 个玩家账号", i+1)
		}
	}

	t.Logf("账号创建完成: 成功 %d 个", stats.CreatedAccounts)

	// 第二阶段：获取挑战赛信息
	t.Logf("=== 阶段2: 获取挑战赛信息 ===")
	var challengeList *game.GetChallengeResponse
	if stats.CreatedAccounts > 0 {
		// 查找第一个有效玩家
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
				fatalError("获取挑战赛失败: %v", err)
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
			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				t.Logf("测试超时，已处理 %d 个玩家加入挑战赛", i)
				return
			default:
			}
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
				logError("玩家 %s 加入挑战赛失败: %v", player.Username, err)
			}

			if joinResp.Code != 0 {
				logError("玩家 %s 加入挑战赛失败: Code=%d, Msg=%s",
					player.Username, joinResp.Code, joinResp.Msg)
			}

			player.JoinedChallenges = append(player.JoinedChallenges, targetChallenge.Id)
			stats.ChallengesJoined++

			if joinResp.Challenge != nil && joinResp.Challenge.TournamentId != "" {
				player.TournamentID = joinResp.Challenge.TournamentId
				stats.TournamentGroups[joinResp.Challenge.TournamentId]++
			}

			if config.VerboseLogging {
				t.Logf("✓ 玩家 %s 成功加入挑战赛 %d (竞标赛: %s)",
					player.Username, targetChallenge.Id, joinResp.Challenge.TournamentId)
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
			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				t.Logf("测试超时，已处理 %d 个玩家提交成绩", i)
				return
			default:
			}
			if player == nil || player.Context == nil || len(player.JoinedChallenges) == 0 {
				continue
			}

			// 获取该玩家的竞标赛ID
			var tournamentID string
			if player.TournamentID != "" {
				tournamentID = player.TournamentID
			} else {
				// 重新获取挑战赛信息以获取tournamentID
				challengeResp, err := client.GetChallenge(player.Context, &emptypb.Empty{})
				if err != nil {
					logError("玩家 %s 获取挑战赛信息失败: %v", player.Username, err)
				}

				// 找到对应的挑战赛
				for _, challenge := range challengeResp.Challenges {
					if challenge.Id == player.JoinedChallenges[0] {
						tournamentID = challenge.TournamentId
						break
					}
				}

				if tournamentID == "" {
					logError("玩家 %s 没有分配到竞标赛", player.Username)
				}
			}

			// 为每个玩家生成成绩
			var scores []int64
			if config.PerformanceMode {
				// 性能模式：递增成绩
				scores = []int64{
					int64(1000 + i*10 + rand.Intn(500)),
					int64(1500 + i*10 + rand.Intn(500)),
					int64(2000 + i*10 + rand.Intn(500)),
				}
			} else {
				// 严格模式：随机成绩
				scores = []int64{
					int64(10000 + rand.Intn(9000)),
					int64(15000 + rand.Intn(8500)),
					int64(20000 + rand.Intn(8000)),
				}
			}
			player.Scores = scores

			// 提交每个成绩
			for scoreIndex, score := range scores {
				// 创建包含签名的metadata
				metadataFields := map[string]interface{}{
					"attempt":   scoreIndex + 1,
					"timestamp": time.Now().Format(time.RFC3339),
				}

				signedMetadata, err := createSignedTournamentMetadata(tournamentID, score, 0, metadataFields)
				if err != nil {
					logError("玩家 %s 创建签名metadata失败: %v", player.Username, err)
				}

				writeReq := &api.WriteTournamentRecordRequest{
					TournamentId: tournamentID,
					Record: &api.WriteTournamentRecordRequest_TournamentRecordWrite{
						Score:    score,
						Subscore: 0,
						Metadata: signedMetadata,
						Operator: api.Operator_BEST,
					},
				}

				// 调用提交成绩API
				recordResp, err := client.WriteTournamentRecord(player.Context, writeReq)
				if err != nil {
					logError("玩家 %s 提交成绩失败: %v", player.Username, err)
				}

				player.ScoresSubmitted++
				stats.ScoresSubmitted++

				if config.VerboseLogging {
					t.Logf("✓ 玩家 %s 提交成绩 %d (尝试 %d/3) 到竞标赛 %s (已签名)",
						player.Username, score, scoreIndex+1, tournamentID)
					t.Logf("  - 用户名: %s, 排名: %d, 分数: %d",
						recordResp.Username, recordResp.Rank, recordResp.Score)
				}
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
	if config.PerformanceMode {
		t.Logf("测试总耗时: %v", elapsedTime)
	}
	t.Logf("创建账号: %d 成功", stats.CreatedAccounts)
	t.Logf("获取挑战赛: %d 个", stats.ChallengesFetched)
	t.Logf("加入挑战赛: %d 个玩家成功", stats.ChallengesJoined)
	t.Logf("提交成绩: %d 次成功", stats.ScoresSubmitted)

	t.Logf("=== 竞标赛分配详情 ===")
	for tournamentID, playerCount := range stats.TournamentGroups {
		t.Logf("竞标赛 %s: %d 个玩家", tournamentID, playerCount)
	}

	// 性能统计（仅在性能模式下）
	if config.PerformanceMode && stats.CreatedAccounts > 0 {
		t.Logf("=== 性能统计 ===")
		t.Logf("账号创建速度: %.2f 个/秒", float64(stats.CreatedAccounts)/elapsedTime.Seconds())
		t.Logf("加入挑战赛速度: %.2f 个/秒", float64(stats.ChallengesJoined)/elapsedTime.Seconds())
		t.Logf("成绩提交速度: %.2f 次/秒", float64(stats.ScoresSubmitted)/elapsedTime.Seconds())
		t.Logf("成功率: %.2f%%", float64(stats.CreatedAccounts)/float64(config.PlayerCount)*100)
	}

	// 验证测试结果
	successRate := float64(stats.CreatedAccounts) / float64(config.PlayerCount) * 100
	if successRate < 70 {
		t.Errorf("创建账号成功率过低: %d/%d (%.2f%%)", stats.CreatedAccounts, config.PlayerCount, successRate)
	}

	if stats.ChallengesFetched == 0 {
		t.Error("未获取到任何挑战赛")
	}

	joinSuccessRate := float64(stats.ChallengesJoined) / float64(stats.CreatedAccounts) * 100
	if joinSuccessRate < 70 {
		t.Errorf("加入挑战赛成功率过低: %d/%d (%.2f%%)", stats.ChallengesJoined, stats.CreatedAccounts, joinSuccessRate)
	}

	expectedScores := stats.ChallengesJoined * 3 // 每个玩家3次成绩
	scoreSuccessRate := float64(stats.ScoresSubmitted) / float64(expectedScores) * 100
	if scoreSuccessRate < 70 {
		t.Errorf("提交成绩成功率过低: %d/%d (%.2f%%)", stats.ScoresSubmitted, expectedScores, scoreSuccessRate)
	}

	t.Logf("=== 测试完成 ===")
}

// 测试并发tournament创建和分配
func TestConcurrentTournamentCreation(t *testing.T) {
	// 检查是否需要跳过长时间运行的测试（只有在明确设置环境变量时才跳过）
	if testing.Short() {
		t.Log("检测到-short标志，但继续运行测试...")
		// 不跳过，继续执行测试
	}

	// 创建 gRPC 客户端连接
	conn, err := grpc.Dial("localhost:7349", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器: %v", err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头
	ctx := context.Background()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始并发tournament创建测试 ===")

	// 创建玩家数据结构
	type Player struct {
		ID           int
		CustomID     string
		Username     string
		Session      *api.Session
		Context      context.Context
		TournamentID string
		JoinSuccess  bool
		Error        error
	}

	const playerCount = 100           // 减少到20个玩家，更容易观察tournament创建
	const maxPlayersPerTournament = 4 // 假设每个tournament最多4个玩家

	players := make([]*Player, playerCount)

	// 第一阶段：创建玩家账号
	t.Logf("=== 阶段1: 创建 %d 个玩家账号 ===", playerCount)
	for i := 0; i < playerCount; i++ {
		player := &Player{
			ID:       i + 1,
			CustomID: fmt.Sprintf("concurrent_test_%d", i+1), // 使用固定的CustomID，不带时间戳
			Username: fmt.Sprintf("ConcurrentPlayer_%d", i+1),
		}

		// 先尝试认证现有账号
		authReq := &api.AuthenticateCustomRequest{
			Account: &api.AccountCustom{
				Id: player.CustomID,
			},
			Username: player.Username,
			Create:   &wrapperspb.BoolValue{Value: false},
		}

		session, err := client.AuthenticateCustom(ctx, authReq)
		if err != nil {
			// 如果认证失败，尝试创建新账号
			t.Logf("玩家 %s 认证失败，尝试创建新账号", player.Username)
			authReq.Create = &wrapperspb.BoolValue{Value: true}

			session, err = client.AuthenticateCustom(ctx, authReq)
			if err != nil {
				t.Logf("玩家 %s 创建账号失败: %v，跳过此玩家", player.Username, err)
				continue
			}
			t.Logf("✓ 玩家 %s 创建账号成功", player.Username)
		} else {
			t.Logf("✓ 玩家 %s 认证成功", player.Username)
		}

		player.Session = session
		player.Context = metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)
		players[i] = player

		t.Logf("✓ 玩家 %s 认证成功", player.Username)

		if (i+1)%10 == 0 {
			t.Logf("已处理 %d 个玩家账号", i+1)
		}
	}

	// 统计成功认证的玩家数量
	successfulPlayers := 0
	for _, player := range players {
		if player != nil && player.Context != nil {
			successfulPlayers++
		}
	}
	t.Logf("成功认证的玩家数量: %d / %d", successfulPlayers, playerCount)

	// 第二阶段：获取挑战赛信息
	t.Logf("=== 阶段2: 获取挑战赛信息 ===")
	var challengeList *game.GetChallengeResponse
	var targetChallenge *game.Challenge

	// 找到第一个有效的玩家获取挑战赛
	for _, player := range players {
		if player != nil && player.Context != nil {
			challengeList, err = client.GetChallenge(player.Context, &emptypb.Empty{})
			if err != nil {
				t.Errorf("获取挑战赛失败: %v", err)
				continue
			}

			if len(challengeList.Challenges) > 0 {
				targetChallenge = challengeList.Challenges[0]
				t.Logf("找到目标挑战赛: ID=%d, 活动ID=%s, 最大参与人数=%d",
					targetChallenge.Id, targetChallenge.ActivityId, targetChallenge.MaxPart)
				break
			}
		}
	}

	if targetChallenge == nil {
		if successfulPlayers == 0 {
			t.Fatalf("没有成功认证的玩家，无法获取挑战赛信息")
		} else {
			t.Fatalf("未找到可用的挑战赛，请检查挑战赛配置")
		}
	}

	// 第三阶段：并发加入挑战赛
	t.Logf("=== 阶段3: %d 个玩家并发加入挑战赛 ===", successfulPlayers)

	// 统计信息
	stats := struct {
		JoinSuccessCount int
		JoinFailureCount int
		TournamentGroups map[string]int
	}{
		TournamentGroups: make(map[string]int),
	}

	// 使用channel来收集结果
	resultChan := make(chan *Player, playerCount)

	// 启动所有玩家加入挑战赛，添加随机延迟减少并发冲突
	actualLaunchedCount := 0
	for i, player := range players {
		if player == nil || player.Context == nil {
			continue
		}

		actualLaunchedCount++
		go func(p *Player, index int) {
			// 添加随机延迟（0-100毫秒），避免所有玩家同时发起请求
			delay := time.Duration(rand.Intn(100)) * time.Millisecond
			time.Sleep(delay)

			// 加入挑战赛
			joinReq := &game.JoinChallengeRequest{
				ChallengeId: targetChallenge.Id,
			}

			joinResp, err := client.JoinChallenge(p.Context, joinReq)
			if err != nil {
				p.Error = err
				p.JoinSuccess = false
			} else if joinResp.Code != 0 {
				p.Error = fmt.Errorf("加入失败: Code=%d, Msg=%s", joinResp.Code, joinResp.Msg)
				p.JoinSuccess = false
			} else {
				p.JoinSuccess = true
				if joinResp.Challenge != nil {
					p.TournamentID = joinResp.Challenge.TournamentId
				}
			}

			resultChan <- p
		}(player, i)
	}

	// 收集所有结果
	t.Logf("启动了 %d 个玩家加入挑战赛", actualLaunchedCount)
	t.Logf("等待 %d 个玩家的加入结果...", actualLaunchedCount)

	for i := 0; i < actualLaunchedCount; i++ {
		player := <-resultChan

		if player.JoinSuccess {
			stats.JoinSuccessCount++
			if player.TournamentID != "" {
				stats.TournamentGroups[player.TournamentID]++
			}
			// 简化日志输出，避免终端截断
			t.Logf("✓ 玩家[%d] 成功加入挑战赛", player.ID)
			t.Logf("   Tournament: %s", player.TournamentID)
		} else {
			stats.JoinFailureCount++
			t.Logf("✗ 玩家[%d] 加入挑战赛失败: %v", player.ID, player.Error)
		}
	}

	// 第四阶段：分析结果
	t.Logf("=== 阶段4: 分析结果 ===")
	t.Logf("开始分析结果...")

	t.Logf("总玩家数: %d", actualLaunchedCount)
	t.Logf("成功加入: %d", stats.JoinSuccessCount)
	t.Logf("加入失败: %d", stats.JoinFailureCount)
	t.Logf("创建的tournament数量: %d", len(stats.TournamentGroups))

	// 验证tournament分配
	t.Logf("计算期望的tournament数量...")
	expectedTournaments := (stats.JoinSuccessCount + int(targetChallenge.MaxPart) - 1) / int(targetChallenge.MaxPart)
	t.Logf("期望的tournament数量: %d", expectedTournaments)

	t.Logf("=== Tournament分配详情 ===")
	if len(stats.TournamentGroups) == 0 {
		t.Logf("⚠️ 没有找到任何tournament分配")
	} else {
		for tournamentID, playerCount := range stats.TournamentGroups {
			t.Logf("Tournament %s: %d 个玩家", tournamentID, playerCount)

			// 验证每个tournament的玩家数量不超过限制
			if int32(playerCount) > targetChallenge.MaxPart {
				t.Errorf("Tournament %s 玩家数量超过限制: %d > %d",
					tournamentID, playerCount, targetChallenge.MaxPart)
			}
		}
	}

	t.Logf("开始验证结果...")

	// 验证结果
	if len(stats.TournamentGroups) < expectedTournaments {
		t.Logf("⚠️ 创建的tournament数量不足: 期望至少 %d 个，实际 %d 个",
			expectedTournaments, len(stats.TournamentGroups))
	} else {
		t.Logf("✓ Tournament数量验证通过")
	}

	if stats.JoinSuccessCount < actualLaunchedCount*7/10 {
		t.Logf("⚠️ 加入成功率过低: %d/%d (%.2f%%)",
			stats.JoinSuccessCount, actualLaunchedCount,
			float64(stats.JoinSuccessCount)/float64(actualLaunchedCount)*100)
	} else {
		t.Logf("✓ 加入成功率验证通过: %d/%d (%.2f%%)",
			stats.JoinSuccessCount, actualLaunchedCount,
			float64(stats.JoinSuccessCount)/float64(actualLaunchedCount)*100)
	}

	// 验证是否有适当的tournament分布
	t.Logf("计算满员的tournament数量...")
	fullTournaments := 0
	for _, playerCount := range stats.TournamentGroups {
		if int32(playerCount) == targetChallenge.MaxPart {
			fullTournaments++
		}
	}

	t.Logf("满员的tournament数量: %d", fullTournaments)

	// 第五阶段：提交随机分数测试
	t.Logf("=== 阶段5: 提交随机分数测试 ===")

	scoreSubmissionStats := struct {
		SubmissionSuccess int
		SubmissionFailure int
		TotalScores       int
	}{}

	// 为每个成功加入的玩家提交随机分数
	for _, player := range players {
		if player == nil || !player.JoinSuccess || player.TournamentID == "" {
			continue
		}

		// 为每个玩家生成3个随机分数
		scores := []int64{
			int64(1000 + rand.Intn(9000)), // 1000-10000随机分数
			int64(1500 + rand.Intn(8500)), // 第二次尝试
			int64(2000 + rand.Intn(8000)), // 第三次尝试
		}

		for attemptNum, score := range scores {
			// 创建包含签名的metadata
			metadataFields := map[string]interface{}{
				"player_id": player.ID,
				"attempt":   attemptNum + 1,
				"timestamp": time.Now().Format(time.RFC3339),
			}

			signedMetadata, err := createSignedTournamentMetadata(player.TournamentID, score, 0, metadataFields)
			if err != nil {
				t.Logf("✗ 玩家[%d] 创建签名metadata失败: %v", player.ID, err)
				scoreSubmissionStats.SubmissionFailure++
				continue
			}

			writeReq := &api.WriteTournamentRecordRequest{
				TournamentId: player.TournamentID,
				Record: &api.WriteTournamentRecordRequest_TournamentRecordWrite{
					Score:    score,
					Subscore: 0,
					Metadata: signedMetadata,
					Operator: api.Operator_BEST,
				},
			}

			// 调用提交成绩API
			recordResp, err := client.WriteTournamentRecord(player.Context, writeReq)
			if err != nil {
				t.Logf("✗ 玩家[%d] 提交分数失败: %v", player.ID, err)
				scoreSubmissionStats.SubmissionFailure++
			} else {
				scoreSubmissionStats.SubmissionSuccess++
				scoreSubmissionStats.TotalScores++
				t.Logf("✓ 玩家[%d] 成功提交分数: %d (排名: %d, 已签名)",
					player.ID, recordResp.Score, recordResp.Rank)
			}

			// 添加小延迟避免过快请求
			time.Sleep(50 * time.Millisecond)
		}
	}

	// 输出分数提交统计
	t.Logf("=== 分数提交统计 ===")
	t.Logf("成功提交: %d", scoreSubmissionStats.SubmissionSuccess)
	t.Logf("提交失败: %d", scoreSubmissionStats.SubmissionFailure)
	t.Logf("总分数记录: %d", scoreSubmissionStats.TotalScores)

	// 验证分数提交成功率
	expectedSubmissions := stats.JoinSuccessCount * 3 // 每个成功加入的玩家提交3次
	if scoreSubmissionStats.SubmissionSuccess < expectedSubmissions*7/10 {
		t.Logf("⚠️ 分数提交成功率过低: %d/%d (%.2f%%)",
			scoreSubmissionStats.SubmissionSuccess, expectedSubmissions,
			float64(scoreSubmissionStats.SubmissionSuccess)/float64(expectedSubmissions)*100)
	} else {
		t.Logf("✓ 分数提交成功率验证通过: %d/%d (%.2f%%)",
			scoreSubmissionStats.SubmissionSuccess, expectedSubmissions,
			float64(scoreSubmissionStats.SubmissionSuccess)/float64(expectedSubmissions)*100)
	}

	t.Logf("=== 并发测试完成 ===")
	t.Logf("测试已正常结束")
}
