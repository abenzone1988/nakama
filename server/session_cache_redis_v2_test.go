package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupTestRedisV2(t *testing.T) (*RedisSessionCacheV2, *redis.Client) {
	logger, _ := zap.NewDevelopment()
	cache := NewRedisSessionCacheV2(
		logger,
		"192.168.102.223:6379", // 确保Redis在本地运行
		"",                     // 无密码
		300,                    // token过期时间5分钟
		3600,                   // refresh token过期时间1小时
		false,                  // 非单token模式
	).(*RedisSessionCacheV2)

	return cache, cache.client
}

func TestRedisSessionCacheV2(t *testing.T) {
	logger := zap.NewExample()

	// 创建Redis会话缓存实例（非单token模式）
	cache := NewRedisSessionCacheV2(logger, "192.168.102.223:6379", "", 60, 3600, false)
	defer cache.Stop()

	// 测试Redis连接
	client := cache.(*RedisSessionCacheV2).client
	pong, err := client.Ping(context.Background()).Result()
	if err != nil {
		t.Fatalf("Redis connection failed: %v", err)
	}
	t.Logf("Redis connection test: %s", pong)

	// 生成测试用的用户ID和token
	userID := uuid.Must(uuid.NewV4())
	tokenID := uuid.Must(uuid.NewV4()).String()
	exp := time.Now().Add(time.Hour).Unix()

	// 测试添加和验证session
	t.Run("Add and validate session", func(t *testing.T) {
		// 清理之前的数据 - 使用V2版本的实际键名
		sessionKey := fmt.Sprintf("user:%s:session", userID.String())
		refreshKey := fmt.Sprintf("user:%s:refresh", userID.String())
		client.Del(context.Background(), sessionKey, refreshKey)

		t.Logf("Adding session for user %s with token %s", userID.String(), tokenID)
		cache.Add(userID, exp, tokenID, exp, tokenID)

		// 验证session token是否正确存储
		sessionValue, err := client.Get(context.Background(), sessionKey).Result()
		if err != nil {
			t.Errorf("Failed to get session token from Redis: %v", err)
		}
		t.Logf("Session token value: %s", sessionValue)
		expectedSessionValue := fmt.Sprintf("%s:valid", tokenID)
		assert.Equal(t, expectedSessionValue, sessionValue)

		// 验证refresh token是否正确存储
		refreshValue, err := client.Get(context.Background(), refreshKey).Result()
		if err != nil {
			t.Errorf("Failed to get refresh token from Redis: %v", err)
		}
		t.Logf("Refresh token value: %s", refreshValue)
		expectedRefreshValue := fmt.Sprintf("%s:valid", tokenID)
		assert.Equal(t, expectedRefreshValue, refreshValue)

		// 检查session是否有效
		valid := cache.IsValidSession(userID, exp, tokenID)
		t.Logf("Session validity check result: %v", valid)
		assert.True(t, valid, "Session should be valid")
	})

	// 测试移除session
	t.Run("Remove session", func(t *testing.T) {
		t.Logf("Removing session for user %s with token %s", userID.String(), tokenID)
		cache.Remove(userID, exp, tokenID, exp, tokenID)

		valid := cache.IsValidSession(userID, exp, tokenID)
		t.Logf("Session validity after removal: %v", valid)
		assert.False(t, valid, "Session should be invalid after removal")

		// 验证V2版本的数据是否被正确删除
		sessionKey := fmt.Sprintf("user:%s:session", userID.String())
		refreshKey := fmt.Sprintf("user:%s:refresh", userID.String())

		// 检查session token是否被删除
		_, err := client.Get(context.Background(), sessionKey).Result()
		assert.ErrorIs(t, err, redis.Nil, "Session token should be removed from Redis")

		// 检查refresh token是否被删除
		_, err = client.Get(context.Background(), refreshKey).Result()
		assert.ErrorIs(t, err, redis.Nil, "Refresh token should be removed from Redis")
	})

	// 测试Ban和Unban功能
	t.Run("Ban and unban user", func(t *testing.T) {
		// 清理之前的数据
		banKey := fmt.Sprintf("user:%s:ban", userID.String())
		client.Del(context.Background(), banKey)

		// 添加新session
		newToken := uuid.Must(uuid.NewV4()).String()
		t.Logf("Adding new session for ban test: user %s, token %s", userID.String(), newToken)
		cache.Add(userID, exp, newToken, exp, newToken)

		// 验证session添加成功
		valid := cache.IsValidSession(userID, exp, newToken)
		t.Logf("Initial session validity: %v", valid)
		assert.True(t, valid)

		// 禁用用户
		t.Log("Banning user")
		cache.Ban([]uuid.UUID{userID})

		// 验证ban状态
		banned, err := client.Get(context.Background(), banKey).Result()
		assert.NoError(t, err)
		t.Logf("Ban status: %s", banned)
		assert.Equal(t, "banned", banned)

		// 验证session在ban后无效
		valid = cache.IsValidSession(userID, exp, newToken)
		t.Logf("Session validity after ban: %v", valid)
		assert.False(t, valid)

		// 解除禁用
		t.Log("Unbanning user")
		cache.Unban([]uuid.UUID{userID})

		// 验证unban是否成功
		_, err = client.Get(context.Background(), banKey).Result()
		assert.ErrorIs(t, err, redis.Nil)

		// 添加新session
		newToken2 := uuid.Must(uuid.NewV4()).String()
		t.Logf("Adding new session after unban: user %s, token %s", userID.String(), newToken2)
		cache.Add(userID, exp, newToken2, exp, newToken2)

		// 验证新session
		valid = cache.IsValidSession(userID, exp, newToken2)
		t.Logf("Session validity after unban: %v", valid)
		assert.True(t, valid, "Session should be valid after unban")
	})
}

func TestRedisSessionCacheV2_SingleToken(t *testing.T) {
	logger := zap.NewExample()

	// 创建Redis会话缓存实例（单token模式）
	cache := NewRedisSessionCacheV2(logger, "192.168.102.223:6379", "", 60, 3600, true).(*RedisSessionCacheV2)
	client := cache.client
	defer cache.Stop()

	// 生成测试用的用户ID
	userID := uuid.Must(uuid.NewV4())
	exp := time.Now().Add(time.Hour).Unix()

	// 测试单token功能
	t.Run("Single token test", func(t *testing.T) {
		// 创建第一个会话
		token1 := uuid.Must(uuid.NewV4()).String()
		t.Logf("Adding first session with token: %s", token1)
		cache.Add(userID, exp, token1, exp, token1)

		valid := cache.IsValidSession(userID, exp, token1)
		t.Logf("First session validity: %v", valid)
		assert.True(t, valid, "First session should be valid")

		// 创建第二个会话（应该使第一个会话失效）
		token2 := uuid.Must(uuid.NewV4()).String()
		t.Logf("Adding second session with token: %s", token2)
		cache.Add(userID, exp, token2, exp, token2)

		// 验证第一个会话已失效
		valid = cache.IsValidSession(userID, exp, token1)
		t.Logf("First session validity after second login: %v", valid)
		assert.False(t, valid, "First session should be invalid after second login")

		// 验证第二个会话有效
		valid = cache.IsValidSession(userID, exp, token2)
		t.Logf("Second session validity: %v", valid)
		assert.True(t, valid, "Second session should be valid")

		// 验证V2版本Redis中的数据
		sessionKey := fmt.Sprintf("user:%s:session", userID.String())
		refreshKey := fmt.Sprintf("user:%s:refresh", userID.String())

		// 检查session token
		sessionValue, err := client.Get(context.Background(), sessionKey).Result()
		assert.NoError(t, err)
		expectedSessionValue := fmt.Sprintf("%s:valid", token2)
		assert.Equal(t, expectedSessionValue, sessionValue, "Session token should be the second token")

		// 检查refresh token
		refreshValue, err := client.Get(context.Background(), refreshKey).Result()
		assert.NoError(t, err)
		expectedRefreshValue := fmt.Sprintf("%s:valid", token2)
		assert.Equal(t, expectedRefreshValue, refreshValue, "Refresh token should be the second token")
	})
}

func TestRedisSessionCacheV2_Performance(t *testing.T) {
	cache, client := setupTestRedisV2(t)
	defer cache.Stop()
	defer client.FlushDB(cache.ctx).Err()

	userID := uuid.Must(uuid.NewV4())

	t.Log("Starting performance test...")

	// 批量添加token测试
	start := time.Now()
	for i := 0; i < testTokenCount; i++ {
		sessionToken := fmt.Sprintf("session%d", i)
		refreshToken := fmt.Sprintf("refresh%d", i)
		cache.Add(userID, time.Now().Unix()+300, sessionToken, time.Now().Unix()+3600, refreshToken)
	}
	addDuration := time.Since(start)
	t.Logf("Add %d pairs of tokens took: %v (%.2f token pairs/sec)", testTokenCount, addDuration, float64(testTokenCount)/addDuration.Seconds())

	// 批量验证token测试
	start = time.Now()
	validCount := 0
	for i := 0; i < testTokenCount; i++ {
		sessionToken := fmt.Sprintf("session%d", i)
		if cache.IsValidSession(userID, time.Now().Unix()+300, sessionToken) {
			validCount++
		}
	}
	validateDuration := time.Since(start)
	t.Logf("Validate %d session tokens took: %v (%.2f tokens/sec)", testTokenCount, validateDuration, float64(testTokenCount)/validateDuration.Seconds())
	t.Logf("Valid session tokens found: %d", validCount)

	// 验证V2版本的数据结构
	sessionKey := fmt.Sprintf("user:%s:session", userID.String())
	refreshKey := fmt.Sprintf("user:%s:refresh", userID.String())

	// 检查session token是否存在
	sessionExists, err := client.Exists(context.Background(), sessionKey).Result()
	assert.NoError(t, err)
	t.Logf("Session token exists: %d", sessionExists)
	assert.Equal(t, int64(1), sessionExists, "Session token should exist")

	// 检查refresh token是否存在
	refreshExists, err := client.Exists(context.Background(), refreshKey).Result()
	assert.NoError(t, err)
	t.Logf("Refresh token exists: %d", refreshExists)
	assert.Equal(t, int64(1), refreshExists, "Refresh token should exist")

	// 验证最后一个token的值
	lastSessionToken := fmt.Sprintf("session%d", testTokenCount-1)
	lastRefreshToken := fmt.Sprintf("refresh%d", testTokenCount-1)

	sessionValue, err := client.Get(context.Background(), sessionKey).Result()
	assert.NoError(t, err)
	expectedSessionValue := fmt.Sprintf("%s:valid", lastSessionToken)
	assert.Equal(t, expectedSessionValue, sessionValue, "Last session token should be stored")

	refreshValue, err := client.Get(context.Background(), refreshKey).Result()
	assert.NoError(t, err)
	expectedRefreshValue := fmt.Sprintf("%s:valid", lastRefreshToken)
	assert.Equal(t, expectedRefreshValue, refreshValue, "Last refresh token should be stored")

	// 性能要求
	assert.Less(t, addDuration.Seconds(), float64(5), "Adding tokens took too long")
	assert.Less(t, validateDuration.Seconds(), float64(5), "Validating tokens took too long")
}
