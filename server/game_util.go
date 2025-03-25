package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
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
