package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisSessionCacheV2 struct {
	client           *redis.Client
	tokenExpirySec   int64
	refreshExpirySec int64
	ctx              context.Context
	ctxCancelFn      context.CancelFunc
	logger           *zap.Logger
	singleToken      bool
}

func NewRedisSessionCacheV2(logger *zap.Logger, address string, password string, tokenExpirySec, refreshExpirySec int64, singleToken bool) SessionCache {
	ctx, ctxCancelFn := context.WithCancel(context.Background())

	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       0,
	})

	return &RedisSessionCacheV2{
		client:           client,
		tokenExpirySec:   tokenExpirySec,
		refreshExpirySec: refreshExpirySec,
		ctx:              ctx,
		ctxCancelFn:      ctxCancelFn,
		logger:           logger,
		singleToken:      singleToken,
	}
}

func (s *RedisSessionCacheV2) Stop() {
	s.ctxCancelFn()
	if err := s.client.Close(); err != nil {
		s.logger.Error("Error closing Redis client", zap.Error(err))
	}
}

func (s *RedisSessionCacheV2) getUserTokenSetKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:tokens", userID.String())
}

func (s *RedisSessionCacheV2) getTokenHashKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:token_details", userID.String())
}

func (s *RedisSessionCacheV2) getBanKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:ban", userID.String())
}

func (s *RedisSessionCacheV2) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
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

	// 检查token是否在用户的token集合中
	tokenSetKey := s.getUserTokenSetKey(userID)
	exists, err := s.client.SIsMember(s.ctx, tokenSetKey, tokenId).Result()
	if err != nil {
		s.logger.Error("Error checking token existence", zap.Error(err))
		return false
	}
	if !exists {
		return false
	}

	// 检查token详细信息
	tokenHashKey := s.getTokenHashKey(userID)
	tokenField := fmt.Sprintf("%s:session", tokenId)
	status, err := s.client.HGet(s.ctx, tokenHashKey, tokenField).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		s.logger.Error("Error checking token status", zap.Error(err))
		return false
	}

	return status == "valid"
}

func (s *RedisSessionCacheV2) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
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

	// 检查token是否在用户的token集合中
	tokenSetKey := s.getUserTokenSetKey(userID)
	exists, err := s.client.SIsMember(s.ctx, tokenSetKey, tokenId).Result()
	if err != nil {
		s.logger.Error("Error checking token existence", zap.Error(err))
		return false
	}
	if !exists {
		return false
	}

	// 检查refresh token详细信息
	tokenHashKey := s.getTokenHashKey(userID)
	tokenField := fmt.Sprintf("%s:refresh", tokenId)
	status, err := s.client.HGet(s.ctx, tokenHashKey, tokenField).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		s.logger.Error("Error checking refresh token status", zap.Error(err))
		return false
	}

	return status == "valid"
}

func (s *RedisSessionCacheV2) Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	pipe := s.client.Pipeline()

	// 如果是单token模式，先移除该用户的所有现有token
	if s.singleToken {
		s.RemoveAll(userID)
	}

	tokenSetKey := s.getUserTokenSetKey(userID)
	tokenHashKey := s.getTokenHashKey(userID)

	// 添加session token
	if sessionTokenId != "" {
		pipe.SAdd(s.ctx, tokenSetKey, sessionTokenId)
		pipe.HSet(s.ctx, tokenHashKey, fmt.Sprintf("%s:session", sessionTokenId), "valid")
		pipe.Expire(s.ctx, tokenHashKey, time.Duration(s.tokenExpirySec)*time.Second)
	}

	// 添加refresh token
	if refreshTokenId != "" {
		pipe.SAdd(s.ctx, tokenSetKey, refreshTokenId)
		pipe.HSet(s.ctx, tokenHashKey, fmt.Sprintf("%s:refresh", refreshTokenId), "valid")
		pipe.Expire(s.ctx, tokenHashKey, time.Duration(s.refreshExpirySec)*time.Second)
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

func (s *RedisSessionCacheV2) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
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
		s.logger.Error("Error removing tokens", zap.Error(err))
	}
}

func (s *RedisSessionCacheV2) RemoveAll(userID uuid.UUID) {
	pipe := s.client.Pipeline()
	tokenSetKey := s.getUserTokenSetKey(userID)
	tokenHashKey := s.getTokenHashKey(userID)

	pipe.Del(s.ctx, tokenSetKey)
	pipe.Del(s.ctx, tokenHashKey)

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error removing all tokens", zap.Error(err))
	}
}

func (s *RedisSessionCacheV2) Ban(userIDs []uuid.UUID) {
	pipe := s.client.Pipeline()

	for _, userID := range userIDs {
		banKey := s.getBanKey(userID)
		pipe.Set(s.ctx, banKey, "banned", 0) // 永久封禁
		s.RemoveAll(userID)
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		s.logger.Error("Error banning users", zap.Error(err))
	}
}

func (s *RedisSessionCacheV2) Unban(userIDs []uuid.UUID) {
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
