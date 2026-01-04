# 随机品质炮台功能实现总结

## 实现内容

### 1. 添加 TplEquipment 表到 TemplateManager

**修改文件**: `server/game_template.go`

- 在 `TemplateManager` 接口中添加 `GetTplEquipment()` 方法
- 在 `LocalTemplateManager` 结构体中添加 `tableTplEquipment` 字段
- 在 `NewLocalTemplateManager` 中初始化 `tableTplEquipment`
- 在 `LoadData` 中加载 `TplEquipment` 数据
- 实现 `GetTplEquipment()` 方法返回表实例

### 2. 实现随机品质炮台逻辑

**修改文件**: `server/game_util.go`

**新增函数**: `distributeRandomQualityTurret`

```go
func distributeRandomQualityTurret(ctx context.Context, logger *zap.Logger, 
    db *sql.DB, templateMgr TemplateManager, metrics Metrics, 
    storageIndex StorageIndex, count int32, quality int32) (map[string]int64, error)
```

**功能说明**:
- 根据 `TplItem` 中的 `quality` 字段，从 `TplEquipment` 表中筛选对应品质的炮台
- 筛选条件：`quality` 匹配、`type=1`（炮台类型）、排除水晶基地（`EQ1999`）
- 随机选择炮台进行发放
- 如果炮台已解锁，转换为 10 个对应的碎片（`debrisId = equipID` 去掉 "EQ" 前缀）
- 如果炮台未解锁，调用 `tryUnlockEquipment` 解锁该炮台

**修改**: `GrantReward` 函数中 `ItemType_RandomQualityTurret` 分支
- 获取道具模板的 `quality` 字段
- 调用 `distributeRandomQualityTurret` 进行随机分配
- 将结果添加到 `inventoryChangeset` 和 `convertedItems`

### 3. 装备解锁/碎片转换逻辑

**已有函数**: `tryUnlockEquipment`

```go
func tryUnlockEquipment(ctx context.Context, logger *zap.Logger, 
    db *sql.DB, metrics Metrics, storageIndex StorageIndex, 
    equipID string) (bool, error)
```

**功能**:
- 检查装备是否已解锁
- 如果已解锁，返回 `true`
- 如果未解锁，将装备添加到 `equipData.UnlockEquips`，初始等级为 1
- 保存装备数据

**碎片转换规则**:
- 炮台已解锁时，转换为碎片数量 = `count * TurretDebrisCount`（10）
- 碎片 ID = 装备 ID 去掉 "EQ" 前缀
- 例如：`EQ1100` → 碎片 ID `1100`

### 4. 测试接口

**新增文件**: `server/api_test.go` 中的 `TestGrantReward` 函数

**功能**:
- 解析奖励字符串格式：`itemid_num,itemid_num`
- 转换为 `game.Reward` 结构
- 调用 `GrantReward` 发放奖励
- 打印钱包和背包变化

**示例代码**: `server/test_grant_reward_example.go`

## 使用示例

### 1. 解析奖励字符串

```go
rewardStr := "10000_1000,10001_500,1100_1,1101_2"
items := strings.Split(rewardStr, ",")
rewardItems := make([]*game.Item, 0)

for _, item := range items {
    parts := strings.Split(strings.TrimSpace(item), "_")
    if len(parts) != 2 {
        continue
    }
    itemID := strings.TrimSpace(parts[0])
    count, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
    rewardItems = append(rewardItems, &game.Item{
        Id:  itemID,
        Num: int32(count),
    })
}

reward := &game.Reward{
    Items: rewardItems,
}
```

### 2. 发放奖励

```go
walletResult, inventoryResult, err := GrantReward(
    ctx, logger, db, templateMgr, metrics, storageIndex,
    reward, "test_grant_reward")
if err != nil {
    logger.Error("发放奖励失败", zap.Error(err))
    return
}
```

### 3. 查看结果

```go
if walletResult != nil {
    logger.Info("钱包变化",
        zap.Int32("coin_before", walletResult.Previous.Coin),
        zap.Int32("coin_after", walletResult.Updated.Coin))
}

if inventoryResult != nil {
    for _, item := range inventoryResult.Updated {
        logger.Info("背包物品", 
            zap.String("id", item.Id), 
            zap.Int32("num", item.Num))
    }
}
```

## 随机品质炮台机制

### 配置说明

**TplItem 表**:
```json
{
  "id": "50001",
  "itemType": 16,  // ItemType_RandomQualityTurret
  "quality": 4     // 品质等级
}
```

**TplEquipment 表**:
```json
{
  "id": "EQ1100",
  "type": 1,      // 炮台类型
  "quality": 4    // 品质等级
}
```

### 随机逻辑

1. 从 `TplEquipment` 中筛选：
   - `quality` = 道具的 `quality`
   - `type` = 1（炮台）
   - `id` ≠ "EQ1999"（排除水晶基地）

2. 随机选择一个炮台

3. 判断是否已解锁：
   - **已解锁**: 转换为 10 个碎片
   - **未解锁**: 直接解锁炮台

### 示例

假设有品质为 4 的随机炮台道具（ID: 50001），数量为 2：

**场景 1**: 两次都抽到已解锁的炮台
- 第1次抽到 EQ1100（已解锁）→ 获得 10 个 1100 碎片
- 第2次抽到 EQ1101（已解锁）→ 获得 10 个 1101 碎片
- 结果：1100 碎片 x10，1101 碎片 x10

**场景 2**: 抽到未解锁的炮台
- 第1次抽到 EQ1102（未解锁）→ 解锁 EQ1102
- 第2次抽到 EQ1102（已解锁）→ 获得 10 个 1102 碎片
- 结果：解锁 EQ1102，1102 碎片 x10

## 相关常量

```go
const TurretDebrisCount = 10  // 每个炮台转换为碎片的数量

const ItemType_RandomQualityTurret = 16  // 随机品质炮台类型

const EquipID_Crystal = "EQ1999"  // 水晶基地ID（排除）
```

## 注意事项

1. **品质匹配**: 道具的 `quality` 必须与装备表中的 `quality` 匹配
2. **炮台类型**: 只选择 `type=1` 的装备（炮台类型）
3. **排除水晶**: 水晶基地（EQ1999）不参与随机
4. **碎片转换**: 已解锁炮台自动转换为 10 个碎片
5. **解锁逻辑**: 未解锁炮台会自动解锁，初始等级为 1

## 测试

由于需要数据库连接，测试函数 `TestGrantReward` 需要在有数据库环境的情况下运行。

运行测试：
```bash
go test -v -run TestGrantReward ./server
```

如果数据库未启动，可以参考 `server/test_grant_reward_example.go` 中的示例代码进行集成。

