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

	// 处理宝箱购买逻辑（包含钥匙扣除）
	cost, rewards, boxLevelUp, keyInventoryResult, err := s.processBuyBoxItemStandalone(ctx, userID, boxShopData, in.UseKey, in.Count)
	if err != nil {
		return &game.BuyBoxItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// 扣除货币并发放奖励
	var walletUpdateResult *game.WalletUpdateResult

	// 收集所有背包更新（从扣除钥匙开始）
	allInventoryUpdates := make(map[string]int64)

	// 如果使用钥匙，先记录钥匙扣除的更新
	if keyInventoryResult != nil && keyInventoryResult.Updated != nil {
		for _, item := range keyInventoryResult.Updated {
			allInventoryUpdates[item.Id] = int64(item.Num)
		}
	}

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

	// 发放所有奖励并累加背包更新
	for _, reward := range rewards {
		_, invResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "box_shop")
		if err != nil {
			s.logger.Error("发放宝箱奖励失败", zap.Error(err))
		} else if invResult != nil && invResult.Updated != nil {
			// 累加每次奖励的背包更新
			for _, item := range invResult.Updated {
				allInventoryUpdates[item.Id] = int64(item.Num)
			}
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

	// 返回合并后的背包更新（包含钥匙扣除和奖励）
	if len(allInventoryUpdates) > 0 {
		response.InventoryUpdated = convertMapInt64ToItems(allInventoryUpdates)
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
func (s *ApiServer) processBuyBoxItemStandalone(ctx context.Context, userID uuid.UUID, boxShopData *BoxShopData, useKey bool, count int32) (*game.Wallet, []*game.Reward, bool, *game.InventoryUpdateResult, error) {
	var cost *game.Wallet
	boxLevelUp := false
	var keyInventoryResult *game.InventoryUpdateResult

	tplBoxItem, ok := s.template.GetTplBoxShopItem().FindByKey(boxShopData.BoxItemID)
	if !ok {
		return nil, nil, false, nil, fmt.Errorf("未找到宝箱配置")
	}

	if useKey {
		// 使用宝箱钥匙（itemId: 20001）
		// 检查并扣除背包中的钥匙
		inventory, err := GetInventory(ctx, s.logger, s.db, userID)
		if err != nil {
			return nil, nil, false, nil, fmt.Errorf("加载玩家背包失败")
		}

		keyCount := inventory["20001"]
		if keyCount < int64(count) {
			return nil, nil, false, nil, fmt.Errorf("宝箱钥匙不足")
		}

		// 直接扣除钥匙并保存更新结果
		results, err := UpdateInventories(ctx, s.logger, s.db, []*inventoryUpdate{
			{
				UserID:    userID,
				Changeset: map[string]int64{"20001": -int64(count)},
				Metadata:  `{"reason": "box_shop_use_key"}`,
			},
		}, true)
		if err != nil {
			return nil, nil, false, nil, fmt.Errorf("扣除宝箱钥匙失败")
		}

		// 保存钥匙扣除的背包更新结果
		if len(results) > 0 {
			keyInventoryResult = &game.InventoryUpdateResult{
				Previous: convertMapInt64ToItems(results[0].Previous),
				Updated:  convertMapInt64ToItems(results[0].Updated),
			}
		}

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

	return cost, rewards, boxLevelUp, keyInventoryResult, nil
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
	// 返回奖励
	return &game.Reward{
		Items: []*game.Item{{
			Id:  selectedOption.ItemID,
			Num: selectedOption.Count,
		}},
	}
}
