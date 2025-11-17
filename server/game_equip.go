package server

import (
	"context"
	"errors"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *ApiServer) GetEquipData(ctx context.Context, in *emptypb.Empty) (*game.EquipData, error) {
	return GetEquipData(ctx, s.logger, s.template)
}

func (s *ApiServer) UpdateEquipData(ctx context.Context, in *game.UpdateEquipDataRequest) (*game.UpdateEquipDataResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	equipData, err := GetEquipData(ctx, s.logger, s.template)
	if err != nil {
		return nil, err
	}

	levelUpEquip, exist := equipData.UnlockEquips[in.EquipId]
	if !exist {
		return nil, errors.New("Equip " + in.EquipId + " not found")
	}

	nextEquips := s.template.GetTplEquipmentDev().FindByFilter(func(equip template.TplEquipmentDev) bool {
		return equip.EquipID == in.EquipId && levelUpEquip+1 == equip.Level
	})

	if nextEquips == nil {
		return nil, errors.New("Equip " + in.EquipId + " not found")
	}
	nextEquip := nextEquips.Get(0)
	costDebrisNum := nextEquip.CostDebris                //需要消耗的碎片数量
	costDebrisId := strings.TrimPrefix(in.EquipId, "EQ") //equipId  格式为 EQ1999 删除EQ就是碎片id

	//从背包中扣除碎片
	results, err := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
		{
			UserID:    userID,
			Changeset: map[string]int64{costDebrisId: -int64(costDebrisNum), nextEquip.CostItemID: -int64(nextEquip.CostItemNum)},
			Metadata:  `{"reason": "equip_level_up", "equip_id": ` + in.EquipId + `}`,
		},
	}, true)

	if err != nil {
		return nil, err
	}

	inventoryUpdateResult = &game.InventoryUpdateResult{
		Previous: convertMapInt64ToInt32(results[0].Previous),
		Updated:  convertMapInt64ToInt32(results[0].Updated),
	}

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
			return nil, errors.New("Invalid cost pay type")
		}

		results1, err := UpdateWallets(ctx, s.logger, s.db, []*walletUpdate{
			{
				UserID:    userID,
				Changeset: changeset,
				Metadata:  `{"reason": "equip_level_up", "equip_id": ` + in.EquipId + `}`,
			},
		}, true)
		if err != nil {
			s.logger.Error("扣除费用失败", zap.Error(err))
			return nil, err
		}
		if len(results1) > 0 && results1[0] != nil {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToInt32(results1[0].Previous),
				Updated:  convertMapInt64ToInt32(results1[0].Updated),
			}
		}
	}

	equipData.UnlockEquips[in.EquipId] += 1
	if err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		meta.Equip = equipData
		return nil
	}); err != nil {
		return nil, err
	}

	return &game.UpdateEquipDataResponse{
		Code:            0,
		Msg:             "Success",
		EquipData:       equipData,
		InventoryUpdate: inventoryUpdateResult,
		WalletUpdate:    walletUpdateResult,
	}, nil
}

// GetEquipData 获取当前装备数据
func GetEquipData(ctx context.Context, logger *zap.Logger, template TemplateManager) (*game.EquipData, error) {
	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		logger.Error("Failed to get UserMeta", zap.Error(err))
		return nil, err
	}

	// 首次访问：初始化装备数据
	if userMeta.Equip == nil {
		if err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
			initializeEquipData(meta, template)
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return userMeta.Equip, nil
}

func initializeEquipData(userMeta *game.UserMeta, table TemplateManager) {
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

	userMeta.Equip = &game.EquipData{
		BattleEquips:         battleEquips,
		UnlockEquips:         unlockEquips,
		CrystalTechTreeIndex: 0,
	}
}
