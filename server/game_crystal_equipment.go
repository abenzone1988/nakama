package server

import (
	"context"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

func (s *ApiServer) RefineCrystalEquipment(ctx context.Context, in *game.RefineCrystalEquipmentRequest) (*game.RefineCrystalEquipmentResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	equipmentID := in.GetEquipmentId()
	if equipmentID == "" {
		return &game.RefineCrystalEquipmentResponse{
			Code: 1,
			Msg:  "装备ID不能为空",
		}, nil
	}

	crystalEquipmentsData := &CrystalEquipmentData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalEquipmentsData); err != nil {
		s.logger.Error("加载水晶装备数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.RefineCrystalEquipmentResponse{
			Code: 2,
			Msg:  "加载数据失败",
		}, nil
	}

	equipment, exists := crystalEquipmentsData.Equipments[equipmentID]
	if !exists {
		s.logger.Warn("装备不存在", zap.String("equipment_id", equipmentID), zap.String("user_id", userID.String()))
		return &game.RefineCrystalEquipmentResponse{
			Code: 3,
			Msg:  "装备不存在",
		}, nil
	}

	tplEquip, found := s.template.GetTplCrystalEquipment().FindByKey(equipment.TplId)
	if !found {
		s.logger.Error("装备模板不存在", zap.String("tpl_id", equipment.TplId))
		return &game.RefineCrystalEquipmentResponse{
			Code: 4,
			Msg:  "装备模板不存在",
		}, nil
	}

	refinementConfigs := s.template.GetTplCrystalEquipmentRefinement().FindByFilter(func(t TplCrystalEquipmentRefinement) bool {
		return t.Quality == tplEquip.Quality && t.Level == 1
	})

	if refinementConfigs.Len() == 0 {
		s.logger.Warn("未找到词条配置", zap.Int32("quality", tplEquip.Quality))
		return &game.RefineCrystalEquipmentResponse{
			Code: 5,
			Msg:  "未找到词条配置",
		}, nil
	}

	refinementConfig := refinementConfigs.Get(0)

	// 收集锁定的稀有词条信息
	// 注意：只有稀有词条可以被锁定，普通词条每次洗炼都会重新生成
	// lockedRareAffixes: 保存锁定的稀有词条完整数据（包含ID和数值）
	lockedRareAffixes := make([]CrystalEquipmentAffix, 0)

	// 遍历所有锁定的词条ID
	for _, lockedID := range equipment.LockedAffixIds {
		// 在当前激活的词条中查找对应的词条
		for _, affix := range equipment.ActiveAffixes {
			if affix.AffixID == lockedID {
				// 验证是否为稀有词条（只有稀有词条才能锁定）
				// Quality == 0: 普通词条（不应被锁定）
				// Quality == 1: 稀有词条（允许锁定）
				affixTpl, found := s.template.GetTplCrystalEquipmentRefinementAffix().FindByKey(lockedID)
				if found && affixTpl.Quality == 1 {
					lockedRareAffixes = append(lockedRareAffixes, affix)
				}
				break
			}
		}
	}

	// 根据锁定词条数量选择消耗物品配置
	lockedCount := len(lockedRareAffixes)
	var costItemInfo string
	switch lockedCount {
	case 0:
		costItemInfo = refinementConfig.CostItemInfo
	case 1:
		costItemInfo = refinementConfig.Lock1CostItemInfo
	case 2:
		costItemInfo = refinementConfig.Lock2CostItemInfo
	default:
		s.logger.Warn("锁定词条数量超过上限", zap.Int("locked_count", lockedCount))
		return &game.RefineCrystalEquipmentResponse{
			Code: 7,
			Msg:  "锁定词条数量超过上限",
		}, nil
	}

	s.logger.Info("开始洗炼水晶装备",
		zap.String("user_id", userID.String()),
		zap.String("equipment_id", equipmentID),
		zap.String("equipment_tpl_id", equipment.TplId),
		zap.Int32("equipment_quality", tplEquip.Quality),
		zap.Int("locked_rare_count", lockedCount),
		zap.String("cost_item_info", costItemInfo))

	// 消耗洗炼材料
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult
	if costItemInfo != "" && costItemInfo != "0" {
		walletResult, inventoryResult, err := ConsumeCostItems(ctx, s.logger, s.db, s.template, costItemInfo, "crystal_equipment_refine")
		if err != nil {
			s.logger.Error("消耗洗炼材料失败", zap.Error(err))
			return &game.RefineCrystalEquipmentResponse{
				Code: 8,
				Msg:  "材料不足或消耗失败",
			}, nil
		}
		walletUpdateResult = walletResult
		inventoryUpdateResult = inventoryResult
		s.logger.Info("消耗洗炼材料成功",
			zap.String("user_id", userID.String()),
			zap.String("cost_item_info", costItemInfo),
			zap.Any("wallet_result", walletResult),
			zap.Any("inventory_result", inventoryResult))
	}

	newAffixes := generateAffixesWithLockedRare(
		s.template,
		tplEquip.EquipType,
		tplEquip.Quality,
		refinementConfig,
		lockedRareAffixes,
		s.logger,
	)

	equipment.PendingAffixes = newAffixes

	// 统计生成结果中的普通和稀有词条数量
	normalCount := 0
	rareCount := 0
	for _, affix := range newAffixes {
		affixTpl, found := s.template.GetTplCrystalEquipmentRefinementAffix().FindByKey(affix.AffixID)
		if found {
			if affixTpl.Quality == 0 {
				normalCount++
			} else if affixTpl.Quality == 1 {
				rareCount++
			}
		}
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalEquipmentsData); err != nil {
		s.logger.Error("保存水晶装备数据失败", zap.Error(err))
		return &game.RefineCrystalEquipmentResponse{
			Code: 6,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("洗炼水晶装备成功",
		zap.String("user_id", userID.String()),
		zap.String("equipment_id", equipmentID),
		zap.Int("total_affix_count", len(newAffixes)),
		zap.Int("normal_affix_count", normalCount),
		zap.Int("rare_affix_count", rareCount),
		zap.Int("locked_rare_count", len(lockedRareAffixes)),
		zap.Int("new_rare_count", rareCount-len(lockedRareAffixes)))

	response := &game.RefineCrystalEquipmentResponse{
		Code:      0,
		Msg:       "Success",
		Equipment: convertToProtoEquipment(equipment),
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}
	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

func (s *ApiServer) LockCrystalAffix(ctx context.Context, in *game.LockCrystalAffixRequest) (*game.LockCrystalAffixResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	equipmentID := in.GetEquipmentId()
	affixID := in.GetAffixId()
	if equipmentID == "" || affixID == "" {
		return &game.LockCrystalAffixResponse{
			Code: 1,
			Msg:  "装备ID或词条ID不能为空",
		}, nil
	}

	crystalEquipmentsData := &CrystalEquipmentData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalEquipmentsData); err != nil {
		s.logger.Error("加载水晶装备数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.LockCrystalAffixResponse{
			Code: 2,
			Msg:  "加载数据失败",
		}, nil
	}

	equipment, exists := crystalEquipmentsData.Equipments[equipmentID]
	if !exists {
		s.logger.Warn("装备不存在", zap.String("equipment_id", equipmentID), zap.String("user_id", userID.String()))
		return &game.LockCrystalAffixResponse{
			Code: 3,
			Msg:  "装备不存在",
		}, nil
	}

	// 检查词条是否存在且为稀有词条
	affixExists := false
	isRare := false
	for _, affix := range equipment.ActiveAffixes {
		if affix.AffixID == affixID {
			affixExists = true

			// 检查是否为稀有词条
			affixTpl, found := s.template.GetTplCrystalEquipmentRefinementAffix().FindByKey(affixID)
			if found && affixTpl.Quality == 1 {
				isRare = true
			}
			break
		}
	}

	if !affixExists {
		return &game.LockCrystalAffixResponse{
			Code: 4,
			Msg:  "词条不存在",
		}, nil
	}

	if !isRare {
		return &game.LockCrystalAffixResponse{
			Code: 7,
			Msg:  "只能锁定稀有词条",
		}, nil
	}

	isLocked := false
	for _, lockedID := range equipment.LockedAffixIds {
		if lockedID == affixID {
			isLocked = true
			break
		}
	}

	if in.GetLock() {
		if isLocked {
			return &game.LockCrystalAffixResponse{
				Code: 5,
				Msg:  "词条已锁定",
			}, nil
		}
		equipment.LockedAffixIds = append(equipment.LockedAffixIds, affixID)
	} else {
		if !isLocked {
			return &game.LockCrystalAffixResponse{
				Code: 5,
				Msg:  "词条未锁定",
			}, nil
		}
		equipment.LockedAffixIds = removeStringFromSlice(equipment.LockedAffixIds, affixID)
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalEquipmentsData); err != nil {
		s.logger.Error("保存水晶装备数据失败", zap.Error(err))
		return &game.LockCrystalAffixResponse{
			Code: 6,
			Msg:  "保存数据失败",
		}, nil
	}

	action := "锁定"
	if !in.GetLock() {
		action = "解锁"
	}
	s.logger.Info(action+"词条成功",
		zap.String("user_id", userID.String()),
		zap.String("equipment_id", equipmentID),
		zap.String("affix_id", affixID),
		zap.Bool("lock", in.GetLock()))

	return &game.LockCrystalAffixResponse{
		Code:      0,
		Msg:       "Success",
		Equipment: convertToProtoEquipment(equipment),
	}, nil
}

func (s *ApiServer) ActivatePendingAffixes(ctx context.Context, in *game.ActivatePendingAffixesRequest) (*game.ActivatePendingAffixesResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	equipmentID := in.GetEquipmentId()
	if equipmentID == "" {
		return &game.ActivatePendingAffixesResponse{
			Code: 1,
			Msg:  "装备ID不能为空",
		}, nil
	}

	crystalEquipmentsData := &CrystalEquipmentData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalEquipmentsData); err != nil {
		s.logger.Error("加载水晶装备数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.ActivatePendingAffixesResponse{
			Code: 2,
			Msg:  "加载数据失败",
		}, nil
	}

	equipment, exists := crystalEquipmentsData.Equipments[equipmentID]
	if !exists {
		s.logger.Warn("装备不存在", zap.String("equipment_id", equipmentID), zap.String("user_id", userID.String()))
		return &game.ActivatePendingAffixesResponse{
			Code: 3,
			Msg:  "装备不存在",
		}, nil
	}

	if len(equipment.PendingAffixes) == 0 {
		return &game.ActivatePendingAffixesResponse{
			Code: 4,
			Msg:  "没有待激活的词条",
		}, nil
	}

	equipment.ActiveAffixes = equipment.PendingAffixes
	equipment.PendingAffixes = make([]CrystalEquipmentAffix, 0)

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalEquipmentsData); err != nil {
		s.logger.Error("保存水晶装备数据失败", zap.Error(err))
		return &game.ActivatePendingAffixesResponse{
			Code: 5,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("激活备用词条成功",
		zap.String("user_id", userID.String()),
		zap.String("equipment_id", equipmentID),
		zap.Int("activated_count", len(equipment.ActiveAffixes)))

	return &game.ActivatePendingAffixesResponse{
		Code:      0,
		Msg:       "Success",
		Equipment: convertToProtoEquipment(equipment),
	}, nil
}

func (s *ApiServer) GetCrystalEquipments(ctx context.Context, in *game.GetCrystalEquipmentsRequest) (*game.GetCrystalEquipmentsResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	crystalEquipmentsData := &CrystalEquipmentData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalEquipmentsData); err != nil {
		s.logger.Error("加载水晶装备数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.GetCrystalEquipmentsResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	if crystalEquipmentsData.GetVersion() == "" {
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalEquipmentsData); err != nil {
			s.logger.Error("保存初始化的水晶装备数据失败", zap.Error(err))
			return &game.GetCrystalEquipmentsResponse{
				Code: 2,
				Msg:  "保存数据失败",
			}, nil
		}
	}

	equipments := make([]*game.CrystalEquipmentInfo, 0, len(crystalEquipmentsData.Equipments))
	for _, equipment := range crystalEquipmentsData.Equipments {
		equipments = append(equipments, convertToProtoEquipment(equipment))
	}

	s.logger.Info("获取水晶装备列表成功",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(equipments)))

	return &game.GetCrystalEquipmentsResponse{
		Code:       0,
		Msg:        "Success",
		Equipments: equipments,
	}, nil
}

func (s *ApiServer) SalvageCrystalEquipment(ctx context.Context, in *game.SalvageCrystalEquipmentRequest) (*game.SalvageCrystalEquipmentResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	equipmentIDs := in.GetEquipmentIds()
	if len(equipmentIDs) == 0 {
		return &game.SalvageCrystalEquipmentResponse{
			Code: 1,
			Msg:  "装备ID不能为空",
		}, nil
	}

	crystalEquipmentsData := &CrystalEquipmentData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalEquipmentsData); err != nil {
		s.logger.Error("加载水晶装备数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.SalvageCrystalEquipmentResponse{
			Code: 2,
			Msg:  "加载数据失败",
		}, nil
	}

	var allRewards []*game.Reward
	var validEquipmentIDs []string

	for _, equipmentID := range equipmentIDs {
		if equipmentID == "" {
			continue
		}

		equipment, exists := crystalEquipmentsData.Equipments[equipmentID]
		if !exists {
			s.logger.Warn("装备不存在", zap.String("equipment_id", equipmentID), zap.String("user_id", userID.String()))
			continue
		}

		tplEquip, found := s.template.GetTplCrystalEquipment().FindByKey(equipment.TplId)
		if !found {
			s.logger.Error("装备模板不存在", zap.String("tpl_id", equipment.TplId), zap.String("equipment_id", equipmentID))
			continue
		}

		if tplEquip.SalvageItems == "" {
			s.logger.Warn("装备无分解奖励", zap.String("equipment_id", equipmentID), zap.String("tpl_id", equipment.TplId))
			continue
		}

		salvageItems := parseItems(tplEquip.SalvageItems, s.logger)
		if len(salvageItems) == 0 {
			s.logger.Warn("分解物品解析失败", zap.String("salvage_items", tplEquip.SalvageItems), zap.String("equipment_id", equipmentID))
			continue
		}

		allRewards = append(allRewards, &game.Reward{
			Items: salvageItems,
		})
		validEquipmentIDs = append(validEquipmentIDs, equipmentID)
	}

	if len(validEquipmentIDs) == 0 {
		return &game.SalvageCrystalEquipmentResponse{
			Code: 3,
			Msg:  "没有可分解的装备",
		}, nil
	}

	mergedReward := MergeRewards(allRewards)
	if mergedReward == nil {
		return &game.SalvageCrystalEquipmentResponse{
			Code: 6,
			Msg:  "分解物品配置错误",
		}, nil
	}

	walletUpdateResult, inventoryUpdateResult, err := GrantReward(
		ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex,
		mergedReward, "crystal_equipment_salvage",
	)
	if err != nil {
		s.logger.Error("发放分解奖励失败", zap.Error(err))
		return &game.SalvageCrystalEquipmentResponse{
			Code: 7,
			Msg:  "发放奖励失败",
		}, nil
	}

	for _, equipmentID := range validEquipmentIDs {
		delete(crystalEquipmentsData.Equipments, equipmentID)
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalEquipmentsData); err != nil {
		s.logger.Error("保存水晶装备数据失败", zap.Error(err))
		return &game.SalvageCrystalEquipmentResponse{
			Code: 8,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("分解水晶装备成功",
		zap.String("user_id", userID.String()),
		zap.Int("equipment_count", len(validEquipmentIDs)),
		zap.Int("item_count", len(mergedReward.Items)))

	response := &game.SalvageCrystalEquipmentResponse{
		Code:   0,
		Msg:    "Success",
		Reward: mergedReward,
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}
	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

func convertToProtoEquipment(equipment *CrystalEquipment) *game.CrystalEquipmentInfo {
	activeAffixesProto := make([]*game.CrystalEquipmentAffix, len(equipment.ActiveAffixes))
	for i, affix := range equipment.ActiveAffixes {
		activeAffixesProto[i] = &game.CrystalEquipmentAffix{
			AffixId: affix.AffixID,
			Value:   affix.Value,
		}
	}

	pendingAffixesProto := make([]*game.CrystalEquipmentAffix, len(equipment.PendingAffixes))
	for i, affix := range equipment.PendingAffixes {
		pendingAffixesProto[i] = &game.CrystalEquipmentAffix{
			AffixId: affix.AffixID,
			Value:   affix.Value,
		}
	}

	return &game.CrystalEquipmentInfo{
		TplId:          equipment.TplId,
		Id:             equipment.ID,
		ActiveAffixes:  activeAffixesProto,
		PendingAffixes: pendingAffixesProto,
		LockedAffixIds: equipment.LockedAffixIds,
	}
}

// removeStringFromSlice 从字符串切片中删除指定元素
func removeStringFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
