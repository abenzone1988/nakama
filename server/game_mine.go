package server

import (
	"context"
	"math/rand"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	. "github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
)

const DailyMineCount = 30  // 每天挖矿次数
const MaxBuyMineCount = 5  // 最大购买挖矿次数
const MineCountPerBuy = 10 // 每次购买挖矿次数
const BuyMineCostGem = 10  // 每次购买挖矿消耗的宝石数量

// RefreshDailyMineCount 刷新每日挖矿次数
func RefreshDailyMineCount(ctx context.Context, logger *zap.Logger, s *ApiServer, mineData *MineData) bool {
	now := time.Now()
	today := now.Format("2006-01-02")

	if mineData.LastRefreshDate != today {
		mineData.DailyMineCount = DailyMineCount
		mineData.BuyCount = 0 // 重置每日购买次数
		mineData.LastRefreshDate = today
		logger.Info("刷新每日挖矿次数", zap.Int32("count", DailyMineCount))
		return true
	}
	return false
}

// GetMineData 获取挖矿数据
func (s *ApiServer) GetMineData(ctx context.Context, in *game.GetMineDataRequest) (*game.GetMineDataResponse, error) {
	// 加载挖矿数据
	mineData := &MineData{}
	if err := LoadUserData(ctx, s.logger, s.db, mineData); err != nil {
		s.logger.Error("加载挖矿数据失败", zap.Error(err))
		return &game.GetMineDataResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	// 刷新每日次数
	refreshed := RefreshDailyMineCount(ctx, s.logger, s, mineData)
	if refreshed {
		// 保存刷新后的数据
		if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, mineData); err != nil {
			s.logger.Error("保存挖矿数据失败", zap.Error(err))
		}
	}

	return &game.GetMineDataResponse{
		Code: 0,
		Msg:  "获取成功",
		Data: &game.MineData{
			DailyMineCount:    mineData.DailyMineCount,
			LastRefreshDate:   mineData.LastRefreshDate,
			CurrentLevel:      mineData.CurrentLevel,
			CurrentMinedCount: mineData.CurrentMinedCount,
			IsCompleted:       mineData.IsCompleted,
			BuyCount:          mineData.BuyCount,
		},
	}, nil
}

// BuyMineCount 购买挖矿次数
func (s *ApiServer) BuyMineCount(ctx context.Context, in *game.BuyMineCountRequest) (*game.BuyMineCountResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 加载挖矿数据
	mineData := &MineData{}
	if err := LoadUserData(ctx, s.logger, s.db, mineData); err != nil {
		s.logger.Error("加载挖矿数据失败", zap.Error(err))
		return &game.BuyMineCountResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	// 刷新每日次数
	RefreshDailyMineCount(ctx, s.logger, s, mineData)

	// 检查购买次数是否已达上限
	if mineData.BuyCount >= MaxBuyMineCount {
		return &game.BuyMineCountResponse{
			Code: 2,
			Msg:  "今日购买次数已达上限",
		}, nil
	}

	// 扣除宝石
	changeset := map[string]int64{
		"gem": -int64(BuyMineCostGem),
	}

	walletResults, err := UpdateWallets(ctx, s.logger, s.db, []*walletUpdate{
		{
			UserID:    userID,
			Changeset: changeset,
			Metadata:  "{\"reason\": \"buy_mine_count\"}",
		},
	}, true)
	if err != nil {
		s.logger.Error("扣除宝石失败", zap.Error(err))
		return &game.BuyMineCountResponse{
			Code: 3,
			Msg:  "宝石不足",
		}, nil
	}

	var walletUpdateResult *game.WalletUpdateResult
	if len(walletResults) > 0 && walletResults[0] != nil {
		walletUpdateResult = &game.WalletUpdateResult{
			Previous: convertMapInt64ToWallet(walletResults[0].Previous),
			Updated:  convertMapInt64ToWallet(walletResults[0].Updated),
		}
	}

	// 增加挖矿次数
	mineData.DailyMineCount += MineCountPerBuy
	mineData.BuyCount++

	// 保存挖矿数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, mineData); err != nil {
		s.logger.Error("保存挖矿数据失败", zap.Error(err))
		return &game.BuyMineCountResponse{
			Code: 4,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("购买挖矿次数成功",
		zap.Int32("buy_count", mineData.BuyCount),
		zap.Int32("daily_mine_count", mineData.DailyMineCount))

	var walletUpdated *game.Wallet
	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}

	return &game.BuyMineCountResponse{
		Code:           0,
		Msg:            "购买成功",
		BuyCount:       mineData.BuyCount,
		DailyMineCount: mineData.DailyMineCount,
		WalletUpdated:  walletUpdated,
	}, nil
}

// DoMine 执行挖矿
func (s *ApiServer) DoMine(ctx context.Context, in *game.DoMineRequest) (*game.DoMineResponse, error) {
	// 加载挖矿数据
	mineData := &MineData{}
	if err := LoadUserData(ctx, s.logger, s.db, mineData); err != nil {
		s.logger.Error("加载挖矿数据失败", zap.Error(err))
		return &game.DoMineResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	// 刷新每日次数
	RefreshDailyMineCount(ctx, s.logger, s, mineData)

	// 检查是否已挖完
	if mineData.IsCompleted {
		return &game.DoMineResponse{
			Code: 2,
			Msg:  "矿产已挖完，请升级到下一等级",
		}, nil
	}

	// 检查剩余次数
	if mineData.DailyMineCount <= 0 {
		return &game.DoMineResponse{
			Code: 3,
			Msg:  "今日挖矿次数已用完",
		}, nil
	}

	mineConfigSlice := s.template.GetTplMine().FindByFilter(func(m TplMine) bool {
		return m.Level == mineData.CurrentLevel
	})

	if mineConfigSlice.Len() == 0 {
		s.logger.Error("未找到矿产配置", zap.Int32("level", mineData.CurrentLevel))
		return &game.DoMineResponse{
			Code: 4,
			Msg:  "矿产配置不存在",
		}, nil
	}

	mineConfig := mineConfigSlice.Get(0)

	// 计算本次挖出的数量
	var minedCount int32
	// 按概率计算是否暴击
	randomValue := rand.Int31n(100) // 0-99
	if randomValue < mineConfig.Crit {
		// 暴击
		minedCount = mineConfig.Critcount
		s.logger.Debug("挖矿暴击", zap.Int32("count", minedCount), zap.Int32("random", randomValue))
	} else {
		// 普通
		minedCount = mineConfig.Count
		s.logger.Debug("挖矿普通", zap.Int32("count", minedCount), zap.Int32("random", randomValue))
	}

	// 累加数量并检查是否超过最大值
	actualGained := minedCount
	if mineData.CurrentMinedCount+minedCount >= mineConfig.Maxcount {
		// 超过最大值，只给剩余的数量
		actualGained = mineConfig.Maxcount - mineData.CurrentMinedCount
		mineData.CurrentMinedCount = mineConfig.Maxcount
		mineData.IsCompleted = true
		s.logger.Info("矿产已挖完",
			zap.Int32("level", mineData.CurrentLevel),
			zap.Int32("total", mineData.CurrentMinedCount))
	} else {
		mineData.CurrentMinedCount += minedCount
	}

	// 消耗挖矿次数
	mineData.DailyMineCount--

	// 发放晶核奖励
	reward := &game.Reward{
		Items: []*game.Item{
			{
				Id:  ItemID_CrystalCore,
				Num: actualGained,
			},
		},
	}

	walletUpdateResult, inventoryUpdateResult, err := GrantReward(ctx, s.logger, s.db, s.template, s.metrics, s.storageIndex, reward, "mine")
	if err != nil {
		s.logger.Error("发放挖矿奖励失败", zap.Error(err))
		return &game.DoMineResponse{
			Code: 5,
			Msg:  "发放奖励失败",
		}, nil
	}

	// 保存挖矿数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, mineData); err != nil {
		s.logger.Error("保存挖矿数据失败", zap.Error(err))
		return &game.DoMineResponse{
			Code: 6,
			Msg:  "保存数据失败",
		}, nil
	}

	// 提取更新后的数据
	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item
	if walletUpdateResult != nil {
		walletUpdated = walletUpdateResult.Updated
	}
	if inventoryUpdateResult != nil {
		inventoryUpdated = inventoryUpdateResult.Updated
	}

	s.logger.Info("挖矿成功",
		zap.Int32("level", mineData.CurrentLevel),
		zap.Int32("gained", actualGained),
		zap.Int32("current_mined", mineData.CurrentMinedCount),
		zap.Int32("remaining_count", mineData.DailyMineCount),
		zap.Bool("is_completed", mineData.IsCompleted))

	return &game.DoMineResponse{
		Code:              0,
		Msg:               "挖矿成功",
		Gained:            actualGained,
		IsCrit:            minedCount == mineConfig.Critcount,
		CurrentMinedCount: mineData.CurrentMinedCount,
		RemainingCount:    mineData.DailyMineCount,
		IsCompleted:       mineData.IsCompleted,
		Reward:            reward,
		WalletUpdated:     walletUpdated,
		InventoryUpdated:  inventoryUpdated,
	}, nil
}

// UpgradeMine 升级矿产
func (s *ApiServer) UpgradeMine(ctx context.Context, in *game.UpgradeMineRequest) (*game.UpgradeMineResponse, error) {
	// 加载挖矿数据
	mineData := &MineData{}
	if err := LoadUserData(ctx, s.logger, s.db, mineData); err != nil {
		s.logger.Error("加载挖矿数据失败", zap.Error(err))
		return &game.UpgradeMineResponse{
			Code: 1,
			Msg:  "加载数据失败",
		}, nil
	}

	// 检查是否已挖完
	if !mineData.IsCompleted {
		return &game.UpgradeMineResponse{
			Code: 2,
			Msg:  "当前矿产尚未挖完，无法升级",
		}, nil
	}

	// 升级到下一等级
	nextLevel := mineData.CurrentLevel + 1

	nextMineConfigSlice := s.template.GetTplMine().FindByFilter(func(m TplMine) bool {
		return m.Level == nextLevel
	})

	// 如果找不到下一等级配置，说明已达到最高等级，重置当前等级继续挖矿
	if nextMineConfigSlice.Len() == 0 {
		s.logger.Info("已达到最高等级，重置当前等级继续挖矿", zap.Int32("current_level", mineData.CurrentLevel))
	} else {
		// 升级到下一等级
		mineData.CurrentLevel = nextLevel
	}

	mineData.CurrentMinedCount = 0
	mineData.IsCompleted = false

	// 保存数据
	if err := SaveUserData(ctx, s.logger, s.db, s.metrics, s.storageIndex, mineData); err != nil {
		s.logger.Error("保存挖矿数据失败", zap.Error(err))
		return &game.UpgradeMineResponse{
			Code: 4,
			Msg:  "保存数据失败",
		}, nil
	}

	s.logger.Info("矿产升级成功",
		zap.Int32("level", mineData.CurrentLevel))

	return &game.UpgradeMineResponse{
		Code:  0,
		Msg:   "升级成功",
		Level: mineData.CurrentLevel,
	}, nil
}
