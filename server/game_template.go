package server

import (
	"context"
	"database/sql"
	"sync"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

const tplCollection = "Tpl"

type TemplateManager interface {
	LoadData()
	GetTplRedemption() *TableTplRedemption
	GetTplLevelInfo() *TableTplLevelInfo
	GetTplActivityLevelInfo() *TableTplActivityLevelInfo
	GetTplRedemptionInfo() *TableTplRedemption
	GetTplItem() *TableTplItem
	GetTplReward() *TableTplReward
	GetTplEquipmentDev() *TableTplEquipmentDev
	GetTplUnlock() *TableTplUnlock
}

type LocalTemplateManager struct {
	sync.RWMutex
	logger                    *zap.Logger
	db                        *sql.DB
	tableTplRedemption        *TableTplRedemption
	tableTplLevelInfo         *TableTplLevelInfo
	tableTplItem              *TableTplItem
	tableTplReward            *TableTplReward
	tableTplActivityLevelInfo *TableTplActivityLevelInfo
	tableTplEquipmentDev      *TableTplEquipmentDev
	tableTplUnlock            *TableTplUnlock
}

func (t *LocalTemplateManager) GetTplRedemptionInfo() *TableTplRedemption {
	//TODO implement me
	panic("implement me")
}

func NewLocalTemplateManager(logger *zap.Logger, db *sql.DB, config Config) TemplateManager {
	jsonPath := config.GetDataDir()
	t := LocalTemplateManager{
		logger:                    logger,
		db:                        db,
		tableTplRedemption:        NewTableTplRedemption(logger, jsonPath),
		tableTplLevelInfo:         NewTableTplLevelInfo(logger, jsonPath),
		tableTplItem:              NewTableTplItem(logger, jsonPath),
		tableTplUnlock:            NewTableTplUnlock(logger, jsonPath),
		tableTplReward:            NewTableTplReward(logger, jsonPath),
		tableTplActivityLevelInfo: NewTableTplActivityLevelInfo(logger, jsonPath),
		tableTplEquipmentDev:      NewTableTplEquipmentDev(logger, jsonPath),
	}
	t.LoadData()
	return &t
}

func (t *LocalTemplateManager) GetTplUnlock() *TableTplUnlock {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplUnlock
}

func (t *LocalTemplateManager) GetTplItem() *TableTplItem {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplItem
}

func (t *LocalTemplateManager) GetTplReward() *TableTplReward {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplReward
}

func (t *LocalTemplateManager) GetTplActivityLevelInfo() *TableTplActivityLevelInfo {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplActivityLevelInfo
}

func (t *LocalTemplateManager) GetTplEquipmentDev() *TableTplEquipmentDev {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplEquipmentDev
}

func (t *LocalTemplateManager) GetTplRedemption() *TableTplRedemption {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplRedemption
}

func (t *LocalTemplateManager) GetTplLevelInfo() *TableTplLevelInfo {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplLevelInfo
}

func (t *LocalTemplateManager) LoadData() {
	t.Lock()
	defer t.Unlock()

	t.tableTplRedemption.LoadData(t.StorageReadTpl("TplRedemption"))
	t.tableTplLevelInfo.LoadData(t.StorageReadTpl("TplLevelInfo"))
	t.tableTplItem.LoadData(t.StorageReadTpl("TplItem"))
	t.tableTplReward.LoadData(t.StorageReadTpl("TplReward"))
	t.tableTplActivityLevelInfo.LoadData(t.StorageReadTpl("TplActivityLevelInfo"))
	t.tableTplEquipmentDev.LoadData(t.StorageReadTpl("TplEquipmentDev"))
	t.tableTplUnlock.LoadData(t.StorageReadTpl("TplUnlock"))
}

func (t *LocalTemplateManager) StorageReadTpl(key string) []byte {
	ids := []*api.ReadStorageObjectId{{
		Collection: tplCollection,
		Key:        key,
	}}

	readData, err := StorageReadObjects(context.Background(), t.logger, t.db, uuid.Nil, ids)
	if err != nil {
		t.logger.Error("Error reading storage object", zap.Error(err), zap.String("key", key))
		return nil
	}
	if readData == nil || readData.Objects == nil || len(readData.Objects) == 0 {
		t.logger.Error("Error reading storage object", zap.Error(err), zap.String("key", key))
		return nil
	}
	return []byte(readData.Objects[0].Value)
}
