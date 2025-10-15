// Copyright 2024 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestSessionCacheClusterSync 测试多节点间的 session cache 同步
func TestSessionCacheClusterSync(t *testing.T) {
	// 跳过测试如果没有 Redis（可以通过环境变量控制）
	// 如果要运行此测试，需要先启动 Redis: docker run -d -p 6379:6379 redis
	// 或设置环境变量: export NAKAMA_TEST_REDIS=localhost:6379

	// 创建测试用的 logger（使用 console logger 以便看到调试信息）
	logger := NewConsoleLogger(os.Stdout, true)

	// 创建测试用的 config
	config1 := NewConfig(logger)
	config1.GetCluster().Enabled = true
	config1.GetCluster().RedisAddress = "localhost:6379" // 确保 Redis 运行在此地址
	config1.GetCluster().RedisPassword = ""
	config1.Name = "test-node-1"

	config2 := NewConfig(logger)
	config2.GetCluster().Enabled = true
	config2.GetCluster().RedisAddress = "localhost:6379"
	config2.GetCluster().RedisPassword = ""
	config2.Name = "test-node-2"

	config3 := NewConfig(logger)
	config3.GetCluster().Enabled = true
	config3.GetCluster().RedisAddress = "localhost:6379"
	config3.GetCluster().RedisPassword = ""
	config3.Name = "test-node-3"

	tokenExpiry := int64(3600)
	refreshExpiry := int64(7200)

	// 创建三个节点的 session cache，根据配置决定是否启用集群
	// 注意：所有节点使用相同的 purpose，这样它们会订阅同一个 Redis 频道
	purpose := "test" // 所有测试节点使用相同的 purpose

	t.Log("创建节点 1...")
	node1 := NewLocalSessionCache(logger, config1, tokenExpiry, refreshExpiry, purpose)
	defer node1.Stop()

	t.Log("创建节点 2...")
	node2 := NewLocalSessionCache(logger, config2, tokenExpiry, refreshExpiry, purpose)
	defer node2.Stop()

	t.Log("创建节点 3...")
	node3 := NewLocalSessionCache(logger, config3, tokenExpiry, refreshExpiry, purpose)
	defer node3.Stop()

	// 等待 Redis 连接和订阅建立（增加等待时间）
	t.Log("等待 Redis 连接建立...")
	time.Sleep(500 * time.Millisecond)
	t.Log("开始测试...")

	t.Run("TestRemoveSyncAcrossNodes", func(t *testing.T) {
		userID := uuid.Must(uuid.NewV4())
		sessionTokenID := "session_token_123"
		refreshTokenID := "refresh_token_456"
		sessionExp := time.Now().UTC().Unix() + 3600
		refreshExp := time.Now().UTC().Unix() + 7200

		// 在节点 1 上移除 session
		t.Log("节点 1 执行 Remove...")
		node1.Remove(userID, sessionExp, sessionTokenID, refreshExp, refreshTokenID)

		// 等待消息传播（增加等待时间）
		t.Log("等待消息传播...")
		time.Sleep(300 * time.Millisecond)

		// 验证节点 2 和节点 3 也看到了这个移除操作
		t.Log("验证节点 2 和节点 3...")
		assert.False(t, node2.IsValidSession(userID, sessionExp, sessionTokenID), "节点 2 应该看到 session 已失效")
		assert.False(t, node2.IsValidRefresh(userID, refreshExp, refreshTokenID), "节点 2 应该看到 refresh token 已失效")

		assert.False(t, node3.IsValidSession(userID, sessionExp, sessionTokenID), "节点 3 应该看到 session 已失效")
		assert.False(t, node3.IsValidRefresh(userID, refreshExp, refreshTokenID), "节点 3 应该看到 refresh token 已失效")
	})

	t.Run("TestRemoveAllSyncAcrossNodes", func(t *testing.T) {
		userID := uuid.Must(uuid.NewV4())
		sessionTokenID := "session_token_abc"
		refreshTokenID := "refresh_token_def"
		sessionExp := time.Now().UTC().Unix() + 3600
		refreshExp := time.Now().UTC().Unix() + 7200

		// 先在节点 2 上添加到黑名单
		t.Log("节点 2 执行 Remove...")
		node2.Remove(userID, sessionExp, sessionTokenID, refreshExp, refreshTokenID)
		time.Sleep(300 * time.Millisecond)

		// 在节点 1 上执行 RemoveAll
		t.Log("节点 1 执行 RemoveAll...")
		node1.RemoveAll(userID)
		time.Sleep(300 * time.Millisecond)

		// 验证所有节点都看到了 RemoveAll（通过 lastInvalidation 时间戳）
		// RemoveAll 会使该用户的所有在 RemoveAll 之前签发的 token 失效
		// 模拟一个在 RemoveAll 之前签发的 token（签发时间为 2 秒前）
		oldTokenIssuedAt := time.Now().UTC().Unix() - 2
		oldSessionExp := oldTokenIssuedAt + tokenExpiry
		assert.False(t, node2.IsValidSession(userID, oldSessionExp, "old_session_token"),
			"节点 2 应该看到该用户所有旧 session 已失效")
		assert.False(t, node3.IsValidSession(userID, oldSessionExp, "old_session_token"),
			"节点 3 应该看到该用户所有旧 session 已失效")

		// 验证在 RemoveAll 之后签发的新 token 应该是有效的
		// 等待一秒确保新 token 的签发时间晚于 RemoveAll
		time.Sleep(1 * time.Second)
		newTokenIssuedAt := time.Now().UTC().Unix()
		newSessionExp := newTokenIssuedAt + tokenExpiry
		assert.True(t, node2.IsValidSession(userID, newSessionExp, "new_session_token"),
			"节点 2 应该看到 RemoveAll 之后签发的新 token 是有效的")
		assert.True(t, node3.IsValidSession(userID, newSessionExp, "new_session_token"),
			"节点 3 应该看到 RemoveAll 之后签发的新 token 是有效的")
	})

	t.Run("TestBanSyncAcrossNodes", func(t *testing.T) {
		userID1 := uuid.Must(uuid.NewV4())
		userID2 := uuid.Must(uuid.NewV4())
		userIDs := []uuid.UUID{userID1, userID2}

		// token 的签发时间设置为 2 秒前，确保早于 Ban 时间
		tokenIssuedAt := time.Now().UTC().Unix() - 2
		sessionExp := tokenIssuedAt + tokenExpiry

		// 在节点 1 上封禁用户
		t.Log("节点 1 执行 Ban...")
		node1.Ban(userIDs)
		time.Sleep(300 * time.Millisecond)

		// 验证节点 2 和节点 3 也看到了封禁
		// 注意：Ban 会设置 lastInvalidation，所有在 Ban 之前签发的 token 都会失效
		assert.False(t, node2.IsValidSession(userID1, sessionExp, "any_token_1"),
			"节点 2 应该看到用户 1 被封禁")
		assert.False(t, node2.IsValidSession(userID2, sessionExp, "any_token_2"),
			"节点 2 应该看到用户 2 被封禁")

		assert.False(t, node3.IsValidSession(userID1, sessionExp, "any_token_1"),
			"节点 3 应该看到用户 1 被封禁")
		assert.False(t, node3.IsValidSession(userID2, sessionExp, "any_token_2"),
			"节点 3 应该看到用户 2 被封禁")

		// 验证在 Ban 之后签发的新 token 应该是有效的（用户可以重新登录）
		time.Sleep(1 * time.Second)
		newTokenIssuedAt := time.Now().UTC().Unix()
		newSessionExp := newTokenIssuedAt + tokenExpiry
		assert.True(t, node2.IsValidSession(userID1, newSessionExp, "new_token_after_ban"),
			"节点 2 应该看到 Ban 之后签发的新 token 是有效的")
		assert.True(t, node3.IsValidSession(userID1, newSessionExp, "new_token_after_ban"),
			"节点 3 应该看到 Ban 之后签发的新 token 是有效的")
	})

	t.Run("TestMessageDeduplication", func(t *testing.T) {
		// 测试消息去重：节点不会处理自己发布的消息
		userID := uuid.Must(uuid.NewV4())
		sessionExp := time.Now().UTC().Unix() + 3600

		// 在节点 1 上执行 Remove
		t.Log("节点 1 执行 Remove...")
		node1.Remove(userID, sessionExp, "token_123", 0, "")
		time.Sleep(300 * time.Millisecond)

		// 节点 1 应该能看到这个移除（因为是本地执行的）
		assert.False(t, node1.IsValidSession(userID, sessionExp, "token_123"))

		// 节点 2 和 3 也应该能看到（通过 pub/sub）
		assert.False(t, node2.IsValidSession(userID, sessionExp, "token_123"))
		assert.False(t, node3.IsValidSession(userID, sessionExp, "token_123"))
	})

	t.Run("TestValidSessionBeforeInvalidation", func(t *testing.T) {
		// 测试：没有被加入黑名单的 session 应该是有效的
		userID := uuid.Must(uuid.NewV4())
		sessionExp := time.Now().UTC().Unix() + 3600

		// 在任何节点上验证，都应该是有效的（因为没有黑名单记录）
		assert.True(t, node1.IsValidSession(userID, sessionExp, "valid_token"))
		assert.True(t, node2.IsValidSession(userID, sessionExp, "valid_token"))
		assert.True(t, node3.IsValidSession(userID, sessionExp, "valid_token"))
	})

	t.Run("TestCrossNodeInvalidation", func(t *testing.T) {
		// 测试：在不同节点上执行的操作应该互相同步
		userID := uuid.Must(uuid.NewV4())
		sessionExp := time.Now().UTC().Unix() + 3600

		// 节点 1 移除 token A
		t.Log("节点 1 移除 token A...")
		node1.Remove(userID, sessionExp, "token_a", 0, "")
		time.Sleep(300 * time.Millisecond)

		// 节点 2 移除 token B
		t.Log("节点 2 移除 token B...")
		node2.Remove(userID, sessionExp, "token_b", 0, "")
		time.Sleep(300 * time.Millisecond)

		// 节点 3 移除 token C
		t.Log("节点 3 移除 token C...")
		node3.Remove(userID, sessionExp, "token_c", 0, "")
		time.Sleep(300 * time.Millisecond)

		// 所有节点都应该看到所有三个 token 都失效了
		assert.False(t, node1.IsValidSession(userID, sessionExp, "token_a"))
		assert.False(t, node1.IsValidSession(userID, sessionExp, "token_b"))
		assert.False(t, node1.IsValidSession(userID, sessionExp, "token_c"))

		assert.False(t, node2.IsValidSession(userID, sessionExp, "token_a"))
		assert.False(t, node2.IsValidSession(userID, sessionExp, "token_b"))
		assert.False(t, node2.IsValidSession(userID, sessionExp, "token_c"))

		assert.False(t, node3.IsValidSession(userID, sessionExp, "token_a"))
		assert.False(t, node3.IsValidSession(userID, sessionExp, "token_b"))
		assert.False(t, node3.IsValidSession(userID, sessionExp, "token_c"))
	})
}

// TestSessionCacheClusterFallback 测试 Redis 连接失败时的降级行为
func TestSessionCacheClusterFallback(t *testing.T) {
	// 使用一个无效的 Redis 地址
	config := NewConfig(zap.NewNop())
	config.GetCluster().Enabled = true
	config.GetCluster().RedisAddress = "localhost:9999" // 不存在的端口
	config.GetCluster().RedisPassword = ""
	config.Name = "test-node-fallback"

	logger := zap.NewNop()
	tokenExpiry := int64(3600)
	refreshExpiry := int64(7200)

	// 创建 session cache，应该降级为单节点模式
	cache := NewLocalSessionCache(logger, config, tokenExpiry, refreshExpiry, "test-fallback")
	defer cache.Stop()

	// 验证基本功能仍然工作（单节点模式）
	userID := uuid.Must(uuid.NewV4())
	sessionExp := time.Now().UTC().Unix() + 3600

	// 应该是有效的（没有黑名单记录）
	assert.True(t, cache.IsValidSession(userID, sessionExp, "token_1"))

	// 移除后应该失效
	cache.Remove(userID, sessionExp, "token_1", 0, "")
	assert.False(t, cache.IsValidSession(userID, sessionExp, "token_1"))
}

// TestSessionCacheNoCluster 测试未启用集群模式时的行为
func TestSessionCacheNoCluster(t *testing.T) {
	config := NewConfig(zap.NewNop())
	config.GetCluster().Enabled = false // 未启用集群
	config.Name = "test-node-standalone"

	logger := zap.NewNop()
	tokenExpiry := int64(3600)
	refreshExpiry := int64(7200)

	cache := NewLocalSessionCache(logger, config, tokenExpiry, refreshExpiry, "test-standalone")
	defer cache.Stop()

	userID := uuid.Must(uuid.NewV4())
	sessionExp := time.Now().UTC().Unix() + 3600

	// 验证基本功能
	assert.True(t, cache.IsValidSession(userID, sessionExp, "token_1"))

	cache.Remove(userID, sessionExp, "token_1", 0, "")
	assert.False(t, cache.IsValidSession(userID, sessionExp, "token_1"))

	cache.RemoveAll(userID)
	assert.False(t, cache.IsValidSession(userID, sessionExp, "token_2"))
}

// BenchmarkSessionCacheValidation 性能测试：验证本地缓存的性能优势
func BenchmarkSessionCacheValidation(b *testing.B) {
	config := NewConfig(zap.NewNop())
	config.GetCluster().Enabled = false
	config.Name = "benchmark-node"

	logger := zap.NewNop()
	cache := NewLocalSessionCache(logger, config, 3600, 7200, "benchmark")
	defer cache.Stop()

	userID := uuid.Must(uuid.NewV4())
	sessionExp := time.Now().UTC().Unix() + 3600

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 模拟 session 验证（最常见的操作）
			cache.IsValidSession(userID, sessionExp, "test_token")
		}
	})
}

// BenchmarkSessionCacheRemove 性能测试：测试 Remove 操作的性能
func BenchmarkSessionCacheRemove(b *testing.B) {
	config := NewConfig(zap.NewNop())
	config.GetCluster().Enabled = false
	config.Name = "benchmark-node"

	logger := zap.NewNop()
	cache := NewLocalSessionCache(logger, config, 3600, 7200, "benchmark")
	defer cache.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userID := uuid.Must(uuid.NewV4())
		sessionExp := time.Now().UTC().Unix() + 3600
		cache.Remove(userID, sessionExp, "token_"+string(rune(i)), 0, "")
	}
}
