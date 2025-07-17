package server

import (
	"context"
	"flag"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func setupRedisLeaderboardCache(t *testing.T) (*RedisLeaderboardRankCache, func()) {
	logger := zap.NewExample()

	// 从环境变量或命令行参数获取Redis配置
	redisHost := "localhost"
	redisPort := "6379"
	redisPassword := ""

	// 检查环境变量
	if envHost := os.Getenv("REDIS_HOST"); envHost != "" {
		redisHost = envHost
	}
	if envPort := os.Getenv("REDIS_PORT"); envPort != "" {
		redisPort = envPort
	}
	if envPassword := os.Getenv("REDIS_PASSWORD"); envPassword != "" {
		redisPassword = envPassword
	}

	// 检查命令行参数
	flag.StringVar(&redisHost, "redis-host", redisHost, "Redis host address")
	flag.StringVar(&redisPort, "redis-port", redisPort, "Redis port")
	flag.StringVar(&redisPassword, "redis-password", redisPassword, "Redis password")
	flag.Parse()

	redisAddress := redisHost + ":" + redisPort

	// 测试配置
	config := &LeaderboardConfig{
		BlacklistRankCache:   []string{},
		CallbackQueueSize:    1000,
		CallbackQueueWorkers: 2,
		RankCacheWorkers:     1,
		UseRedis:             true,
		RedisAddress:         redisAddress,
		RedisPassword:        redisPassword,
	}

	cache := NewRedisLeaderboardRankCache(
		context.Background(),
		logger,
		config.RedisAddress,
		config.RedisPassword,
		config,
	).(*RedisLeaderboardRankCache)

	// 清理函数
	cleanup := func() {
		// 清理测试数据
		pattern := "leaderboard:*"
		keys, _ := cache.client.Keys(context.Background(), pattern).Result()
		if len(keys) > 0 {
			cache.client.Del(context.Background(), keys...)
		}
		cache.Stop()
	}

	return cache, cleanup
}

func TestRedisLeaderboardRankCache_BasicOperations(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	// 测试数据
	leaderboardID := "test_leaderboard"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID1 := uuid.Must(uuid.NewV4())
	ownerID2 := uuid.Must(uuid.NewV4())

	// 测试插入
	rank1 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID1, true)
	if rank1 != 1 {
		t.Errorf("Expected rank 1, got %d", rank1)
	}

	rank2 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 900, 90, 1, expiryUnix, ownerID2, true)
	if rank2 != 2 {
		t.Errorf("Expected rank 2, got %d", rank2)
	}

	// 测试查询排名
	queriedRank1 := cache.Get(leaderboardID, expiryUnix, ownerID1)
	if queriedRank1 != 1 {
		t.Errorf("Expected rank 1, got %d", queriedRank1)
	}

	queriedRank2 := cache.Get(leaderboardID, expiryUnix, ownerID2)
	if queriedRank2 != 2 {
		t.Errorf("Expected rank 2, got %d", queriedRank2)
	}

	// 测试按排名查询数据
	ownerID, score, subscore, err := cache.GetDataByRank(leaderboardID, expiryUnix, LeaderboardSortOrderDescending, 1)
	if err != nil {
		t.Errorf("GetDataByRank failed: %v", err)
	}
	if ownerID != ownerID1 {
		t.Errorf("Expected ownerID %s, got %s", ownerID1, ownerID)
	}

	t.Log(score, subscore)

	// 测试删除
	deleted := cache.Delete(leaderboardID, expiryUnix, ownerID1)
	if !deleted {
		t.Error("Expected delete to succeed")
	}

	// 验证删除后的排名
	newRank2 := cache.Get(leaderboardID, expiryUnix, ownerID2)
	if newRank2 != 1 {
		t.Errorf("Expected rank 1 after deletion, got %d", newRank2)
	}

	// 测试删除整个排行榜
	deleted = cache.DeleteLeaderboard(leaderboardID, expiryUnix)
	if !deleted {
		t.Error("Expected leaderboard delete to succeed")
	}
}

func TestRedisLeaderboardRankCache_Generation(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	leaderboardID := "test_generation"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID := uuid.Must(uuid.NewV4())

	// 插入初始数据
	rank1 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
	if rank1 != 1 {
		t.Errorf("Expected rank 1, got %d", rank1)
	}

	// 尝试用更小的generation更新，应该被拒绝
	_ = cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 2000, 200, 0, expiryUnix, ownerID, true)
	// 由于generation较小，排名应该保持不变
	currentRank := cache.Get(leaderboardID, expiryUnix, ownerID)
	if currentRank != 1 {
		t.Errorf("Expected rank to remain 1, got %d", currentRank)
	}

	// 用更大的generation更新，应该成功
	rank3 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 2000, 200, 2, expiryUnix, ownerID, true)
	if rank3 != 1 {
		t.Errorf("Expected rank 1 after valid update, got %d", rank3)
	}
}

func TestRedisLeaderboardRankCache_LocalCache(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	leaderboardID := "test_local_cache"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID := uuid.Must(uuid.NewV4())

	// 插入数据
	cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)

	// 第一次查询，会缓存到本地
	rank1 := cache.Get(leaderboardID, expiryUnix, ownerID)
	if rank1 != 1 {
		t.Errorf("Expected rank 1, got %d", rank1)
	}

	// 第二次查询，应该从本地缓存返回
	rank2 := cache.Get(leaderboardID, expiryUnix, ownerID)
	if rank2 != 1 {
		t.Errorf("Expected rank 1 from local cache, got %d", rank2)
	}

	// 手动使本地缓存失效
	cacheKey := cache.getLocalCacheKey(leaderboardID, expiryUnix, ownerID)
	cache.localCache.invalidate(cacheKey)

	// 再次查询，应该从Redis获取
	rank3 := cache.Get(leaderboardID, expiryUnix, ownerID)
	if rank3 != 1 {
		t.Errorf("Expected rank 1 after cache invalidation, got %d", rank3)
	}
}

func TestRedisLeaderboardRankCache_Blacklist(t *testing.T) {
	logger := zap.NewExample()

	// 带黑名单的配置
	config := &LeaderboardConfig{
		BlacklistRankCache:   []string{"blacklisted_leaderboard"},
		CallbackQueueSize:    1000,
		CallbackQueueWorkers: 2,
		RankCacheWorkers:     1,
		UseRedis:             true,
		RedisAddress:         "localhost:6379",
		RedisPassword:        "",
	}

	cache := NewRedisLeaderboardRankCache(
		context.Background(),
		logger,
		config.RedisAddress,
		config.RedisPassword,
		config,
	).(*RedisLeaderboardRankCache)
	defer cache.Stop()

	leaderboardID := "blacklisted_leaderboard"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID := uuid.Must(uuid.NewV4())

	// 黑名单中的排行榜应该返回0
	rank := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
	if rank != 0 {
		t.Errorf("Expected rank 0 for blacklisted leaderboard, got %d", rank)
	}

	queriedRank := cache.Get(leaderboardID, expiryUnix, ownerID)
	if queriedRank != 0 {
		t.Errorf("Expected rank 0 for blacklisted leaderboard, got %d", queriedRank)
	}
}

func BenchmarkRedisLeaderboardRankCache_Get(b *testing.B) {
	cache, cleanup := setupRedisLeaderboardCache(nil)
	defer cleanup()

	leaderboardID := "benchmark_leaderboard"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerIDs := make([]uuid.UUID, 1000)

	// 预填充数据
	for i := 0; i < 1000; i++ {
		ownerIDs[i] = uuid.Must(uuid.NewV4())
		cache.Insert(leaderboardID, LeaderboardSortOrderDescending, int64(1000-i), int64(i), 1, expiryUnix, ownerIDs[i], true)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ownerID := ownerIDs[i%len(ownerIDs)]
			cache.Get(leaderboardID, expiryUnix, ownerID)
			i++
		}
	})
}

func BenchmarkRedisLeaderboardRankCache_Insert(b *testing.B) {
	cache, cleanup := setupRedisLeaderboardCache(nil)
	defer cleanup()

	leaderboardID := "benchmark_insert"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ownerID := uuid.Must(uuid.NewV4())
			cache.Insert(leaderboardID, LeaderboardSortOrderDescending, int64(i), int64(i), 1, expiryUnix, ownerID, true)
			i++
		}
	})
}

// TestRedisLeaderboardRankCache_DistributedConsistency 分布式一致性测试
func TestRedisLeaderboardRankCache_DistributedConsistency(t *testing.T) {
	// 创建多个缓存实例模拟不同节点
	cache1, cleanup1 := setupRedisLeaderboardCache(t)
	defer cleanup1()
	cache2, cleanup2 := setupRedisLeaderboardCache(t)
	defer cleanup2()
	cache3, cleanup3 := setupRedisLeaderboardCache(t)
	defer cleanup3()

	leaderboardID := "distributed_test"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID := uuid.Must(uuid.NewV4())

	t.Log("=== 分布式一致性测试 ===")

	// 1. 测试基本插入和查询一致性
	t.Run("Basic Insert and Query Consistency", func(t *testing.T) {
		// 在节点1插入数据
		rank := cache1.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
		if rank != 1 {
			t.Errorf("Expected rank 1, got %d", rank)
		}

		// 等待消息传播
		time.Sleep(100 * time.Millisecond)

		// 在所有节点查询，应该得到相同结果
		rank1 := cache1.Get(leaderboardID, expiryUnix, ownerID)
		rank2 := cache2.Get(leaderboardID, expiryUnix, ownerID)
		rank3 := cache3.Get(leaderboardID, expiryUnix, ownerID)

		if rank1 != 1 {
			t.Errorf("节点1查询结果: expected 1, got %d", rank1)
		}
		if rank2 != 1 {
			t.Errorf("节点2查询结果: expected 1, got %d", rank2)
		}
		if rank3 != 1 {
			t.Errorf("节点3查询结果: expected 1, got %d", rank3)
		}
	})

	// 2. 测试并发更新一致性
	t.Run("Concurrent Update Consistency", func(t *testing.T) {
		ownerID2 := uuid.Must(uuid.NewV4())
		ownerID3 := uuid.Must(uuid.NewV4())

		var wg sync.WaitGroup
		results := make([]int64, 3)

		// 并发插入数据
		wg.Add(3)
		go func() {
			defer wg.Done()
			results[0] = cache1.Insert(leaderboardID, LeaderboardSortOrderDescending, 900, 90, 1, expiryUnix, ownerID2, true)
		}()
		go func() {
			defer wg.Done()
			results[1] = cache2.Insert(leaderboardID, LeaderboardSortOrderDescending, 800, 80, 1, expiryUnix, ownerID3, true)
		}()
		go func() {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond) // 稍微延迟
			results[2] = cache3.Insert(leaderboardID, LeaderboardSortOrderDescending, 950, 95, 1, expiryUnix, uuid.Must(uuid.NewV4()), true)
		}()

		wg.Wait()

		// 等待消息传播
		time.Sleep(200 * time.Millisecond)

		// 验证所有节点的数据一致
		for i, cache := range []*RedisLeaderboardRankCache{cache1, cache2, cache3} {
			rank1 := cache.Get(leaderboardID, expiryUnix, ownerID)
			rank2 := cache.Get(leaderboardID, expiryUnix, ownerID2)
			rank3 := cache.Get(leaderboardID, expiryUnix, ownerID3)

			t.Logf("节点%d - 排名: owner1=%d, owner2=%d, owner3=%d", i+1, rank1, rank2, rank3)

			// 验证排名逻辑正确
			if rank1 != 1 {
				t.Errorf("节点%d: 最高分应该排名1, got %d", i+1, rank1)
			}
			if rank2 < 1 || rank2 > 3 {
				t.Errorf("节点%d: 排名应该在1-3之间, got %d", i+1, rank2)
			}
			if rank3 < 1 || rank3 > 3 {
				t.Errorf("节点%d: 排名应该在1-3之间, got %d", i+1, rank3)
			}
		}
	})

	// 3. 测试删除一致性
	t.Run("Delete Consistency", func(t *testing.T) {
		// 在节点1删除数据
		deleted := cache1.Delete(leaderboardID, expiryUnix, ownerID)
		if !deleted {
			t.Error("删除应该成功")
		}

		// 等待消息传播
		time.Sleep(100 * time.Millisecond)

		// 验证所有节点都删除了数据
		rank1 := cache1.Get(leaderboardID, expiryUnix, ownerID)
		rank2 := cache2.Get(leaderboardID, expiryUnix, ownerID)
		rank3 := cache3.Get(leaderboardID, expiryUnix, ownerID)

		if rank1 != 0 {
			t.Errorf("节点1应该返回0, got %d", rank1)
		}
		if rank2 != 0 {
			t.Errorf("节点2应该返回0, got %d", rank2)
		}
		if rank3 != 0 {
			t.Errorf("节点3应该返回0, got %d", rank3)
		}
	})
}

// TestRedisLeaderboardRankCache_NetworkPartition 网络分区测试
func TestRedisLeaderboardRankCache_NetworkPartition(t *testing.T) {
	cache1, cleanup1 := setupRedisLeaderboardCache(t)
	defer cleanup1()
	cache2, cleanup2 := setupRedisLeaderboardCache(t)
	defer cleanup2()

	leaderboardID := "network_partition_test"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID := uuid.Must(uuid.NewV4())

	t.Log("=== 网络分区测试 ===")

	// 1. 正常情况下的数据同步
	t.Run("Normal Synchronization", func(t *testing.T) {
		rank := cache1.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
		if rank != 1 {
			t.Errorf("Expected rank 1, got %d", rank)
		}

		time.Sleep(100 * time.Millisecond)

		rank2 := cache2.Get(leaderboardID, expiryUnix, ownerID)
		if rank2 != 1 {
			t.Errorf("节点2应该能获取到数据, expected 1, got %d", rank2)
		}
	})

	// 2. 模拟网络分区（通过禁用发布/订阅）
	t.Run("Network Partition Simulation", func(t *testing.T) {
		// 模拟网络分区：停止cache2的发布/订阅
		if cache2.pubsub != nil {
			cache2.pubsub.Close()
			cache2.pubsub = nil
		}

		ownerID2 := uuid.Must(uuid.NewV4())
		rank := cache1.Insert(leaderboardID, LeaderboardSortOrderDescending, 900, 90, 1, expiryUnix, ownerID2, true)
		if rank != 2 {
			t.Errorf("Expected rank 2, got %d", rank)
		}

		// 等待一段时间
		time.Sleep(100 * time.Millisecond)

		// cache2应该无法获取到新数据（因为网络分区）
		rank2 := cache2.Get(leaderboardID, expiryUnix, ownerID2)
		if rank2 != 0 {
			t.Errorf("网络分区时应该无法获取新数据, expected 0, got %d", rank2)
		}

		// 但cache1应该能正常工作
		rank1 := cache1.Get(leaderboardID, expiryUnix, ownerID2)
		if rank1 != 2 {
			t.Errorf("cache1应该能正常工作, expected 2, got %d", rank1)
		}
	})

	// 3. 网络恢复测试
	t.Run("Network Recovery", func(t *testing.T) {
		// 重新初始化cache2的发布/订阅
		cache2.initPubSub()

		// 等待重新连接
		time.Sleep(200 * time.Millisecond)

		// 插入新数据
		ownerID3 := uuid.Must(uuid.NewV4())
		rank := cache1.Insert(leaderboardID, LeaderboardSortOrderDescending, 800, 80, 1, expiryUnix, ownerID3, true)

		time.Sleep(100 * time.Millisecond)

		// 验证数据同步
		rank1 := cache1.Get(leaderboardID, expiryUnix, ownerID3)
		rank2 := cache2.Get(leaderboardID, expiryUnix, ownerID3)

		if rank != rank1 {
			t.Errorf("cache1应该能获取数据, expected %d, got %d", rank, rank1)
		}
		if rank != rank2 {
			t.Errorf("cache2应该能获取数据, expected %d, got %d", rank, rank2)
		}
	})
}

// TestRedisLeaderboardRankCache_Performance 性能测试
func TestRedisLeaderboardRankCache_Performance(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	leaderboardID := "performance_test"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()

	t.Log("=== 性能测试 ===")

	// 1. 批量插入性能测试
	t.Run("Batch Insert Performance", func(t *testing.T) {
		const testSize = 1000
		ownerIDs := make([]uuid.UUID, testSize)
		for i := 0; i < testSize; i++ {
			ownerIDs[i] = uuid.Must(uuid.NewV4())
		}

		start := time.Now()
		for i := 0; i < testSize; i++ {
			cache.Insert(leaderboardID, LeaderboardSortOrderDescending, int64(1000-i), int64(100-i), 1, expiryUnix, ownerIDs[i], true)
		}
		duration := time.Since(start)

		t.Logf("插入%d条记录耗时: %v", testSize, duration)
		t.Logf("平均每条记录: %v", duration/time.Duration(testSize))
		t.Logf("吞吐量: %.2f ops/sec", float64(testSize)/duration.Seconds())

		// 性能要求：每秒至少1000次操作
		if float64(testSize)/duration.Seconds() < 1000 {
			t.Errorf("吞吐量应该大于1000 ops/sec, got %.2f", float64(testSize)/duration.Seconds())
		}
	})

	// 2. 批量查询性能测试
	t.Run("Batch Query Performance", func(t *testing.T) {
		const testSize = 1000
		ownerIDs := make([]uuid.UUID, testSize)
		for i := 0; i < testSize; i++ {
			ownerIDs[i] = uuid.Must(uuid.NewV4())
			cache.Insert(leaderboardID, LeaderboardSortOrderDescending, int64(1000-i), int64(100-i), 1, expiryUnix, ownerIDs[i], true)
		}

		// 等待数据同步
		time.Sleep(100 * time.Millisecond)

		start := time.Now()
		for i := 0; i < testSize; i++ {
			cache.Get(leaderboardID, expiryUnix, ownerIDs[i])
		}
		duration := time.Since(start)

		t.Logf("查询%d条记录耗时: %v", testSize, duration)
		t.Logf("平均每条记录: %v", duration/time.Duration(testSize))
		t.Logf("吞吐量: %.2f ops/sec", float64(testSize)/duration.Seconds())

		// 性能要求：每秒至少2000次查询
		if float64(testSize)/duration.Seconds() < 2000 {
			t.Errorf("查询吞吐量应该大于2000 ops/sec, got %.2f", float64(testSize)/duration.Seconds())
		}
	})

	// 3. 并发性能测试
	t.Run("Concurrent Performance", func(t *testing.T) {
		const testSize = 1000
		const numGoroutines = 10

		var wg sync.WaitGroup
		start := time.Now()

		// 启动多个goroutine并发操作
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < testSize/numGoroutines; j++ {
					ownerID := uuid.Must(uuid.NewV4())
					score := int64(1000 - j)
					cache.Insert(leaderboardID, LeaderboardSortOrderDescending, score, int64(100-j), 1, expiryUnix, ownerID, true)
					cache.Get(leaderboardID, expiryUnix, ownerID)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		t.Logf("并发操作%d条记录耗时: %v", testSize, duration)
		t.Logf("平均每条记录: %v", duration/time.Duration(testSize))
		t.Logf("并发吞吐量: %.2f ops/sec", float64(testSize)/duration.Seconds())
	})
}

// TestRedisLeaderboardRankCache_FaultTolerance 故障恢复测试
func TestRedisLeaderboardRankCache_FaultTolerance(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	leaderboardID := "fault_tolerance_test"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()

	t.Log("=== 故障恢复测试 ===")

	// 1. Redis连接失败时的降级测试
	t.Run("Redis Connection Failure", func(t *testing.T) {
		// 插入一些数据
		ownerID := uuid.Must(uuid.NewV4())
		rank := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
		if rank != 1 {
			t.Errorf("Expected rank 1, got %d", rank)
		}

		// 模拟Redis连接失败
		cache.client.Close()

		// 尝试查询，应该返回0（降级处理）
		rank2 := cache.Get(leaderboardID, expiryUnix, ownerID)
		if rank2 != 0 {
			t.Errorf("Redis连接失败时应该返回0, got %d", rank2)
		}

		// 尝试插入，应该返回0（降级处理）
		ownerID2 := uuid.Must(uuid.NewV4())
		rank3 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 900, 90, 1, expiryUnix, ownerID2, true)
		if rank3 != 0 {
			t.Errorf("Redis连接失败时插入应该返回0, got %d", rank3)
		}
	})

	// 2. 数据一致性验证
	t.Run("Data Consistency Verification", func(t *testing.T) {
		// 重新连接Redis
		cache.client = redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       1,
		})

		// 验证数据是否仍然存在
		ownerID := uuid.Must(uuid.NewV4())
		rank := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
		if rank != 1 {
			t.Errorf("重新连接后应该能正常工作, expected 1, got %d", rank)
		}

		rank2 := cache.Get(leaderboardID, expiryUnix, ownerID)
		if rank2 != 1 {
			t.Errorf("重新连接后应该能正常查询, expected 1, got %d", rank2)
		}
	})
}

// TestRedisLeaderboardRankCache_GenerationConflict 代数冲突测试
func TestRedisLeaderboardRankCache_GenerationConflict(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	leaderboardID := "generation_conflict_test"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()
	ownerID := uuid.Must(uuid.NewV4())

	t.Log("=== 代数冲突测试 ===")

	// 1. 测试新数据覆盖旧数据
	t.Run("New Data Overwrites Old Data", func(t *testing.T) {
		// 插入低代数数据
		rank1 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 1000, 100, 1, expiryUnix, ownerID, true)
		if rank1 != 1 {
			t.Errorf("Expected rank 1, got %d", rank1)
		}

		// 插入高代数数据
		rank2 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 900, 90, 2, expiryUnix, ownerID, true)
		if rank2 != 1 {
			t.Errorf("高代数数据应该覆盖低代数数据, expected 1, got %d", rank2)
		}

		// 验证最终数据
		finalRank := cache.Get(leaderboardID, expiryUnix, ownerID)
		if finalRank != 1 {
			t.Errorf("应该使用最新的数据, expected 1, got %d", finalRank)
		}
	})

	// 2. 测试旧数据不覆盖新数据
	t.Run("Old Data Does Not Overwrite New Data", func(t *testing.T) {
		ownerID2 := uuid.Must(uuid.NewV4())

		// 插入高代数数据
		rank1 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 800, 80, 3, expiryUnix, ownerID2, true)
		if rank1 != 2 {
			t.Errorf("Expected rank 2, got %d", rank1)
		}

		// 尝试插入低代数数据
		rank2 := cache.Insert(leaderboardID, LeaderboardSortOrderDescending, 700, 70, 2, expiryUnix, ownerID2, true)
		if rank2 != 2 {
			t.Errorf("低代数数据不应该覆盖高代数数据, expected 2, got %d", rank2)
		}

		// 验证数据没有被覆盖
		finalRank := cache.Get(leaderboardID, expiryUnix, ownerID2)
		if finalRank != 2 {
			t.Errorf("应该保持原有数据, expected 2, got %d", finalRank)
		}
	})
}

// TestRedisLeaderboardRankCache_Fill 批量填充测试
func TestRedisLeaderboardRankCache_Fill(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	leaderboardID := "fill_test"
	expiryUnix := time.Now().Add(1 * time.Hour).Unix()

	t.Log("=== 批量填充测试 ===")

	// 插入测试数据
	ownerIDs := make([]uuid.UUID, 10)
	records := make([]*api.LeaderboardRecord, 10)

	for i := 0; i < 10; i++ {
		ownerIDs[i] = uuid.Must(uuid.NewV4())
		records[i] = &api.LeaderboardRecord{
			OwnerId:  ownerIDs[i].String(),
			Score:    int64(1000 - i*10),
			Subscore: int64(100 - i),
		}
		cache.Insert(leaderboardID, LeaderboardSortOrderDescending, int64(1000-i*10), int64(100-i), 1, expiryUnix, ownerIDs[i], true)
	}

	// 等待数据同步
	time.Sleep(100 * time.Millisecond)

	// 测试批量填充
	count := cache.Fill(leaderboardID, expiryUnix, records, true)
	if count != 10 {
		t.Errorf("应该填充10条记录, got %d", count)
	}

	// 验证排名是否正确
	for i, record := range records {
		expectedRank := int64(i + 1)
		if record.Rank != expectedRank {
			t.Errorf("排名应该正确, expected %d, got %d", expectedRank, record.Rank)
		}
		t.Logf("记录%d: 用户=%s, 分数=%d, 排名=%d", i+1, record.OwnerId, record.Score, record.Rank)
	}
}

// TestRedisLeaderboardRankCache_HealthCheck 健康检查测试
func TestRedisLeaderboardRankCache_HealthCheck(t *testing.T) {
	cache, cleanup := setupRedisLeaderboardCache(t)
	defer cleanup()

	t.Log("=== 健康检查测试 ===")

	// 测试健康检查
	err := cache.HealthCheck()
	if err != nil {
		t.Errorf("健康检查应该通过: %v", err)
	}

	// 测试统计信息
	stats := cache.GetStats()
	if stats == nil {
		t.Error("统计信息不应该为空")
	}

	// 验证必要的统计字段
	if _, ok := stats["local_cache_size"]; !ok {
		t.Error("应该包含本地缓存大小")
	}
	if _, ok := stats["node_id"]; !ok {
		t.Error("应该包含节点ID")
	}
	if _, ok := stats["blacklist_all"]; !ok {
		t.Error("应该包含黑名单状态")
	}

	t.Logf("缓存统计信息: %+v", stats)
}
