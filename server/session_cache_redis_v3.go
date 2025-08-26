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

	// Redis PubSub channels
	tokenInvalidateChannel = "token:invalidate" // token失效通知channel
	userBanChannel         = "user:ban"         // 用户封禁通知channel
	nodeHeartbeatChannel   = "node:heartbeat"   // 节点心跳通知channel

	// 节点恢复相关常量
	heartbeatInterval     = 30 * time.Second // 心跳间隔
	nodeTimeoutDuration   = 90 * time.Second // 节点超时时间
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

// NodeHeartbeatMessage 节点心跳消息
type NodeHeartbeatMessage struct {
	NodeID    string    `json:"node_id"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"` // online, offline, recovering
}

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
	nodeID           string        // 节点唯一标识
}

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

	// 初始化订阅和恢复机制
	cache.initPubSub()
	cache.initNodeRecovery()

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

			s.logger.Debug("收到Redis PubSub消息",
				zap.String("channel", msg.Channel),
				zap.String("payload", msg.Payload),
				zap.String("nodeId", s.nodeID))

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
				s.logger.Debug("忽略自己发出的消息",
					zap.String("sourceID", invalidateMsg.SourceID),
					zap.String("currentNode", s.nodeID))
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
					// 直接将token标记为无效，并设置较长的过期时间
					// 这样可以避免短期内去Redis查询
					cacheKey := s.getCacheKey(userID, tokenID, invalidateMsg.TokenType)
					s.localCache.set(cacheKey, false, time.Hour*24) // 设置24小时过期

					s.logger.Debug("本地缓存中标记token无效",
						zap.String("userID", invalidateMsg.UserID),
						zap.String("tokenID", tokenID),
						zap.String("tokenType", invalidateMsg.TokenType),
						zap.String("action", invalidateMsg.Action),
						zap.String("cacheKey", cacheKey))
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

				// 获取用户所有token并在本地缓存中标记为无效
				tokenSetKey := s.getUserTokenSetKey(userID)
				tokens, err := s.client.SMembers(s.ctx, tokenSetKey).Result()
				if err == nil {
					for _, token := range tokens {
						// 同样设置较长的过期时间
						s.localCache.set(s.getCacheKey(userID, token, "session"), false, time.Hour*24)
						s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, time.Hour*24)

						s.logger.Debug("Marked banned user token as invalid in local cache",
							zap.String("userID", invalidateMsg.UserID),
							zap.String("tokenID", token))
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
		s.logger.Debug("Published cache invalidate message",
			zap.String("channel", channel),
			zap.String("nodeID", s.nodeID),
			zap.Any("message", msg))
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

func (s *RedisSessionCacheV3) getUserTokenSetKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:tokens", userID.String())
}

func (s *RedisSessionCacheV3) getTokenHashKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:token_details", userID.String())
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

	// 使用Pipeline批量检查token
	pipe := s.client.Pipeline()

	// 检查token是否在Set中
	tokenSetKey := s.getUserTokenSetKey(userID)
	existsCmd := pipe.SIsMember(s.ctx, tokenSetKey, tokenId)

	// 检查token状态
	tokenHashKey := s.getTokenHashKey(userID)
	tokenField := fmt.Sprintf("%s:%s", tokenId, tokenType)
	statusCmd := pipe.HGet(s.ctx, tokenHashKey, tokenField)

	_, err = pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error checking token validity", zap.Error(err))
		s.localCache.set(cacheKey, false, time.Hour) // 错误情况下缓存1小时
		return false
	}

	exists, err := existsCmd.Result()
	if err != nil || !exists {
		s.localCache.set(cacheKey, false, time.Hour*24) // 不存在的token缓存24小时
		s.logger.Debug("Token not found in Redis",
			zap.String("userID", userID.String()),
			zap.String("tokenID", tokenId),
			zap.String("tokenType", tokenType))
		return false
	}

	status, err := statusCmd.Result()
	valid := err == nil && status == "valid"
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

	// 使用Pipeline批量检查token
	pipe := s.client.Pipeline()

	// 检查token是否在Set中
	tokenSetKey := s.getUserTokenSetKey(userID)
	existsCmd := pipe.SIsMember(s.ctx, tokenSetKey, tokenId)

	// 检查token状态
	tokenHashKey := s.getTokenHashKey(userID)
	tokenField := fmt.Sprintf("%s:%s", tokenId, tokenType)
	statusCmd := pipe.HGet(s.ctx, tokenHashKey, tokenField)

	_, err = pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error checking token validity", zap.Error(err))
		return false
	}

	exists, err := existsCmd.Result()
	if err != nil || !exists {
		s.logger.Debug("Token不存在或查询失败",
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId),
			zap.Bool("exists", exists),
			zap.Error(err))
		return false
	}

	status, err := statusCmd.Result()
	if err == redis.Nil {
		s.logger.Debug("Token状态不存在",
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId))
		return false
	}
	if err != nil {
		s.logger.Error("Error checking token status", zap.Error(err))
		return false
	}

	isValid := status == "valid"
	s.logger.Debug("Token验证结果",
		zap.String("userId", userID.String()),
		zap.String("tokenId", tokenId),
		zap.String("status", status),
		zap.Bool("isValid", isValid))

	return isValid
}

func (s *RedisSessionCacheV3) Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	s.logger.Debug("添加新token",
		zap.String("userId", userID.String()),
		zap.String("sessionTokenId", sessionTokenId),
		zap.String("refreshTokenId", refreshTokenId),
		zap.Int64("sessionExp", sessionExp),
		zap.Int64("refreshExp", refreshExp),
		zap.Bool("singleToken", s.singleToken),
		zap.String("nodeId", s.nodeID))

	pipe := s.client.Pipeline()

	// 如果是单token模式，先移除该用户的所有现有token
	if s.singleToken {
		s.logger.Debug("单token模式：移除用户现有token", zap.String("userId", userID.String()))

		// 获取现有token并更新本地缓存
		tokenSetKey := s.getUserTokenSetKey(userID)
		tokens, err := s.client.SMembers(s.ctx, tokenSetKey).Result()
		if err == nil && len(tokens) > 0 {
			s.logger.Debug("发现现有token，准备移除",
				zap.String("userId", userID.String()),
				zap.Strings("existingTokens", tokens),
				zap.Int("tokenCount", len(tokens)))

			for _, token := range tokens {
				s.localCache.set(s.getCacheKey(userID, token, "session"), false, cacheExpiry)
				s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, cacheExpiry)
			}

			// 发送token失效消息到其他节点
			s.publishTokenInvalidateMessage(userID, tokens, "all", "single_session_replace")
		} else if err != nil {
			s.logger.Warn("获取现有token失败", zap.Error(err), zap.String("userId", userID.String()))
		}
		s.RemoveAll(userID)
	}

	tokenSetKey := s.getUserTokenSetKey(userID)
	tokenHashKey := s.getTokenHashKey(userID)

	// 添加session token
	if sessionTokenId != "" {
		pipe.SAdd(s.ctx, tokenSetKey, sessionTokenId)
		pipe.HSet(s.ctx, tokenHashKey, fmt.Sprintf("%s:session", sessionTokenId), "valid")
		pipe.Expire(s.ctx, tokenHashKey, time.Duration(s.tokenExpirySec)*time.Second)

		// 更新本地缓存
		s.localCache.set(s.getCacheKey(userID, sessionTokenId, "session"), true, cacheExpiry)

		s.logger.Debug("添加session token到Redis",
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.Int64("expirySec", s.tokenExpirySec))
	}

	// 添加refresh token
	if refreshTokenId != "" {
		pipe.SAdd(s.ctx, tokenSetKey, refreshTokenId)
		pipe.HSet(s.ctx, tokenHashKey, fmt.Sprintf("%s:refresh", refreshTokenId), "valid")
		pipe.Expire(s.ctx, tokenHashKey, time.Duration(s.refreshExpirySec)*time.Second)

		// 更新本地缓存
		s.localCache.set(s.getCacheKey(userID, refreshTokenId, "refresh"), true, cacheExpiry)

		s.logger.Debug("添加refresh token到Redis",
			zap.String("userId", userID.String()),
			zap.String("refreshTokenId", refreshTokenId),
			zap.Int64("expirySec", s.refreshExpirySec))
	}

	// 设置token集合的过期时间为较长的那个
	expiry := s.tokenExpirySec
	if s.refreshExpirySec > expiry {
		expiry = s.refreshExpirySec
	}
	pipe.Expire(s.ctx, tokenSetKey, time.Duration(expiry)*time.Second)

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("添加token到Redis失败", zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))
	} else {
		s.logger.Debug("成功添加token到Redis",
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))

		// 发送新token添加消息到其他节点（用于缓存同步）
		var newTokens []string
		if sessionTokenId != "" {
			newTokens = append(newTokens, sessionTokenId)
		}
		if refreshTokenId != "" {
			newTokens = append(newTokens, refreshTokenId)
		}
		if len(newTokens) > 0 {
			s.publishTokenInvalidateMessage(userID, newTokens, "all", "token_added")
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

	// 第一步：更新Redis
	pipe := s.client.Pipeline()
	tokenSetKey := s.getUserTokenSetKey(userID)
	tokenHashKey := s.getTokenHashKey(userID)

	if sessionTokenId != "" {
		pipe.SRem(s.ctx, tokenSetKey, sessionTokenId)
		pipe.HDel(s.ctx, tokenHashKey, fmt.Sprintf("%s:session", sessionTokenId))
	}

	if refreshTokenId != "" {
		pipe.SRem(s.ctx, tokenSetKey, refreshTokenId)
		pipe.HDel(s.ctx, tokenHashKey, fmt.Sprintf("%s:refresh", refreshTokenId))
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
	tokenSetKey := s.getUserTokenSetKey(userID)
	tokenHashKey := s.getTokenHashKey(userID)

	// 第一步：获取所有token
	tokens, err := s.client.SMembers(s.ctx, tokenSetKey).Result()
	if err != nil {
		s.logger.Error("Error getting tokens for removal",
			zap.Error(err),
			zap.String("userID", userID.String()))
		return
	}

	// 第二步：获取token类型信息
	var sessionTokens []string
	var refreshTokens []string

	for _, token := range tokens {
		// 检查session token
		sessionField := fmt.Sprintf("%s:session", token)
		refreshField := fmt.Sprintf("%s:refresh", token)

		if _, err := s.client.HGet(s.ctx, tokenHashKey, sessionField).Result(); err == nil {
			sessionTokens = append(sessionTokens, token)
		}
		if _, err := s.client.HGet(s.ctx, tokenHashKey, refreshField).Result(); err == nil {
			refreshTokens = append(refreshTokens, token)
		}
	}

	// 第三步：从Redis中删除所有token
	if err := s.client.Del(s.ctx, tokenSetKey, tokenHashKey).Err(); err != nil {
		s.logger.Error("Error removing all tokens from Redis",
			zap.Error(err),
			zap.String("userID", userID.String()))
		return
	}

	// 第四步：更新本地缓存
	for _, token := range sessionTokens {
		s.localCache.set(s.getCacheKey(userID, token, "session"), false, time.Hour*24)
	}
	for _, token := range refreshTokens {
		s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, time.Hour*24)
	}

	// 第五步：发送消息通知其他节点
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

		// 获取用户所有token以更新本地缓存
		tokenSetKey := s.getUserTokenSetKey(userID)
		tokens, err := s.client.SMembers(s.ctx, tokenSetKey).Result()
		if err == nil {
			for _, token := range tokens {
				s.localCache.set(s.getCacheKey(userID, token, "session"), false, cacheExpiry)
				s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, cacheExpiry)
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

// healthCheck 健康检查，确保Redis连接正常
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

// initNodeRecovery 初始化节点恢复机制
func (s *RedisSessionCacheV3) initNodeRecovery() {
	s.logger.Info("初始化节点恢复机制", zap.String("nodeId", s.nodeID))

	// 注册节点上线
	s.registerNodeOnline()

	// 启动心跳发送
	go s.startHeartbeat()

	// 启动恢复检查
	go s.startRecoveryCheck()

	// 启动心跳监听
	go s.startHeartbeatListener()
}

// registerNodeOnline 注册节点上线状态
func (s *RedisSessionCacheV3) registerNodeOnline() {
	nodeKey := fmt.Sprintf("node:status:%s", s.nodeID)

	nodeInfo := map[string]interface{}{
		"node_id":        s.nodeID,
		"status":         "online",
		"start_time":     time.Now().Unix(),
		"last_heartbeat": time.Now().Unix(),
	}

	err := s.client.HMSet(s.ctx, nodeKey, nodeInfo).Err()
	if err != nil {
		s.logger.Error("注册节点状态失败", zap.Error(err), zap.String("nodeId", s.nodeID))
		return
	}

	// 设置节点状态过期时间
	s.client.Expire(s.ctx, nodeKey, nodeTimeoutDuration)

	s.logger.Info("节点已注册上线", zap.String("nodeId", s.nodeID))

	// 发送上线通知
	s.sendHeartbeat("online")
}

// startHeartbeat 启动心跳发送
func (s *RedisSessionCacheV3) startHeartbeat() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	s.logger.Info("开始发送心跳", zap.String("nodeId", s.nodeID), zap.Duration("interval", heartbeatInterval))

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("停止心跳发送", zap.String("nodeId", s.nodeID))
			// 发送下线通知
			s.sendHeartbeat("offline")
			return
		case <-ticker.C:
			s.sendHeartbeat("online")
			s.updateNodeHeartbeat()
		}
	}
}

// sendHeartbeat 发送心跳消息
func (s *RedisSessionCacheV3) sendHeartbeat(status string) {
	message := NodeHeartbeatMessage{
		NodeID:    s.nodeID,
		Timestamp: time.Now(),
		Status:    status,
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		s.logger.Error("序列化心跳消息失败", zap.Error(err), zap.String("nodeId", s.nodeID))
		return
	}

	err = s.client.Publish(s.ctx, nodeHeartbeatChannel, messageData).Err()
	if err != nil {
		s.logger.Warn("发送心跳消息失败", zap.Error(err), zap.String("nodeId", s.nodeID))
	} else {
		s.logger.Debug("发送心跳消息", zap.String("nodeId", s.nodeID), zap.String("status", status))
	}
}

// updateNodeHeartbeat 更新节点心跳时间
func (s *RedisSessionCacheV3) updateNodeHeartbeat() {
	nodeKey := fmt.Sprintf("node:status:%s", s.nodeID)

	err := s.client.HSet(s.ctx, nodeKey, "last_heartbeat", time.Now().Unix()).Err()
	if err != nil {
		s.logger.Warn("更新节点心跳时间失败", zap.Error(err), zap.String("nodeId", s.nodeID))
	}

	// 续期节点状态
	s.client.Expire(s.ctx, nodeKey, nodeTimeoutDuration)
}

// startHeartbeatListener 启动心跳监听
func (s *RedisSessionCacheV3) startHeartbeatListener() {
	// 订阅心跳频道
	heartbeatPubSub := s.client.Subscribe(s.ctx, nodeHeartbeatChannel)
	defer heartbeatPubSub.Close()

	s.logger.Info("开始监听节点心跳", zap.String("nodeId", s.nodeID))

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("停止监听节点心跳", zap.String("nodeId", s.nodeID))
			return
		default:
			msg, err := heartbeatPubSub.ReceiveMessage(s.ctx)
			if err != nil {
				s.logger.Warn("接收心跳消息失败", zap.Error(err), zap.String("nodeId", s.nodeID))
				continue
			}

			var heartbeatMsg NodeHeartbeatMessage
			if err := json.Unmarshal([]byte(msg.Payload), &heartbeatMsg); err != nil {
				s.logger.Warn("解析心跳消息失败", zap.Error(err), zap.String("payload", msg.Payload))
				continue
			}

			// 忽略自己的心跳消息
			if heartbeatMsg.NodeID == s.nodeID {
				continue
			}

			s.logger.Debug("收到其他节点心跳",
				zap.String("fromNode", heartbeatMsg.NodeID),
				zap.String("status", heartbeatMsg.Status),
				zap.Time("timestamp", heartbeatMsg.Timestamp))

			// 处理节点状态变化
			s.handleNodeStatusChange(heartbeatMsg)
		}
	}
}

// handleNodeStatusChange 处理节点状态变化
func (s *RedisSessionCacheV3) handleNodeStatusChange(msg NodeHeartbeatMessage) {
	switch msg.Status {
	case "online":
		s.logger.Debug("检测到节点上线", zap.String("nodeId", msg.NodeID))
	case "offline":
		s.logger.Info("检测到节点下线", zap.String("nodeId", msg.NodeID))
		// 可以在这里处理节点下线的清理工作
	case "recovering":
		s.logger.Info("检测到节点恢复中", zap.String("nodeId", msg.NodeID))
		// 节点恢复时，可能需要重新同步数据
	}
}

// startRecoveryCheck 启动恢复检查
func (s *RedisSessionCacheV3) startRecoveryCheck() {
	ticker := time.NewTicker(recoveryCheckInterval)
	defer ticker.Stop()

	s.logger.Info("开始恢复检查", zap.String("nodeId", s.nodeID), zap.Duration("interval", recoveryCheckInterval))

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("停止恢复检查", zap.String("nodeId", s.nodeID))
			return
		case <-ticker.C:
			s.performRecoveryCheck()
		}
	}
}

// performRecoveryCheck 执行恢复检查
func (s *RedisSessionCacheV3) performRecoveryCheck() {
	// 1. 检查Redis连接状态
	if err := s.healthCheck(); err != nil {
		s.logger.Error("Redis连接异常，尝试重连", zap.Error(err))
		s.reconnectRedis()
		return
	}

	// 2. 检查PubSub连接状态
	if s.pubsub == nil {
		s.logger.Warn("PubSub连接丢失，尝试重新初始化")
		s.initPubSub()
		return
	}

	// 3. 检查本地缓存一致性
	s.validateCacheConsistency()
}

// reconnectRedis 重新连接Redis
func (s *RedisSessionCacheV3) reconnectRedis() {
	s.logger.Info("开始重新连接Redis", zap.String("nodeId", s.nodeID))

	// 关闭现有连接
	if s.pubsub != nil {
		s.pubsub.Close()
		s.pubsub = nil
	}

	// 等待一段时间后重试
	time.Sleep(5 * time.Second)

	// 重新初始化PubSub
	s.initPubSub()

	// 清空本地缓存，强制从Redis重新加载
	s.recoverSessionConsistency()

	// 重新注册节点状态
	s.registerNodeOnline()

	s.logger.Info("Redis重连完成", zap.String("nodeId", s.nodeID))
}

// validateCacheConsistency 验证缓存一致性
func (s *RedisSessionCacheV3) validateCacheConsistency() {
	// 随机采样一些本地缓存条目，与Redis进行比较
	sampleCount := 0
	maxSamples := 10

	s.localCache.cache.Range(func(key, value interface{}) bool {
		if sampleCount >= maxSamples {
			return false
		}

		cacheKey := key.(string)
		localEntry := value.(tokenCacheEntry)

		// 解析缓存键获取用户ID和token ID
		parts := strings.Split(cacheKey, ":")
		if len(parts) != 3 {
			return true
		}

		userID, err := uuid.FromString(parts[0])
		if err != nil {
			return true
		}

		tokenID := parts[1]
		tokenType := parts[2]

		// 检查Redis中的实际状态
		redisValid := s.isValidTokenNoCache(userID, 0, tokenID, tokenType)

		// 如果本地缓存与Redis状态不一致，记录警告
		if localEntry.valid != redisValid {
			s.logger.Warn("检测到缓存不一致",
				zap.String("cacheKey", cacheKey),
				zap.Bool("localValid", localEntry.valid),
				zap.Bool("redisValid", redisValid),
				zap.String("nodeId", s.nodeID))

			// 更新本地缓存以匹配Redis状态
			s.localCache.set(cacheKey, redisValid, cacheExpiry)
		}

		sampleCount++
		return true
	})

	if sampleCount > 0 {
		s.logger.Debug("缓存一致性检查完成",
			zap.Int("sampledEntries", sampleCount),
			zap.String("nodeId", s.nodeID))
	}
}
