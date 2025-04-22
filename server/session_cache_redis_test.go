package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/gofrs/uuid/v5"
	"go.uber.org/zap"
)

func TestRedisSessionCache(t *testing.T) {
	logger := zap.NewExample()

	// 创建Redis会话缓存实例（非单token模式）
	cache := NewRedisSessionCache(logger, "192.168.102.223:6379", "", 60, 3600, false)
	defer cache.Stop()

	// 测试Redis连接
	client := cache.(*RedisSessionCache).client
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
		tokenKey := "session:token:" + userID.String() + ":" + tokenID
		client.Del(context.Background(), tokenKey)

		t.Logf("Adding session for user %s with token %s", userID.String(), tokenID)
		cache.Add(userID, exp, tokenID, exp, tokenID)

		// 验证token是否正确添加
		val, err := client.Get(context.Background(), tokenKey).Result()
		if err != nil {
			t.Errorf("Failed to get token from Redis: %v", err)
		} else {
			t.Logf("Token value in Redis: %s", val)
		}

		// 检查session是否有效
		valid := cache.IsValidSession(userID, exp, tokenID)
		t.Logf("Session validity check result: %v", valid)
		if !valid {
			t.Error("Session should be valid")
		}
	})

	// 测试移除session
	t.Run("Remove session", func(t *testing.T) {
		cache.Remove(userID, exp, tokenID, exp, tokenID)
		if cache.IsValidSession(userID, exp, tokenID) {
			t.Error("Session should be invalid after removal")
		}
	})

	// 测试Ban和Unban功能
	t.Run("Ban and unban user", func(t *testing.T) {
		// 清理之前的数据
		banKey := "user:ban:" + userID.String()
		client.Del(context.Background(), banKey)

		// 添加新session
		newToken := uuid.Must(uuid.NewV4()).String()
		tokenKey := "session:token:" + userID.String() + ":" + newToken

		t.Logf("Adding new session for ban test: user %s, token %s", userID.String(), newToken)
		cache.Add(userID, exp, newToken, exp, newToken)

		// 验证session添加成功
		val, err := client.Get(context.Background(), tokenKey).Result()
		if err != nil {
			t.Errorf("Failed to get token from Redis: %v", err)
		}
		t.Logf("Initial token value: %s", val)

		// 禁用用户
		t.Log("Banning user")
		cache.Ban([]uuid.UUID{userID})

		// 验证ban状态
		banned, err := client.Get(context.Background(), banKey).Result()
		if err != nil {
			t.Errorf("Failed to get ban status: %v", err)
		}
		t.Logf("Ban status: %s", banned)

		// 解除禁用
		t.Log("Unbanning user")
		cache.Unban([]uuid.UUID{userID})

		// 验证unban是否成功
		banned, err = client.Get(context.Background(), banKey).Result()
		if err != redis.Nil {
			t.Logf("Ban key after unban - error: %v, value: %s", err, banned)
		}

		// 添加新session
		newToken2 := uuid.Must(uuid.NewV4()).String()
		tokenKey2 := "session:token:" + userID.String() + ":" + newToken2

		t.Logf("Adding new session after unban: user %s, token %s", userID.String(), newToken2)
		cache.Add(userID, exp, newToken2, exp, newToken2)

		// 验证新session
		val, err = client.Get(context.Background(), tokenKey2).Result()
		if err != nil {
			t.Errorf("Failed to get new token from Redis: %v", err)
		}
		t.Logf("New token value after unban: %s", val)

		// 检查session有效性
		valid := cache.IsValidSession(userID, exp, newToken2)
		t.Logf("Session validity after unban: %v", valid)
		if !valid {
			t.Error("Session should be valid after unban")
		}
	})
}

func TestSingleTokenMode(t *testing.T) {
	logger := zap.NewExample()

	// 创建Redis会话缓存实例（单token模式）
	cache := NewRedisSessionCache(logger, "192.168.102.223:6379", "", 60, 3600, true)
	defer cache.Stop()

	// 生成测试用的用户ID
	userID := uuid.Must(uuid.NewV4())
	exp := time.Now().Add(time.Hour).Unix()

	// 测试单token功能
	t.Run("Single token test", func(t *testing.T) {
		// 创建第一个会话
		token1 := uuid.Must(uuid.NewV4()).String()
		cache.Add(userID, exp, token1, exp, token1)
		if !cache.IsValidSession(userID, exp, token1) {
			t.Error("First session should be valid")
		}

		// 创建第二个会话（应该使第一个会话失效）
		token2 := uuid.Must(uuid.NewV4()).String()
		cache.Add(userID, exp, token2, exp, token2)

		// 验证第一个会话已失效
		if cache.IsValidSession(userID, exp, token1) {
			t.Error("First session should be invalid after second login")
		}

		// 验证第二个会话有效
		if !cache.IsValidSession(userID, exp, token2) {
			t.Error("Second session should be valid")
		}
	})
}

func TestRedisSessionCache_Performance(t *testing.T) {
	logger := zap.NewExample()
	cache := NewRedisSessionCache(logger, "192.168.102.223:6379", "", 60, 3600, false)
	defer cache.Stop()

	client := cache.(*RedisSessionCache).client
	defer client.FlushDB(context.Background()).Err()

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
	// 计算所有相关的token数量
	pattern := fmt.Sprintf("session:token:%s:*", userID.String())
	keys, err := client.Keys(context.Background(), pattern).Result()
	if err != nil {
		t.Errorf("Failed to get keys: %v", err)
	}
	t.Logf("Total session tokens found: %d", len(keys))

	pattern = fmt.Sprintf("session:refresh:%s:*", userID.String())
	refreshKeys, err := client.Keys(context.Background(), pattern).Result()
	if err != nil {
		t.Errorf("Failed to get refresh keys: %v", err)
	}
	t.Logf("Total refresh tokens found: %d", len(refreshKeys))

	totalKeys := len(keys) + len(refreshKeys)
	t.Logf("Total tokens: %d (including both session and refresh tokens)", totalKeys)
	expectedTotal := testTokenCount * 2
	if totalKeys != expectedTotal {
		t.Errorf("Expected %d total tokens, got %d", expectedTotal, totalKeys)
	}

	// 验证token状态
	validTokens := 0
	for _, key := range keys {
		val, err := client.Get(context.Background(), key).Result()
		if err != nil {
			t.Errorf("Failed to get token value: %v", err)
			continue
		}
		if val == "valid" {
			validTokens++
		}
	}
	t.Logf("Valid session tokens in Redis: %d", validTokens)

	// 性能要求
	if addDuration.Seconds() > 5 {
		t.Errorf("Adding tokens took too long: %v", addDuration)
	}
	if validateDuration.Seconds() > 5 {
		t.Errorf("Validating tokens took too long: %v", validateDuration)
	}
}
