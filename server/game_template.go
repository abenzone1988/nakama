package server

import (
	"bytes"
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
	GetTplReward() *TableTplReward
	GetTplChallenge() *TableTplChallenge
	GetTplActivityReward() *TableTplActivityReward
	GetTplActivityInfo() *TableTplActivityInfo
	GetTplPay() *TableTplPay
	GetTplFirstCharge() *TableTplFirstCharge
}

type LocalTemplateManager struct {
	logger                 *zap.Logger
	db                     *sql.DB
	tableTplRedemption     *TableTplRedemption
	tableTplReward         *TableTplReward
	tableTplChallenge      *TableTplChallenge
	tableTplActivityReward *TableTplActivityReward
	tableTplActivityInfo   *TableTplActivityInfo
	tableTplPay            *TableTplPay
	tableTplFirstCharge    *TableTplFirstCharge
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
		tableTplPay:            NewTableTplPay(logger, jsonPath),
		tableTplFirstCharge:    NewTableTplFirstCharge(logger, jsonPath),
	}
	t.LoadData()
	return &t
}

func (t *LocalTemplateManager) GetTplActivityInfo() *TableTplActivityInfo {
	return t.tableTplActivityInfo
}

func (t *LocalTemplateManager) GetTplRedemption() *TableTplRedemption {
	return t.tableTplRedemption
}

func (t *LocalTemplateManager) GetTplActivityReward() *TableTplActivityReward {
	return t.tableTplActivityReward
}

func (t *LocalTemplateManager) GetTplReward() *TableTplReward {
	return t.tableTplReward
}

func (t *LocalTemplateManager) GetTplChallenge() *TableTplChallenge {
	return t.tableTplChallenge
}

func (t *LocalTemplateManager) GetTplPay() *TableTplPay {
	return t.tableTplPay
}

func (t *LocalTemplateManager) GetTplFirstCharge() *TableTplFirstCharge {
	return t.tableTplFirstCharge
}

func (t *LocalTemplateManager) LoadData() {
	t.tableTplRedemption.LoadData(t.StorageReadTpl("TplRedemption"))
	t.tableTplReward.LoadData(t.StorageReadTpl("TplReward"))
	t.tableTplChallenge.LoadData(t.StorageReadTpl("TplChallenge"))
	t.tableTplActivityReward.LoadData(t.StorageReadTpl("TplActivityReward"))
	t.tableTplActivityInfo.LoadData(t.StorageReadTpl("TplActivityInfo"))
	t.tableTplPay.LoadData(t.StorageReadTpl("TplPay"))
	t.tableTplFirstCharge.LoadData(t.StorageReadTpl("TplFirstCharge"))
}

func (t *LocalTemplateManager) StorageReadTpl(key string) []byte {
	if t.db == nil {
		return nil
	}

	ids := []*api.ReadStorageObjectId{{
		Collection: tplCollection,
		Key:        key,
	}}

	readData, err := StorageReadObjects(context.Background(), t.logger, t.db, uuid.Nil, ids)
	if err != nil {
		t.logger.Error("读取数据库模板失败", zap.String("table", key), zap.Error(err))
		return nil
	}
	if readData == nil || readData.Objects == nil || len(readData.Objects) == 0 {
		t.logger.Warn("数据库无此模板，回退本地文件", zap.String("table", key))
		return nil
	}

	// 数据库中存在该对象但内容为空或全为空白时，同样视为数据库为空，
	// 返回 nil 让各模板表的 LoadData 兜底读取 data 目录下的同名 JSON 文件。
	content := bytes.TrimSpace([]byte(readData.Objects[0].Value))
	if len(content) == 0 {
		t.logger.Warn("数据库模板内容为空，回退本地文件", zap.String("table", key))
		return nil
	}
	return content
}
