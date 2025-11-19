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

// ShopData 商店数据存储结构（包含所有类型商店）
type ShopData struct {
	DateTime time.Time            `json:"date_time"`
	Shops    map[string]*ShopInfo `json:"shops"` // key: ShopType string
}

func (d *ShopData) GetCollection() string {
	return "shop"
}

func (d *ShopData) GetKey() string {
	return "data"
}

func (d *ShopData) Init() {
	d.DateTime = time.Now().UTC()
	d.Shops = make(map[string]*ShopInfo)
}

// ChapterShopData 章节商店数据（独立存储）
type ChapterShopData struct {
	Items []*ShopItem `json:"items"`
}

func (d *ChapterShopData) GetCollection() string {
	return "shop"
}

func (d *ChapterShopData) GetKey() string {
	return "chapter"
}

func (d *ChapterShopData) Init() {
	d.Items = []*ShopItem{}
}

// GemShopData 钻石商店数据（独立存储）
type GemShopData struct {
	Items []*ShopItem `json:"items"`
}

func (d *GemShopData) GetCollection() string {
	return "shop"
}

func (d *GemShopData) GetKey() string {
	return "gem"
}

func (d *GemShopData) Init() {
	d.Items = []*ShopItem{}
}

type ShopInfo struct {
	ShopType        game.ShopType `json:"shop_type"`
	Items           []*ShopItem   `json:"items"`
	CanRefresh      bool          `json:"can_refresh"`
	RefreshCount    int32         `json:"refresh_count"`
	NextRefreshTime time.Time     `json:"next_refresh_time"`
}

type ShopItem struct {
	Index       int32        `json:"index"`
	ShopItemID  string       `json:"shop_item_id"`
	ItemID      string       `json:"item_id"`
	PayType     game.PayType `json:"pay_type"`
	Price       int32        `json:"price"`
	Discount    int32        `json:"discount"`
	ItemCount   int32        `json:"item_count"`
	MaxBuyCount int32        `json:"max_buy_count"`
	BoughtCount int32        `json:"bought_count"`
	// 分段购买配置（用于支持多种支付方式，如先免费再广告）
	PaymentStages []PaymentStage `json:"payment_stages,omitempty"`
}

// PaymentStage 支付阶段配置
type PaymentStage struct {
	PayType    game.PayType `json:"pay_type"`    // 支付类型
	Price      int32        `json:"price"`       // 价格
	LimitCount int32        `json:"limit_count"` // 该阶段购买次数限制
}

// checkNeedRefresh 检查是否需要刷新（跨天）
func checkNeedRefresh(lastTime time.Time) bool {
	if lastTime.IsZero() {
		return true
	}

	now := time.Now().UTC()
	lastDay := time.Date(lastTime.Year(), lastTime.Month(), lastTime.Day(), 0, 0, 0, 0, time.UTC)
	nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	return nowDay.After(lastDay)
}

// GetShopData RPC获取所有商店数据
func (s *ApiServer) GetShopData(ctx context.Context, in *emptypb.Empty) (*game.ShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	shopData := &ShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, shopData); err != nil {
		s.logger.Error("加载商店数据失败", zap.Error(err))
		return nil, err
	}

	// 检查是否需要跨天刷新
	needRefresh := checkNeedRefresh(shopData.DateTime)
	if needRefresh {
		s.logger.Info("检测到需要刷新商店", zap.Time("last_date", shopData.DateTime))
		shopData.DateTime = time.Now().UTC()

		// 重置每日商店的手动刷新次数
		if dailyShop, ok := shopData.Shops[game.ShopType_SHOP_DAILY.String()]; ok {
			dailyShop.RefreshCount = 0
		}
	}

	// 只初始化基于TplShop的常规商店
	s.initShop(shopData, game.ShopType_SHOP_DAILY, needRefresh)
	s.initShop(shopData, game.ShopType_SHOP_COIN, false)
	s.initShop(shopData, game.ShopType_SHOP_STRENGTH, false)

	//if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
	//	s.logger.Error("保存商店数据失败", zap.Error(err))
	//	return nil, err
	//}

	return convertToProtoShopData(shopData), nil
}

// GetChapterShop RPC获取章节商店数据（IAP）
func (s *ApiServer) GetChapterShop(ctx context.Context, in *emptypb.Empty) (*game.ChapterShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	chapterShopData := &ChapterShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败", zap.Error(err))
		// 首次初始化
		chapterShopData.Init()
	}

	// 初始化章节商店
	s.initChapterShopStandalone(chapterShopData)

	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, chapterShopData); err != nil {
		s.logger.Error("保存章节商店数据失败", zap.Error(err))
		return nil, err
	}

	return convertToProtoChapterShop(chapterShopData), nil
}

// GetGemShop RPC获取钻石商店数据（IAP）
func (s *ApiServer) GetGemShop(ctx context.Context, in *emptypb.Empty) (*game.GemShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	gemShopData := &GemShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, gemShopData); err != nil {
		s.logger.Error("加载钻石商店数据失败", zap.Error(err))
		// 首次初始化
		gemShopData.Init()
	}

	// 初始化钻石商店
	s.initGemShopStandalone(gemShopData)

	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, gemShopData); err != nil {
		s.logger.Error("保存钻石商店数据失败", zap.Error(err))
		return nil, err
	}

	return convertToProtoGemShop(gemShopData), nil
}

// initShop 初始化常规商店（只处理TplShop配置的商店）
func (s *ApiServer) initShop(shopData *ShopData, shopType game.ShopType, refresh bool) {
	if shopData.Shops == nil {
		shopData.Shops = make(map[string]*ShopInfo)
	}

	key := shopType.String()
	switch shopType {
	case game.ShopType_SHOP_DAILY:
		s.initCommonShop(shopData.Shops, key, shopType, "Daily", refresh)
	case game.ShopType_SHOP_COIN:
		s.initCommonShop(shopData.Shops, key, shopType, "Coin", refresh)
	case game.ShopType_SHOP_STRENGTH:
		s.initCommonShop(shopData.Shops, key, shopType, "Strength", refresh)
	}
}

// initCommonShop 统一的商店初始化（支持随机商品+固定商品）
func (s *ApiServer) initCommonShop(shops map[string]*ShopInfo, key string, shopType game.ShopType, configKey string, refresh bool) {
	// 初始化商店结构
	if shops[key] == nil {
		shops[key] = &ShopInfo{
			ShopType:     shopType,
			Items:        []*ShopItem{},
			CanRefresh:   false, // 默认不可刷新
			RefreshCount: 0,
		}
	}

	shop := shops[key]

	// 从配置表中读取商店配置
	tplShop, ok := s.template.GetTplShop().FindByKey(configKey)
	if !ok {
		s.logger.Error("未找到商店配置", zap.String("shop_config_key", configKey))
		return
	}

	// 判断是否支持手动刷新
	if tplShop.HandleRefreshTimes > 0 {
		shop.CanRefresh = true
	}

	// 如果需要刷新或首次初始化
	if len(shop.Items) == 0 || refresh {
		// 清空商品列表
		shop.Items = []*ShopItem{}

		// 1. 添加固定商品（如果配置了）
		s.addPermanentShopItems(shop, &tplShop)

		// 2. 添加随机商品（如果配置了）
		s.addRandomShopItems(shop, &tplShop)

		// 设置下次刷新时间为第二天0点（UTC）
		now := time.Now().UTC()
		nextRefreshTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		shop.NextRefreshTime = nextRefreshTime
	}
}

// addRandomShopItems 添加随机商品
func (s *ApiServer) addRandomShopItems(shop *ShopInfo, tplShop *TplShop) {
	// 如果没有配置随机商品数量，直接返回
	if tplShop.FreeProductCount == 0 && tplShop.AdProductCount == 0 &&
		tplShop.CoinProductCount == 0 && tplShop.GemProductCount == 0 {
		return
	}

	allDailyItems := s.template.GetTplShopDailyItem().FindAll().ToSlice()

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
				Index:       int32(len(shop.Items)),
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
				Index:       int32(len(shop.Items)),
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
				Index:       int32(len(shop.Items)),
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
				Index:       int32(len(shop.Items)),
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

// addPermanentShopItems 添加固定商品
func (s *ApiServer) addPermanentShopItems(shop *ShopInfo, tplShop *TplShop) {
	if tplShop.PermanentProduct == "" {
		return
	}

	permanentIDs := strings.Split(tplShop.PermanentProduct, ",")
	for _, permanentID := range permanentIDs {
		permanentID = strings.TrimSpace(permanentID)
		if permanentID == "" {
			continue
		}

		tplPermanent, ok := s.template.GetTplShopPermanentItem().FindByKey(permanentID)
		if !ok {
			s.logger.Warn("未找到固定商品配置", zap.String("item_id", permanentID))
			continue
		}

		// 解析多种支付方式
		paymentStages := parsePaymentStages(tplPermanent.PayType, tplPermanent.SalePrice, tplPermanent.LimitTimes)

		// 使用第一个支付阶段的信息作为默认显示
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

		shopItem := &ShopItem{
			Index:         int32(len(shop.Items)),
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
		shop.Items = append(shop.Items, shopItem)
	}
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

// initChapterShopStandalone 初始化章节商店（独立）
func (s *ApiServer) initChapterShopStandalone(chapterShopData *ChapterShopData) {
	if len(chapterShopData.Items) > 0 {
		return
	}

	allItems := s.template.GetTplShopChapterItem().FindAll().ToSlice()
	for idx, tplItem := range allItems {
		shopItem := &ShopItem{
			Index:       int32(idx),
			ShopItemID:  tplItem.ID,
			ItemID:      "",
			PayType:     game.PayType_IAP, // IAP
			Price:       0,
			Discount:    0,
			ItemCount:   1,
			MaxBuyCount: tplItem.Count,
			BoughtCount: 0,
		}
		chapterShopData.Items = append(chapterShopData.Items, shopItem)
	}
}

// initGemShopStandalone 初始化钻石商店（独立）
func (s *ApiServer) initGemShopStandalone(gemShopData *GemShopData) {
	if len(gemShopData.Items) > 0 {
		return
	}

	allItems := s.template.GetTplShopGemItem().FindAll().ToSlice()
	for idx, tplItem := range allItems {
		shopItem := &ShopItem{
			Index:       int32(idx),
			ShopItemID:  tplItem.ID,
			ItemID:      "",
			PayType:     game.PayType_IAP, // IAP
			Price:       0,
			Discount:    0,
			ItemCount:   tplItem.Num,
			MaxBuyCount: 0, // 无限制
			BoughtCount: 0,
		}
		gemShopData.Items = append(gemShopData.Items, shopItem)
	}
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

	key := in.ShopType.String()
	shop, ok := shopData.Shops[key]
	if !ok {
		return &game.BuyShopItemResponse{Code: 1, Msg: "商店不存在"}, nil
	}

	if in.Index < 0 || in.Index >= int32(len(shop.Items)) {
		return &game.BuyShopItemResponse{Code: 2, Msg: "无效的商品索引"}, nil
	}

	item := shop.Items[in.Index]

	// 检查购买次数
	if item.MaxBuyCount > 0 && item.BoughtCount >= item.MaxBuyCount {
		return &game.BuyShopItemResponse{Code: 3, Msg: "购买次数已达上限"}, nil
	}

	// 计算消耗和奖励
	var cost *game.Wallet
	var reward *game.Reward
	var err error

	// 常规商店只处理 Daily、Coin、Strength
	cost, reward, err = s.processBuyNormalItem(item, in.Count)

	if err != nil {
		return &game.BuyShopItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// 扣除货币
	var walletUpdateResult *game.WalletUpdateResult
	if cost != nil && (cost.Coin > 0 || cost.Gem > 0 || cost.Ad > 0) {
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

		metadata, _ := json.Marshal(map[string]interface{}{
			"source":    "shop_purchase",
			"shop_type": in.ShopType.String(),
			"index":     in.Index,
		})

		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: walletChangeset,
			Metadata:  string(metadata),
		}}

		results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
		if err != nil {
			s.logger.Error("扣除货币失败", zap.Error(err))
			return &game.BuyShopItemResponse{Code: 5, Msg: "货币不足"}, nil
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 发放奖励（使用统一的发放函数，支持特殊道具处理）
	var inventoryUpdateResult *game.InventoryUpdateResult
	if reward != nil {
		source := fmt.Sprintf("shop_%s_%d", in.ShopType.String(), in.Index)
		rewardWalletResult, rewardInventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, source)
		if err != nil {
			s.logger.Error("发放奖励失败", zap.Error(err))
		} else {
			// 合并钱包结果（如果之前扣除货币的结果存在，使用扣除后的结果）
			if rewardWalletResult != nil {
				walletUpdateResult = rewardWalletResult
			}
			inventoryUpdateResult = rewardInventoryResult
		}
	}

	// 更新购买次数
	item.BoughtCount++

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
		s.logger.Error("保存商店数据失败", zap.Error(err))
		return &game.BuyShopItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	response := &game.BuyShopItemResponse{
		Code:   0,
		Msg:    "购买成功",
		Reward: reward,
		// ShopData 字段在购买接口中不返回，客户端可以单独调用GetShopData刷新
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

	key := in.ShopType.String()
	shop, ok := shopData.Shops[key]
	if !ok {
		return &game.RefreshShopResponse{Code: 1, Msg: "商店不存在"}, nil
	}

	// 检查是否支持手动刷新
	if !shop.CanRefresh {
		return &game.RefreshShopResponse{Code: 2, Msg: "该商店不支持手动刷新"}, nil
	}

	// 获取商店配置
	var tplShop TplShop
	var found bool
	switch in.ShopType {
	case game.ShopType_SHOP_DAILY:
		tplShop, found = s.template.GetTplShop().FindByKey("Daily")
	default:
		return &game.RefreshShopResponse{Code: 3, Msg: "不支持的商店类型"}, nil
	}

	if !found {
		return &game.RefreshShopResponse{Code: 4, Msg: "未找到商店配置"}, nil
	}

	// 检查刷新次数限制
	if shop.RefreshCount >= tplShop.HandleRefreshTimes {
		return &game.RefreshShopResponse{Code: 5, Msg: "已达到最大刷新次数"}, nil
	}

	// 执行刷新（重新初始化商店）
	s.initShop(shopData, in.ShopType, true)

	// 增加刷新次数
	shop.RefreshCount++

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

// BuyChapterItem 购买章节商品（独立接口，IAP）
func (s *ApiServer) BuyChapterItem(ctx context.Context, in *game.BuyChapterItemRequest) (*game.BuyChapterItemResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	chapterShopData := &ChapterShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败", zap.Error(err))
		return &game.BuyChapterItemResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	if int(in.Index) >= len(chapterShopData.Items) {
		return &game.BuyChapterItemResponse{Code: 1, Msg: "商品不存在"}, nil
	}

	item := chapterShopData.Items[in.Index]

	// 处理章节商品购买逻辑
	reward, err := s.processBuyChapterItemStandalone(item)
	if err != nil {
		return &game.BuyChapterItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// IAP 商品无需扣除货币，直接发放奖励
	_, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, "chapter_shop")
	if err != nil {
		return &game.BuyChapterItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, chapterShopData); err != nil {
		s.logger.Error("保存章节商店数据失败", zap.Error(err))
		return &game.BuyChapterItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	response := &game.BuyChapterItemResponse{
		Code:            0,
		Msg:             "购买成功",
		Reward:          reward,
		ChapterShopData: convertToProtoChapterShop(chapterShopData),
	}

	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

// BuyGemItem 购买钻石商品（独立接口，IAP）
func (s *ApiServer) BuyGemItem(ctx context.Context, in *game.BuyGemItemRequest) (*game.BuyGemItemResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	gemShopData := &GemShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, gemShopData); err != nil {
		s.logger.Error("加载钻石商店数据失败", zap.Error(err))
		return &game.BuyGemItemResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	if int(in.Index) >= len(gemShopData.Items) {
		return &game.BuyGemItemResponse{Code: 1, Msg: "商品不存在"}, nil
	}

	item := gemShopData.Items[in.Index]

	// 处理钻石商品购买逻辑
	reward, err := s.processBuyGemItemStandalone(item)
	if err != nil {
		return &game.BuyGemItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// IAP 商品直接发放钻石
	walletUpdateResult, _, err := GrantReward(ctx, s.logger, s.db, s.template, reward, "gem_shop")
	if err != nil {
		return &game.BuyGemItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, gemShopData); err != nil {
		s.logger.Error("保存钻石商店数据失败", zap.Error(err))
		return &game.BuyGemItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	response := &game.BuyGemItemResponse{
		Code:        0,
		Msg:         "购买成功",
		Reward:      reward,
		GemShopData: convertToProtoGemShop(gemShopData),
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}

	return response, nil
}

// processBuyNormalItem 处理普通商品购买
func (s *ApiServer) processBuyNormalItem(item *ShopItem, count int32) (*game.Wallet, *game.Reward, error) {
	// 如果配置了分段购买，根据已购买次数确定当前支付阶段
	var payType game.PayType
	var price int32

	if len(item.PaymentStages) > 0 {
		// 根据已购买次数找到当前所处的阶段
		currentBought := item.BoughtCount
		accumulatedLimit := int32(0)

		foundStage := false
		for _, stage := range item.PaymentStages {
			if currentBought < accumulatedLimit+stage.LimitCount {
				payType = stage.PayType
				price = stage.Price
				foundStage = true
				break
			}
			accumulatedLimit += stage.LimitCount
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

	cost := calculateCost(payType, price*count)

	reward := &game.Reward{
		Items: []*game.Item{{
			Id:  item.ItemID,
			Num: item.ItemCount * count,
		}},
	}

	return cost, reward, nil
}

// processBuyChapterItem 处理章节商店购买
func (s *ApiServer) processBuyChapterItem(item *ShopItem) (*game.Reward, error) {
	tplChapterItem, ok := s.template.GetTplShopChapterItem().FindByKey(item.ShopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到章节商店配置")
	}

	reward := parseRewardString(tplChapterItem.Reward, s.logger)
	return reward, nil
}

// processBuyGemItem 处理钻石商店购买
func (s *ApiServer) processBuyGemItem(item *ShopItem) (*game.Reward, error) {
	tplGemItem, ok := s.template.GetTplShopGemItem().FindByKey(item.ShopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到钻石商店配置")
	}

	gemCount := tplGemItem.Num
	if item.BoughtCount == 0 {
		gemCount += tplGemItem.ExtraNum
	}

	reward := &game.Reward{
		Wallet: &game.Wallet{
			Gem: gemCount,
		},
	}

	return reward, nil
}

// convertToProtoShopData 转换商店数据为proto ShopData
func convertToProtoShopData(data *ShopData) *game.ShopData {
	protoShops := make([]*game.SingleShopData, 0)

	for _, shop := range data.Shops {
		protoShop := convertToProtoSingleShop(shop)
		protoShops = append(protoShops, protoShop)
	}

	return &game.ShopData{
		DateTime: data.DateTime.Format(time.RFC3339),
		Shops:    protoShops,
	}
}

// convertToProtoSingleShop 转换单个商店为proto格式
func convertToProtoSingleShop(shop *ShopInfo) *game.SingleShopData {
	protoItems := make([]*game.ShopItem, 0, len(shop.Items))
	for _, item := range shop.Items {
		// 如果有分段购买配置，根据已购买次数计算当前显示的支付方式和价格
		displayPayType := item.PayType
		displayPrice := item.Price

		if len(item.PaymentStages) > 0 {
			currentBought := item.BoughtCount
			accumulatedLimit := int32(0)

			for _, stage := range item.PaymentStages {
				if currentBought < accumulatedLimit+stage.LimitCount {
					displayPayType = stage.PayType
					displayPrice = stage.Price
					break
				}
				accumulatedLimit += stage.LimitCount
			}
		}

		protoItems = append(protoItems, &game.ShopItem{
			Index:       item.Index,
			ShopItemId:  item.ShopItemID,
			ItemId:      item.ItemID,
			PayType:     displayPayType,
			Price:       displayPrice,
			Discount:    item.Discount,
			ItemCount:   item.ItemCount,
			MaxBuyCount: item.MaxBuyCount,
			BoughtCount: item.BoughtCount,
		})
	}

	protoShop := &game.SingleShopData{
		ShopType:   shop.ShopType,
		Items:      protoItems,
		CanRefresh: shop.CanRefresh,
	}

	if shop.CanRefresh {
		protoShop.RefreshCount = shop.RefreshCount
		protoShop.NextRefreshTime = shop.NextRefreshTime.Format(time.RFC3339)
	}

	return protoShop
}

// 辅助函数
func parseDiscount(discountStr string) int32 {
	if discountStr == "" {
		return 0
	}
	discount, err := strconv.ParseInt(discountStr, 10, 32)
	if err != nil {
		return 0
	}
	return int32(discount)
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

type PayInfo struct {
	PayType    game.PayType
	Price      int32
	LimitCount int32
}

func parsePayInfos(payInfoStr string) []PayInfo {
	if payInfoStr == "" {
		return []PayInfo{}
	}

	parts := strings.Split(payInfoStr, ";")
	payInfos := make([]PayInfo, 0, len(parts))

	for _, part := range parts {
		fields := strings.Split(part, "_")
		if len(fields) != 3 {
			continue
		}

		price, _ := strconv.ParseInt(fields[0], 10, 32)
		limitCount, _ := strconv.ParseInt(fields[1], 10, 32)

		payInfos = append(payInfos, PayInfo{
			PayType:    parsePayType(fields[2]),
			Price:      int32(price),
			LimitCount: int32(limitCount),
		})
	}

	return payInfos
}

func calculateCost(payType game.PayType, amount int32) *game.Wallet {
	cost := &game.Wallet{}

	switch payType {
	case game.PayType_COIN:
		cost.Coin = amount
	case game.PayType_GEM:
		cost.Gem = amount
	case game.PayType_AD:
		cost.Ad = amount
	}

	return cost
}

func parseRewardString(rewardStr string, logger *zap.Logger) *game.Reward {
	if rewardStr == "" {
		return nil
	}

	reward := &game.Reward{
		Wallet: &game.Wallet{},
		Items:  []*game.Item{},
	}

	pairs := strings.Split(rewardStr, ",")
	for _, pair := range pairs {
		trimmedPair := strings.TrimSpace(pair)
		if trimmedPair == "" {
			continue
		}

		keyValue := strings.Split(trimmedPair, "_")
		if len(keyValue) != 2 {
			logger.Error("无效的奖励配置", zap.String("pair", trimmedPair))
			continue
		}

		key := strings.TrimSpace(keyValue[0])
		numStr := strings.TrimSpace(keyValue[1])

		num, err := strconv.ParseInt(numStr, 10, 32)
		if err != nil {
			logger.Error("无效的奖励数量", zap.String("pair", trimmedPair), zap.Error(err))
			continue
		}

		switch key {
		case "coin":
			reward.Wallet.Coin = int32(num)
		case "gem":
			reward.Wallet.Gem = int32(num)
		case "ad":
			reward.Wallet.Ad = int32(num)
		default:
			reward.Items = append(reward.Items, &game.Item{
				Id:  key,
				Num: int32(num),
			})
		}
	}

	return reward
}

// convertToProtoChapterShop 转换章节商店数据
func convertToProtoChapterShop(data *ChapterShopData) *game.ChapterShopData {
	protoItems := make([]*game.ShopItem, 0, len(data.Items))
	for _, item := range data.Items {
		protoItems = append(protoItems, &game.ShopItem{
			Index:       item.Index,
			ShopItemId:  item.ShopItemID,
			ItemId:      item.ItemID,
			PayType:     item.PayType,
			Price:       item.Price,
			Discount:    item.Discount,
			ItemCount:   item.ItemCount,
			MaxBuyCount: item.MaxBuyCount,
			BoughtCount: item.BoughtCount,
		})
	}

	return &game.ChapterShopData{
		Items: protoItems,
	}
}

// convertToProtoGemShop 转换钻石商店数据
func convertToProtoGemShop(data *GemShopData) *game.GemShopData {
	protoItems := make([]*game.ShopItem, 0, len(data.Items))
	for _, item := range data.Items {
		protoItems = append(protoItems, &game.ShopItem{
			Index:       item.Index,
			ShopItemId:  item.ShopItemID,
			ItemId:      item.ItemID,
			PayType:     item.PayType,
			Price:       item.Price,
			Discount:    item.Discount,
			ItemCount:   item.ItemCount,
			MaxBuyCount: item.MaxBuyCount,
			BoughtCount: item.BoughtCount,
		})
	}

	return &game.GemShopData{
		Items: protoItems,
	}
}

// processBuyChapterItemStandalone 处理章节商品购买（独立版本）
func (s *ApiServer) processBuyChapterItemStandalone(item *ShopItem) (*game.Reward, error) {
	if item.BoughtCount >= item.MaxBuyCount {
		return nil, fmt.Errorf("已达到最大购买次数")
	}

	item.BoughtCount++

	tplItem, ok := s.template.GetTplShopChapterItem().FindByKey(item.ShopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到章节商品配置")
	}

	reward := parseRewardString(tplItem.Reward, s.logger)
	return reward, nil
}

// processBuyGemItemStandalone 处理钻石商品购买（独立版本）
func (s *ApiServer) processBuyGemItemStandalone(item *ShopItem) (*game.Reward, error) {
	item.BoughtCount++

	tplItem, ok := s.template.GetTplShopGemItem().FindByKey(item.ShopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到钻石商品配置")
	}

	reward := &game.Reward{
		Wallet: &game.Wallet{
			Gem: tplItem.Num + tplItem.ExtraNum,
		},
	}

	return reward, nil
}

// processPaymentAndReward 处理支付和奖励发放
func (s *ApiServer) processPaymentAndReward(ctx context.Context, cost *game.Wallet, reward *game.Reward, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	// 扣除货币
	var walletUpdateResult *game.WalletUpdateResult
	if cost != nil && (cost.Coin > 0 || cost.Gem > 0 || cost.Ad > 0) {
		userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
		metadata, _ := json.Marshal(map[string]interface{}{"source": source})

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
			Metadata:  string(metadata),
		}}

		results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
		if err != nil {
			return nil, nil, fmt.Errorf("货币不足或扣除失败")
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 发放奖励
	_, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, source)
	if err != nil {
		return walletUpdateResult, nil, err
	}

	return walletUpdateResult, inventoryUpdateResult, nil
}
