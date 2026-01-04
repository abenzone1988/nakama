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
func GrantReward(ctx context.Context, logger *zap.Logger, db *sql.DB, template TemplateManager, metrics Metrics, storageIndex StorageIndex, reward *game.Reward, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
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
		convertedRandomCrystals := make(map[string]int32)
		convertedRandomEquips := make(map[string]int32)
		convertedEquipToDebris := make(map[string]int32) // 装备转换为碎片
		for _, item := range reward.Items {
			//logger.Info("处理道具奖励", zap.Any("item", item))
			if item == nil || item.Num <= 0 {
				continue
			}

			// 处理特殊道具
			switch item.Id {
			case ItemID_Coin: // 金币
				walletChangeset["coin"] += int64(item.Num)
			case ItemID_Gem: // 钻石
				walletChangeset["gem"] += int64(item.Num)
			case ItemID_AdTicket: // 广告券
				walletChangeset["ad"] += int64(item.Num)
			case ItemID_Stamina: // 体力
				walletChangeset["stamina"] += int64(item.Num)
			case ItemID_RandomCrystal: // 随机水晶
				// 等概率转换为4种水晶 (30001-30004)
				crystalIds := []string{CrystalID_1, CrystalID_2, CrystalID_3, CrystalID_4}
				for i := int32(0); i < item.Num; i++ {
					randomCrystal := crystalIds[rand.Intn(len(crystalIds))]
					inventoryChangeset[randomCrystal]++
					convertedRandomCrystals[randomCrystal]++
				}
			case ItemID_RandomTurret: // 随机炮台（转为碎片）
				// 随机选择一个炮台，给10个碎片
				distributedDebris, err := distributeTurretDebris(ctx, logger, db, template, metrics, storageIndex, item.Num, true)
				if err != nil {
					logger.Error("分配随机炮台碎片失败", zap.Error(err))
				} else {
					for debrisId, count := range distributedDebris {
						inventoryChangeset[debrisId] += count
						convertedRandomEquips[debrisId] += int32(count)
					}
				}
			case ItemID_TurretDebris: // 炮台碎片
				// 根据炮台等级概率分配，每个碎片单独随机
				distributedDebris, err := distributeTurretDebris(ctx, logger, db, template, metrics, storageIndex, item.Num, false)
				if err != nil {
					logger.Error("分配炮台碎片失败", zap.Error(err))
				} else {
					for debrisId, count := range distributedDebris {
						inventoryChangeset[debrisId] += count
					}
				}
			default:
				// 检查是否是炮台装备（以EQ开头）
				if strings.HasPrefix(item.Id, "EQ") {
					// 炮台装备特殊处理
					logger.Info("尝试解锁炮台", zap.String("equip_id", item.Id), zap.Int32("num", item.Num))
					alreadyUnlocked, err := tryUnlockEquipment(ctx, logger, db, metrics, storageIndex, item.Id)
					if err != nil {
						logger.Error("解锁炮台失败",
							zap.Error(err),
							zap.String("equip_id", item.Id))
						// 解锁失败，仍然尝试发放原物品
						inventoryChangeset[item.Id] += int64(item.Num)
					} else if alreadyUnlocked {
						// 炮台已解锁，转换为碎片（10个 × 数量）
						debrisId := strings.TrimPrefix(item.Id, "EQ")
						debrisCount := item.Num * 10 // 每个炮台转换为10个碎片
						inventoryChangeset[debrisId] += int64(debrisCount)
						convertedEquipToDebris[debrisId] += debrisCount // 记录转换的碎片
						// 标记装备ID需要跳过（已解锁的装备应该被移除，只保留碎片）
						convertedEquipToDebris[item.Id] = 0 // 0表示需要移除装备，只保留碎片

						logger.Info("炮台已解锁，转换为碎片",
							zap.String("equip_id", item.Id),
							zap.String("debris_id", debrisId),
							zap.Int32("debris_count", debrisCount))
					} else {
						// 炮台首次解锁成功，装备不发到背包，只解锁
						if item.Num == 1 {
							// 只有1个，只解锁，不发到背包，但奖励中保留装备
							logger.Info("成功解锁新炮台",
								zap.String("equip_id", item.Id),
								zap.Int32("equip_count", 1))
							// 不添加到 convertedEquipToDebris，保留在奖励中
						} else {
							// 数量大于1：第1个用于解锁（不发到背包，但奖励中保留），剩下的转换为碎片
							remainingCount := item.Num - 1 // 剩下的装备数量
							debrisId := strings.TrimPrefix(item.Id, "EQ")
							debrisCount := remainingCount * 10 // 每个装备转换为10个碎片
							inventoryChangeset[debrisId] += int64(debrisCount)
							convertedEquipToDebris[debrisId] += debrisCount // 记录转换的碎片
							// 标记需要将装备数量改为1（保留第1个装备）
							convertedEquipToDebris[item.Id] = -item.Num // 负数表示需要修改数量

							logger.Info("成功解锁新炮台，部分转换为碎片",
								zap.String("equip_id", item.Id),
								zap.Int32("equip_count", 1),
								zap.Int32("remaining_count", remainingCount),
								zap.String("debris_id", debrisId),
								zap.Int32("debris_count", debrisCount))
						}
					}
				} else {
					// 普通道具直接进背包
					inventoryChangeset[item.Id] += int64(item.Num)
				}
			}
		}
		// 更新 reward.Items 以反映转换后的结果
		if len(convertedRandomCrystals) > 0 || len(convertedRandomEquips) > 0 || len(convertedEquipToDebris) > 0 {
			skipIds := map[string]struct{}{}
			if len(convertedRandomCrystals) > 0 {
				skipIds[ItemID_RandomCrystal] = struct{}{}
			}
			if len(convertedRandomEquips) > 0 {
				skipIds[ItemID_RandomTurret] = struct{}{}
			}
			// 标记需要移除或修改数量的装备
			equipCountModify := make(map[string]int32) // 需要修改数量的装备
			for equipId, count := range convertedEquipToDebris {
				if count < 0 {
					// 负数表示需要修改数量（首次解锁且数量>1的情况）
					equipCountModify[equipId] = 1 // 将数量改为1
					skipIds[equipId] = struct{}{} // 先跳过，后面重新添加
				} else if count == 0 && strings.HasPrefix(equipId, "EQ") {
					// 0表示需要移除的装备（已解锁，全部转换为碎片）
					skipIds[equipId] = struct{}{}
				} else if count > 0 && strings.HasPrefix(equipId, "EQ") {
					// 正数且以EQ开头，这不应该发生，但为了安全也跳过
					skipIds[equipId] = struct{}{}
				}
			}

			newItems := make([]*game.Item, 0, len(reward.Items)+len(convertedRandomCrystals)+len(convertedRandomEquips)+len(convertedEquipToDebris))
			for _, item := range reward.Items {
				if item == nil {
					continue
				}
				if _, skip := skipIds[item.Id]; skip {
					// 如果是需要修改数量的装备，重新添加数量为1的版本
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

			// 添加转换后的随机水晶
			for crystalID, count := range convertedRandomCrystals {
				newItems = append(newItems, &game.Item{
					Id:  crystalID,
					Num: count,
				})
			}
			// 添加转换后的随机炮台碎片
			for equipID, count := range convertedRandomEquips {
				newItems = append(newItems, &game.Item{
					Id:  equipID,
					Num: count,
				})
			}
			// 添加转换后的装备碎片
			for debrisId, count := range convertedEquipToDebris {
				if count > 0 {
					newItems = append(newItems, &game.Item{
						Id:  debrisId,
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

// distributeTurretDebris 根据炮台等级概率分配炮台碎片
func distributeTurretDebris(ctx context.Context, logger *zap.Logger, db *sql.DB, templateMgr TemplateManager, metrics Metrics, storageIndex StorageIndex, totalDebrisCount int32, singleTurret bool) (map[string]int64, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	logger.Debug("开始分配炮台碎片",
		zap.String("user_id", userID.String()),
		zap.Int32("total_debris_count", totalDebrisCount),
		zap.Bool("single_turret", singleTurret))

	// 直接加载装备数据（不需要保存，所以传入 nil）
	equipData := &EquipData{}
	if err := LoadUserData(ctx, logger, db, equipData); err != nil {
		logger.Error("加载装备数据失败", zap.Error(err))
		return nil, err
	}

	// 如果数据为空，返回空结果
	if len(equipData.UnlockEquips) == 0 {
		logger.Debug("装备数据为空，无法分配碎片")
		return make(map[string]int64), nil
	}

	logger.Debug("玩家已解锁炮台数量",
		zap.Int("unlocked_count", len(equipData.UnlockEquips)))

	// 获取玩家背包数据（碎片数量）
	inventory, err := GetInventory(ctx, logger, db, userID)
	if err != nil {
		logger.Error("获取背包数据失败", zap.Error(err))
		return nil, err
	}

	// 计算每个炮台的总碎片数（已升级消耗 + 背包持有）
	type TurretInfo struct {
		EquipID      string
		Level        int32
		TotalDebris  int32 // 已消耗 + 背包持有的总碎片数
		DebrisWeight int32 // 权重（碎片数越少权重越高）
	}

	turrets := make([]TurretInfo, 0)

	// 遍历所有已解锁的炮台
	for equipID, level := range equipData.UnlockEquips {
		// 跳过 EQ1999（水晶），它有独立的升级逻辑
		if equipID == "EQ1999" {
			//logger.Debug("跳过水晶装备，不参与碎片随机分配",
			//	zap.String("equip_id", equipID))
			continue
		}

		debrisId := strings.TrimPrefix(equipID, "EQ") // EQ1999 -> 1999

		// 计算已消耗的碎片数量（从等级1升到当前等级）
		consumedDebris := int32(0)
		for lv := int32(1); lv < level; lv++ {
			equipDevs := templateMgr.GetTplEquipmentDev().FindByFilter(func(equip template.TplEquipmentDev) bool {
				return equip.EquipID == equipID && equip.Level == lv+1
			})
			if equipDevs.Len() > 0 {
				consumedDebris += equipDevs.Get(0).CostDebris
			}
		}

		// 背包中持有的碎片数量
		inventoryDebris := int32(0)
		if count, ok := inventory[debrisId]; ok {
			inventoryDebris = int32(count)
		}

		// 总碎片数 = 已消耗 + 背包持有
		totalDebris := consumedDebris + inventoryDebris

		// 权重计算：碎片越少权重越高（使用反比例）
		// 基础权重 = 10000，权重 = 基础权重 / (totalDebris + 1)
		weight := int32(10000) / (totalDebris + 1)

		//logger.Debug("炮台碎片计算详情",
		//	zap.String("equip_id", equipID),
		//	zap.String("debris_id", debrisId),
		//	zap.Int32("level", level),
		//	zap.Int32("consumed_debris", consumedDebris),
		//	zap.Int32("inventory_debris", inventoryDebris),
		//	zap.Int32("total_debris", totalDebris),
		//	zap.Int32("weight", weight))

		turrets = append(turrets, TurretInfo{
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

	// 计算总权重
	totalWeight := int32(0)
	for _, t := range turrets {
		totalWeight += t.DebrisWeight
	}
	//
	//logger.Debug("炮台权重汇总",
	//	zap.Int("turret_count", len(turrets)),
	//	zap.Int32("total_weight", totalWeight))

	result := make(map[string]int64)

	if singleTurret {
		// 50000 模式：只随机选择一个炮台，给全部碎片
		randomValue := rand.Int31n(totalWeight)
		currentWeight := int32(0)

		for _, t := range turrets {
			currentWeight += t.DebrisWeight
			if randomValue < currentWeight {
				debrisId := strings.TrimPrefix(t.EquipID, "EQ")
				result[debrisId] = int64(totalDebrisCount) * int64(TurretDebrisCount) // 给全部碎片

				logger.Info("随机选择炮台（50000模式）",
					zap.String("user_id", userID.String()),
					zap.String("selected_equip_id", t.EquipID),
					zap.String("debris_id", debrisId),
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
			// 随机选择一个炮台
			randomValue := rand.Int31n(totalWeight)
			currentWeight := int32(0)

			selectedEquipID := ""
			for _, t := range turrets {
				currentWeight += t.DebrisWeight
				if randomValue < currentWeight {
					debrisId := strings.TrimPrefix(t.EquipID, "EQ")
					result[debrisId]++
					selectedEquipID = t.EquipID

					logger.Debug("随机抽取炮台碎片",
						zap.Int32("round", i+1),
						zap.Int32("random_value", randomValue),
						zap.String("selected_equip_id", selectedEquipID),
						zap.String("debris_id", debrisId),
						zap.Int32("current_total", int32(result[debrisId])))
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
func ConsumeCostItems(ctx context.Context, logger *zap.Logger, db *sql.DB, costItemStr string, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
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

		// 根据 itemID 判断是钱包类型还是背包道具
		switch itemID {
		case ItemID_Coin: // 金币
			walletChangeset["coin"] -= count
		case ItemID_Gem: // 钻石
			walletChangeset["gem"] -= count
		case ItemID_Stamina: // 体力
			walletChangeset["stamina"] -= count
		case ItemID_AdTicket: // 广告券
			walletChangeset["ad"] -= count
		default:
			// 其他道具从背包扣除
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
