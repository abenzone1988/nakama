package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupRedisV3(t *testing.T) (*RedisSessionCacheV3, func()) {
	logger := zap.NewNop()
	cache := NewRedisSessionCacheV3(logger, "localhost:6379", "", 300, 3600, false).(*RedisSessionCacheV3)

	// 清理函数
	cleanup := func() {
		cache.client.FlushDB(context.Background())
		cache.Stop()
	}

	return cache, cleanup
}

func TestRedisSessionCacheV3_Performance(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())

	// 生成测试数据
	sessionTokens := make([]string, testTokenCount)
	refreshTokens := make([]string, testTokenCount)
	for i := 0; i < testTokenCount; i++ {
		sessionTokens[i] = fmt.Sprintf("session-%d", i)
		refreshTokens[i] = fmt.Sprintf("refresh-%d", i)
	}

	// 测试添加性能
	startAdd := time.Now()
	for i := 0; i < testTokenCount; i++ {
		cache.Add(userID, 300, sessionTokens[i], 3600, refreshTokens[i])
	}
	addDuration := time.Since(startAdd)
	t.Logf("添加 %d 个token对用时: %v", testTokenCount, addDuration)

	// 测试验证性能
	startValidate := time.Now()
	for i := 0; i < testTokenCount; i++ {
		assert.True(t, cache.IsValidSession(userID, 300, sessionTokens[i]))
		assert.True(t, cache.IsValidRefresh(userID, 3600, refreshTokens[i]))
	}
	validateDuration := time.Since(startValidate)
	t.Logf("验证 %d 个token对用时: %v", testTokenCount, validateDuration)

	// 验证总执行时间
	totalDuration := addDuration + validateDuration
	t.Logf("总执行时间: %v", totalDuration)

	// 验证性能要求
	assert.Less(t, validateDuration.Seconds(), float64(5), "验证时间应小于5秒")
}

func TestRedisSessionCacheV3_LocalCache(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())

	// 生成多个不同的token进行测试
	sessionTokens := make([]string, 1000) // 使用1000个不同的token
	for i := 0; i < len(sessionTokens); i++ {
		sessionTokens[i] = fmt.Sprintf("session-token-%d", i)
		cache.Add(userID, 300, sessionTokens[i], 3600, "")
	}

	// 清除本地缓存，确保第一次验证从Redis读取
	cache.localCache = newTokenCache()

	// 预热Redis连接
	cache.IsValidSession(userID, 300, sessionTokens[0])

	// 第一次验证（从Redis读取）
	start := time.Now()
	for _, token := range sessionTokens {
		assert.True(t, cache.IsValidSession(userID, 300, token))
	}
	firstDuration := time.Since(start)

	// 第二次验证（从本地缓存读取）
	start = time.Now()
	for _, token := range sessionTokens {
		assert.True(t, cache.IsValidSession(userID, 300, token))
	}
	secondDuration := time.Since(start)

	// 计算性能差异
	redisAvg := float64(firstDuration.Nanoseconds()) / float64(len(sessionTokens))
	cacheAvg := float64(secondDuration.Nanoseconds()) / float64(len(sessionTokens))
	speedup := redisAvg / cacheAvg

	// 验证本地缓存是否提升性能
	t.Logf("首次验证用时(Redis): %v, 平均每个token: %v", firstDuration, time.Duration(redisAvg))
	t.Logf("缓存验证用时(Local): %v, 平均每个token: %v", secondDuration, time.Duration(cacheAvg))
	t.Logf("性能提升倍数: %.2f", speedup)

	// 验证本地缓存是否显著快于Redis
	// 考虑到本地Redis服务器的情况，只要求本地缓存比Redis快20%即可
	assert.Greater(t, speedup, float64(1.2), "本地缓存应该比Redis快至少20%")
}

func TestRedisSessionCacheV3_CacheExpiry(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session"

	// 添加token
	cache.Add(userID, 300, sessionToken, 3600, "")

	// 修改缓存过期时间为1秒用于测试
	cache.localCache.set(cache.getCacheKey(userID, sessionToken, "session"), true, 1*time.Second)

	// 验证token（应该从缓存读取）
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken))

	// 等待缓存过期
	time.Sleep(2 * time.Second)

	// 再次验证（应该从Redis读取）
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken))
}

func TestRedisSessionCacheV3_Ban(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session"
	refreshToken := "test-refresh"

	// 添加token
	cache.Add(userID, 300, sessionToken, 3600, refreshToken)

	// 验证token有效
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 3600, refreshToken))

	// 封禁用户
	cache.Ban([]uuid.UUID{userID})

	// 验证token已失效
	assert.False(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.False(t, cache.IsValidRefresh(userID, 3600, refreshToken))

	// 验证本地缓存已更新
	valid, exists := cache.localCache.get(cache.getCacheKey(userID, sessionToken, "session"))
	assert.True(t, exists)
	assert.False(t, valid)
}

func TestRedisSessionCacheV3_SingleToken(t *testing.T) {
	logger := zap.NewNop()
	cache := NewRedisSessionCacheV3(logger, "localhost:6379", "", 300, 3600, true).(*RedisSessionCacheV3)
	defer func() {
		cache.client.FlushDB(context.Background())
		cache.Stop()
	}()

	userID := uuid.Must(uuid.NewV4())
	sessionToken1 := "session-1"
	sessionToken2 := "session-2"

	// 添加第一个token
	cache.Add(userID, 300, sessionToken1, 3600, "")
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken1))

	// 添加第二个token（应该使第一个token失效）
	cache.Add(userID, 300, sessionToken2, 3600, "")
	assert.False(t, cache.IsValidSession(userID, 300, sessionToken1))
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken2))
}

func TestRedisSessionCacheV3_Remove(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session"
	refreshToken := "test-refresh"

	// 添加token
	cache.Add(userID, 300, sessionToken, 3600, refreshToken)

	// 验证token有效
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 3600, refreshToken))

	// 移除session token
	cache.Remove(userID, 300, sessionToken, 3600, "")

	// 验证session token已失效，refresh token仍有效
	assert.False(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 3600, refreshToken))

	// 验证本地缓存已更新
	valid, exists := cache.localCache.get(cache.getCacheKey(userID, sessionToken, "session"))
	assert.True(t, exists)
	assert.False(t, valid)
}

func TestRedisSessionCacheV3_RemoveAll(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session"
	refreshToken := "test-refresh"

	// 添加token
	cache.Add(userID, 300, sessionToken, 3600, refreshToken)

	// 验证token有效
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 3600, refreshToken))

	// 移除所有token
	cache.RemoveAll(userID)

	// 验证所有token已失效
	assert.False(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.False(t, cache.IsValidRefresh(userID, 3600, refreshToken))

	// 验证Redis中的数据已被删除
	tokenSetKey := cache.getUserTokenSetKey(userID)
	tokenHashKey := cache.getTokenHashKey(userID)

	exists, err := cache.client.Exists(context.Background(), tokenSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	exists, err = cache.client.Exists(context.Background(), tokenHashKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

// 模拟分布式环境下的性能测试
func TestRedisSessionCacheV3_DistributedPerformance(t *testing.T) {
	// 创建两个缓存实例模拟不同节点
	cache1, cleanup1 := setupRedisV3(t)
	defer cleanup1()
	cache2, cleanup2 := setupRedisV3(t)
	defer cleanup2()

	userID := uuid.Must(uuid.NewV4())
	testSize := 1000
	operationsPerToken := 5 // 每个token的操作次数

	t.Logf("\n=== 分布式环境性能测试 ===")
	t.Logf("配置:")
	t.Logf("- Token数量: %d", testSize)
	t.Logf("- 每个Token操作次数: %d", operationsPerToken)

	// 1. 测试写入性能（包含Pub/Sub开销）
	tokens := make([]string, testSize)
	start := time.Now()
	for i := 0; i < testSize; i++ {
		tokens[i] = fmt.Sprintf("session-token-%d", i)
		cache1.Add(userID, 300, tokens[i], 3600, "")
	}
	addDuration := time.Since(start)
	t.Logf("\n1. 写入性能（带消息发布）:")
	t.Logf("   - 总耗时: %v", addDuration)
	t.Logf("   - 平均每个token: %v", addDuration/time.Duration(testSize))

	// 等待消息传播
	time.Sleep(100 * time.Millisecond)

	// 2. 测试节点间的缓存同步
	var syncErrors int
	start = time.Now()
	for i := 0; i < testSize; i++ {
		if !cache2.IsValidSession(userID, 300, tokens[i]) {
			syncErrors++
		}
	}
	syncDuration := time.Since(start)
	t.Logf("\n2. 节点间同步验证:")
	t.Logf("   - 总耗时: %v", syncDuration)
	t.Logf("   - 平均每个token: %v", syncDuration/time.Duration(testSize))
	t.Logf("   - 同步错误数: %d", syncErrors)

	// 3. 测试删除性能（包含Pub/Sub开销）
	start = time.Now()
	for i := 0; i < testSize; i++ {
		cache1.Remove(userID, 300, tokens[i], 3600, "")
	}
	removeDuration := time.Since(start)
	t.Logf("\n3. 删除性能（带消息发布）:")
	t.Logf("   - 总耗时: %v", removeDuration)
	t.Logf("   - 平均每个token: %v", removeDuration/time.Duration(testSize))

	// 等待消息传播
	time.Sleep(100 * time.Millisecond)

	// 4. 测试并发读写性能
	t.Logf("\n4. 并发读写性能测试:")

	// 准备新的测试数据
	concurrentTokens := make([]string, testSize)
	for i := 0; i < testSize; i++ {
		concurrentTokens[i] = fmt.Sprintf("concurrent-token-%d", i)
		cache1.Add(userID, 300, concurrentTokens[i], 3600, "")
	}

	var wg sync.WaitGroup
	start = time.Now()

	// 启动读取goroutine（模拟多个节点的读取）
	for i := 0; i < 3; i++ { // 模拟3个读取节点
		wg.Add(1)
		go func(cacheInstance *RedisSessionCacheV3) {
			defer wg.Done()
			for j := 0; j < testSize; j++ {
				cacheInstance.IsValidSession(userID, 300, concurrentTokens[j])
			}
		}(cache2)
	}

	// 启动写入goroutine（模拟主节点的写入）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < testSize/10; i++ { // 10%的写入操作
			cache1.Remove(userID, 300, concurrentTokens[i], 3600, "")
		}
	}()

	wg.Wait()
	concurrentDuration := time.Since(start)
	t.Logf("   - 并发操作总耗时: %v", concurrentDuration)
	t.Logf("   - 平均每个操作: %v", concurrentDuration/time.Duration(testSize*4)) // 3个读节点+1个写节点

	// 5. 测试缓存一致性
	t.Logf("\n5. 缓存一致性验证:")
	inconsistencies := 0
	for i := 0; i < testSize/10; i++ { // 检查被删除的token
		if cache1.IsValidSession(userID, 300, concurrentTokens[i]) !=
			cache2.IsValidSession(userID, 300, concurrentTokens[i]) {
			inconsistencies++
		}
	}
	t.Logf("   - 不一致数量: %d", inconsistencies)
	assert.Equal(t, 0, inconsistencies, "节点间缓存应该保持一致")
}

// 修改原有的CacheVsNoCache测试，加入Pub/Sub开销测试
func TestRedisSessionCacheV3_CacheVsNoCache(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	testSize := 1000
	requestsPerToken := 10

	// 生成测试数据
	sessionTokens := make([]string, testSize)

	// 1. 测试添加token的性能（包含Pub/Sub）
	start := time.Now()
	for i := 0; i < len(sessionTokens); i++ {
		sessionTokens[i] = fmt.Sprintf("session-token-%d", i)
		cache.Add(userID, 300, sessionTokens[i], 3600, "")
	}
	addDuration := time.Since(start)

	// 2. 测试直接访问Redis（不使用缓存）
	start = time.Now()
	for i := 0; i < requestsPerToken; i++ {
		for _, token := range sessionTokens {
			assert.True(t, cache.isValidTokenNoCache(userID, 300, token, "session"))
		}
	}
	noCacheDuration := time.Since(start)

	// 3. 测试使用本地缓存
	// 第一轮：填充缓存
	for _, token := range sessionTokens {
		cache.IsValidSession(userID, 300, token)
	}

	// 第二轮：测试带缓存的性能
	start = time.Now()
	for i := 0; i < requestsPerToken; i++ {
		for _, token := range sessionTokens {
			assert.True(t, cache.IsValidSession(userID, 300, token))
		}
	}
	withCacheDuration := time.Since(start)

	// 计算性能指标
	totalRequests := testSize * requestsPerToken
	noCacheAvg := float64(noCacheDuration.Nanoseconds()) / float64(totalRequests)
	withCacheAvg := float64(withCacheDuration.Nanoseconds()) / float64(totalRequests)
	speedup := noCacheAvg / withCacheAvg
	addAvg := float64(addDuration.Nanoseconds()) / float64(testSize)

	// 输出详细的性能对比数据
	t.Logf("\n=== 缓存性能对比测试 ===")
	t.Logf("测试配置:")
	t.Logf("- Token数量: %d", testSize)
	t.Logf("- 每个Token请求次数: %d", requestsPerToken)
	t.Logf("- 总请求数: %d", totalRequests)

	t.Logf("\n1. 写入性能（带Pub/Sub）:")
	t.Logf("   - 总耗时: %v", addDuration)
	t.Logf("   - 平均每个token: %v", time.Duration(addAvg))

	t.Logf("\n2. 读取性能:")
	t.Logf("   a) 直接访问Redis:")
	t.Logf("      - 总耗时: %v", noCacheDuration)
	t.Logf("      - 平均每次请求: %v", time.Duration(noCacheAvg))
	t.Logf("   b) 使用本地缓存:")
	t.Logf("      - 总耗时: %v", withCacheDuration)
	t.Logf("      - 平均每次请求: %v", time.Duration(withCacheAvg))

	t.Logf("\n3. 性能提升:")
	t.Logf("   - 提升倍数: %.2f倍", speedup)
	t.Logf("   - 性能提升: %.1f%%", (speedup-1)*100)

	// QPS计算
	noCacheQPS := float64(totalRequests) / noCacheDuration.Seconds()
	withCacheQPS := float64(totalRequests) / withCacheDuration.Seconds()
	t.Logf("\n4. QPS对比:")
	t.Logf("   - 直接访问Redis: %.0f QPS", noCacheQPS)
	t.Logf("   - 使用本地缓存: %.0f QPS", withCacheQPS)

	// 验证性能要求
	assert.Greater(t, speedup, float64(1.2), "本地缓存应该比Redis快至少20%")
}
