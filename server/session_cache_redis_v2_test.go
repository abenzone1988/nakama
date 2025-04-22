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
		// 清理之前的数据
		tokenSetKey := fmt.Sprintf("user:%s:tokens", userID.String())
		tokenHashKey := fmt.Sprintf("user:%s:token_details", userID.String())
		client.Del(context.Background(), tokenSetKey, tokenHashKey)

		t.Logf("Adding session for user %s with token %s", userID.String(), tokenID)
		cache.Add(userID, exp, tokenID, exp, tokenID)

		// 验证token是否正确添加到Set中
		exists, err := client.SIsMember(context.Background(), tokenSetKey, tokenID).Result()
		if err != nil {
			t.Errorf("Failed to check token in Redis Set: %v", err)
		}
		t.Logf("Token exists in Set: %v", exists)
		assert.True(t, exists)

		// 验证token状态是否正确添加到Hash中
		sessionField := fmt.Sprintf("%s:session", tokenID)
		val, err := client.HGet(context.Background(), tokenHashKey, sessionField).Result()
		if err != nil {
			t.Errorf("Failed to get token status from Redis Hash: %v", err)
		}
		t.Logf("Token status in Hash: %s", val)
		assert.Equal(t, "valid", val)

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

		// 验证Set和Hash中的数据是否被正确删除
		tokenSetKey := fmt.Sprintf("user:%s:tokens", userID.String())
		exists, err := client.SIsMember(context.Background(), tokenSetKey, tokenID).Result()
		assert.NoError(t, err)
		assert.False(t, exists, "Token should be removed from Set")

		tokenHashKey := fmt.Sprintf("user:%s:token_details", userID.String())
		sessionField := fmt.Sprintf("%s:session", tokenID)
		val, err := client.HGet(context.Background(), tokenHashKey, sessionField).Result()
		assert.Error(t, redis.Nil, err)
		assert.Empty(t, val, "Token status should be removed from Hash")
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

		// 验证Redis中的数据
		tokenSetKey := fmt.Sprintf("user:%s:tokens", userID.String())
		members, err := client.SMembers(context.Background(), tokenSetKey).Result()
		assert.NoError(t, err)
		assert.Len(t, members, 1, "Should only have one token in Set")
		assert.Contains(t, members, token2, "Set should contain the second token")

		tokenHashKey := fmt.Sprintf("user:%s:token_details", userID.String())
		values, err := client.HGetAll(context.Background(), tokenHashKey).Result()
		assert.NoError(t, err)
		assert.Len(t, values, 2, "Should have two fields in Hash (session and refresh)")
		assert.Equal(t, "valid", values[fmt.Sprintf("%s:session", token2)])
		assert.Equal(t, "valid", values[fmt.Sprintf("%s:refresh", token2)])
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

	// 验证数据结构
	tokenSetKey := fmt.Sprintf("user:%s:tokens", userID.String())
	tokenCount, err := client.SCard(context.Background(), tokenSetKey).Result()
	assert.NoError(t, err)
	t.Logf("Total tokens in Set: %d (including both session and refresh tokens)", tokenCount)
	assert.Equal(t, int64(testTokenCount*2), tokenCount, "Should have %d tokens in Set (%d session + %d refresh)", testTokenCount*2, testTokenCount, testTokenCount)

	tokenHashKey := fmt.Sprintf("user:%s:token_details", userID.String())
	hashSize, err := client.HLen(context.Background(), tokenHashKey).Result()
	assert.NoError(t, err)
	t.Logf("Fields in Hash: %d", hashSize)
	assert.Equal(t, int64(testTokenCount*2), hashSize, "Should have %d fields in Hash (%d session + %d refresh)", testTokenCount*2, testTokenCount, testTokenCount)

	// 性能要求
	assert.Less(t, addDuration.Seconds(), float64(5), "Adding tokens took too long")
	assert.Less(t, validateDuration.Seconds(), float64(5), "Validating tokens took too long")
}
