package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"github.com/heroiclabs/nakama/v3/template"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	permissionRead  = int32(1)
	permissionWrite = int32(0)
)

func CreateStorageOpWrite(collection, key, value, ownerID string) *StorageOpWrite {
	return CreateStorageOpWriteWithVersion(collection, key, value, ownerID, "")
}

func CreateStorageOpWriteWithVersion(collection, key, value, ownerID, version string) *StorageOpWrite {
	return &StorageOpWrite{
		Object: &api.WriteStorageObject{
			Collection:      collection,
			Key:             key,
			Value:           value,
			Version:         version,
			PermissionRead:  &wrapperspb.Int32Value{Value: permissionRead},
			PermissionWrite: &wrapperspb.Int32Value{Value: permissionWrite},
		},
		OwnerID: ownerID,
	}
}

// parseTime 解析时间字符串，支持UTC和自定义格式
func parseTime(timeStr string) (time.Time, error) {
	// 尝试解析UTC格式
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}

	// UTC格式解析失败，尝试解析自定义格式
	return time.Parse("2006:01:02:15:04:05", timeStr)
}

// convertMapInt64ToItems 将 map[string]int64 转换为 []*Item
func convertMapInt64ToItems(m map[string]int64) []*game.Item {
	if m == nil {
		return nil
	}
	items := make([]*game.Item, 0, len(m))
	for id, num := range m {
		items = append(items, &game.Item{
			Id:  id,
			Num: int32(num),
		})
	}
	return items
}

// convertMapInt64ToWallet 将 map[string]int64 转换为 *Wallet
func convertMapInt64ToWallet(m map[string]int64) *game.Wallet {
	if m == nil {
		return nil
	}
	return &game.Wallet{
		Coin:    int32(m["coin"]),
		Gem:     int32(m["gem"]),
		Ad:      int32(m["ad"]),
		Stamina: int32(m["stamina"]),
	}
}

// GrantReward 统一的奖励发放函数（处理特殊道具逻辑）
// 用于商店购买、战斗结算、礼包兑换等所有奖励发放场景
func GrantReward(ctx context.Context, logger *zap.Logger, db *sql.DB, templateMgr TemplateManager, metrics Metrics, storageIndex StorageIndex, reward *game.Reward, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	if reward == nil {
		return nil, nil, nil
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	// 1. 处理道具奖励，转换特殊道具
	walletChangeset := make(map[string]int64)
	inventoryChangeset := make(map[string]int64)

	// 初始化钱包奖励
	if reward.Wallet != nil {
		if reward.Wallet.Coin > 0 {
			walletChangeset["coin"] = int64(reward.Wallet.Coin)
		}
		if reward.Wallet.Gem > 0 {
			walletChangeset["gem"] = int64(reward.Wallet.Gem)
		}
		if reward.Wallet.Ad > 0 {
			walletChangeset["ad"] = int64(reward.Wallet.Ad)
		}
		if reward.Wallet.Stamina > 0 {
			walletChangeset["stamina"] = int64(reward.Wallet.Stamina)
		}
	}

	// 处理道具奖励（包含特殊道具转换）
	if len(reward.Items) > 0 {
		convertedItems := make(map[string]int32)
		skipItemIds := make(map[string]bool)
		equipCountModify := make(map[string]int32)

		for _, item := range reward.Items {
			if item == nil || item.Num <= 0 {
				continue
			}

			itemTpl, found := templateMgr.GetTplItem().FindByKey(item.Id)
			if !found {
				logger.Warn("物品模板未找到", zap.String("item_id", item.Id))
				inventoryChangeset[item.Id] += int64(item.Num)
				continue
			}

			switch itemTpl.ItemType {
			case ItemType_Coin:
				walletChangeset["coin"] += int64(item.Num)
				skipItemIds[item.Id] = true
			case ItemType_Gem:
				walletChangeset["gem"] += int64(item.Num)
				skipItemIds[item.Id] = true
			case ItemType_AdTicket:
				walletChangeset["ad"] += int64(item.Num)
				skipItemIds[item.Id] = true
			case ItemType_Stamina:
				walletChangeset["stamina"] += int64(item.Num)
				skipItemIds[item.Id] = true
			case ItemType_RandomElementCrystal:
				crystalItems := templateMgr.GetTplItem().FindByFilter(func(t template.TplItem) bool {
					return t.ItemType == ItemType_ElementCrystal
				})
				if crystalItems.Len() == 0 {
					logger.Warn("未找到元素晶体配置")
					break
				}
				crystalIds := make([]string, 0, crystalItems.Len())
				for i := 0; i < crystalItems.Len(); i++ {
					crystalIds = append(crystalIds, crystalItems.Get(i).ID)
				}
				for i := int32(0); i < item.Num; i++ {
					randomCrystal := crystalIds[rand.Intn(len(crystalIds))]
					inventoryChangeset[randomCrystal]++
					convertedItems[randomCrystal]++
				}
				skipItemIds[item.Id] = true
			case ItemType_RandomQualityTurret:
				itemTpl, _ := templateMgr.GetTplItem().FindByKey(item.Id)
				quality := itemTpl.Quality

				distributedItems, err := distributeRandomQualityTurret(ctx, logger, db, templateMgr, metrics, storageIndex, item.Num, quality)
				if err != nil {
					logger.Error("分配随机品质炮台失败", zap.Error(err))
				} else {
					for itemId, count := range distributedItems {
						inventoryChangeset[itemId] += count
						convertedItems[itemId] += int32(count)
					}
				}
				skipItemIds[item.Id] = true
			case ItemType_RandomTurretFragment:
				distributedDebris, err := distributeTurretDebris(ctx, logger, db, templateMgr, metrics, storageIndex, item.Num, false)
				if err != nil {
					logger.Error("分配炮台碎片失败", zap.Error(err))
				} else {
					for debrisId, count := range distributedDebris {
						inventoryChangeset[debrisId] += count
						convertedItems[debrisId] += int32(count)
					}
				}
				skipItemIds[item.Id] = true
			case ItemType_RandomCrystalEquipment:
				quality := itemTpl.Quality
				level := itemTpl.Level
				err := generateRandomCrystalEquipment(ctx, logger, db, templateMgr, metrics, storageIndex, item.Num, quality, level)
				if err != nil {
					logger.Error("生成随机水晶装备失败", zap.Error(err))
				}
				skipItemIds[item.Id] = true
			case ItemType_Turret:
				logger.Info("尝试解锁炮台", zap.String("equip_id", item.Id), zap.Int32("num", item.Num))
				alreadyUnlocked, err := tryUnlockEquipment(ctx, logger, db, metrics, storageIndex, item.Id)
				if err != nil {
					logger.Error("解锁炮台失败",
						zap.Error(err),
						zap.String("equip_id", item.Id))
					inventoryChangeset[item.Id] += int64(item.Num)
				} else if alreadyUnlocked {
					debrisId := strings.TrimPrefix(item.Id, "EQ")
					debrisCount := item.Num * 10
					inventoryChangeset[debrisId] += int64(debrisCount)
					convertedItems[debrisId] += debrisCount
					skipItemIds[item.Id] = true

					logger.Info("炮台已解锁，转换为碎片",
						zap.String("equip_id", item.Id),
						zap.String("debris_id", debrisId),
						zap.Int32("debris_count", debrisCount))
				} else {
					if item.Num == 1 {
						logger.Info("成功解锁新炮台",
							zap.String("equip_id", item.Id),
							zap.Int32("equip_count", 1))
					} else {
						remainingCount := item.Num - 1
						debrisId := strings.TrimPrefix(item.Id, "EQ")
						debrisCount := remainingCount * 10
						inventoryChangeset[debrisId] += int64(debrisCount)
						convertedItems[debrisId] += debrisCount
						equipCountModify[item.Id] = 1

						logger.Info("成功解锁新炮台，部分转换为碎片",
							zap.String("equip_id", item.Id),
							zap.Int32("equip_count", 1),
							zap.Int32("remaining_count", remainingCount),
							zap.String("debris_id", debrisId),
							zap.Int32("debris_count", debrisCount))
					}
				}
			default:
				inventoryChangeset[item.Id] += int64(item.Num)
			}
		}

		if len(convertedItems) > 0 || len(skipItemIds) > 0 {
			newItems := make([]*game.Item, 0, len(reward.Items)+len(convertedItems))

			for _, item := range reward.Items {
				if item == nil {
					continue
				}
				if skipItemIds[item.Id] {
					if newCount, needModify := equipCountModify[item.Id]; needModify {
						newItems = append(newItems, &game.Item{
							Id:  item.Id,
							Num: newCount,
						})
						logger.Info("修改装备数量", zap.String("equip_id", item.Id), zap.Int32("old_count", item.Num), zap.Int32("new_count", newCount))
					}
					continue
				}
				newItems = append(newItems, item)
			}

			for itemId, count := range convertedItems {
				if count > 0 {
					newItems = append(newItems, &game.Item{
						Id:  itemId,
						Num: count,
					})
				}
			}
			reward.Items = newItems
		}
	}

	// 2. 发放钱包奖励
	var walletUpdateResult *game.WalletUpdateResult
	if len(walletChangeset) > 0 {
		metadata, _ := json.Marshal(map[string]interface{}{
			"source": source,
		})

		walletUpdates := []*walletUpdate{{
			UserID:    userID,
			Changeset: walletChangeset,
			Metadata:  string(metadata),
		}}

		results, err := UpdateWallets(ctx, logger, db, walletUpdates, true)
		if err != nil {
			logger.Error("发放钱包奖励失败", zap.Error(err))
			return nil, nil, err
		}

		if len(results) > 0 {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 3. 发放道具奖励
	var inventoryUpdateResult *game.InventoryUpdateResult
	if len(inventoryChangeset) > 0 {
		metadata, _ := json.Marshal(map[string]interface{}{
			"source": source,
		})

		inventoryUpdates := []*inventoryUpdate{{
			UserID:    userID,
			Changeset: inventoryChangeset,
			Metadata:  string(metadata),
		}}

		results, err := UpdateInventories(ctx, logger, db, inventoryUpdates, true)
		if err != nil {
			logger.Error("发放道具奖励失败", zap.Error(err))
			return nil, nil, err
		}

		if len(results) > 0 {
			inventoryUpdateResult = &game.InventoryUpdateResult{
				Previous: convertMapInt64ToItems(results[0].Previous),
				Updated:  convertMapInt64ToItems(results[0].Updated),
			}
		}
	}

	return walletUpdateResult, inventoryUpdateResult, nil
}

// tryUnlockEquipment 尝试解锁炮台装备
// 返回值：alreadyUnlocked（是否已解锁）, error（错误）
func tryUnlockEquipment(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, equipID string) (bool, error) {

	// 加载装备数据
	equipData := &EquipData{}
	if err := LoadUserData(ctx, logger, db, equipData); err != nil {
		logger.Error("加载装备数据失败", zap.Error(err))
		return false, fmt.Errorf("加载装备数据失败")
	}

	// 检查是否已解锁
	if _, exists := equipData.UnlockEquips[equipID]; exists {
		logger.Debug("炮台已解锁",
			zap.String("equip_id", equipID))
		return true, nil
	}

	// 解锁炮台，初始等级为1
	equipData.UnlockEquips[equipID] = 1

	// 保存装备数据
	if err := SaveUserData(ctx, logger, db, metrics, storageIndex, equipData); err != nil {
		logger.Error("保存装备数据失败",
			zap.Error(err),
			zap.String("equip_id", equipID))
		return false, fmt.Errorf("保存装备数据失败")
	}

	logger.Info("解锁新炮台",
		zap.String("equip_id", equipID))

	return false, nil
}

// distributeRandomQualityTurret 根据品质随机炮台，如果已解锁则转换为碎片
func distributeRandomQualityTurret(ctx context.Context, logger *zap.Logger, db *sql.DB, templateMgr TemplateManager, metrics Metrics, storageIndex StorageIndex, count int32, quality int32) (map[string]int64, error) {
	result := make(map[string]int64)

	equipments := templateMgr.GetTplEquipment().FindByFilter(func(t template.TplEquipment) bool {
		return t.Quality == quality && t.Type == 1 && t.ID != EquipID_Crystal
	})

	if equipments.Len() == 0 {
		logger.Warn("未找到指定品质的炮台", zap.Int32("quality", quality))
		return result, nil
	}

	equipData := &EquipData{}
	if err := LoadUserData(ctx, logger, db, equipData); err != nil {
		logger.Error("加载装备数据失败", zap.Error(err))
		return nil, err
	}

	for i := int32(0); i < count; i++ {
		randomIdx := rand.Intn(equipments.Len())
		selectedEquip := equipments.Get(randomIdx)
		equipID := selectedEquip.ID

		_, alreadyUnlocked := equipData.UnlockEquips[equipID]

		if alreadyUnlocked {
			debrisId := strings.TrimPrefix(equipID, "EQ")
			debrisCount := int64(TurretDebrisCount)
			result[debrisId] += debrisCount

			logger.Info("炮台已解锁，转换为碎片",
				zap.String("equip_id", equipID),
				zap.String("debris_id", debrisId),
				zap.Int64("debris_count", debrisCount))
		} else {
			alreadyUnlocked, err := tryUnlockEquipment(ctx, logger, db, metrics, storageIndex, equipID)
			if err != nil {
				logger.Error("解锁炮台失败", zap.Error(err), zap.String("equip_id", equipID))
				return nil, err
			}

			if alreadyUnlocked {
				debrisId := strings.TrimPrefix(equipID, "EQ")
				debrisCount := int64(TurretDebrisCount)
				result[debrisId] += debrisCount
			} else {
				equipData.UnlockEquips[equipID] = 1
				logger.Info("解锁新炮台", zap.String("equip_id", equipID), zap.Int32("quality", quality))
			}
		}
	}

	return result, nil
}

// distributeTurretDebris 根据炮台等级概率分配炮台碎片
func distributeTurretDebris(ctx context.Context, logger *zap.Logger, db *sql.DB, templateMgr TemplateManager, metrics Metrics, storageIndex StorageIndex, totalDebrisCount int32, singleTurret bool) (map[string]int64, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	logger.Debug("开始分配炮台碎片",
		zap.String("user_id", userID.String()),
		zap.Int32("total_debris_count", totalDebrisCount),
		zap.Bool("single_turret", singleTurret))

	equipData := &EquipData{}
	if err := LoadUserData(ctx, logger, db, equipData); err != nil {
		logger.Error("加载装备数据失败", zap.Error(err))
		return nil, err
	}

	if len(equipData.UnlockEquips) == 0 {
		logger.Debug("装备数据为空，无法分配碎片")
		return make(map[string]int64), nil
	}

	logger.Debug("玩家已解锁炮台数量",
		zap.Int("unlocked_count", len(equipData.UnlockEquips)))

	inventory, err := GetInventory(ctx, logger, db, userID)
	if err != nil {
		logger.Error("获取背包数据失败", zap.Error(err))
		return nil, err
	}

	debrisItems := templateMgr.GetTplItem().FindByFilter(func(t template.TplItem) bool {
		return t.ItemType == ItemType_EquipmentFragment
	})

	if debrisItems.Len() == 0 {
		logger.Warn("未找到装备碎片配置")
		return make(map[string]int64), nil
	}

	type TurretInfo struct {
		DebrisID     string
		EquipID      string
		Level        int32
		TotalDebris  int32
		DebrisWeight int32
	}

	turrets := make([]TurretInfo, 0)

	for i := 0; i < debrisItems.Len(); i++ {
		debrisItem := debrisItems.Get(i)
		debrisId := debrisItem.ID
		equipID := "EQ" + debrisId

		level, unlocked := equipData.UnlockEquips[equipID]
		if !unlocked {
			continue
		}

		if equipID == "EQ1999" {
			continue
		}

		consumedDebris := int32(0)
		for lv := int32(1); lv < level; lv++ {
			equipDevs := templateMgr.GetTplEquipmentDev().FindByFilter(func(equip template.TplEquipmentDev) bool {
				return equip.EquipID == equipID && equip.Level == lv+1
			})
			if equipDevs.Len() > 0 {
				consumedDebris += equipDevs.Get(0).CostDebris
			}
		}

		inventoryDebris := int32(0)
		if count, ok := inventory[debrisId]; ok {
			inventoryDebris = int32(count)
		}
		// 总碎片数 = 已消耗 + 背包持有
		totalDebris := consumedDebris + inventoryDebris
		// 权重计算：碎片越少权重越高（使用反比例）
		// 基础权重 = 10000，权重 = 基础权重 / (totalDebris + 1)		weight := int32(10000) / (totalDebris + 1)
		weight := int32(10000) / (totalDebris + 1)
		turrets = append(turrets, TurretInfo{
			DebrisID:     debrisId,
			EquipID:      equipID,
			Level:        level,
			TotalDebris:  totalDebris,
			DebrisWeight: weight,
		})
	}

	if len(turrets) == 0 {
		logger.Warn("没有已解锁的炮台，无法分配碎片", zap.String("user_id", userID.String()))
		return make(map[string]int64), nil
	}

	totalWeight := int32(0)
	for _, t := range turrets {
		totalWeight += t.DebrisWeight
	}

	result := make(map[string]int64)

	if singleTurret {
		// 50000 模式：只随机选择一个炮台，给全部碎片
		randomValue := rand.Int31n(totalWeight)
		currentWeight := int32(0)

		for _, t := range turrets {
			currentWeight += t.DebrisWeight
			if randomValue < currentWeight {
				result[t.DebrisID] = int64(totalDebrisCount) * int64(TurretDebrisCount)

				logger.Info("随机选择炮台（随机品质炮台模式）",
					zap.String("user_id", userID.String()),
					zap.String("selected_equip_id", t.EquipID),
					zap.String("debris_id", t.DebrisID),
					zap.Int32("debris_count", totalDebrisCount),
					zap.Int32("random_value", randomValue),
					zap.Int32("weight", t.DebrisWeight))
				break
			}
		}

		if len(result) == 0 {
			logger.Error("随机抽取炮台失败：未找到匹配项",
				zap.Int32("random_value", randomValue),
				zap.Int32("total_weight", totalWeight))
		}
	} else {
		// 90000 模式：每个碎片单独随机分配
		for i := int32(0); i < totalDebrisCount; i++ {
			randomValue := rand.Int31n(totalWeight)
			currentWeight := int32(0)

			selectedEquipID := ""
			for _, t := range turrets {
				currentWeight += t.DebrisWeight
				if randomValue < currentWeight {
					result[t.DebrisID]++
					selectedEquipID = t.EquipID

					logger.Debug("随机抽取炮台碎片",
						zap.Int32("round", i+1),
						zap.Int32("random_value", randomValue),
						zap.String("selected_equip_id", selectedEquipID),
						zap.String("debris_id", t.DebrisID),
						zap.Int64("current_total", result[t.DebrisID]))
					break
				}
			}

			if selectedEquipID == "" {
				logger.Error("随机抽取失败：未找到匹配炮台",
					zap.Int32("round", i+1),
					zap.Int32("random_value", randomValue),
					zap.Int32("total_weight", totalWeight))
			}
		}
	}

	logger.Debug("炮台碎片分配结果",
		zap.String("user_id", userID.String()),
		zap.Bool("single_turret", singleTurret),
		zap.Any("result", result))

	return result, nil
}

// containsString 检查字符串切片中是否包含指定元素
func containsString(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func GetReward(rewardId string, tplReward *template.TableTplReward, logger *zap.Logger) *game.Reward {
	reward, exist := tplReward.FindByKey(rewardId)
	if !exist {
		return nil
	}

	items := parseItems(reward.Items, logger)

	return &game.Reward{
		Wallet: &game.Wallet{
			Coin: reward.Coin,
			Gem:  reward.Gem,
			Ad:   reward.Coupon,
		},
		Items: items,
	}
}

func parseItems(itemsString string, logger *zap.Logger) []*game.Item {
	if itemsString == "" {
		return nil
	}

	pairs := strings.Split(itemsString, ",")
	result := make([]*game.Item, 0, len(pairs))

	for _, pair := range pairs {
		trimmedPair := strings.TrimSpace(pair)
		if trimmedPair == "" {
			continue
		}

		keyValue := strings.Split(trimmedPair, "_")
		if len(keyValue) != 2 {
			if logger != nil {
				logger.Error("Invalid key-value pair", zap.String("pair", trimmedPair))
			}
			continue
		}

		id := strings.TrimSpace(keyValue[0])
		numStr := strings.TrimSpace(keyValue[1])

		num, err := strconv.ParseInt(numStr, 10, 32)
		if err != nil {
			if logger != nil {
				logger.Error("Warning: Invalid key-value pair", zap.String("pair", trimmedPair), zap.Error(err))
			}
			continue
		}

		result = append(result, &game.Item{
			Id:  id,
			Num: int32(num),
		})
	}

	return result
}

// MergeRewards 合并多个奖励为一个奖励
// 将 wallet 的值相加，items 数组整合（相同 Id 的 Num 相加）
func MergeRewards(rewards []*game.Reward) *game.Reward {
	if len(rewards) == 0 {
		return nil
	}

	mergedWallet := &game.Wallet{}
	itemMap := make(map[string]int32)

	for _, reward := range rewards {
		if reward == nil {
			continue
		}

		// 合并 wallet
		if reward.Wallet != nil {
			mergedWallet.Coin += reward.Wallet.Coin
			mergedWallet.Gem += reward.Wallet.Gem
			mergedWallet.Ad += reward.Wallet.Ad
			mergedWallet.Stamina += reward.Wallet.Stamina
		}

		// 合并 items
		if len(reward.Items) > 0 {
			for _, item := range reward.Items {
				if item != nil && item.Id != "" && item.Num > 0 {
					itemMap[item.Id] += item.Num
				}
			}
		}
	}

	// 检查是否有有效的 wallet 值
	hasWallet := mergedWallet.Coin > 0 || mergedWallet.Gem > 0 || mergedWallet.Ad > 0 || mergedWallet.Stamina > 0

	// 构建合并后的 items 数组
	var mergedItems []*game.Item
	if len(itemMap) > 0 {
		mergedItems = make([]*game.Item, 0, len(itemMap))
		for id, num := range itemMap {
			if num > 0 {
				mergedItems = append(mergedItems, &game.Item{
					Id:  id,
					Num: num,
				})
			}
		}
	}

	// 如果既没有 wallet 也没有 items，返回 nil
	if !hasWallet && len(mergedItems) == 0 {
		return nil
	}

	result := &game.Reward{}
	if hasWallet {
		result.Wallet = mergedWallet
	}
	if len(mergedItems) > 0 {
		result.Items = mergedItems
	}

	return result
}

// GrantMergedRewards 通用奖励发放函数
// 接收多个已处理的 game.Reward，合并后一次性发放
// rewards: 奖励列表（调用方已完成GetReward转换、进度折扣等业务逻辑处理）
// source: 奖励来源标识
// 返回: 合并后的奖励、钱包更新结果、背包更新结果、错误信息
func GrantMergedRewards(ctx context.Context, logger *zap.Logger, db *sql.DB, template TemplateManager, metrics Metrics, storageIndex StorageIndex, rewards []*game.Reward, source string) (*game.Reward, *game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	if len(rewards) == 0 {
		return nil, nil, nil, nil
	}

	// 合并所有奖励
	mergedReward := MergeRewards(rewards)
	if mergedReward == nil {
		return nil, nil, nil, nil
	}

	// 发放合并后的奖励
	walletUpdateResult, inventoryUpdateResult, err := GrantReward(ctx, logger, db, template, metrics, storageIndex, mergedReward, source)
	if err != nil {
		return nil, nil, nil, err
	}

	return mergedReward, walletUpdateResult, inventoryUpdateResult, nil
}

// ConsumeCostItems 通用的消耗道具函数
// 解析 costItem 字符串（格式："itemId_count,itemId_count"），根据道具类型从钱包或背包中扣除
// source: 消耗来源标识（如 "crystal_slot_upgrade", "equip_upgrade" 等）
// 返回：钱包更新结果、背包更新结果、错误信息
func ConsumeCostItems(ctx context.Context, logger *zap.Logger, db *sql.DB, template TemplateManager, costItemStr string, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
	if costItemStr == "" {
		return nil, nil, nil
	}

	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	walletChangeset := make(map[string]int64)
	inventoryChangeset := make(map[string]int64)

	items := strings.Split(costItemStr, ",")
	for _, item := range items {
		parts := strings.Split(strings.TrimSpace(item), "_")
		if len(parts) != 2 {
			logger.Warn("无效的 costItem 格式", zap.String("item", item))
			continue
		}

		itemID := strings.TrimSpace(parts[0])
		count, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || count <= 0 {
			logger.Warn("无效的道具数量", zap.String("item", item), zap.Error(err))
			continue
		}

		itemTpl, found := template.GetTplItem().FindByKey(itemID)
		if !found {
			logger.Warn("物品模板未找到", zap.String("item_id", itemID))
			inventoryChangeset[itemID] -= count
			continue
		}

		switch itemTpl.ItemType {
		case ItemType_Coin:
			walletChangeset["coin"] -= count
		case ItemType_Gem:
			walletChangeset["gem"] -= count
		case ItemType_Stamina:
			walletChangeset["stamina"] -= count
		case ItemType_AdTicket:
			walletChangeset["ad"] -= count
		default:
			inventoryChangeset[itemID] -= count
		}
	}

	metadata, _ := json.Marshal(map[string]interface{}{
		"source": source,
	})
	metadataStr := string(metadata)

	var walletUpdateResult *game.WalletUpdateResult
	var inventoryUpdateResult *game.InventoryUpdateResult

	// 扣除钱包资源
	if len(walletChangeset) > 0 {
		results, err := UpdateWallets(ctx, logger, db, []*walletUpdate{
			{
				UserID:    userID,
				Changeset: walletChangeset,
				Metadata:  metadataStr,
			},
		}, true)
		if err != nil {
			logger.Error("扣除钱包资源失败", zap.Error(err))
			return nil, nil, err
		}

		if len(results) > 0 && results[0] != nil {
			walletUpdateResult = &game.WalletUpdateResult{
				Previous: convertMapInt64ToWallet(results[0].Previous),
				Updated:  convertMapInt64ToWallet(results[0].Updated),
			}
		}
	}

	// 扣除背包道具
	if len(inventoryChangeset) > 0 {
		results, err := UpdateInventories(ctx, logger, db, []*inventoryUpdate{
			{
				UserID:    userID,
				Changeset: inventoryChangeset,
				Metadata:  metadataStr,
			},
		}, true)
		if err != nil {
			logger.Error("扣除背包道具失败", zap.Error(err))
			// 如果背包扣除失败且已经扣除了钱包，需要回滚钱包
			if len(walletChangeset) > 0 {
				rollbackMetadata, _ := json.Marshal(map[string]interface{}{
					"source": source + "_rollback",
				})
				rollbackChangeset := make(map[string]int64)
				for key, value := range walletChangeset {
					rollbackChangeset[key] = -value
				}
				_, rollbackErr := UpdateWallets(ctx, logger, db, []*walletUpdate{
					{
						UserID:    userID,
						Changeset: rollbackChangeset,
						Metadata:  string(rollbackMetadata),
					},
				}, true)
				if rollbackErr != nil {
					logger.Error("回滚钱包失败", zap.Error(rollbackErr))
				}
			}
			return nil, nil, err
		}

		if len(results) > 0 && results[0] != nil {
			inventoryUpdateResult = &game.InventoryUpdateResult{
				Previous: convertMapInt64ToItems(results[0].Previous),
				Updated:  convertMapInt64ToItems(results[0].Updated),
			}
		}
	}

	return walletUpdateResult, inventoryUpdateResult, nil
}

// generateRandomCrystalEquipment 生成随机水晶装备
//
// 算法流程：
// 1. 加载玩家现有的水晶装备数据
// 2. 根据品质(quality)从TplCrystalEquipment表中过滤可用的装备模板
// 3. 根据品质和等级(level)从TplCrystalEquipmentRefinement表中获取词条配置
//   - 精确匹配品质和等级，选择对应的配置
//     4. 循环生成count个装备：
//     a. 随机选择一个装备模板
//     b. 生成UUID作为装备的唯一实例ID
//     c. 根据配置生成词条（使用位运算判断品质和类型，使用maxRatio计算数值上限）
//     d. 将新装备添加到玩家的装备合集中
//     5. 保存更新后的装备数据到storage
//
// 品质和类型使用位运算判断：
//   - 品质：1=白色, 2=绿色, 4=蓝色, 8=紫色(史诗), 16=橙色(传说)
//   - 类型：1=共鸣核心, 2=加固模组, 4=增幅器, 8=传导矩阵, 15=通用(1|2|4|8)
//   - 判断逻辑：(装备品质 & 词条需求品质) != 0 表示品质满足要求
//
// 参数：
//   - count: 生成的装备数量
//   - quality: 装备品质（单一品质值，如8表示紫色）
//   - level: 装备等级（用于选择词条配置）
func generateRandomCrystalEquipment(ctx context.Context, logger *zap.Logger, db *sql.DB, templateMgr TemplateManager, metrics Metrics, storageIndex StorageIndex, count int32, quality int32, level int32) error {
	crystalEquipmentsData := &CrystalEquipmentData{}
	if err := LoadUserData(ctx, logger, db, crystalEquipmentsData); err != nil {
		logger.Error("加载水晶装备数据失败", zap.Error(err))
		return err
	}

	crystalEquipments := templateMgr.GetTplCrystalEquipment().FindByFilter(func(t template.TplCrystalEquipment) bool {
		return t.Quality == quality
	})

	if crystalEquipments.Len() == 0 {
		logger.Warn("未找到指定品质的水晶装备", zap.Int32("quality", quality))
		return nil
	}

	refinementConfigs := templateMgr.GetTplCrystalEquipmentRefinement().FindByFilter(func(t template.TplCrystalEquipmentRefinement) bool {
		return t.Quality == quality && t.Level == level
	})

	var hasRefinementConfig bool
	var refinementConfig template.TplCrystalEquipmentRefinement

	if refinementConfigs.Len() == 0 {
		logger.Warn("未找到指定品质和等级的词条配置，将生成无词条装备", zap.Int32("quality", quality), zap.Int32("level", level))
		hasRefinementConfig = false
	} else {
		refinementConfig = refinementConfigs.Get(0)
		hasRefinementConfig = true
	}

	for i := int32(0); i < count; i++ {
		randomIdx := rand.Intn(crystalEquipments.Len())
		selectedEquip := crystalEquipments.Get(randomIdx)

		equipID, err := uuid.NewV4()
		if err != nil {
			logger.Error("生成UUID失败", zap.Error(err))
			continue
		}

		var affixes []CrystalEquipmentAffix
		if hasRefinementConfig {
			affixes = generateAffixes(templateMgr, selectedEquip.EquipType, quality, refinementConfig, false, logger)
		} else {
			affixes = make([]CrystalEquipmentAffix, 0)
		}

		crystalEquipmentsData.Equipments[equipID.String()] = &CrystalEquipment{
			TplId:          selectedEquip.ID,
			ID:             equipID.String(),
			ActiveAffixes:  affixes,
			PendingAffixes: make([]CrystalEquipmentAffix, 0),
			LockedAffixIds: make([]string, 0),
		}

		logger.Info("生成随机水晶装备",
			zap.String("equip_id", equipID.String()),
			zap.String("type_id", selectedEquip.ID),
			zap.Int32("quality", quality),
			zap.Int("affix_count", len(affixes)))
	}

	if err := SaveUserData(ctx, logger, db, metrics, storageIndex, crystalEquipmentsData); err != nil {
		logger.Error("保存水晶装备数据失败", zap.Error(err))
		return err
	}

	return nil
}

// generateAffixes 生成装备词条
//
// 算法流程：
// 1. 从TplCrystalEquipmentRefinementAffix表中过滤符合条件的词条：
//   - 使用位运算判断类型：(装备类型 & 词条需求类型) != 0
//   - 使用位运算判断品质：(装备品质 & 词条需求品质) != 0
//     例如：词条needQuality=24(二进制11000)表示支持8(紫色)和16(橙色)品质
//     装备quality=8时，8 & 24 = 8 != 0，满足条件
//
// 2. 将词条分为两类：
//   - 普通词条（quality=0）：基础属性词条
//   - 稀有词条（quality=1）：高级属性词条
//
// 3. 根据refinementConfig配置生成词条：
//   - 普通词条数量 = entryNum
//   - 稀有词条数量 = allowRare为true时根据概率生成，false时为0
//
// 4. 使用权重随机算法选择词条，确保不重复
// 5. 为每个词条生成随机数值：
//   - 数值范围：minValue 到 (maxValue * maxRatio / 100)
//   - maxRatio控制实际最大值的百分比
//
// 参数：
//   - equipType: 装备类型（1=共鸣核心, 2=加固模组, 4=增幅器, 8=传导矩阵）
//   - quality: 装备品质（1=白色, 2=绿色, 4=蓝色, 8=紫色, 16=橙色）
//   - refinementConfig: 词条配置（包含entryNum、rareEntryNum、maxRatio等）
//   - allowRare: 是否允许生成稀有词条（false=只生成普通词条，true=可根据概率生成稀有词条）
func generateAffixes(templateMgr TemplateManager, equipType int32, quality int32, refinementConfig template.TplCrystalEquipmentRefinement, allowRare bool, logger *zap.Logger) []CrystalEquipmentAffix {
	affixes := make([]CrystalEquipmentAffix, 0)

	allAffixes := templateMgr.GetTplCrystalEquipmentRefinementAffix().FindByFilter(func(t template.TplCrystalEquipmentRefinementAffix) bool {
		equipTypeMatch := (equipType & t.NeedEequipType) != 0
		qualityMatch := (quality & t.NeedQuality) != 0
		return equipTypeMatch && qualityMatch
	})

	if allAffixes.Len() == 0 {
		logger.Warn("未找到符合条件的词条", zap.Int32("equip_type", equipType), zap.Int32("quality", quality))
		return affixes
	}

	normalAffixes := make([]template.TplCrystalEquipmentRefinementAffix, 0)
	rareAffixes := make([]template.TplCrystalEquipmentRefinementAffix, 0)

	for i := 0; i < allAffixes.Len(); i++ {
		affix := allAffixes.Get(i)
		if affix.Quality == 0 {
			normalAffixes = append(normalAffixes, affix)
		} else if affix.Quality == 1 {
			rareAffixes = append(rareAffixes, affix)
		}
	}

	normalCount := refinementConfig.EntryNum
	maxRatio := refinementConfig.MaxRatio

	var rareCount int32
	if allowRare {
		rareCount = calculateRareAffixCount(refinementConfig)
	} else {
		rareCount = 0
	}

	usedNormalIDs := make(map[string]bool)
	for i := int32(0); i < normalCount && len(usedNormalIDs) < len(normalAffixes); i++ {
		selected := selectAffixByWeightExcluding(normalAffixes, usedNormalIDs)
		if selected != nil {
			maxVal := selected.MaxValue * maxRatio / 100
			if maxVal < selected.MinValue {
				maxVal = selected.MinValue
			}
			valueRange := maxVal - selected.MinValue + 1
			value := selected.MinValue
			if valueRange > 0 {
				value = rand.Int31n(valueRange) + selected.MinValue
			}
			affixes = append(affixes, CrystalEquipmentAffix{
				AffixID: selected.ID,
				Value:   value,
			})
			usedNormalIDs[selected.ID] = true

			logger.Debug("生成普通词条",
				zap.String("affix_id", selected.ID),
				zap.Int32("min_value", selected.MinValue),
				zap.Int32("max_value", maxVal),
				zap.Int32("random_value", value),
				zap.Int32("max_ratio", maxRatio))
		}
	}

	usedRareIDs := make(map[string]bool)
	for i := int32(0); i < rareCount && len(usedRareIDs) < len(rareAffixes); i++ {
		selected := selectAffixByWeightExcluding(rareAffixes, usedRareIDs)
		if selected != nil {
			maxVal := selected.MaxValue * maxRatio / 100
			if maxVal < selected.MinValue {
				maxVal = selected.MinValue
			}
			valueRange := maxVal - selected.MinValue + 1
			value := selected.MinValue
			if valueRange > 0 {
				value = rand.Int31n(valueRange) + selected.MinValue
			}
			affixes = append(affixes, CrystalEquipmentAffix{
				AffixID: selected.ID,
				Value:   value,
			})
			usedRareIDs[selected.ID] = true

			logger.Debug("生成稀有词条",
				zap.String("affix_id", selected.ID),
				zap.Int32("min_value", selected.MinValue),
				zap.Int32("max_value", maxVal),
				zap.Int32("random_value", value),
				zap.Int32("max_ratio", maxRatio))
		}
	}

	return affixes
}

// calculateRareAffixCount 根据配置计算稀有词条数量
// 使用rareEntryWeight权重来决定生成的稀有词条数量
// 权重越高，生成稀有词条的概率越大
func calculateRareAffixCount(config template.TplCrystalEquipmentRefinement) int32 {
	if config.RareEntryNum <= 0 {
		return 0
	}

	totalWeight := int32(10000)
	randomValue := rand.Int31n(totalWeight)

	if randomValue < config.RareEntryWeight {
		return rand.Int31n(config.RareEntryNum) + 1
	}

	return 0
}

// generateAffixesWithLockedRare 生成装备词条（考虑锁定的稀有词条）
//
// 算法流程：
// 1. 生成所有普通词条（数量 = entryNum，每次洗炼都重新生成）
// 2. 将锁定的稀有词条直接添加到结果中
// 3. 计算剩余稀有词条配额（rareEntryNum - 锁定的稀有数）
// 4. 根据概率判断是否生成新的稀有词条来填充剩余配额
//
// 示例：
//
//	配置：entryNum=4, rareEntryNum=2, rareEntryWeight=5000
//	锁定：1个稀有词条
//	生成：
//	  - 4个普通词条（全部重新生成）
//	  - 1个锁定的稀有词条（直接保留）
//	  - 剩余配额1个，根据概率判断是否生成新稀有词条
//
// 参数：
//   - equipType: 装备类型
//   - quality: 装备品质
//   - refinementConfig: 词条配置（entryNum=普通词条数, rareEntryNum=稀有词条上限）
//   - lockedRareAffixes: 锁定的稀有词条列表
func generateAffixesWithLockedRare(templateMgr TemplateManager, equipType int32, quality int32, refinementConfig template.TplCrystalEquipmentRefinement, lockedRareAffixes []CrystalEquipmentAffix, logger *zap.Logger) []CrystalEquipmentAffix {
	affixes := make([]CrystalEquipmentAffix, 0)

	allAffixes := templateMgr.GetTplCrystalEquipmentRefinementAffix().FindByFilter(func(t template.TplCrystalEquipmentRefinementAffix) bool {
		equipTypeMatch := (equipType & t.NeedEequipType) != 0
		qualityMatch := (quality & t.NeedQuality) != 0
		return equipTypeMatch && qualityMatch
	})

	if allAffixes.Len() == 0 {
		logger.Warn("未找到符合条件的词条", zap.Int32("equip_type", equipType), zap.Int32("quality", quality))
		return affixes
	}

	normalAffixes := make([]template.TplCrystalEquipmentRefinementAffix, 0)
	rareAffixes := make([]template.TplCrystalEquipmentRefinementAffix, 0)

	for i := 0; i < allAffixes.Len(); i++ {
		affix := allAffixes.Get(i)
		if affix.Quality == 0 {
			normalAffixes = append(normalAffixes, affix)
		} else if affix.Quality == 1 {
			rareAffixes = append(rareAffixes, affix)
		}
	}

	// ===== 计算需要生成的词条数量 =====
	//
	// 1. 普通词条：固定生成 entryNum 个（不受锁定影响，每次洗炼都重新生成）
	normalCount := refinementConfig.EntryNum
	maxRatio := refinementConfig.MaxRatio

	// 2. 稀有词条：
	//    a) 先添加锁定的稀有词条
	//    b) 计算剩余配额 = rareEntryNum - 锁定的稀有数
	//    c) 根据概率判断是否生成新稀有词条来填充剩余配额
	lockedRareCount := int32(len(lockedRareAffixes))
	remainingRareSlots := refinementConfig.RareEntryNum - lockedRareCount

	logger.Info("稀有词条洗炼配置",
		zap.Int32("rare_entry_num", refinementConfig.RareEntryNum),
		zap.Int32("locked_rare_count", lockedRareCount),
		zap.Int32("remaining_slots", remainingRareSlots),
		zap.Int32("rare_entry_weight", refinementConfig.RareEntryWeight),
		zap.Float64("probability_percent", float64(refinementConfig.RareEntryWeight)/100.0))

	// 每个剩余的稀有词条槽位独立判断概率
	// 例如：剩余2个槽位，每个都有20%概率，可能生成0个、1个或2个稀有词条
	newRareCount := int32(0)
	totalWeight := int32(100)

	if remainingRareSlots > 0 {
		logger.Info("开始独立判断每个稀有词条槽位",
			zap.Int32("remaining_slots", remainingRareSlots),
			zap.Int32("weight_per_slot", refinementConfig.RareEntryWeight),
			zap.Float64("probability_per_slot_percent", float64(refinementConfig.RareEntryWeight)/100.0))

		// 对每个剩余槽位独立进行概率判断
		for slot := int32(0); slot < remainingRareSlots; slot++ {
			randomValue := rand.Int31n(totalWeight)
			triggered := randomValue < refinementConfig.RareEntryWeight

			if triggered {
				newRareCount++
				logger.Info("稀有词条槽位触发成功",
					zap.Int32("slot_index", slot+1),
					zap.Int32("random_value", randomValue),
					zap.Int32("threshold", refinementConfig.RareEntryWeight),
					zap.Bool("triggered", true))
			} else {
				logger.Info("稀有词条槽位未触发",
					zap.Int32("slot_index", slot+1),
					zap.Int32("random_value", randomValue),
					zap.Int32("threshold", refinementConfig.RareEntryWeight),
					zap.Bool("triggered", false))
			}
		}

		logger.Info("稀有词条槽位判断完成",
			zap.Int32("total_slots", remainingRareSlots),
			zap.Int32("triggered_slots", newRareCount),
			zap.Int32("failed_slots", remainingRareSlots-newRareCount))
	} else {
		logger.Info("稀有词条配额已满，跳过概率判断",
			zap.Int32("locked_rare_count", lockedRareCount),
			zap.Int32("rare_entry_num", refinementConfig.RareEntryNum))
	}

	// 记录锁定的稀有词条ID，避免重复生成
	usedAffixIDs := make(map[string]bool)
	for _, affix := range lockedRareAffixes {
		usedAffixIDs[affix.AffixID] = true
	}

	// 步骤1: 生成普通词条（全部重新生成）
	for i := int32(0); i < normalCount && len(usedAffixIDs) < len(normalAffixes)+len(rareAffixes); i++ {
		selected := selectAffixByWeightExcluding(normalAffixes, usedAffixIDs)
		if selected != nil {
			maxVal := selected.MaxValue * maxRatio / 100
			if maxVal < selected.MinValue {
				maxVal = selected.MinValue
			}
			valueRange := maxVal - selected.MinValue + 1
			value := selected.MinValue
			if valueRange > 0 {
				value = rand.Int31n(valueRange) + selected.MinValue
			}
			affixes = append(affixes, CrystalEquipmentAffix{
				AffixID: selected.ID,
				Value:   value,
			})
			usedAffixIDs[selected.ID] = true

			logger.Debug("生成普通词条",
				zap.String("affix_id", selected.ID),
				zap.Int32("min_value", selected.MinValue),
				zap.Int32("max_value", maxVal),
				zap.Int32("random_value", value),
				zap.Int32("max_ratio", maxRatio))
		}
	}

	// 步骤2: 添加锁定的稀有词条
	affixes = append(affixes, lockedRareAffixes...)

	if len(lockedRareAffixes) > 0 {
		logger.Debug("添加锁定的稀有词条",
			zap.Int("locked_rare_count", len(lockedRareAffixes)))
		for _, affix := range lockedRareAffixes {
			logger.Debug("锁定的稀有词条详情",
				zap.String("affix_id", affix.AffixID),
				zap.Int32("value", affix.Value))
		}
	}

	// 步骤3: 生成新的稀有词条（填充剩余配额）
	for i := int32(0); i < newRareCount && len(usedAffixIDs) < len(normalAffixes)+len(rareAffixes); i++ {
		selected := selectAffixByWeightExcluding(rareAffixes, usedAffixIDs)
		if selected != nil {
			maxVal := selected.MaxValue * maxRatio / 100
			if maxVal < selected.MinValue {
				maxVal = selected.MinValue
			}
			valueRange := maxVal - selected.MinValue + 1
			value := selected.MinValue
			if valueRange > 0 {
				value = rand.Int31n(valueRange) + selected.MinValue
			}
			affixes = append(affixes, CrystalEquipmentAffix{
				AffixID: selected.ID,
				Value:   value,
			})
			usedAffixIDs[selected.ID] = true

			logger.Info("生成新的稀有词条",
				zap.String("affix_id", selected.ID),
				zap.Int32("min_value", selected.MinValue),
				zap.Int32("max_value", maxVal),
				zap.Int32("random_value", value),
				zap.Int32("max_ratio", maxRatio),
				zap.Int32("weight", selected.Weight))
		}
	}

	// 最终汇总日志
	logger.Info("词条生成完成",
		zap.Int("total_affixes", len(affixes)),
		zap.Int32("normal_count", normalCount),
		zap.Int32("locked_rare_count", lockedRareCount),
		zap.Int32("new_rare_count", newRareCount),
		zap.Int32("total_rare_count", lockedRareCount+newRareCount))

	return affixes
}

// selectAffixByWeightExcluding 使用权重随机选择一个未使用的词条
//
// 算法说明：权重随机算法（Weighted Random Selection）
//
// 原理：
// 1. 将每个词条的权重看作一段区间
// 2. 所有区间首尾相连，形成一条线段，总长度为所有权重之和
// 3. 在这条线段上随机选择一个点
// 4. 该点落在哪个区间，就选择对应的词条
//
// 示例：
// 假设有3个词条：
//   - 词条A: weight=10, 区间[0, 10)
//   - 词条B: weight=20, 区间[10, 30)
//   - 词条C: weight=5,  区间[30, 35)
//
// 总权重=35，随机值范围[0, 35)
//   - 如果随机到8，落在[0,10)，选择词条A
//   - 如果随机到25，落在[10,30)，选择词条B
//   - 如果随机到32，落在[30,35)，选择词条C
//
// 权重越大的词条，其区间越长，被选中的概率越高
//
// 参数：
//   - affixes: 候选词条列表
//   - excludedIDs: 已使用的词条ID集合（这些词条将被排除）
//
// 返回：
//   - 根据权重随机选择的词条，如果无可用词条则返回nil
func selectAffixByWeightExcluding(affixes []template.TplCrystalEquipmentRefinementAffix, excludedIDs map[string]bool) *template.TplCrystalEquipmentRefinementAffix {
	if len(affixes) == 0 {
		return nil
	}

	// 步骤1: 过滤出未使用的词条
	availableAffixes := make([]template.TplCrystalEquipmentRefinementAffix, 0)
	for _, affix := range affixes {
		if !excludedIDs[affix.ID] {
			availableAffixes = append(availableAffixes, affix)
		}
	}

	if len(availableAffixes) == 0 {
		return nil
	}

	// 步骤2: 计算总权重
	totalWeight := int32(0)
	for _, affix := range availableAffixes {
		totalWeight += affix.Weight
	}

	// 特殊情况：如果所有词条权重都为0，则使用均匀随机
	if totalWeight == 0 {
		return &availableAffixes[rand.Intn(len(availableAffixes))]
	}

	// 步骤3: 在[0, totalWeight)范围内生成随机值
	randomValue := rand.Int31n(totalWeight)

	// 步骤4: 累加权重，找到随机值落在哪个区间
	// 例如：随机值=15，词条权重为[10, 20, 5]
	// - 累加到10: 15 >= 10，继续
	// - 累加到30: 15 < 30，选择第二个词条
	currentWeight := int32(0)
	for i := range availableAffixes {
		currentWeight += availableAffixes[i].Weight
		if randomValue < currentWeight {
			return &availableAffixes[i]
		}
	}

	// 容错：理论上不会执行到这里，但为了安全返回最后一个
	return &availableAffixes[len(availableAffixes)-1]
}
