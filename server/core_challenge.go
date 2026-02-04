package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/heroiclabs/nakama/v3/internal/cronexpr"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// 在多节点下安全地为指定 challenge 查找一个可加入的 tournament；
// 若不存在，则在事务内（持有事务级顾问锁）创建一个新的 tournament 并返回其 ID。
// 仅在“创建路径”上串行化，避免影响“直接加入”的高并发性能。
func ChallengeCreateAndJoin(ctx context.Context, logger *zap.Logger, db *sql.DB, cache LeaderboardCache, challengeID int32, challengeName string, startTime, endTime time.Time, maxParticipants int32) (string, error) {
	var tournamentID string
	var created bool
	// For cache insertion after commit (newly created)
	var createdAuthoritative bool = false
	var createdSortOrder int = LeaderboardSortOrderDescending
	var createdOperator int = LeaderboardOperatorBest
	var createdResetSchedule string = ""
	var createdMetadata string = "{}"
	var createdTitle string
	var createdDescription string
	var createdCategory int = int(challengeID)
	var createdDuration int = int(endTime.Sub(startTime).Seconds())
	var createdMaxSize int = int(maxParticipants)
	var createdMaxNumScore int = 1000
	var createdJoinRequired bool = true
	var createdEnableRanks bool = true
	var createdCreateTime pgtype.Timestamptz
	var createdStartTime int64
	var createdEndTime int64

	err := ExecuteInTx(ctx, db, func(tx *sql.Tx) error {
		// 1) 事务级顾问锁，按 challengeID 串行化创建窗口
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(challengeID)); err != nil {
			return err
		}

		// 2) 锁内复查是否已有可加入的 tournament（避免重复创建）
		row := tx.QueryRowContext(ctx, `
            SELECT id
            FROM leaderboard
            WHERE duration > 0
              AND category = $1
              AND start_time <= NOW()
              AND end_time > NOW()
              AND size < max_size
            ORDER BY create_time ASC
            LIMIT 1
        `, int(challengeID))
		var existingID sql.NullString
		_ = row.Scan(&existingID)
		if existingID.Valid {
			tournamentID = existingID.String
			created = false
			return nil
		}

		// 3) 获取下一个批次号（事务内原子递增）
		var nextBatch int32
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO challenge_batch (challenge_id, current_batch)
			VALUES ($1, 1)
			ON CONFLICT (challenge_id)
			DO UPDATE SET
				current_batch = challenge_batch.current_batch + 1,
				updated_at = NOW()
			RETURNING current_batch
        `, challengeID).Scan(&nextBatch); err != nil {
			logger.Error("获取事务内挑战赛批次号失败",
				zap.Int32("challenge_id", challengeID),
				zap.Error(err))
			return err
		}

		// 4) 生成唯一 tournamentID
		newID := fmt.Sprintf("challenge_%d_%d", challengeID, nextBatch)

		// 5) 在事务内创建 Tournament 记录（与 CreateNewChallengeTournament 语义一致）
		createdTitle = fmt.Sprintf("%s - 第%d轮", challengeName, nextBatch)
		createdDescription = fmt.Sprintf("挑战赛 %s 的竞标赛，由服务器自动创建和管理", challengeName)
		createdStartTime = startTime.UTC().Unix()
		createdEndTime = endTime.UTC().Unix()

		if err := tx.QueryRowContext(ctx, `
            INSERT INTO leaderboard (
                id, authoritative, sort_order, operator, metadata, category, description,
                duration, end_time, title, start_time, max_size, max_num_score, join_required, enable_ranks
            ) VALUES (
                $1, $2, $3, $4, $5, $6, $7,
                $8, $9, $10, $11, $12, $13, $14, $15
            )
            RETURNING create_time
        `,
			newID, createdAuthoritative, createdSortOrder, createdOperator, createdMetadata, createdCategory, createdDescription,
			createdDuration, time.Unix(createdEndTime, 0).UTC(), createdTitle, time.Unix(createdStartTime, 0).UTC(), createdMaxSize, createdMaxNumScore, createdJoinRequired, createdEnableRanks,
		).Scan(&createdCreateTime); err != nil {
			return err
		}

		tournamentID = newID
		created = true
		return nil
	})
	if err != nil {
		return "", err
	}

	// 事务提交后，确保本节点缓存可见：
	if created {
		cache.InsertTournament(
			tournamentID,
			createdAuthoritative,
			createdSortOrder,
			createdOperator,
			createdResetSchedule,
			createdMetadata,
			createdTitle,
			createdDescription,
			createdCategory,
			createdDuration,
			createdMaxSize,
			createdMaxNumScore,
			createdJoinRequired,
			createdCreateTime.Time.Unix(),
			createdStartTime,
			createdEndTime,
			createdEnableRanks,
		)
		// 集群同步：若启用了 Redis 包装，则主动广播 create_tournament 消息
		if rc, ok := cache.(*RedisLeaderboardCache); ok {
			msg := &leaderboardCacheSyncMsg{
				Type:          LeaderboardCacheMsgTypeCreateTournament,
				NodeID:        rc.nodeID,
				ID:            tournamentID,
				Authoritative: createdAuthoritative,
				SortOrder:     createdSortOrder,
				Operator:      createdOperator,
				ResetSchedule: createdResetSchedule,
				Metadata:      createdMetadata,
				CreateTime:    createdCreateTime.Time.Unix(),
				EnableRanks:   createdEnableRanks,
				Category:      createdCategory,
				Description:   createdDescription,
				Duration:      createdDuration,
				EndTime:       createdEndTime,
				JoinRequired:  createdJoinRequired,
				MaxSize:       createdMaxSize,
				MaxNumScore:   createdMaxNumScore,
				Title:         createdTitle,
				StartTime:     createdStartTime,
			}
			rc.publishSyncMessage(msg)
		}

	} else {
		// 非本节点新建：完全依赖创建节点广播同步缓存；不做本地填充
	}

	return tournamentID, nil
}

// 快速查找第一个可加入的tournament
func ChallengeFindAvailable(ctx context.Context, logger *zap.Logger, db *sql.DB, leaderboardCache LeaderboardCache, category int) (*api.Tournament, error) {
	// 改进查询：添加时间窗口检查和更严格的并发控制
	now := time.Now().UTC()
	nowUnix := now.Unix()

	query := `
SELECT id, sort_order, operator, reset_schedule, metadata, create_time,
       category, description, duration, end_time, max_size, max_num_score,
       title, size, start_time
FROM leaderboard
WHERE duration > 0
  AND category = $1
  AND start_time <= $2
  AND end_time > $3
  AND size < max_size`

	params := []interface{}{
		category,
		now, // start_time <= now
		now, // end_time > now
	}

	// 优先选择更空的房间，其次按创建时间早的优先
	query += " ORDER BY size ASC, create_time ASC LIMIT 1"

	row := db.QueryRowContext(ctx, query, params...)
	tournament, err := parseTournament(row, time.Now().UTC())
	if err != nil {
		if errors.Is(err, runtime.ErrTournamentNotFound) || errors.Is(err, sql.ErrNoRows) {
			logger.Debug("没有找到可加入的tournament",
				zap.Int("category", category),
				zap.Int64("current_time", nowUnix))
			return nil, nil // 没有找到可加入的tournament
		}
		logger.Error("Error finding first available tournament", zap.Error(err))
		return nil, err
	}

	logger.Debug("快速查找到第一个可加入的tournament",
		zap.Int("category", category),
		zap.String("tournament_id", tournament.Id),
		zap.Uint32("current_size", tournament.Size),
		zap.Uint32("max_size", tournament.MaxSize),
		zap.Int64("start_time", tournament.StartTime.Seconds),
		zap.Int64("end_time", tournament.EndTime.Seconds))

	return tournament, nil
}

// 在单个事务内（持有 challengeID 的事务级顾问锁）完成：
// 1) 查找可加入的 tournament（size < max_size），若无则创建
// 2) 将玩家加入该 tournament（原子插入记录并递增 size）
// 不依赖本地缓存，不做客户端重试，完全依赖数据库串行化保证跨节点一致。
func ChallengeJoin(ctx context.Context, logger *zap.Logger, db *sql.DB, cache LeaderboardCache, rankCache LeaderboardRankCache, userID uuid.UUID, username string, challengeID int32, challengeName string,
	startTime, endTime time.Time,
	maxParticipants int32,
) (string, error) {
	var selectedTournamentID string
	var created bool
	var createdCreateTime pgtype.Timestamptz
	var createdAuthoritative bool = false
	var createdSortOrder int = LeaderboardSortOrderDescending
	var createdOperator int = LeaderboardOperatorBest
	var createdResetSchedule string = ""
	var createdMetadata string = "{}"
	var createdTitle string = fmt.Sprintf("%s - 动态房间", challengeName)
	var createdDescription string = fmt.Sprintf("挑战赛 %s 的竞标赛，由服务器自动创建和管理", challengeName)
	var createdCategory int = int(challengeID)
	var createdDuration int = int(endTime.Sub(startTime).Seconds())
	var createdMaxSize int = int(maxParticipants)
	var createdMaxNumScore int = 1000
	var createdJoinRequired bool = true
	var createdEnableRanks bool = true

	// 事务包裹 + 顾问锁串行化整个“选房/建房+加入”流程
	if err := ExecuteInTx(ctx, db, func(tx *sql.Tx) error {
		// 1) 对 challenge 维度加事务级顾问锁，串行化同一挑战的分配
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(challengeID)); err != nil {
			return err
		}

		// 2) 查找可加入的 tournament（更空优先，其次创建时间早）
		row := tx.QueryRowContext(ctx, `
            SELECT id
            FROM leaderboard
            WHERE duration > 0
              AND category = $1::int
              AND start_time <= NOW()
              AND end_time > NOW()
              AND size < max_size
            ORDER BY size ASC, create_time ASC
            LIMIT 1
        `, int(challengeID))
		var existingID sql.NullString
		_ = row.Scan(&existingID)
		if existingID.Valid {
			selectedTournamentID = existingID.String
			created = false
		} else {

			// 3) 获取下一个批次号（事务内原子递增）
			var nextBatch int32
			if err := tx.QueryRowContext(ctx, `
			INSERT INTO challenge_batch (challenge_id, current_batch)
			VALUES ($1, 1)
			ON CONFLICT (challenge_id)
			DO UPDATE SET
				current_batch = challenge_batch.current_batch + 1,
				updated_at = NOW()
			RETURNING current_batch
        `, challengeID).Scan(&nextBatch); err != nil {
				logger.Error("获取事务内挑战赛批次号失败",
					zap.Int32("challenge_id", challengeID),
					zap.Error(err))
				return err
			}

			// 4) 生成唯一 tournamentID
			newID := fmt.Sprintf("challenge_%d_%d_%d", challengeID, startTime.Unix(), nextBatch)

			if err := tx.QueryRowContext(ctx, `
                INSERT INTO leaderboard (
                    id, authoritative, sort_order, operator, metadata, category, description,
                    duration, end_time, title, start_time, max_size, max_num_score, join_required, enable_ranks
                ) VALUES (
                    $1, $2, $3, $4, $5, $6, $7,
                    $8, $9, $10, $11, $12, $13, $14, $15
                )
                RETURNING create_time
            `,
				newID, createdAuthoritative, createdSortOrder, createdOperator, createdMetadata, createdCategory, createdDescription,
				createdDuration, endTime.UTC(), createdTitle, startTime.UTC(), createdMaxSize, createdMaxNumScore, createdJoinRequired, createdEnableRanks,
			).Scan(&createdCreateTime); err != nil {
				return err
			}

			selectedTournamentID = newID
			created = true
		}

		// 4) 计算本期记录过期时间（用于记录表）
		// 复用 calculateTournamentDeadlines 逻辑获取 expiryUnix
		// 这里简化：从 leaderboard 表取 start_time/duration/reset_schedule 重新计算更严谨
		var dbStartTime pgtype.Timestamptz
		var dbEndTime pgtype.Timestamptz
		var dbDuration int
		var dbResetSchedule sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT start_time, end_time, duration, COALESCE(reset_schedule, '') FROM leaderboard WHERE id = $1::text`, selectedTournamentID).Scan(&dbStartTime, &dbEndTime, &dbDuration, &dbResetSchedule); err != nil {
			return err
		}
		var resetExpr *cronexpr.Expression
		if dbResetSchedule.Valid && dbResetSchedule.String != "" {
			if expr, e := cronexpr.Parse(dbResetSchedule.String); e == nil {
				resetExpr = expr
			}
		}
		now := time.Now().UTC()
		_, _, expiryUnix := calculateTournamentDeadlines(dbStartTime.Time.UTC().Unix(), dbEndTime.Time.UTC().Unix(), int64(dbDuration), resetExpr, now)
		expiryTime := time.Unix(expiryUnix, 0).UTC()

		// 5) 插入参赛记录（若已存在则忽略）
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO leaderboard_record (leaderboard_id, owner_id, expiry_time, username, num_score, max_num_score)
            SELECT $1::text, $2::uuid, $3::timestamptz, $4::text, 0, l.max_num_score FROM leaderboard l WHERE l.id = $1::text
            ON CONFLICT (owner_id, leaderboard_id, expiry_time) DO NOTHING
        `, selectedTournamentID, userID.String(), expiryTime, username); err != nil {
			return err
		}

		// 6) 尝试占用一个名额
		res, err := tx.ExecContext(ctx, `UPDATE leaderboard SET size = size + 1 WHERE id = $1::text AND (max_size = 0 OR size < max_size)`, selectedTournamentID)
		if err != nil {
			return err
		}
		if rows, _ := res.RowsAffected(); rows != 1 {
			// 房间刚满，返回满员错误
			return runtime.ErrTournamentMaxSizeReached
		}

		return nil
	}); err != nil {
		return "", err
	}

	// 提交后：若是新建房间，写入本地缓存（并依赖广播同步其它节点）
	if created {
		cache.InsertTournament(
			selectedTournamentID,
			createdAuthoritative,
			LeaderboardSortOrderDescending,
			LeaderboardOperatorBest,
			createdResetSchedule,
			createdMetadata,
			createdTitle,
			createdDescription,
			createdCategory,
			createdDuration,
			createdMaxSize,
			createdMaxNumScore,
			createdJoinRequired,
			createdCreateTime.Time.Unix(),
			startTime.UTC().Unix(),
			endTime.UTC().Unix(),
			createdEnableRanks,
		)
		// 集群同步：若启用了 Redis 包装，则主动广播 create_tournament 消息
		if rc, ok := cache.(*RedisLeaderboardCache); ok {
			msg := &leaderboardCacheSyncMsg{
				Type:          LeaderboardCacheMsgTypeCreateTournament,
				NodeID:        rc.nodeID,
				ID:            selectedTournamentID,
				Authoritative: createdAuthoritative,
				SortOrder:     createdSortOrder,
				Operator:      createdOperator,
				ResetSchedule: createdResetSchedule,
				Metadata:      createdMetadata,
				CreateTime:    createdCreateTime.Time.Unix(),
				EnableRanks:   createdEnableRanks,
				Category:      createdCategory,
				Description:   createdDescription,
				Duration:      createdDuration,
				EndTime:       endTime.UTC().Unix(),
				JoinRequired:  createdJoinRequired,
				MaxSize:       createdMaxSize,
				MaxNumScore:   createdMaxNumScore,
				Title:         createdTitle,
				StartTime:     startTime.UTC().Unix(),
			}
			rc.publishSyncMessage(msg)
		}
	}

	return selectedTournamentID, nil
}
