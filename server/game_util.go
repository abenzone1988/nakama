package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	permissionRead  = int32(1)
	permissionWrite = int32(0)
)

func CreateStorageOpWrite(collection, key, value, ownerID string) *StorageOpWrite {
	return &StorageOpWrite{
		Object: &api.WriteStorageObject{
			Collection:      collection,
			Key:             key,
			Value:           value,
			PermissionRead:  &wrapperspb.Int32Value{Value: permissionRead},
			PermissionWrite: &wrapperspb.Int32Value{Value: permissionWrite},
		},
		OwnerID: ownerID,
	}
}

type Storable interface {
	GetCollection() string
	GetKey() string
	Init()
}

func LoadData(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, storable Storable) error {
	readOp := &api.ReadStorageObjectId{
		Collection: storable.GetCollection(),
		Key:        storable.GetKey(),
		UserId:     userID.String(),
	}

	objectIDs := []*api.ReadStorageObjectId{readOp}

	storageObjects, err := StorageReadObjects(ctx, logger, db, userID, objectIDs)
	if err != nil {
		logger.Error("无法从存储系统读取数据", zap.Error(err))
		return err
	}

	if len(storageObjects.Objects) == 0 {
		storable.Init()
		return nil
	}

	if err := json.Unmarshal([]byte(storageObjects.Objects[0].Value), storable); err != nil {
		logger.Error("无法反序列化数据", zap.Error(err))
		return err
	}

	return nil
}

func SaveData(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, userID uuid.UUID, storable Storable) error {
	serializedData, err := json.Marshal(storable)
	if err != nil {
		logger.Error("无法序列化数据", zap.Error(err))
		return err
	}

	writeOp := CreateStorageOpWrite(storable.GetCollection(), storable.GetKey(), string(serializedData), userID.String())

	ops := []*StorageOpWrite{writeOp}

	_, _, err = StorageWriteObjects(ctx, logger, db, metrics, storageIndex, true, ops)
	if err != nil {
		logger.Error("无法保存数据到存储系统", zap.Error(err))
		return err
	}

	return nil
}

// parseTime 解析时间字符串，支持UTC和自定义格式
func parseTime(timeStr string) (time.Time, error) {
	// 尝试解析UTC格式
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}

	// UTC格式解析失败，尝试解析自定义格式
	return time.Parse("2006:01:02:15:04:05", timeStr)
}

// parseDateTime 解析日期和时间字符串为 time.Time
// 适配格式如 "2025/7/11 10:00"
func parseDateTime(datetimeStr string) (time.Time, error) {
	// 解析格式为 "2006/1/2 15:04"
	parsedTime, err := time.ParseInLocation("2006-1-2 15:04", datetimeStr, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析时间失败 '%s': %w", datetimeStr, err)
	}

	return parsedTime, nil
}

// ParseRewards 解析奖励ID数组，返回奖励对象数组
func ParseRewards(logger *zap.Logger, tplRewardTable *template.TableTplReward, rewardIds []string) ([]*game.Reward, error) {
	if len(rewardIds) == 0 {
		return nil, nil
	}

	rewards := make([]*game.Reward, 0, len(rewardIds))

	for _, rewardId := range rewardIds {
		if rewardId == "" {
			continue
		}

		tplReward, found := tplRewardTable.FindByKey(rewardId)
		if !found {
			logger.Warn("奖励配置不存在",
				zap.String("reward_id", rewardId))
			continue
		}

		// 构造钱包奖励
		walletReward := &game.Wallet{
			Gem:  tplReward.Gem,
			Coin: tplReward.Coin,
			Ad:   tplReward.Coupon,
		}

		// 解析物品奖励
		items, err := ParseItemRewards(logger, tplReward.Items, rewardId)
		if err != nil {
			logger.Warn("解析物品奖励失败",
				zap.String("reward_id", rewardId),
				zap.Error(err))
			continue
		}

		// 构造奖励对象
		reward := &game.Reward{
			Wallet: walletReward,
			Items:  items,
		}

		rewards = append(rewards, reward)
	}

	return rewards, nil
}

// ParseItemRewards 解析物品奖励字符串 "90000_20,90001_30"
func ParseItemRewards(logger *zap.Logger, itemsStr, rewardId string) ([]*game.Item, error) {
	if itemsStr == "" || strings.TrimSpace(itemsStr) == "" {
		return nil, nil
	}

	var items []*game.Item
	itemStrings := strings.Split(itemsStr, ",")

	for _, itemStr := range itemStrings {
		itemStr = strings.TrimSpace(itemStr)
		if itemStr == "" {
			continue
		}

		parts := strings.Split(itemStr, "_")
		if len(parts) != 2 {
			logger.Warn("物品奖励格式错误",
				zap.String("item_string", itemStr),
				zap.String("reward_id", rewardId))
			continue
		}

		itemID := strings.TrimSpace(parts[0])
		numStr := strings.TrimSpace(parts[1])

		num, err := strconv.ParseInt(numStr, 10, 32)
		if err != nil {
			logger.Warn("物品数量解析失败",
				zap.String("item_string", itemStr),
				zap.String("num_string", numStr),
				zap.String("reward_id", rewardId),
				zap.Error(err))
			continue
		}

		item := &game.Item{
			Id:  itemID,
			Num: int32(num),
		}
		items = append(items, item)
	}

	return items, nil
}

// ParseSingleReward 解析单个奖励ID，返回奖励对象（兼容性方法）
func ParseSingleReward(logger *zap.Logger, tplRewardTable *template.TableTplReward, rewardId string) (*game.Reward, error) {
	rewards, err := ParseRewards(logger, tplRewardTable, []string{rewardId})
	if err != nil {
		return nil, err
	}

	if len(rewards) == 0 {
		return nil, fmt.Errorf("奖励不存在: %s", rewardId)
	}

	return rewards[0], nil
}
