package server

import (
	"context"
	"database/sql"
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

const tplCollection = "Tpl"

type TemplateManager interface {
	LoadData()
	GetTplRedemption() *TableTplRedemption
}

type LocalTemplateManager struct {
	logger             *zap.Logger
	db                 *sql.DB
	tableTplRedemption *TableTplRedemption
}

func NewLocalTemplateManager(logger *zap.Logger, db *sql.DB, config Config) TemplateManager {
	jsonPath := config.GetDataDir()
	t := LocalTemplateManager{
		logger:             logger,
		db:                 db,
		tableTplRedemption: NewTableTplRedemption(logger, jsonPath),
	}
	t.LoadData()
	return &t
}

func (t *LocalTemplateManager) GetTplRedemption() *TableTplRedemption {
	return t.tableTplRedemption
}

func (t *LocalTemplateManager) LoadData() {
	t.tableTplRedemption.LoadData(t.StorageReadTpl("TplRedemption"))
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
