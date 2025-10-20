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
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
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
	tokenExpirySec        int64
	refreshTokenExpirySec int64

	ctx         context.Context
	ctxCancelFn context.CancelFunc

	cache map[uuid.UUID]*sessionCacheUser
}

func NewLocalSessionCache(logger *zap.Logger, config Config, tokenExpirySec, refreshTokenExpirySec int64) SessionCache {
	ctx, ctxCancelFn := context.WithCancel(context.Background())

	s := &LocalSessionCache{
		logger:                logger,
		ctx:                   ctx,
		ctxCancelFn:           ctxCancelFn,
		tokenExpirySec:        tokenExpirySec,
		refreshTokenExpirySec: refreshTokenExpirySec,
		cache:                 make(map[uuid.UUID]*sessionCacheUser),
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

func (s *LocalSessionCache) Stop() {
	s.ctxCancelFn()
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
	// 调用内部方法执行本地操作（本地实现不再发布跨节点消息）
	s.removeInternal(userID, sessionExp, sessionTokenId, refreshExp, refreshTokenId)
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
	// 调用内部方法执行本地操作（本地实现不再发布跨节点消息）
	s.removeAllInternal(userID)
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
	// 调用内部方法执行本地操作（本地实现不再发布跨节点消息）
	s.banInternal(userIDs)
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
// Local 实现不包含 Redis 订阅/发布逻辑，相关能力由 RedisSessionCache 包装器提供
