package server

import (
	"context"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

func (s *ApiServer) UnlockCrystalSkin(ctx context.Context, in *game.UnlockCrystalSkinRequest) (*game.UnlockCrystalSkinResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	skinID := in.GetSkinId()
	if skinID == "" {
		return &game.UnlockCrystalSkinResponse{
			Code: 1,
			Msg:  "皮肤ID不能为空",
		}, nil
	}

	tplSkin, found := s.template.GetTplCrystalSkin().FindByKey(skinID)
	if !found {
		s.logger.Warn("皮肤模板不存在", zap.String("skin_id", skinID), zap.String("user_id", userID.String()))
		return &game.UnlockCrystalSkinResponse{
			Code: 2,
			Msg:  "皮肤模板不存在",
		}, nil
	}

	crystalSkinData := &CrystalSkinData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalSkinData); err != nil {
		s.logger.Error("加载水晶皮肤数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.UnlockCrystalSkinResponse{
			Code: 3,
			Msg:  "加载数据失败",
		}, nil
	}

	if _, exists := crystalSkinData.Skins[skinID]; exists {
		s.logger.Warn("皮肤已解锁", zap.String("skin_id", skinID), zap.String("user_id", userID.String()))
		return &game.UnlockCrystalSkinResponse{
			Code: 4,
			Msg:  "皮肤已解锁",
		}, nil
	}

	if tplSkin.UnlockCostItems != "" && tplSkin.UnlockCostItems != "0" {
		source := "crystal_skin_unlock_" + skinID
		walletResult, inventoryResult, err := ConsumeCostItems(ctx, s.logger, s.db, s.template, tplSkin.UnlockCostItems, source)
		if err != nil {
			s.logger.Error("消耗解锁材料失败", zap.Error(err))
			return &game.UnlockCrystalSkinResponse{
				Code: 5,
				Msg:  "材料不足或消耗失败",
			}, nil
		}

		skinUUID, err := uuid.NewV4()
		if err != nil {
			s.logger.Error("生成皮肤UUID失败", zap.Error(err))
			return &game.UnlockCrystalSkinResponse{
				Code: 6,
				Msg:  "生成ID失败",
			}, nil
		}

		crystalSkinData.Skins[skinID] = &CrystalSkin{
			SkinID:    skinID,
			ID:        skinUUID.String(),
			StarLevel: 1,
		}

		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalSkinData); err != nil {
			s.logger.Error("保存水晶皮肤数据失败", zap.Error(err))
			return &game.UnlockCrystalSkinResponse{
				Code: 7,
				Msg:  "保存数据失败",
			}, nil
		}

		s.logger.Info("解锁水晶皮肤成功",
			zap.String("user_id", userID.String()),
			zap.String("skin_id", skinID))

		response := &game.UnlockCrystalSkinResponse{
			Code: 0,
			Msg:  "Success",
			Skin: &game.CrystalSkinInfo{
				SkinId:    skinID,
				StarLevel: 1,
			},
		}

		if walletResult != nil {
			response.WalletUpdated = walletResult.Updated
		}
		if inventoryResult != nil {
			response.InventoryUpdated = inventoryResult.Updated
		}

		return response, nil
	}

	crystalSkinData.Skins[skinID] = &CrystalSkin{
		SkinID:    skinID,
		StarLevel: 0,
	}

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalSkinData); err != nil {
		s.logger.Error("保存水晶皮肤数据失败", zap.Error(err))
		return &game.UnlockCrystalSkinResponse{
			Code: 7,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("解锁水晶皮肤成功",
		zap.String("user_id", userID.String()),
		zap.String("skin_id", skinID))

	return &game.UnlockCrystalSkinResponse{
		Code: 0,
		Msg:  "Success",
		Skin: &game.CrystalSkinInfo{
			SkinId:    skinID,
			StarLevel: 0,
		},
	}, nil
}

func (s *ApiServer) UpgradeCrystalSkin(ctx context.Context, in *game.UpgradeCrystalSkinRequest) (*game.UpgradeCrystalSkinResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	skinID := in.GetSkinId()
	if skinID == "" {
		return &game.UpgradeCrystalSkinResponse{
			Code: 1,
			Msg:  "皮肤ID不能为空",
		}, nil
	}

	crystalSkinData := &CrystalSkinData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalSkinData); err != nil {
		s.logger.Error("加载水晶皮肤数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.UpgradeCrystalSkinResponse{
			Code: 2,
			Msg:  "加载数据失败",
		}, nil
	}

	skin, exists := crystalSkinData.Skins[skinID]
	if !exists {
		s.logger.Warn("皮肤未解锁", zap.String("skin_id", skinID), zap.String("user_id", userID.String()))
		return &game.UpgradeCrystalSkinResponse{
			Code: 3,
			Msg:  "皮肤未解锁",
		}, nil
	}

	nextStarLevel := skin.StarLevel + 1
	upgradeConfigs := s.template.GetTplCrystalSkinDev().FindByFilter(func(t TplCrystalSkinDev) bool {
		return t.SkinID == skinID && t.StarLevel == nextStarLevel
	})

	if upgradeConfigs.Len() == 0 {
		s.logger.Warn("已达到最高等级", zap.String("skin_id", skinID), zap.Int32("current_level", skin.StarLevel))
		return &game.UpgradeCrystalSkinResponse{
			Code: 4,
			Msg:  "已达到最高等级",
		}, nil
	}

	upgradeConfig := upgradeConfigs.Get(0)
	if upgradeConfig.CostItemInfo == "" || upgradeConfig.CostItemInfo == "0" {
		s.logger.Warn("升级配置错误", zap.String("skin_id", skinID), zap.Int32("next_level", nextStarLevel))
		return &game.UpgradeCrystalSkinResponse{
			Code: 5,
			Msg:  "升级配置错误",
		}, nil
	}

	source := fmt.Sprintf("crystal_skin_upgrade_%s_level_%d", skinID, nextStarLevel)
	walletResult, inventoryResult, err := ConsumeCostItems(ctx, s.logger, s.db, s.template, upgradeConfig.CostItemInfo, source)
	if err != nil {
		s.logger.Error("消耗升级材料失败", zap.Error(err))
		return &game.UpgradeCrystalSkinResponse{
			Code: 6,
			Msg:  "材料不足或消耗失败",
		}, nil
	}

	skin.StarLevel = nextStarLevel

	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, crystalSkinData); err != nil {
		s.logger.Error("保存水晶皮肤数据失败", zap.Error(err))
		return &game.UpgradeCrystalSkinResponse{
			Code: 7,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("升级水晶皮肤成功",
		zap.String("user_id", userID.String()),
		zap.String("skin_id", skinID),
		zap.Int32("new_level", nextStarLevel))

	response := &game.UpgradeCrystalSkinResponse{
		Code: 0,
		Msg:  "Success",
		Skin: &game.CrystalSkinInfo{
			SkinId:    skin.SkinID,
			StarLevel: skin.StarLevel,
		},
	}

	if walletResult != nil {
		response.WalletUpdated = walletResult.Updated
	}
	if inventoryResult != nil {
		response.InventoryUpdated = inventoryResult.Updated
	}

	return response, nil
}

func (s *ApiServer) GetCrystalSkins(ctx context.Context, in *game.GetCrystalSkinsRequest) (*game.GetCrystalSkinsResponse, error) {
	userID, _ := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	crystalSkinData := &CrystalSkinData{}
	if err := LoadUserData(ctx, s.logger, s.db, crystalSkinData); err != nil {
		s.logger.Error("加载水晶皮肤数据失败", zap.Error(err), zap.String("user_id", userID.String()))
		return &game.GetCrystalSkinsResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	skins := make([]*game.CrystalSkinInfo, 0, len(crystalSkinData.Skins))
	for skinID, skin := range crystalSkinData.Skins {
		skins = append(skins, &game.CrystalSkinInfo{
			SkinId:    skinID,
			StarLevel: skin.StarLevel,
		})
	}

	s.logger.Info("获取水晶皮肤列表成功",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(skins)))

	return &game.GetCrystalSkinsResponse{
		Code:  0,
		Msg:   "Success",
		Skins: skins,
	}, nil
}
