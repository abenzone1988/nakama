package server

import (
	"time"

	"github.com/heroiclabs/nakama/v3/game"
)

// 初始化关卡数据
func initializeLevelData(userMeta *game.UserMeta) {
	now := time.Now().Format(time.RFC3339)
	userMeta.Level = &game.LevelData{
		CurLevelId:             ZeroLevelId, // "L1000"
		BestProgress:           0,
		HasMoppingTimes:        0,
		HasMoppingTimesForAdv:  0,
		LastMoppingTimestamp:   now,
		LastGetOnHookTimestamp: now, // 初始化为当前时间
	}
}
