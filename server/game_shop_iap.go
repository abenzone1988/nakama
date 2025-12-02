package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetChapterShop RPC获取章节商店数据（IAP）
func (s *ApiServer) GetChapterShop(ctx context.Context, in *emptypb.Empty) (*game.ChapterShopData, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	chapterShopData := &ChapterShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败", zap.Error(err))
		// 首次初始化
		chapterShopData.Init()
	}

	// 确保 BoughtCounts 不为 nil
	if chapterShopData.BoughtCounts == nil {
		chapterShopData.BoughtCounts = make(map[string]int32)
	}

	// 每次都从模板读取商品列表和价格，合并购买次数
	return convertToProtoChapterShop(chapterShopData, s.template), nil
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

	// 确保 BoughtCounts 不为 nil
	if gemShopData.BoughtCounts == nil {
		gemShopData.BoughtCounts = make(map[string]int32)
	}

	// 每次都从模板读取商品列表和价格，合并购买次数
	return convertToProtoGemShop(gemShopData, s.template), nil
}

// BuyChapterItem 购买章节商品（独立接口，IAP）
func (s *ApiServer) BuyChapterItem(ctx context.Context, in *game.BuyChapterItemRequest) (*game.BuyChapterItemResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	chapterShopData := &ChapterShopData{}
	if err := LoadData(ctx, s.logger, s.db, userID, chapterShopData); err != nil {
		s.logger.Error("加载章节商店数据失败", zap.Error(err))
		return &game.BuyChapterItemResponse{Code: -1, Msg: "加载商店数据失败"}, nil
	}

	// 确保 BoughtCounts 不为 nil
	if chapterShopData.BoughtCounts == nil {
		chapterShopData.BoughtCounts = make(map[string]int32)
	}

	// 通过 id 查找商品
	tplItem, ok := s.template.GetTplShopChapterItem().FindByKey(in.Id)
	if !ok {
		return &game.BuyChapterItemResponse{Code: 1, Msg: "商品不存在"}, nil
	}

	shopItemID := tplItem.ID

	// 检查购买次数限制
	boughtCount := chapterShopData.BoughtCounts[shopItemID]
	if tplItem.Count > 0 && boughtCount >= tplItem.Count {
		return &game.BuyChapterItemResponse{Code: 3, Msg: "已达到最大购买次数"}, nil
	}

	// 处理章节商品购买逻辑
	reward, err := s.processBuyChapterItemStandalone(shopItemID)
	if err != nil {
		return &game.BuyChapterItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// IAP 商品无需扣除货币，直接发放奖励
	_, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "chapter_shop")
	if err != nil {
		return &game.BuyChapterItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 更新购买次数
	chapterShopData.BoughtCounts[shopItemID] = boughtCount + 1

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, chapterShopData); err != nil {
		s.logger.Error("保存章节商店数据失败", zap.Error(err))
		return &game.BuyChapterItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	response := &game.BuyChapterItemResponse{
		Code:            0,
		Msg:             "购买成功",
		Reward:          reward,
		ChapterShopData: convertToProtoChapterShop(chapterShopData, s.template),
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

	// 确保 BoughtCounts 不为 nil
	if gemShopData.BoughtCounts == nil {
		gemShopData.BoughtCounts = make(map[string]int32)
	}

	// 通过 id 查找商品
	tplItem, ok := s.template.GetTplShopGemItem().FindByKey(in.Id)
	if !ok {
		return &game.BuyGemItemResponse{Code: 1, Msg: "商品不存在"}, nil
	}

	shopItemID := tplItem.ID
	boughtCount := gemShopData.BoughtCounts[shopItemID]

	// 处理钻石商品购买逻辑（首次购买有额外奖励）
	reward, err := s.processBuyGemItemStandalone(shopItemID, boughtCount)
	if err != nil {
		return &game.BuyGemItemResponse{Code: 4, Msg: err.Error()}, nil
	}

	// IAP 商品直接发放钻石
	walletUpdateResult, _, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "gem_shop")
	if err != nil {
		return &game.BuyGemItemResponse{Code: 5, Msg: err.Error()}, nil
	}

	// 更新购买次数
	gemShopData.BoughtCounts[shopItemID] = gemShopData.BoughtCounts[shopItemID] + 1

	// 保存商店数据
	if err := SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, gemShopData); err != nil {
		s.logger.Error("保存钻石商店数据失败", zap.Error(err))
		return &game.BuyGemItemResponse{Code: 6, Msg: "保存商店数据失败"}, nil
	}

	response := &game.BuyGemItemResponse{
		Code:        0,
		Msg:         "购买成功",
		Reward:      reward,
		GemShopData: convertToProtoGemShop(gemShopData, s.template),
	}

	if walletUpdateResult != nil {
		response.WalletUpdated = walletUpdateResult.Updated
	}

	return response, nil
}

// convertToProtoChapterShop 转换章节商店数据（从模板读取商品列表和价格）
func convertToProtoChapterShop(data *ChapterShopData, tmpl TemplateManager) *game.ChapterShopData {
	allItems := tmpl.GetTplShopChapterItem().FindAll().ToSlice()
	protoItems := make([]*game.ShopItem, 0, len(allItems))

	for _, tplItem := range allItems {
		boughtCount := int32(0)
		if data.BoughtCounts != nil {
			boughtCount = data.BoughtCounts[tplItem.ID]
		}

		protoItems = append(protoItems, &game.ShopItem{
			Id:      tplItem.ID,
			ItemId:  tplItem.Reward,
			PayType: game.PayType_IAP,
			//Price:       tplItem.Price,
			Discount:    0,
			ItemCount:   1,
			MaxBuyCount: tplItem.Count,
			BoughtCount: boughtCount,
		})
	}

	return &game.ChapterShopData{
		Items: protoItems,
	}
}

// convertToProtoGemShop 转换钻石商店数据（从模板读取商品列表和价格）
func convertToProtoGemShop(data *GemShopData, tmpl TemplateManager) *game.GemShopData {
	allItems := tmpl.GetTplShopGemItem().FindAll().ToSlice()
	protoItems := make([]*game.ShopItem, 0, len(allItems))

	for _, tplItem := range allItems {
		boughtCount := int32(0)
		if data.BoughtCounts != nil {
			boughtCount = data.BoughtCounts[tplItem.ID]
		}

		protoItems = append(protoItems, &game.ShopItem{
			Id:      tplItem.ID,
			ItemId:  ItemID_Gem,
			PayType: game.PayType_IAP,
			//Price:       tplItem.Price,
			Discount:    0,
			ItemCount:   tplItem.Num,
			MaxBuyCount: 0, // 无限制
			BoughtCount: boughtCount,
		})
	}

	return &game.GemShopData{
		Items: protoItems,
	}
}

// processBuyChapterItemStandalone 处理章节商品购买（独立版本）
func (s *ApiServer) processBuyChapterItemStandalone(shopItemID string) (*game.Reward, error) {
	tplItem, ok := s.template.GetTplShopChapterItem().FindByKey(shopItemID)
	if !ok {
		return nil, fmt.Errorf("未找到章节商品配置")
	}

	reward := parseRewardString(tplItem.Reward, s.logger)
	return reward, nil
}

// processBuyGemItemStandalone 处理钻石商品购买（独立版本）
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

// parseRewardString 解析奖励字符串（格式：key_num,key_num，如 "coin_100,gem_50"）
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
