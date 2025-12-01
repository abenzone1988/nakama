package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetShopData RPC获取所有商店数据
func (s *ApiServer) GetShopData(ctx context.Context, in *emptypb.Empty) (*game.ShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	shopData := &ShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, shopData); err != nil {
		s.logger.Error("加载商店数据失败", zap.Error(err))
		return nil, err
	}

	// 初始化基于TplShop的常规商店（每个商店独立检查刷新时间）
	s.initCommonShop(ctx, userID, shopData.Shops, game.ShopType_SHOP_DAILY, "Daily")
	s.initCommonShop(ctx, userID, shopData.Shops, game.ShopType_SHOP_COIN, "Coin")
	s.initCommonShop(ctx, userID, shopData.Shops, game.ShopType_SHOP_STRENGTH, "Strength")

	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
		s.logger.Error("保存商店数据失败", zap.Error(err))
		return nil, err
	}

	return convertToProtoShopData(shopData), nil
}

// initCommonShop 统一的商店初始化（支持随机商品+固定商品）
func (s *ApiServer) initCommonShop(ctx context.Context, userID uuid.UUID, shops map[game.ShopType]*ShopInfo, shopType game.ShopType, configKey string) {
	// 初始化商店结构
	if shops[shopType] == nil {
		shops[shopType] = &ShopInfo{
			ShopType:     shopType,
			Items:        []*ShopItem{},
			CanRefresh:   false, // 默认不可刷新
			RefreshCount: 0,
		}
	}

	shop := shops[shopType]

	// 检查是否需要刷新
	needRefresh := false
	now := time.Now().UTC()

	if len(shop.Items) == 0 {
		needRefresh = true
	}

	if !shop.NextRefreshTime.IsZero() && now.After(shop.NextRefreshTime) {
		needRefresh = true
	}
	// 如果需要刷新
	if needRefresh {
		// 清空商品列表
		shop.Items = []*ShopItem{}

		// 从配置表中读取商店配置
		tplShop, ok := s.template.GetTplShop().FindByKey(configKey)
		if !ok {
			s.logger.Error("未找到商店配置", zap.String("shop_config_key", configKey))
			return
		}

		// 判断是否支持手动刷新
		if tplShop.HandleRefreshTimes > 0 {
			shop.CanRefresh = true
			shop.RefreshCount = tplShop.HandleRefreshTimes
		} else {
			shop.CanRefresh = false
			shop.RefreshCount = 0
		}

		// 1. 添加固定商品（如果配置了）
		s.addPermanentShopItems(shop, &tplShop)

		// 2. 添加随机商品（如果配置了）
		s.addRandomShopItems(ctx, userID, shop, &tplShop)

		nextRefreshTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		shop.NextRefreshTime = nextRefreshTime
	}
}

// forceRefreshShop 强制刷新商店（用于手动刷新）
func (s *ApiServer) forceRefreshShop(ctx context.Context, userID uuid.UUID, shops map[game.ShopType]*ShopInfo, shopType game.ShopType, tplShop *TplShop) {
	shop := shops[shopType]
	if shop == nil {
		shop = &ShopInfo{
			ShopType:     shopType,
			Items:        []*ShopItem{},
			CanRefresh:   tplShop.HandleRefreshTimes > 0,
			RefreshCount: tplShop.HandleRefreshTimes,
		}
		shops[shopType] = shop
	}

	// 1. 从现有商品中筛选出固定商品（手动刷新时只保留已存在的固定商品）
	permanentIDs := parsePermanentProductIDs(tplShop.PermanentProduct)

	// 从现有商品中提取固定商品
	existingPermanentMap := make(map[string]*ShopItem)
	for _, item := range shop.Items {
		// 检查该商品ID是否属于固定商品配置
		if _, ok := s.template.GetTplShopPermanentItem().FindByKey(item.ShopItemID); ok {
			existingPermanentMap[item.ShopItemID] = item
		}
	}

	// 按配置顺序保留已存在的固定商品
	newItems := make([]*ShopItem, 0, len(permanentIDs))
	for _, id := range permanentIDs {
		if id == "" {
			continue
		}

		// 只使用已存在的固定商品，不创建新的
		if existing, exists := existingPermanentMap[id]; exists {
			newItems = append(newItems, existing)
		}
	}

	shop.Items = newItems

	// 2. 添加随机商品（如果配置了）
	s.addRandomShopItems(ctx, userID, shop, tplShop)

}

// addRandomShopItems 添加随机商品
func (s *ApiServer) addRandomShopItems(ctx context.Context, userID uuid.UUID, shop *ShopInfo, tplShop *TplShop) {
	// 如果没有配置随机商品数量，直接返回
	if tplShop.FreeProductCount == 0 && tplShop.AdProductCount == 0 &&
		tplShop.CoinProductCount == 0 && tplShop.GemProductCount == 0 {
		return
	}

	allDailyItems := s.template.GetTplShopDailyItem().FindAll().ToSlice()

	// 加载玩家装备数据，用于过滤已拥有装备的碎片
	equipData := &EquipData{}
	if err := LoadData(ctx, s.logger, s.db, userID, equipData); err != nil {
		s.logger.Warn("加载装备数据失败，跳过碎片过滤", zap.Error(err))
		equipData = nil
	}

	// 过滤掉未拥有装备的碎片类型商品
	filteredDailyItems := make([]TplShopDailyItem, 0, len(allDailyItems))
	for _, item := range allDailyItems {
		// 检查是否为碎片类型（类型4）
		tplItem, ok := s.template.GetTplItem().FindByKey(item.Item)
		if !ok {
			s.logger.Warn("找不到item配置 过滤掉该商品", zap.String("id", item.Item))
			// 找不到item配置，过滤掉该商品
			continue
		}

		// 如果不是碎片类型，直接保留
		if tplItem.ItemType != ItemType_EquipmentFragment {
			filteredDailyItems = append(filteredDailyItems, item)
			s.logger.Debug("不是碎片类型，直接保留", zap.String("id", item.Item))
			continue
		}

		// 如果是碎片类型，检查玩家是否已拥有对应装备
		if equipData == nil {
			s.logger.Debug("装备数据加载失败，无法判断，过滤掉碎片商品", zap.String("id", item.Item))
			// 装备数据加载失败，无法判断，过滤掉碎片商品
			continue
		}

		equipID := "EQ" + item.Item
		if _, exists := equipData.UnlockEquips[equipID]; !exists {
			s.logger.Debug("玩家未拥有该装备，过滤掉该碎片商品", zap.String("id", item.Item))
			// 玩家未拥有该装备，过滤掉该碎片商品
			continue
		}

		// 保留该商品
		filteredDailyItems = append(filteredDailyItems, item)
		s.logger.Debug("保留该商品", zap.String("id", item.Item))
	}

	allDailyItems = filteredDailyItems

	// 按支付类型分类
	freeItems := []TplShopDailyItem{}
	adItems := []TplShopDailyItem{}
	coinItems := []TplShopDailyItem{}
	gemItems := []TplShopDailyItem{}

	for _, item := range allDailyItems {
		if item.CanFree == 1 {
			freeItems = append(freeItems, item)
		}
		if item.CanAd == 1 {
			adItems = append(adItems, item)
		}
		if item.CanCoin == 1 {
			coinItems = append(coinItems, item)
		}
		if item.CanGem == 1 {
			gemItems = append(gemItems, item)
		}
	}

	// 添加免费商品
	for i := int32(0); i < tplShop.FreeProductCount; i++ {
		if item := getRandomDailyItem(freeItems); item != nil {
			shopItem := &ShopItem{
				ShopItemID:  item.ID,
				ItemID:      item.Item,
				PayType:     game.PayType_FREE,
				Price:       0,
				Discount:    0,
				ItemCount:   item.Num,
				MaxBuyCount: 1,
				BoughtCount: 0,
			}
			shop.Items = append(shop.Items, shopItem)
		}
	}

	// 添加广告商品
	for i := int32(0); i < tplShop.AdProductCount; i++ {
		if item := getRandomDailyItem(adItems); item != nil {
			shopItem := &ShopItem{
				ShopItemID:  item.ID,
				ItemID:      item.Item,
				PayType:     game.PayType_AD,
				Price:       0,
				Discount:    0,
				ItemCount:   item.Num,
				MaxBuyCount: 1,
				BoughtCount: 0,
			}
			shop.Items = append(shop.Items, shopItem)
		}
	}

	// 添加金币商品
	for i := int32(0); i < tplShop.CoinProductCount; i++ {
		if item := getRandomDailyItem(coinItems); item != nil {
			discount := parseDiscountRate(tplShop.CoinProductDiscountRate)
			shopItem := &ShopItem{
				ShopItemID:  item.ID,
				ItemID:      item.Item,
				PayType:     game.PayType_COIN,
				Price:       item.CoinPrice,
				Discount:    discount,
				ItemCount:   item.Num,
				MaxBuyCount: 1,
				BoughtCount: 0,
			}
			shop.Items = append(shop.Items, shopItem)
		}
	}

	// 添加钻石商品
	for i := int32(0); i < tplShop.GemProductCount; i++ {
		if item := getRandomDailyItem(gemItems); item != nil {
			discount := parseDiscountRate(tplShop.GemProductDiscountRate)
			shopItem := &ShopItem{
				ShopItemID:  item.ID,
				ItemID:      item.Item,
				PayType:     game.PayType_GEM,
				Price:       item.GemPrice,
				Discount:    discount,
				ItemCount:   item.Num,
				MaxBuyCount: 1,
				BoughtCount: 0,
			}
			shop.Items = append(shop.Items, shopItem)
		}
	}
}

// 添加固定商品
func (s *ApiServer) addPermanentShopItems(shop *ShopInfo, tplShop *TplShop) {
	permanentIDs := parsePermanentProductIDs(tplShop.PermanentProduct)
	for _, permanentID := range permanentIDs {
		if permanentID == "" {
			continue
		}

		tplPermanent, ok := s.template.GetTplShopPermanentItem().FindByKey(permanentID)
		if !ok {
			s.logger.Warn("未找到固定商品配置", zap.String("item_id", permanentID))
			continue
		}

		shopItem := s.createPermanentShopItem(&tplPermanent)
		shop.Items = append(shop.Items, shopItem)
	}
}

func (s *ApiServer) createPermanentShopItem(tplPermanent *TplShopPermanentItem) *ShopItem {
	paymentStages := parsePaymentStages(tplPermanent.PayType, tplPermanent.SalePrice, tplPermanent.LimitTimes)

	var defaultPayType game.PayType = game.PayType_COIN
	var defaultPrice int32
	var totalLimitCount int32

	if len(paymentStages) > 0 {
		defaultPayType = paymentStages[0].PayType
		defaultPrice = paymentStages[0].Price
		for _, stage := range paymentStages {
			totalLimitCount += stage.LimitCount
		}
	}

	return &ShopItem{
		ShopItemID:    tplPermanent.ID,
		ItemID:        tplPermanent.Item,
		PayType:       defaultPayType,
		Price:         defaultPrice,
		Discount:      0,
		ItemCount:     tplPermanent.Num,
		MaxBuyCount:   totalLimitCount,
		BoughtCount:   0,
		PaymentStages: paymentStages,
	}
}

func parsePermanentProductIDs(permanentProduct string) []string {
	if permanentProduct == "" {
		return []string{}
	}

	parts := strings.Split(permanentProduct, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// getRandomDailyItem 随机获取每日商品
func getRandomDailyItem(items []TplShopDailyItem) *TplShopDailyItem {
	if len(items) == 0 {
		return nil
	}
	idx := rand.Intn(len(items))
	return &items[idx]
}

// parseDiscountRate 解析折扣配置（格式：折扣_概率，如 "8_50,10_50" 表示80%折扣50%概率，无折扣50%概率）
func parseDiscountRate(discountInfo string) int32 {
	if discountInfo == "" {
		return 0 // 无折扣
	}

	discountParts := strings.Split(discountInfo, ",")
	totalRate := 0
	type discountConfig struct {
		discount int32
		maxRate  int32
	}
	discounts := []discountConfig{}

	for _, part := range discountParts {
		info := strings.Split(part, "_")
		if len(info) == 2 {
			discount, _ := strconv.ParseInt(info[0], 10, 32)
			rate, _ := strconv.ParseInt(info[1], 10, 32)
			totalRate += int(rate)
			discounts = append(discounts, discountConfig{
				discount: int32(discount),
				maxRate:  int32(totalRate),
			})
		}
	}

	if len(discounts) == 0 {
		return 0
	}

	value := rand.Intn(100)
	for _, dc := range discounts {
		if value < int(dc.maxRate) {
			return dc.discount
		}
	}

	return 0
}

// BuyShopItem 统一的商店购买接口
func (s *ApiServer) BuyShopItem(ctx context.Context, in *game.BuyShopItemRequest) (*game.BuyShopItemResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载商店数据
	shopData := &ShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, shopData); err != nil {
		s.logger.Error("加载商店数据失败", zap.Error(err))
		return &game.BuyShopItemResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	key := in.ShopType
	shop, ok := shopData.Shops[key]
	if !ok {
		return &game.BuyShopItemResponse{Code: 1, Msg: "商店不存在"}, nil
	}

	// 通过 index 查找商品
	index := int(in.Index)
	if index < 0 || index >= len(shop.Items) {
		return &game.BuyShopItemResponse{Code: 2, Msg: "商品不存在"}, nil
	}
	item := shop.Items[index]
	if item == nil {
		return &game.BuyShopItemResponse{Code: 2, Msg: "商品不存在"}, nil
	}

	// 检查购买次数
	if item.MaxBuyCount > 0 && item.BoughtCount >= item.MaxBuyCount {
		return &game.BuyShopItemResponse{Code: 3, Msg: "购买次数已达上限"}, nil
	}

	// 计算消耗和奖励
	var cost *game.Wallet
	var reward *game.Reward
	var err error

	// 常规商店只处理 Daily、Coin、Strength
	cost, reward, err = s.processBuyNormalItem(item, in.AdWatched)

	if err != nil {
		return &game.BuyShopItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// 使用统一的支付和奖励处理函数
	source := fmt.Sprintf("shop_%s_%s", in.ShopType.String(), item.ShopItemID)
	metadata := map[string]interface{}{
		"shop_type": in.ShopType.String(),
		"item_id":   item.ShopItemID,
	}

	walletUpdateResult, inventoryUpdateResult, err := s.processPaymentAndReward(ctx, cost, reward, source, metadata)
	if err != nil {
		s.logger.Error("处理支付和奖励失败", zap.Error(err))
		return &game.BuyShopItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 更新购买次数
	item.BoughtCount++

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
		s.logger.Error("保存商店数据失败", zap.Error(err))
		return &game.BuyShopItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	response := &game.BuyShopItemResponse{
		Code:     0,
		Msg:      "购买成功",
		Reward:   reward,
		ShopItem: convertToProtoShopItem(item, in.Index),
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}

	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

// RefreshShop 手动刷新商店
func (s *ApiServer) RefreshShop(ctx context.Context, in *game.RefreshShopRequest) (*game.RefreshShopResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载商店数据
	shopData := &ShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, shopData); err != nil {
		s.logger.Error("加载商店数据失败", zap.Error(err))
		return &game.RefreshShopResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	shop, ok := shopData.Shops[in.ShopType]
	if !ok {
		return &game.RefreshShopResponse{Code: 1, Msg: "商店不存在"}, nil
	}

	// 检查是否支持手动刷新
	if !shop.CanRefresh {
		return &game.RefreshShopResponse{Code: 2, Msg: "该商店不支持手动刷新"}, nil
	}

	tplShop, found := s.template.GetTplShop().FindByKey("Daily")

	if !found {
		return &game.RefreshShopResponse{Code: 4, Msg: "未找到商店配置"}, nil
	}

	// 检查刷新次数限制
	if shop.RefreshCount <= 0 {
		return &game.RefreshShopResponse{Code: 5, Msg: "已达到最大刷新次数"}, nil
	}

	// 执行刷新（强制刷新商店）
	s.forceRefreshShop(ctx, userID, shopData.Shops, in.ShopType, &tplShop)

	// 减少刷新次数
	shop.RefreshCount--

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
		s.logger.Error("保存商店数据失败", zap.Error(err))
		return &game.RefreshShopResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	return &game.RefreshShopResponse{
		Code:     0,
		Msg:      "刷新成功",
		ShopData: convertToProtoSingleShop(shop),
	}, nil
}

// 处理普通商品购买
func (s *ApiServer) processBuyNormalItem(item *ShopItem, adWatched bool) (*game.Wallet, *game.Reward, error) {
	// 如果配置了分段购买，根据已购买次数确定当前支付阶段
	var payType game.PayType
	var price int32

	if len(item.PaymentStages) > 0 {
		// 根据已购买次数找到当前所处的阶段
		currentBought := item.BoughtCount
		accumulatedLimit := int32(0)

		foundStage := false
		for _, stage := range item.PaymentStages {
			if stage.LimitCount < 0 || currentBought < accumulatedLimit+stage.LimitCount {
				payType = stage.PayType
				price = stage.Price
				foundStage = true
				break
			}
			if stage.LimitCount > 0 {
				accumulatedLimit += stage.LimitCount
			}
		}

		if !foundStage {
			return nil, nil, fmt.Errorf("已超过所有阶段的购买限制")
		}
	} else {
		// 没有配置分段购买，使用原有逻辑
		payType = item.PayType
		price = item.Price
		if item.Discount > 0 {
			price = int32(float32(price) * float32(item.Discount) * 0.1)
		}
	}

	cost := calculateCost(payType, price)

	if adWatched {
		cost.Ad = 0
	}

	reward := &game.Reward{
		Items: []*game.Item{{
			Id:  item.ItemID,
			Num: item.ItemCount,
		}},
	}

	return cost, reward, nil
}

// convertToProtoShopData 转换商店数据为proto ShopData
func convertToProtoShopData(data *ShopData) *game.ShopData {
	protoShops := make([]*game.SingleShopData, 0)

	for _, shop := range data.Shops {
		protoShop := convertToProtoSingleShop(shop)
		protoShops = append(protoShops, protoShop)
	}

	return &game.ShopData{
		Shops: protoShops,
	}
}

// convertToProtoSingleShop 转换单个商店为proto格式
func convertToProtoSingleShop(shop *ShopInfo) *game.SingleShopData {
	protoItems := make([]*game.ShopItem, 0, len(shop.Items))
	index := int32(0)
	for _, item := range shop.Items {
		protoItems = append(protoItems, convertToProtoShopItem(item, index))
		index++
	}

	protoShop := &game.SingleShopData{
		ShopType:        shop.ShopType,
		Items:           protoItems,
		CanRefresh:      shop.CanRefresh,
		NextRefreshTime: shop.NextRefreshTime.Format(time.RFC3339),
	}

	return protoShop
}

func convertToProtoShopItem(item *ShopItem, index int32) *game.ShopItem {
	displayPayType := item.PayType
	displayPrice := item.Price
	if len(item.PaymentStages) > 0 {
		currentBought := item.BoughtCount
		accumulatedLimit := int32(0)
		for _, stage := range item.PaymentStages {
			if stage.LimitCount < 0 || currentBought < accumulatedLimit+stage.LimitCount {
				displayPayType = stage.PayType
				displayPrice = stage.Price
				break
			}
			if stage.LimitCount > 0 {
				accumulatedLimit += stage.LimitCount
			}
		}
	}

	return &game.ShopItem{
		Id:          item.ShopItemID,
		ItemId:      item.ItemID,
		PayType:     displayPayType,
		Price:       displayPrice,
		Discount:    item.Discount,
		ItemCount:   item.ItemCount,
		MaxBuyCount: item.MaxBuyCount,
		BoughtCount: item.BoughtCount,
		Index:       index,
	}
}

func parsePayType(payTypeStr string) game.PayType {
	switch strings.ToLower(payTypeStr) {
	case "coin":
		return game.PayType_COIN
	case "gem":
		return game.PayType_GEM
	case "ad":
		return game.PayType_AD
	case "iap":
		return game.PayType_IAP
	case "free":
		return game.PayType_FREE
	default:
		return game.PayType_COIN
	}
}

// parsePaymentStages 解析支付阶段（支持多种支付方式）
// 例如：payType="free,ad"，salePrice=[0,0]，limitTimes=[1,3]
// 表示：前1次免费，接下来3次看广告
func parsePaymentStages(payTypeStr string, salePrices []int32, limitTimes []int32) []PaymentStage {
	if payTypeStr == "" {
		return nil
	}

	payTypes := strings.Split(payTypeStr, ",")
	stages := make([]PaymentStage, 0, len(payTypes))

	for i, pt := range payTypes {
		pt = strings.TrimSpace(pt)
		if pt == "" {
			continue
		}

		stage := PaymentStage{
			PayType: parsePayType(pt),
		}

		// 获取对应的价格
		if i < len(salePrices) {
			stage.Price = salePrices[i]
		}

		// 获取对应的购买次数限制
		if i < len(limitTimes) {
			stage.LimitCount = limitTimes[i]
		}

		stages = append(stages, stage)
	}

	return stages
}

func calculateCost(payType game.PayType, amount int32) *game.Wallet {
	cost := &game.Wallet{}

	switch payType {
	case game.PayType_COIN:
		cost.Coin = amount
	case game.PayType_GEM:
		cost.Gem = amount
	case game.PayType_AD: //使用广告购买默认扣1
		cost.Ad = 1
	}

	return cost
}

// processPaymentAndReward 处理支付和奖励发放
// metadata 为可选的额外元数据，会合并到 source 中
func (s *ApiServer) processPaymentAndReward(ctx context.Context, cost *game.Wallet, reward *game.Reward, source string, metadata map[string]interface{}) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	// 扣除货币
	var walletUpdateResult *game.WalletUpdateResult
	if cost != nil && (cost.Coin > 0 || cost.Gem > 0 || cost.Ad > 0) {
		userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

		// 构建 metadata
		meta := map[string]interface{}{"source": source}
		for k, v := range metadata {
			meta[k] = v
		}
		metadataBytes, _ := json.Marshal(meta)

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

		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: walletChangeset,
			Metadata:  string(metadataBytes),
		}}

		results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
		if err != nil {
			return nil, nil, fmt.Errorf("货币不足")
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 发放奖励（GrantReward 返回的钱包结果包含最终状态，包含扣除和奖励后的余额）
	var inventoryUpdateResult *game.InventoryUpdateResult
	if reward != nil {
		rewardWalletResult, rewardInventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, source)
		if err != nil {
			return walletUpdateResult, nil, err
		}

		// 如果奖励中包含钱包奖励，使用 GrantReward 返回的最终钱包状态
		// 否则使用扣除货币后的钱包状态
		if rewardWalletResult != nil {
			walletUpdateResult = rewardWalletResult
		}
		inventoryUpdateResult = rewardInventoryResult
	}

	return walletUpdateResult, inventoryUpdateResult, nil
}
