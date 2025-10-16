// Copyright 2021 The Nakama Authors
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
	// Session cache sync message types
	SessionCacheMsgTypeRemove    = "remove"
	SessionCacheMsgTypeRemoveAll = "remove_all"
	SessionCacheMsgTypeBan       = "ban"
)

// sessionCacheSyncMsg 用于节点间同步的消息结构
type sessionCacheSyncMsg struct {
	Type           string   `json:"type"`
	NodeID         string   `json:"node_id"`
	UserID         string   `json:"user_id,omitempty"`
	UserIDs        []string `json:"user_ids,omitempty"`
	SessionExp     int64    `json:"session_exp,omitempty"`
	SessionTokenID string   `json:"session_token_id,omitempty"`
	RefreshExp     int64    `json:"refresh_exp,omitempty"`
	RefreshTokenID string   `json:"refresh_token_id,omitempty"`
	// 注意：不使用 Timestamp 字段来判断消息顺序
	// 每个节点使用自己的本地时间更新 lastInvalidation
}

type SessionCache interface {
	Stop()

	// Check if a given user, expiry, and session token combination is valid.
	IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool
	// Check if a given user, expiry, and refresh token combination is valid.
	IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool
	// Add a valid session and/or refresh token for a given user.
	Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string)
	// Remove a session and/or refresh token for a given user.
	Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string)
	// Remove all of a user's session and refresh tokens.
	RemoveAll(userID uuid.UUID)
	// Mark a set of users as banned.
	Ban(userIDs []uuid.UUID)
	// Unban a set of users.
	Unban(userIDs []uuid.UUID)
}

type sessionCacheUser struct {
	lastInvalidation int64
	sessionTokens    map[string]int64
	refreshTokens    map[string]int64
}

type LocalSessionCache struct {
	sync.RWMutex

	logger                *zap.Logger
	nodeID                string
	tokenExpirySec        int64
	refreshTokenExpirySec int64

	ctx         context.Context
	ctxCancelFn context.CancelFunc

	cache map[uuid.UUID]*sessionCacheUser

	// Redis pub/sub for cluster mode
	redisClient  *redis.Client
	redisPubSub  *redis.PubSub
	redisChannel string // Redis 频道名称
	clusterMode  bool
}

func NewLocalSessionCache(logger *zap.Logger, config Config, tokenExpirySec, refreshTokenExpirySec int64, enableCluster bool, purpose string) SessionCache {
	ctx, ctxCancelFn := context.WithCancel(context.Background())

	// 生成唯一的节点ID
	nodeID := uuid.Must(uuid.NewV4()).String()

	s := &LocalSessionCache{
		logger:                logger,
		nodeID:                nodeID,
		ctx:                   ctx,
		ctxCancelFn:           ctxCancelFn,
		tokenExpirySec:        tokenExpirySec,
		refreshTokenExpirySec: refreshTokenExpirySec,
		cache:                 make(map[uuid.UUID]*sessionCacheUser),
		clusterMode:           false,
	}

	// 初始化集群模式
	if enableCluster {
		s.initializeClusterMode(config, purpose)
	}

	// 启动清理 goroutine
	// 注意：清理操作是本地内存优化，不会触发 Redis 同步
	// 原因：
	// 1. 每个节点都会独立运行相同的清理逻辑
	// 2. 过期时间是固定的，各节点会在相近时间自动清理
	// 3. 清理不影响正确性（验证时会检查过期时间）
	// 4. 避免产生大量不必要的网络流量
	go func() {
		ticker := time.NewTicker(2 * time.Duration(tokenExpirySec) * time.Second)
		for {
			select {
			case <-s.ctx.Done():
				ticker.Stop()
				return
			case t := <-ticker.C:
				ts := t.UTC().Unix()
				s.Lock()
				// 直接操作 map，不调用 Remove/RemoveAll 等方法，避免触发 Redis 同步
				for userID, cache := range s.cache {
					for token, exp := range cache.sessionTokens {
						if exp <= ts {
							delete(cache.sessionTokens, token)
						}
					}
					for token, exp := range cache.refreshTokens {
						if exp <= ts {
							delete(cache.refreshTokens, token)
						}
					}
					if len(cache.sessionTokens) == 0 && len(cache.refreshTokens) == 0 && (cache.lastInvalidation == 0 || (cache.lastInvalidation < ts-tokenExpirySec && cache.lastInvalidation < ts-refreshTokenExpirySec)) {
						delete(s.cache, userID)
					}
				}
				s.Unlock()
			}
		}
	}()

	return s
}

// initializeClusterMode 初始化 Redis 集群模式
func (s *LocalSessionCache) initializeClusterMode(config Config, purpose string) {
	if !config.GetCluster().Enabled {
		s.logger.Warn("Cluster mode requested for session cache but not enabled in config",
			zap.String("purpose", purpose))
		return
	}

	redisAddress := config.GetCluster().RedisAddress
	redisPassword := config.GetCluster().RedisPassword

	if redisAddress == "" {
		s.logger.Warn("Cluster mode enabled but Redis address is empty",
			zap.String("purpose", purpose))
		return
	}
	// 动态生成 Redis 频道名称
	s.redisChannel = fmt.Sprintf("nakama:session_cache:%s:sync", purpose)
	// 创建 Redis 客户端
	s.redisClient = redis.NewClient(&redis.Options{
		Addr:     redisAddress,
		Password: redisPassword,
		DB:       0,
	})

	pingCtx, pingCancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer pingCancel()

	if err := s.redisClient.Ping(pingCtx).Err(); err != nil {
		s.logger.Error("Failed to connect to Redis for session cache, cluster mode disabled",
			zap.Error(err),
			zap.String("redis_address", redisAddress))
		s.redisClient.Close()
		s.redisClient = nil
		return
	}

	s.logger.Info("Successfully connected to Redis for session cache",
		zap.String("redis_address", redisAddress))

	s.redisPubSub = s.redisClient.Subscribe(s.ctx, s.redisChannel)
	s.clusterMode = true
	// 启动消息订阅 goroutine
	go s.subscribeRedisMessages()
}

func (s *LocalSessionCache) Stop() {
	s.ctxCancelFn()

	// 关闭 Redis 连接
	if s.redisPubSub != nil {
		if err := s.redisPubSub.Close(); err != nil {
			s.logger.Error("Failed to close Redis pub/sub connection", zap.Error(err))
		}
	}
	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.Error("Failed to close Redis client", zap.Error(err))
		}
	}
}

func (s *LocalSessionCache) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
	s.RLock()
	cache, found := s.cache[userID]
	if !found {
		// There are no invalidated records for this user, any session token is valid if it passed other JWT checks.
		s.RUnlock()
		return true
	}
	if cache.lastInvalidation > 0 && exp-s.tokenExpirySec < cache.lastInvalidation {
		// The user has a full invalidation recorded, and this token's expiry indicates it was issued before this point.
		s.RUnlock()
		return false
	}
	if _, isInvalidated := cache.sessionTokens[tokenId]; isInvalidated {
		// This token ID has been invalidated.
		s.RUnlock()
		return false
	}
	s.RUnlock()
	return true
}

func (s *LocalSessionCache) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
	s.RLock()
	cache, found := s.cache[userID]
	if !found {
		// There are no invalidated records for this user, any refresh token is valid if it passed other JWT checks.
		s.RUnlock()
		return true
	}
	if cache.lastInvalidation > 0 && exp-s.refreshTokenExpirySec < cache.lastInvalidation {
		// The user has a full invalidation recorded, and this token's expiry indicates it was issued before this point.
		s.RUnlock()
		return false
	}
	if _, isInvalidated := cache.refreshTokens[tokenId]; isInvalidated {
		// This token ID has been invalidated.
		s.RUnlock()
		return false
	}
	s.RUnlock()
	return true
}

func (s *LocalSessionCache) Add(userID uuid.UUID, sessionExp int64, tokenId string, refreshExp int64, refreshTokenId string) {
	// No-op, blacklist only.
}

func (s *LocalSessionCache) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 调用内部方法执行本地操作
	s.removeInternal(userID, sessionExp, sessionTokenId, refreshExp, refreshTokenId)

	// 发送同步消息到其他节点
	if s.clusterMode {
		msg := &sessionCacheSyncMsg{
			Type:           SessionCacheMsgTypeRemove,
			NodeID:         s.nodeID,
			UserID:         userID.String(),
			SessionExp:     sessionExp,
			SessionTokenID: sessionTokenId,
			RefreshExp:     refreshExp,
			RefreshTokenID: refreshTokenId,
		}
		s.publishSyncMessage(msg)
	}
}

// removeInternal 内部方法：只操作本地缓存，不发布消息
func (s *LocalSessionCache) removeInternal(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	s.Lock()
	defer s.Unlock()

	cache, found := s.cache[userID]
	if !found {
		cache = &sessionCacheUser{
			lastInvalidation: 0,
			sessionTokens:    make(map[string]int64),
			refreshTokens:    make(map[string]int64),
		}
		s.cache[userID] = cache
	}
	if sessionTokenId != "" {
		cache.sessionTokens[sessionTokenId] = sessionExp + 1
	}
	if refreshTokenId != "" {
		cache.refreshTokens[refreshTokenId] = refreshExp + 1
	}
}

func (s *LocalSessionCache) RemoveAll(userID uuid.UUID) {
	// 调用内部方法执行本地操作
	s.removeAllInternal(userID)

	// 发送同步消息到其他节点（不携带时间戳）
	if s.clusterMode {
		msg := &sessionCacheSyncMsg{
			Type:   SessionCacheMsgTypeRemoveAll,
			NodeID: s.nodeID,
			UserID: userID.String(),
		}
		s.publishSyncMessage(msg)
	}
}

// removeAllInternal 内部方法：只操作本地缓存，不发布消息
func (s *LocalSessionCache) removeAllInternal(userID uuid.UUID) {
	// 使用秒级时间戳，与 JWT exp 字段保持一致
	ts := time.Now().UTC().Unix()

	s.Lock()
	defer s.Unlock()

	cache, found := s.cache[userID]
	if !found {
		cache = &sessionCacheUser{
			lastInvalidation: 0,
			sessionTokens:    make(map[string]int64),
			refreshTokens:    make(map[string]int64),
		}
		s.cache[userID] = cache
	}
	// 直接更新，不比较时间戳
	cache.lastInvalidation = ts
	cache.sessionTokens = make(map[string]int64)
	cache.refreshTokens = make(map[string]int64)
}

func (s *LocalSessionCache) Ban(userIDs []uuid.UUID) {
	// 调用内部方法执行本地操作
	s.banInternal(userIDs)

	// 发送同步消息到其他节点（不携带时间戳）
	if s.clusterMode && len(userIDs) > 0 {
		userIDStrs := make([]string, len(userIDs))
		for i, uid := range userIDs {
			userIDStrs[i] = uid.String()
		}
		msg := &sessionCacheSyncMsg{
			Type:    SessionCacheMsgTypeBan,
			NodeID:  s.nodeID,
			UserIDs: userIDStrs,
		}
		s.publishSyncMessage(msg)
	}
}

// banInternal 内部方法：只操作本地缓存，不发布消息
func (s *LocalSessionCache) banInternal(userIDs []uuid.UUID) {
	// 使用秒级时间戳，与 JWT exp 字段保持一致
	ts := time.Now().UTC().Unix()

	s.Lock()
	defer s.Unlock()

	for _, userID := range userIDs {
		cache, found := s.cache[userID]
		if !found {
			cache = &sessionCacheUser{
				lastInvalidation: 0,
				sessionTokens:    make(map[string]int64),
				refreshTokens:    make(map[string]int64),
			}
			s.cache[userID] = cache
		}
		// 直接更新，不比较时间戳
		cache.lastInvalidation = ts
	}
}

func (s *LocalSessionCache) Unban(userIDs []uuid.UUID) {
	// 本地不需要做任何操作（黑名单模式）
}

// subscribeRedisMessages 订阅并处理来自其他节点的同步消息
func (s *LocalSessionCache) subscribeRedisMessages() {
	ch := s.redisPubSub.Channel()
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}

			var syncMsg sessionCacheSyncMsg
			if err := json.Unmarshal([]byte(msg.Payload), &syncMsg); err != nil {
				s.logger.Warn("Failed to unmarshal session cache sync message",
					zap.Error(err),
					zap.String("payload", msg.Payload))
				continue
			}

			// 忽略自己发布的消息
			if syncMsg.NodeID == s.nodeID {
				continue
			}

			// 处理同步消息
			s.handleSyncMessage(&syncMsg)
		}
	}
}

// handleSyncMessage 处理从其他节点接收到的同步消息
func (s *LocalSessionCache) handleSyncMessage(msg *sessionCacheSyncMsg) {
	switch msg.Type {
	case SessionCacheMsgTypeRemove:
		if msg.UserID == "" {
			s.logger.Warn("Remove message missing user ID")
			return
		}
		userID, err := uuid.FromString(msg.UserID)
		if err != nil {
			s.logger.Warn("Invalid user ID in Remove message",
				zap.String("user_id", msg.UserID),
				zap.Error(err))
			return
		}

		// 直接调用内部方法，避免重复代码和消息循环
		s.removeInternal(userID, msg.SessionExp, msg.SessionTokenID, msg.RefreshExp, msg.RefreshTokenID)

	case SessionCacheMsgTypeRemoveAll:
		if msg.UserID == "" {
			s.logger.Warn("RemoveAll message missing user ID")
			return
		}
		userID, err := uuid.FromString(msg.UserID)
		if err != nil {
			s.logger.Warn("Invalid user ID in RemoveAll message",
				zap.String("user_id", msg.UserID),
				zap.Error(err))
			return
		}

		// 直接调用内部方法，避免重复代码和消息循环
		s.removeAllInternal(userID)

	case SessionCacheMsgTypeBan:
		if len(msg.UserIDs) == 0 {
			s.logger.Warn("Ban message missing user IDs")
			return
		}

		// 解析 UserIDs
		userIDs := make([]uuid.UUID, 0, len(msg.UserIDs))
		for _, userIDStr := range msg.UserIDs {
			userID, err := uuid.FromString(userIDStr)
			if err != nil {
				s.logger.Warn("Invalid user ID in Ban message",
					zap.String("user_id", userIDStr),
					zap.Error(err))
				continue
			}
			userIDs = append(userIDs, userID)
		}

		// 直接调用内部方法，避免重复代码和消息循环
		if len(userIDs) > 0 {
			s.banInternal(userIDs)
		}
	}
}

// publishSyncMessage 发布同步消息到 Redis（带重试机制）
func (s *LocalSessionCache) publishSyncMessage(msg *sessionCacheSyncMsg) {
	if !s.clusterMode || s.redisClient == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("Failed to marshal session cache sync message", zap.Error(err))
		return
	}

	// 使用重试机制发送消息，确保消息能够成功发送
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = s.redisClient.Publish(s.ctx, s.redisChannel, data).Err()
		if err == nil {
			return
		}

		s.logger.Warn("Failed to publish session cache sync message, retrying",
			zap.Error(err),
			zap.String("type", msg.Type),
			zap.String("channel", s.redisChannel),
			zap.Int("attempt", i+1),
			zap.Int("max_retries", maxRetries))

		// 如果不是最后一次重试，等待后重试
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		}
	}

	s.logger.Error("Failed to publish session cache sync message after retries",
		zap.String("type", msg.Type),
		zap.String("channel", s.redisChannel),
		zap.String("user_id", msg.UserID),
		zap.Int("max_retries", maxRetries))
}
