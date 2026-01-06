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
		exData.Init()
	}
	if exData.WeekKey != weekKey {
		exData.WeekKey = weekKey
		exData.BoughtCounts = make(map[string]int32)
	}

	bought := exData.BoughtCounts[in.Id]
	remain := cfg.WeekLimit - bought
	if remain <= 0 {
		return &game.ExchangeEquipResponse{Code: 4, Msg: "本周购买已达上限"}, nil
	}
	if reqCount > remain {
		reqCount = remain
	}

	var gained int32
	var totalCost int32
	currentBought := bought
	purchaseIndex := currentBought / cfg.Count

	for gained < reqCount && currentBought < cfg.WeekLimit {
		add := cfg.Count
		if gained+add > reqCount {
			add = reqCount - gained
		}

		costIdx := int(purchaseIndex)
		if costIdx >= len(cfg.Costs) {
			costIdx = len(cfg.Costs) - 1
		}
		if costIdx < 0 || len(cfg.Costs) == 0 {
			return &game.ExchangeEquipResponse{Code: 5, Msg: "配置错误"}, nil
		}

		// totalCost 累加每一次购买批次的消耗（超过长度用末尾值）
		totalCost += cfg.Costs[costIdx]
		gained += add
		currentBought += add
		purchaseIndex++
	}

	if gained <= 0 {
		return &game.ExchangeEquipResponse{Code: 4, Msg: "本周购买已达上限"}, nil
	}

	costStr := fmt.Sprintf("%s_%d", ItemID_ExchangePoint, totalCost)
	wCost, iCost, err := ConsumeCostItems(ctx, s.logger, s.db, s.template, costStr, "equip_exchange_cost")
	if err != nil {
		return &game.ExchangeEquipResponse{Code: 6, Msg: "积分不足或扣除失败"}, nil
	}

	reward := &game.Reward{
		Items: []*game.Item{
			{Id: cfg.ItemID, Num: gained},
		},
	}
	wReward, iReward, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "equip_exchange_reward")
	if err != nil {
		return &game.ExchangeEquipResponse{Code: 7, Msg: "发放奖励失败"}, nil
	}

	exData.BoughtCounts[in.Id] = currentBought
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
		zap.Int32("count", gained),
		zap.Int32("cost", totalCost),
		zap.Int32("bought_total", currentBought))

	return &game.ExchangeEquipResponse{
		Code:             0,
		Msg:              "Success",
		Reward:           reward,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
		ExchangedCount:   gained,
		TotalCost:        totalCost,
		BoughtCount:      currentBought,
	}, nil
}
