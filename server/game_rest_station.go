package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/console"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *ApiServer) GetRestStation(ctx context.Context, in *emptypb.Empty) (*game.GetRestStationResponse, error) {
	ctx = LoadPlayerTimeZone(ctx, s.logger, s.db, s.statusRegistry)

	data, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry, s.tracker, s.router)
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

func GetRestStationData(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, tracker Tracker, router MessageRouter) (*game.RestStationData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	userMeta, _, err := LoadUserMeta(ctx, logger, db, statusRegistry, userID)
	if err != nil {
		return nil, err
	}

	needUpdate := false
	currentCount := userMeta.RestStationStamina
	lastGrantTime := userMeta.RestStationLastGrantTime

	if lastGrantTime.IsZero() {
		lastGrantTime = time.Now().UTC()
		userMeta.RestStationLastGrantTime = lastGrantTime
		needUpdate = true
	}

	newCount, newGrantTime := calculateRestStationGrant(ctx, currentCount, lastGrantTime, logger, db, tracker, router, userID)
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

	nextGrantTime := GetNextDailyTime(ctx, RestStationDailyTimes)

	return &game.RestStationData{
		StaminaCount:  userMeta.RestStationStamina,
		NextGrantTime: nextGrantTime.UTC().Format(time.RFC3339),
	}, nil
}

func (s *ApiServer) ClaimRestStationStamina(ctx context.Context, in *game.ClaimRestStationStaminaRequest) (*game.ClaimRestStationStaminaResponse, error) {
	ctx = LoadPlayerTimeZone(ctx, s.logger, s.db, s.statusRegistry)

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	claimCount := in.GetCount()

	restData, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry, s.tracker, s.router)
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

	updatedRestData, err := GetRestStationData(ctx, s.logger, s.db, s.statusRegistry, s.tracker, s.router)
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

func calculateRestStationGrant(ctx context.Context, currentCount int32, lastGrantTime time.Time, logger *zap.Logger, db *sql.DB, tracker Tracker, router MessageRouter, userID uuid.UUID) (int32, time.Time) {
	if currentCount >= RestStationMaxCount {
		return currentCount, lastGrantTime
	}

	if lastGrantTime.IsZero() {
		lastGrantTime = time.Now().UTC()
	}

	passedCount, latestPassedTime := CountPassedDailyTimes(ctx, lastGrantTime, RestStationDailyTimes)

	newCount := currentCount + passedCount
	overflowCount := int32(0)
	if newCount > RestStationMaxCount {
		overflowCount = newCount - RestStationMaxCount
		newCount = RestStationMaxCount
	}

	if overflowCount > 0 {
		staminaToAdd := overflowCount * RestStationEveryStamina
		if err := sendRestStationOverflowEmail(ctx, logger, db, tracker, router, userID, overflowCount, staminaToAdd); err != nil {
			logger.Error("发送休息站溢出邮件失败",
				zap.String("user_id", userID.String()),
				zap.Int32("overflow_count", overflowCount),
				zap.Error(err))
		}
	}

	if passedCount > 0 {
		return newCount, latestPassedTime
	}

	return currentCount, lastGrantTime
}

func sendRestStationOverflowEmail(ctx context.Context, logger *zap.Logger, db *sql.DB, tracker Tracker, router MessageRouter, userID uuid.UUID, overflowCount int32, staminaToAdd int32) error {
	reward := &game.Reward{
		Wallet: &game.Wallet{
			Stamina: staminaToAdd,
		},
	}

	content := &console.NoticeContent{
		Description: fmt.Sprintf("您的休息站体力已满，超过的 %d 个休息站体力已自动转换为 %d 点体力，请查收！", overflowCount, staminaToAdd),
		Rewards:     []*game.Reward{reward},
	}

	contentBytes, err := json.Marshal(content)
	if err != nil {
		logger.Error("序列化邮件内容失败", zap.Error(err))
		return err
	}

	expiryTime := time.Now().UTC().Add(3 * 24 * time.Hour)
	notification := &api.Notification{
		Id:         uuid.Must(uuid.NewV4()).String(),
		Subject:    "休息站体力溢出补偿",
		Content:    string(contentBytes),
		Code:       NotificationSystemNotice,
		SenderId:   uuid.Nil.String(),
		Persistent: true,
		ExpiryTime: timestamppb.New(expiryTime),
	}

	notifications := make(map[uuid.UUID][]*api.Notification)
	notifications[userID] = []*api.Notification{notification}

	err = NotificationSend(ctx, logger, db, tracker, router, notifications)
	if err != nil {
		logger.Error("发送休息站溢出邮件失败",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return err
	}

	logger.Info("成功发送休息站溢出邮件",
		zap.String("user_id", userID.String()),
		zap.Int32("overflow_count", overflowCount),
		zap.Int32("stamina_added", staminaToAdd),
		zap.Time("expiry_time", expiryTime))

	return nil
}
