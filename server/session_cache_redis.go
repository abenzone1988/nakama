package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisSessionCache 包装 LocalSessionCache，实现跨节点同步
type RedisSessionCache struct {
	logger       *zap.Logger
	inner        *LocalSessionCache
	ctx          context.Context
	ctxCancelFn  context.CancelFunc
	nodeID       string
	redisClient  *redis.Client
	redisPubSub  *redis.PubSub
	redisChannel string
}

func NewRedisSessionCache(logger *zap.Logger, config Config, tokenExpirySec, refreshTokenExpirySec int64) SessionCache {
	inner := NewLocalSessionCache(logger, config, tokenExpirySec, refreshTokenExpirySec).(*LocalSessionCache)
	ctx, cancel := context.WithCancel(context.Background())

	c := &RedisSessionCache{
		logger:      logger,
		inner:       inner,
		ctx:         ctx,
		ctxCancelFn: cancel,
		nodeID:      uuid.Must(uuid.NewV4()).String(),
	}

	// 仅当配置启用集群时连接 Redis
	if !config.GetCluster().Enabled {
		return c
	}

	addr := config.GetCluster().RedisAddress
	pass := config.GetCluster().RedisPassword
	if addr == "" {
		logger.Warn("Cluster mode enabled but Redis address is empty")
		return c
	}

	c.redisChannel = "nakama:session_cache:sync"
	c.redisClient = redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     pass,
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  0,
		WriteTimeout: 5 * time.Second,
		PoolSize:     10,
		MaxRetries:   3,
	})

	pingCtx, pingCancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer pingCancel()
	if err := c.redisClient.Ping(pingCtx).Err(); err != nil {
		logger.Error("Failed to connect to Redis for session cache, cluster mode disabled",
			zap.Error(err),
			zap.String("redis_address", addr))
		_ = c.redisClient.Close()
		c.redisClient = nil
		return c
	}

	c.redisPubSub = c.redisClient.Subscribe(c.ctx, c.redisChannel)
	go c.subscribeRedisMessages()
	go c.pingLoop()
	logger.Info("Successfully connected to Redis for session cache",
		zap.String("redis_address", addr),
		zap.String("channel", c.redisChannel))
	return c
}

func (c *RedisSessionCache) Stop() {
	c.ctxCancelFn()
	if c.redisPubSub != nil {
		_ = c.redisPubSub.Close()
	}
	if c.redisClient != nil {
		_ = c.redisClient.Close()
	}
	c.inner.Stop()
}

func (c *RedisSessionCache) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
	return c.inner.IsValidSession(userID, exp, tokenId)
}
func (c *RedisSessionCache) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
	return c.inner.IsValidRefresh(userID, exp, tokenId)
}
func (c *RedisSessionCache) Add(userID uuid.UUID, sessionExp int64, tokenId string, refreshExp int64, refreshTokenId string) {
	c.inner.Add(userID, sessionExp, tokenId, refreshExp, refreshTokenId)
}
func (c *RedisSessionCache) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 本地执行
	c.inner.removeInternal(userID, sessionExp, sessionTokenId, refreshExp, refreshTokenId)
	// 广播
	c.publishSyncMessage(&sessionCacheSyncMsg{
		Type:           SessionCacheMsgTypeRemove,
		NodeID:         c.nodeID,
		UserID:         userID.String(),
		SessionExp:     sessionExp,
		SessionTokenID: sessionTokenId,
		RefreshExp:     refreshExp,
		RefreshTokenID: refreshTokenId,
	})
}
func (c *RedisSessionCache) RemoveAll(userID uuid.UUID) {
	c.inner.removeAllInternal(userID)
	c.publishSyncMessage(&sessionCacheSyncMsg{
		Type:   SessionCacheMsgTypeRemoveAll,
		NodeID: c.nodeID,
		UserID: userID.String(),
	})
}
func (c *RedisSessionCache) Ban(userIDs []uuid.UUID) {
	c.inner.banInternal(userIDs)
	if len(userIDs) == 0 {
		return
	}
	ids := make([]string, len(userIDs))
	for i, id := range userIDs {
		ids[i] = id.String()
	}
	c.publishSyncMessage(&sessionCacheSyncMsg{
		Type:    SessionCacheMsgTypeBan,
		NodeID:  c.nodeID,
		UserIDs: ids,
	})
}
func (c *RedisSessionCache) Unban(userIDs []uuid.UUID) { /* no-op for blacklist */ }

func (c *RedisSessionCache) subscribeRedisMessages() {
	ch := c.redisPubSub.Channel()
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			var syncMsg sessionCacheSyncMsg
			if err := json.Unmarshal([]byte(msg.Payload), &syncMsg); err != nil {
				c.logger.Warn("Failed to unmarshal session cache sync message",
					zap.Error(err),
					zap.String("payload", msg.Payload))
				continue
			}
			if syncMsg.NodeID == c.nodeID {
				continue
			}
			switch syncMsg.Type {
			case SessionCacheMsgTypeRemove:
				userID, err := uuid.FromString(syncMsg.UserID)
				if err == nil {
					c.inner.removeInternal(userID, syncMsg.SessionExp, syncMsg.SessionTokenID, syncMsg.RefreshExp, syncMsg.RefreshTokenID)
				}
			case SessionCacheMsgTypeRemoveAll:
				userID, err := uuid.FromString(syncMsg.UserID)
				if err == nil {
					c.inner.removeAllInternal(userID)
				}
			case SessionCacheMsgTypeBan:
				userIDs := make([]uuid.UUID, 0, len(syncMsg.UserIDs))
				for _, s := range syncMsg.UserIDs {
					if uid, err := uuid.FromString(s); err == nil {
						userIDs = append(userIDs, uid)
					}
				}
				if len(userIDs) > 0 {
					c.inner.banInternal(userIDs)
				}
			}
		}
	}
}

func (c *RedisSessionCache) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.redisPubSub != nil {
				_ = c.redisPubSub.Ping(c.ctx)
			} else if c.redisClient != nil {
				_ = c.redisClient.Ping(c.ctx).Err()
			}
		}
	}
}

func (c *RedisSessionCache) publishSyncMessage(msg *sessionCacheSyncMsg) {
	if c.redisClient == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal session cache sync message", zap.Error(err))
		return
	}
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := c.redisClient.Publish(c.ctx, c.redisChannel, data).Err(); err == nil {
			return
		} else if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		}
	}
}
