package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// ExchangeEquip 武器兑换
func (s *ApiServer) ExchangeEquip(ctx context.Context, in *game.ExchangeEquipRequest) (*game.ExchangeEquipResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	if in.Id == "" {
		return &game.ExchangeEquipResponse{Code: 1, Msg: "id不能为空"}, nil
	}
	reqCount := in.Count
	if reqCount <= 0 {
		reqCount = 1
	}

	cfg, ok := s.template.GetTplEquipExchange().FindByKey(in.Id)
	if !ok {
		return &game.ExchangeEquipResponse{Code: 2, Msg: "兑换配置不存在"}, nil
	}
	if cfg.Count <= 0 {
		return &game.ExchangeEquipResponse{Code: 3, Msg: "配置数量无效"}, nil
	}

	now := time.Now().UTC()
	weekKey := GetWeekKey(ctx, now)

	exData := &EquipExchangeData{}
	if err := LoadUserData(ctx, s.logger, s.db, exData); err != nil {
		s.logger.Error("加载兑换数据失败", zap.Error(err))
	}
	if exData.WeekKey != weekKey {
		exData.WeekKey = weekKey
		exData.BoughtCounts = make(map[string]int32)
	}

	// cfg.Count 表示【每次购买】获得的物品数量（例如：每买1次给2个碎片）
	// 因此：
	// - in.Count / BoughtCounts / WeekLimit 都应该用【购买次数】作为单位
	// - 最终发放数量 = 本次实际购买次数 * cfg.Count

	boughtTimes := exData.BoughtCounts[in.Id]  // 本周已购买次数
	remainTimes := cfg.WeekLimit - boughtTimes // 本周剩余可购买次数
	if remainTimes <= 0 {
		return &game.ExchangeEquipResponse{Code: 4, Msg: "本周购买已达上限"}, nil
	}

	buyTimes := reqCount // 用户请求的购买次数
	if buyTimes > remainTimes {
		buyTimes = remainTimes
	}

	s.logger.Info("装备兑换-购买次数计算",
		zap.String("user_id", userID.String()),
		zap.String("exchange_id", in.Id),
		zap.Int32("req_count", reqCount),
		zap.Int32("bought_times", boughtTimes),
		zap.Int32("week_limit", cfg.WeekLimit),
		zap.Int32("remain_times", remainTimes),
		zap.Int32("buy_times", buyTimes))

	// cfg.Costs 表示每次购买的消耗梯度（第1次、第2次……），
	// 如果购买次数超过 Costs 长度，则后续都按最后一个价格计算。
	if len(cfg.Costs) == 0 {
		return &game.ExchangeEquipResponse{Code: 5, Msg: "配置错误：Costs为空"}, nil
	}

	var totalCost int32
	var costDetails []int32
	for i := int32(0); i < buyTimes; i++ {
		costIdx := int(boughtTimes + i) // 第 (boughtTimes+i+1) 次购买对应的价格档位
		if costIdx >= len(cfg.Costs) {
			costIdx = len(cfg.Costs) - 1
		}
		if costIdx < 0 {
			return &game.ExchangeEquipResponse{Code: 5, Msg: "配置错误：costIdx无效"}, nil
		}
		cost := cfg.Costs[costIdx]
		costDetails = append(costDetails, cost)
		totalCost += cost
	}

	// 本次实际获得的物品数量 = 购买次数 * cfg.Count
	gainNum := buyTimes * cfg.Count

	s.logger.Info("装备兑换-价格和数量计算",
		zap.String("user_id", userID.String()),
		zap.String("exchange_id", in.Id),
		zap.Int32("buy_times", buyTimes),
		zap.Int32s("cost_details", costDetails),
		zap.Int32("total_cost", totalCost),
		zap.Int32("cfg_count", cfg.Count),
		zap.Int32("gain_num", gainNum))

	costStr := fmt.Sprintf("%s_%d", ItemID_ExchangePoint, totalCost)
	wCost, iCost, err := ConsumeCostItems(ctx, s.logger, s.db, s.template, costStr, "equip_exchange_cost")
	if err != nil {
		return &game.ExchangeEquipResponse{Code: 6, Msg: "积分不足或扣除失败"}, nil
	}

	reward := &game.Reward{
		Items: []*game.Item{
			{Id: cfg.ItemID, Num: gainNum},
		},
	}
	wReward, iReward, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "equip_exchange_reward")
	if err != nil {
		return &game.ExchangeEquipResponse{Code: 7, Msg: "发放奖励失败"}, nil
	}

	// 写回：累计购买次数（不是累计物品数量）
	newBoughtTimes := boughtTimes + buyTimes
	exData.BoughtCounts[in.Id] = newBoughtTimes
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, exData); err != nil {
		return &game.ExchangeEquipResponse{Code: 8, Msg: "保存数据失败"}, nil
	}

	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item
	if wCost != nil {
		walletUpdated = wCost.Updated
	}
	if wReward != nil {
		walletUpdated = wReward.Updated
	}
	if iCost != nil {
		inventoryUpdated = append(inventoryUpdated, iCost.Updated...)
	}
	if iReward != nil {
		inventoryUpdated = append(inventoryUpdated, iReward.Updated...)
	}

	s.logger.Info("装备兑换成功",
		zap.String("user_id", userID.String()),
		zap.String("exchange_id", in.Id),
		zap.Int32("count", buyTimes),
		zap.Int32("cost", totalCost),
		zap.Int32("bought_total", gainNum))

	return &game.ExchangeEquipResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ExchangedCount:   gainNum,
		TotalCost:        totalCost,
		BoughtCount:      buyTimes,
	}, nil
}

// GetEquipExchange 获取装备兑换数据
func (s *ApiServer) GetEquipExchange(ctx context.Context, in *game.GetEquipExchangeRequest) (*game.GetEquipExchangeResponse, error) {
	now := time.Now().UTC()
	weekKey := GetWeekKey(ctx, now)

	exData := &EquipExchangeData{}
	if err := LoadUserData(ctx, s.logger, s.db, exData); err != nil {
		s.logger.Warn("加载兑换数据失败，初始化新数据", zap.Error(err))
		exData.Init()
	}
	if exData.WeekKey != weekKey {
		exData.WeekKey = weekKey
		exData.BoughtCounts = make(map[string]int32)
	}

	exchangeInfos := make(map[string]*game.EquipExchangeInfo, len(exData.BoughtCounts))

	for id, boughtCount := range exData.BoughtCounts {
		exchangeInfos[id] = &game.EquipExchangeInfo{
			Id:          id,
			BoughtCount: boughtCount,
			WeekLimit:   0,
		}
	}

	return &game.GetEquipExchangeResponse{
		Code:          0,
		Msg:           "Success",
		ExchangeInfos: exchangeInfos,
	}, nil
}
