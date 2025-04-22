package server

import (
	"context"
	"encoding/json"
	"fmt"
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
)

// CacheInvalidateMessage 缓存失效消息
type CacheInvalidateMessage struct {
	UserID    string   `json:"user_id"`
	TokenIDs  []string `json:"token_ids"`
	TokenType string   `json:"token_type"`
	Action    string   `json:"action"`    // invalidate, ban, unban
	SourceID  string   `json:"source_id"` // 发送消息的节点ID
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

	// 初始化订阅
	cache.initPubSub()

	return cache
}

func (s *RedisSessionCacheV3) initPubSub() {
	// 创建订阅连接
	s.pubsub = s.client.Subscribe(s.ctx, tokenInvalidateChannel, userBanChannel)

	// 启动消息处理goroutine
	go s.handlePubSubMessages()
}

func (s *RedisSessionCacheV3) handlePubSubMessages() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			msg, err := s.pubsub.ReceiveMessage(s.ctx)
			if err != nil {
				s.logger.Error("Error receiving pubsub message", zap.Error(err))
				continue
			}

			var invalidateMsg CacheInvalidateMessage
			if err := json.Unmarshal([]byte(msg.Payload), &invalidateMsg); err != nil {
				s.logger.Error("Error unmarshaling pubsub message", zap.Error(err))
				continue
			}

			// 忽略自己发出的消息
			if invalidateMsg.SourceID == s.nodeID {
				s.logger.Debug("Ignoring self-published message",
					zap.String("sourceID", invalidateMsg.SourceID),
					zap.String("currentNode", s.nodeID))
				continue
			}

			s.logger.Debug("Received cache invalidate message",
				zap.String("channel", msg.Channel),
				zap.String("sourceNode", invalidateMsg.SourceID),
				zap.String("currentNode", s.nodeID),
				zap.Any("message", invalidateMsg))

			// 处理缓存失效消息
			switch invalidateMsg.Action {
			case "invalidate":
				userID, err := uuid.FromString(invalidateMsg.UserID)
				if err != nil {
					s.logger.Error("Invalid user ID in pubsub message", zap.Error(err))
					continue
				}

				for _, tokenID := range invalidateMsg.TokenIDs {
					// 直接将token标记为无效，并设置较长的过期时间
					// 这样可以避免短期内去Redis查询
					cacheKey := s.getCacheKey(userID, tokenID, invalidateMsg.TokenType)
					s.localCache.set(cacheKey, false, time.Hour*24) // 设置24小时过期

					s.logger.Debug("Marked token as invalid in local cache",
						zap.String("userID", invalidateMsg.UserID),
						zap.String("tokenID", tokenID),
						zap.String("tokenType", invalidateMsg.TokenType),
						zap.String("cacheKey", cacheKey))
				}

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
	// 检查用户是否被禁用
	banKey := s.getBanKey(userID)
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking ban status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if banned != "" {
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
		return false
	}

	status, err := statusCmd.Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		s.logger.Error("Error checking token status", zap.Error(err))
		return false
	}

	return status == "valid"
}

func (s *RedisSessionCacheV3) Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	pipe := s.client.Pipeline()

	// 如果是单token模式，先移除该用户的所有现有token
	if s.singleToken {
		// 获取现有token并更新本地缓存
		tokenSetKey := s.getUserTokenSetKey(userID)
		tokens, err := s.client.SMembers(s.ctx, tokenSetKey).Result()
		if err == nil {
			for _, token := range tokens {
				s.localCache.set(s.getCacheKey(userID, token, "session"), false, cacheExpiry)
				s.localCache.set(s.getCacheKey(userID, token, "refresh"), false, cacheExpiry)
			}
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
	}

	// 添加refresh token
	if refreshTokenId != "" {
		pipe.SAdd(s.ctx, tokenSetKey, refreshTokenId)
		pipe.HSet(s.ctx, tokenHashKey, fmt.Sprintf("%s:refresh", refreshTokenId), "valid")
		pipe.Expire(s.ctx, tokenHashKey, time.Duration(s.refreshExpirySec)*time.Second)

		// 更新本地缓存
		s.localCache.set(s.getCacheKey(userID, refreshTokenId, "refresh"), true, cacheExpiry)
	}

	// 设置token集合的过期时间为较长的那个
	expiry := s.tokenExpirySec
	if s.refreshExpirySec > expiry {
		expiry = s.refreshExpirySec
	}
	pipe.Expire(s.ctx, tokenSetKey, time.Duration(expiry)*time.Second)

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error adding tokens", zap.Error(err))
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
