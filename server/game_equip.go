package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ApiServer) GetEquipData(ctx context.Context, in *emptypb.Empty) (*game.EquipData, error) {
	// 加载装备数据
	equipData := &EquipData{}
	if err := LoadUserData(ctx, s.logger, s.db, equipData); err != nil {
		s.logger.Error("加载装备数据失败", zap.Error(err))
		return nil, err
	}

	// 如果数据为空，初始化装备数据
	if len(equipData.UnlockEquips) == 0 {
		initializeEquipData(equipData, s.template)
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, equipData); err != nil {
			s.logger.Error("保存初始化的装备数据失败", zap.Error(err))
			return nil, err
		}
	}

	// 初始化水晶槽位数据
	if len(equipData.CrystalSlots) == 0 {
		initializeCrystalSlots(equipData)
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, equipData); err != nil {
			s.logger.Error("保存初始化的水晶槽位数据失败", zap.Error(err))
		}
	}

	return &game.EquipData{
		BattleEquips:           equipData.BattleEquips,
		UnlockEquips:           equipData.UnlockEquips,
		UnlockedCrystalTechIds: equipData.UnlockedCrystalTechIDs,
		CrystalSlots:           equipData.CrystalSlots,
	}, nil
}

func (s *ApiServer) UpgradeEquip(ctx context.Context, in *game.UpgradeEquipRequest) (*game.UpgradeEquipResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载实际的 EquipData 结构用于修改
	equipData := &EquipData{}
	if err := LoadUserData(ctx, s.logger, s.db, equipData); err != nil {
		return nil, err
	}

	levelUpEquip, exist := equipData.UnlockEquips[in.EquipId]
	if !exist {
		return nil, errors.New("equip " + in.EquipId + " not found")
	}

	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	walletUpdateResult, inventoryUpdateResult, err := s.handleEquipUpgrade(ctx, userID, in.EquipId, levelUpEquip, equipData)
	if err != nil {
		return &game.UpgradeEquipResponse{
			Code: 1,
			Msg:  err.Error(),
		}, nil
	}

	// 保存装备数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, equipData); err != nil {
		return nil, err
	}

	// 提取更新后的数据
	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item

	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}

	if inventoryUpdateResult != nil {
		inventoryUpdated = inventoryUpdateResult.Updated
	}

	return &game.UpgradeEquipResponse{
		Code:             0,
		Msg:              "Success",
		EquipId:          in.EquipId,
		Level:            equipData.UnlockEquips[in.EquipId],
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

func (s *ApiServer) UpgradeCrystalTech(ctx context.Context, in *game.UpgradeCrystalTechRequest) (*game.UpgradeCrystalTechResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	if in.TechId == "" {
		return &game.UpgradeCrystalTechResponse{
			Code: 1,
			Msg:  "tech_id is required",
		}, nil
	}

	equipData := &EquipData{}
	if err := LoadUserData(ctx, s.logger, s.db, equipData); err != nil {
		return nil, err
	}

	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	walletUpdateResult, inventoryUpdateResult, err := s.handleCrystalTechUpgrade(ctx, userID, in.TechId, equipData)
	if err != nil {
		return &game.UpgradeCrystalTechResponse{
			Code: 1,
			Msg:  err.Error(),
		}, nil
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, equipData); err != nil {
		return nil, err
	}

	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item

	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}

	if inventoryUpdateResult != nil {
		inventoryUpdated = inventoryUpdateResult.Updated
	}

	return &game.UpgradeCrystalTechResponse{
		Code:                   0,
		Msg:                    "Success",
		UnlockedCrystalTechIds: equipData.UnlockedCrystalTechIDs,
		WalletUpdated:          walletUpdated,
		InventoryUpdated:       inventoryUpdated,
	}, nil

}

// 初始化装备数据
func initializeEquipData(equipData *EquipData, table TemplateManager) {
	initEquips := table.GetTplUnlock().FindByFilter(func(unlock template.TplUnlock) bool {
		return unlock.ConditionParameter == ZeroLevelId && unlock.Type == UnlockType_Equipment
	})

	battleEquips := make([]string, 0, initEquips.Len())
	unlockEquips := make(map[string]int32, initEquips.Len())
	for i := 0; i < initEquips.Len(); i++ {
		equip := initEquips.Get(i)
		battleEquips = append(battleEquips, equip.Parameter)
		unlockEquips[equip.Parameter] = 1 // 等级为1
	}

	equipData.BattleEquips = battleEquips
	equipData.UnlockEquips = unlockEquips
	equipData.UnlockedCrystalTechIDs = []string{}
}

func (s *ApiServer) handleEquipUpgrade(ctx context.Context, userID uuid.UUID, equipID string, currentLevel int32, equipData *EquipData) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	nextEquips := s.template.GetTplEquipmentDev().FindByFilter(func(equip template.TplEquipmentDev) bool {
		return equip.EquipID == equipID && currentLevel+1 == equip.Level
	})
	if nextEquips == nil || nextEquips.Len() == 0 {
		return nil, nil, errors.New("equip " + equipID + " not found")
	}

	nextEquip := nextEquips.Get(0)
	costDebrisNum := nextEquip.CostDebris
	costDebrisID := strings.TrimPrefix(equipID, "EQ")

	metadata := buildEquipLevelUpMetadata(equipID)
	results, err := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
		{
			UserID: userID,
			Changeset: map[string]int64{
				costDebrisID:         -int64(costDebrisNum),
				nextEquip.CostItemID: -int64(nextEquip.CostItemNum),
			},
			Metadata: metadata,
		},
	}, true)
	if err != nil {
		s.logger.Warn("材料不足", zap.String("equip", equipID), zap.Int32("拥有", costDebrisNum), zap.Int32("需要", nextEquip.CostItemNum))
		return nil, nil, errors.New("材料不足")
	}

	var inventoryUpdateResult *game.InventoryUpdateResult
	if len(results) > 0 && results[0] != nil {
		inventoryUpdateResult = &game.InventoryUpdateResult{
			Previous: convertMapInt64ToItems(results[0].Previous),
			Updated:  convertMapInt64ToItems(results[0].Updated),
		}
	}

	var walletUpdateResult *game.WalletUpdateResult
	if nextEquip.Price > 0 {
		changeset := make(map[string]int64)
		switch nextEquip.CostPayType {
		case PayType_Coin:
			changeset["coin"] = -int64(nextEquip.Price)
		case PayType_Gem:
			changeset["gem"] = -int64(nextEquip.Price)
		case PayType_Ad:
			changeset["ad"] = -int64(nextEquip.Price)
		default:
			return nil, nil, errors.New("invalid cost pay type")
		}

		results1, err := UpdateWallets(ctx, s.logger, s.db, []*walletUpdate{
			{
				UserID:    userID,
				Changeset: changeset,
				Metadata:  metadata,
			},
		}, true)
		if err != nil {
			s.logger.Error("扣除费用失败", zap.Error(err))
			return nil, nil, errors.New("货币不足")
		}
		if len(results1) > 0 && results1[0] != nil {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results1[0].Previous),
				Updated:  convertMapInt64ToWallet(results1[0].Updated),
			}
		}
	}

	equipData.UnlockEquips[equipID] = currentLevel + 1
	return walletUpdateResult, inventoryUpdateResult, nil
}

func (s *ApiServer) handleCrystalTechUpgrade(ctx context.Context, userID uuid.UUID, techID string, equipData *EquipData) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	techData, ok := s.template.GetTplCrystalTechnologyDev().FindByKey(techID)
	if !ok {
		return nil, nil, errors.New("crystal tech not found")
	}

	for _, unlockedID := range equipData.UnlockedCrystalTechIDs {
		if unlockedID == techID {
			return nil, nil, errors.New("crystal tech already unlocked")
		}
	}

	if techData.PretechID != "" {
		pretechIDs := strings.Split(techData.PretechID, ",")
		for _, pretechID := range pretechIDs {
			pretechID = strings.TrimSpace(pretechID)
			if pretechID == "" {
				continue
			}
			found := false
			for _, unlockedID := range equipData.UnlockedCrystalTechIDs {
				if unlockedID == pretechID {
					found = true
					break
				}
			}
			if !found {
				return nil, nil, errors.New("pretech not unlocked: " + pretechID)
			}
		}
	}

	if techData.CostItemID == "" || techData.CostItemNum <= 0 {
		return nil, nil, errors.New("invalid crystal tech cost config")
	}

	metadata := buildEquipLevelUpMetadata(EquipID_Crystal)
	results, err := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
		{
			UserID: userID,
			Changeset: map[string]int64{
				techData.CostItemID: -int64(techData.CostItemNum),
			},
			Metadata: metadata,
		},
	}, true)
	if err != nil {
		return nil, nil, err
	}

	var inventoryUpdateResult *game.InventoryUpdateResult
	if len(results) > 0 && results[0] != nil {
		inventoryUpdateResult = &game.InventoryUpdateResult{
			Previous: convertMapInt64ToItems(results[0].Previous),
			Updated:  convertMapInt64ToItems(results[0].Updated),
		}
	}

	var walletUpdateResult *game.WalletUpdateResult
	if techData.Price > 0 {
		changeset := make(map[string]int64)
		switch techData.CostPayType {
		case PayType_Coin:
			changeset["coin"] = -int64(techData.Price)
		case PayType_Gem:
			changeset["gem"] = -int64(techData.Price)
		case PayType_Ad:
			changeset["ad"] = -int64(techData.Price)
		default:
			return nil, nil, errors.New("invalid cost pay type")
		}

		if len(changeset) > 0 {
			walletResults, err := UpdateWallets(ctx, s.logger, s.db, []*walletUpdate{
				{
					UserID:    userID,
					Changeset: changeset,
					Metadata:  metadata,
				},
			}, true)
			if err != nil {
				_, rollbackErr := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
					{
						UserID: userID,
						Changeset: map[string]int64{
							techData.CostItemID: int64(techData.CostItemNum),
						},
						Metadata: buildEquipLevelUpRollbackMetadata(EquipID_Crystal),
					},
				}, true)
				if rollbackErr != nil {
					s.logger.Error("Crystal equip rollback failed", zap.Error(rollbackErr))
				}
				return nil, nil, err
			}
			if len(walletResults) > 0 && walletResults[0] != nil {
				walletUpdateResult = &game.WalletUpdateResult{
					Previous: convertMapInt64ToWallet(walletResults[0].Previous),
					Updated:  convertMapInt64ToWallet(walletResults[0].Updated),
				}
			}
		}
	}

	equipData.UnlockedCrystalTechIDs = append(equipData.UnlockedCrystalTechIDs, techID)

	return walletUpdateResult, inventoryUpdateResult, nil
}

func buildEquipLevelUpMetadata(equipID string) string {
	return fmt.Sprintf("{\"reason\": \"equip_level_up\", \"equip_id\": \"%s\"}", equipID)
}

func buildEquipLevelUpRollbackMetadata(equipID string) string {
	return fmt.Sprintf("{\"reason\": \"equip_level_up_rollback\", \"equip_id\": \"%s\"}", equipID)
}
