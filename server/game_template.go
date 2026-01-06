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
	GetTplCrystalTechnologyDev() *TableTplCrystalTechnologyDev
	GetTplRedemptionInfo() *TableTplRedemption
	GetTplItem() *TableTplItem
	GetTplReward() *TableTplReward
	GetTplEquipment() *TableTplEquipment
	GetTplEquipmentDev() *TableTplEquipmentDev
	GetTplUnlock() *TableTplUnlock
	GetTplShop() *TableTplShop
	GetTplShopDailyItem() *TableTplShopDailyItem
	GetTplShopPermanentItem() *TableTplShopPermanentItem
	GetTplBoxShop() *TableTplBoxShop
	GetTplBoxShopItem() *TableTplBoxShopItem
	GetTplShopChapterItem() *TableTplShopChapterItem
	GetTplShopGemItem() *TableTplShopGemItem
	GetTplTasks() *TableTplTasks
	GetTplProgressReward() *TableTplProgressReward
	GetTplFirstCharge() *TableTplFirstCharge
	GetTplSevenDay() *TableTplSevenDay
	GetTplDailySignIn() *TableTplDailySignIn
	GetTplSevenDaySignIn() *TableTplSevenDaySignIn
	GetTplPay() *TableTplPay
	GetTplPlayerLevel() *TableTplPlayerLevel
	GetTplEliteLevelInfo() *TableTplEliteLevelInfo
	GetTplEliteLevelDrop() *TableTplEliteLevelDrop
	GetTplMine() *TableTplMine
	GetTplCrystalEquipmentSlotDev() *TableTplCrystalEquipmentSlotDev
	GetTplCrystalEquipment() *TableTplCrystalEquipment
	GetTplCrystalEquipmentRefinement() *TableTplCrystalEquipmentRefinement
	GetTplCrystalEquipmentRefinementAffix() *TableTplCrystalEquipmentRefinementAffix
	GetTplCrystalEquipmentRandomReward() *TableTplCrystalEquipmentRandomReward
	GetTplCrystalSkin() *TableTplCrystalSkin
	GetTplCrystalSkinDev() *TableTplCrystalSkinDev
}

type LocalTemplateManager struct {
	sync.RWMutex
	logger                                  *zap.Logger
	db                                      *sql.DB
	tableTplRedemption                      *TableTplRedemption
	tableTplLevelInfo                       *TableTplLevelInfo
	tableTplItem                            *TableTplItem
	tableTplReward                          *TableTplReward
	tableTplActivityLevelInfo               *TableTplActivityLevelInfo
	tableTplCrystalTechnology               *TableTplCrystalTechnologyDev
	tableTplEquipment                       *TableTplEquipment
	tableTplEquipmentDev                    *TableTplEquipmentDev
	tableTplUnlock                          *TableTplUnlock
	tableTplShop                            *TableTplShop
	tableTplShopDailyItem                   *TableTplShopDailyItem
	tableTplShopPermanentItem               *TableTplShopPermanentItem
	tableTplBoxShop                         *TableTplBoxShop
	tableTplBoxShopItem                     *TableTplBoxShopItem
	tableTplShopChapterItem                 *TableTplShopChapterItem
	tableTplShopGemItem                     *TableTplShopGemItem
	tableTplTasks                           *TableTplTasks
	tableTplProgressReward                  *TableTplProgressReward
	tableTplFirstCharge                     *TableTplFirstCharge
	tableTplSevenDay                        *TableTplSevenDay
	tableTplDailySignIn                     *TableTplDailySignIn
	tableTplSevenDaySignIn                  *TableTplSevenDaySignIn
	tableTplPay                             *TableTplPay
	tableTplPlayerLevel                     *TableTplPlayerLevel
	tableTplEliteLevelInfo                  *TableTplEliteLevelInfo
	tableTplEliteLevelDrop                  *TableTplEliteLevelDrop
	tableTplMine                            *TableTplMine
	tableTplCrystalEquipmentSlotDev         *TableTplCrystalEquipmentSlotDev
	tableTplCrystalEquipment                *TableTplCrystalEquipment
	tableTplCrystalEquipmentRefinement      *TableTplCrystalEquipmentRefinement
	tableTplCrystalEquipmentRefinementAffix *TableTplCrystalEquipmentRefinementAffix
	tableTplCrystalEquipmentRandomReward    *TableTplCrystalEquipmentRandomReward
	tableTplCrystalSkin                     *TableTplCrystalSkin
	tableTplCrystalSkinDev                  *TableTplCrystalSkinDev
}

func NewLocalTemplateManager(logger *zap.Logger, db *sql.DB, config Config) TemplateManager {
	jsonPath := config.GetDataDir()
	t := LocalTemplateManager{
		logger:                                  logger,
		db:                                      db,
		tableTplRedemption:                      NewTableTplRedemption(logger, jsonPath),
		tableTplLevelInfo:                       NewTableTplLevelInfo(logger, jsonPath),
		tableTplItem:                            NewTableTplItem(logger, jsonPath),
		tableTplUnlock:                          NewTableTplUnlock(logger, jsonPath),
		tableTplReward:                          NewTableTplReward(logger, jsonPath),
		tableTplActivityLevelInfo:               NewTableTplActivityLevelInfo(logger, jsonPath),
		tableTplCrystalTechnology:               NewTableTplCrystalTechnologyDev(logger, jsonPath),
		tableTplEquipment:                       NewTableTplEquipment(logger, jsonPath),
		tableTplEquipmentDev:                    NewTableTplEquipmentDev(logger, jsonPath),
		tableTplShop:                            NewTableTplShop(logger, jsonPath),
		tableTplShopDailyItem:                   NewTableTplShopDailyItem(logger, jsonPath),
		tableTplShopPermanentItem:               NewTableTplShopPermanentItem(logger, jsonPath),
		tableTplBoxShop:                         NewTableTplBoxShop(logger, jsonPath),
		tableTplBoxShopItem:                     NewTableTplBoxShopItem(logger, jsonPath),
		tableTplShopChapterItem:                 NewTableTplShopChapterItem(logger, jsonPath),
		tableTplShopGemItem:                     NewTableTplShopGemItem(logger, jsonPath),
		tableTplTasks:                           NewTableTplTasks(logger, jsonPath),
		tableTplProgressReward:                  NewTableTplProgressReward(logger, jsonPath),
		tableTplFirstCharge:                     NewTableTplFirstCharge(logger, jsonPath),
		tableTplSevenDay:                        NewTableTplSevenDay(logger, jsonPath),
		tableTplDailySignIn:                     NewTableTplDailySignIn(logger, jsonPath),
		tableTplSevenDaySignIn:                  NewTableTplSevenDaySignIn(logger, jsonPath),
		tableTplPay:                             NewTableTplPay(logger, jsonPath),
		tableTplPlayerLevel:                     NewTableTplPlayerLevel(logger, jsonPath),
		tableTplEliteLevelInfo:                  NewTableTplEliteLevelInfo(logger, jsonPath),
		tableTplEliteLevelDrop:                  NewTableTplEliteLevelDrop(logger, jsonPath),
		tableTplMine:                            NewTableTplMine(logger, jsonPath),
		tableTplCrystalEquipmentSlotDev:         NewTableTplCrystalEquipmentSlotDev(logger, jsonPath),
		tableTplCrystalEquipment:                NewTableTplCrystalEquipment(logger, jsonPath),
		tableTplCrystalEquipmentRefinement:      NewTableTplCrystalEquipmentRefinement(logger, jsonPath),
		tableTplCrystalEquipmentRefinementAffix: NewTableTplCrystalEquipmentRefinementAffix(logger, jsonPath),
		tableTplCrystalEquipmentRandomReward:    NewTableTplCrystalEquipmentRandomReward(logger, jsonPath),
		tableTplCrystalSkin:                     NewTableTplCrystalSkin(logger, jsonPath),
		tableTplCrystalSkinDev:                  NewTableTplCrystalSkinDev(logger, jsonPath),
	}
	t.LoadData()
	return &t
}

func (t *LocalTemplateManager) LoadData() {
	t.Lock()
	defer t.Unlock()

	t.tableTplRedemption.LoadData(t.StorageReadTpl("TplRedemption"))
	t.tableTplLevelInfo.LoadData(t.StorageReadTpl("TplLevelInfo"))
	t.tableTplItem.LoadData(t.StorageReadTpl("TplItem"))
	t.tableTplReward.LoadData(t.StorageReadTpl("TplReward"))
	t.tableTplActivityLevelInfo.LoadData(t.StorageReadTpl("TplActivityLevelInfo"))
	t.tableTplCrystalTechnology.LoadData(t.StorageReadTpl("TplCrystalTechnologyDev"))
	t.tableTplEquipment.LoadData(t.StorageReadTpl("TplEquipment"))
	t.tableTplEquipmentDev.LoadData(t.StorageReadTpl("TplEquipmentDev"))
	t.tableTplUnlock.LoadData(t.StorageReadTpl("TplUnlock"))
	t.tableTplShop.LoadData(t.StorageReadTpl("TplShop"))
	t.tableTplShopDailyItem.LoadData(t.StorageReadTpl("TplShopDailyItem"))
	t.tableTplShopPermanentItem.LoadData(t.StorageReadTpl("TplShopPermanentItem"))
	t.tableTplBoxShop.LoadData(t.StorageReadTpl("TplBoxShop"))
	t.tableTplBoxShopItem.LoadData(t.StorageReadTpl("TplBoxShopItem"))
	t.tableTplShopChapterItem.LoadData(t.StorageReadTpl("TplShopChapterItem"))
	t.tableTplShopGemItem.LoadData(t.StorageReadTpl("TplShopGemItem"))
	t.tableTplTasks.LoadData(t.StorageReadTpl("TplTasks"))
	t.tableTplProgressReward.LoadData(t.StorageReadTpl("TplProgressReward"))
	t.tableTplFirstCharge.LoadData(t.StorageReadTpl("TplFirstCharge"))
	t.tableTplSevenDay.LoadData(t.StorageReadTpl("TplSevenDay"))
	t.tableTplDailySignIn.LoadData(t.StorageReadTpl("TplDailySignIn"))
	t.tableTplSevenDaySignIn.LoadData(t.StorageReadTpl("TplSevenDaySignIn"))
	t.tableTplPay.LoadData(t.StorageReadTpl("TplPay"))
	t.tableTplPlayerLevel.LoadData(t.StorageReadTpl("TplPlayerLevel"))
	t.tableTplEliteLevelInfo.LoadData(t.StorageReadTpl("TplEliteLevelInfo"))
	t.tableTplEliteLevelDrop.LoadData(t.StorageReadTpl("TplEliteLevelDrop"))
	t.tableTplMine.LoadData(t.StorageReadTpl("TplMine"))
	t.tableTplCrystalEquipmentSlotDev.LoadData(t.StorageReadTpl("TplCrystalEquipmentSlotDev"))
	t.tableTplCrystalEquipment.LoadData(t.StorageReadTpl("TplCrystalEquipment"))
	t.tableTplCrystalEquipmentRefinement.LoadData(t.StorageReadTpl("TplCrystalEquipmentRefinement"))
	t.tableTplCrystalEquipmentRefinementAffix.LoadData(t.StorageReadTpl("TplCrystalEquipmentRefinementAffix"))
	t.tableTplCrystalEquipmentRandomReward.LoadData(t.StorageReadTpl("TplCrystalEquipmentRandomReward"))
	t.tableTplCrystalSkin.LoadData(t.StorageReadTpl("TplCrystalSkin"))
	t.tableTplCrystalSkinDev.LoadData(t.StorageReadTpl("TplCrystalSkinDev"))
}

func (t *LocalTemplateManager) GetTplCrystalSkin() *TableTplCrystalSkin {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalSkin
}

func (t *LocalTemplateManager) GetTplCrystalSkinDev() *TableTplCrystalSkinDev {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalSkinDev
}

func (t *LocalTemplateManager) GetTplCrystalEquipmentRandomReward() *TableTplCrystalEquipmentRandomReward {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalEquipmentRandomReward
}

func (t *LocalTemplateManager) GetTplCrystalEquipmentSlotDev() *TableTplCrystalEquipmentSlotDev {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalEquipmentSlotDev
}

func (t *LocalTemplateManager) GetTplEliteLevelInfo() *TableTplEliteLevelInfo {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplEliteLevelInfo
}

func (t *LocalTemplateManager) GetTplEliteLevelDrop() *TableTplEliteLevelDrop {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplEliteLevelDrop
}

func (t *LocalTemplateManager) GetTplPlayerLevel() *TableTplPlayerLevel {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplPlayerLevel
}

func (t *LocalTemplateManager) GetTplFirstCharge() *TableTplFirstCharge {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplFirstCharge
}

func (t *LocalTemplateManager) GetTplPay() *TableTplPay {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplPay
}

func (t *LocalTemplateManager) GetTplSevenDay() *TableTplSevenDay {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplSevenDay
}

func (t *LocalTemplateManager) GetTplDailySignIn() *TableTplDailySignIn {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplDailySignIn
}

func (t *LocalTemplateManager) GetTplSevenDaySignIn() *TableTplSevenDaySignIn {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplSevenDaySignIn
}

func (t *LocalTemplateManager) GetTplRedemptionInfo() *TableTplRedemption {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplRedemption
}

func (t *LocalTemplateManager) GetTplShop() *TableTplShop {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplShop
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

func (t *LocalTemplateManager) GetTplCrystalTechnologyDev() *TableTplCrystalTechnologyDev {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalTechnology
}

func (t *LocalTemplateManager) GetTplEquipment() *TableTplEquipment {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplEquipment
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

func (t *LocalTemplateManager) GetTplShopDailyItem() *TableTplShopDailyItem {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplShopDailyItem
}

func (t *LocalTemplateManager) GetTplShopPermanentItem() *TableTplShopPermanentItem {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplShopPermanentItem
}

func (t *LocalTemplateManager) GetTplBoxShop() *TableTplBoxShop {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplBoxShop
}

func (t *LocalTemplateManager) GetTplBoxShopItem() *TableTplBoxShopItem {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplBoxShopItem
}

func (t *LocalTemplateManager) GetTplShopChapterItem() *TableTplShopChapterItem {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplShopChapterItem
}

func (t *LocalTemplateManager) GetTplShopGemItem() *TableTplShopGemItem {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplShopGemItem
}

func (t *LocalTemplateManager) GetTplTasks() *TableTplTasks {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplTasks
}

func (t *LocalTemplateManager) GetTplProgressReward() *TableTplProgressReward {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplProgressReward
}

func (t *LocalTemplateManager) GetTplMine() *TableTplMine {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplMine
}

func (t *LocalTemplateManager) GetTplCrystalEquipment() *TableTplCrystalEquipment {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalEquipment
}

func (t *LocalTemplateManager) GetTplCrystalEquipmentRefinement() *TableTplCrystalEquipmentRefinement {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalEquipmentRefinement
}

func (t *LocalTemplateManager) GetTplCrystalEquipmentRefinementAffix() *TableTplCrystalEquipmentRefinementAffix {
	t.RLock()
	defer t.RUnlock()
	return t.tableTplCrystalEquipmentRefinementAffix
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
