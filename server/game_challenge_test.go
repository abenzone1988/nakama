package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
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

// getServerAddress 获取正确的服务器地址
func getServerAddress(node int) string {
	// 节点映射（本地多节点）
	nodes := []string{
		"localhost:7349", // 节点0
		"localhost:7449", // 节点1
	}

	// 检查是否在 Docker/CI 环境中（保持向后兼容）
	if os.Getenv("DOCKER_ENV") == "true" || os.Getenv("TEST_DB_URL") != "" {
		// 默认单入口，具体路由由集群/负载均衡决定
		return "nakama:7349"
	}

	if node < 0 || node >= len(nodes) {
		node = 0
	}
	return nodes[node]
}

// getServerAddresses 从环境变量中获取服务器地址列表
// 优先使用 NAKAMA_ADDRS（以逗号、分号或空白分隔），否则回退到 NAKAMA_ADDR_1..N；再回退到默认
func getServerAddresses() []string {
	addrsEnv := os.Getenv("NAKAMA_ADDRS")
	var addrs []string
	if addrsEnv != "" {
		fields := strings.FieldsFunc(addrsEnv, func(r rune) bool {
			switch r {
			case ',', ';', ' ', '\t', '\n':
				return true
			default:
				return false
			}
		})
		addrs = append(addrs, fields...)
	}

	// 回退读取 NAKAMA_ADDR_1..9
	if len(addrs) == 0 {
		for i := 1; i <= 9; i++ {
			v := os.Getenv(fmt.Sprintf("NAKAMA_ADDR_%d", i))
			if v != "" {
				addrs = append(addrs, v)
			}
		}
	}

	// 最后回退到 CI/本地默认
	if len(addrs) == 0 {
		if os.Getenv("DOCKER_ENV") == "true" || os.Getenv("TEST_DB_URL") != "" {
			addrs = []string{"nakama:7349"}
		} else {
			addrs = []string{getServerAddress(0), getServerAddress(1)}
		}
	}

	// 去重并清理空白
	seen := make(map[string]struct{}, len(addrs))
	uniq := make([]string, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		uniq = append(uniq, a)
	}
	return uniq
}

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
		PlayerCount:     100,
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
	serverAddr := getServerAddress(0)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器 %s: %v", serverAddr, err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头 - 使用配置文件中的server_key "sparkgame"
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始%d个玩家的API挑战赛测试 ===", config.PlayerCount)
	t.Logf("服务器地址: %s", serverAddr)
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
	serverAddr := getServerAddress(0)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器 %s: %v", serverAddr, err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头
	ctx := context.Background()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始并发tournament创建测试 ===")
	t.Logf("服务器地址: %s", serverAddr)

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
		ResponseTime time.Duration
	}

	const playerCount = 100

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
		JoinSuccessCount  int
		JoinFailureCount  int
		TournamentGroups  map[string]int
		StartTime         time.Time
		EndTime           time.Time
		ResponseTimes     []time.Duration
		MinResponseTime   time.Duration
		MaxResponseTime   time.Duration
		TotalResponseTime time.Duration
		ErrorCounts       map[string]int
	}{
		TournamentGroups: make(map[string]int),
		ResponseTimes:    make([]time.Duration, 0, playerCount),
		ErrorCounts:      make(map[string]int),
		StartTime:        time.Now(),
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
			// 记录请求开始时间
			requestStartTime := time.Now()
			// 加入挑战赛
			joinReq := &game.JoinChallengeRequest{
				ChallengeId: targetChallenge.Id,
			}
			joinResp, err := client.JoinChallenge(p.Context, joinReq)

			// 记录响应时间
			responseTime := time.Since(requestStartTime)
			p.ResponseTime = responseTime

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

	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for i := 0; i < actualLaunchedCount; i++ {
		select {
		case player := <-resultChan:
			// 记录响应时间统计
			stats.ResponseTimes = append(stats.ResponseTimes, player.ResponseTime)
			stats.TotalResponseTime += player.ResponseTime

			// 更新最小/最大响应时间
			if stats.MinResponseTime == 0 || player.ResponseTime < stats.MinResponseTime {
				stats.MinResponseTime = player.ResponseTime
			}
			if player.ResponseTime > stats.MaxResponseTime {
				stats.MaxResponseTime = player.ResponseTime
			}

			if player.JoinSuccess {
				stats.JoinSuccessCount++
				if player.TournamentID != "" {
					stats.TournamentGroups[player.TournamentID]++
				}
				// 简化日志输出，避免终端截断
				t.Logf("✓ 玩家[%d] 成功加入挑战赛 (响应时间: %v)", player.ID, player.ResponseTime)
				t.Logf("   Tournament: %s", player.TournamentID)
			} else {
				stats.JoinFailureCount++
				if player.Error != nil {
					stats.ErrorCounts[player.Error.Error()]++
				}
				t.Logf("✗ 玩家[%d] 加入挑战赛失败: %v (响应时间: %v)", player.ID, player.Error, player.ResponseTime)
			}

			// 每10个玩家输出一次进度
			if (i+1)%10 == 0 {
				select {
				case <-progressTicker.C:
					t.Logf("进度: 已处理 %d/%d 个玩家 (%.1f%%)",
						i+1, actualLaunchedCount, float64(i+1)/float64(actualLaunchedCount)*100)
				default:
				}
			}

		case <-time.After(30 * time.Second):
			t.Logf("等待结果超时，已处理 %d 个玩家", i)
			i = actualLaunchedCount // 结束外层for
		}
	}

	stats.EndTime = time.Now()
	totalDuration := stats.EndTime.Sub(stats.StartTime)

	// 第四阶段：分析结果
	t.Logf("=== 阶段4: 分析结果 ===")
	t.Logf("开始分析结果...")

	t.Logf("总玩家数: %d", actualLaunchedCount)
	t.Logf("成功加入: %d", stats.JoinSuccessCount)
	t.Logf("加入失败: %d", stats.JoinFailureCount)
	t.Logf("创建的tournament数量: %d", len(stats.TournamentGroups))
	t.Logf("测试总耗时: %v", totalDuration)

	// 性能统计
	if len(stats.ResponseTimes) > 0 {
		// 计算平均响应时间
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		t.Logf("=== 性能统计 ===")
		t.Logf("平均响应时间: %v", avgResponseTime)
		t.Logf("最小响应时间: %v", stats.MinResponseTime)
		t.Logf("最大响应时间: %v", stats.MaxResponseTime)

		// 计算QPS (每秒请求数)
		qps := float64(actualLaunchedCount) / totalDuration.Seconds()
		t.Logf("QPS (每秒请求数): %.2f", qps)

		// 计算P50, P90, P95, P99响应时间
		sort.Slice(stats.ResponseTimes, func(i, j int) bool {
			return stats.ResponseTimes[i] < stats.ResponseTimes[j]
		})

		p50Index := int(float64(len(stats.ResponseTimes)) * 0.5)
		p90Index := int(float64(len(stats.ResponseTimes)) * 0.9)
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		p99Index := int(float64(len(stats.ResponseTimes)) * 0.99)

		if p50Index < len(stats.ResponseTimes) {
			t.Logf("P50响应时间: %v", stats.ResponseTimes[p50Index])
		}
		if p90Index < len(stats.ResponseTimes) {
			t.Logf("P90响应时间: %v", stats.ResponseTimes[p90Index])
		}
		if p95Index < len(stats.ResponseTimes) {
			t.Logf("P95响应时间: %v", stats.ResponseTimes[p95Index])
		}
		if p99Index < len(stats.ResponseTimes) {
			t.Logf("P99响应时间: %v", stats.ResponseTimes[p99Index])
		}
	}

	// 输出错误统计
	if len(stats.ErrorCounts) > 0 {
		t.Logf("=== 错误统计 ===")
		for errorMsg, count := range stats.ErrorCounts {
			t.Logf("错误: %s (次数: %d)", errorMsg, count)
		}
	}

	// 验证tournament分配
	t.Logf("计算期望的tournament数量...")
	expectedTournaments := (stats.JoinSuccessCount + int(targetChallenge.MaxPart) - 1) / int(targetChallenge.MaxPart)
	t.Logf("期望的tournament数量: %d", expectedTournaments)

	// 期望与实际数量差异判断
	actualTournaments := len(stats.TournamentGroups)
	diff := expectedTournaments - actualTournaments
	if diff != 0 {
		if diff > 0 {
			t.Errorf("Tournament数量与期望不一致: 期望=%d 实际=%d 差异=%d(少创建)", expectedTournaments, actualTournaments, diff)
		} else {
			t.Errorf("Tournament数量与期望不一致: 期望=%d 实际=%d 差异=%d(多创建)", expectedTournaments, actualTournaments, diff)
		}
	} else {
		t.Logf("✓ Tournament数量与期望一致: %d", actualTournaments)
	}

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

	// 性能验证
	successRate := float64(stats.JoinSuccessCount) / float64(actualLaunchedCount) * 100
	if successRate < 70 {
		t.Errorf("加入挑战赛成功率过低: %.2f%% (期望 >= 70%%)", successRate)
	}

	// 响应时间验证
	if len(stats.ResponseTimes) > 0 {
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		if avgResponseTime > 2*time.Second {

			t.Errorf("平均响应时间过长: %v (期望 < 2秒)", avgResponseTime)
		}

		// 检查P95响应时间
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		if p95Index < len(stats.ResponseTimes) && stats.ResponseTimes[p95Index] > 5*time.Second {
			t.Errorf("P95响应时间过长: %v (期望 < 5秒)", stats.ResponseTimes[p95Index])
		}
	}

	// QPS验证
	qps := float64(actualLaunchedCount) / totalDuration.Seconds()
	if qps < 10 {
		t.Errorf("QPS过低: %.2f (期望 >= 10)", qps)
	}

	t.Logf("=== 并发测试完成 ===")
	t.Logf("测试已正常结束")
}

// 测试并发tournament创建和分配 和 提交分数
func TestConcurrentTournamentCreationAndSubmit(t *testing.T) {
	// 检查是否需要跳过长时间运行的测试（只有在明确设置环境变量时才跳过）
	if testing.Short() {
		t.Log("检测到-short标志，但继续运行测试...")
		// 不跳过，继续执行测试
	}

	// 创建 gRPC 客户端连接
	serverAddr := getServerAddress(0)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器 %s: %v", serverAddr, err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头
	ctx := context.Background()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始并发tournament创建测试 ===")
	t.Logf("服务器地址: %s", serverAddr)

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
		ResponseTime time.Duration
	}

	const playerCount = 100

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
		JoinSuccessCount  int
		JoinFailureCount  int
		TournamentGroups  map[string]int
		StartTime         time.Time
		EndTime           time.Time
		ResponseTimes     []time.Duration
		MinResponseTime   time.Duration
		MaxResponseTime   time.Duration
		TotalResponseTime time.Duration
		ErrorCounts       map[string]int
	}{
		TournamentGroups: make(map[string]int),
		ResponseTimes:    make([]time.Duration, 0, playerCount),
		ErrorCounts:      make(map[string]int),
		StartTime:        time.Now(),
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
			// 记录请求开始时间
			requestStartTime := time.Now()
			// 加入挑战赛
			joinReq := &game.JoinChallengeRequest{
				ChallengeId: targetChallenge.Id,
			}
			joinResp, err := client.JoinChallenge(p.Context, joinReq)

			// 记录响应时间
			responseTime := time.Since(requestStartTime)
			p.ResponseTime = responseTime

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

	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for i := 0; i < actualLaunchedCount; i++ {
		select {
		case player := <-resultChan:
			// 记录响应时间统计
			stats.ResponseTimes = append(stats.ResponseTimes, player.ResponseTime)
			stats.TotalResponseTime += player.ResponseTime

			// 更新最小/最大响应时间
			if stats.MinResponseTime == 0 || player.ResponseTime < stats.MinResponseTime {
				stats.MinResponseTime = player.ResponseTime
			}
			if player.ResponseTime > stats.MaxResponseTime {
				stats.MaxResponseTime = player.ResponseTime
			}

			if player.JoinSuccess {
				stats.JoinSuccessCount++
				if player.TournamentID != "" {
					stats.TournamentGroups[player.TournamentID]++
				}
				// 简化日志输出，避免终端截断
				t.Logf("✓ 玩家[%d] 成功加入挑战赛 (响应时间: %v)", player.ID, player.ResponseTime)
				t.Logf("   Tournament: %s", player.TournamentID)
			} else {
				stats.JoinFailureCount++
				if player.Error != nil {
					stats.ErrorCounts[player.Error.Error()]++
				}
				t.Logf("✗ 玩家[%d] 加入挑战赛失败: %v (响应时间: %v)", player.ID, player.Error, player.ResponseTime)
			}

			// 每10个玩家输出一次进度
			if (i+1)%10 == 0 {
				select {
				case <-progressTicker.C:
					t.Logf("进度: 已处理 %d/%d 个玩家 (%.1f%%)",
						i+1, actualLaunchedCount, float64(i+1)/float64(actualLaunchedCount)*100)
				default:
				}
			}

		case <-time.After(30 * time.Second):
			t.Logf("等待结果超时，已处理 %d 个玩家", i)
			i = actualLaunchedCount // 结束外层for
		}
	}

	stats.EndTime = time.Now()
	totalDuration := stats.EndTime.Sub(stats.StartTime)

	// 第四阶段：分析结果
	t.Logf("=== 阶段4: 分析结果 ===")
	t.Logf("开始分析结果...")

	t.Logf("总玩家数: %d", actualLaunchedCount)
	t.Logf("成功加入: %d", stats.JoinSuccessCount)
	t.Logf("加入失败: %d", stats.JoinFailureCount)
	t.Logf("创建的tournament数量: %d", len(stats.TournamentGroups))
	t.Logf("测试总耗时: %v", totalDuration)

	// 性能统计
	if len(stats.ResponseTimes) > 0 {
		// 计算平均响应时间
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		t.Logf("=== 性能统计 ===")
		t.Logf("平均响应时间: %v", avgResponseTime)
		t.Logf("最小响应时间: %v", stats.MinResponseTime)
		t.Logf("最大响应时间: %v", stats.MaxResponseTime)

		// 计算QPS (每秒请求数)
		qps := float64(actualLaunchedCount) / totalDuration.Seconds()
		t.Logf("QPS (每秒请求数): %.2f", qps)

		// 计算P50, P90, P95, P99响应时间
		sort.Slice(stats.ResponseTimes, func(i, j int) bool {
			return stats.ResponseTimes[i] < stats.ResponseTimes[j]
		})

		p50Index := int(float64(len(stats.ResponseTimes)) * 0.5)
		p90Index := int(float64(len(stats.ResponseTimes)) * 0.9)
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		p99Index := int(float64(len(stats.ResponseTimes)) * 0.99)

		if p50Index < len(stats.ResponseTimes) {
			t.Logf("P50响应时间: %v", stats.ResponseTimes[p50Index])
		}
		if p90Index < len(stats.ResponseTimes) {
			t.Logf("P90响应时间: %v", stats.ResponseTimes[p90Index])
		}
		if p95Index < len(stats.ResponseTimes) {
			t.Logf("P95响应时间: %v", stats.ResponseTimes[p95Index])
		}
		if p99Index < len(stats.ResponseTimes) {
			t.Logf("P99响应时间: %v", stats.ResponseTimes[p99Index])
		}
	}

	// 输出错误统计
	if len(stats.ErrorCounts) > 0 {
		t.Logf("=== 错误统计 ===")
		for errorMsg, count := range stats.ErrorCounts {
			t.Logf("错误: %s (次数: %d)", errorMsg, count)
		}
	}

	// 验证tournament分配
	t.Logf("计算期望的tournament数量...")
	expectedTournaments := (stats.JoinSuccessCount + int(targetChallenge.MaxPart) - 1) / int(targetChallenge.MaxPart)
	t.Logf("期望的tournament数量: %d", expectedTournaments)

	// 期望与实际数量差异判断
	actualTournaments := len(stats.TournamentGroups)
	diff := expectedTournaments - actualTournaments
	if diff != 0 {
		if diff > 0 {
			t.Errorf("Tournament数量与期望不一致: 期望=%d 实际=%d 差异=%d(少创建)", expectedTournaments, actualTournaments, diff)
		} else {
			t.Errorf("Tournament数量与期望不一致: 期望=%d 实际=%d 差异=%d(多创建)", expectedTournaments, actualTournaments, diff)
		}
	} else {
		t.Logf("✓ Tournament数量与期望一致: %d", actualTournaments)
	}

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

	// 性能验证
	successRate := float64(stats.JoinSuccessCount) / float64(actualLaunchedCount) * 100
	if successRate < 70 {
		t.Errorf("加入挑战赛成功率过低: %.2f%% (期望 >= 70%%)", successRate)
	}

	// 响应时间验证
	if len(stats.ResponseTimes) > 0 {
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		if avgResponseTime > 2*time.Second {

			t.Errorf("平均响应时间过长: %v (期望 < 2秒)", avgResponseTime)
		}

		// 检查P95响应时间
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		if p95Index < len(stats.ResponseTimes) && stats.ResponseTimes[p95Index] > 5*time.Second {
			t.Errorf("P95响应时间过长: %v (期望 < 5秒)", stats.ResponseTimes[p95Index])
		}
	}

	// QPS验证
	qps := float64(actualLaunchedCount) / totalDuration.Seconds()
	if qps < 10 {
		t.Errorf("QPS过低: %.2f (期望 >= 10)", qps)
	}

	// 第五阶段：提交随机分数测试 Start ========================== //
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
			int64(1000 + rand.Intn(9000)), // 1000-10000随机分数
			int64(1000 + rand.Intn(9000)), // 1000-10000随机分数
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

	// 第五阶段：提交随机分数测试 End ========================== //
	t.Logf("=== 并发测试完成 ===")
	t.Logf("测试已正常结束")
}

// 测试GetChallenge性能 - 连续请求1000次
func TestGetChallengePerformance(t *testing.T) {
	// 检查是否需要跳过长时间运行的测试
	if testing.Short() {
		t.Log("检测到-short标志，但继续运行性能测试...")
	}

	// 设置测试超时时间（10分钟）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 测试配置
	config := struct {
		RequestCount    int
		ConcurrentUsers int
		VerboseLogging  bool
		PerformanceMode bool
	}{
		RequestCount:    10000,
		ConcurrentUsers: 100, // 10个并发用户
		VerboseLogging:  true,
		PerformanceMode: true,
	}

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		t.Fatal("测试上下文已取消")
	default:
	}

	// 创建 gRPC 客户端连接
	serverAddr := getServerAddress(0)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("无法连接到 gRPC 服务器 %s: %v", serverAddr, err)
	}
	defer conn.Close()

	// 创建 Nakama 客户端
	client := apigrpc.NewNakamaClient(conn)

	// 设置认证头
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	t.Logf("=== 开始GetChallenge性能测试 ===")
	t.Logf("服务器地址: %s", serverAddr)
	t.Logf("请求次数: %d", config.RequestCount)
	t.Logf("并发用户数: %d", config.ConcurrentUsers)
	t.Logf("详细日志: %v", config.VerboseLogging)

	// 创建测试用户
	type TestUser struct {
		ID      int
		Session *api.Session
		Context context.Context
	}

	users := make([]*TestUser, config.ConcurrentUsers)

	// 创建并发用户
	t.Logf("=== 阶段1: 创建%d个测试用户 ===", config.ConcurrentUsers)
	for i := 0; i < config.ConcurrentUsers; i++ {
		user := &TestUser{
			ID: i + 1,
		}

		// 创建/认证账号
		authReq := &api.AuthenticateCustomRequest{
			Account: &api.AccountCustom{
				Id: fmt.Sprintf("perf_test_user_%d", i+1),
			},
			Username: fmt.Sprintf("PerfTestUser_%d", i+1),
			Create:   &wrapperspb.BoolValue{Value: true},
		}

		session, err := client.AuthenticateCustom(authCtx, authReq)
		if err != nil {
			t.Fatalf("用户 %d 认证失败: %v", i+1, err)
		}

		user.Session = session
		user.Context = metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+session.Token)
		users[i] = user

		if config.VerboseLogging {
			t.Logf("✓ 用户 %d 创建成功", i+1)
		}
	}

	t.Logf("用户创建完成: 成功 %d 个", config.ConcurrentUsers)

	// 性能统计
	stats := struct {
		StartTime         time.Time
		EndTime           time.Time
		TotalRequests     int
		SuccessRequests   int
		FailedRequests    int
		MinResponseTime   time.Duration
		MaxResponseTime   time.Duration
		TotalResponseTime time.Duration
		ResponseTimes     []time.Duration
		ErrorCounts       map[string]int
	}{
		ResponseTimes: make([]time.Duration, 0, config.RequestCount),
		ErrorCounts:   make(map[string]int),
		StartTime:     time.Now(),
	}

	// 使用channel收集结果
	resultChan := make(chan struct {
		Success      bool
		ResponseTime time.Duration
		Error        error
		UserID       int
		RequestID    int
	}, config.RequestCount)

	// 启动并发请求
	t.Logf("=== 阶段2: 开始%d次并发GetChallenge请求 ===", config.RequestCount)

	// 为每个用户分配请求数量
	requestsPerUser := config.RequestCount / config.ConcurrentUsers
	extraRequests := config.RequestCount % config.ConcurrentUsers

	requestID := 0
	for userIndex, user := range users {
		userRequestCount := requestsPerUser
		if userIndex < extraRequests {
			userRequestCount++
		}

		// 为每个用户启动goroutine
		go func(u *TestUser, userID, requestCount int) {
			for i := 0; i < requestCount; i++ {
				startTime := time.Now()

				// 调用GetChallenge
				_, err := client.GetChallenge(u.Context, &emptypb.Empty{})

				responseTime := time.Since(startTime)

				resultChan <- struct {
					Success      bool
					ResponseTime time.Duration
					Error        error
					UserID       int
					RequestID    int
				}{
					Success:      err == nil,
					ResponseTime: responseTime,
					Error:        err,
					UserID:       userID,
					RequestID:    requestID + i,
				}
			}
		}(user, user.ID, userRequestCount)

		requestID += userRequestCount
	}

	// 收集所有结果
	t.Logf("等待 %d 个请求完成...", config.RequestCount)

	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for i := 0; i < config.RequestCount; i++ {
		select {
		case result := <-resultChan:
			stats.TotalRequests++

			if result.Success {
				stats.SuccessRequests++
			} else {
				stats.FailedRequests++
				if result.Error != nil {
					stats.ErrorCounts[result.Error.Error()]++
				}
			}

			// 记录响应时间
			stats.ResponseTimes = append(stats.ResponseTimes, result.ResponseTime)
			stats.TotalResponseTime += result.ResponseTime

			// 更新最小/最大响应时间
			if stats.MinResponseTime == 0 || result.ResponseTime < stats.MinResponseTime {
				stats.MinResponseTime = result.ResponseTime
			}
			if result.ResponseTime > stats.MaxResponseTime {
				stats.MaxResponseTime = result.ResponseTime
			}

			// 每100个请求输出一次进度
			if (i+1)%100 == 0 {
				select {
				case <-progressTicker.C:
					t.Logf("进度: 已完成 %d/%d 个请求 (%.1f%%)",
						i+1, config.RequestCount, float64(i+1)/float64(config.RequestCount)*100)
				default:
				}
			}

			if config.VerboseLogging && result.Error != nil {
				t.Logf("请求 %d (用户 %d) 失败: %v (响应时间: %v)",
					result.RequestID, result.UserID, result.Error, result.ResponseTime)
			}

		case <-ctx.Done():
			t.Logf("测试超时，已完成 %d 个请求", i)
			goto finish
		}
	}

finish:
	stats.EndTime = time.Now()
	totalDuration := stats.EndTime.Sub(stats.StartTime)

	// 计算统计信息
	t.Logf("=== 性能测试结果 ===")
	t.Logf("测试总耗时: %v", totalDuration)
	t.Logf("总请求数: %d", stats.TotalRequests)
	t.Logf("成功请求: %d", stats.SuccessRequests)
	t.Logf("失败请求: %d", stats.FailedRequests)
	t.Logf("成功率: %.2f%%", float64(stats.SuccessRequests)/float64(stats.TotalRequests)*100)

	if len(stats.ResponseTimes) > 0 {
		// 计算平均响应时间
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		t.Logf("平均响应时间: %v", avgResponseTime)
		t.Logf("最小响应时间: %v", stats.MinResponseTime)
		t.Logf("最大响应时间: %v", stats.MaxResponseTime)

		// 计算QPS (每秒请求数)
		qps := float64(stats.TotalRequests) / totalDuration.Seconds()
		t.Logf("QPS (每秒请求数): %.2f", qps)

		// 计算P50, P90, P95, P99响应时间
		sort.Slice(stats.ResponseTimes, func(i, j int) bool {
			return stats.ResponseTimes[i] < stats.ResponseTimes[j]
		})

		p50Index := int(float64(len(stats.ResponseTimes)) * 0.5)
		p90Index := int(float64(len(stats.ResponseTimes)) * 0.9)
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		p99Index := int(float64(len(stats.ResponseTimes)) * 0.99)

		if p50Index < len(stats.ResponseTimes) {
			t.Logf("P50响应时间: %v", stats.ResponseTimes[p50Index])
		}
		if p90Index < len(stats.ResponseTimes) {
			t.Logf("P90响应时间: %v", stats.ResponseTimes[p90Index])
		}
		if p95Index < len(stats.ResponseTimes) {
			t.Logf("P95响应时间: %v", stats.ResponseTimes[p95Index])
		}
		if p99Index < len(stats.ResponseTimes) {
			t.Logf("P99响应时间: %v", stats.ResponseTimes[p99Index])
		}
	}

	// 输出错误统计
	if len(stats.ErrorCounts) > 0 {
		t.Logf("=== 错误统计 ===")
		for errorMsg, count := range stats.ErrorCounts {
			t.Logf("错误: %s (次数: %d)", errorMsg, count)
		}
	}

	// 性能验证
	successRate := float64(stats.SuccessRequests) / float64(stats.TotalRequests) * 100
	if successRate < 95 {
		t.Errorf("成功率过低: %.2f%% (期望 >= 95%%)", successRate)
	}

	// 响应时间验证
	if len(stats.ResponseTimes) > 0 {
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		if avgResponseTime > 1*time.Second {
			t.Errorf("平均响应时间过长: %v (期望 < 1秒)", avgResponseTime)
		}

		// 检查P95响应时间
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		if p95Index < len(stats.ResponseTimes) && stats.ResponseTimes[p95Index] > 2*time.Second {
			t.Errorf("P95响应时间过长: %v (期望 < 2秒)", stats.ResponseTimes[p95Index])
		}
	}

	// QPS验证
	qps := float64(stats.TotalRequests) / totalDuration.Seconds()
	if qps < 50 {
		t.Errorf("QPS过低: %.2f (期望 >= 50)", qps)
	}

	t.Logf("=== GetChallenge性能测试完成 ===")
}

// TestMultiNodeJoinChallenge 并发地向两个不同节点发起加入挑战赛请求，验证多节点一致性与分布式锁效果
// 现在使用通用的多节点测试函数，支持2-3个节点
func TestMultiNodeJoinChallenge(t *testing.T) {
	// 调用通用的多节点测试函数
	TestMultiNodeJoinChallengeGeneric(t)
}

// TestMultiNodeJoinChallengeGeneric 通用的多节点并发加入挑战赛测试
// 支持2-3个节点的并发测试，验证分布式锁和一致性
func TestMultiNodeJoinChallengeGeneric(t *testing.T) {
	// 快速跳过策略：仅在显式要求时执行
	if testing.Short() {
		t.Log("-short 模式下仍执行多节点并发测试…")
	}

	t.Logf("=== 开始多节点并发加入挑战赛测试 ===")

	// 第一阶段：获取节点地址和建立连接
	t.Logf("=== 阶段1: 获取节点地址和建立连接 ===")

	// 获取节点地址列表（支持任意数量，来自环境变量）
	availableNodes := getServerAddresses()
	if len(availableNodes) == 0 {
		t.Fatalf("未找到任何可用的节点地址，请设置环境变量 NAKAMA_ADDRS 或 NAKAMA_ADDR_1..N")
	}

	nodeCount := len(availableNodes)
	t.Logf("可用节点数: %d", nodeCount)
	for i, addr := range availableNodes {
		t.Logf("节点%d: %s", i+1, addr)
	}

	// 建立到所有节点的连接
	type NodeConnection struct {
		Address string
		Conn    *grpc.ClientConn
		Client  apigrpc.NakamaClient
	}

	connections := make([]*NodeConnection, 0, nodeCount)
	dial := func(address string) (*NodeConnection, error) {
		conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("无法连接到 gRPC 服务器 %s: %v", address, err)
		}
		client := apigrpc.NewNakamaClient(conn)
		return &NodeConnection{
			Address: address,
			Conn:    conn,
			Client:  client,
		}, nil
	}

	// 建立所有连接
	for _, addr := range availableNodes {
		conn, err := dial(addr)
		if err != nil {
			t.Fatalf("连接失败: %v", err)
		}
		connections = append(connections, conn)
	}

	// 确保所有连接都被关闭
	defer func() {
		for _, conn := range connections {
			if conn.Conn != nil {
				conn.Conn.Close()
			}
		}
	}()

	// 基础认证头
	baseAuth := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("sparkgame:")))

	// 第二阶段：创建玩家账号
	t.Logf("=== 阶段2: 创建玩家账号 ===")

	const playersPerNode = 50
	type Player struct {
		ID                int
		Username          string
		Session           *api.Session
		Context           context.Context
		TournamentID      string
		JoinSuccess       bool
		Error             error
		ResponseTime      time.Duration
		Node              string
		SelectedConnIndex int
	}

	// 工具：创建或认证账号
	authUser := func(c apigrpc.NakamaClient, prefix string, idx int) (*api.Session, error) {
		req := &api.AuthenticateCustomRequest{
			Account:  &api.AccountCustom{Id: fmt.Sprintf("%s_%d", prefix, idx)},
			Username: fmt.Sprintf("%s_%d", prefix, idx),
			Create:   &wrapperspb.BoolValue{Value: true},
		}
		return c.AuthenticateCustom(baseAuth, req)
	}

	// 为每个节点创建玩家（随机选择连接进行认证与后续请求）
	allPlayers := make([][]*Player, nodeCount)
	totalPlayers := 0

	for nodeIdx := range connections {
		nodeName := fmt.Sprintf("n%d", nodeIdx+1)
		t.Logf("为分组 %s 预定 %d 个玩家（实际将随机分配到各节点）", nodeName, playersPerNode)

		players := make([]*Player, 0, playersPerNode)
		for i := 0; i < playersPerNode; i++ {
			playerID := totalPlayers + i + 1
			// 随机选择一个连接进行后续所有请求
			selectedIdx := rand.Intn(nodeCount)
			selectedConn := connections[selectedIdx]
			sess, err := authUser(selectedConn.Client, "multi_node_user_"+nodeName, i+1)
			if err != nil {
				t.Fatalf("%s 认证失败: %v", nodeName, err)
			}
			p := &Player{
				ID:                playerID,
				Username:          fmt.Sprintf("multi_node_user_%s_%d", nodeName, i+1),
				Session:           sess,
				Context:           metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+sess.Token),
				Node:              selectedConn.Address,
				SelectedConnIndex: selectedIdx,
			}
			players = append(players, p)
		}
		allPlayers[nodeIdx] = players
		totalPlayers += playersPerNode

		if (nodeIdx+1)%10 == 0 || nodeIdx == nodeCount-1 {
			t.Logf("已处理节点 %d/%d", nodeIdx+1, nodeCount)
		}
	}

	t.Logf("成功创建玩家总数: %d", totalPlayers)

	// 第三阶段：获取挑战赛信息
	t.Logf("=== 阶段3: 获取挑战赛信息 ===")

	var targetChallenge *game.Challenge
	{
		// 先创建一个探测用户
		sess, err := authUser(connections[0].Client, "multi_node_probe", 1)
		if err != nil {
			t.Fatalf("探测用户认证失败: %v", err)
		}
		ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+sess.Token)
		resp, err := connections[0].Client.GetChallenge(ctx, &emptypb.Empty{})
		if err != nil || resp == nil || len(resp.Challenges) == 0 {
			t.Fatalf("获取挑战赛失败或为空: %v", err)
		}
		targetChallenge = resp.Challenges[0]
		t.Logf("找到目标挑战赛: ID=%d, 活动ID=%s, 最大参与人数=%d",
			targetChallenge.Id, targetChallenge.ActivityId, targetChallenge.MaxPart)
	}

	// 第四阶段：并发加入挑战赛
	t.Logf("=== 阶段4: %d 个玩家并发加入挑战赛 ===", totalPlayers)

	// 统计信息
	stats := struct {
		JoinSuccessCount  int
		JoinFailureCount  int
		TournamentGroups  map[string]int
		NodeStats         map[string]int
		StartTime         time.Time
		EndTime           time.Time
		ResponseTimes     []time.Duration
		MinResponseTime   time.Duration
		MaxResponseTime   time.Duration
		TotalResponseTime time.Duration
		ErrorCounts       map[string]int
	}{
		TournamentGroups: make(map[string]int),
		NodeStats:        make(map[string]int),
		ResponseTimes:    make([]time.Duration, 0, totalPlayers),
		ErrorCounts:      make(map[string]int),
		StartTime:        time.Now(),
	}

	// 使用channel来收集结果
	resultChan := make(chan *Player, totalPlayers)

	// 启动所有玩家加入挑战赛
	actualLaunchedCount := 0
	for _, players := range allPlayers {
		for _, player := range players {
			if player == nil || player.Context == nil {
				continue
			}

			actualLaunchedCount++
			go func(p *Player) {
				// 记录请求开始时间
				requestStartTime := time.Now()
				// 加入挑战赛
				joinReq := &game.JoinChallengeRequest{
					ChallengeId: targetChallenge.Id,
				}
				joinResp, err := connections[p.SelectedConnIndex].Client.JoinChallenge(p.Context, joinReq)

				// 记录响应时间
				responseTime := time.Since(requestStartTime)
				p.ResponseTime = responseTime

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
			}(player)
		}
	}

	// 收集所有结果
	t.Logf("启动了 %d 个玩家加入挑战赛", actualLaunchedCount)
	t.Logf("等待 %d 个玩家的加入结果...", actualLaunchedCount)

	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for i := 0; i < actualLaunchedCount; i++ {
		select {
		case player := <-resultChan:
			// 记录响应时间统计
			stats.ResponseTimes = append(stats.ResponseTimes, player.ResponseTime)
			stats.TotalResponseTime += player.ResponseTime

			// 更新最小/最大响应时间
			if stats.MinResponseTime == 0 || player.ResponseTime < stats.MinResponseTime {
				stats.MinResponseTime = player.ResponseTime
			}
			if player.ResponseTime > stats.MaxResponseTime {
				stats.MaxResponseTime = player.ResponseTime
			}

			if player.JoinSuccess {
				stats.JoinSuccessCount++
				if player.TournamentID != "" {
					stats.TournamentGroups[player.TournamentID]++
				}
				stats.NodeStats[player.Node]++
				// 简化日志输出，避免终端截断
				t.Logf("✓ 玩家[%d] 成功加入挑战赛 (响应时间: %v)", player.ID, player.ResponseTime)
				t.Logf("   Tournament: %s, 节点: %s", player.TournamentID, player.Node)
			} else {
				stats.JoinFailureCount++
				if player.Error != nil {
					stats.ErrorCounts[player.Error.Error()]++
				}
				t.Logf("✗ 玩家[%d] 加入挑战赛失败: %v (响应时间: %v)", player.ID, player.Error, player.ResponseTime)
			}

			// 每10个玩家输出一次进度
			if (i+1)%10 == 0 {
				select {
				case <-progressTicker.C:
					t.Logf("进度: 已处理 %d/%d 个玩家 (%.1f%%)",
						i+1, actualLaunchedCount, float64(i+1)/float64(actualLaunchedCount)*100)
				default:
				}
			}

		case <-time.After(30 * time.Second):
			t.Logf("等待结果超时，已处理 %d 个玩家", i)
			i = actualLaunchedCount // 结束外层for
		}
	}

	stats.EndTime = time.Now()
	totalDuration := stats.EndTime.Sub(stats.StartTime)

	// 第五阶段：分析结果
	t.Logf("=== 阶段5: 分析结果 ===")
	t.Logf("开始分析结果...")

	t.Logf("总玩家数: %d", actualLaunchedCount)
	t.Logf("成功加入: %d", stats.JoinSuccessCount)
	t.Logf("加入失败: %d", stats.JoinFailureCount)
	t.Logf("创建的tournament数量: %d", len(stats.TournamentGroups))
	t.Logf("测试总耗时: %v", totalDuration)

	// 性能统计
	if len(stats.ResponseTimes) > 0 {
		// 计算平均响应时间
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		t.Logf("=== 性能统计 ===")
		t.Logf("平均响应时间: %v", avgResponseTime)
		t.Logf("最小响应时间: %v", stats.MinResponseTime)
		t.Logf("最大响应时间: %v", stats.MaxResponseTime)

		// 计算QPS (每秒请求数)
		qps := float64(actualLaunchedCount) / totalDuration.Seconds()
		t.Logf("QPS (每秒请求数): %.2f", qps)

		// 计算P50, P90, P95, P99响应时间
		sort.Slice(stats.ResponseTimes, func(i, j int) bool {
			return stats.ResponseTimes[i] < stats.ResponseTimes[j]
		})

		p50Index := int(float64(len(stats.ResponseTimes)) * 0.5)
		p90Index := int(float64(len(stats.ResponseTimes)) * 0.9)
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		p99Index := int(float64(len(stats.ResponseTimes)) * 0.99)

		if p50Index < len(stats.ResponseTimes) {
			t.Logf("P50响应时间: %v", stats.ResponseTimes[p50Index])
		}
		if p90Index < len(stats.ResponseTimes) {
			t.Logf("P90响应时间: %v", stats.ResponseTimes[p90Index])
		}
		if p95Index < len(stats.ResponseTimes) {
			t.Logf("P95响应时间: %v", stats.ResponseTimes[p95Index])
		}
		if p99Index < len(stats.ResponseTimes) {
			t.Logf("P99响应时间: %v", stats.ResponseTimes[p99Index])
		}
	}

	// 输出错误统计
	if len(stats.ErrorCounts) > 0 {
		t.Logf("=== 错误统计 ===")
		for errorMsg, count := range stats.ErrorCounts {
			t.Logf("错误: %s (次数: %d)", errorMsg, count)
		}
	}

	// 验证tournament分配
	t.Logf("计算期望的tournament数量...")
	expectedTournaments := (stats.JoinSuccessCount + int(targetChallenge.MaxPart) - 1) / int(targetChallenge.MaxPart)
	t.Logf("期望的tournament数量: %d", expectedTournaments)

	// 期望与实际数量差异判断
	actualTournaments := len(stats.TournamentGroups)
	diff := expectedTournaments - actualTournaments
	if diff != 0 {
		if diff > 0 {
			t.Errorf("Tournament数量与期望不一致: 期望=%d 实际=%d 差异=%d(少创建)", expectedTournaments, actualTournaments, diff)
		} else {
			t.Errorf("Tournament数量与期望不一致: 期望=%d 实际=%d 差异=%d(多创建)", expectedTournaments, actualTournaments, diff)
		}
	} else {
		t.Logf("✓ Tournament数量与期望一致: %d", actualTournaments)
	}

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

	// 输出节点分布统计
	t.Logf("=== 节点分布统计 ===")
	for node, count := range stats.NodeStats {
		t.Logf("节点 %s: %d 个玩家成功加入", node, count)
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

	// 性能验证
	successRate := float64(stats.JoinSuccessCount) / float64(actualLaunchedCount) * 100
	if successRate < 70 {
		t.Errorf("加入挑战赛成功率过低: %.2f%% (期望 >= 70%%)", successRate)
	}

	// 响应时间验证
	if len(stats.ResponseTimes) > 0 {
		avgResponseTime := stats.TotalResponseTime / time.Duration(len(stats.ResponseTimes))
		if avgResponseTime > 2*time.Second {
			t.Errorf("平均响应时间过长: %v (期望 < 2秒)", avgResponseTime)
		}

		// 检查P95响应时间
		p95Index := int(float64(len(stats.ResponseTimes)) * 0.95)
		if p95Index < len(stats.ResponseTimes) && stats.ResponseTimes[p95Index] > 5*time.Second {
			t.Errorf("P95响应时间过长: %v (期望 < 5秒)", stats.ResponseTimes[p95Index])
		}
	}

	// QPS验证
	qps := float64(actualLaunchedCount) / totalDuration.Seconds()
	if qps < 10 {
		t.Errorf("QPS过低: %.2f (期望 >= 10)", qps)
	}

	// 验证节点分布相对均匀（允许一定偏差）
	expectedPerNode := actualLaunchedCount / nodeCount
	for node, count := range stats.NodeStats {
		diff := count - expectedPerNode
		if diff > expectedPerNode/2 || diff < -expectedPerNode/2 {
			t.Logf("⚠️ 节点 %s 分布偏差较大: %d (期望约 %d)", node, count, expectedPerNode)
		}
	}

	// 第六阶段：提交随机分数测试
	t.Logf("=== 阶段6: 提交随机分数测试 ===")

	scoreSubmissionStats := struct {
		SubmissionSuccess   int
		SubmissionFailure   int
		TotalScores         int
		SubmissionTimes     []time.Duration
		MinSubmissionTime   time.Duration
		MaxSubmissionTime   time.Duration
		TotalSubmissionTime time.Duration
		ErrorCounts         map[string]int
	}{
		SubmissionTimes: make([]time.Duration, 0, stats.JoinSuccessCount*3),
		ErrorCounts:     make(map[string]int),
	}

	// 为每个成功加入的玩家提交随机分数
	for _, players := range allPlayers {
		for _, player := range players {
			if player == nil || !player.JoinSuccess || player.TournamentID == "" {
				continue
			}

			// 为每个玩家生成3个随机分数
			scores := []int64{
				int64(1000 + rand.Intn(9000)), // 1000-10000随机分数
				int64(1000 + rand.Intn(9000)), // 1000-10000随机分数
				int64(1000 + rand.Intn(9000)), // 1000-10000随机分数
			}

			for attemptNum, score := range scores {
				// 记录提交开始时间
				submissionStartTime := time.Now()

				// 创建包含签名的metadata
				metadataFields := map[string]interface{}{
					"player_id": player.ID,
					"attempt":   attemptNum + 1,
					"timestamp": time.Now().Format(time.RFC3339),
					"node":      player.Node,
				}

				signedMetadata, err := createSignedTournamentMetadata(player.TournamentID, score, 0, metadataFields)
				if err != nil {
					t.Logf("✗ 玩家[%d] 创建签名metadata失败: %v", player.ID, err)
					scoreSubmissionStats.SubmissionFailure++
					scoreSubmissionStats.ErrorCounts[err.Error()]++
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
				recordResp, err := connections[player.SelectedConnIndex].Client.WriteTournamentRecord(player.Context, writeReq)

				// 记录提交时间
				submissionTime := time.Since(submissionStartTime)
				scoreSubmissionStats.SubmissionTimes = append(scoreSubmissionStats.SubmissionTimes, submissionTime)
				scoreSubmissionStats.TotalSubmissionTime += submissionTime

				// 更新最小/最大提交时间
				if scoreSubmissionStats.MinSubmissionTime == 0 || submissionTime < scoreSubmissionStats.MinSubmissionTime {
					scoreSubmissionStats.MinSubmissionTime = submissionTime
				}
				if submissionTime > scoreSubmissionStats.MaxSubmissionTime {
					scoreSubmissionStats.MaxSubmissionTime = submissionTime
				}

				if err != nil {
					t.Logf("✗ 玩家[%d] 提交分数失败: %v (耗时: %v)", player.ID, err, submissionTime)
					scoreSubmissionStats.SubmissionFailure++
					scoreSubmissionStats.ErrorCounts[err.Error()]++
				} else {
					scoreSubmissionStats.SubmissionSuccess++
					scoreSubmissionStats.TotalScores++
					t.Logf("✓ 玩家[%d] 成功提交分数: %d (排名: %d, 已签名, 耗时: %v)",
						player.ID, recordResp.Score, recordResp.Rank, submissionTime)
				}

				// 添加小延迟避免过快请求
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	// 输出分数提交统计
	t.Logf("=== 分数提交统计 ===")
	t.Logf("成功提交: %d", scoreSubmissionStats.SubmissionSuccess)
	t.Logf("提交失败: %d", scoreSubmissionStats.SubmissionFailure)
	t.Logf("总分数记录: %d", scoreSubmissionStats.TotalScores)

	if len(scoreSubmissionStats.SubmissionTimes) > 0 {
		avgSubmissionTime := scoreSubmissionStats.TotalSubmissionTime / time.Duration(len(scoreSubmissionStats.SubmissionTimes))
		t.Logf("平均提交时间: %v", avgSubmissionTime)
		t.Logf("最小提交时间: %v", scoreSubmissionStats.MinSubmissionTime)
		t.Logf("最大提交时间: %v", scoreSubmissionStats.MaxSubmissionTime)
	}

	// 输出提交错误统计
	if len(scoreSubmissionStats.ErrorCounts) > 0 {
		t.Logf("=== 分数提交错误统计 ===")
		for errorMsg, count := range scoreSubmissionStats.ErrorCounts {
			t.Logf("错误: %s (次数: %d)", errorMsg, count)
		}
	}

	// 验证分数提交成功率
	expectedSubmissions := stats.JoinSuccessCount * 3 // 每个成功加入的玩家提交3次
	if expectedSubmissions > 0 {
		submissionSuccessRate := float64(scoreSubmissionStats.SubmissionSuccess) / float64(expectedSubmissions) * 100
		if submissionSuccessRate < 70 {
			t.Logf("⚠️ 分数提交成功率过低: %d/%d (%.2f%%)",
				scoreSubmissionStats.SubmissionSuccess, expectedSubmissions, submissionSuccessRate)
		} else {
			t.Logf("✓ 分数提交成功率验证通过: %d/%d (%.2f%%)",
				scoreSubmissionStats.SubmissionSuccess, expectedSubmissions, submissionSuccessRate)
		}
	}

	t.Logf("=== 多节点并发测试完成 ===")
	t.Logf("测试已正常结束")
}

// TestThreeNodeJoinChallenge 专门测试3个节点的并发加入挑战赛
func TestThreeNodeJoinChallenge(t *testing.T) {
	// 快速跳过策略：仅在显式要求时执行
	if testing.Short() {
		t.Log("-short 模式下仍执行三节点并发测试…")
	}

	// 强制使用3个节点
	os.Setenv("NAKAMA_ADDR_1", "localhost:7349")
	os.Setenv("NAKAMA_ADDR_2", "localhost:7449")
	os.Setenv("NAKAMA_ADDR_3", "localhost:7549")

	// 调用通用的多节点测试函数
	TestMultiNodeJoinChallengeGeneric(t)
}
