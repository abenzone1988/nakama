package server

import (
	"context"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Redis 分布式锁相关常量和结构
const (
	redisLockPrefix    = "challenge_lock:"
	redisLockTTL       = 3 * time.Second
	redisSpinAttempts  = 300                    // 增加到300次，30ms自旋时间（覆盖2个锁周期）
	redisSpinInterval  = 100 * time.Microsecond // 0.1ms
	redisFallbackRetry = 10                     // 增加降级重试到10次
	redisFallbackDelay = 15 * time.Millisecond  // 增加降级延迟到15ms
)

// RedisDistributedLock Redis 分布式锁结构
type RedisDistributedLock struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisDistributedLock 创建 Redis 分布式锁实例
func NewRedisDistributedLock(client *redis.Client, logger *zap.Logger) *RedisDistributedLock {
	return &RedisDistributedLock{
		client: client,
		logger: logger,
	}
}

// AcquireLock 获取分布式锁
func (r *RedisDistributedLock) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, string, error) {
	lockKey := redisLockPrefix + key
	lockValue := uuid.Must(uuid.NewV4()).String()

	// 使用 SET NX EX 命令原子性地设置锁
	result := r.client.SetNX(ctx, lockKey, lockValue, ttl)
	if result.Err() != nil {
		r.logger.Error("获取Redis分布式锁失败",
			zap.String("lock_key", lockKey),
			zap.Error(result.Err()))
		return false, "", result.Err()
	}

	if result.Val() {
		r.logger.Debug("成功获取Redis分布式锁",
			zap.String("lock_key", lockKey),
			zap.String("lock_value", lockValue))
		return true, lockValue, nil
	}

	return false, "", nil
}

// ReleaseLock 释放分布式锁
func (r *RedisDistributedLock) ReleaseLock(ctx context.Context, key, lockValue string) error {
	lockKey := redisLockPrefix + key

	// 使用 Lua 脚本确保只有锁的持有者才能释放锁
	luaScript := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result := r.client.Eval(ctx, luaScript, []string{lockKey}, lockValue)
	if result.Err() != nil {
		r.logger.Error("释放Redis分布式锁失败",
			zap.String("lock_key", lockKey),
			zap.String("lock_value", lockValue),
			zap.Error(result.Err()))
		return result.Err()
	}

	if result.Val().(int64) == 1 {
		r.logger.Debug("成功释放Redis分布式锁",
			zap.String("lock_key", lockKey),
			zap.String("lock_value", lockValue))
	} else {
		r.logger.Warn("Redis分布式锁已被其他进程释放或过期",
			zap.String("lock_key", lockKey),
			zap.String("lock_value", lockValue))
	}

	return nil
}

// TryLockWithSpin 自旋锁 + 快速失败
func (r *RedisDistributedLock) TryLockWithSpin(ctx context.Context, key string, ttl time.Duration, maxSpinAttempts int, spinInterval time.Duration) (bool, string, error) {
	startTime := time.Now()

	for i := 0; i < maxSpinAttempts; i++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		default:
		}

		acquired, lockValue, err := r.AcquireLock(ctx, key, ttl)
		if err != nil {
			// Redis连接错误，立即返回
			r.logger.Error("Redis自旋锁获取失败，连接错误",
				zap.String("lock_key", key),
				zap.Int("spin_count", i+1),
				zap.Error(err))
			return false, "", err
		}

		if acquired {
			elapsed := time.Since(startTime)
			r.logger.Debug("Redis自旋锁获取成功",
				zap.String("lock_key", key),
				zap.Int("spin_count", i+1),
				zap.Duration("elapsed", elapsed),
				zap.String("lock_value", lockValue))
			return true, lockValue, nil
		}

		// 微秒级自旋等待
		if i < maxSpinAttempts-1 {
			time.Sleep(spinInterval)
		}
	}

	// 自旋结束，快速失败
	elapsed := time.Since(startTime)
	r.logger.Debug("Redis自旋锁获取失败，快速失败",
		zap.String("lock_key", key),
		zap.Int("max_spin_attempts", maxSpinAttempts),
		zap.Duration("elapsed", elapsed))
	return false, "", nil
}

// TryLockWithFallback 自旋锁 + 降级重试
func (r *RedisDistributedLock) TryLockWithFallback(ctx context.Context, key string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (bool, string, error) {
	// 先尝试自旋锁
	acquired, lockValue, err := r.TryLockWithSpin(ctx, key, ttl, redisSpinAttempts, redisSpinInterval)
	if err != nil {
		return false, "", err
	}
	if acquired {
		return true, lockValue, nil
	}

	// 自旋失败，降级到退避重试
	r.logger.Debug("自旋锁失败，降级到退避重试",
		zap.String("lock_key", key))

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		default:
		}

		acquired, lockValue, err := r.AcquireLock(ctx, key, ttl)
		if err != nil {
			return false, "", err
		}

		if acquired {
			r.logger.Debug("降级重试成功获取锁",
				zap.String("lock_key", key),
				zap.Int("retry", i+1))
			return true, lockValue, nil
		}

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	return false, "", nil
}
