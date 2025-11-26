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
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载装备数据
	equipData := &EquipData{}
	if err := LoadData(ctx, s.logger, s.db, userID, equipData); err != nil {
		s.logger.Error("加载装备数据失败", zap.Error(err))
		return nil, err
	}

	// 如果数据为空，初始化装备数据
	if len(equipData.UnlockEquips) == 0 {
		initializeEquipData(equipData, s.template)
		if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, equipData); err != nil {
			s.logger.Error("保存初始化的装备数据失败", zap.Error(err))
			return nil, err
		}
	}
	// 转换为 proto 格式返回
	return &game.EquipData{
		BattleEquips:         equipData.BattleEquips,
		UnlockEquips:         equipData.UnlockEquips,
		CrystalTechTreeIndex: equipData.CrystalTechTreeIndex,
	}, nil
}

func (s *ApiServer) UpdateEquipData(ctx context.Context, in *game.UpdateEquipDataRequest) (*game.UpdateEquipDataResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载实际的 EquipData 结构用于修改
	equipData := &EquipData{}
	if err := LoadData(ctx, s.logger, s.db, userID, equipData); err != nil {
		return nil, err
	}

	levelUpEquip, exist := equipData.UnlockEquips[in.EquipId]
	if !exist {
		return nil, errors.New("equip " + in.EquipId + " not found")
	}

	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	var err error
	if in.EquipId == EquipID_Crystal {
		walletUpdateResult, inventoryUpdateResult, err = s.handleCrystalEquipUpgrade(ctx, userID, in.EquipId, equipData)
		if err != nil {
			return nil, err
		}
	} else {
		walletUpdateResult, inventoryUpdateResult, err = s.handleNormalEquipUpgrade(ctx, userID, in.EquipId, levelUpEquip, equipData)
		if err != nil {
			return nil, err
		}
	}

	// 保存装备数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, equipData); err != nil {
		return nil, err
	}

	// 转换为 proto 格式返回
	protoEquipData := &game.EquipData{
		BattleEquips:         equipData.BattleEquips,
		UnlockEquips:         equipData.UnlockEquips,
		CrystalTechTreeIndex: equipData.CrystalTechTreeIndex,
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

	return &game.UpdateEquipDataResponse{
		Code:             0,
		Msg:              "Success",
		EquipData:        protoEquipData,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
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
	equipData.CrystalTechTreeIndex = 0
}

func (s *ApiServer) handleNormalEquipUpgrade(ctx context.Context, userID uuid.UUID, equipID string, currentLevel int32, equipData *EquipData) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
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
			return nil, nil, err
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

func (s *ApiServer) handleCrystalEquipUpgrade(ctx context.Context, userID uuid.UUID, equipID string, equipData *EquipData) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	targetIndex := equipData.CrystalTechTreeIndex + 1
	crystalTechs := s.template.GetTplCrystalTechnologyDev().FindByFilter(func(dev template.TplCrystalTechnologyDev) bool {
		return dev.PointIndex == targetIndex
	})
	if crystalTechs == nil || crystalTechs.Len() == 0 {
		return nil, nil, errors.New("crystal tech data not found")
	}
	nextTech := crystalTechs.Get(0)

	if nextTech.CostItemID == "" || nextTech.CostItemNum <= 0 {
		return nil, nil, errors.New("invalid crystal tech cost config")
	}

	metadata := buildEquipLevelUpMetadata(equipID)
	results, err := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
		{
			UserID: userID,
			Changeset: map[string]int64{
				nextTech.CostItemID: -int64(nextTech.CostItemNum),
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
	if nextTech.Price > 0 {
		changeset := make(map[string]int64)
		switch nextTech.CostPayType {
		case PayType_Coin:
			changeset["coin"] = -int64(nextTech.Price)
		case PayType_Gem:
			changeset["gem"] = -int64(nextTech.Price)
		case PayType_Ad:
			changeset["ad"] = -int64(nextTech.Price)
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
				// 回滚背包扣除
				_, rollbackErr := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
					{
						UserID: userID,
						Changeset: map[string]int64{
							nextTech.CostItemID: int64(nextTech.CostItemNum),
						},
						Metadata: buildEquipLevelUpRollbackMetadata(equipID),
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

	equipData.UnlockEquips[equipID] += 1
	equipData.CrystalTechTreeIndex = nextTech.PointIndex

	return walletUpdateResult, inventoryUpdateResult, nil
}

func buildEquipLevelUpMetadata(equipID string) string {
	return fmt.Sprintf("{\"reason\": \"equip_level_up\", \"equip_id\": \"%s\"}", equipID)
}

func buildEquipLevelUpRollbackMetadata(equipID string) string {
	return fmt.Sprintf("{\"reason\": \"equip_level_up_rollback\", \"equip_id\": \"%s\"}", equipID)
}
