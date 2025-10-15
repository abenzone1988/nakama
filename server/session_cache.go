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
	SessionCacheMsgTypeAdd       = "add"
	SessionCacheMsgTypeRemove    = "remove"
	SessionCacheMsgTypeRemoveAll = "remove_all"
	SessionCacheMsgTypeBan       = "ban"
	SessionCacheMsgTypeUnban     = "unban"
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

func NewLocalSessionCache(logger *zap.Logger, config Config, tokenExpirySec, refreshTokenExpirySec int64, purpose string) SessionCache {
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

	// 检查是否启用集群模式
	if config.GetCluster().Enabled {
		redisAddress := config.GetCluster().RedisAddress
		redisPassword := config.GetCluster().RedisPassword

		if redisAddress != "" {
			s.logger.Info("Attempting to enable session cache cluster mode",
				zap.String("node_id", nodeID),
				zap.String("redis_address", redisAddress),
				zap.String("purpose", purpose))

			// 动态生成 Redis 频道名称
			s.redisChannel = fmt.Sprintf("nakama:session_cache:%s:sync", purpose)

			// 初始化 Redis 客户端
			s.redisClient = redis.NewClient(&redis.Options{
				Addr:     redisAddress,
				Password: redisPassword,
				DB:       0,
			})

			// 测试 Redis 连接
			s.logger.Info("Connecting to Redis for session cache...",
				zap.String("redis_address", redisAddress))

			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			defer pingCancel()

			if err := s.redisClient.Ping(pingCtx).Err(); err != nil {
				s.logger.Error("Failed to connect to Redis for session cache, cluster mode disabled",
					zap.Error(err),
					zap.String("redis_address", redisAddress))
				s.redisClient.Close()
				s.redisClient = nil
			} else {
				s.logger.Info("Successfully connected to Redis for session cache",
					zap.String("redis_address", redisAddress))

				// 订阅 Redis 频道
				s.logger.Info("Subscribing to Redis channel for session cache sync",
					zap.String("channel", s.redisChannel))
				s.redisPubSub = s.redisClient.Subscribe(ctx, s.redisChannel)

				s.clusterMode = true
				s.logger.Info("Session cache cluster mode enabled",
					zap.String("node_id", nodeID),
					zap.String("redis_address", redisAddress),
					zap.String("channel", s.redisChannel))

				// 启动消息订阅 goroutine
				go s.subscribeRedisMessages()
			}
		}
	}

	// 启动清理 goroutine
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

	// 但如果启用了集群模式，发送同步消息通知其他节点
	if s.clusterMode {
		msg := &sessionCacheSyncMsg{
			Type:           SessionCacheMsgTypeAdd,
			NodeID:         s.nodeID,
			UserID:         userID.String(),
			SessionExp:     sessionExp,
			SessionTokenID: tokenId,
			RefreshExp:     refreshExp,
			RefreshTokenID: refreshTokenId,
		}
		s.publishSyncMessage(msg)
	}
}

func (s *LocalSessionCache) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	s.Lock()
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
	s.Unlock()

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

func (s *LocalSessionCache) RemoveAll(userID uuid.UUID) {
	// 使用秒级时间戳，与 JWT exp 字段保持一致
	ts := time.Now().UTC().Unix()

	s.Lock()
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
	s.Unlock()

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

func (s *LocalSessionCache) Ban(userIDs []uuid.UUID) {
	// 使用秒级时间戳，与 JWT exp 字段保持一致
	ts := time.Now().UTC().Unix()

	s.Lock()
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
	s.Unlock()

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

func (s *LocalSessionCache) Unban(userIDs []uuid.UUID) {
	// 本地不需要做任何操作（黑名单模式）

	// 但如果启用了集群模式，发送同步消息通知其他节点
	if s.clusterMode && len(userIDs) > 0 {
		userIDStrs := make([]string, len(userIDs))
		for i, uid := range userIDs {
			userIDStrs[i] = uid.String()
		}
		msg := &sessionCacheSyncMsg{
			Type:    SessionCacheMsgTypeUnban,
			NodeID:  s.nodeID,
			UserIDs: userIDStrs,
		}
		s.publishSyncMessage(msg)
	}
}

// subscribeRedisMessages 订阅并处理来自其他节点的同步消息
func (s *LocalSessionCache) subscribeRedisMessages() {
	s.logger.Info("Session cache Redis message subscription started",
		zap.String("node_id", s.nodeID),
		zap.String("channel", s.redisChannel))

	ch := s.redisPubSub.Channel()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Session cache Redis message subscription stopped",
				zap.String("node_id", s.nodeID))
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
				s.logger.Debug("Ignoring self-published session cache sync message",
					zap.String("type", syncMsg.Type),
					zap.String("node_id", syncMsg.NodeID))
				continue
			}

			s.logger.Info("Received session cache sync message from other node",
				zap.String("type", syncMsg.Type),
				zap.String("from_node", syncMsg.NodeID),
				zap.String("current_node", s.nodeID),
				zap.String("user_id", syncMsg.UserID))

			// 处理同步消息
			s.handleSyncMessage(&syncMsg)
		}
	}
}

// handleSyncMessage 处理从其他节点接收到的同步消息
func (s *LocalSessionCache) handleSyncMessage(msg *sessionCacheSyncMsg) {
	switch msg.Type {
	case SessionCacheMsgTypeAdd:
		// Add 操作在黑名单模式下是 no-op，不需要处理
		s.logger.Debug("Received Add message (no-op in blacklist mode)",
			zap.String("user_id", msg.UserID),
			zap.String("from_node", msg.NodeID))

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

		s.Lock()
		cache, found := s.cache[userID]
		if !found {
			cache = &sessionCacheUser{
				lastInvalidation: 0,
				sessionTokens:    make(map[string]int64),
				refreshTokens:    make(map[string]int64),
			}
			s.cache[userID] = cache
		}
		if msg.SessionTokenID != "" {
			cache.sessionTokens[msg.SessionTokenID] = msg.SessionExp + 1
			s.logger.Info("Marked session token as invalid via sync",
				zap.String("user_id", msg.UserID),
				zap.String("token_id", msg.SessionTokenID),
				zap.String("from_node", msg.NodeID))
		}
		if msg.RefreshTokenID != "" {
			cache.refreshTokens[msg.RefreshTokenID] = msg.RefreshExp + 1
			s.logger.Info("Marked refresh token as invalid via sync",
				zap.String("user_id", msg.UserID),
				zap.String("token_id", msg.RefreshTokenID),
				zap.String("from_node", msg.NodeID))
		}
		s.Unlock()

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

		// 直接应用 RemoveAll，使用接收时的本地秒级时间戳，不依赖消息中的时间戳
		ts := time.Now().UTC().Unix()

		s.Lock()
		cache, found := s.cache[userID]
		if !found {
			cache = &sessionCacheUser{
				lastInvalidation: 0,
				sessionTokens:    make(map[string]int64),
				refreshTokens:    make(map[string]int64),
			}
			s.cache[userID] = cache
		}

		// 直接应用，不判断时间戳大小
		cache.lastInvalidation = ts
		cache.sessionTokens = make(map[string]int64)
		cache.refreshTokens = make(map[string]int64)
		s.Unlock()

		s.logger.Info("Applied RemoveAll sync from other node",
			zap.String("user_id", msg.UserID),
			zap.String("from_node", msg.NodeID))

	case SessionCacheMsgTypeBan:
		s.logger.Info("Processing Ban sync message",
			zap.Int("user_count", len(msg.UserIDs)),
			zap.String("from_node", msg.NodeID))

		// 直接应用 Ban，使用接收时的本地秒级时间戳
		ts := time.Now().UTC().Unix()

		for _, userIDStr := range msg.UserIDs {
			userID, err := uuid.FromString(userIDStr)
			if err != nil {
				s.logger.Warn("Invalid user ID in Ban message",
					zap.String("user_id", userIDStr),
					zap.Error(err))
				continue
			}

			s.Lock()
			cache, found := s.cache[userID]
			if !found {
				cache = &sessionCacheUser{
					lastInvalidation: 0,
					sessionTokens:    make(map[string]int64),
					refreshTokens:    make(map[string]int64),
				}
				s.cache[userID] = cache
			}
			// 直接应用，不判断时间戳大小
			cache.lastInvalidation = ts
			s.Unlock()

			s.logger.Info("Banned user via sync",
				zap.String("user_id", userIDStr),
				zap.String("from_node", msg.NodeID))
		}

	case SessionCacheMsgTypeUnban:
		// Unban 是 no-op，不需要处理
		s.logger.Debug("Received Unban message (no-op)",
			zap.Int("user_count", len(msg.UserIDs)),
			zap.String("from_node", msg.NodeID))
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

	s.logger.Debug("Publishing session cache sync message",
		zap.String("type", msg.Type),
		zap.String("node_id", msg.NodeID),
		zap.String("user_id", msg.UserID),
		zap.String("channel", s.redisChannel),
		zap.Int("user_ids_count", len(msg.UserIDs)))

	// 使用重试机制发送消息，确保消息能够成功发送
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = s.redisClient.Publish(s.ctx, s.redisChannel, data).Err()
		if err == nil {
			s.logger.Info("Successfully published session cache sync message",
				zap.String("type", msg.Type),
				zap.String("channel", s.redisChannel),
				zap.String("user_id", msg.UserID),
				zap.Int("attempt", i+1))
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
