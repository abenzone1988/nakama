# 关卡奖励接口文档

## 功能概述

本文档包含两个奖励系统：
1. **扫荡奖励**：通过消耗体力快速获取关卡奖励，无需实际进行战斗
2. **挂机奖励（巡逻奖励）**：根据挂机时长自动累积奖励，每小时可领取一次

## 接口信息

**方法名称**: `ClaimMoppingReward`

**请求消息**: `ClaimMoppingRewardRequest`

**响应消息**: `ClaimMoppingRewardResponse`

## 请求参数

```protobuf
message ClaimMoppingRewardRequest{
  string level_id = 1;    // 关卡ID，如 "L1001"
  bool watched_ad = 2;    // 是否观看了广告（第二阶段使用）
}
```

### 参数说明

- `level_id`: 要扫荡的关卡 ID（必须是已经通过的关卡）
- `watched_ad`: 在第二阶段扫荡时，标识是否观看了广告
  - `true`: 已观看广告，只扣除体力
  - `false`: 未观看广告，需要扣除体力 + 1个广告券

## 响应参数

```protobuf
message ClaimMoppingRewardResponse{
  int32 code = 1;                          // 状态码
  string msg = 2;                          // 提示信息
  Reward reward = 3;                       // 奖励数据
  Wallet wallet_updated = 4;               // 钱包更新结果
  repeated Item inventory_updated = 5;     // 背包更新结果
  int32 has_mopping_times = 6;            // 第一阶段已使用次数
  int32 has_mopping_times_for_adv = 7;    // 第二阶段已使用次数
}
```

### 状态码说明

| Code | 说明 |
|------|------|
| 0 | 扫荡成功 |
| 1 | 关卡不存在 |
| 2 | 该关卡没有扫荡奖励 |
| 3 | 获取用户数据失败 |
| 4 | 检查每日重置失败 |
| 5 | 重置每日次数失败 |
| 6 | 今日扫荡次数已用完 |
| 7 | 体力不足 |
| 8 | 广告券不足，请观看广告或购买广告券 |
| 9 | 奖励发放失败 |
| 10 | 更新扫荡次数失败 |

## 扫荡机制

### 阶段划分

扫荡分为两个阶段，每天共 **8 次**机会：

#### 第一阶段（前 3 次）
- **消耗**: 5 点体力
- **次数**: 每天 3 次
- **特点**: 只需要体力，无需广告

#### 第二阶段（后 5 次）
- **消耗**: 5 点体力 + 广告/广告券
- **次数**: 每天 5 次
- **特点**: 
  - 如果客户端观看了广告 (`watched_ad = true`)，只扣除体力
  - 如果未观看广告 (`watched_ad = false`)，扣除体力 + 1个广告券

### 每日重置

- 扫荡次数在每天 **00:00** 重置（使用本地时区）
- 重置后，第一阶段和第二阶段的次数都会归零
- 玩家可以重新获得 3+5=8 次扫荡机会

## UserMeta 数据结构

扫荡相关数据保存在 `UserMeta.Level` 中：

```json
{
  "level": {
    "cur_level_id": "L1011",              // 当前最大关卡ID
    "best_progress": 3,                    // 最好进度
    "has_mopping_times": 2,                // 第一阶段已使用次数（0-3）
    "has_mopping_times_for_adv": 1,        // 第二阶段已使用次数（0-5）
    "last_mopping_timestamp": "2025-11-20T14:30:00Z"  // 最后一次扫荡时间
  }
}
```

### 字段说明

- `has_mopping_times`: 第一阶段已使用次数，范围 0-3
- `has_mopping_times_for_adv`: 第二阶段已使用次数，范围 0-5
- `last_mopping_timestamp`: ISO 8601 格式的时间戳，用于判断是否跨天重置

## 客户端集成示例

### C# 调用示例

```csharp
// 第一阶段扫荡（只需体力）
var request1 = new ClaimMoppingRewardRequest
{
    LevelId = "L1001",
    WatchedAd = false  // 第一阶段此字段无效
};

var response1 = await client.ClaimMoppingRewardAsync(request1);
if (response1.Code == 0)
{
    Debug.Log($"扫荡成功！剩余次数：第一阶段 {3 - response1.HasMoppingTimes}/3");
}

// 第二阶段扫荡（观看了广告）
var request2 = new ClaimMoppingRewardRequest
{
    LevelId = "L1001",
    WatchedAd = true  // 已观看广告
};

var response2 = await client.ClaimMoppingRewardAsync(request2);
if (response2.Code == 0)
{
    Debug.Log($"扫荡成功！剩余次数：第二阶段 {5 - response2.HasMoppingTimesForAdv}/5");
}

// 第二阶段扫荡（未观看广告，使用广告券）
var request3 = new ClaimMoppingRewardRequest
{
    LevelId = "L1001",
    WatchedAd = false  // 未观看广告，需要扣除广告券
};

var response3 = await client.ClaimMoppingRewardAsync(request3);
if (response3.Code == 8)
{
    Debug.Log("广告券不足，请观看广告或购买广告券");
}
```

### 获取扫荡次数

客户端可以通过 `GetAccount` 接口获取 `UserMeta` 数据：

```csharp
var account = await client.GetAccountAsync();
var metadata = JsonUtility.FromJson<UserMeta>(account.User.Metadata);

int remainingStage1 = 3 - metadata.level.has_mopping_times;
int remainingStage2 = 5 - metadata.level.has_mopping_times_for_adv;

Debug.Log($"第一阶段剩余次数：{remainingStage1}/3");
Debug.Log($"第二阶段剩余次数：{remainingStage2}/5");
```

## 注意事项

1. **体力检查**: 扫荡前会先检查体力是否足够（需要 5 点），不足则返回错误码 7
2. **广告券检查**: 第二阶段未观看广告时，需要检查广告券是否足够（需要 1 个），不足则返回错误码 8
3. **每日重置**: 扫荡次数在每天 00:00 重置，跨天后自动恢复到 0/3 和 0/5
4. **关卡验证**: 只能扫荡配置了 `moppingReward` 的关卡
5. **奖励发放**: 扫荡奖励与战斗通关奖励相同（基于 `moppingReward` 配置）

## 配置说明

扫荡相关常量定义在 `server/game_mopping.go` 中：

```go
const (
    MoppingStage1MaxTimes = 3  // 第一阶段最大次数
    MoppingStage2MaxTimes = 5  // 第二阶段最大次数
    MoppingCostStamina    = 5  // 扫荡消耗的体力值
)
```

如需修改每日次数或体力消耗，可在此处调整。

---

## 挂机奖励接口（巡逻奖励）

### 接口信息

**方法名称**: `ClaimOnHookReward`

**请求消息**: `ClaimOnHookRewardRequest`

**响应消息**: `ClaimOnHookRewardResponse`

### 请求参数

```protobuf
message ClaimOnHookRewardRequest{
  // 无参数
}
```

### 响应参数

```protobuf
message ClaimOnHookRewardResponse{
  int32 code = 1;                      // 状态码
  string msg = 2;                      // 提示信息
  int32 hours = 3;                     // 领取的小时数
  int32 coin = 4;                      // 金币奖励
  Reward reward = 5;                   // 其他奖励数据
  Wallet wallet_updated = 6;           // 钱包更新结果
  repeated Item inventory_updated = 7; // 背包更新结果
}
```

### 状态码说明

| Code | 说明 |
|------|------|
| 0 | 领取成功 |
| 1 | 获取用户元数据失败 |
| 2 | 暂无挂机奖励 |
| 3 | 关卡不存在 |
| 4 | 当前关卡没有挂机奖励 |
| 5 | 初始化挂机时间失败 |
| 6 | 挂机时间已开始计算 |
| 7 | 解析上次领取时间失败 |
| 8 | 挂机时间不足1小时 |
| 9 | 发放金币失败 |
| 10 | 更新挂机时间失败 |

### 挂机奖励机制

#### 奖励计算规则

1. **首次领取**：首次领取挂机奖励时，直接获得 8 小时的奖励（新手福利）
2. **时间要求**：非首次领取必须满足至少 1 小时才能领取
3. **时间上限**：单次最多领取 8 小时的奖励（超过 8 小时只能领取 8 小时）
4. **奖励来源**：
   - 金币：`OnHookRewardCoin` × 小时数
   - 道具：`OnHookRewardGroupID` 对应的奖励 × 小时数
5. **时间累积**：不满 1 小时的分钟数会累积到下次领取

#### 时间计算示例

**示例 1：首次领取（新手福利）**
- 上次领取时间：无
- 当前时间：任意时间
- 领取奖励：8 小时奖励（直接发放）
- 新的领取时间：当前时间
- 说明：首次领取不需要等待，直接获得 8 小时奖励

**示例 2：标准领取**
- 上次领取时间：10:00
- 当前时间：13:00
- 间隔：3 小时
- 领取奖励：3 小时奖励
- 新的领取时间：13:00
- 剩余累积：0 分钟

**示例 3：有余数的领取**
- 上次领取时间：10:00
- 当前时间：13:45
- 间隔：3 小时 45 分钟
- 领取奖励：3 小时奖励
- 新的领取时间：13:00（而非 13:45）
- 剩余累积：45 分钟（累积到下次）

**示例 4：不满 1 小时**
- 上次领取时间：10:00
- 当前时间：10:30
- 间隔：30 分钟
- 提示：还需 30 分钟

**示例 5：累积效果**
- 第一次：10:00 领取，当前 13:45，领取 3 小时奖励，新时间 13:00
- 第二次：当前 14:30，距离 13:00 是 1.5 小时，领取 1 小时奖励，新时间 14:00
- 剩余：30 分钟累积到下次

**示例 6：超过上限**
- 上次领取时间：昨天 10:00
- 当前时间：今天 12:00（间隔 26 小时）
- 领取奖励：8 小时奖励（超过上限，只领取 8 小时）
- 新的领取时间：昨天 18:00（10:00 + 8小时）
- 剩余累积：18 小时（今天 12:00 - 昨天 18:00）

#### 关键特性

1. **新手福利**：首次领取直接获得 8 小时奖励，无需等待
2. **精确计算**：只领取完整小时数的奖励
3. **时间不浪费**：不满 1 小时的时间自动累积到下一次
4. **时间上限**：单次最多领取 8 小时奖励，超过部分自动累积到下次
5. **关卡绑定**：奖励根据当前最大关卡的配置发放
6. **自动累积**：离线时间也会累积，可以多次领取

### 客户端集成示例

#### C# 调用示例

```csharp
// 领取挂机奖励
var request = new ClaimOnHookRewardRequest();

var response = await client.ClaimOnHookRewardAsync(request);
if (response.Code == 0)
{
    Debug.Log($"领取成功！{response.Hours}小时，获得金币：{response.Coin}");
}
else if (response.Code == 8)
{
    Debug.Log($"挂机时间不足：{response.Msg}");
}

// 计算可领取时间
var account = await client.GetAccountAsync();
var metadata = JsonUtility.FromJson<UserMeta>(account.User.Metadata);

if (!string.IsNullOrEmpty(metadata.level.last_get_on_hook_timestamp))
{
    DateTime lastGetTime = DateTime.Parse(metadata.level.last_get_on_hook_timestamp);
    DateTime now = DateTime.Now;
    TimeSpan elapsed = now - lastGetTime;
    
    int hours = (int)elapsed.TotalHours;
    int remainingMinutes = 60 - (int)(elapsed.TotalMinutes % 60);
    
    if (hours >= 1)
    {
        Debug.Log($"可领取 {hours} 小时的挂机奖励");
    }
    else
    {
        Debug.Log($"还需要 {remainingMinutes} 分钟才能领取");
    }
}
```

### 配置说明

挂机奖励配置在 `TplLevelInfo.json` 中：

```json
{
  "L1001": {
    "onHookRewardCoin": 16,           // 每小时金币奖励
    "onHookRewardGroupId": "L2002"    // 每小时道具奖励组ID
  }
}
```

- `onHookRewardCoin`: 每小时获得的金币数量
- `onHookRewardGroupId`: 道具奖励的配置 ID（在 TplReward.json 中配置）

---

## 测试建议

### 扫荡奖励测试
1. 测试第一阶段扫荡（3次）
2. 测试第二阶段扫荡，观看广告的情况
3. 测试第二阶段扫荡，不观看广告的情况（使用广告券）
4. 测试体力不足的情况
5. 测试广告券不足的情况
6. 测试每日次数用完的情况
7. 测试跨天重置功能

### 挂机奖励测试
1. 测试首次初始化挂机时间
2. 测试不满 1 小时无法领取
3. 测试恰好 1 小时领取
4. 测试多小时（如 3 小时）领取
5. 测试有余数（如 3.5 小时）的领取，验证时间累积
6. 测试连续领取多次，验证累积逻辑
7. 测试关卡切换后的挂机奖励

