package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// 初始化关卡数据
func initializeLevelData(userMeta *game.UserMeta) {
	now := time.Now().Format(time.RFC3339)
	userMeta.Level = &game.LevelData{
		CurLevelId:             ZeroLevelId, // "L1000"
		BestProgress:           0,
		HasMoppingTimes:        0,
		HasMoppingTimesForAdv:  0,
		LastMoppingTimestamp:   now,
		LastGetOnHookTimestamp: now, // 初始化为当前时间
	}
}

// LevelBoxData 关卡宝箱数据存储结构
type LevelBoxData struct {
	ClaimedBoxes map[string][]int32 `json:"claimed_boxes"` // key: level_id, value: 已领取的宝箱ID列表
}

func (d *LevelBoxData) GetCollection() string {
	return "level_box"
}

func (d *LevelBoxData) GetKey() string {
	return "data"
}

func (d *LevelBoxData) Init() {
	d.ClaimedBoxes = make(map[string][]int32)
}

// ClaimLevelBox 领取关卡宝箱奖励
func (s *ApiServer) ClaimLevelBox(ctx context.Context, in *game.ClaimLevelBoxRequest) (*game.ClaimLevelBoxResponse, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 验证参数
	if in.GetLevelId() == "" {
		return &game.ClaimLevelBoxResponse{
			Code: 1,
			Msg:  "关卡ID不能为空",
		}, nil
	}

	if len(in.GetBoxIds()) == 0 {
		return &game.ClaimLevelBoxResponse{
			Code: 2,
			Msg:  "请选择要领取的宝箱",
		}, nil
	}

	// 验证宝箱ID是否有效（1-3）
	for _, boxId := range in.GetBoxIds() {
		if boxId < 1 || boxId > 3 {
			return &game.ClaimLevelBoxResponse{
				Code: 3,
				Msg:  "宝箱ID无效，只能是1、2或3",
			}, nil
		}
	}

	// 检查关卡配置是否存在
	levelInfo, exist := s.template.GetTplLevelInfo().FindByKey(in.GetLevelId())
	if !exist {
		return &game.ClaimLevelBoxResponse{
			Code: 4,
			Msg:  "关卡不存在",
		}, nil
	}

	// 获取用户元数据，检查关卡是否已通过
	userMeta, _, err := GetUserMeta(ctx)
	if err != nil {
		return &game.ClaimLevelBoxResponse{
			Code: 5,
			Msg:  "获取用户数据失败",
		}, nil
	}

	// 初始化关卡数据
	if userMeta.Level == nil {
		initializeLevelData(userMeta)
	}

	// 检查关卡是否已通过（当前关卡ID要小于等于已通过的关卡）
	if !isLevelPassed(in.GetLevelId(), userMeta.Level.CurLevelId) {
		return &game.ClaimLevelBoxResponse{
			Code: 6,
			Msg:  "关卡尚未通过",
		}, nil
	}

	// 加载宝箱数据
	levelBoxData := &LevelBoxData{}
	if err := LoadData(ctx, s.logger, s.db, userID, levelBoxData); err != nil {
		s.logger.Warn("加载宝箱数据失败，初始化新数据", zap.Error(err))
		// 首次加载或数据不存在，初始化
		levelBoxData.Init()
	}

	// 检查哪些宝箱已经领取
	claimedBoxes := levelBoxData.ClaimedBoxes[in.GetLevelId()]
	var toClaimBoxIds []int32
	var alreadyClaimedBoxIds []int32

	for _, boxId := range in.GetBoxIds() {
		if contains(claimedBoxes, boxId) {
			alreadyClaimedBoxIds = append(alreadyClaimedBoxIds, boxId)
		} else {
			toClaimBoxIds = append(toClaimBoxIds, boxId)
		}
	}

	// 如果所有宝箱都已领取
	if len(toClaimBoxIds) == 0 {
		return &game.ClaimLevelBoxResponse{
			Code: 8,
			Msg:  "宝箱已经领取过了",
		}, nil
	}

	// 收集要发放的奖励
	var allRewards []*game.Reward
	var totalWalletUpdate *game.WalletUpdateResult
	var totalInventoryUpdate *game.InventoryUpdateResult

	for _, boxId := range toClaimBoxIds {
		var rewardId string
		switch boxId {
		case 1:
			rewardId = levelInfo.Reward01
		case 2:
			rewardId = levelInfo.Reward02
		case 3:
			rewardId = levelInfo.Reward03
		}

		if rewardId == "" {
			s.logger.Warn("宝箱奖励未配置", zap.String("level_id", in.GetLevelId()), zap.Int32("box_id", boxId))
			continue
		}

		reward := GetReward(rewardId, s.template.GetTplReward(), s.logger)
		if reward != nil {
			allRewards = append(allRewards, reward)

			// 发放奖励
			source := fmt.Sprintf("level_box_%s_%d", in.GetLevelId(), boxId)
			walletResult, inventoryResult, err := GrantReward(ctx, s.logger, s.db, s.template, reward, source)
			if err != nil {
				s.logger.Error("发放宝箱奖励失败", zap.Error(err), zap.String("level_id", in.GetLevelId()), zap.Int32("box_id", boxId))
				return &game.ClaimLevelBoxResponse{
					Code: 9,
					Msg:  "发放奖励失败: " + err.Error(),
				}, nil
			}

			// 合并钱包更新结果
			if walletResult != nil {
				totalWalletUpdate = walletResult
			}

			// 合并背包更新结果
			if inventoryResult != nil {
				if totalInventoryUpdate == nil {
					totalInventoryUpdate = inventoryResult
				} else {
					totalInventoryUpdate.Updated = append(totalInventoryUpdate.Updated, inventoryResult.Updated...)
				}
			}
		}
	}

	// 更新已领取的宝箱列表
	if levelBoxData.ClaimedBoxes[in.GetLevelId()] == nil {
		levelBoxData.ClaimedBoxes[in.GetLevelId()] = toClaimBoxIds
	} else {
		levelBoxData.ClaimedBoxes[in.GetLevelId()] = append(levelBoxData.ClaimedBoxes[in.GetLevelId()], toClaimBoxIds...)
	}

	// 保存宝箱数据
	err = SaveData(ctx, s.logger, s.db, s.metrics, s.storageIndex, userID, levelBoxData)
	if err != nil {
		s.logger.Error("保存宝箱数据失败", zap.Error(err))
		return &game.ClaimLevelBoxResponse{
			Code: 10,
			Msg:  "保存数据失败",
		}, nil
	}

	// 提取更新后的数据
	var walletUpdated *game.Wallet
	var inventoryUpdated []*game.Item

	if totalWalletUpdate != nil {
		walletUpdated = totalWalletUpdate.Updated
	}

	if totalInventoryUpdate != nil {
		inventoryUpdated = totalInventoryUpdate.Updated
	}

	s.logger.Info("宝箱领取成功",
		zap.String("level_id", in.GetLevelId()),
		zap.Int32s("box_ids", toClaimBoxIds))

	return &game.ClaimLevelBoxResponse{
		Code:             0,
		Msg:              "领取成功",
		Rewards:          allRewards,
		WalletUpdated:    walletUpdated,
		InventoryUpdated: inventoryUpdated,
	}, nil
}

// isLevelPassed 检查关卡是否已通过
func isLevelPassed(targetLevelId, currentLevelId string) bool {
	// 将 "L1001" 转换为 1001 进行比较
	targetNum, err1 := parseLevelIdToInt(targetLevelId)
	currentNum, err2 := parseLevelIdToInt(currentLevelId)

	if err1 != nil || err2 != nil {
		return false
	}

	return targetNum <= currentNum
}

// parseLevelIdToInt 解析关卡ID为数字
func parseLevelIdToInt(levelId string) (int32, error) {
	if len(levelId) == 0 || levelId[0] != 'L' {
		return 0, fmt.Errorf("invalid level id format")
	}
	num, err := strconv.ParseInt(levelId[1:], 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(num), nil
}

// contains 检查切片中是否包含指定元素
func contains(slice []int32, item int32) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
