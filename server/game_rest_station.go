package server

import (
	"context"
	"database/sql"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetRestStation 获取休息站状态
func (s *ApiServer) GetRestStation(ctx context.Context, in *emptypb.Empty) (*game.GetRestStationResponse, error) {
	data, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry)
	if err != nil {
		s.logger.Error("获取休息站数据失败", zap.Error(err))
		return &game.GetRestStationResponse{
			Code: 1,
			Msg:  "获取休息站数据失败",
		}, nil
	}

	return &game.GetRestStationResponse{
		Code: 0,
		Msg:  "成功",
		Data: data,
	}, nil
}

// GetRestStationData 获取休息站数据（内部函数）
func GetRestStationData(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (*game.RestStationData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载 UserMeta
	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		return nil, err
	}

	// 检查并自动补充体力
	needUpdate := false
	currentCount := userMeta.RestStationStamina
	lastGrantTime := userMeta.RestStationLastGrantTime

	// 如果是首次使用，初始化时间
	if lastGrantTime == "" {
		lastGrantTime = time.Now().Format(time.RFC3339)
		userMeta.RestStationLastGrantTime = lastGrantTime
		needUpdate = true
	}

	// 计算应该补充的体力
	newCount, newGrantTime := calculateRestStationGrant(currentCount, lastGrantTime, logger)
	if newCount != currentCount {
		userMeta.RestStationStamina = newCount
		userMeta.RestStationLastGrantTime = newGrantTime
		needUpdate = true
	}

	// 保存更新
	if needUpdate {
		if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
			logger.Error("保存休息站数据失败", zap.Error(err))
			return nil, err
		}
		logger.Info("休息站体力已自动补充",
			zap.Int32("old_count", currentCount),
			zap.Int32("new_count", newCount))
	}

	// 计算下次补充时间
	nextGrantTime := calculateNextGrantTime(newGrantTime)

	// 获取当前体力数据
	staminaData, err := GetCurrentStamina(ctx, logger, db, statusRegistry)
	if err != nil {
		return nil, err
	}

	return &game.RestStationData{
		StaminaCount:   userMeta.RestStationStamina,
		NextGrantTime:  nextGrantTime.UTC().Format(time.RFC3339),
		CurrentStamina: staminaData,
	}, nil
}

// ClaimRestStationStamina 领取休息站体力
func (s *ApiServer) ClaimRestStationStamina(ctx context.Context, in *game.ClaimRestStationStaminaRequest) (*game.ClaimRestStationStaminaResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	claimCount := in.GetCount()

	// 获取当前休息站数据
	restData, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry)
	if err != nil {
		return &game.ClaimRestStationStaminaResponse{
			Code: 1,
			Msg:  "获取休息站数据失败",
		}, nil
	}

	// 检查是否有体力可领取
	if restData.StaminaCount <= 0 {
		return &game.ClaimRestStationStaminaResponse{
			Code: 2,
			Msg:  "休息站没有可领取的体力",
		}, nil
	}

	// 如果 count 为 0，表示一键领取
	if claimCount == 0 {
		claimCount = restData.StaminaCount
	}

	// 检查领取数量是否合法
	if claimCount > restData.StaminaCount {
		return &game.ClaimRestStationStaminaResponse{
			Code: 3,
			Msg:  "领取数量超过可用数量",
		}, nil
	}

	// 计算实际补充的体力
	staminaToAdd := claimCount * RestStationEveryStamina

	// 补充体力到玩家
	staminaData, err := RefillStamina(ctx, s.logger, s.db, s.statusRegistry, staminaToAdd)
	if err != nil {
		s.logger.Error("补充体力失败", zap.Error(err))
		return &game.ClaimRestStationStaminaResponse{
			Code: 4,
			Msg:  "补充体力失败",
		}, nil
	}

	// 扣除休息站体力
	userMeta, _, err := LoadUserMeta(ctx, s.logger, s.db, s.statusRegistry, userID)
	if err != nil {
		return &game.ClaimRestStationStaminaResponse{
			Code: 1,
			Msg:  "获取用户数据失败",
		}, nil
	}

	userMeta.RestStationStamina -= claimCount
	if err := SaveUserMeta(ctx, s.logger, s.db, userID, userMeta); err != nil {
		s.logger.Error("保存休息站数据失败", zap.Error(err))
		return &game.ClaimRestStationStaminaResponse{
			Code: 5,
			Msg:  "保存数据失败",
		}, nil
	}

	// 获取更新后的休息站数据
	updatedRestData, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry)
	if err != nil {
		s.logger.Error("获取更新后的休息站数据失败", zap.Error(err))
	}

	s.logger.Info("领取休息站体力成功",
		zap.String("user_id", userID.String()),
		zap.Int32("claim_count", claimCount),
		zap.Int32("stamina_added", staminaToAdd))

	return &game.ClaimRestStationStaminaResponse{
		Code:        0,
		Msg:         "领取成功",
		RestStation: updatedRestData,
		Stamina:     staminaData,
	}, nil
}

// calculateRestStationGrant 计算自动补充的体力
// 返回：新的体力数量、新的补充时间
func calculateRestStationGrant(currentCount int32, lastGrantTimeStr string, logger *zap.Logger) (int32, string) {
	// 如果已经达到上限，不再补充
	if currentCount >= RestStationMaxCount {
		return currentCount, lastGrantTimeStr
	}

	lastGrantTime, err := time.Parse(time.RFC3339, lastGrantTimeStr)
	if err != nil {
		logger.Error("解析时间失败", zap.Error(err), zap.String("time", lastGrantTimeStr))
		return currentCount, time.Now().Format(time.RFC3339)
	}

	now := time.Now()
	newCount := currentCount
	latestGrantTime := lastGrantTime

	// 遍历每个补充时间点，检查是否应该补充
	for _, hour := range RestStationDailyTimes {
		// 从上次补充时间开始，检查每一天的补充时间点
		checkTime := time.Date(lastGrantTime.Year(), lastGrantTime.Month(), lastGrantTime.Day(),
			hour, 0, 0, 0, lastGrantTime.Location())

		// 如果这个时间点在上次补充之前，跳到第二天
		if checkTime.Before(lastGrantTime) || checkTime.Equal(lastGrantTime) {
			checkTime = checkTime.AddDate(0, 0, 1)
		}

		// 检查从上次补充到现在，经过了多少个这个时间点
		for checkTime.Before(now) || checkTime.Equal(now) {
			// 如果已经达到上限，停止补充
			if newCount >= RestStationMaxCount {
				break
			}

			newCount++
			latestGrantTime = checkTime
			checkTime = checkTime.AddDate(0, 0, 1) // 检查下一天的同一时间点
		}

		if newCount >= RestStationMaxCount {
			break
		}
	}

	// 如果有补充，更新时间为最后一次补充的时间
	if newCount != currentCount {
		return newCount, latestGrantTime.Format(time.RFC3339)
	}

	return currentCount, lastGrantTimeStr
}

// calculateNextGrantTime 计算下次补充时间
func calculateNextGrantTime(lastGrantTimeStr string) time.Time {
	now := time.Now()
	var nextTime time.Time

	// 找到下一个补充时间点
	for _, hour := range RestStationDailyTimes {
		checkTime := time.Date(now.Year(), now.Month(), now.Day(),
			hour, 0, 0, 0, now.Location())

		// 如果这个时间点还没到，就是下次补充时间
		if checkTime.After(now) {
			if nextTime.IsZero() || checkTime.Before(nextTime) {
				nextTime = checkTime
			}
		}
	}

	// 如果今天所有时间点都过了，取明天第一个时间点
	if nextTime.IsZero() {
		tomorrow := now.AddDate(0, 0, 1)
		nextTime = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
			RestStationDailyTimes[0], 0, 0, 0, tomorrow.Location())
	}

	return nextTime
}
