package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Leaderboard Cache 消息类型
const (
	LeaderboardCacheMsgTypeCreate           = "create"
	LeaderboardCacheMsgTypeDelete           = "delete"
	LeaderboardCacheMsgTypeCreateTournament = "create_tournament"
)

// leaderboardCacheSyncMsg 同步消息
type leaderboardCacheSyncMsg struct {
	Type          string `json:"type"`
	NodeID        string `json:"node_id"`
	ID            string `json:"id"`
	Authoritative bool   `json:"authoritative,omitempty"`
	SortOrder     int    `json:"sort_order,omitempty"`
	Operator      int    `json:"operator,omitempty"`
	ResetSchedule string `json:"reset_schedule,omitempty"`
	Metadata      string `json:"metadata,omitempty"`
	CreateTime    int64  `json:"create_time,omitempty"`
	EnableRanks   bool   `json:"enable_ranks,omitempty"`
	// Tournament specific fields
	Category     int    `json:"category,omitempty"`
	Description  string `json:"description,omitempty"`
	Duration     int    `json:"duration,omitempty"`
	EndTime      int64  `json:"end_time,omitempty"`
	JoinRequired bool   `json:"join_required,omitempty"`
	MaxSize      int    `json:"max_size,omitempty"`
	MaxNumScore  int    `json:"max_num_score,omitempty"`
	Title        string `json:"title,omitempty"`
	StartTime    int64  `json:"start_time,omitempty"`
}

// RedisLeaderboardCache 基于 Redis 同步的排行榜缓存实现（包装本地缓存）
type RedisLeaderboardCache struct {
	ctx          context.Context
	logger       *zap.Logger
	inner        LeaderboardCache
	redisClient  *redis.Client
	redisPubSub  *redis.PubSub
	redisChannel string
	nodeID       string
}

// NewRedisLeaderboardCache 创建包装实现
func NewRedisLeaderboardCache(ctx context.Context, logger, startupLogger *zap.Logger, db *sql.DB, config Config) LeaderboardCache {
	inner := &LocalLeaderboardCache{
		ctx:          ctx,
		logger:       logger,
		db:           db,
		leaderboards: make(map[string]*Leaderboard),

		allList:         make([]*Leaderboard, 0),
		leaderboardList: make([]*Leaderboard, 0),
		tournamentList:  make([]*Leaderboard, 0),
	}

	if err := inner.RefreshAllLeaderboards(ctx); err != nil {
		startupLogger.Fatal("Error loading leaderboard cache from database", zap.Error(err))
	}

	c := &RedisLeaderboardCache{
		ctx:          ctx,
		logger:       logger,
		inner:        inner,
		nodeID:       uuid.Must(uuid.NewV4()).String(),
		redisChannel: "nakama:leaderboard_cache:sync",
		redisClient: redis.NewClient(&redis.Options{
			Addr:            config.GetCluster().RedisAddress,
			Password:        config.GetCluster().RedisPassword,
			DB:              1,
			DialTimeout:     5 * time.Second,
			ReadTimeout:     0,
			WriteTimeout:    5 * time.Second,
			MaxRetries:      3,
			MinRetryBackoff: 100 * time.Millisecond,
			MaxRetryBackoff: 2 * time.Second,
			PoolSize:        10,
		}),
	}

	if err := c.redisClient.Ping(ctx).Err(); err != nil {
		startupLogger.Error("Failed to connect to Redis for leaderboard cache",
			zap.Error(err),
			zap.String("redis_address", config.GetCluster().RedisAddress))
		return c
	}

	c.redisPubSub = c.redisClient.Subscribe(ctx, c.redisChannel)
	startupLogger.Info("Leaderboard cache cluster sync via Redis enabled",
		zap.String("node_id", c.nodeID),
		zap.String("redis_address", config.GetCluster().RedisAddress),
		zap.String("channel", c.redisChannel))

	go c.subscribeRedisMessages()
	go c.redisPingLoop()
	return c
}

// 接口实现（读操作转发）
func (c *RedisLeaderboardCache) Get(id string) *Leaderboard { return c.inner.Get(id) }

func (c *RedisLeaderboardCache) ListAll(limit int, reverse bool, cursor *LeaderboardAllCursor) ([]*Leaderboard, int, *LeaderboardAllCursor) {
	return c.inner.ListAll(limit, reverse, cursor)
}

func (c *RedisLeaderboardCache) RefreshAllLeaderboards(ctx context.Context) error {
	return c.inner.RefreshAllLeaderboards(ctx)
}

func (c *RedisLeaderboardCache) List(limit int, cursor *LeaderboardListCursor) ([]*Leaderboard, *LeaderboardListCursor, error) {
	return c.inner.List(limit, cursor)
}

func (c *RedisLeaderboardCache) ListTournaments(now int64, categoryStart, categoryEnd int, startTime, endTime int64, limit int, cursor *TournamentListCursor) ([]*Leaderboard, *TournamentListCursor, error) {
	return c.inner.ListTournaments(now, categoryStart, categoryEnd, startTime, endTime, limit, cursor)
}

// 接口实现（写操作：本地执行 + 发布同步）
func (c *RedisLeaderboardCache) Create(ctx context.Context, id string, authoritative bool, sortOrder, operator int, resetSchedule, metadata string, enableRanks bool) (*Leaderboard, bool, error) {
	lb, created, err := c.inner.Create(ctx, id, authoritative, sortOrder, operator, resetSchedule, metadata, enableRanks)
	if err != nil {
		return lb, created, err
	}
	if created {
		msg := &leaderboardCacheSyncMsg{
			Type:          LeaderboardCacheMsgTypeCreate,
			NodeID:        c.nodeID,
			ID:            id,
			Authoritative: authoritative,
			SortOrder:     sortOrder,
			Operator:      operator,
			ResetSchedule: resetSchedule,
			Metadata:      metadata,
			CreateTime:    lb.CreateTime,
			EnableRanks:   enableRanks,
		}
		c.publishSyncMessage(msg)
	}
	return lb, created, nil
}

func (c *RedisLeaderboardCache) Insert(id string, authoritative bool, sortOrder, operator int, resetSchedule, metadata string, createTime int64, enableRanks bool) {
	c.inner.Insert(id, authoritative, sortOrder, operator, resetSchedule, metadata, createTime, enableRanks)
}

func (c *RedisLeaderboardCache) CreateTournament(ctx context.Context, id string, authoritative bool, sortOrder, operator int, resetSchedule, metadata, title, description string, category, startTime, endTime, duration, maxSize, maxNumScore int, joinRequired, enableRanks bool) (*Leaderboard, bool, error) {
	lb, created, err := c.inner.CreateTournament(ctx, id, authoritative, sortOrder, operator, resetSchedule, metadata, title, description, category, startTime, endTime, duration, maxSize, maxNumScore, joinRequired, enableRanks)
	if err != nil {
		return lb, created, err
	}
	if created {
		msg := &leaderboardCacheSyncMsg{
			Type:          LeaderboardCacheMsgTypeCreateTournament,
			NodeID:        c.nodeID,
			ID:            id,
			Authoritative: authoritative,
			SortOrder:     sortOrder,
			Operator:      operator,
			ResetSchedule: resetSchedule,
			Metadata:      metadata,
			CreateTime:    lb.CreateTime,
			EnableRanks:   enableRanks,
			Category:      lb.Category,
			Description:   lb.Description,
			Duration:      lb.Duration,
			EndTime:       lb.EndTime,
			JoinRequired:  lb.JoinRequired,
			MaxSize:       lb.MaxSize,
			MaxNumScore:   lb.MaxNumScore,
			Title:         lb.Title,
			StartTime:     lb.StartTime,
		}
		c.publishSyncMessage(msg)
	}
	return lb, created, nil
}

func (c *RedisLeaderboardCache) InsertTournament(id string, authoritative bool, sortOrder, operator int, resetSchedule, metadata, title, description string, category, duration, maxSize, maxNumScore int, joinRequired bool, createTime, startTime, endTime int64, enableRanks bool) {
	c.inner.InsertTournament(id, authoritative, sortOrder, operator, resetSchedule, metadata, title, description, category, duration, maxSize, maxNumScore, joinRequired, createTime, startTime, endTime, enableRanks)
}

func (c *RedisLeaderboardCache) Delete(ctx context.Context, rankCache LeaderboardRankCache, scheduler LeaderboardScheduler, id string) (bool, error) {
	changed, err := c.inner.Delete(ctx, rankCache, scheduler, id)
	if err == nil && changed {
		msg := &leaderboardCacheSyncMsg{
			Type:   LeaderboardCacheMsgTypeDelete,
			NodeID: c.nodeID,
			ID:     id,
		}
		c.publishSyncMessage(msg)
	}
	return changed, err
}

func (c *RedisLeaderboardCache) Remove(id string) {
	c.inner.Remove(id)
	msg := &leaderboardCacheSyncMsg{
		Type:   LeaderboardCacheMsgTypeDelete,
		NodeID: c.nodeID,
		ID:     id,
	}
	c.publishSyncMessage(msg)
}

// 消息订阅与发布
func (c *RedisLeaderboardCache) subscribeRedisMessages() {
	backoff := 500 * time.Millisecond
	maxBackoff := 30 * time.Second
	for {
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
			}
		}

		if c.redisPubSub == nil {
			c.redisPubSub = c.redisClient.Subscribe(c.ctx, c.redisChannel)
		}

		for {
			if c.ctx != nil {
				select {
				case <-c.ctx.Done():
					return
				default:
				}
			}
			msg, err := c.redisPubSub.ReceiveMessage(c.ctx)
			if err != nil {
				c.logger.Warn("Redis PubSub receive error, reconnecting",
					zap.Error(err),
					zap.String("channel", c.redisChannel))
				break
			}
			var syncMsg leaderboardCacheSyncMsg
			if err := json.Unmarshal([]byte(msg.Payload), &syncMsg); err != nil {
				c.logger.Error("Failed to unmarshal leaderboard cache sync message",
					zap.Error(err),
					zap.String("payload", msg.Payload))
				continue
			}
			if syncMsg.NodeID == c.nodeID {
				continue
			}
			c.handleSyncMessage(&syncMsg)
		}

		if c.redisPubSub != nil {
			_ = c.redisPubSub.Close()
			c.redisPubSub = nil
		}
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (c *RedisLeaderboardCache) redisPingLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				return
			case <-t.C:
			}
		} else {
			<-t.C
		}
		if c.redisPubSub != nil {
			_ = c.redisPubSub.Ping(c.ctx)
			continue
		}
		if c.redisClient != nil {
			_ = c.redisClient.Ping(c.ctx).Err()
		}
	}
}

func (c *RedisLeaderboardCache) publishSyncMessage(msg *leaderboardCacheSyncMsg) {
	if c.redisClient == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal leaderboard cache sync message",
			zap.Error(err),
			zap.String("type", msg.Type),
			zap.String("leaderboard_id", msg.ID))
		return
	}
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := c.redisClient.Publish(c.ctx, c.redisChannel, data).Err(); err != nil {
			c.logger.Warn("Failed to publish leaderboard cache sync message",
				zap.Error(err),
				zap.String("type", msg.Type),
				zap.String("leaderboard_id", msg.ID),
				zap.Int("attempt", attempt))
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				continue
			}
		} else {
			break
		}
	}
}

func (c *RedisLeaderboardCache) handleSyncMessage(msg *leaderboardCacheSyncMsg) {
	c.logger.Info("Handling leaderboard cache sync message",
		zap.String("type", msg.Type),
		zap.String("from_node", msg.NodeID),
		zap.String("leaderboard_id", msg.ID))
	switch msg.Type {
	case LeaderboardCacheMsgTypeCreate:
		c.Insert(msg.ID, msg.Authoritative, msg.SortOrder, msg.Operator,
			msg.ResetSchedule, msg.Metadata, msg.CreateTime, msg.EnableRanks)
	case LeaderboardCacheMsgTypeDelete:
		c.Remove(msg.ID)
	case LeaderboardCacheMsgTypeCreateTournament:
		c.InsertTournament(msg.ID, msg.Authoritative, msg.SortOrder, msg.Operator,
			msg.ResetSchedule, msg.Metadata, msg.Title, msg.Description,
			msg.Category, msg.Duration, msg.MaxSize, msg.MaxNumScore,
			msg.JoinRequired, msg.CreateTime, msg.StartTime, msg.EndTime, msg.EnableRanks)
	default:
		c.logger.Warn("Unknown leaderboard cache sync message type",
			zap.String("type", msg.Type),
			zap.String("from_node", msg.NodeID))
	}
}
