package server

import (
	"context"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisSessionCache 直接使用 Redis 存储会话失效信息（黑名单 + 全量失效时间）
type RedisSessionCache struct {
	logger                *zap.Logger
	client                *redis.Client
	ctx                   context.Context
	ctxCancelFn           context.CancelFunc
	tokenExpirySec        int64
	refreshTokenExpirySec int64
	singleSessionEnabled  bool
}

func NewRedisSessionCache(logger *zap.Logger, config Config, tokenExpirySec, refreshTokenExpirySec int64) SessionCache {
	ctx, cancel := context.WithCancel(context.Background())

	c := &RedisSessionCache{
		logger:                logger,
		ctx:                   ctx,
		ctxCancelFn:           cancel,
		tokenExpirySec:        tokenExpirySec,
		refreshTokenExpirySec: refreshTokenExpirySec,
		singleSessionEnabled:  config.GetSession().SingleSession,
	}

	c.client = redis.NewClient(&redis.Options{
		Addr:         config.GetCluster().RedisAddress,
		Password:     config.GetCluster().RedisPassword,
		DB:           config.GetCluster().RedisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     10,
		MaxRetries:   3,
	})

	if err := c.client.Ping(ctx).Err(); err != nil {
		logger.Warn("Failed to connect to Redis for session cache; best-effort only",
			zap.Error(err))
	}

	return c
}

func (c *RedisSessionCache) Stop() {
	c.ctxCancelFn()
	if c.client != nil {
		_ = c.client.Close()
	}
}

// IsValidSession: 返回该 session 是否有效（不在黑名单，且未被全量失效覆盖）
func (c *RedisSessionCache) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
	if c.client == nil {
		return true
	}
	// 优先使用“当前会话token”硬约束单会话，避免与 lastInvalidation 秒级边界冲突
	if tokenId != "" && c.singleSessionEnabled {
		cur := c.client.Get(c.ctx, c.keyCurrentSession(userID)).Val()
		if cur != "" {
			if cur == tokenId {
				return true
			}
			c.logger.Debug("Session invalid by current-token mismatch",
				zap.String("user_id", userID.String()))
			return false
		}
	}
	lastInv, _ := c.client.Get(c.ctx, c.keyUserLast(userID)).Int64()
	issuedAt := exp - c.tokenExpirySec
	if lastInv > 0 && issuedAt < lastInv {
		c.logger.Debug("Session invalid by lastInvalidation",
			zap.String("user_id", userID.String()),
			zap.Int64("exp", exp),
			zap.Int64("issued_at_calc", issuedAt),
			zap.Int64("last_invalidation", lastInv))
		return false
	}
	if tokenId == "" {
		return true
	}
	black := c.client.Exists(c.ctx, c.keySessionToken(tokenId)).Val() != 0
	if black {
		c.logger.Debug("Session invalid by blacklist",
			zap.String("user_id", userID.String()),
			zap.String("token_id", tokenId))
	}
	return !black
}

// IsValidRefresh: 返回该 refresh token 是否有效
func (c *RedisSessionCache) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
	if c.client == nil {
		return true
	}
	lastInv, _ := c.client.Get(c.ctx, c.keyUserLast(userID)).Int64()
	if lastInv > 0 && exp-c.refreshTokenExpirySec < lastInv {
		return false
	}
	if tokenId == "" {
		return true
	}
	return c.client.Exists(c.ctx, c.keyRefreshToken(tokenId)).Val() == 0
}

// Add: 黑名单模型不需要白名单添加，留空
func (c *RedisSessionCache) Add(userID uuid.UUID, sessionExp int64, tokenId string, refreshExp int64, refreshTokenId string) {
	if c.client == nil {
		return
	}
	now := time.Now().UTC().Unix()
	if tokenId != "" && sessionExp > 0 && c.singleSessionEnabled {
		ttlSec := max64(1, sessionExp-now)
		_ = c.client.Set(c.ctx, c.keyCurrentSession(userID), tokenId, time.Duration(ttlSec)*time.Second).Err()
	}
	if refreshTokenId != "" && refreshExp > 0 {
		ttlSec := max64(1, refreshExp-now)
		_ = c.client.Set(c.ctx, c.keyCurrentRefresh(userID), refreshTokenId, time.Duration(ttlSec)*time.Second).Err()
	}
}

// Remove: 将给定 token 加入黑名单（设置 TTL 到期自动清除）
func (c *RedisSessionCache) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	if c.client == nil {
		return
	}
	now := time.Now().UTC().Unix()
	// 记录全量失效时间，确保单会话场景下其他旧会话立即失效
	if err := c.client.Set(c.ctx, c.keyUserLast(userID), now, 0).Err(); err == nil {
		c.logger.Debug("Set lastInvalidation on Remove",
			zap.String("user_id", userID.String()),
			zap.Int64("last_invalidation", now))
	}
	if sessionTokenId != "" {
		ttlSec := max64(1, sessionExp+1-now)
		if err := c.client.Set(c.ctx, c.keySessionToken(sessionTokenId), userID.String(), time.Duration(ttlSec)*time.Second).Err(); err == nil {
			c.logger.Debug("Blacklist session token",
				zap.String("user_id", userID.String()),
				zap.String("token_id", sessionTokenId),
				zap.Int64("ttl_sec", ttlSec))
		}
	}
	if refreshTokenId != "" {
		ttlSec := max64(1, refreshExp+1-now)
		if err := c.client.Set(c.ctx, c.keyRefreshToken(refreshTokenId), userID.String(), time.Duration(ttlSec)*time.Second).Err(); err == nil {
			c.logger.Debug("Blacklist refresh token",
				zap.String("user_id", userID.String()),
				zap.String("token_id", refreshTokenId),
				zap.Int64("ttl_sec", ttlSec))
		}
	}
}

// RemoveAll: 记录用户最后一次全量失效时间（永久存储）
func (c *RedisSessionCache) RemoveAll(userID uuid.UUID) {
	if c.client == nil {
		return
	}
	ts := time.Now().UTC().Unix()
	if err := c.client.Set(c.ctx, c.keyUserLast(userID), ts, 0).Err(); err == nil {
		c.logger.Debug("Set lastInvalidation on RemoveAll",
			zap.String("user_id", userID.String()),
			zap.Int64("last_invalidation", ts))
	}
	// 同时清理当前会话指针，避免刚好等于的 token 误判
	_ = c.client.Del(c.ctx, c.keyCurrentSession(userID)).Err()
	_ = c.client.Del(c.ctx, c.keyCurrentRefresh(userID)).Err()
}

// Ban: 同 RemoveAll，对多用户执行
func (c *RedisSessionCache) Ban(userIDs []uuid.UUID) {
	if c.client == nil || len(userIDs) == 0 {
		return
	}
	ts := time.Now().UTC().Unix()
	for _, uid := range userIDs {
		_ = c.client.Set(c.ctx, c.keyUserLast(uid), ts, 0).Err()
	}
}

// Unban: 黑名单模式通常不恢复历史 token，保持空实现
func (c *RedisSessionCache) Unban(userIDs []uuid.UUID) {}

// key helpers
func (c *RedisSessionCache) keyUserLast(userID uuid.UUID) string {
	return "nakama:session:last:" + userID.String()
}
func (c *RedisSessionCache) keySessionToken(tokenId string) string {
	return "nakama:session:black:s:" + tokenId
}
func (c *RedisSessionCache) keyRefreshToken(tokenId string) string {
	return "nakama:session:black:r:" + tokenId
}
func (c *RedisSessionCache) keyCurrentSession(userID uuid.UUID) string {
	return "nakama:session:current:s:" + userID.String()
}
func (c *RedisSessionCache) keyCurrentRefresh(userID uuid.UUID) string {
	return "nakama:session:current:r:" + userID.String()
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
