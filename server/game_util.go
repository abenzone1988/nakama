package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
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

func convertMapInt64ToInt32(m map[string]int64) map[string]int32 {
	if m == nil {
		return nil
	}
	result := make(map[string]int32, len(m))
	for k, v := range m {
		result[k] = int32(v)
	}
	return result
}

// convertMapInt64ToItems 将 map[string]int64 转换为 []*Item
func convertMapInt64ToItems(m map[string]int64) []*game.Item {
	if m == nil {
		return nil
	}
	items := make([]*game.Item, 0, len(m))
	for id, num := range m {
		items = append(items, &game.Item{
			Id:  id,
			Num: int32(num),
		})
	}
	return items
}

// convertMapInt64ToWallet 将 map[string]int64 转换为 *Wallet
func convertMapInt64ToWallet(m map[string]int64) *game.Wallet {
	if m == nil {
		return nil
	}
	return &game.Wallet{
		Coin: int32(m["coin"]),
		Gem:  int32(m["gem"]),
		Ad:   int32(m["ad"]),
	}
}
