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

func (s *ApiServer) GetRestStation(ctx context.Context, in *emptypb.Empty) (*game.GetRestStationResponse, error) {
	ctx = LoadPlayerTimeZone(ctx, s.logger, s.db, s.statusRegistry)

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

func GetRestStationData(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (*game.RestStationData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		return nil, err
	}

	needUpdate := false
	currentCount := userMeta.RestStationStamina
	lastGrantTime := userMeta.RestStationLastGrantTime

	if lastGrantTime == "" {
		lastGrantTime = time.Now().UTC().Format(time.RFC3339)
		userMeta.RestStationLastGrantTime = lastGrantTime
		needUpdate = true
	}

	newCount, newGrantTime := calculateRestStationGrant(ctx, currentCount, lastGrantTime, logger)
	if newCount != currentCount {
		userMeta.RestStationStamina = newCount
		userMeta.RestStationLastGrantTime = newGrantTime
		needUpdate = true
	}

	if needUpdate {
		if err := SaveUserMeta(ctx, logger, db, userID, userMeta); err != nil {
			logger.Error("保存休息站数据失败", zap.Error(err))
			return nil, err
		}
		logger.Info("休息站体力已自动补充",
			zap.Int32("old_count", currentCount),
			zap.Int32("new_count", newCount))
	}

	nextGrantTime := calculateNextGrantTime(ctx)

	return &game.RestStationData{
		StaminaCount:  userMeta.RestStationStamina,
		NextGrantTime: nextGrantTime.UTC().Format(time.RFC3339),
	}, nil
}

func (s *ApiServer) ClaimRestStationStamina(ctx context.Context, in *game.ClaimRestStationStaminaRequest) (*game.ClaimRestStationStaminaResponse, error) {
	ctx = LoadPlayerTimeZone(ctx, s.logger, s.db, s.statusRegistry)

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	claimCount := in.GetCount()

	restData, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry)
	if err != nil {
		return &game.ClaimRestStationStaminaResponse{
			Code: 1,
			Msg:  "获取休息站数据失败",
		}, nil
	}

	if restData.StaminaCount <= 0 {
		return &game.ClaimRestStationStaminaResponse{
			Code: 2,
			Msg:  "休息站没有可领取的体力",
		}, nil
	}

	if claimCount == 0 {
		claimCount = restData.StaminaCount
	}

	if claimCount > restData.StaminaCount {
		return &game.ClaimRestStationStaminaResponse{
			Code: 3,
			Msg:  "领取数量超过可用数量",
		}, nil
	}

	staminaToAdd := claimCount * RestStationEveryStamina

	staminaData, err := RefillStamina(ctx, s.logger, s.db, s.statusRegistry, staminaToAdd)
	if err != nil {
		s.logger.Error("补充体力失败", zap.Error(err))
		return &game.ClaimRestStationStaminaResponse{
			Code: 4,
			Msg:  "补充体力失败",
		}, nil
	}

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

func calculateRestStationGrant(ctx context.Context, currentCount int32, lastGrantTimeStr string, logger *zap.Logger) (int32, string) {
	if currentCount >= RestStationMaxCount {
		return currentCount, lastGrantTimeStr
	}

	lastGrantTime, err := ParseTimeString(lastGrantTimeStr)
	if err != nil {
		logger.Error("解析时间失败", zap.Error(err), zap.String("time", lastGrantTimeStr))
		return currentCount, GetCurrentTimeUTC()
	}

	passedCount, latestPassedTime := CountPassedDailyTimes(ctx, lastGrantTime, RestStationDailyTimes)

	newCount := currentCount + passedCount
	if newCount > RestStationMaxCount {
		newCount = RestStationMaxCount
	}

	if passedCount > 0 {
		return newCount, FormatTimeToRFC3339(latestPassedTime)
	}

	return currentCount, lastGrantTimeStr
}

func calculateNextGrantTime(ctx context.Context) time.Time {
	return GetNextDailyTime(ctx, RestStationDailyTimes)
}
