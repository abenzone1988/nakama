package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Leaderboard Cache 消息类型
const (
	LeaderboardCacheMsgTypeInsert           = "insert"
	LeaderboardCacheMsgTypeRemove           = "remove"
	LeaderboardCacheMsgTypeInsertTournament = "insert_tournament"
)

// leaderboardCacheSyncMsg 排行榜缓存同步消息
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

// initializeClusterMode 初始化 Redis 集群模式
func (l *LocalLeaderboardCache) initializeClusterMode(ctx context.Context, startupLogger *zap.Logger, config Config) {
	if !config.GetCluster().Enabled {
		startupLogger.Warn("Cluster mode requested for leaderboard cache but not enabled in config")
		return
	}

	l.nodeID = uuid.Must(uuid.NewV4()).String()
	l.redisChannel = "nakama:leaderboard_cache:sync"
	// 创建 Redis 客户端
	l.redisClient = redis.NewClient(&redis.Options{
		Addr:     config.GetCluster().RedisAddress,
		Password: config.GetCluster().RedisPassword,
		DB:       0,
	})

	if err := l.redisClient.Ping(ctx).Err(); err != nil {
		startupLogger.Error("Failed to connect to Redis for leaderboard cache",
			zap.Error(err),
			zap.String("redis_address", config.GetCluster().RedisAddress))
		return
	}
	// 订阅 Redis 频道
	l.redisPubSub = l.redisClient.Subscribe(ctx, l.redisChannel)
	l.clusterMode = true

	startupLogger.Info("Leaderboard cache cluster mode enabled",
		zap.String("node_id", l.nodeID),
		zap.String("redis_address", config.GetCluster().RedisAddress),
		zap.String("channel", l.redisChannel))

	// 启动消息订阅 goroutine
	go l.subscribeRedisMessages()
}

// subscribeRedisMessages 订阅 Redis Pub/Sub 消息
func (l *LocalLeaderboardCache) subscribeRedisMessages() {
	ch := l.redisPubSub.Channel()

	for msg := range ch {
		var syncMsg leaderboardCacheSyncMsg
		if err := json.Unmarshal([]byte(msg.Payload), &syncMsg); err != nil {
			l.logger.Error("Failed to unmarshal leaderboard cache sync message",
				zap.Error(err),
				zap.String("payload", msg.Payload))
			continue
		}

		// 过滤掉自己发送的消息
		if syncMsg.NodeID == l.nodeID {
			continue
		}
		l.handleSyncMessage(&syncMsg)
	}
}

// publishSyncMessage 发布同步消息到 Redis
func (l *LocalLeaderboardCache) publishSyncMessage(msg *leaderboardCacheSyncMsg) {
	data, err := json.Marshal(msg)
	if err != nil {
		l.logger.Error("Failed to marshal leaderboard cache sync message",
			zap.Error(err),
			zap.String("type", msg.Type),
			zap.String("leaderboard_id", msg.ID))
		return
	}
	// 重试机制
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := l.redisClient.Publish(l.ctx, l.redisChannel, data).Err(); err != nil {
			l.logger.Warn("Failed to publish leaderboard cache sync message",
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

// handleSyncMessage 处理收到的同步消息
func (l *LocalLeaderboardCache) handleSyncMessage(msg *leaderboardCacheSyncMsg) {
	switch msg.Type {
	case LeaderboardCacheMsgTypeInsert:
		// 插入或更新排行榜配置
		l.Insert(msg.ID, msg.Authoritative, msg.SortOrder, msg.Operator,
			msg.ResetSchedule, msg.Metadata, msg.CreateTime, msg.EnableRanks)
	case LeaderboardCacheMsgTypeRemove:
		// 删除排行榜配置（不发布消息，避免循环）
		l.removeWithoutPublish(msg.ID)
	case LeaderboardCacheMsgTypeInsertTournament:
		// 插入或更新锦标赛配置
		l.InsertTournament(msg.ID, msg.Authoritative, msg.SortOrder, msg.Operator,
			msg.ResetSchedule, msg.Metadata, msg.Title, msg.Description,
			msg.Category, msg.Duration, msg.MaxSize, msg.MaxNumScore,
			msg.JoinRequired, msg.CreateTime, msg.StartTime, msg.EndTime, msg.EnableRanks)
	default:
		l.logger.Warn("Unknown leaderboard cache sync message type",
			zap.String("type", msg.Type),
			zap.String("from_node", msg.NodeID))
	}
}

// removeWithoutPublish 删除排行榜但不发布同步消息（避免循环）
func (l *LocalLeaderboardCache) removeWithoutPublish(id string) {
	l.Lock()
	if leaderboard, ok := l.leaderboards[id]; ok {
		delete(l.leaderboards, id)
		for i, currentAll := range l.allList {
			if currentAll.Id == id {
				copy(l.allList[i:], l.allList[i+1:])
				l.allList[len(l.allList)-1] = nil
				l.allList = l.allList[:len(l.allList)-1]
				break
			}
		}
		if leaderboard.IsTournament() {
			for i, currentLeaderboard := range l.tournamentList {
				if currentLeaderboard.Id == id {
					copy(l.tournamentList[i:], l.tournamentList[i+1:])
					l.tournamentList[len(l.tournamentList)-1] = nil
					l.tournamentList = l.tournamentList[:len(l.tournamentList)-1]
					break
				}
			}
		} else {
			for i, currentLeaderboard := range l.leaderboardList {
				if currentLeaderboard.Id == id {
					copy(l.leaderboardList[i:], l.leaderboardList[i+1:])
					l.leaderboardList[len(l.leaderboardList)-1] = nil
					l.leaderboardList = l.leaderboardList[:len(l.leaderboardList)-1]
					break
				}
			}
		}
	}
	l.Unlock()
}
