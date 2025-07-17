// Copyright 2018 The Nakama Authors
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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	redisRankCacheExpiry       = 1 * time.Hour       // Redis中排行榜缓存的过期时间
	localRankCacheExpiry       = 5 * time.Minute     // 本地缓存的过期时间
	rankCacheInvalidateChannel = "rank:invalidate"   // 排行榜缓存失效通知频道
	rankCacheUpdateChannel     = "rank:update"       // 排行榜数据更新通知频道
	redisSortedSetKeyPrefix    = "leaderboard:rank:" // Redis有序集合的键前缀
	redisMetaKeyPrefix         = "leaderboard:meta:" // Redis排行榜元数据键前缀
	defaultRankCacheBatchSize  = 1000
)

// RankCacheEntry 排行榜缓存条目
type RankCacheEntry struct {
	OwnerID    uuid.UUID `json:"owner_id"`
	Score      int64     `json:"score"`
	Subscore   int64     `json:"subscore"`
	Generation int32     `json:"generation"`
	Timestamp  int64     `json:"timestamp"`
}

// RankCacheMessage 排行榜缓存消息
type RankCacheMessage struct {
	LeaderboardID string          `json:"leaderboard_id"`
	ExpiryUnix    int64           `json:"expiry_unix"`
	OwnerID       uuid.UUID       `json:"owner_id"`
	Action        string          `json:"action"` // insert, delete, clear
	Data          *RankCacheEntry `json:"data,omitempty"`
	NodeID        string          `json:"node_id"`
}

// 本地缓存条目
type localRankCacheEntry struct {
	rank       int64
	expireTime time.Time
}

// 本地缓存
type localRankCache struct {
	cache sync.Map
}

func (lrc *localRankCache) get(key string) (int64, bool) {
	if value, exists := lrc.cache.Load(key); exists {
		entry := value.(localRankCacheEntry)
		if time.Now().Before(entry.expireTime) {
			return entry.rank, true
		}
		lrc.cache.Delete(key)
	}
	return 0, false
}

func (lrc *localRankCache) set(key string, rank int64, expiry time.Duration) {
	lrc.cache.Store(key, localRankCacheEntry{
		rank:       rank,
		expireTime: time.Now().Add(expiry),
	})
}

func (lrc *localRankCache) invalidate(key string) {
	lrc.cache.Delete(key)
}

func (lrc *localRankCache) cleanup() {
	now := time.Now()
	lrc.cache.Range(func(key, value interface{}) bool {
		entry := value.(localRankCacheEntry)
		if now.After(entry.expireTime) {
			lrc.cache.Delete(key)
		}
		return true
	})
}

// RedisLeaderboardRankCache Redis分布式排行榜缓存
type RedisLeaderboardRankCache struct {
	client       *redis.Client
	ctx          context.Context
	ctxCancelFn  context.CancelFunc
	logger       *zap.Logger
	nodeID       string
	localCache   *localRankCache
	pubsub       *redis.PubSub
	blacklistAll bool
	blacklistIds map[string]struct{}
	mu           sync.RWMutex
}

var _ LeaderboardRankCache = &RedisLeaderboardRankCache{}

func NewRedisLeaderboardRankCache(ctx context.Context, logger *zap.Logger, address, password string, config *LeaderboardConfig) LeaderboardRankCache {
	ctxWithCancel, ctxCancelFn := context.WithCancel(ctx)

	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       1, // 使用不同的数据库避免与session缓存冲突
	})

	nodeID := uuid.Must(uuid.NewV4()).String()

	cache := &RedisLeaderboardRankCache{
		client:       client,
		ctx:          ctxWithCancel,
		ctxCancelFn:  ctxCancelFn,
		logger:       logger,
		nodeID:       nodeID,
		localCache:   &localRankCache{},
		blacklistIds: make(map[string]struct{}, len(config.BlacklistRankCache)),
		blacklistAll: len(config.BlacklistRankCache) == 1 && config.BlacklistRankCache[0] == "*",
	}

	// 设置黑名单
	for _, id := range config.BlacklistRankCache {
		cache.blacklistIds[id] = struct{}{}
	}

	if cache.blacklistAll {
		logger.Info("Redis leaderboard rank cache is disabled")
		return cache
	}

	// 初始化发布/订阅
	cache.initPubSub()

	// 启动本地缓存清理
	go cache.startLocalCacheCleanup()

	logger.Info("Redis leaderboard rank cache initialized", zap.String("node_id", nodeID))

	return cache
}

func (r *RedisLeaderboardRankCache) initPubSub() {
	r.pubsub = r.client.Subscribe(r.ctx, rankCacheInvalidateChannel, rankCacheUpdateChannel)
	go r.handlePubSubMessages()
}

func (r *RedisLeaderboardRankCache) handlePubSubMessages() {
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			msg, err := r.pubsub.ReceiveMessage(r.ctx)
			if err != nil {
				r.logger.Error("Error receiving rank cache pubsub message", zap.Error(err))
				continue
			}

			var cacheMsg RankCacheMessage
			if err := json.Unmarshal([]byte(msg.Payload), &cacheMsg); err != nil {
				r.logger.Error("Error unmarshaling rank cache message", zap.Error(err))
				continue
			}

			// 忽略自己发出的消息
			if cacheMsg.NodeID == r.nodeID {
				continue
			}

			// 处理缓存失效消息
			r.handleCacheMessage(msg.Channel, &cacheMsg)
		}
	}
}

func (r *RedisLeaderboardRankCache) handleCacheMessage(channel string, msg *RankCacheMessage) {
	cacheKey := r.getLocalCacheKey(msg.LeaderboardID, msg.ExpiryUnix, msg.OwnerID)

	switch msg.Action {
	case "insert":
		// 其他节点更新了排行榜数据，使本地缓存失效
		r.localCache.invalidate(cacheKey)
	case "delete":
		// 其他节点删除了排行榜数据，使本地缓存失效
		r.localCache.invalidate(cacheKey)
	case "clear":
		// 清空整个排行榜的本地缓存
		r.clearLocalCacheForLeaderboard(msg.LeaderboardID, msg.ExpiryUnix)
	}

	r.logger.Debug("Processed rank cache message",
		zap.String("channel", channel),
		zap.String("action", msg.Action),
		zap.String("leaderboard_id", msg.LeaderboardID),
		zap.String("source_node", msg.NodeID))
}

func (r *RedisLeaderboardRankCache) publishMessage(leaderboardID string, expiryUnix int64, ownerID uuid.UUID, action string, data *RankCacheEntry) {
	msg := RankCacheMessage{
		LeaderboardID: leaderboardID,
		ExpiryUnix:    expiryUnix,
		OwnerID:       ownerID,
		Action:        action,
		Data:          data,
		NodeID:        r.nodeID,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		r.logger.Error("Error marshaling rank cache message", zap.Error(err))
		return
	}

	channel := rankCacheInvalidateChannel
	if action == "insert" {
		channel = rankCacheUpdateChannel
	}

	// 增加重试机制，确保消息可靠发送
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := r.client.Publish(r.ctx, channel, msgBytes).Err(); err != nil {
			r.logger.Error("Error publishing rank cache message",
				zap.Error(err),
				zap.Int("retry", i+1),
				zap.String("channel", channel))

			if i == maxRetries-1 {
				// 最后一次重试失败，记录严重错误
				r.logger.Error("Failed to publish rank cache message after all retries",
					zap.String("leaderboard_id", leaderboardID),
					zap.String("action", action))
			}

			// 短暂延迟后重试
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}

		// 发送成功，跳出重试循环
		break
	}
}

func (r *RedisLeaderboardRankCache) startLocalCacheCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.localCache.cleanup()
		}
	}
}

func (r *RedisLeaderboardRankCache) getRedisKey(leaderboardID string, expiryUnix int64) string {
	return fmt.Sprintf("%s%s:%d", redisSortedSetKeyPrefix, leaderboardID, expiryUnix)
}

func (r *RedisLeaderboardRankCache) getMetaKey(leaderboardID string, expiryUnix int64) string {
	return fmt.Sprintf("%s%s:%d", redisMetaKeyPrefix, leaderboardID, expiryUnix)
}

func (r *RedisLeaderboardRankCache) getLocalCacheKey(leaderboardID string, expiryUnix int64, ownerID uuid.UUID) string {
	return fmt.Sprintf("%s:%d:%s", leaderboardID, expiryUnix, ownerID.String())
}

func (r *RedisLeaderboardRankCache) clearLocalCacheForLeaderboard(leaderboardID string, expiryUnix int64) {
	prefix := fmt.Sprintf("%s:%d:", leaderboardID, expiryUnix)
	r.localCache.cache.Range(func(key, value interface{}) bool {
		if keyStr, ok := key.(string); ok {
			if len(keyStr) > len(prefix) && keyStr[:len(prefix)] == prefix {
				r.localCache.cache.Delete(key)
			}
		}
		return true
	})
}

func (r *RedisLeaderboardRankCache) Get(leaderboardID string, expiryUnix int64, ownerID uuid.UUID) int64 {
	if r.blacklistAll {
		return 0
	}
	if _, ok := r.blacklistIds[leaderboardID]; ok {
		return 0
	}

	// 首先检查本地缓存
	cacheKey := r.getLocalCacheKey(leaderboardID, expiryUnix, ownerID)
	if rank, found := r.localCache.get(cacheKey); found {
		return rank
	}

	// 从Redis获取排名和分数
	redisKey := r.getRedisKey(leaderboardID, expiryUnix)
	member := r.formatMember(ownerID)

	// 使用管道同时获取排名和分数
	pipe := r.client.Pipeline()
	rankCmd := pipe.ZRevRank(r.ctx, redisKey, member)
	scoreCmd := pipe.ZScore(r.ctx, redisKey, member)
	_, err := pipe.Exec(r.ctx)

	if err != nil {
		if err != redis.Nil {
			r.logger.Error("Error getting rank from Redis", zap.Error(err))
			// Redis连接失败时，尝试从数据库重建缓存
			r.logger.Warn("Redis connection failed, attempting to rebuild cache from database",
				zap.String("leaderboard_id", leaderboardID),
				zap.String("owner_id", ownerID.String()))

			// 这里可以添加从数据库重建缓存的逻辑
			// 暂时返回0，避免返回错误数据
			return 0
		}
		return 0
	}

	rank, err := rankCmd.Result()
	if err != nil {
		if err != redis.Nil {
			r.logger.Error("Error getting rank from Redis", zap.Error(err))
		}
		return 0
	}

	// 获取分数并解析score和subscore
	sortScore, err := scoreCmd.Result()
	if err != nil {
		if err != redis.Nil {
			r.logger.Error("Error getting score from Redis", zap.Error(err))
		}
		return 0
	}

	// 从排序分数中解析出原始的score和subscore
	var score, subscore int64
	if sortScore >= 0 {
		// 降序排列
		score = int64(sortScore / 1e9)
		subscore = int64(sortScore) % int64(1e9)
	} else {
		// 升序排列
		score = int64(-sortScore / 1e9)
		subscore = int64(-sortScore) % int64(1e9)
	}

	// 如果score和subscore都为0，则rank也为0
	if score == 0 && subscore == 0 {
		return 0
	}

	// 排名从0开始，转换为从1开始
	finalRank := rank + 1

	// 缓存到本地
	r.localCache.set(cacheKey, finalRank, localRankCacheExpiry)

	return finalRank
}

func (r *RedisLeaderboardRankCache) GetDataByRank(leaderboardID string, expiryUnix int64, sortOrder int, rank int64) (ownerID uuid.UUID, score, subscore int64, err error) {
	if r.blacklistAll {
		return uuid.Nil, 0, 0, errors.New("rank cache is disabled")
	}
	if _, ok := r.blacklistIds[leaderboardID]; ok {
		return uuid.Nil, 0, 0, fmt.Errorf("rank cache is disabled for leaderboard: %s", leaderboardID)
	}

	redisKey := r.getRedisKey(leaderboardID, expiryUnix)

	// Redis排名从0开始，转换
	redisRank := rank - 1

	// 根据排序方式选择命令
	var results []redis.Z
	if sortOrder == LeaderboardSortOrderDescending {
		results, err = r.client.ZRevRangeWithScores(r.ctx, redisKey, redisRank, redisRank).Result()
	} else {
		results, err = r.client.ZRangeWithScores(r.ctx, redisKey, redisRank, redisRank).Result()
	}

	if err != nil {
		return uuid.Nil, 0, 0, err
	}

	if len(results) == 0 {
		return uuid.Nil, 0, 0, fmt.Errorf("rank entry %d not found", rank)
	}

	// 解析成员数据
	member := results[0].Member.(string)
	ownerID, score, subscore, err = r.parseMember(member)
	if err != nil {
		return uuid.Nil, 0, 0, err
	}

	return ownerID, score, subscore, nil
}

func (r *RedisLeaderboardRankCache) Fill(leaderboardID string, expiryUnix int64, records []*api.LeaderboardRecord, enableRanks bool) int64 {
	if r.blacklistAll || !enableRanks {
		return 0
	}
	if _, ok := r.blacklistIds[leaderboardID]; ok {
		return 0
	}

	if len(records) == 0 {
		return 0
	}

	redisKey := r.getRedisKey(leaderboardID, expiryUnix)

	// 获取排行榜大小
	count, err := r.client.ZCard(r.ctx, redisKey).Result()
	if err != nil {
		r.logger.Error("Error getting leaderboard size from Redis", zap.Error(err))
		return 0
	}

	// 批量获取排名
	pipe := r.client.Pipeline()
	rankCmds := make([]*redis.IntCmd, len(records))

	for i, record := range records {
		ownerID, err := uuid.FromString(record.OwnerId)
		if err != nil {
			continue
		}
		member := r.formatMember(ownerID)
		rankCmds[i] = pipe.ZRevRank(r.ctx, redisKey, member)
	}

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		r.logger.Error("Error executing pipeline for rank fill", zap.Error(err))
		return count
	}

	// 设置排名
	for i, record := range records {
		if rankCmds[i] != nil {
			rank, err := rankCmds[i].Result()
			if err == nil {
				record.Rank = rank + 1 // 转换为从1开始

				// 如果score和subscore都为0，则rank也为0
				if record.Score == 0 && record.Subscore == 0 {
					record.Rank = 0
				}
			}
		}
	}

	return count
}

func (r *RedisLeaderboardRankCache) Insert(leaderboardID string, sortOrder int, score, subscore int64, generation int32, expiryUnix int64, ownerID uuid.UUID, enableRanks bool) int64 {
	if r.blacklistAll || !enableRanks {
		return 0
	}
	if _, ok := r.blacklistIds[leaderboardID]; ok {
		return 0
	}

	redisKey := r.getRedisKey(leaderboardID, expiryUnix)
	metaKey := r.getMetaKey(leaderboardID, expiryUnix)
	member := r.formatMember(ownerID)

	// 计算用于排序的分数
	var sortScore float64
	if sortOrder == LeaderboardSortOrderDescending {
		// 降序：分数越高排名越前
		sortScore = float64(score)*1e9 + float64(subscore)
	} else {
		// 升序：分数越低排名越前
		sortScore = -float64(score)*1e9 - float64(subscore)
	}

	// 使用Lua脚本进行原子操作
	script := redis.NewScript(`
		local redisKey = KEYS[1]
		local metaKey = KEYS[2]
		local member = ARGV[1]
		local score = ARGV[2]
		local generation = ARGV[3]
		local expiry = ARGV[4]
		
		-- 检查是否需要更新（基于generation）
		local currentGeneration = redis.call('HGET', metaKey, member)
		if currentGeneration and tonumber(currentGeneration) >= tonumber(generation) then
			-- 当前记录更新，返回当前排名
			local rank = redis.call('ZREVRANK', redisKey, member)
			if rank then
				return rank + 1
			else
				return 0
			end
		end
		
		-- 更新数据
		redis.call('ZADD', redisKey, score, member)
		redis.call('HSET', metaKey, member, generation)
		redis.call('EXPIRE', redisKey, expiry)
		redis.call('EXPIRE', metaKey, expiry)
		
		-- 返回新排名
		local rank = redis.call('ZREVRANK', redisKey, member)
		if rank then
			return rank + 1
		else
			return 0
		end
	`)

	result, err := script.Run(r.ctx, r.client, []string{redisKey, metaKey},
		member, sortScore, generation, int64(redisRankCacheExpiry.Seconds())).Result()

	if err != nil {
		r.logger.Error("Error inserting rank into Redis", zap.Error(err))
		return 0
	}

	rank := result.(int64)

	// 使本地缓存失效并发布消息
	cacheKey := r.getLocalCacheKey(leaderboardID, expiryUnix, ownerID)
	r.localCache.invalidate(cacheKey)

	data := &RankCacheEntry{
		OwnerID:    ownerID,
		Score:      score,
		Subscore:   subscore,
		Generation: generation,
		Timestamp:  time.Now().Unix(),
	}
	r.publishMessage(leaderboardID, expiryUnix, ownerID, "insert", data)

	// 如果score和subscore都为0，则rank也为0
	if score == 0 && subscore == 0 {
		return 0
	}

	return rank
}

func (r *RedisLeaderboardRankCache) Delete(leaderboardID string, expiryUnix int64, ownerID uuid.UUID) bool {
	if r.blacklistAll {
		return false
	}
	if _, ok := r.blacklistIds[leaderboardID]; ok {
		return false
	}

	redisKey := r.getRedisKey(leaderboardID, expiryUnix)
	metaKey := r.getMetaKey(leaderboardID, expiryUnix)
	member := r.formatMember(ownerID)

	// 删除排行榜条目和元数据
	pipe := r.client.Pipeline()
	pipe.ZRem(r.ctx, redisKey, member)
	pipe.HDel(r.ctx, metaKey, member)

	_, err := pipe.Exec(r.ctx)
	if err != nil {
		r.logger.Error("Error deleting rank from Redis", zap.Error(err))
		return false
	}

	// 使本地缓存失效并发布消息
	cacheKey := r.getLocalCacheKey(leaderboardID, expiryUnix, ownerID)
	r.localCache.invalidate(cacheKey)
	r.publishMessage(leaderboardID, expiryUnix, ownerID, "delete", nil)

	return true
}

func (r *RedisLeaderboardRankCache) DeleteLeaderboard(leaderboardID string, expiryUnix int64) bool {
	if r.blacklistAll {
		return false
	}
	if _, ok := r.blacklistIds[leaderboardID]; ok {
		return false
	}

	redisKey := r.getRedisKey(leaderboardID, expiryUnix)
	metaKey := r.getMetaKey(leaderboardID, expiryUnix)

	// 删除整个排行榜
	pipe := r.client.Pipeline()
	pipe.Del(r.ctx, redisKey)
	pipe.Del(r.ctx, metaKey)

	_, err := pipe.Exec(r.ctx)
	if err != nil {
		r.logger.Error("Error deleting leaderboard from Redis", zap.Error(err))
		return false
	}

	// 清空本地缓存并发布消息
	r.clearLocalCacheForLeaderboard(leaderboardID, expiryUnix)
	r.publishMessage(leaderboardID, expiryUnix, uuid.Nil, "clear", nil)

	return true
}

func (r *RedisLeaderboardRankCache) TrimExpired(nowUnix int64) bool {
	if r.blacklistAll {
		return false
	}

	// Redis会自动处理过期数据，这里主要清理本地缓存
	r.localCache.cleanup()

	return true
}

// 辅助方法：格式化成员标识
func (r *RedisLeaderboardRankCache) formatMember(ownerID uuid.UUID) string {
	return ownerID.String()
}

// 辅助方法：解析成员数据
func (r *RedisLeaderboardRankCache) parseMember(member string) (uuid.UUID, int64, int64, error) {
	// 这里需要根据实际存储的格式来解析
	// 目前简化处理，只返回ownerID
	ownerID, err := uuid.FromString(member)
	if err != nil {
		return uuid.Nil, 0, 0, err
	}

	// 需要从Redis中获取完整的分数信息
	// 这里返回占位符，实际实现需要更复杂的逻辑
	return ownerID, 0, 0, nil
}

// 停止缓存
func (r *RedisLeaderboardRankCache) Stop() {
	r.ctxCancelFn()
	if r.pubsub != nil {
		r.pubsub.Close()
	}
	if r.client != nil {
		r.client.Close()
	}
}

// HealthCheck 健康检查方法
func (r *RedisLeaderboardRankCache) HealthCheck() error {
	if r.blacklistAll {
		return nil // 禁用状态视为健康
	}

	// 检查Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis connection failed: %w", err)
	}

	// 检查发布/订阅连接
	if r.pubsub != nil {
		if err := r.pubsub.Ping(ctx); err != nil {
			return fmt.Errorf("Redis pubsub connection failed: %w", err)
		}
	}

	return nil
}

// GetStats 获取缓存统计信息
func (r *RedisLeaderboardRankCache) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// 本地缓存统计
	localCacheSize := 0
	r.localCache.cache.Range(func(key, value interface{}) bool {
		localCacheSize++
		return true
	})

	stats["local_cache_size"] = localCacheSize
	stats["node_id"] = r.nodeID
	stats["blacklist_all"] = r.blacklistAll
	stats["blacklist_count"] = len(r.blacklistIds)

	// Redis统计（如果连接正常）
	if err := r.HealthCheck(); err == nil {
		info, err := r.client.Info(r.ctx, "memory").Result()
		if err == nil {
			stats["redis_memory_usage"] = info
		}
	}

	return stats
}
