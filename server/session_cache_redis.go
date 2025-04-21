package server

import (
	"context"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisSessionCache struct {
	client           *redis.Client
	tokenExpirySec   int64
	refreshExpirySec int64
	ctx              context.Context
	ctxCancelFn      context.CancelFunc
	logger           *zap.Logger
	singleToken      bool
}

func NewRedisSessionCache(logger *zap.Logger, address string, password string, tokenExpirySec, refreshExpirySec int64, singleToken bool) SessionCache {
	ctx, ctxCancelFn := context.WithCancel(context.Background())

	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       0,
	})

	return &RedisSessionCache{
		client:           client,
		tokenExpirySec:   tokenExpirySec,
		refreshExpirySec: refreshExpirySec,
		ctx:              ctx,
		ctxCancelFn:      ctxCancelFn,
		logger:           logger,
		singleToken:      singleToken,
	}
}

func (s *RedisSessionCache) Stop() {
	s.ctxCancelFn()
	if err := s.client.Close(); err != nil {
		s.logger.Error("Error closing Redis client", zap.Error(err))
	}
}

func (s *RedisSessionCache) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
	// 检查用户是否被禁用
	banKey := "user:ban:" + userID.String()
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking ban status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if banned != "" {
		return false
	}

	// 检查是否有全局失效标记
	invalidationKey := "user:invalidation:" + userID.String()
	invalidationTime, err := s.client.Get(s.ctx, invalidationKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking invalidation status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if invalidationTime != "" {
		return false
	}

	// 检查token是否有效
	tokenKey := "session:token:" + userID.String() + ":" + tokenId
	value, err := s.client.Get(s.ctx, tokenKey).Result()
	if err == redis.Nil {
		return false // token 不存在
	}
	if err != nil {
		s.logger.Error("Error checking token validity", zap.Error(err), zap.String("userId", userID.String()), zap.String("tokenId", tokenId))
		return false
	}
	return value == "valid"
}

func (s *RedisSessionCache) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
	// 检查用户是否被禁用
	banKey := "user:ban:" + userID.String()
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking ban status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if banned != "" {
		return false
	}

	// 检查是否有全局失效标记
	invalidationKey := "user:invalidation:" + userID.String()
	invalidationTime, err := s.client.Get(s.ctx, invalidationKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Error("Error checking invalidation status", zap.Error(err), zap.String("userId", userID.String()))
		return false
	}
	if invalidationTime != "" {
		return false
	}

	// 检查refresh token是否有效
	tokenKey := "session:refresh:" + userID.String() + ":" + tokenId
	value, err := s.client.Get(s.ctx, tokenKey).Result()
	if err == redis.Nil {
		return false // token 不存在
	}
	if err != nil {
		s.logger.Error("Error checking refresh token validity", zap.Error(err), zap.String("userId", userID.String()), zap.String("tokenId", tokenId))
		return false
	}
	return value == "valid"
}

func (s *RedisSessionCache) Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 如果是单token模式，先移除该用户的所有现有token
	if s.singleToken {
		s.RemoveAll(userID)
	}

	// 添加session token
	if sessionTokenId != "" {
		tokenKey := "session:token:" + userID.String() + ":" + sessionTokenId
		err := s.client.Set(s.ctx, tokenKey, "valid", time.Duration(s.tokenExpirySec)*time.Second).Err()
		if err != nil {
			s.logger.Error("Error adding session token", zap.Error(err), zap.String("userId", userID.String()), zap.String("tokenId", sessionTokenId))
		}
	}

	// 添加refresh token
	if refreshTokenId != "" {
		tokenKey := "session:refresh:" + userID.String() + ":" + refreshTokenId
		err := s.client.Set(s.ctx, tokenKey, "valid", time.Duration(s.refreshExpirySec)*time.Second).Err()
		if err != nil {
			s.logger.Error("Error adding refresh token", zap.Error(err), zap.String("userId", userID.String()), zap.String("tokenId", refreshTokenId))
		}
	}
}

func (s *RedisSessionCache) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 将session token加入黑名单
	if sessionTokenId != "" {
		tokenKey := "session:token:" + userID.String() + ":" + sessionTokenId
		err := s.client.Set(s.ctx, tokenKey, "invalid", time.Duration(sessionExp-time.Now().Unix())*time.Second).Err()
		if err != nil {
			s.logger.Error("Error removing session token", zap.Error(err), zap.String("userId", userID.String()), zap.String("tokenId", sessionTokenId))
		}
	}

	// 将refresh token加入黑名单
	if refreshTokenId != "" {
		tokenKey := "session:refresh:" + userID.String() + ":" + refreshTokenId
		err := s.client.Set(s.ctx, tokenKey, "invalid", time.Duration(refreshExp-time.Now().Unix())*time.Second).Err()
		if err != nil {
			s.logger.Error("Error removing refresh token", zap.Error(err), zap.String("userId", userID.String()), zap.String("tokenId", refreshTokenId))
		}
	}
}

func (s *RedisSessionCache) RemoveAll(userID uuid.UUID) {
	// 查找并移除该用户的所有token
	pattern := "session:*:" + userID.String() + ":*"
	iter := s.client.Scan(s.ctx, 0, pattern, 0).Iterator()
	for iter.Next(s.ctx) {
		key := iter.Val()
		err := s.client.Del(s.ctx, key).Err()
		if err != nil {
			s.logger.Error("Error removing token", zap.Error(err), zap.String("key", key))
		}
	}
	if err := iter.Err(); err != nil {
		s.logger.Error("Error scanning for tokens", zap.Error(err), zap.String("userId", userID.String()))
	}
}

func (s *RedisSessionCache) Ban(userIDs []uuid.UUID) {
	for _, userID := range userIDs {
		banKey := "user:ban:" + userID.String()
		err := s.client.Set(s.ctx, banKey, "banned", 0).Err() // 永久封禁
		if err != nil {
			s.logger.Error("Error banning user", zap.Error(err), zap.String("userId", userID.String()))
		}
		s.RemoveAll(userID)
	}
}

func (s *RedisSessionCache) Unban(userIDs []uuid.UUID) {
	for _, userID := range userIDs {
		banKey := "user:ban:" + userID.String()
		err := s.client.Del(s.ctx, banKey).Err()
		if err != nil {
			s.logger.Error("Error unbanning user", zap.Error(err), zap.String("userId", userID.String()))
		}
	}
}
