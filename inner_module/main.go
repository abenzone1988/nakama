package inner_module

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

var (
	errNoUserIdFound = runtime.NewError("no user ID in context", 1) // INVALID_ARGUMENT
)

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterAfterAuthenticateDevice(InitializeDeviceUser); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterAfterGetAccount(InitializeAccount); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	// 注册挑战赛Tournament结束回调
	if err := initializer.RegisterTournamentEnd(handleChallengetournamentEnd); err != nil {
		logger.Error("Failed to register tournament end callback: %v", err)
		return err
	}

	// 注册挑战赛Tournament重置回调
	if err := initializer.RegisterTournamentReset(handleChallengetournamentReset); err != nil {
		logger.Error("Failed to register tournament reset callback: %v", err)
		return err
	}

	logger.Info("Inner module initialized with challenge reward callbacks")
	return nil
}

// ChallengeTournamentMetadata 挑战赛竞标赛元数据结构
type ChallengeTournamentMetadata struct {
	ChallengeID     int32  `json:"challenge_id"`
	ChallengeType   string `json:"challenge_type"`
	CreatedBy       string `json:"created_by"`
	BatchNumber     int    `json:"batch_number"`
	StartTimestamp  int64  `json:"start_timestamp"`
	EndTimestamp    int64  `json:"end_timestamp"`
	MaxParticipants int32  `json:"max_participants"`
}

// handleChallengetournamentEnd 处理挑战赛竞标赛结束事件
func handleChallengetournamentEnd(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, tournament *api.Tournament, end, reset int64) error {
	// 检查是否是挑战赛竞标赛
	if !isChallengetournament(tournament) {
		// 不是挑战赛竞标赛，跳过处理
		return nil
	}

	logger.Info("Processing challenge tournament end", map[string]interface{}{
		"tournament_id": tournament.Id,
		"title":         tournament.Title,
		"end_time":      end,
		"reset_time":    reset,
	})

	// 解析竞标赛元数据
	var metadata ChallengeTournamentMetadata
	if err := json.Unmarshal([]byte(tournament.Metadata), &metadata); err != nil {
		logger.Error("Failed to parse tournament metadata", map[string]interface{}{
			"tournament_id": tournament.Id,
			"error":         err.Error(),
		})
		return nil // 不返回错误，避免影响其他竞标赛
	}

	// 获取竞标赛排行榜
	records, _, _, _, err := nk.TournamentRecordsList(ctx, tournament.Id, nil, 100, "", 0)
	if err != nil {
		logger.Error("Failed to get tournament records", map[string]interface{}{
			"tournament_id": tournament.Id,
			"error":         err.Error(),
		})
		return nil
	}

	logger.Info("Tournament records retrieved", map[string]interface{}{
		"tournament_id": tournament.Id,
		"records_count": len(records),
		"challenge_id":  metadata.ChallengeID,
		"batch_number":  metadata.BatchNumber,
	})

	// 处理奖励分发
	err = distributeChallengeRewards(ctx, nk, logger, tournament, metadata, records)
	if err != nil {
		logger.Error("Failed to distribute rewards", map[string]interface{}{
			"tournament_id": tournament.Id,
			"challenge_id":  metadata.ChallengeID,
			"error":         err.Error(),
		})
		return nil // 不返回错误，避免影响调度器
	}

	logger.Info("Challenge tournament rewards distributed successfully", map[string]interface{}{
		"tournament_id": tournament.Id,
		"challenge_id":  metadata.ChallengeID,
		"players_count": len(records),
	})

	return nil
}

// handleChallengetournamentReset 处理挑战赛竞标赛重置事件
func handleChallengetournamentReset(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, tournament *api.Tournament, end, reset int64) error {
	// 检查是否是挑战赛竞标赛
	if !isChallengetournament(tournament) {
		return nil
	}

	logger.Info("Processing challenge tournament reset", map[string]interface{}{
		"tournament_id": tournament.Id,
		"title":         tournament.Title,
		"end_time":      end,
		"reset_time":    reset,
	})

	// 挑战赛竞标赛通常不需要重置逻辑，因为它们有固定的生命周期
	// 这里可以添加清理逻辑或记录重置事件
	return nil
}

// isChallengetournament 检查是否是挑战赛竞标赛
func isChallengetournament(tournament *api.Tournament) bool {
	// 通过ID前缀判断
	if strings.HasPrefix(tournament.Id, "challenge_") {
		return true
	}

	// 通过元数据判断
	var metadata ChallengeTournamentMetadata
	if err := json.Unmarshal([]byte(tournament.Metadata), &metadata); err == nil {
		return metadata.ChallengeType == "server_managed" && metadata.CreatedBy == "server"
	}

	return false
}

// distributeChallengeRewards 分发挑战赛奖励
func distributeChallengeRewards(ctx context.Context, nk runtime.NakamaModule, logger runtime.Logger, tournament *api.Tournament, metadata ChallengeTournamentMetadata, records []*api.LeaderboardRecord) error {
	if len(records) == 0 {
		logger.Info("No players to reward in tournament: %s", tournament.Id)
		return nil
	}

	logger.Info("=== 开始分发挑战赛奖励 ===")
	logger.Info("竞标赛信息: ID=%s, 挑战赛ID=%d, 参与人数=%d", tournament.Id, metadata.ChallengeID, len(records))

	// 目前为空实现，仅用于测试和演示
	// 在这里可以实现具体的奖励分发逻辑

	// 示例：为前10名玩家分发不同的奖励
	for i, record := range records {
		rank := i + 1
		if rank > 10 { // 只奖励前10名
			break
		}

		// 计算奖励（示例逻辑）
		reward := calculateChallengeReward(rank, metadata.ChallengeID, len(records))

		logger.Info("🎁 奖励分发", map[string]interface{}{
			"tournament_id": tournament.Id,
			"challenge_id":  metadata.ChallengeID,
			"player_id":     record.OwnerId,
			"username":      record.Username,
			"rank":          rank,
			"score":         record.Score,
			"gold":          reward["gold"],
			"gems":          reward["gems"],
			"title":         reward["title"],
		})

		// TODO: 实际的奖励分发逻辑
		// 例如：发送邮件、增加货币、发放道具等
		// err := sendRewardToPlayer(ctx, nk, logger, record.OwnerId, reward)
		// if err != nil {
		//     logger.Error("Failed to send reward to player %s: %v", record.OwnerId, err)
		// }
	}

	logger.Info("=== 挑战赛奖励分发完成 ===")
	return nil
}

// calculateChallengeReward 计算挑战赛奖励（示例逻辑）
func calculateChallengeReward(rank int, challengeID int32, totalPlayers int) map[string]interface{} {
	reward := map[string]interface{}{
		"challenge_id":  challengeID,
		"rank":          rank,
		"total_players": totalPlayers,
	}

	// 根据排名分发不同奖励
	switch {
	case rank == 1:
		reward["gold"] = 1000
		reward["gems"] = 100
		reward["title"] = "🏆 冠军"
	case rank <= 3:
		reward["gold"] = 500
		reward["gems"] = 50
		reward["title"] = "🥉 精英"
	case rank <= 10:
		reward["gold"] = 200
		reward["gems"] = 20
		reward["title"] = "⭐ 竞争者"
	default:
		reward["gold"] = 50
		reward["gems"] = 5
		reward["title"] = "🎯 参与者"
	}

	return reward
}
