package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	defaultBatchSize = 100             // 默认批处理大小
	cacheExpiry      = 5 * time.Minute // 本地缓存过期时间
	cleanupInterval  = 1 * time.Minute // 清理间隔

	// Redis token过期时间 - 使用较短的时间来自动清理不活跃用户的token
	redisTokenExpiry = 24 * time.Hour // Redis中token的过期时间，1小时后自动清理

	// Redis PubSub channels
	tokenInvalidateChannel = "token:invalidate" // token失效通知channel
	userBanChannel         = "user:ban"         // 用户封禁通知channel

	// Redis连接检查间隔
	recoveryCheckInterval = 60 * time.Second // 恢复检查间隔
)

// CacheInvalidateMessage 缓存失效消息
type CacheInvalidateMessage struct {
	UserID    string   `json:"user_id"`
	TokenIDs  []string `json:"token_ids"`
	TokenType string   `json:"token_type"`
	Action    string   `json:"action"`    // invalidate, ban, unban
	SourceID  string   `json:"source_id"` // 发送消息的节点ID
}

// NodeHeartbeatMessage 已移除心跳机制，此结构体保留用于兼容性
// type NodeHeartbeatMessage struct {
//	NodeID    string    `json:"node_id"`
//	Timestamp time.Time `json:"timestamp"`
//	Status    string    `json:"status"` // online, offline, recovering
// }

// tokenCacheEntry 表示缓存中的token条目
type tokenCacheEntry struct {
	valid      bool
	expireTime time.Time
}

// tokenCache 实现了一个简单的本地缓存
type tokenCache struct {
	cache sync.Map
}

func newTokenCache() *tokenCache {
	tc := &tokenCache{}
	go tc.startCleanup()
	return tc
}

func (tc *tokenCache) set(key string, valid bool, expiry time.Duration) {
	tc.cache.Store(key, tokenCacheEntry{
		valid:      valid,
		expireTime: time.Now().Add(expiry),
	})
}

func (tc *tokenCache) get(key string) (bool, bool) {
	if value, exists := tc.cache.Load(key); exists {
		entry := value.(tokenCacheEntry)
		if time.Now().Before(entry.expireTime) {
			return entry.valid, true
		}
		// 过期了，需要删除
		tc.cache.Delete(key)
	}
	return false, false
}

func (tc *tokenCache) startCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	for range ticker.C {
		tc.cleanup()
	}
}

func (tc *tokenCache) cleanup() {
	now := time.Now()
	tc.cache.Range(func(key, value interface{}) bool {
		entry := value.(tokenCacheEntry)
		if now.After(entry.expireTime) {
			tc.cache.Delete(key)
		}
		return true
	})
}

type RedisSessionCacheV3 struct {
	client           *redis.Client
	tokenExpirySec   int64
	refreshExpirySec int64
	ctx              context.Context
	ctxCancelFn      context.CancelFunc
	logger           *zap.Logger
	singleToken      bool
	localCache       *tokenCache
	batchSize        int
	pubsub           *redis.PubSub // Redis订阅连接
	nodeID           string        // 节点唯一标识（用于PubSub消息去重）
}

// NewRedisSessionCacheV3 创建Redis会话缓存v3版本
// 注意：tokenExpirySec和refreshExpirySec参数仍然保留用于兼容性，但Redis中的实际过期时间
// 使用redisTokenExpiry常量（1小时），这样可以自动清理不活跃用户的token数据，
// 当玩家重新登录时会重新写入Redis
func NewRedisSessionCacheV3(logger *zap.Logger, address string, password string, tokenExpirySec, refreshExpirySec int64, singleToken bool) SessionCache {
	ctx, ctxCancelFn := context.WithCancel(context.Background())

	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       0,
	})

	// 生成唯一的节点ID
	nodeID := uuid.Must(uuid.NewV4()).String()

	cache := &RedisSessionCacheV3{
		client:           client,
		tokenExpirySec:   tokenExpirySec,
		refreshExpirySec: refreshExpirySec,
		ctx:              ctx,
		ctxCancelFn:      ctxCancelFn,
		logger:           logger,
		singleToken:      singleToken,
		localCache:       newTokenCache(),
		batchSize:        defaultBatchSize,
		nodeID:           nodeID,
	}

	// 初始化订阅机制（仅用于token失效通知）
	cache.initPubSub()

	// Redis高可用环境下不需要心跳机制，依赖Redis自身的故障检测
	// cache.initNodeRecovery() // 已禁用心跳机制

	// Redis高可用环境下不需要定期健康检查，依赖Redis客户端的自动重连机制
	// go cache.startSimpleRecoveryCheck() // 已禁用

	return cache
}

func (s *RedisSessionCacheV3) initPubSub() {
	// 创建订阅连接
	s.pubsub = s.client.Subscribe(s.ctx, tokenInvalidateChannel, userBanChannel)

	// 启动消息处理goroutine
	go s.handlePubSubMessages()
}

func (s *RedisSessionCacheV3) handlePubSubMessages() {
	s.logger.Info("开始监听Redis PubSub消息", zap.String("nodeId", s.nodeID))

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("停止监听Redis PubSub消息", zap.String("nodeId", s.nodeID))
			return
		default:
			msg, err := s.pubsub.ReceiveMessage(s.ctx)
			if err != nil {
				s.logger.Error("接收PubSub消息失败", zap.Error(err), zap.String("nodeId", s.nodeID))
				// 检查是否需要重新连接
				if s.ctx.Err() == nil {
					time.Sleep(1 * time.Second)
					s.logger.Info("尝试重新连接Redis PubSub", zap.String("nodeId", s.nodeID))
				}
				continue
			}

			// 减少PubSub消息的调试日志，只在需要时记录
			// s.logger.Debug("收到Redis PubSub消息",
			//	zap.String("channel", msg.Channel),
			//	zap.String("payload", msg.Payload),
			//	zap.String("nodeId", s.nodeID))

			var invalidateMsg CacheInvalidateMessage
			if err := json.Unmarshal([]byte(msg.Payload), &invalidateMsg); err != nil {
				s.logger.Error("解析PubSub消息失败",
					zap.Error(err),
					zap.String("payload", msg.Payload),
					zap.String("nodeId", s.nodeID))
				continue
			}

			// 忽略自己发出的消息
			if invalidateMsg.SourceID == s.nodeID {
				// 减少自己发出消息的调试日志
				// s.logger.Debug("忽略自己发出的消息",
				//	zap.String("sourceID", invalidateMsg.SourceID),
				//	zap.String("currentNode", s.nodeID))
				continue
			}

			s.logger.Info("收到其他节点的缓存失效消息",
				zap.String("channel", msg.Channel),
				zap.String("sourceNode", invalidateMsg.SourceID),
				zap.String("currentNode", s.nodeID),
				zap.String("userId", invalidateMsg.UserID),
				zap.Strings("tokenIds", invalidateMsg.TokenIDs),
				zap.String("tokenType", invalidateMsg.TokenType),
				zap.String("action", invalidateMsg.Action))

			// 处理缓存失效消息
			switch invalidateMsg.Action {
			case "invalidate", "single_session_replace":
				userID, err := uuid.FromString(invalidateMsg.UserID)
				if err != nil {
					s.logger.Error("PubSub消息中的用户ID无效",
						zap.Error(err),
						zap.String("userId", invalidateMsg.UserID))
					continue
				}

				s.logger.Info("处理token失效消息",
					zap.String("userId", invalidateMsg.UserID),
					zap.Strings("tokenIds", invalidateMsg.TokenIDs),
					zap.String("action", invalidateMsg.Action))

				for _, tokenID := range invalidateMsg.TokenIDs {
					// 根据token类型处理
					if invalidateMsg.TokenType == "all" {
						// 同时标记session和refresh token无效
						sessionCacheKey := s.getCacheKey(userID, tokenID, "session")
						refreshCacheKey := s.getCacheKey(userID, tokenID, "refresh")
						s.localCache.set(sessionCacheKey, false, time.Hour*24)
						s.localCache.set(refreshCacheKey, false, time.Hour*24)
					} else {
						// 标记特定类型的token无效
						cacheKey := s.getCacheKey(userID, tokenID, invalidateMsg.TokenType)
						s.localCache.set(cacheKey, false, time.Hour*24)
					}
				}

			case "token_added":
				s.logger.Info("收到token添加通知",
					zap.String("userId", invalidateMsg.UserID),
					zap.Strings("tokenIds", invalidateMsg.TokenIDs),
					zap.String("sourceNode", invalidateMsg.SourceID))
				// 对于新添加的token，我们不需要特别处理，让它们通过正常验证流程

			case "ban":
				userID, err := uuid.FromString(invalidateMsg.UserID)
				if err != nil {
					s.logger.Error("Invalid user ID in pubsub message", zap.Error(err))
					continue
				}

				// 获取用户所有token并在本地缓存中标记为无效（使用v2的数据结构）
				sessionKey := s.getSessionTokenKey(userID)
				refreshKey := s.getRefreshTokenKey(userID)

				// 检查现有token并更新本地缓存
				if sessionValue, err := s.client.Get(s.ctx, sessionKey).Result(); err == nil {
					if parts := strings.Split(sessionValue, ":"); len(parts) == 2 {
						s.localCache.set(s.getCacheKey(userID, parts[0], "session"), false, time.Hour*24)
						s.logger.Debug("Marked banned user session token as invalid in local cache",
							zap.String("userID", invalidateMsg.UserID),
							zap.String("tokenID", parts[0]))
					}
				}
				if refreshValue, err := s.client.Get(s.ctx, refreshKey).Result(); err == nil {
					if parts := strings.Split(refreshValue, ":"); len(parts) == 2 {
						s.localCache.set(s.getCacheKey(userID, parts[0], "refresh"), false, time.Hour*24)
						s.logger.Debug("Marked banned user refresh token as invalid in local cache",
							zap.String("userID", invalidateMsg.UserID),
							zap.String("tokenID", parts[0]))
					}
				}
			}
		}
	}
}

func (s *RedisSessionCacheV3) publishInvalidateMessage(userID uuid.UUID, tokenIDs []string, tokenType, action string) {
	msg := CacheInvalidateMessage{
		UserID:    userID.String(),
		TokenIDs:  tokenIDs,
		TokenType: tokenType,
		Action:    action,
		SourceID:  s.nodeID,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("Error marshaling invalidate message", zap.Error(err))
		return
	}

	channel := tokenInvalidateChannel
	if action == "ban" || action == "unban" {
		channel = userBanChannel
	}

	if err := s.client.Publish(s.ctx, channel, msgBytes).Err(); err != nil {
		s.logger.Error("Error publishing invalidate message", zap.Error(err))
	} else {
		// 减少发布消息的调试日志，只在出错时记录
		// s.logger.Debug("Published cache invalidate message",
		//	zap.String("channel", channel),
		//	zap.String("nodeID", s.nodeID),
		//	zap.Any("message", msg))
	}
}

func (s *RedisSessionCacheV3) Stop() {
	s.ctxCancelFn()
	if s.pubsub != nil {
		if err := s.pubsub.Close(); err != nil {
			s.logger.Error("Error closing pubsub connection", zap.Error(err))
		}
	}
	if err := s.client.Close(); err != nil {
		s.logger.Error("Error closing Redis client", zap.Error(err))
	}
}

// 使用与v2版本相同的键名结构
func (s *RedisSessionCacheV3) getSessionTokenKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:session", userID.String())
}

func (s *RedisSessionCacheV3) getRefreshTokenKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:refresh", userID.String())
}

func (s *RedisSessionCacheV3) getBanKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:ban", userID.String())
}

func (s *RedisSessionCacheV3) getCacheKey(userID uuid.UUID, tokenId, tokenType string) string {
	return fmt.Sprintf("%s:%s:%s", userID.String(), tokenId, tokenType)
}

func (s *RedisSessionCacheV3) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
	return s.isValidToken(userID, exp, tokenId, "session")
}

func (s *RedisSessionCacheV3) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
	return s.isValidToken(userID, exp, tokenId, "refresh")
}

func (s *RedisSessionCacheV3) isValidToken(userID uuid.UUID, exp int64, tokenId, tokenType string) bool {
	// 检查本地缓存
	cacheKey := s.getCacheKey(userID, tokenId, tokenType)
	if valid, exists := s.localCache.get(cacheKey); exists {
		return valid
	}

	// 检查用户是否被禁用
	banKey := s.getBanKey(userID)
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking ban status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if banned != "" {
		s.localCache.set(cacheKey, false, time.Hour*24) // 被ban的用户token缓存24小时
		return false
	}

	// 使用v2版本的数据结构：直接检查对应的token键
	var tokenKey string
	if tokenType == "session" {
		tokenKey = s.getSessionTokenKey(userID)
	} else {
		tokenKey = s.getRefreshTokenKey(userID)
	}

	tokenValue, err := s.client.Get(s.ctx, tokenKey).Result()
	if err == redis.Nil {
		s.localCache.set(cacheKey, false, time.Hour*24) // 不存在的token缓存24小时
		s.logger.Debug("Token not found in Redis",
			zap.String("userID", userID.String()),
			zap.String("tokenID", tokenId),
			zap.String("tokenType", tokenType))
		return false
	}
	if err != nil {
		s.logger.Error("Error checking token validity", zap.Error(err))
		s.localCache.set(cacheKey, false, time.Hour) // 错误情况下缓存1小时
		return false
	}

	// 解析token值：格式为 "tokenID:valid"
	expectedValue := fmt.Sprintf("%s:valid", tokenId)
	valid := tokenValue == expectedValue

	// 根据token状态设置不同的缓存时间
	if valid {
		s.localCache.set(cacheKey, true, cacheExpiry) // 有效token使用默认过期时间
	} else {
		s.localCache.set(cacheKey, false, time.Hour*24) // 无效token缓存24小时
	}

	s.logger.Debug("Token validation from Redis",
		zap.String("userID", userID.String()),
		zap.String("tokenID", tokenId),
		zap.String("tokenType", tokenType),
		zap.String("tokenValue", tokenValue),
		zap.String("expectedValue", expectedValue),
		zap.Bool("valid", valid))
	return valid
}

// isValidTokenNoCache 是不使用本地缓存的token验证方法
func (s *RedisSessionCacheV3) isValidTokenNoCache(userID uuid.UUID, exp int64, tokenId, tokenType string) bool {
	s.logger.Debug("检查token有效性",
		zap.String("userId", userID.String()),
		zap.String("tokenId", tokenId),
		zap.String("tokenType", tokenType),
		zap.Int64("exp", exp),
		zap.String("nodeId", s.nodeID))

	// 检查用户是否被禁用
	banKey := s.getBanKey(userID)
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking ban status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if banned != "" {
		s.logger.Debug("用户被禁用", zap.String("userId", userID.String()))
		return false
	}

	// 使用v2版本的数据结构：直接检查对应的token键
	var tokenKey string
	if tokenType == "session" {
		tokenKey = s.getSessionTokenKey(userID)
	} else {
		tokenKey = s.getRefreshTokenKey(userID)
	}

	tokenValue, err := s.client.Get(s.ctx, tokenKey).Result()
	if err == redis.Nil {
		s.logger.Debug("Token不存在",
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId),
			zap.String("tokenType", tokenType))
		return false
	}
	if err != nil {
		s.logger.Error("Error checking token validity", zap.Error(err))
		return false
	}

	// 解析token值：格式为 "tokenID:valid"
	expectedValue := fmt.Sprintf("%s:valid", tokenId)
	isValid := tokenValue == expectedValue

	s.logger.Debug("Token验证结果",
		zap.String("userId", userID.String()),
		zap.String("tokenId", tokenId),
		zap.String("tokenType", tokenType),
		zap.String("tokenValue", tokenValue),
		zap.String("expectedValue", expectedValue),
		zap.Bool("isValid", isValid))

	return isValid
}

// Add 添加新的session token和refresh token
//
// Token处理逻辑：
// 1. 单token模式(singleToken=true): 新token会替换所有现有token
// 2. 非单token模式(singleToken=false): session token和refresh token可以共存
//   - session token失效时，可以使用refresh token申请新的session token
//   - 申请新refresh token时，旧的refresh token仍然有效（支持多设备登录）
func (s *RedisSessionCacheV3) Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 只在需要调试时启用详细日志
	s.logger.Info("添加新token",
		zap.String("userId", userID.String()),
		zap.Bool("singleToken", s.singleToken),
		zap.Bool("tokensAreSame", sessionTokenId == refreshTokenId))

	// 处理token替换逻辑
	if s.singleToken {
		// 单token模式：移除该用户的所有现有token
		s.logger.Debug("单token模式：移除用户现有token", zap.String("userId", userID.String()))

		// 获取现有token并更新本地缓存
		sessionKey := s.getSessionTokenKey(userID)
		refreshKey := s.getRefreshTokenKey(userID)

		// 分别检查现有的session token和refresh token
		var existingSessionTokens []string
		var existingRefreshTokens []string

		if sessionValue, err := s.client.Get(s.ctx, sessionKey).Result(); err == nil {
			if parts := strings.Split(sessionValue, ":"); len(parts) == 2 {
				existingSessionTokens = append(existingSessionTokens, parts[0])
			}
		}
		if refreshValue, err := s.client.Get(s.ctx, refreshKey).Result(); err == nil {
			if parts := strings.Split(refreshValue, ":"); len(parts) == 2 {
				existingRefreshTokens = append(existingRefreshTokens, parts[0])
			}
		}

		// 合并处理现有token的失效
		var allExistingTokens []string
		allExistingTokens = append(allExistingTokens, existingSessionTokens...)
		allExistingTokens = append(allExistingTokens, existingRefreshTokens...)

		if len(allExistingTokens) > 0 {
			s.logger.Debug("发现现有token，准备移除",
				zap.String("userId", userID.String()),
				zap.Strings("existingTokens", allExistingTokens))

			// 更新本地缓存
			for _, token := range existingSessionTokens {
				s.localCache.set(s.getCacheKey(userID, token, "session"), false, cacheExpiry)
			}
			for _, token := range existingRefreshTokens {
				s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, cacheExpiry)
			}

			// 如果session和refresh token是同一个，只发送一次消息
			if len(existingSessionTokens) == 1 && len(existingRefreshTokens) == 1 &&
				existingSessionTokens[0] == existingRefreshTokens[0] {
				// 发送合并的失效消息
				s.publishInvalidateMessage(userID, []string{existingSessionTokens[0]}, "all", "single_session_replace")
			} else {
				// 分别发送失效消息
				if len(existingSessionTokens) > 0 {
					s.publishInvalidateMessage(userID, existingSessionTokens, "session", "single_session_replace")
				}
				if len(existingRefreshTokens) > 0 {
					s.publishInvalidateMessage(userID, existingRefreshTokens, "refresh", "single_session_replace")
				}
			}
		}
		s.RemoveAll(userID)
	} else {
		// 非单token模式：refresh token可以有效共存，不需要特殊处理
		s.logger.Debug("非单token模式：允许多个token共存", zap.String("userId", userID.String()))
	}

	// 使用v2版本的存储格式：直接设置用户的当前token
	pipe := s.client.Pipeline()

	// 添加session token
	if sessionTokenId != "" {
		sessionKey := s.getSessionTokenKey(userID)
		sessionValue := fmt.Sprintf("%s:valid", sessionTokenId)
		// 使用固定的短过期时间，而不是token本身的失效时间
		pipe.Set(s.ctx, sessionKey, sessionValue, redisTokenExpiry)

		// 更新本地缓存
		s.localCache.set(s.getCacheKey(userID, sessionTokenId, "session"), true, cacheExpiry)

		s.logger.Debug("添加session token到Redis",
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.Duration("redisExpiry", redisTokenExpiry))
	}

	// 添加refresh token
	if refreshTokenId != "" {
		refreshKey := s.getRefreshTokenKey(userID)
		refreshValue := fmt.Sprintf("%s:valid", refreshTokenId)
		// 使用固定的短过期时间，而不是token本身的失效时间
		pipe.Set(s.ctx, refreshKey, refreshValue, redisTokenExpiry)

		// 更新本地缓存
		s.localCache.set(s.getCacheKey(userID, refreshTokenId, "refresh"), true, cacheExpiry)

		s.logger.Debug("添加refresh token到Redis",
			zap.String("userId", userID.String()),
			zap.String("refreshTokenId", refreshTokenId),
			zap.Duration("redisExpiry", redisTokenExpiry))
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("添加token到Redis失败", zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))
	} else {
		s.logger.Info("成功添加token到Redis",
			zap.String("userId", userID.String()))

		// 分别发送session token和refresh token的添加通知
		// 如果两个token相同，只发送一次通知以减少消息量
		if sessionTokenId != "" && refreshTokenId != "" && sessionTokenId == refreshTokenId {
			// session token和refresh token相同，发送合并通知
			s.publishInvalidateMessage(userID, []string{sessionTokenId}, "all", "token_added")
		} else {
			// 分别发送通知
			if sessionTokenId != "" {
				s.publishInvalidateMessage(userID, []string{sessionTokenId}, "session", "token_added")
			}
			if refreshTokenId != "" {
				s.publishInvalidateMessage(userID, []string{refreshTokenId}, "refresh", "token_added")
			}
		}
	}
}

func (s *RedisSessionCacheV3) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 分别记录session和refresh token
	var sessionTokens []string
	var refreshTokens []string

	if sessionTokenId != "" {
		sessionTokens = append(sessionTokens, sessionTokenId)
	}
	if refreshTokenId != "" {
		refreshTokens = append(refreshTokens, refreshTokenId)
	}

	// 第一步：更新Redis（使用v2的数据结构）
	pipe := s.client.Pipeline()

	if sessionTokenId != "" {
		sessionKey := s.getSessionTokenKey(userID)
		pipe.Del(s.ctx, sessionKey)
	}

	if refreshTokenId != "" {
		refreshKey := s.getRefreshTokenKey(userID)
		pipe.Del(s.ctx, refreshKey)
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error removing tokens from Redis",
			zap.Error(err),
			zap.String("userID", userID.String()),
			zap.String("sessionToken", sessionTokenId),
			zap.String("refreshToken", refreshTokenId))
		return
	}

	// 第二步：更新本地缓存
	if sessionTokenId != "" {
		s.localCache.set(s.getCacheKey(userID, sessionTokenId, "session"), false, cacheExpiry)
	}
	if refreshTokenId != "" {
		s.localCache.set(s.getCacheKey(userID, refreshTokenId, "refresh"), false, cacheExpiry)
	}

	// 第三步：发送消息通知其他节点
	if len(sessionTokens) > 0 {
		s.publishInvalidateMessage(userID, sessionTokens, "session", "invalidate")
	}
	if len(refreshTokens) > 0 {
		s.publishInvalidateMessage(userID, refreshTokens, "refresh", "invalidate")
	}
}

func (s *RedisSessionCacheV3) RemoveAll(userID uuid.UUID) {
	sessionKey := s.getSessionTokenKey(userID)
	refreshKey := s.getRefreshTokenKey(userID)

	// 第一步：获取现有token信息
	var sessionTokens []string
	var refreshTokens []string

	if sessionValue, err := s.client.Get(s.ctx, sessionKey).Result(); err == nil {
		if parts := strings.Split(sessionValue, ":"); len(parts) == 2 {
			sessionTokens = append(sessionTokens, parts[0])
		}
	}
	if refreshValue, err := s.client.Get(s.ctx, refreshKey).Result(); err == nil {
		if parts := strings.Split(refreshValue, ":"); len(parts) == 2 {
			refreshTokens = append(refreshTokens, parts[0])
		}
	}

	// 第二步：从Redis中删除所有token（使用v2的数据结构）
	pipe := s.client.Pipeline()
	pipe.Del(s.ctx, sessionKey)
	pipe.Del(s.ctx, refreshKey)

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error removing all tokens from Redis",
			zap.Error(err),
			zap.String("userID", userID.String()))
		return
	}

	// 第三步：更新本地缓存
	for _, token := range sessionTokens {
		s.localCache.set(s.getCacheKey(userID, token, "session"), false, time.Hour*24)
	}
	for _, token := range refreshTokens {
		s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, time.Hour*24)
	}

	// 第四步：发送消息通知其他节点
	if len(sessionTokens) > 0 {
		s.publishInvalidateMessage(userID, sessionTokens, "session", "invalidate")
	}
	if len(refreshTokens) > 0 {
		s.publishInvalidateMessage(userID, refreshTokens, "refresh", "invalidate")
	}
}

func (s *RedisSessionCacheV3) Ban(userIDs []uuid.UUID) {
	pipe := s.client.Pipeline()

	for _, userID := range userIDs {
		banKey := s.getBanKey(userID)
		pipe.Set(s.ctx, banKey, "banned", 0) // 永久封禁

		// 获取用户所有token以更新本地缓存（使用v2的数据结构）
		sessionKey := s.getSessionTokenKey(userID)
		refreshKey := s.getRefreshTokenKey(userID)

		// 检查现有token并更新本地缓存
		if sessionValue, err := s.client.Get(s.ctx, sessionKey).Result(); err == nil {
			if parts := strings.Split(sessionValue, ":"); len(parts) == 2 {
				s.localCache.set(s.getCacheKey(userID, parts[0], "session"), false, cacheExpiry)
			}
		}
		if refreshValue, err := s.client.Get(s.ctx, refreshKey).Result(); err == nil {
			if parts := strings.Split(refreshValue, ":"); len(parts) == 2 {
				s.localCache.set(s.getCacheKey(userID, parts[0], "refresh"), false, cacheExpiry)
			}
		}

		s.RemoveAll(userID)

		// 发布封禁消息
		s.publishInvalidateMessage(userID, nil, "", "ban")
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error banning users", zap.Error(err))
	}
}

func (s *RedisSessionCacheV3) Unban(userIDs []uuid.UUID) {
	pipe := s.client.Pipeline()

	for _, userID := range userIDs {
		banKey := s.getBanKey(userID)
		pipe.Del(s.ctx, banKey)
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error unbanning users", zap.Error(err))
	}
}

// publishTokenInvalidateMessage 发送token失效消息到其他节点
func (s *RedisSessionCacheV3) publishTokenInvalidateMessage(userID uuid.UUID, tokenIDs []string, tokenType, action string) {
	if len(tokenIDs) == 0 {
		return
	}

	message := CacheInvalidateMessage{
		UserID:    userID.String(),
		TokenIDs:  tokenIDs,
		TokenType: tokenType,
		Action:    action,
		SourceID:  s.nodeID,
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		s.logger.Error("序列化token失效消息失败",
			zap.Error(err),
			zap.String("userId", userID.String()),
			zap.Strings("tokenIds", tokenIDs),
			zap.String("action", action))
		return
	}

	s.logger.Debug("发送token失效消息",
		zap.String("userId", userID.String()),
		zap.Strings("tokenIds", tokenIDs),
		zap.String("tokenType", tokenType),
		zap.String("action", action),
		zap.String("sourceNodeId", s.nodeID))

	// 使用重试机制发送消息
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = s.client.Publish(s.ctx, tokenInvalidateChannel, messageData).Err()
		if err == nil {
			s.logger.Debug("成功发送token失效消息",
				zap.String("userId", userID.String()),
				zap.String("action", action),
				zap.Int("attempt", i+1))
			return
		}

		s.logger.Warn("发送token失效消息失败，准备重试",
			zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("action", action),
			zap.Int("attempt", i+1),
			zap.Int("maxRetries", maxRetries))

		// 如果不是最后一次重试，等待一段时间
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		}
	}

	s.logger.Error("发送token失效消息最终失败",
		zap.Error(err),
		zap.String("userId", userID.String()),
		zap.Strings("tokenIds", tokenIDs),
		zap.String("action", action),
		zap.Int("maxRetries", maxRetries))
}

// recoverSessionConsistency 恢复session一致性（节点重启时调用）
func (s *RedisSessionCacheV3) recoverSessionConsistency() {
	s.logger.Info("开始恢复session一致性", zap.String("nodeId", s.nodeID))

	// 清空本地缓存，强制从Redis重新加载
	s.localCache.cache.Range(func(key, value interface{}) bool {
		s.localCache.cache.Delete(key)
		return true
	})

	s.logger.Info("已清空本地缓存，将从Redis重新加载session状态", zap.String("nodeId", s.nodeID))
}

// healthCheck 健康检查，确保Redis连接正常（仅在实际需要时调用）
func (s *RedisSessionCacheV3) healthCheck() error {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	err := s.client.Ping(ctx).Err()
	if err != nil {
		s.logger.Error("Redis健康检查失败", zap.Error(err), zap.String("nodeId", s.nodeID))
		return err
	}

	s.logger.Debug("Redis健康检查通过", zap.String("nodeId", s.nodeID))
	return nil
}

// ==================== 已移除的功能 ====================
// 以下功能在Redis高可用环境下已被移除：
// 1. 心跳机制 - Redis集群自动处理节点故障检测
// 2. 定期健康检查 - Redis客户端有自动重连机制
// 3. 节点状态管理 - 依赖Redis高可用架构
//
// 保留功能：
// - Token失效通知PubSub（用于多节点缓存同步）
// - 本地token缓存（提升性能）
// - Redis连接异常时的重连机制（在实际操作失败时触发）

// Redis高可用环境下已禁用定期检查，依赖Redis客户端的自动重连和故障检测
//
// startSimpleRecoveryCheck 已禁用 - Redis高可用环境下不需要定期检查
// func (s *RedisSessionCacheV3) startSimpleRecoveryCheck() { ... }
//
// performSimpleRecoveryCheck 已禁用 - Redis高可用环境下不需要定期检查
// func (s *RedisSessionCacheV3) performSimpleRecoveryCheck() { ... }

// reconnectRedis 重新连接Redis
func (s *RedisSessionCacheV3) reconnectRedis() {
	s.logger.Info("开始重新连接Redis", zap.String("nodeId", s.nodeID))

	// 关闭现有连接
	if s.pubsub != nil {
		err := s.pubsub.Close()
		if err != nil {
			return
		}
		s.pubsub = nil
	}

	// 等待一段时间后重试
	time.Sleep(5 * time.Second)

	// 重新初始化PubSub
	s.initPubSub()

	// 清空本地缓存，强制从Redis重新加载
	s.recoverSessionConsistency()

	// Redis高可用环境下不需要注册节点状态

	s.logger.Info("Redis重连完成", zap.String("nodeId", s.nodeID))
}
