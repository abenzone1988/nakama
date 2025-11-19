package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// BoxShopData 宝箱商店数据（独立存储）
type BoxShopData struct {
	BoxItemID string `json:"box_item_id"` // 当前宝箱商品ID
	BoxLevel  int32  `json:"box_level"`   // 宝箱等级
	BoxExp    int32  `json:"box_exp"`     // 宝箱经验
}

func (d *BoxShopData) GetCollection() string {
	return "shop"
}

func (d *BoxShopData) GetKey() string {
	return "box"
}

func (d *BoxShopData) Init() {
	d.BoxItemID = ""
	d.BoxLevel = 1
	d.BoxExp = 0
}

// BuyBoxItem 购买宝箱商品（独立接口）
func (s *ApiServer) BuyBoxItem(ctx context.Context, in *game.BuyBoxItemRequest) (*game.BuyBoxItemResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证购买数量
	if in.Count < 1 {
		return &game.BuyBoxItemResponse{Code: 1, Msg: "购买数量必须大于0"}, nil
	}

	// 如果使用货币购买，数量只能是1或10
	if !in.UseKey && in.Count != 1 && in.Count != 10 {
		return &game.BuyBoxItemResponse{Code: 1, Msg: "货币购买数量只能是1或10"}, nil
	}

	boxShopData := &BoxShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, boxShopData); err != nil {
		s.logger.Error("加载宝箱商店数据失败", zap.Error(err))
		// 首次初始化
		boxShopData.Init()
	}

	// 初始化宝箱商店
	s.initBoxShopStandalone(boxShopData)

	if boxShopData.BoxItemID == "" {
		return &game.BuyBoxItemResponse{Code: 2, Msg: "宝箱商店未初始化"}, nil
	}

	// 处理宝箱购买逻辑
	cost, rewards, boxLevelUp, err := s.processBuyBoxItemStandalone(ctx, userID, boxShopData, in.UseKey, in.Count)
	if err != nil {
		return &game.BuyBoxItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// 扣除货币/钥匙并发放奖励
	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	if cost != nil && (cost.Coin > 0 || cost.Gem > 0 || cost.Ad > 0) {
		// 扣除货币
		walletChangeset := make(map[string]int64)
		if cost.Coin > 0 {
			walletChangeset["coin"] = -int64(cost.Coin)
		}
		if cost.Gem > 0 {
			walletChangeset["gem"] = -int64(cost.Gem)
		}
		if cost.Ad > 0 {
			walletChangeset["ad"] = -int64(cost.Ad)
		}

		metadata, _ := json.Marshal(map[string]interface{}{"source": "box_shop"})
		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: walletChangeset,
			Metadata:  string(metadata),
		}}

		results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
		if err != nil {
			return &game.BuyBoxItemResponse{Code: 5, Msg: "货币不足"}, nil
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 发放所有奖励
	for _, reward := range rewards {
		_, invResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, "box_shop")
		if err != nil {
			s.logger.Error("发放宝箱奖励失败", zap.Error(err))
		} else if invResult != nil {
			inventoryUpdateResult = invResult
		}
	}

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, boxShopData); err != nil {
		s.logger.Error("保存宝箱商店数据失败", zap.Error(err))
		return &game.BuyBoxItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	// 合并所有奖励
	mergedReward := &game.Reward{
		Wallet: &game.Wallet{},
		Items:  []*game.Item{},
	}
	for _, r := range rewards {
		if r.Wallet != nil {
			mergedReward.Wallet.Coin += r.Wallet.Coin
			mergedReward.Wallet.Gem += r.Wallet.Gem
			mergedReward.Wallet.Ad += r.Wallet.Ad
		}
		mergedReward.Items = append(mergedReward.Items, r.Items...)
	}

	response := &game.BuyBoxItemResponse{
		Code:        0,
		Msg:         "购买成功",
		Reward:      mergedReward,
		BoxShopData: convertToProtoBoxShop(boxShopData),
		BoxLevelUp:  boxLevelUp,
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}

	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

// GetBoxShop RPC获取宝箱商店数据（独立）
func (s *ApiServer) GetBoxShop(ctx context.Context, in *emptypb.Empty) (*game.BoxShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	boxShopData := &BoxShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, boxShopData); err != nil {
		s.logger.Error("加载宝箱商店数据失败", zap.Error(err))
		// 首次初始化
		boxShopData.Init()
	}

	// 初始化宝箱商店
	s.initBoxShopStandalone(boxShopData)

	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, boxShopData); err != nil {
		s.logger.Error("保存宝箱商店数据失败", zap.Error(err))
		return nil, err
	}

	return convertToProtoBoxShop(boxShopData), nil
}

// 处理宝箱购买（独立版本）
func (s *ApiServer) processBuyBoxItemStandalone(ctx context.Context, userID uuid.UUID, boxShopData *BoxShopData, useKey bool, count int32) (*game.Wallet, []*game.Reward, bool, error) {
	var cost *game.Wallet
	boxLevelUp := false

	tplBoxItem, ok := s.template.GetTplBoxShopItem().FindByKey(boxShopData.BoxItemID)
	if !ok {
		return nil, nil, false, fmt.Errorf("未找到宝箱配置")
	}

	if useKey {
		// 使用宝箱钥匙（itemId: 20001）
		// 检查背包中的钥匙数量
		inventory, err := GetInventory(ctx, s.logger, s.db, userID)
		if err != nil {
			return nil, nil, false, fmt.Errorf("加载玩家背包失败")
		}

		keyCount := inventory["20001"]
		if keyCount < int64(count) {
			return nil, nil, false, fmt.Errorf("宝箱钥匙不足")
		}

		// 扣除钥匙的逻辑会在后面的 GrantReward 中通过负数处理
		cost = &game.Wallet{} // 使用钥匙不扣除货币
	} else {
		// 使用货币购买
		payType := parsePayType(tplBoxItem.PayType)
		if count == 10 {
			cost = calculateCost(payType, tplBoxItem.WholesalePrice)
		} else {
			cost = calculateCost(payType, tplBoxItem.RetailPrice)
		}
	}

	// 每次打开宝箱都抽取奖励
	rewards := make([]*game.Reward, 0, count)
	for i := int32(0); i < count; i++ {
		reward := s.rollBoxReward(ctx, userID, &tplBoxItem)
		if reward != nil {
			rewards = append(rewards, reward)
		}

		// 增加经验
		boxShopData.BoxExp += tplBoxItem.BoxExp

		// 检查是否可以升级
		nextLevel := boxShopData.BoxLevel + 1
		tplNextBoxShops := s.template.GetTplBoxShop().FindByFilter(func(bs TplBoxShop) bool {
			return bs.Level == nextLevel
		}).ToSlice()

		if len(tplNextBoxShops) > 0 {
			tplNextBoxShop := tplNextBoxShops[0]
			if boxShopData.BoxExp >= tplNextBoxShop.RequiredExp {
				// 升级
				boxShopData.BoxLevel = nextLevel
				boxShopData.BoxExp = 0
				boxLevelUp = true

				// 更新宝箱商品ID
				boxShopData.BoxItemID = tplNextBoxShop.Box1
				// 更新模板引用，下次开宝箱使用新等级
				if tplNewBoxItem, ok := s.template.GetTplBoxShopItem().FindByKey(tplNextBoxShop.Box1); ok {
					tplBoxItem = tplNewBoxItem
				}
			}
		}
	}

	// 如果使用钥匙，添加扣除钥匙的奖励项
	if useKey {
		keyReward := &game.Reward{
			Items: []*game.Item{{
				Id:  "20001",
				Num: -count, // 负数表示扣除
			}},
		}
		rewards = append(rewards, keyReward)
	}

	return cost, rewards, boxLevelUp, nil
}

// 初始化宝箱商店（独立）
func (s *ApiServer) initBoxShopStandalone(boxShopData *BoxShopData) {
	// 根据当前等级查找对应的宝箱配置
	tplBoxShops := s.template.GetTplBoxShop().FindByFilter(func(bs TplBoxShop) bool {
		return bs.Level == boxShopData.BoxLevel
	}).ToSlice()

	if len(tplBoxShops) == 0 {
		s.logger.Warn("未找到对应等级的宝箱配置", zap.Int32("level", boxShopData.BoxLevel))
		return
	}

	tplBoxShop := tplBoxShops[0]
	boxShopData.BoxItemID = tplBoxShop.Box1
}

// 转换宝箱商店数据
func convertToProtoBoxShop(data *BoxShopData) *game.BoxShopData {
	return &game.BoxShopData{
		BoxItemId: data.BoxItemID,
		BoxLevel:  data.BoxLevel,
		BoxExp:    data.BoxExp,
	}
}

// 抽取宝箱奖励
func (s *ApiServer) rollBoxReward(ctx context.Context, userID uuid.UUID, tplBoxItem *TplBoxShopItem) *game.Reward {
	// 解析 rewardInfo 格式：itemid_count_weight
	rewardInfo := tplBoxItem.RewardInfo
	if rewardInfo == "" {
		return nil
	}

	items := strings.Split(rewardInfo, ",")
	type RewardOption struct {
		ItemID string
		Count  int32
		Weight int32
	}

	options := make([]RewardOption, 0)
	totalWeight := int32(0)

	for _, item := range items {
		parts := strings.Split(strings.TrimSpace(item), "_")
		if len(parts) != 3 {
			continue
		}

		itemID := parts[0]
		count, _ := strconv.ParseInt(parts[1], 10, 32)
		weight, _ := strconv.ParseFloat(parts[2], 32)

		weightInt := int32(weight * 10) // 将浮点权重转换为整数
		options = append(options, RewardOption{
			ItemID: itemID,
			Count:  int32(count),
			Weight: weightInt,
		})
		totalWeight += weightInt
	}

	if len(options) == 0 || totalWeight == 0 {
		return nil
	}

	// 根据权重随机抽取
	roll := rand.Int31n(totalWeight)
	currentWeight := int32(0)

	var selectedOption *RewardOption
	for i := range options {
		currentWeight += options[i].Weight
		if roll < currentWeight {
			selectedOption = &options[i]
			break
		}
	}

	if selectedOption == nil {
		s.logger.Error("宝箱奖励抽取失败", zap.String("user_id", userID.String()), zap.String("reward_info", rewardInfo))
		return nil
	}

	// 检查是否是随机炮台（itemID = 50000）
	if selectedOption.ItemID == "50000" {
		itemID, itemCount, err := s.rollRandomTurret(ctx, userID, tplBoxItem.UnlockEquipInfo, selectedOption.Count)
		if err != nil {
			s.logger.Error("抽取随机炮台失败",
				zap.Error(err),
				zap.String("user_id", userID.String()),
				zap.String("unlock_equip_info", tplBoxItem.UnlockEquipInfo))
			return nil // 抽取失败，返回空奖励
		}

		// 检查是否是炮台（非碎片），并尝试解锁
		alreadyUnlocked, err := s.tryUnlockTurret(ctx, userID, itemID)
		if err != nil {
			// 非炮台类型（如碎片），直接返回
			s.logger.Debug("物品非炮台，直接发放",
				zap.String("item_id", itemID),
				zap.Int32("count", itemCount))
		} else if alreadyUnlocked {
			// 炮台已解锁，转换为碎片（10个）
			debrisID := strings.TrimPrefix(itemID, "EQ")
			s.logger.Info("炮台已解锁，转换为碎片",
				zap.String("user_id", userID.String()),
				zap.String("turret_id", itemID),
				zap.String("debris_id", debrisID),
				zap.Int32("count", 10))
			return &game.Reward{
				Items: []*game.Item{{
					Id:  debrisID,
					Num: 10,
				}},
			}
		}

		return &game.Reward{
			Items: []*game.Item{{
				Id:  itemID,
				Num: itemCount,
			}},
		}
	}

	// 检查普通奖励是否是炮台（itemType = 8）
	finalItemID := selectedOption.ItemID
	finalItemCount := selectedOption.Count

	alreadyUnlocked, err := s.tryUnlockTurret(ctx, userID, selectedOption.ItemID)
	if err != nil {
		s.logger.Debug("物品非炮台或解锁失败",
			zap.String("item_id", selectedOption.ItemID),
			zap.Error(err))
	} else if alreadyUnlocked {
		// 炮台已解锁，转换为碎片（10个）
		debrisID := strings.TrimPrefix(selectedOption.ItemID, "EQ")
		s.logger.Info("炮台已解锁，转换为碎片",
			zap.String("user_id", userID.String()),
			zap.String("turret_id", selectedOption.ItemID),
			zap.String("debris_id", debrisID),
			zap.Int32("count", 10))
		finalItemID = debrisID
		finalItemCount = 10
	}

	// 返回奖励
	return &game.Reward{
		Items: []*game.Item{{
			Id:  finalItemID,
			Num: finalItemCount,
		}},
	}
}

// 随机抽取炮台
// 返回值：itemID（炮台ID或碎片ID）, count（数量）, error
func (s *ApiServer) rollRandomTurret(ctx context.Context, userID uuid.UUID, unlockEquipInfo string, defaultCount int32) (string, int32, error) {
	if unlockEquipInfo == "" {
		s.logger.Error("宝箱奖励炮台配置为空", zap.String("user_id", userID.String()))
		return "", 0, fmt.Errorf("宝箱奖励炮台配置为空")
	}

	// 解析 unlockEquipInfo 格式：id_weight
	items := strings.Split(unlockEquipInfo, ",")
	type TurretOption struct {
		TurretID string
		Weight   float32
	}

	options := make([]TurretOption, 0)
	totalWeight := float32(0)

	for _, item := range items {
		parts := strings.Split(strings.TrimSpace(item), "_")
		if len(parts) != 2 {
			continue
		}

		turretID := parts[0]
		weight, _ := strconv.ParseFloat(parts[1], 32)

		options = append(options, TurretOption{
			TurretID: turretID,
			Weight:   float32(weight),
		})
		totalWeight += float32(weight)
	}

	if len(options) == 0 {
		s.logger.Error("炮台配置解析失败",
			zap.String("user_id", userID.String()),
			zap.String("unlock_equip_info", unlockEquipInfo))
		return "", 0, fmt.Errorf("炮台配置解析失败")
	}

	// 根据权重随机抽取
	roll := rand.Float32() * totalWeight
	currentWeight := float32(0)

	for _, option := range options {
		currentWeight += option.Weight
		if roll < currentWeight {
			// 如果抽到的还是 50000，则从已经拥有的炮台中随机选择一个，兑换成10个碎片
			if option.TurretID == "50000" {
				ownedTurret := s.getRandomOwnedTurret(ctx, userID)
				if ownedTurret == "" {
					s.logger.Error("玩家没有已解锁的炮台，无法兑换碎片",
						zap.String("user_id", userID.String()),
						zap.String("抽取结果", "50000"))
					return "", 0, fmt.Errorf("玩家没有已解锁的炮台")
				}
				// 返回碎片ID，去掉炮台ID前缀"EQ"，数量固定为10
				debrisID := strings.TrimPrefix(ownedTurret, "EQ")
				s.logger.Debug("抽到50000，兑换炮台碎片",
					zap.String("user_id", userID.String()),
					zap.String("owned_turret", ownedTurret),
					zap.String("debris_id", debrisID),
					zap.Int32("count", 10))
				return debrisID, 10, nil
			}
			// 正常炮台，使用默认数量
			return option.TurretID, defaultCount, nil
		}
	}

	// 理论上不应该到达这里
	s.logger.Error("炮台随机抽取失败：未找到匹配项",
		zap.String("user_id", userID.String()),
		zap.Float32("roll", roll),
		zap.Float32("total_weight", totalWeight))
	return "", 0, fmt.Errorf("炮台随机抽取失败")
}

// 获取玩家拥有的随机炮台
func (s *ApiServer) getRandomOwnedTurret(ctx context.Context, userID uuid.UUID) string {
	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		s.logger.Error("获取玩家元数据失败", zap.Error(err))
		return ""
	}

	if userMeta.Equip == nil {
		return ""
	}

	// 找到所有已解锁的炮台（以 EQ 开头）
	ownedTurrets := make([]string, 0)
	for turretID := range userMeta.Equip.UnlockEquips {
		if strings.HasPrefix(turretID, "EQ") {
			ownedTurrets = append(ownedTurrets, turretID)
		}
	}

	if len(ownedTurrets) == 0 {
		return ""
	}

	// 随机选择一个
	idx := rand.Intn(len(ownedTurrets))
	return ownedTurrets[idx]
}

// tryUnlockTurret 尝试解锁炮台（如果物品是炮台类型）
// 返回值：alreadyUnlocked（是否已解锁）, error（错误，如果物品不是炮台）
func (s *ApiServer) tryUnlockTurret(ctx context.Context, userID uuid.UUID, itemID string) (bool, error) {
	// 检查物品是否存在于配置表中
	tplItem, ok := s.template.GetTplItem().FindByKey(itemID)
	if !ok {
		// 物品不存在，可能是碎片等非配置表物品
		return false, fmt.Errorf("物品未在配置表中: %s", itemID)
	}

	// 检查是否是炮台类型（itemType = 8）
	if tplItem.ItemType != 8 {
		return false, fmt.Errorf("物品不是炮台类型: %s (itemType=%d)", itemID, tplItem.ItemType)
	}

	// 检查是否已解锁
	alreadyUnlocked := false

	// 更新玩家装备数据，解锁炮台
	err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		if meta.Equip == nil {
			meta.Equip = &game.EquipData{
				BattleEquips: []string{},
				UnlockEquips: make(map[string]int32),
			}
		}

		// 检查是否已解锁
		if _, exists := meta.Equip.UnlockEquips[itemID]; exists {
			alreadyUnlocked = true
			s.logger.Debug("炮台已解锁",
				zap.String("user_id", userID.String()),
				zap.String("item_id", itemID))
			return nil
		}

		// 解锁炮台，初始等级为1
		meta.Equip.UnlockEquips[itemID] = 1

		s.logger.Info("成功解锁新炮台",
			zap.String("user_id", userID.String()),
			zap.String("item_id", itemID),
			zap.String("item_name", tplItem.Name))

		return nil
	})

	if err != nil {
		s.logger.Error("更新玩家装备数据失败",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("item_id", itemID))
		return false, err
	}

	return alreadyUnlocked, nil
}
