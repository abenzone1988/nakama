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

// ShopData 商店数据存储结构
type ShopData struct {
	DateTime     time.Time         `json:"date_time"`
	DailyShop    *DailyShopData    `json:"daily_shop"`
	CoinBagShop  *CoinBagShopData  `json:"coin_bag_shop"`
	StrengthShop *StrengthShopData `json:"strength_shop"`
	BoxShop      *BoxShopData      `json:"box_shop"`
	ChapterShop  map[string]int32  `json:"chapter_shop"`
	GemShop      map[string]int32  `json:"gem_shop"`
}

type DailyShopData struct {
	AutoRefreshTime    int32                `json:"auto_refresh_time"`
	HandleRefreshTimes int32                `json:"handle_refresh_times"`
	PermanentItem      *PermanentShopItem   `json:"permanent_item"`
	DailyItems         []*DailyShopItemData `json:"daily_items"`
}

// DailyShopItemData 每日商店商品数据（内部存储）
type DailyShopItemData struct {
	Index      int32          `json:"index"`        // 列表索引
	ShopItemID string         `json:"shop_item_id"` // 配置表ID
	ItemID     string         `json:"item_id"`      // 物品ID
	PayType    game.MoneyType `json:"pay_type"`
	Price      int32          `json:"price"`
	Discount   int32          `json:"discount"`
	Count      int32          `json:"count"`
	LimitCount int32          `json:"limit_count"`
}

type PermanentShopItem struct {
	ShopItemID string         `json:"shop_item_id"` // 配置表ID（唯一标识）
	ItemID     string         `json:"item_id"`      // 物品ID
	PayType    game.MoneyType `json:"pay_type"`
	Price      int32          `json:"price"`
	Count      int32          `json:"count"`
	LimitCount int32          `json:"limit_count"`
	PayInfos   []*PayInfo     `json:"pay_infos"`
}

type PayInfo struct {
	PayType    game.MoneyType `json:"pay_type"`
	Price      int32          `json:"price"`
	LimitCount int32          `json:"limit_count"`
}

type CoinBagShopData struct {
	PermanentItems []*PermanentShopItem `json:"permanent_items"`
}

type StrengthShopData struct {
	PermanentItems []*PermanentShopItem `json:"permanent_items"`
}

type BoxShopData struct {
	BoxShopLevel int32        `json:"box_shop_level"`
	BoxShopExp   int32        `json:"box_shop_exp"`
	Box1Item     *BoxShopItem `json:"box1_item"`
}

type BoxShopItem struct {
	CanSeeAD        bool      `json:"can_see_ad"`
	NextRefreshTime time.Time `json:"next_refresh_time"`
	ShopItemID      string    `json:"shop_item_id"` // 配置表ID（唯一标识）
	PayType         string    `json:"pay_type"`
	RetailPrice     int32     `json:"retail_price"`
	WholesalePrice  int32     `json:"wholesale_price"`
	RefreshTime     int32     `json:"refresh_time"`
}

// Storable 接口实现
func (s *ShopData) GetCollection() string {
	return "user_data"
}

func (s *ShopData) GetKey() string {
	return "shop"
}

func (s *ShopData) Init() {
	s.DateTime = time.Now().UTC()
	s.DailyShop = &DailyShopData{DailyItems: []*DailyShopItemData{}}
	s.CoinBagShop = &CoinBagShopData{PermanentItems: []*PermanentShopItem{}}
	s.StrengthShop = &StrengthShopData{PermanentItems: []*PermanentShopItem{}}
	s.BoxShop = &BoxShopData{}
	s.ChapterShop = make(map[string]int32)
	s.GemShop = make(map[string]int32)
}

// GetShopData RPC接口实现
func (s *ApiServer) GetShopData(ctx context.Context, in *emptypb.Empty) (*game.ShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载商店数据
	shopData := &ShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, shopData); err != nil {
		s.logger.Error("加载商店数据失败", zap.Error(err))
		return nil, err
	}

	// 初始化或刷新商店数据
	//needRefresh := checkNeedRefresh(shopData.DateTime)
	//if needRefresh {
	//	s.logger.Info("检测到需要刷新商店", zap.Time("last_date", shopData.DateTime))
	//	shopData.DateTime = time.Now().UTC()
	//}
	needRefresh := true
	// 初始化各个商店
	if err := s.initOrRefreshShop(ctx, shopData, needRefresh); err != nil {
		s.logger.Error("初始化商店失败", zap.Error(err))
		return nil, err
	}

	// 保存数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
		s.logger.Error("保存商店数据失败", zap.Error(err))
		return nil, err
	}

	// 转换为protobuf格式
	return convertToProtoShopData(shopData), nil
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

// initOrRefreshShop 初始化或刷新商店
func (s *ApiServer) initOrRefreshShop(ctx context.Context, shopData *ShopData, needRefresh bool) error {
	// 每日商店
	if err := s.initDailyShop(shopData, needRefresh); err != nil {
		return err
	}

	// 金币袋商店
	if err := s.initCoinBagShop(shopData, needRefresh); err != nil {
		return err
	}

	// 体力商店
	if err := s.initStrengthShop(shopData, needRefresh); err != nil {
		return err
	}

	// 宝箱商店
	if err := s.initBoxShop(shopData, needRefresh); err != nil {
		return err
	}

	return nil
}

// initDailyShop 初始化每日商店
func (s *ApiServer) initDailyShop(shopData *ShopData, needRefresh bool) error {
	tplShop, ok := s.template.GetTplShop().FindByKey("Daily")
	if !ok {
		return fmt.Errorf("未找到每日商店配置")
	}

	if shopData.DailyShop == nil || needRefresh {
		shopData.DailyShop = &DailyShopData{
			HandleRefreshTimes: tplShop.HandleRefreshTimes,
			DailyItems:         []*DailyShopItemData{},
		}

		// 刷新每日商店商品
		s.refreshDailyShop(shopData, &tplShop)
	} else {
		// 不需要刷新，但需要更新价格和数量
		for _, item := range shopData.DailyShop.DailyItems {
			if tplItem, ok := s.template.GetTplShopDailyItem().FindByKey(item.ShopItemID); ok {
				item.Count = tplItem.Num
				switch item.PayType {
				case game.MoneyType_MONEY_TYPE_COIN:
					item.Price = tplItem.CoinPrice
				case game.MoneyType_MONEY_TYPE_GEM:
					item.Price = tplItem.GemPrice
				default:
					item.Price = 0
				}
			}
		}
	}

	// 初始化固定商品
	if shopData.DailyShop.PermanentItem == nil || needRefresh {
		permanentIDs := strings.Split(tplShop.PermanentProduct, ",")
		if len(permanentIDs) > 0 {
			if tplPermanent, ok := s.template.GetTplShopPermanentItem().FindByKey(permanentIDs[0]); ok {
				shopData.DailyShop.PermanentItem = s.createPermanentItem(&tplPermanent)
			}
		}
	} else {
		s.checkPermanentData(shopData.DailyShop.PermanentItem)
	}

	return nil
}

// refreshDailyShop 刷新每日商店商品
func (s *ApiServer) refreshDailyShop(shopData *ShopData, tplShop *TplShop) {
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

	shopData.DailyShop.DailyItems = []*DailyShopItemData{}

	// 添加免费商品
	for i := int32(0); i < tplShop.FreeProductCount; i++ {
		if item := s.getRandomDailyItem(freeItems); item != nil {
			shopData.DailyShop.DailyItems = append(shopData.DailyShop.DailyItems, &DailyShopItemData{
				Index:      int32(len(shopData.DailyShop.DailyItems)), // 使用索引
				ShopItemID: item.ID,
				ItemID:     item.Item,
				PayType:    game.MoneyType_MONEY_TYPE_NULL,
				Price:      0,
				Discount:   0,
				Count:      item.Num,
				LimitCount: 1,
			})
		}
	}

	// 添加广告商品
	for i := int32(0); i < tplShop.AdProductCount; i++ {
		if item := s.getRandomDailyItem(adItems); item != nil {
			shopData.DailyShop.DailyItems = append(shopData.DailyShop.DailyItems, &DailyShopItemData{
				Index:      int32(len(shopData.DailyShop.DailyItems)), // 使用索引
				ShopItemID: item.ID,
				ItemID:     item.Item,
				PayType:    game.MoneyType_MONEY_TYPE_AD,
				Price:      0,
				Discount:   0,
				Count:      item.Num,
				LimitCount: 1,
			})
		}
	}

	// 添加金币商品
	for i := int32(0); i < tplShop.CoinProductCount; i++ {
		if item := s.getRandomDailyItem(coinItems); item != nil {
			discount := s.parseDiscount(tplShop.CoinProductDiscountRate)
			shopData.DailyShop.DailyItems = append(shopData.DailyShop.DailyItems, &DailyShopItemData{
				Index:      int32(len(shopData.DailyShop.DailyItems)), // 使用索引
				ShopItemID: item.ID,
				ItemID:     item.Item,
				PayType:    game.MoneyType_MONEY_TYPE_COIN,
				Price:      item.CoinPrice,
				Discount:   discount,
				Count:      item.Num,
				LimitCount: 1,
			})
		}
	}

	// 添加钻石商品
	for i := int32(0); i < tplShop.GemProductCount; i++ {
		if item := s.getRandomDailyItem(gemItems); item != nil {
			discount := s.parseDiscount(tplShop.GemProductDiscountRate)
			shopData.DailyShop.DailyItems = append(shopData.DailyShop.DailyItems, &DailyShopItemData{
				Index:      int32(len(shopData.DailyShop.DailyItems)), // 使用索引
				ShopItemID: item.ID,
				ItemID:     item.Item,
				PayType:    game.MoneyType_MONEY_TYPE_GEM,
				Price:      item.GemPrice,
				Discount:   discount,
				Count:      item.Num,
				LimitCount: 1,
			})
		}
	}
}

// getRandomDailyItem 随机获取每日商品
func (s *ApiServer) getRandomDailyItem(items []TplShopDailyItem) *TplShopDailyItem {
	if len(items) == 0 {
		return nil
	}
	idx := rand.Intn(len(items))
	return &items[idx]
}

// parseDiscount 解析折扣配置
func (s *ApiServer) parseDiscount(discountInfo string) int32 {
	if discountInfo == "" {
		return 10 // 无折扣
	}

	discountParts := strings.Split(discountInfo, ",")
	totalRate := 0
	discountMap := make(map[int32]int32)

	for _, part := range discountParts {
		info := strings.Split(part, "_")
		if len(info) == 2 {
			discount, _ := strconv.ParseInt(info[0], 10, 32)
			rate, _ := strconv.ParseInt(info[1], 10, 32)
			totalRate += int(rate)
			discountMap[int32(discount)] = int32(totalRate)
		}
	}

	value := rand.Intn(100)
	preRate := int32(0)
	for discount, rate := range discountMap {
		if int32(value) > preRate && int32(value) < rate {
			return discount
		}
		preRate = rate
	}

	return 10
}

// initCoinBagShop 初始化金币袋商店
func (s *ApiServer) initCoinBagShop(shopData *ShopData, needRefresh bool) error {
	tplShop, ok := s.template.GetTplShop().FindByKey("Coin")
	if !ok {
		return fmt.Errorf("未找到金币袋商店配置")
	}

	if shopData.CoinBagShop == nil || needRefresh || len(shopData.CoinBagShop.PermanentItems) == 0 {
		shopData.CoinBagShop = &CoinBagShopData{
			PermanentItems: []*PermanentShopItem{},
		}

		permanentIDs := strings.Split(tplShop.PermanentProduct, ",")
		for _, pid := range permanentIDs {
			if tplItem, ok := s.template.GetTplShopPermanentItem().FindByKey(pid); ok {
				shopData.CoinBagShop.PermanentItems = append(
					shopData.CoinBagShop.PermanentItems,
					s.createPermanentItem(&tplItem),
				)
			}
		}
	} else {
		for _, item := range shopData.CoinBagShop.PermanentItems {
			s.checkPermanentData(item)
		}
	}

	return nil
}

// initStrengthShop 初始化体力商店
func (s *ApiServer) initStrengthShop(shopData *ShopData, needRefresh bool) error {
	tplShop, ok := s.template.GetTplShop().FindByKey("Strength")
	if !ok {
		return fmt.Errorf("未找到体力商店配置")
	}

	if shopData.StrengthShop == nil || needRefresh || len(shopData.StrengthShop.PermanentItems) == 0 {
		shopData.StrengthShop = &StrengthShopData{
			PermanentItems: []*PermanentShopItem{},
		}

		permanentIDs := strings.Split(tplShop.PermanentProduct, ",")
		for _, pid := range permanentIDs {
			if tplItem, ok := s.template.GetTplShopPermanentItem().FindByKey(pid); ok {
				shopData.StrengthShop.PermanentItems = append(
					shopData.StrengthShop.PermanentItems,
					s.createPermanentItem(&tplItem),
				)
			}
		}
	} else {
		for _, item := range shopData.StrengthShop.PermanentItems {
			s.checkPermanentData(item)
		}
	}

	return nil
}

// initBoxShop 初始化宝箱商店
func (s *ApiServer) initBoxShop(shopData *ShopData, needRefresh bool) error {
	if shopData.BoxShop == nil || needRefresh || shopData.BoxShop.Box1Item == nil {
		level := int32(1)
		exp := int32(0)
		if shopData.BoxShop != nil {
			if shopData.BoxShop.BoxShopLevel > 0 {
				level = shopData.BoxShop.BoxShopLevel
			}
			exp = shopData.BoxShop.BoxShopExp
		}

		shopData.BoxShop = &BoxShopData{
			BoxShopLevel: level,
			BoxShopExp:   exp,
		}

		tplBoxShops := s.template.GetTplBoxShop().FindByFilter(func(bs TplBoxShop) bool {
			return bs.Level == level
		}).ToSlice()

		if len(tplBoxShops) > 0 {
			tplBoxShop := tplBoxShops[0]
			if tplBoxItem, ok := s.template.GetTplBoxShopItem().FindByKey(tplBoxShop.Box1); ok {
				shopData.BoxShop.Box1Item = &BoxShopItem{
					CanSeeAD:        true,
					NextRefreshTime: time.Now().UTC().Add(time.Duration(tplBoxItem.RefreshTime) * time.Hour),
					ShopItemID:      tplBoxItem.ID,
					PayType:         tplBoxItem.PayType,
					RetailPrice:     tplBoxItem.RetailPrice,
					WholesalePrice:  tplBoxItem.WholesalePrice,
					RefreshTime:     tplBoxItem.RefreshTime,
				}
			}
		}
	} else {
		s.checkBoxShopData(shopData.BoxShop.Box1Item)
	}

	return nil
}

// createPermanentItem 创建固定商品
func (s *ApiServer) createPermanentItem(tpl *TplShopPermanentItem) *PermanentShopItem {
	item := &PermanentShopItem{
		ShopItemID: tpl.ID,
		ItemID:     tpl.Item,
		Count:      tpl.Num,
		PayInfos:   s.parsePayType(tpl),
	}

	if len(item.PayInfos) > 0 {
		firstPay := item.PayInfos[0]
		item.PayType = firstPay.PayType
		item.Price = firstPay.Price
		item.LimitCount = firstPay.LimitCount
	}

	return item
}

// parsePayType 解析支付类型配置
func (s *ApiServer) parsePayType(tpl *TplShopPermanentItem) []*PayInfo {
	payTypes := strings.Split(tpl.PayType, ",")
	if len(payTypes) != len(tpl.SalePrice) || len(payTypes) != len(tpl.LimitTimes) {
		s.logger.Error("固定商品价格配置错误", zap.String("id", tpl.ID))
		return []*PayInfo{}
	}

	payInfos := make([]*PayInfo, 0, len(payTypes))
	for i, pt := range payTypes {
		var moneyType game.MoneyType
		switch pt {
		case "free":
			moneyType = game.MoneyType_MONEY_TYPE_NULL
		case "ad":
			moneyType = game.MoneyType_MONEY_TYPE_AD
		case "gem":
			moneyType = game.MoneyType_MONEY_TYPE_GEM
		case "coin":
			moneyType = game.MoneyType_MONEY_TYPE_COIN
		default:
			moneyType = game.MoneyType_MONEY_TYPE_NULL
		}

		payInfos = append(payInfos, &PayInfo{
			PayType:    moneyType,
			Price:      tpl.SalePrice[i],
			LimitCount: tpl.LimitTimes[i],
		})
	}

	return payInfos
}

// checkPermanentData 检查并更新固定商品数据
func (s *ApiServer) checkPermanentData(item *PermanentShopItem) {
	tpl, ok := s.template.GetTplShopPermanentItem().FindByKey(item.ShopItemID)
	if !ok {
		return
	}

	item.Count = tpl.Num

	// 检查支付类型是否有变化
	excelPayInfo := s.parsePayType(&tpl)
	hasNewPayType := false

	for _, oldPay := range item.PayInfos {
		found := false
		for _, newPay := range excelPayInfo {
			if newPay.PayType == oldPay.PayType {
				found = true
				break
			}
		}
		if !found {
			hasNewPayType = true
			break
		}
	}

	if hasNewPayType {
		// 表数据变动较大，重置商品信息
		item.PayInfos = excelPayInfo
		if len(item.PayInfos) > 0 {
			firstPay := item.PayInfos[0]
			item.PayType = firstPay.PayType
			item.Price = firstPay.Price
			item.LimitCount = firstPay.LimitCount
		}
	} else {
		// 只更新价格
		for _, newPay := range excelPayInfo {
			if newPay.PayType == item.PayType {
				item.Price = newPay.Price
			}
		}
	}
}

// checkBoxShopData 检查并更新宝箱商店数据
func (s *ApiServer) checkBoxShopData(item *BoxShopItem) {
	tpl, ok := s.template.GetTplBoxShopItem().FindByKey(item.ShopItemID)
	if !ok {
		return
	}

	item.PayType = tpl.PayType
	item.RetailPrice = tpl.RetailPrice
	item.WholesalePrice = tpl.WholesalePrice
}

// convertToProtoShopData 转换为protobuf格式
func convertToProtoShopData(data *ShopData) *game.ShopData {
	result := &game.ShopData{
		DateTime: data.DateTime.Format(time.RFC3339),
	}

	// 每日商店
	if data.DailyShop != nil {
		result.DailyShop = &game.DailyShopData{
			AutoRefreshTime:    data.DailyShop.AutoRefreshTime,
			HandleRefreshTimes: data.DailyShop.HandleRefreshTimes,
		}

		if data.DailyShop.PermanentItem != nil {
			result.DailyShop.PermanentItem = convertToPermanentShopItem(data.DailyShop.PermanentItem)
		}

		for _, item := range data.DailyShop.DailyItems {
			result.DailyShop.DailyItems = append(result.DailyShop.DailyItems, &game.DailyShopItem{
				Index:      item.Index,
				ShopItemId: item.ShopItemID,
				ItemId:     item.ItemID,
				PayType:    item.PayType,
				Price:      item.Price,
				Discount:   item.Discount,
				Count:      item.Count,
				LimitCount: item.LimitCount,
			})
		}
	}

	// 金币袋商店
	if data.CoinBagShop != nil {
		result.CoinBagShop = &game.CoinBagShopData{}
		for _, item := range data.CoinBagShop.PermanentItems {
			result.CoinBagShop.PermanentItems = append(
				result.CoinBagShop.PermanentItems,
				convertToPermanentShopItem(item),
			)
		}
	}

	// 体力商店
	if data.StrengthShop != nil {
		result.StrengthShop = &game.StrengthShopData{}
		for _, item := range data.StrengthShop.PermanentItems {
			result.StrengthShop.PermanentItems = append(
				result.StrengthShop.PermanentItems,
				convertToPermanentShopItem(item),
			)
		}
	}

	// 宝箱商店
	if data.BoxShop != nil {
		result.BoxShop = &game.BoxShopData{
			BoxShopLevel: data.BoxShop.BoxShopLevel,
			BoxShopExp:   data.BoxShop.BoxShopExp,
		}

		if data.BoxShop.Box1Item != nil {
			result.BoxShop.Box1Item = &game.BoxShopItem{
				CanSeeAd:        data.BoxShop.Box1Item.CanSeeAD,
				NextRefreshTime: data.BoxShop.Box1Item.NextRefreshTime.Format(time.RFC3339),
				ShopItemId:      data.BoxShop.Box1Item.ShopItemID,
				PayType:         convertToProtoMoneyType(data.BoxShop.Box1Item.PayType),
				RetailPrice:     data.BoxShop.Box1Item.RetailPrice,
				WholesalePrice:  data.BoxShop.Box1Item.WholesalePrice,
				RefreshTime:     data.BoxShop.Box1Item.RefreshTime,
			}
		}
	}

	// 章节商店
	if data.ChapterShop != nil {
		result.ChapterShop = &game.ChapterShopData{
			ChapterDataMap: data.ChapterShop,
		}
	}

	// 钻石商店
	if data.GemShop != nil {
		result.GemShop = &game.GemShopData{
			GemDataMap: data.GemShop,
		}
	}

	return result
}

func convertToPermanentShopItem(item *PermanentShopItem) *game.PermanentShopItem {
	result := &game.PermanentShopItem{
		ShopItemId: item.ShopItemID,
		ItemId:     item.ItemID,
		PayType:    item.PayType,
		Price:      item.Price,
		Count:      item.Count,
		LimitCount: item.LimitCount,
	}

	for _, payInfo := range item.PayInfos {
		result.PayInfos = append(result.PayInfos, &game.PayInfo{
			PayType:    payInfo.PayType,
			Price:      payInfo.Price,
			LimitCount: payInfo.LimitCount,
		})
	}

	return result
}

func convertToProtoMoneyType(payType string) game.MoneyType {
	switch payType {
	case "coin":
		return game.MoneyType_MONEY_TYPE_COIN
	case "gem":
		return game.MoneyType_MONEY_TYPE_GEM
	case "ad":
		return game.MoneyType_MONEY_TYPE_AD
	default:
		return game.MoneyType_MONEY_TYPE_NULL
	}
}

// processPermanentItemPurchase 处理固定商品购买次数
func (s *ApiServer) processPermanentItemPurchase(item *PermanentShopItem) {
	if item.LimitCount > 0 {
		item.LimitCount--
		if item.LimitCount <= 0 && len(item.PayInfos) > 0 {
			item.PayInfos = item.PayInfos[1:]
			if len(item.PayInfos) > 0 {
				newPay := item.PayInfos[0]
				item.PayType = newPay.PayType
				item.Price = newPay.Price
				item.LimitCount = newPay.LimitCount
			}
		}
	}
}

// HandRefreshDailyShop 手动刷新每日商店
func (s *ApiServer) HandRefreshDailyShop(ctx context.Context) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	shopData := &ShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, shopData); err != nil {
		return err
	}

	if shopData.DailyShop.HandleRefreshTimes <= 0 {
		return fmt.Errorf("刷新次数不足")
	}

	shopData.DailyShop.HandleRefreshTimes--

	tplShop, ok := s.template.GetTplShop().FindByKey("Daily")
	if !ok {
		return fmt.Errorf("未找到每日商店配置")
	}

	s.refreshDailyShop(shopData, &tplShop)

	return SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData)
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

	// 根据商店类型处理购买
	var reward *game.Reward
	var costWallet *game.Wallet
	var err error
	var boxLevelUp bool

	switch in.ShopType {
	case game.ShopType_SHOP_TYPE_DAILY:
		reward, costWallet, err = s.buyDailyShopItem(ctx, shopData, in.ItemId)

	case game.ShopType_SHOP_TYPE_DAILY_PERMANENT, game.ShopType_SHOP_TYPE_COIN_BAG, game.ShopType_SHOP_TYPE_STRENGTH:
		reward, costWallet, err = s.buyPermanentShopItem(ctx, shopData, in.ShopType, in.ItemId)

	case game.ShopType_SHOP_TYPE_BOX:
		reward, costWallet, boxLevelUp, err = s.buyBoxShopItem(ctx, shopData, in.UseAd, in.AdWatched, in.Count)

	case game.ShopType_SHOP_TYPE_CHAPTER:
		reward, err = s.buyChapterShopItem(ctx, shopData, in.ItemId)

	case game.ShopType_SHOP_TYPE_GEM:
		reward, err = s.buyGemShopItem(ctx, shopData, in.ItemId)

	default:
		return &game.BuyShopItemResponse{Code: 1, Msg: "未知的商店类型"}, nil
	}

	if err != nil {
		s.logger.Error("购买失败", zap.Error(err), zap.String("shop_type", in.ShopType.String()))
		return &game.BuyShopItemResponse{Code: 2, Msg: err.Error()}, nil
	}

	// 扣除货币
	var walletUpdateResult *game.WalletUpdateResult
	if costWallet != nil && (costWallet.Coin > 0 || costWallet.Gem > 0 || costWallet.Ad > 0) {
		walletChangeset := make(map[string]int64)
		if costWallet.Coin > 0 {
			walletChangeset["coin"] = -int64(costWallet.Coin)
		}
		if costWallet.Gem > 0 {
			walletChangeset["gem"] = -int64(costWallet.Gem)
		}
		if costWallet.Ad > 0 {
			walletChangeset["ad"] = -int64(costWallet.Ad)
		}

		metadata, _ := json.Marshal(map[string]interface{}{
			"source":    "shop_purchase",
			"shop_type": in.ShopType.String(),
			"item_id":   in.ItemId,
		})

		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: walletChangeset,
			Metadata:  string(metadata),
		}}

		results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
		if err != nil {
			s.logger.Error("扣除货币失败", zap.Error(err))
			return &game.BuyShopItemResponse{Code: 3, Msg: "货币不足"}, nil
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 发放奖励
	var inventoryUpdateResult *game.InventoryUpdateResult
	if reward != nil {
		// 发放钱包奖励
		if reward.Wallet != nil && (reward.Wallet.Coin > 0 || reward.Wallet.Gem > 0 || reward.Wallet.Ad > 0) {
			walletChangeset := make(map[string]int64)
			if reward.Wallet.Coin > 0 {
				walletChangeset["coin"] = int64(reward.Wallet.Coin)
			}
			if reward.Wallet.Gem > 0 {
				walletChangeset["gem"] = int64(reward.Wallet.Gem)
			}
			if reward.Wallet.Ad > 0 {
				walletChangeset["ad"] = int64(reward.Wallet.Ad)
			}

			metadata, _ := json.Marshal(map[string]interface{}{
				"source":    "shop_reward",
				"shop_type": in.ShopType.String(),
			})

			walletUpdates := []*walletUpdate{{
				UserID:    userID,
				Changeset: walletChangeset,
				Metadata:  string(metadata),
			}}

			results, err := UpdateWallets(ctx, s.logger, s.db, walletUpdates, true)
			if err != nil {
				s.logger.Error("发放钱包奖励失败", zap.Error(err))
			} else if len(results) > 0 {
				walletUpdateResult = &game.WalletUpdateResult{
					Previous: convertMapInt64ToWallet(results[0].Previous),
					Updated:  convertMapInt64ToWallet(results[0].Updated),
				}
			}
		}

		// 发放道具奖励
		if reward.Items != nil && len(reward.Items) > 0 {
			inventoryChangeset := make(map[string]int64)
			for _, item := range reward.Items {
				if item != nil && item.Num > 0 {
					inventoryChangeset[item.Id] = int64(item.Num)
				}
			}

			if len(inventoryChangeset) > 0 {
				metadata, _ := json.Marshal(map[string]interface{}{
					"source":    "shop_reward",
					"shop_type": in.ShopType.String(),
				})

				inventoryUpdates := []*inventoryUpdate{{
					UserID:    userID,
					Changeset: inventoryChangeset,
					Metadata:  string(metadata),
				}}

				results, err := UpdateInventories(ctx, s.logger, s.db, inventoryUpdates, true)
				if err != nil {
					s.logger.Error("发放道具奖励失败", zap.Error(err))
				} else if len(results) > 0 {
					inventoryUpdateResult = &game.InventoryUpdateResult{
						Previous: convertMapInt64ToItems(results[0].Previous),
						Updated:  convertMapInt64ToItems(results[0].Updated),
					}
				}
			}
		}
	}

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, shopData); err != nil {
		s.logger.Error("保存商店数据失败", zap.Error(err))
		return &game.BuyShopItemResponse{Code: 4, Msg: "保存商店数据失败"}, nil
	}

	// 构建响应
	response := &game.BuyShopItemResponse{
		Code:       0,
		Msg:        "购买成功",
		Reward:     reward,
		ShopData:   convertToProtoShopData(shopData),
		BoxLevelUp: boxLevelUp,
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}

	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

// buyDailyShopItem 购买每日商店商品
func (s *ApiServer) buyDailyShopItem(ctx context.Context, shopData *ShopData, itemId string) (*game.Reward, *game.Wallet, error) {
	// itemId 是索引（字符串形式）
	index, err := strconv.ParseInt(itemId, 10, 32)
	if err != nil || index < 0 || index >= int64(len(shopData.DailyShop.DailyItems)) {
		return nil, nil, fmt.Errorf("无效的商品索引")
	}

	item := shopData.DailyShop.DailyItems[index]
	if item.LimitCount <= 0 {
		return nil, nil, fmt.Errorf("商品购买次数已用完")
	}

	// 计算消耗
	cost := &game.Wallet{}
	if item.Discount > 0 {
		cost = calculateCost(item.PayType, int32(float32(item.Price)*float32(item.Discount)*0.1))
	} else {
		cost = calculateCost(item.PayType, item.Price)
	}

	// 构建奖励
	reward := &game.Reward{
		Items: []*game.Item{{
			Id:  item.ItemID,
			Num: item.Count,
		}},
	}

	// 更新购买次数
	item.LimitCount--

	return reward, cost, nil
}

// buyPermanentShopItem 购买固定商店商品
func (s *ApiServer) buyPermanentShopItem(ctx context.Context, shopData *ShopData, shopType game.ShopType, itemId string) (*game.Reward, *game.Wallet, error) {
	var targetItem *PermanentShopItem

	switch shopType {
	case game.ShopType_SHOP_TYPE_DAILY_PERMANENT:
		if shopData.DailyShop.PermanentItem != nil && shopData.DailyShop.PermanentItem.ShopItemID == itemId {
			targetItem = shopData.DailyShop.PermanentItem
		}

	case game.ShopType_SHOP_TYPE_COIN_BAG:
		for _, item := range shopData.CoinBagShop.PermanentItems {
			if item.ShopItemID == itemId {
				targetItem = item
				break
			}
		}

	case game.ShopType_SHOP_TYPE_STRENGTH:
		for _, item := range shopData.StrengthShop.PermanentItems {
			if item.ShopItemID == itemId {
				targetItem = item
				break
			}
		}
	}

	if targetItem == nil {
		return nil, nil, fmt.Errorf("未找到商品")
	}

	if targetItem.LimitCount <= 0 {
		return nil, nil, fmt.Errorf("商品购买次数已用完")
	}

	// 计算消耗
	cost := calculateCost(targetItem.PayType, targetItem.Price)

	// 构建奖励
	reward := &game.Reward{
		Items: []*game.Item{{
			Id:  targetItem.ItemID,
			Num: targetItem.Count,
		}},
	}

	// 更新购买次数
	s.processPermanentItemPurchase(targetItem)

	return reward, cost, nil
}

// buyBoxShopItem 购买宝箱商店商品
func (s *ApiServer) buyBoxShopItem(ctx context.Context, shopData *ShopData, useAd bool, adWatched bool, count int32) (*game.Reward, *game.Wallet, bool, error) {
	if shopData.BoxShop.Box1Item == nil {
		return nil, nil, false, fmt.Errorf("宝箱商店未初始化")
	}

	boxItem := shopData.BoxShop.Box1Item

	// 计算消耗
	cost := &game.Wallet{}
	if useAd {
		if adWatched {
			// 客户端已看完广告，免费
			cost = &game.Wallet{}
		} else {
			// 客户端未看完广告，扣除广告券
			cost.Ad = 1
		}
		boxItem.CanSeeAD = false
	} else {
		// 使用货币购买
		payType := convertToProtoMoneyType(boxItem.PayType)
		if count > 1 {
			cost = calculateCost(payType, boxItem.WholesalePrice*count)
		} else {
			cost = calculateCost(payType, boxItem.RetailPrice)
		}
	}

	// 从模板读取奖励
	tplBoxItem, ok := s.template.GetTplBoxShopItem().FindByKey(boxItem.ShopItemID)
	if !ok {
		return nil, nil, false, fmt.Errorf("未找到宝箱配置")
	}

	// 解析奖励
	reward := parseRewardString(tplBoxItem.RewardInfo, s.logger)

	// 增加宝箱经验并检查升级
	shopData.BoxShop.BoxShopExp += tplBoxItem.BoxExp * count

	nextLevel := shopData.BoxShop.BoxShopLevel + 1
	tplBoxShops := s.template.GetTplBoxShop().FindByFilter(func(bs TplBoxShop) bool {
		return bs.Level == nextLevel
	}).ToSlice()

	levelUp := false
	if len(tplBoxShops) > 0 {
		tplBoxShop := tplBoxShops[0]
		if shopData.BoxShop.BoxShopExp >= tplBoxShop.RequiredExp {
			// 升级
			shopData.BoxShop.BoxShopExp = 0
			shopData.BoxShop.BoxShopLevel = nextLevel

			if tplNewBoxItem, ok := s.template.GetTplBoxShopItem().FindByKey(tplBoxShop.Box1); ok {
				shopData.BoxShop.Box1Item = &BoxShopItem{
					CanSeeAD:        true,
					NextRefreshTime: time.Now().UTC().Add(3 * time.Hour),
					ShopItemID:      tplNewBoxItem.ID,
					PayType:         tplNewBoxItem.PayType,
					RetailPrice:     tplNewBoxItem.RetailPrice,
					WholesalePrice:  tplNewBoxItem.WholesalePrice,
					RefreshTime:     tplNewBoxItem.RefreshTime,
				}
			}

			levelUp = true
		}
	}

	return reward, cost, levelUp, nil
}

// buyChapterShopItem 购买章节商店商品
func (s *ApiServer) buyChapterShopItem(ctx context.Context, shopData *ShopData, itemId string) (*game.Reward, error) {
	// 查找章节商店配置
	tplChapterItem, ok := s.template.GetTplShopChapterItem().FindByKey(itemId)
	if !ok {
		return nil, fmt.Errorf("未找到章节商店配置")
	}

	// 检查购买次数
	if shopData.ChapterShop == nil {
		shopData.ChapterShop = make(map[string]int32)
	}

	// 使用配置表中的购买次数限制
	maxCount := tplChapterItem.Count
	if maxCount <= 0 {
		maxCount = 1 // 默认1次
	}

	currentCount := shopData.ChapterShop[itemId]
	if currentCount >= maxCount {
		return nil, fmt.Errorf("购买次数已达上限")
	}

	// 解析奖励
	reward := parseRewardString(tplChapterItem.Reward, s.logger)

	// 更新购买次数
	shopData.ChapterShop[itemId] = currentCount + 1

	return reward, nil
}

// buyGemShopItem 购买钻石商店商品
func (s *ApiServer) buyGemShopItem(ctx context.Context, shopData *ShopData, itemId string) (*game.Reward, error) {
	// 查找钻石商店配置
	tplGemItem, ok := s.template.GetTplShopGemItem().FindByKey(itemId)
	if !ok {
		return nil, fmt.Errorf("未找到钻石商店配置")
	}

	// 检查是否首次购买
	if shopData.GemShop == nil {
		shopData.GemShop = make(map[string]int32)
	}

	currentCount := shopData.GemShop[itemId]
	isFirstBuy := currentCount == 0

	// 计算钻石数量
	gemCount := tplGemItem.Num
	if isFirstBuy {
		gemCount += tplGemItem.ExtraNum // 首次购买额外赠送
	}

	// 构建奖励
	reward := &game.Reward{
		Wallet: &game.Wallet{
			Gem: gemCount,
		},
	}

	// 更新购买次数
	shopData.GemShop[itemId] = currentCount + 1

	return reward, nil
}

// calculateCost 计算消耗的货币
func calculateCost(payType game.MoneyType, amount int32) *game.Wallet {
	cost := &game.Wallet{}

	switch payType {
	case game.MoneyType_MONEY_TYPE_COIN:
		cost.Coin = amount
	case game.MoneyType_MONEY_TYPE_GEM:
		cost.Gem = amount
	case game.MoneyType_MONEY_TYPE_AD:
		cost.Ad = amount
	}

	return cost
}

// parseRewardString 解析奖励字符串（格式: "coin_100,gem_50,10001_10"）
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

		// 判断是货币还是道具
		switch key {
		case "coin":
			reward.Wallet.Coin = int32(num)
		case "gem":
			reward.Wallet.Gem = int32(num)
		case "ad":
			reward.Wallet.Ad = int32(num)
		default:
			// 道具
			reward.Items = append(reward.Items, &game.Item{
				Id:  key,
				Num: int32(num),
			})
		}
	}

	return reward
}
