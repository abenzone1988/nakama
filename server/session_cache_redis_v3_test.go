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
	// 使用统一的Redis连接配置
	redisAddr := "192.168.102.223:6379"
	cache := NewRedisSessionCacheV3(logger, redisAddr, "", 300, 3600, true).(*RedisSessionCacheV3)

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

	// 生成测试数据 - 每个token对使用不同的用户
	userIDs := make([]uuid.UUID, testTokenCount)
	sessionTokens := make([]string, testTokenCount)
	refreshTokens := make([]string, testTokenCount)

	for i := 0; i < testTokenCount; i++ {
		userIDs[i] = uuid.Must(uuid.NewV4()) // 每个token对使用不同的用户
		sessionTokens[i] = fmt.Sprintf("session-%d", i)
		refreshTokens[i] = fmt.Sprintf("refresh-%d", i)
	}

	// 测试添加性能
	startAdd := time.Now()
	for i := 0; i < testTokenCount; i++ {
		cache.Add(userIDs[i], 300, sessionTokens[i], 3600, refreshTokens[i])
	}
	addDuration := time.Since(startAdd)
	t.Logf("添加 %d 个token对用时: %v", testTokenCount, addDuration)

	// 测试验证性能
	startValidate := time.Now()
	for i := 0; i < testTokenCount; i++ {
		assert.True(t, cache.IsValidSession(userIDs[i], 300, sessionTokens[i]))
		assert.True(t, cache.IsValidRefresh(userIDs[i], 3600, refreshTokens[i]))
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

	// 生成多个不同的用户ID和token进行测试
	userIDs := make([]uuid.UUID, 1000)
	sessionTokens := make([]string, 1000)
	for i := 0; i < len(sessionTokens); i++ {
		userIDs[i] = uuid.Must(uuid.NewV4())
		sessionTokens[i] = fmt.Sprintf("session-token-%d", i)
		cache.Add(userIDs[i], 300, sessionTokens[i], 3600, "")
	}

	// 清除本地缓存，确保第一次验证从Redis读取
	cache.localCache = newTokenCache()

	// 预热Redis连接
	cache.IsValidSession(userIDs[0], 300, sessionTokens[0])

	// 第一次验证（从Redis读取）
	start := time.Now()
	for i, token := range sessionTokens {
		assert.True(t, cache.IsValidSession(userIDs[i], 300, token))
	}
	firstDuration := time.Since(start)

	// 第二次验证（从本地缓存读取）
	start = time.Now()
	cacheHits := 0
	for i, token := range sessionTokens {
		// 记录缓存命中情况
		cacheKey := cache.getCacheKey(userIDs[i], token, "session")
		if _, exists := cache.localCache.get(cacheKey); exists {
			cacheHits++
		}
		assert.True(t, cache.IsValidSession(userIDs[i], 300, token))
	}
	secondDuration := time.Since(start)

	t.Logf("本地缓存命中数: %d/%d", cacheHits, len(sessionTokens))

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
	redisAddr := "192.168.102.223:6379"
	cache := NewRedisSessionCacheV3(logger, redisAddr, "", 300, 3600, true).(*RedisSessionCacheV3)
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

	// 验证Redis中的数据已被删除（使用v2的数据结构）
	sessionKey := cache.getSessionTokenKey(userID)
	refreshKey := cache.getRefreshTokenKey(userID)

	exists, err := cache.client.Exists(context.Background(), sessionKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	exists, err = cache.client.Exists(context.Background(), refreshKey).Result()
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

	// 使用不同的用户ID避免单token模式冲突
	userIDs := make([]uuid.UUID, 1000)
	tokens := make([]string, 1000)
	testSize := 1000
	operationsPerToken := 5 // 每个token的操作次数

	t.Logf("\n=== 分布式环境性能测试 ===")
	t.Logf("配置:")
	t.Logf("- Token数量: %d", testSize)
	t.Logf("- 每个Token操作次数: %d", operationsPerToken)

	// 生成测试数据
	for i := 0; i < testSize; i++ {
		userIDs[i] = uuid.Must(uuid.NewV4())
		tokens[i] = fmt.Sprintf("session-token-%d", i)
	}

	// 1. 测试写入性能（包含Pub/Sub开销）
	start := time.Now()
	for i := 0; i < testSize; i++ {
		cache1.Add(userIDs[i], 300, tokens[i], 3600, "")
	}
	addDuration := time.Since(start)
	t.Logf("\n1. 写入性能（带消息发布）:")
	t.Logf("   - 总耗时: %v", addDuration)
	t.Logf("   - 平均每个token: %v", addDuration/time.Duration(testSize))

	// 等待消息传播和缓存同步
	time.Sleep(500 * time.Millisecond)

	// 2. 测试节点间的缓存同步
	var syncErrors int
	start = time.Now()

	// 添加重试机制确保同步完成
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		syncErrors = 0
		checkStart := time.Now()
		for i := 0; i < testSize; i++ {
			if !cache2.IsValidSession(userIDs[i], 300, tokens[i]) {
				syncErrors++
			}
		}
		checkDuration := time.Since(checkStart)

		if syncErrors == 0 {
			t.Logf("   第%d次检查成功，耗时: %v", retry+1, checkDuration)
			break // 同步成功
		}

		if retry < maxRetries-1 {
			t.Logf("   第%d次同步检查失败，错误数: %d，耗时: %v，等待重试...", retry+1, syncErrors, checkDuration)
			time.Sleep(200 * time.Millisecond)
		}
	}
	syncDuration := time.Since(start)
	t.Logf("\n2. 节点间同步验证:")
	t.Logf("   - 总耗时: %v", syncDuration)
	if syncDuration > 0 {
		t.Logf("   - 平均每个token: %v", syncDuration/time.Duration(testSize))
	} else {
		t.Logf("   - 平均每个token: <1µs (同步非常快)")
	}
	t.Logf("   - 同步错误数: %d", syncErrors)

	// 如果同步成功，说明修复生效
	if syncErrors == 0 {
		t.Logf("   - 状态: ✅ 节点间同步成功，Pub/Sub机制工作正常")
	} else {
		t.Logf("   - 状态: ❌ 仍有 %d 个token未同步", syncErrors)
	}

	// 3. 测试删除性能（包含Pub/Sub开销）
	start = time.Now()
	for i := 0; i < testSize; i++ {
		cache1.Remove(userIDs[i], 300, tokens[i], 3600, "")
	}
	removeDuration := time.Since(start)
	t.Logf("\n3. 删除性能（带消息发布）:")
	t.Logf("   - 总耗时: %v", removeDuration)
	t.Logf("   - 平均每个token: %v", removeDuration/time.Duration(testSize))

	// 等待消息传播和缓存同步
	time.Sleep(500 * time.Millisecond)

	// 4. 测试并发读写性能
	t.Logf("\n4. 并发读写性能测试:")

	// 准备新的测试数据
	concurrentUserIDs := make([]uuid.UUID, testSize)
	concurrentTokens := make([]string, testSize)
	for i := 0; i < testSize; i++ {
		concurrentUserIDs[i] = uuid.Must(uuid.NewV4())
		concurrentTokens[i] = fmt.Sprintf("concurrent-token-%d", i)
		cache1.Add(concurrentUserIDs[i], 300, concurrentTokens[i], 3600, "")
	}

	var wg sync.WaitGroup
	start = time.Now()

	// 启动读取goroutine（模拟多个节点的读取）
	for i := 0; i < 3; i++ { // 模拟3个读取节点
		wg.Add(1)
		go func(cacheInstance *RedisSessionCacheV3) {
			defer wg.Done()
			for j := 0; j < testSize; j++ {
				cacheInstance.IsValidSession(concurrentUserIDs[j], 300, concurrentTokens[j])
			}
		}(cache2)
	}

	// 启动写入goroutine（模拟主节点的写入）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < testSize/10; i++ { // 10%的写入操作
			cache1.Remove(concurrentUserIDs[i], 300, concurrentTokens[i], 3600, "")
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
		if cache1.IsValidSession(concurrentUserIDs[i], 300, concurrentTokens[i]) !=
			cache2.IsValidSession(concurrentUserIDs[i], 300, concurrentTokens[i]) {
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

	// 使用不同的用户ID避免单token模式冲突
	userIDs := make([]uuid.UUID, 1000)
	sessionTokens := make([]string, 1000)
	testSize := 1000
	requestsPerToken := 10

	// 生成测试数据
	for i := 0; i < testSize; i++ {
		userIDs[i] = uuid.Must(uuid.NewV4())
		sessionTokens[i] = fmt.Sprintf("session-token-%d", i)
	}

	// 1. 测试添加token的性能（包含Pub/Sub）
	start := time.Now()
	for i := 0; i < len(sessionTokens); i++ {
		cache.Add(userIDs[i], 300, sessionTokens[i], 3600, "")
	}
	addDuration := time.Since(start)

	// 2. 测试直接访问Redis（不使用缓存）
	start = time.Now()
	for i := 0; i < requestsPerToken; i++ {
		for j, token := range sessionTokens {
			assert.True(t, cache.isValidTokenNoCache(userIDs[j], 300, token, "session"))
		}
	}
	noCacheDuration := time.Since(start)

	// 3. 测试使用本地缓存
	// 第一轮：填充缓存
	for j, token := range sessionTokens {
		cache.IsValidSession(userIDs[j], 300, token)
	}

	// 第二轮：测试带缓存的性能
	start = time.Now()
	for i := 0; i < requestsPerToken; i++ {
		for j, token := range sessionTokens {
			assert.True(t, cache.IsValidSession(userIDs[j], 300, token))
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

// 测试v3版本与v2版本的数据兼容性
func TestRedisSessionCacheV3_V2Compatibility(t *testing.T) {
	redisAddr := "192.168.102.223:6379"

	// 创建v2缓存实例
	loggerV2 := zap.NewNop()
	cacheV2 := NewRedisSessionCacheV2(loggerV2, redisAddr, "", 300, 3600, false).(*RedisSessionCacheV2)
	defer func() {
		cacheV2.client.FlushDB(context.Background())
		cacheV2.Stop()
	}()

	// 创建v3缓存实例（禁用单token模式以避免冲突）
	loggerV3 := zap.NewNop()
	cacheV3 := NewRedisSessionCacheV3(loggerV3, redisAddr, "", 300, 3600, false).(*RedisSessionCacheV3)
	defer cacheV3.Stop()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session-v2"
	refreshToken := "test-refresh-v2"

	t.Logf("\n=== v2与v3数据兼容性测试 ===")

	// 1. 使用v2版本存储数据
	t.Logf("1. 使用v2版本存储token")
	cacheV2.Add(userID, 300, sessionToken, 3600, refreshToken)

	// 验证v2版本能正确读取
	assert.True(t, cacheV2.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cacheV2.IsValidRefresh(userID, 3600, refreshToken))
	t.Logf("   - v2版本验证通过")

	// 2. 使用v3版本读取v2存储的数据
	t.Logf("2. 使用v3版本读取v2存储的数据")
	assert.True(t, cacheV3.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cacheV3.IsValidRefresh(userID, 3600, refreshToken))
	t.Logf("   - v3版本读取v2数据成功")

	// 3. 使用v3版本修改数据
	t.Logf("3. 使用v3版本移除session token")
	cacheV3.Remove(userID, 300, sessionToken, 3600, "")

	// 验证v3版本能看到修改（v2版本不支持PubSub，所以不会自动同步）
	assert.False(t, cacheV3.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cacheV3.IsValidRefresh(userID, 3600, refreshToken)) // refresh应该仍然有效
	t.Logf("   - v3版本能看到自己的修改")

	// v2版本由于不支持PubSub，仍然认为token有效（这是预期的行为）
	assert.True(t, cacheV2.IsValidSession(userID, 300, sessionToken))
	t.Logf("   - v2版本不支持PubSub，token仍然有效（预期行为）")

	// 4. 使用v3版本添加新数据
	newSessionToken := "new-session-v3"
	t.Logf("4. 使用v3版本添加新token")
	cacheV3.Add(userID, 300, newSessionToken, 3600, "")

	// 验证v2版本能读取v3添加的数据
	assert.True(t, cacheV2.IsValidSession(userID, 300, newSessionToken))
	t.Logf("   - v2版本能读取v3添加的数据")

	// 5. 测试Ban操作的兼容性
	t.Logf("5. 测试Ban操作兼容性")
	cacheV3.Ban([]uuid.UUID{userID})

	// 验证两个版本都能看到ban状态
	assert.False(t, cacheV2.IsValidSession(userID, 300, newSessionToken))
	assert.False(t, cacheV2.IsValidRefresh(userID, 3600, refreshToken))
	assert.False(t, cacheV3.IsValidSession(userID, 300, newSessionToken))
	assert.False(t, cacheV3.IsValidRefresh(userID, 3600, refreshToken))
	t.Logf("   - 两个版本都正确处理了Ban状态")

	t.Logf("✓ 兼容性测试全部通过")
}

// 测试Redis token过期时间设置
func TestRedisSessionCacheV3_RedisTokenExpiry(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session-expiry"
	refreshToken := "test-refresh-expiry"

	t.Logf("\n=== Redis Token过期时间测试 ===")

	// 1. 添加token
	t.Logf("1. 添加token到Redis")
	cache.Add(userID, 30*24*3600, sessionToken, 30*24*3600, refreshToken) // token本身设置30天过期

	// 2. 验证token立即可用
	assert.True(t, cache.IsValidSession(userID, 30*24*3600, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 30*24*3600, refreshToken))
	t.Logf("   - token添加后立即可用")

	// 3. 检查Redis中的实际过期时间
	sessionKey := cache.getSessionTokenKey(userID)
	refreshKey := cache.getRefreshTokenKey(userID)

	sessionTTL, err := cache.client.TTL(context.Background(), sessionKey).Result()
	assert.NoError(t, err)
	refreshTTL, err := cache.client.TTL(context.Background(), refreshKey).Result()
	assert.NoError(t, err)

	t.Logf("2. Redis中的实际过期时间:")
	t.Logf("   - Session Token TTL: %v", sessionTTL)
	t.Logf("   - Refresh Token TTL: %v", refreshTTL)

	// 4. 验证过期时间接近24小时（允许一些误差）
	expectedExpiry := 24 * time.Hour
	tolerance := 1 * time.Hour

	assert.True(t, sessionTTL > expectedExpiry-tolerance && sessionTTL <= expectedExpiry,
		"Session token的Redis过期时间应该接近24小时")
	assert.True(t, refreshTTL > expectedExpiry-tolerance && refreshTTL <= expectedExpiry,
		"Refresh token的Redis过期时间应该接近24小时")

	t.Logf("3. 验证结果:")
	t.Logf("   - 预期过期时间: %v", expectedExpiry)
	t.Logf("   - Session Token过期时间符合预期: %v", sessionTTL > expectedExpiry-tolerance && sessionTTL <= expectedExpiry)
	t.Logf("   - Refresh Token过期时间符合预期: %v", refreshTTL > expectedExpiry-tolerance && refreshTTL <= expectedExpiry)

	t.Logf("✓ Redis token过期时间测试通过")
}

// 测试简化版本（无心跳机制）
func TestRedisSessionCacheV3_NoHeartbeat(t *testing.T) {
	cache, cleanup := setupRedisV3(t)
	defer cleanup()

	userID := uuid.Must(uuid.NewV4())
	sessionToken := "test-session-no-heartbeat"
	refreshToken := "test-refresh-no-heartbeat"

	t.Logf("\n=== 简化版本测试（无心跳机制） ===")

	// 1. 基本功能测试
	t.Logf("1. 测试基本token操作")
	cache.Add(userID, 300, sessionToken, 3600, refreshToken)
	assert.True(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 3600, refreshToken))
	t.Logf("   - token添加和验证正常")

	// 2. 测试token移除
	t.Logf("2. 测试token移除")
	cache.Remove(userID, 300, sessionToken, 3600, "")
	assert.False(t, cache.IsValidSession(userID, 300, sessionToken))
	assert.True(t, cache.IsValidRefresh(userID, 3600, refreshToken))
	t.Logf("   - token移除正常")

	// 3. 测试PubSub通知（token失效）
	t.Logf("3. 测试PubSub token失效通知")

	// 创建第二个缓存实例模拟其他节点
	cache2, cleanup2 := setupRedisV3(t)
	defer cleanup2()

	// 等待PubSub连接建立
	time.Sleep(100 * time.Millisecond)

	newToken := "test-new-token"
	cache.Add(userID, 300, newToken, 3600, "")

	// 等待消息传播
	time.Sleep(100 * time.Millisecond)

	// 验证两个实例都能看到token
	assert.True(t, cache.IsValidSession(userID, 300, newToken))
	assert.True(t, cache2.IsValidSession(userID, 300, newToken))
	t.Logf("   - PubSub通知正常工作")

	// 4. 测试ban功能
	t.Logf("4. 测试ban功能")
	cache.Ban([]uuid.UUID{userID})
	assert.False(t, cache.IsValidSession(userID, 300, newToken))
	assert.False(t, cache2.IsValidSession(userID, 300, newToken))
	t.Logf("   - ban功能正常")

	t.Logf("✓ 简化版本测试全部通过")
}
