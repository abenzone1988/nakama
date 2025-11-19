package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
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
	return &StorageOpWrite{
		Object: &api.WriteStorageObject{
			Collection:      collection,
			Key:             key,
			Value:           value,
			PermissionRead:  &wrapperspb.Int32Value{Value: permissionRead},
			PermissionWrite: &wrapperspb.Int32Value{Value: permissionWrite},
		},
		OwnerID: ownerID,
	}
}

type Storable interface {
	GetCollection() string
	GetKey() string
	Init()
}

func LoadData(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, storable Storable) error {
	readOp := &api.ReadStorageObjectId{
		Collection: storable.GetCollection(),
		Key:        storable.GetKey(),
		UserId:     userID.String(),
	}

	objectIDs := []*api.ReadStorageObjectId{readOp}

	storageObjects, err := StorageReadObjects(ctx, logger, db, userID, objectIDs)
	if err != nil {
		logger.Error("无法从存储系统读取数据", zap.Error(err))
		return err
	}

	if len(storageObjects.Objects) == 0 {
		storable.Init()
		return nil
	}

	if err := json.Unmarshal([]byte(storageObjects.Objects[0].Value), storable); err != nil {
		logger.Error("无法反序列化数据", zap.Error(err))
		return err
	}

	return nil
}

func SaveData(ctx context.Context, logger *zap.Logger, db *sql.DB, metrics Metrics, storageIndex StorageIndex, userID uuid.UUID, storable Storable) error {
	serializedData, err := json.Marshal(storable)
	if err != nil {
		logger.Error("无法序列化数据", zap.Error(err))
		return err
	}

	writeOp := CreateStorageOpWrite(storable.GetCollection(), storable.GetKey(), string(serializedData), userID.String())

	ops := []*StorageOpWrite{writeOp}

	_, _, err = StorageWriteObjects(ctx, logger, db, metrics, storageIndex, true, ops)
	if err != nil {
		logger.Error("无法保存数据到存储系统", zap.Error(err))
		return err
	}

	return nil
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

func convertMapInt64ToInt32(m map[string]int64) map[string]int32 {
	if m == nil {
		return nil
	}
	result := make(map[string]int32, len(m))
	for k, v := range m {
		result[k] = int32(v)
	}
	return result
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
		Coin: int32(m["coin"]),
		Gem:  int32(m["gem"]),
		Ad:   int32(m["ad"]),
	}
}

// GrantReward 统一的奖励发放函数（处理特殊道具逻辑）
// 用于商店购买、战斗结算、礼包兑换等所有奖励发放场景
func GrantReward(ctx context.Context, logger *zap.Logger, db *sql.DB, template TemplateManager, reward *game.Reward, source string) (*game.WalletUpdateResult, *game.InventoryUpdateResult, error) {
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
	}

	// 处理道具奖励（包含特殊道具转换）
	if len(reward.Items) > 0 {
		for _, item := range reward.Items {
			if item == nil || item.Num <= 0 {
				continue
			}

			// 处理特殊道具
			switch item.Id {
			case "10000": // 金币
				walletChangeset["coin"] += int64(item.Num)
			case "10001": // 钻石
				walletChangeset["gem"] += int64(item.Num)
			case "10002": // 体力
				// 体力需要特殊处理，使用RefillStamina函数
				if err := RefillStamina(ctx, logger, item.Num); err != nil {
					logger.Error("添加体力失败", zap.Error(err))
				}
			case "20000": // 广告券
				walletChangeset["ad"] += int64(item.Num)
			case "60000": // 随机水晶
				// 等概率转换为4种水晶 (30001-30004)
				crystalIds := []string{"30001", "30002", "30003", "30004"}
				for i := int32(0); i < item.Num; i++ {
					randomCrystal := crystalIds[rand.Intn(len(crystalIds))]
					inventoryChangeset[randomCrystal]++
				}
			case "50000": // 随机炮台（转为碎片）
				// 随机选择一个炮台，给10个碎片
				distributedDebris, err := distributeTurretDebris(ctx, logger, db, template, item.Num, true)
				if err != nil {
					logger.Error("分配随机炮台碎片失败", zap.Error(err))
				} else {
					for debrisId, count := range distributedDebris {
						inventoryChangeset[debrisId] += count
					}
				}
			case "90000": // 炮台碎片
				// 根据炮台等级概率分配，每个碎片单独随机
				distributedDebris, err := distributeTurretDebris(ctx, logger, db, template, item.Num, false)
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
					alreadyUnlocked, err := tryUnlockEquipment(ctx, logger, item.Id)
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

						logger.Info("炮台已解锁，转换为碎片",
							zap.String("equip_id", item.Id),
							zap.String("debris_id", debrisId),
							zap.Int32("debris_count", debrisCount))
					} else {
						// 炮台首次解锁成功，也发放炮台到背包
						inventoryChangeset[item.Id] += int64(item.Num)
						logger.Info("成功解锁新炮台",
							zap.String("equip_id", item.Id))
					}
				} else {
					// 普通道具直接进背包
					inventoryChangeset[item.Id] += int64(item.Num)
				}
			}
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
func tryUnlockEquipment(ctx context.Context, logger *zap.Logger, equipID string) (bool, error) {
	// 检查是否已解锁
	alreadyUnlocked := false

	// 更新玩家装备数据
	err := UpdateUserMeta(ctx, func(meta *game.UserMeta) error {
		// 初始化装备数据（如果不存在）
		if meta.Equip == nil {
			return fmt.Errorf("装备数据为空")
		}

		// 检查是否已解锁
		if _, exists := meta.Equip.UnlockEquips[equipID]; exists {
			alreadyUnlocked = true
			logger.Debug("炮台已解锁",
				zap.String("equip_id", equipID))
			return nil
		}

		// 解锁炮台，初始等级为1
		meta.Equip.UnlockEquips[equipID] = 1

		logger.Info("解锁新炮台",
			zap.String("equip_id", equipID))

		return nil
	})

	if err != nil {
		logger.Error("更新玩家装备数据失败",
			zap.Error(err),
			zap.String("equip_id", equipID))
		return false, fmt.Errorf("更新玩家装备数据失败")
	}
	return alreadyUnlocked, nil
}

// distributeTurretDebris 根据炮台等级概率分配炮台碎片
func distributeTurretDebris(ctx context.Context, logger *zap.Logger, db *sql.DB, templateMgr TemplateManager, totalDebrisCount int32, singleTurret bool) (map[string]int64, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)

	logger.Debug("开始分配炮台碎片",
		zap.String("user_id", userID.String()),
		zap.Int32("total_debris_count", totalDebrisCount),
		zap.Bool("single_turret", singleTurret))

	// 获取玩家装备数据
	equipData, err := GetEquipData(ctx, logger, templateMgr)
	if err != nil {
		logger.Error("获取装备数据失败", zap.Error(err))
		return nil, err
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
			logger.Debug("跳过水晶装备，不参与碎片随机分配",
				zap.String("equip_id", equipID))
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

		logger.Debug("炮台碎片计算详情",
			zap.String("equip_id", equipID),
			zap.String("debris_id", debrisId),
			zap.Int32("level", level),
			zap.Int32("consumed_debris", consumedDebris),
			zap.Int32("inventory_debris", inventoryDebris),
			zap.Int32("total_debris", totalDebris),
			zap.Int32("weight", weight))

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

	logger.Debug("炮台权重汇总",
		zap.Int("turret_count", len(turrets)),
		zap.Int32("total_weight", totalWeight))

	result := make(map[string]int64)

	if singleTurret {
		// 50000 模式：只随机选择一个炮台，给全部碎片
		randomValue := rand.Int31n(totalWeight)
		currentWeight := int32(0)

		for _, t := range turrets {
			currentWeight += t.DebrisWeight
			if randomValue < currentWeight {
				debrisId := strings.TrimPrefix(t.EquipID, "EQ")
				result[debrisId] = int64(totalDebrisCount)

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
