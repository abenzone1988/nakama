package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	db           *sql.DB
	inner        LeaderboardCache
	redisClient  *redis.Client
	nodeID       string
	streamKey    string
	groupName    string
	consumerName string
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
		db:           db,
		inner:        inner,
		nodeID:       config.GetName(),
		streamKey:    "nakama:leaderboard_cache:stream",
		groupName:    "", // per-node group for fan-out
		consumerName: "",
		redisClient: redis.NewClient(&redis.Options{
			Addr:            config.GetCluster().RedisAddress,
			Password:        config.GetCluster().RedisPassword,
			DB:              config.GetCluster().RedisDB,
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

	// Initialize per-node consumer group to achieve broadcast (fan-out)
	c.consumerName = c.nodeID
	c.groupName = "nakama-leaderboard-sync:" + c.nodeID
	// Ensure stream group exists
	if err := c.redisClient.XGroupCreateMkStream(ctx, c.streamKey, c.groupName, "$").Err(); err != nil {
		// If group already exists, ignore; otherwise warn
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			startupLogger.Warn("Failed to create stream group (may already exist)", zap.Error(err), zap.String("stream", c.streamKey), zap.String("group", c.groupName))
		}
	}

	startupLogger.Info("Leaderboard cache stream fan-out via Redis Streams enabled",
		zap.String("node_id", c.nodeID),
		zap.String("redis_address", config.GetCluster().RedisAddress),
		zap.String("stream", c.streamKey),
		zap.String("group", c.groupName))

	go c.consumeStream()
	go c.redisPingLoop()
	return c
}

// 接口实现（读操作转发，带数据库兜底）
func (c *RedisLeaderboardCache) Get(id string) *Leaderboard {
	lb := c.inner.Get(id)
	if lb != nil {
		return lb
	}

	// 兜底：尝试从数据库加载（支持普通排行榜和锦标赛）
	lb = c.loadFromDB(id)
	return lb
}

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
func (c *RedisLeaderboardCache) consumeStream() {
	backoff := 500 * time.Millisecond
	maxBackoff := 30 * time.Second
	c.logger.Info("Leaderboard stream consumer started",
		zap.String("node_id", c.nodeID),
		zap.String("stream", c.streamKey),
		zap.String("group", c.groupName),
		zap.String("consumer", c.consumerName))
	for {
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
			}
		}

		// Blocking read from stream group
		res, err := c.redisClient.XReadGroup(c.ctx, &redis.XReadGroupArgs{
			Group:    c.groupName,
			Consumer: c.consumerName,
			Streams:  []string{c.streamKey, ">"},
			Count:    128,
			Block:    5 * time.Second,
			NoAck:    false,
		}).Result()
		if err != nil && err != redis.Nil {
			// If NOGROUP, (re)create group and retry quickly
			if strings.Contains(err.Error(), "NOGROUP") {
				if e := c.redisClient.XGroupCreateMkStream(c.ctx, c.streamKey, c.groupName, "$").Err(); e != nil {
					c.logger.Warn("Failed to create stream group on demand",
						zap.Error(e),
						zap.String("stream", c.streamKey),
						zap.String("group", c.groupName))
				} else {
					c.logger.Info("Created stream group on demand",
						zap.String("stream", c.streamKey),
						zap.String("group", c.groupName))
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			c.logger.Warn("Redis stream read error", zap.Error(err), zap.String("stream", c.streamKey))
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		backoff = 500 * time.Millisecond

		if len(res) == 0 {
			continue
		}
		total := 0
		for _, stream := range res {
			for _, m := range stream.Messages {
				total++
				// Extract payload
				var syncMsg leaderboardCacheSyncMsg
				if raw, ok := m.Values["payload"]; ok {
					switch v := raw.(type) {
					case string:
						_ = json.Unmarshal([]byte(v), &syncMsg)
					case []byte:
						_ = json.Unmarshal(v, &syncMsg)
					}
				} else {
					// Fallback: try assemble from fields
					if t, ok := m.Values["type"].(string); ok {
						syncMsg.Type = t
					}
					if nid, ok := m.Values["node_id"].(string); ok {
						syncMsg.NodeID = nid
					}
					if id, ok := m.Values["id"].(string); ok {
						syncMsg.ID = id
					}
				}

				c.logger.Info("Consumed leaderboard stream message",
					zap.String("msg_id", m.ID),
					zap.String("type", syncMsg.Type),
					zap.String("from_node", syncMsg.NodeID),
					zap.String("local_node", c.nodeID))

				if syncMsg.NodeID != c.nodeID {
					c.handleSyncMessage(&syncMsg)
				}

				// Acknowledge
				_ = c.redisClient.XAck(c.ctx, c.streamKey, c.groupName, m.ID).Err()
			}
		}
		c.logger.Info("Leaderboard stream batch processed",
			zap.Int("count", total),
			zap.String("group", c.groupName),
			zap.String("consumer", c.consumerName))
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
		if err := c.redisClient.XAdd(c.ctx, &redis.XAddArgs{
			Stream: c.streamKey,
			Values: map[string]any{
				"type":    msg.Type,
				"payload": string(data),
			},
		}).Err(); err != nil {
			c.logger.Warn("Failed to XADD leaderboard cache sync message",
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

// loadFromDB 从数据库加载 leaderboard 并填充到本地缓存（支持普通排行榜和锦标赛）
func (c *RedisLeaderboardCache) loadFromDB(id string) *Leaderboard {
	if c.db == nil {
		return nil
	}

	// 从数据库读取定义
	var dbAuthoritative bool
	var dbSortOrder int
	var dbOperator int
	var dbResetSchedule string
	var dbMetadata string
	var dbCreateTime time.Time
	var dbCategory int
	var dbDescription string
	var dbDuration int
	var dbEndTime sql.NullTime
	var dbJoinRequired bool
	var dbMaxSize int
	var dbMaxNumScore int
	var dbTitle string
	var dbStartTime time.Time
	var dbEnableRanks bool

	err := c.db.QueryRowContext(c.ctx, `
		SELECT authoritative, sort_order, operator, COALESCE(reset_schedule, ''), metadata, create_time,
		       category, description, duration, end_time, join_required, max_size, max_num_score, title, start_time, enable_ranks
		FROM leaderboard WHERE id = $1`, id).
		Scan(&dbAuthoritative, &dbSortOrder, &dbOperator, &dbResetSchedule, &dbMetadata, &dbCreateTime,
			&dbCategory, &dbDescription, &dbDuration, &dbEndTime, &dbJoinRequired, &dbMaxSize, &dbMaxNumScore, &dbTitle, &dbStartTime, &dbEnableRanks)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logger.Warn("Error loading leaderboard from database for cache fill",
				zap.String("leaderboard_id", id),
				zap.Error(err))
		}
		return nil
	}

	// 填充到本地缓存（不广播）
	if localCache, ok := c.inner.(*LocalLeaderboardCache); ok {
		var endUnix int64
		if dbEndTime.Valid {
			endUnix = dbEndTime.Time.Unix()
		}

		if dbDuration > 0 {
			// 锦标赛
			c.logger.Info("Loading tournament from database to local cache",
				zap.String("tournament_id", id),
				zap.String("title", dbTitle))
			localCache.insertTournamentLocal(
				id,
				dbAuthoritative,
				dbSortOrder,
				dbOperator,
				dbResetSchedule,
				dbMetadata,
				dbTitle,
				dbDescription,
				dbCategory,
				dbDuration,
				dbMaxSize,
				dbMaxNumScore,
				dbJoinRequired,
				dbCreateTime.Unix(),
				dbStartTime.Unix(),
				endUnix,
				dbEnableRanks,
			)
		} else {
			// 普通排行榜
			c.logger.Info("Loading leaderboard from database to local cache",
				zap.String("leaderboard_id", id))
			localCache.Insert(
				id,
				dbAuthoritative,
				dbSortOrder,
				dbOperator,
				dbResetSchedule,
				dbMetadata,
				dbCreateTime.Unix(),
				dbEnableRanks,
			)
		}
	}

	// 返回填充后的缓存对象
	return c.inner.Get(id)
}
