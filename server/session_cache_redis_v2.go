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
		Addr:         address,
		Password:     password,
		DB:           0,
		PoolSize:     20,               // 增加连接池大小
		MinIdleConns: 10,               // 增加最小空闲连接
		MaxRetries:   5,                // 增加重试次数
		DialTimeout:  10 * time.Second, // 增加连接超时
		ReadTimeout:  5 * time.Second,  // 增加读取超时
		WriteTimeout: 5 * time.Second,  // 增加写入超时
	})

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		logger.Error("Redis连接测试失败", zap.Error(err))
	} else {
		logger.Info("Redis连接测试成功")
	}

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

// 单Token优化：直接存储token状态
func (s *RedisSessionCacheV2) getSessionTokenKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:session", userID.String())
}

func (s *RedisSessionCacheV2) getRefreshTokenKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:refresh", userID.String())
}

func (s *RedisSessionCacheV2) getBanKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:ban", userID.String())
}

func (s *RedisSessionCacheV2) IsValidSession(userID uuid.UUID, exp int64, tokenId string) bool {
	s.logger.Debug("V2验证session token",
		zap.String("userId", userID.String()),
		zap.String("tokenId", tokenId))

	// 修复Pipeline问题：分别检查ban状态和session token
	banKey := s.getBanKey(userID)
	sessionKey := s.getSessionTokenKey(userID)

	// 1. 检查封禁状态
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Warn("Redis封禁状态查询失败，允许refresh token验证通过",
			zap.Error(err),
			zap.String("userId", userID.String()))
		return false
	}
	if err == redis.Nil {
		s.logger.Debug("banKey不能存在验证通过",
			zap.String("userId", userID.String()))
		return true
	}
	if banned != "" {
		s.logger.Info("用户被封禁，拒绝refresh token验证",
			zap.String("userId", userID.String()))
		return false
	}

	// 2. 检查session token
	sessionValue, err := s.client.Get(s.ctx, sessionKey).Result()
	if err == redis.Nil {
		s.logger.Debug("Token在Redis中不存在",
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId))
		return false
	}
	if err != nil {
		s.logger.Warn("Redis session token查询失败，允许token验证通过",
			zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId))
		return true
	}

	// 3. 解析token值：格式为 "tokenID:status"
	expectedValue := fmt.Sprintf("%s:valid", tokenId)
	isValid := sessionValue == expectedValue

	s.logger.Debug("Token验证结果",
		zap.String("userId", userID.String()),
		zap.String("tokenId", tokenId),
		zap.String("sessionValue", sessionValue),
		zap.String("expectedValue", expectedValue),
		zap.Bool("isValid", isValid))

	return isValid
}

func (s *RedisSessionCacheV2) IsValidRefresh(userID uuid.UUID, exp int64, tokenId string) bool {
	// 修复Pipeline问题：分别检查ban状态和refresh token
	banKey := s.getBanKey(userID)
	refreshKey := s.getRefreshTokenKey(userID)

	// 1. 检查封禁状态
	banned, err := s.client.Get(s.ctx, banKey).Result()
	if err != nil && err != redis.Nil {
		s.logger.Warn("Redis封禁状态查询失败，允许refresh token验证通过",
			zap.Error(err),
			zap.String("userId", userID.String()))
		return false
	}
	if err == redis.Nil {
		s.logger.Debug("banKey不能存在验证通过",
			zap.String("userId", userID.String()))
		return true
	}
	if banned != "" {
		s.logger.Info("用户被封禁，拒绝refresh token验证",
			zap.String("userId", userID.String()))
		return false
	}

	// 2. 检查refresh token
	refreshValue, err := s.client.Get(s.ctx, refreshKey).Result()
	if err == redis.Nil {
		s.logger.Debug("Refresh token在Redis中不存在",
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId))
		return false
	}
	if err != nil {
		s.logger.Warn("Redis refresh token查询失败，允许token验证通过",
			zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("tokenId", tokenId))
		return true
	}

	// 3. 解析token值：格式为 "tokenID:status"
	expectedValue := fmt.Sprintf("%s:valid", tokenId)
	isValid := refreshValue == expectedValue

	s.logger.Debug("Refresh token验证结果",
		zap.String("userId", userID.String()),
		zap.String("tokenId", tokenId),
		zap.String("refreshValue", refreshValue),
		zap.String("expectedValue", expectedValue),
		zap.Bool("isValid", isValid))

	return isValid
}

func (s *RedisSessionCacheV2) Add(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	s.logger.Info("V2添加token",
		zap.String("userId", userID.String()),
		zap.String("sessionTokenId", sessionTokenId),
		zap.String("refreshTokenId", refreshTokenId),
		zap.Bool("singleToken", s.singleToken))

	// 如果是单token模式，先移除该用户的所有现有token
	if s.singleToken {
		s.logger.Info("V2单token模式：移除用户现有token", zap.String("userId", userID.String()))
		s.RemoveAll(userID)
	}

	// 单Token优化：直接设置用户的当前token
	pipe := s.client.Pipeline()

	// 添加session token
	if sessionTokenId != "" {
		sessionKey := s.getSessionTokenKey(userID)
		sessionValue := fmt.Sprintf("%s:valid", sessionTokenId)
		pipe.Set(s.ctx, sessionKey, sessionValue, time.Duration(s.tokenExpirySec)*time.Second)
	}

	// 添加refresh token
	if refreshTokenId != "" {
		refreshKey := s.getRefreshTokenKey(userID)
		refreshValue := fmt.Sprintf("%s:valid", refreshTokenId)
		pipe.Set(s.ctx, refreshKey, refreshValue, time.Duration(s.refreshExpirySec)*time.Second)
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		// Redis不可用时记录警告，但不影响业务流程
		s.logger.Warn("Redis不可用，无法存储token，但允许用户继续使用",
			zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))
		// 不返回错误，让业务继续进行
	} else {
		s.logger.Debug("成功添加token到Redis",
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))
	}
}

func (s *RedisSessionCacheV2) Remove(userID uuid.UUID, sessionExp int64, sessionTokenId string, refreshExp int64, refreshTokenId string) {
	// 单Token优化：直接删除对应的token
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
		// Redis不可用时记录警告，但不影响业务流程
		s.logger.Warn("Redis不可用，无法删除token",
			zap.Error(err),
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))
	} else {
		s.logger.Debug("成功从Redis删除token",
			zap.String("userId", userID.String()),
			zap.String("sessionTokenId", sessionTokenId),
			zap.String("refreshTokenId", refreshTokenId))
	}
}

func (s *RedisSessionCacheV2) RemoveAll(userID uuid.UUID) {
	// 单Token优化：删除用户的所有token
	pipe := s.client.Pipeline()

	sessionKey := s.getSessionTokenKey(userID)
	refreshKey := s.getRefreshTokenKey(userID)

	s.logger.Info("V2删除用户所有token",
		zap.String("userId", userID.String()),
		zap.String("sessionKey", sessionKey),
		zap.String("refreshKey", refreshKey))

	pipe.Del(s.ctx, sessionKey)
	pipe.Del(s.ctx, refreshKey)

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		// Redis不可用时记录警告，但不影响业务流程
		s.logger.Warn("Redis不可用，无法删除用户所有token",
			zap.Error(err),
			zap.String("userId", userID.String()))
	} else {
		s.logger.Info("V2成功从Redis删除用户所有token",
			zap.String("userId", userID.String()))
	}
}

func (s *RedisSessionCacheV2) Ban(userIDs []uuid.UUID) {
	pipe := s.client.Pipeline()

	for _, userID := range userIDs {
		// 设置封禁状态
		banKey := s.getBanKey(userID)
		pipe.Set(s.ctx, banKey, "banned", 0) // 永久封禁

		// 删除用户的所有token
		sessionKey := s.getSessionTokenKey(userID)
		refreshKey := s.getRefreshTokenKey(userID)
		pipe.Del(s.ctx, sessionKey)
		pipe.Del(s.ctx, refreshKey)
	}

	_, err := pipe.Exec(s.ctx)
	if err != nil {
		// Redis不可用时记录警告，但不影响业务流程
		s.logger.Warn("Redis不可用，无法执行用户封禁操作", zap.Error(err))
	} else {
		s.logger.Info("成功执行用户封禁操作", zap.Int("userCount", len(userIDs)))
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
		// Redis不可用时记录警告，但不影响业务流程
		s.logger.Warn("Redis不可用，无法执行用户解封操作", zap.Error(err))
	} else {
		s.logger.Info("成功执行用户解封操作", zap.Int("userCount", len(userIDs)))
	}
}
