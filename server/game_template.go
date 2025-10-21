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
	GetTplReward() *TableTplReward
	GetTplChallenge() *TableTplChallenge
	GetTplActivityReward() *TableTplActivityReward
	GetTplActivityInfo() *TableTplActivityInfo
	GetTplStarterPack() *TableTplStarterPack
	GetTplDailySignIn() *TableTplDailySignIn
}

type LocalTemplateManager struct {
	sync.RWMutex
	logger                 *zap.Logger
	db                     *sql.DB
	tableTplRedemption     *TableTplRedemption
	tableTplReward         *TableTplReward
	tableTplChallenge      *TableTplChallenge
	tableTplActivityReward *TableTplActivityReward
	tableTplActivityInfo   *TableTplActivityInfo
	tableTplStarterPack    *TableTplStarterPack
	tableTplDailySignIn    *TableTplDailySignIn
}

func NewLocalTemplateManager(logger *zap.Logger, db *sql.DB, config Config) TemplateManager {
	jsonPath := config.GetDataDir()
	t := LocalTemplateManager{
		logger:                 logger,
		db:                     db,
		tableTplRedemption:     NewTableTplRedemption(logger, jsonPath),
		tableTplReward:         NewTableTplReward(logger, jsonPath),
		tableTplChallenge:      NewTableTplChallenge(logger, jsonPath),
		tableTplActivityReward: NewTableTplActivityReward(logger, jsonPath),
		tableTplActivityInfo:   NewTableTplActivityInfo(logger, jsonPath),
		tableTplStarterPack:    NewTableTplStarterPack(logger, jsonPath),
		tableTplDailySignIn:    NewTableTplDailySignIn(logger, jsonPath),
	}
	t.LoadData()
	return &t
}

func (t *LocalTemplateManager) GetTplStarterPack() *TableTplStarterPack {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplStarterPack
}

func (t *LocalTemplateManager) GetTplDailySignIn() *TableTplDailySignIn {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplDailySignIn
}

func (t *LocalTemplateManager) GetTplActivityInfo() *TableTplActivityInfo {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplActivityInfo
}

func (t *LocalTemplateManager) GetTplRedemption() *TableTplRedemption {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplRedemption
}

func (t *LocalTemplateManager) GetTplActivityReward() *TableTplActivityReward {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplActivityReward
}

func (t *LocalTemplateManager) GetTplReward() *TableTplReward {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplReward
}

func (t *LocalTemplateManager) GetTplChallenge() *TableTplChallenge {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplChallenge
}

func (t *LocalTemplateManager) LoadData() {
	t.Lock()
	defer t.Unlock()

	t.tableTplRedemption.LoadData(t.StorageReadTpl("TplRedemption"))
	t.tableTplReward.LoadData(t.StorageReadTpl("TplReward"))
	t.tableTplChallenge.LoadData(t.StorageReadTpl("TplChallenge"))
	t.tableTplActivityReward.LoadData(t.StorageReadTpl("TplActivityReward"))
	t.tableTplActivityInfo.LoadData(t.StorageReadTpl("TplActivityInfo"))
}

func (t *LocalTemplateManager) StorageReadTpl(key string) []byte {
	ids := []*api.ReadStorageObjectId{{
		Collection: tplCollection,
		Key:        key,
	}}

	readData, err := StorageReadObjects(context.Background(), t.logger, t.db, uuid.Nil, ids)
	if err != nil {
		t.logger.Error("Error reading storage object", zap.Error(err))
		return nil
	}
	if readData == nil || readData.Objects == nil || len(readData.Objects) == 0 {
		t.logger.Error("Error reading storage object", zap.Error(err))
		return nil
	}
	return []byte(readData.Objects[0].Value)
}
