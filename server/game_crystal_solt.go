package server

import (
	"context"
	"fmt"

	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

func (s *ApiServer) UpgradeCrystalSlot(ctx context.Context, in *game.UpgradeCrystalSlotRequest) (*game.UpgradeCrystalSlotResponse, error) {
	if in.EquipType != 1 && in.EquipType != 2 && in.EquipType != 4 && in.EquipType != 8 {
		return &game.UpgradeCrystalSlotResponse{
			Code: 1,
			Msg:  "无效的装备类型",
		}, nil
	}

	equipData := &EquipData{}
	if err := LoadUserData(ctx, s.logger, s.db, equipData); err != nil {
		s.logger.Error("加载装备数据失败", zap.Error(err))
		return &game.UpgradeCrystalSlotResponse{
			Code: 2,
			Msg:  "加载数据失败",
		}, nil
	}

	if equipData.CrystalSlots == nil {
		initializeCrystalSlots(equipData)
	}

	currentLevel, exists := equipData.CrystalSlots[in.EquipType]
	if !exists {
		currentLevel = 1
		equipData.CrystalSlots[in.EquipType] = 0
	}

	nextLevel := currentLevel + 1
	nextLevelConfig := s.template.GetTplCrystalEquipmentSlotDev().FindByFilter(func(slot template.TplCrystalEquipmentSlotDev) bool {
		return slot.EquipType == in.EquipType && slot.Level == nextLevel
	})

	if nextLevelConfig == nil || nextLevelConfig.Len() == 0 {
		return &game.UpgradeCrystalSlotResponse{
			Code: 3,
			Msg:  "已达到最高等级",
		}, nil
	}

	config := nextLevelConfig.Get(0)
	if config.CostItem == "" {
		return &game.UpgradeCrystalSlotResponse{
			Code: 4,
			Msg:  "配置错误",
		}, nil
	}

	source := fmt.Sprintf("crystal_slot_upgrade_type_%d_level_%d", in.EquipType, nextLevel)
	walletUpdateResult, inventoryUpdateResult, err := ConsumeCostItems(ctx, s.logger, s.db, s.template, config.CostItem, source)
	if err != nil {
		s.logger.Warn("消耗道具失败", zap.Int32("equip_type", in.EquipType), zap.Error(err))
		return &game.UpgradeCrystalSlotResponse{
			Code: 6,
			Msg:  "材料不足",
		}, nil
	}

	equipData.CrystalSlots[in.EquipType] = nextLevel

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, equipData); err != nil {
		s.logger.Error("保存装备数据失败", zap.Error(err))
		return &game.UpgradeCrystalSlotResponse{
			Code: 7,
			Msg:  "保存数据失败",
		}, nil
	}

	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item

	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}

	if inventoryUpdateResult != nil {
		inventoryUpdated = inventoryUpdateResult.Updated
	}

	s.logger.Info("水晶槽位升级成功",
		zap.Int32("equip_type", in.EquipType),
		zap.Int32("level", nextLevel))

	return &game.UpgradeCrystalSlotResponse{
		Code:             0,
		Msg:              "Success",
		EquipType:        in.EquipType,
		Level:            nextLevel,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

func initializeCrystalSlots(equipData *EquipData) {
	equipData.CrystalSlots = map[int32]int32{
		1: 0,
		2: 0,
		4: 0,
		8: 0,
	}
}
