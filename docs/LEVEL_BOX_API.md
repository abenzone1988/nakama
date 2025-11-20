# 关卡宝箱接口文档

## 功能概述

关卡宝箱系统允许玩家在通过关卡后领取宝箱奖励。每个关卡有3个宝箱，玩家可以一次性领取多个宝箱，但每个宝箱只能领取一次。

## 接口信息

**方法名称**: `ClaimLevelBox`

**请求消息**: `ClaimLevelBoxRequest`

**响应消息**: `ClaimLevelBoxResponse`

## 请求参数

```protobuf
message ClaimLevelBoxRequest{
  string level_id = 1;        // 关卡ID，如 "L1001"
  repeated int32 box_ids = 2; // 宝箱ID列表，1-3
}
```

### 参数说明

- `level_id`: 要领取宝箱的关卡 ID（必须是已经通过的关卡）
- `box_ids`: 要领取的宝箱 ID 列表，可以包含 1、2、3 中的一个或多个
  - `1`: 第一个宝箱（对应 `Reward01`）
  - `2`: 第二个宝箱（对应 `Reward02`）
  - `3`: 第三个宝箱（对应 `Reward03`）

## 响应参数

```protobuf
message ClaimLevelBoxResponse{
  int32 code = 1;
  string msg = 2;
  repeated Reward rewards = 3;         // 奖励列表
  Wallet wallet_updated = 4;           // 钱包更新结果
  repeated Item inventory_updated = 5; // 背包更新结果
}
```

### 状态码说明

| Code | 说明 |
|------|------|
| 0 | 领取成功 |
| 1 | 关卡ID不能为空 |
| 2 | 请选择要领取的宝箱 |
| 3 | 宝箱ID无效，只能是1、2或3 |
| 4 | 关卡不存在 |
| 5 | 获取用户数据失败 |
| 6 | 关卡尚未通过 |
| 7 | 加载宝箱数据失败 |
| 8 | 宝箱已经领取过了 |
| 9 | 发放奖励失败 |
| 10 | 保存数据失败 |

## 领取规则

### 前置条件

1. **关卡已通过**：玩家必须已经通过该关卡才能领取宝箱
   - 判断依据：`UserMeta.Level.CurLevelId >= 目标关卡ID`
   - 例如：当前最大关卡是 L1005，可以领取 L1001~L1005 的宝箱

2. **宝箱未领取**：每个宝箱只能领取一次
   - 已领取的宝箱会被记录
   - 尝试重复领取会返回错误

### 领取方式

1. **单个领取**：一次领取一个宝箱
   ```json
   {
     "level_id": "L1001",
     "box_ids": [1]
   }
   ```

2. **批量领取**：一次领取多个宝箱
   ```json
   {
     "level_id": "L1001",
     "box_ids": [1, 2, 3]
   }
   ```

3. **部分领取**：先领取部分，后续再领取剩余的
   ```json
   // 第一次领取
   {
     "level_id": "L1001",
     "box_ids": [1, 2]
   }
   
   // 第二次领取
   {
     "level_id": "L1001",
     "box_ids": [3]
   }
   ```

## 数据存储

宝箱领取数据存储在独立的 Storage 中：

```json
{
  "claimed_boxes": {
    "L1001": [1, 2, 3],
    "L1002": [1],
    "L1003": [1, 2]
  }
}
```

- **Collection**: `level_box`
- **Key**: `data`
- **结构**：记录每个关卡已领取的宝箱ID列表

## 配置说明

宝箱奖励配置在 `TplLevelInfo.json` 中：

```json
{
  "L1001": {
    "reward01": "Treasure1_Level001",  // 第一个宝箱奖励ID
    "reward02": "Treasure2_Level001",  // 第二个宝箱奖励ID
    "reward03": "Treasure3_Level001"   // 第三个宝箱奖励ID
  }
}
```

- `reward01`: 第一个宝箱的奖励配置 ID（在 TplReward.json 中配置）
- `reward02`: 第二个宝箱的奖励配置 ID
- `reward03`: 第三个宝箱的奖励配置 ID

## 客户端集成示例

### C# 调用示例

```csharp
// 单个宝箱领取
var request1 = new ClaimLevelBoxRequest
{
    LevelId = "L1001",
    BoxIds = { 1 }
};

var response1 = await client.ClaimLevelBoxAsync(request1);
if (response1.Code == 0)
{
    Debug.Log("宝箱1领取成功！");
    foreach (var reward in response1.Rewards)
    {
        Debug.Log($"获得金币：{reward.Wallet.Coin}");
    }
}

// 批量领取所有宝箱
var request2 = new ClaimLevelBoxRequest
{
    LevelId = "L1001",
    BoxIds = { 1, 2, 3 }
};

var response2 = await client.ClaimLevelBoxAsync(request2);
if (response2.Code == 0)
{
    Debug.Log($"成功领取 {response2.Rewards.Count} 个宝箱！");
}
else if (response2.Code == 6)
{
    Debug.Log("关卡尚未通过，无法领取宝箱");
}
else if (response2.Code == 8)
{
    Debug.Log("宝箱已经领取过了");
}

// 检查哪些宝箱可以领取
public bool CanClaimBox(string levelId, int boxId)
{
    // 1. 检查关卡是否通过
    var currentLevelNum = ParseLevelId(currentLevelId);  // 从 UserMeta 获取
    var targetLevelNum = ParseLevelId(levelId);
    
    if (targetLevelNum > currentLevelNum)
    {
        return false; // 关卡未通过
    }
    
    // 2. 检查宝箱是否已领取（从本地缓存的 LevelBoxData 检查）
    if (levelBoxData.ClaimedBoxes.ContainsKey(levelId))
    {
        if (levelBoxData.ClaimedBoxes[levelId].Contains(boxId))
        {
            return false; // 已领取
        }
    }
    
    return true;
}
```

### 获取已领取宝箱数据

客户端需要通过 Storage 接口获取宝箱领取数据：

```csharp
var objects = await client.ReadStorageObjectsAsync(session, new StorageObjectId
{
    Collection = "level_box",
    Key = "data",
    UserId = session.UserId
});

if (objects.Objects.Count > 0)
{
    var levelBoxData = JsonUtility.FromJson<LevelBoxData>(objects.Objects[0].Value);
    // 使用 levelBoxData.ClaimedBoxes 检查哪些宝箱已领取
}
```

## 使用场景

### 场景1：通关后立即领取所有宝箱

```
玩家通过关卡 L1001
→ 自动弹出宝箱界面
→ 一键领取所有宝箱
→ 调用 ClaimLevelBox(level_id: "L1001", box_ids: [1, 2, 3])
```

### 场景2：回到关卡列表补领宝箱

```
玩家查看关卡列表
→ 发现之前的关卡还有未领取的宝箱
→ 点击关卡宝箱图标
→ 选择要领取的宝箱
→ 调用 ClaimLevelBox(level_id: "L1003", box_ids: [2, 3])
```

### 场景3：分次领取宝箱

```
第一次：领取宝箱1和2
→ ClaimLevelBox(level_id: "L1001", box_ids: [1, 2])

第二次：领取宝箱3
→ ClaimLevelBox(level_id: "L1001", box_ids: [3])
```

## 注意事项

1. **关卡顺序**：必须按顺序通过关卡才能领取宝箱
2. **一次性领取**：每个宝箱只能领取一次，已领取的宝箱不能重复领取
3. **批量优化**：建议一次性领取所有可领取的宝箱，减少请求次数
4. **数据同步**：领取后及时更新本地缓存的宝箱领取状态
5. **错误处理**：如果批量领取中部分宝箱已领取，会过滤掉已领取的宝箱，只发放未领取的奖励

## 测试建议

1. 测试未通过关卡时领取宝箱（应该失败）
2. 测试通过关卡后领取单个宝箱
3. 测试通过关卡后一次性领取所有宝箱
4. 测试重复领取同一个宝箱（应该失败）
5. 测试分批领取宝箱
6. 测试无效的宝箱ID（如4、0、-1）
7. 测试跨关卡领取宝箱
8. 测试空的 box_ids 参数

