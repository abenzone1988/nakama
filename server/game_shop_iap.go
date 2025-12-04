package server

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetChapterShop RPC获取章节商店数据（IAP）
func (s *ApiServer) GetChapterShop(ctx context.Context, in *emptypb.Empty) (*game.ChapterShopData, error) {
	chapterShopData := &ChapterShopData{}
	if err := LoadUserData(ctx, s.logger, s.db, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败", zap.Error(err))
		// 首次初始化
		chapterShopData.Init()
	}

	// 确保 BoughtCounts 不为 nil
	if chapterShopData.BoughtCounts == nil {
		chapterShopData.BoughtCounts = make(map[string]int32)
	}

	// 确保 ClaimedCounts 不为 nil
	if chapterShopData.ClaimedCounts == nil {
		chapterShopData.ClaimedCounts = make(map[string]int32)
	}

	// 每次都从模板读取商品列表和价格，合并购买次数
	return convertToProtoChapterShop(chapterShopData, s.template, s.logger), nil
}

// GetGemShop RPC获取钻石商店数据（IAP）
func (s *ApiServer) GetGemShop(ctx context.Context, in *emptypb.Empty) (*game.GemShopData, error) {
	gemShopData := &GemShopData{}
	if err := LoadUserData(ctx, s.logger, s.db, gemShopData); err != nil {
		s.logger.Error("加载钻石商店数据失败", zap.Error(err))
		// 首次初始化
		gemShopData.Init()
	}

	// 确保 BoughtCounts 不为 nil
	if gemShopData.BoughtCounts == nil {
		gemShopData.BoughtCounts = make(map[string]int32)
	}

	// 确保 ClaimedCounts 不为 nil
	if gemShopData.ClaimedCounts == nil {
		gemShopData.ClaimedCounts = make(map[string]int32)
	}

	// 每次都从模板读取商品列表和价格，合并购买次数
	return convertToProtoGemShop(gemShopData, s.template), nil
}

// 购买章节商品（独立接口，IAP）
func (s *ApiServer) ClaimChapterItem(ctx context.Context, in *game.ClaimChapterItemRequest) (*game.ClaimChapterItemResponse, error) {
	chapterShopData := &ChapterShopData{}
	if err := LoadUserData(ctx, s.logger, s.db, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败", zap.Error(err))
		return &game.ClaimChapterItemResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	// 确保 BoughtCounts 不为 nil
	if chapterShopData.BoughtCounts == nil {
		chapterShopData.BoughtCounts = make(map[string]int32)
	}

	// 通过 id 查找商品
	tplItem, ok := s.template.GetTplShopChapterItem().FindByKey(in.Id)
	if !ok {
		return &game.ClaimChapterItemResponse{Code: 1, Msg: "商品不存在"}, nil
	}

	shopItemID := tplItem.ID

	// 确保 ClaimedCounts 不为 nil
	if chapterShopData.ClaimedCounts == nil {
		chapterShopData.ClaimedCounts = make(map[string]int32)
	}

	// 检查是否有可领取次数
	boughtCount := int32(0)
	if chapterShopData.BoughtCounts != nil {
		boughtCount = chapterShopData.BoughtCounts[shopItemID]
	}
	claimedCount := chapterShopData.ClaimedCounts[shopItemID]

	if boughtCount <= claimedCount {
		return &game.ClaimChapterItemResponse{Code: 2, Msg: "没有可领取的商品"}, nil
	}

	// 处理章节商品购买逻辑
	reward, err := s.processBuyChapterItemStandalone(shopItemID)
	if err != nil {
		return &game.ClaimChapterItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// IAP 商品无需扣除货币，直接发放奖励
	_, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "chapter_shop")
	if err != nil {
		return &game.ClaimChapterItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 更新领取次数
	chapterShopData.ClaimedCounts[shopItemID] = claimedCount + 1

	// 保存商店数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, chapterShopData); err != nil {
		s.logger.Error("保存章节商店数据失败", zap.Error(err))
		return &game.ClaimChapterItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	// 获取价格
	price := ""
	tplPay, ok := s.template.GetTplPay().FindByKey(tplItem.PayID)
	if ok {
		price = tplPay.Money
	}

	// 构建返回的 ShopChapterItem
	claimedCountAfter := chapterShopData.ClaimedCounts[shopItemID]
	shopChapterItem := &game.ShopChapterItem{
		Id:           shopItemID,
		LevelId:      tplItem.LevelID,
		Price:        price,
		RewardId:     tplItem.Reward,
		MaxBuyCount:  tplItem.Count,
		BoughtCount:  boughtCount,
		ClaimedCount: claimedCountAfter,
	}

	response := &game.ClaimChapterItemResponse{
		Code:     0,
		Msg:      "领取成功",
		Reward:   reward,
		ShopItem: shopChapterItem,
	}

	if inventoryUpdateResult != nil {
		response.InventoryUpdated = inventoryUpdateResult.Updated
	}

	return response, nil
}

// 购买钻石商品（独立接口，IAP）
func (s *ApiServer) ClaimGemItem(ctx context.Context, in *game.ClaimGemItemRequest) (*game.ClaimGemItemResponse, error) {
	gemShopData := &GemShopData{}
	if err := LoadUserData(ctx, s.logger, s.db, gemShopData); err != nil {
		s.logger.Error("加载钻石商店数据失败", zap.Error(err))
		return &game.ClaimGemItemResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	// 确保 BoughtCounts 不为 nil
	if gemShopData.BoughtCounts == nil {
		gemShopData.BoughtCounts = make(map[string]int32)
	}

	// 通过 id 查找商品
	tplItem, ok := s.template.GetTplShopGemItem().FindByKey(in.Id)
	if !ok {
		return &game.ClaimGemItemResponse{Code: 1, Msg: "商品不存在"}, nil
	}

	shopItemID := tplItem.ID

	// 确保 ClaimedCounts 不为 nil
	if gemShopData.ClaimedCounts == nil {
		gemShopData.ClaimedCounts = make(map[string]int32)
	}

	// 检查是否有可领取次数
	boughtCount := int32(0)
	if gemShopData.BoughtCounts != nil {
		boughtCount = gemShopData.BoughtCounts[shopItemID]
	}
	claimedCount := gemShopData.ClaimedCounts[shopItemID]

	if boughtCount <= claimedCount {
		return &game.ClaimGemItemResponse{Code: 2, Msg: "没有可领取的商品"}, nil
	}

	// 处理钻石商品购买逻辑（首次购买有额外奖励）
	reward, err := s.processBuyGemItemStandalone(shopItemID, claimedCount)
	if err != nil {
		return &game.ClaimGemItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// IAP 商品直接发放钻石
	walletUpdateResult, _, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "gem_shop")
	if err != nil {
		return &game.ClaimGemItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 更新领取次数
	gemShopData.ClaimedCounts[shopItemID] = claimedCount + 1

	// 保存商店数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, gemShopData); err != nil {
		s.logger.Error("保存钻石商店数据失败", zap.Error(err))
		return &game.ClaimGemItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	// 获取价格
	price := ""
	tplPay, ok := s.template.GetTplPay().FindByKey(tplItem.PayID)
	if ok {
		price = tplPay.Money
	}

	// 构建返回的 ShopGemItem
	claimedCountAfter := gemShopData.ClaimedCounts[shopItemID]
	shopGemItem := &game.ShopGemItem{
		Id:           shopItemID,
		Num:          tplItem.Num,
		Price:        price, // 价格从 TplPay 获取，这里不返回
		BoughtCount:  boughtCount,
		ClaimedCount: claimedCountAfter,
	}

	response := &game.ClaimGemItemResponse{
		Code:     0,
		Msg:      "领取成功",
		Reward:   reward,
		ShopItem: shopGemItem,
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}

	return response, nil
}

// 转换章节商店数据（从模板读取商品列表和价格）
func convertToProtoChapterShop(data *ChapterShopData, tmpl TemplateManager, logger *zap.Logger) *game.ChapterShopData {
	allItems := tmpl.GetTplShopChapterItem().FindAll().ToSlice()
	protoItems := make([]*game.ShopChapterItem, 0, len(allItems))

	for _, tplItem := range allItems {
		boughtCount := int32(0)
		if data.BoughtCounts != nil {
			boughtCount = data.BoughtCounts[tplItem.ID]
		}
		claimedCount := int32(0)
		if data.ClaimedCounts != nil {
			claimedCount = data.ClaimedCounts[tplItem.ID]
		}

		tplPay, ok := tmpl.GetTplPay().FindByKey(tplItem.PayID)
		if !ok {
			logger.Error("未找到支付配置", zap.String("pay_id", tplItem.PayID))
			continue
		}

		protoItems = append(protoItems, &game.ShopChapterItem{
			Id:           tplItem.ID,
			LevelId:      tplItem.LevelID,
			Price:        tplPay.Money,
			RewardId:     tplItem.Reward,
			MaxBuyCount:  tplItem.Count,
			BoughtCount:  boughtCount,
			ClaimedCount: claimedCount,
		})
	}

	// 按照商品ID由小到大排序（按数字大小排序，如果ID不是数字则按字符串排序）
	sort.Slice(protoItems, func(i, j int) bool {
		idI, errI := strconv.Atoi(protoItems[i].Id)
		idJ, errJ := strconv.Atoi(protoItems[j].Id)

		// 如果两个ID都是数字，按数字大小排序
		if errI == nil && errJ == nil {
			return idI < idJ
		}

		// 如果只有一个ID是数字，数字排在前面
		if errI == nil {
			return true
		}
		if errJ == nil {
			return false
		}

		// 如果两个ID都不是数字，按字符串排序
		return protoItems[i].Id < protoItems[j].Id
	})

	return &game.ChapterShopData{
		Items: protoItems,
	}
}

// convertToProtoGemShop 转换钻石商店数据（从模板读取商品列表和价格）
func convertToProtoGemShop(data *GemShopData, tmpl TemplateManager) *game.GemShopData {
	allItems := tmpl.GetTplShopGemItem().FindAll().ToSlice()
	protoItems := make([]*game.ShopGemItem, 0, len(allItems))

	for _, tplItem := range allItems {
		boughtCount := int32(0)
		if data.BoughtCounts != nil {
			boughtCount = data.BoughtCounts[tplItem.ID]
		}
		claimedCount := int32(0)
		if data.ClaimedCounts != nil {
			claimedCount = data.ClaimedCounts[tplItem.ID]
		}

		// 获取价格
		price := ""
		tplPay, ok := tmpl.GetTplPay().FindByKey(tplItem.PayID)
		if ok {
			price = tplPay.Money
		}

		protoItems = append(protoItems, &game.ShopGemItem{
			Id:           tplItem.ID,
			Num:          tplItem.Num,
			Price:        price,
			BoughtCount:  boughtCount,
			ClaimedCount: claimedCount,
		})
	}

	// 按照商品ID由小到大排序（按数字大小排序，如果ID不是数字则按字符串排序）
	sort.Slice(protoItems, func(i, j int) bool {
		idI, errI := strconv.Atoi(protoItems[i].Id)
		idJ, errJ := strconv.Atoi(protoItems[j].Id)

		// 如果两个ID都是数字，按数字大小排序
		if errI == nil && errJ == nil {
			return idI < idJ
		}

		// 如果只有一个ID是数字，数字排在前面
		if errI == nil {
			return true
		}
		if errJ == nil {
			return false
		}

		// 如果两个ID都不是数字，按字符串排序
		return protoItems[i].Id < protoItems[j].Id
	})

	return &game.GemShopData{
		Items: protoItems,
	}
}

// 处理章节商品购买（独立版本）
func (s *ApiServer) processBuyChapterItemStandalone(shopItemID string) (*game.Reward, error) {
	tplItem, ok := s.template.GetTplShopChapterItem().FindByKey(shopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到章节商品配置")
	}

	reward := GetReward(tplItem.Reward, s.template.GetTplReward(), s.logger)
	return reward, nil
}

// 处理钻石商品购买（独立版本）
func (s *ApiServer) processBuyGemItemStandalone(shopItemID string, boughtCount int32) (*game.Reward, error) {
	tplItem, ok := s.template.GetTplShopGemItem().FindByKey(shopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到钻石商品配置")
	}

	gemCount := tplItem.Num
	// 首次购买（boughtCount == 0）有额外奖励
	if boughtCount == 0 {
		gemCount += tplItem.ExtraNum
	}

	reward := &game.Reward{
		Wallet: &game.Wallet{
			Gem: gemCount,
		},
	}

	return reward, nil
}
