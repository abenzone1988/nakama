package server

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.uber.org/zap"
)

func setupRedisLeaderboardCache(t *testing.T) (*RedisLeaderboardRankCache, func()) {
	logger := zap.NewExample()

	// 测试配置
	config := &LeaderboardConfig{
		BlacklistRankCache:   []string{},
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
