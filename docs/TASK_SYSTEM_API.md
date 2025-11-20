# 任务系统接口文档

## 功能概述

任务系统包含每日任务和活跃度奖励两部分：
1. **每日任务**：完成任务领取奖励并获得活跃度
2. **活跃度奖励**：累积活跃度后领取额外奖励
3. **每日重置**：每天00:00重置，可以重新领取所有奖励

## 接口信息

### 1. 获取任务状态

**方法名称**: `GetTask`

**请求消息**: `GetTaskRequest`

**响应消息**: `GetTaskResponse`

### 2. 领取任务奖励

**方法名称**: `ClaimTaskReward`

**请求消息**: `ClaimTaskRewardRequest`

**响应消息**: `ClaimTaskRewardResponse`

### 3. 领取活跃度奖励

**方法名称**: `ClaimLivenessReward`

**请求消息**: `ClaimLivenessRewardRequest`

**响应消息**: `ClaimLivenessRewardResponse`

## 请求参数

### GetTaskRequest

```protobuf
message GetTaskRequest{
  // 无参数
}
```

### ClaimTaskRewardRequest

```protobuf
message ClaimTaskRewardRequest{
  string task_id = 1; // 任务ID
}
```

### ClaimLivenessRewardRequest

```protobuf
message ClaimLivenessRewardRequest{
  string reward_id = 1; // 活跃度奖励ID
}
```

## 响应参数

### GetTaskResponse

```protobuf
message GetTaskResponse{
  int32 code = 1;
  string msg = 2;
  int32 current_liveness = 3;              // 当前活跃度
  repeated string claimed_tasks = 4;       // 已领取的任务ID列表
  repeated string claimed_liveness_rewards = 5; // 已领取的活跃度奖励ID列表
}
```

### ClaimTaskRewardResponse

```protobuf
message ClaimTaskRewardResponse{
  int32 code = 1;
  string msg = 2;
  int32 liveness = 3;                  // 获得的活跃度
  Reward reward = 4;                   // 任务奖励
  Wallet wallet_updated = 5;           // 钱包更新结果
  repeated Item inventory_updated = 6; // 背包更新结果
}
```

### ClaimLivenessRewardResponse

```protobuf
message ClaimLivenessRewardResponse{
  int32 code = 1;
  string msg = 2;
  Reward reward = 3;                   // 奖励数据
  Wallet wallet_updated = 4;           // 钱包更新结果
  repeated Item inventory_updated = 5; // 背包更新结果
}
```

## 状态码说明

### 获取任务状态状态码

| Code | 说明 |
|------|------|
| 0 | 获取成功 |

### 任务奖励状态码

| Code | 说明 |
|------|------|
| 0 | 领取成功 |
| 1 | 任务ID不能为空 |
| 2 | 任务不存在 |
| 3 | 任务奖励已经领取过了 |
| 4 | 发放奖励失败 |
| 5 | 保存数据失败 |

### 活跃度奖励状态码

| Code | 说明 |
|------|------|
| 0 | 领取成功 |
| 1 | 奖励ID不能为空 |
| 2 | 活跃度奖励不存在 |
| 3 | 获取任务数据失败 |
| 4 | 活跃度不足（每日重置后） |
| 5 | 活跃度不足 |
| 6 | 活跃度奖励已经领取过了 |
| 7 | 发放奖励失败 |
| 8 | 保存数据失败 |

## 系统机制

### 每日任务

1. **任务配置**：在 `TplTasks.json` 中配置
   - `ID`: 任务ID
   - `Name`: 任务名称
   - `Brief`: 任务描述
   - `TargetType`: 目标类型（如 "PlayMainLevel"）
   - `TargetValue`: 目标数值
   - `Liveness`: 完成任务获得的活跃度
   - `Reward`: 任务奖励ID
   - `TaskType`: 任务类型（2表示每日任务）

2. **领取流程**：
   - 玩家完成任务
   - 调用 `ClaimTaskReward` 领取奖励
   - 获得任务奖励 + 活跃度
   - 活跃度累加到当前总活跃度

3. **每日重置**：
   - 每天00:00重置所有任务
   - 可以重新领取所有任务奖励
   - 活跃度清零重新累积

### 活跃度奖励

1. **奖励配置**：在 `TplProgressReward.json` 中配置
   - `ID`: 奖励ID
   - `Type`: 奖励类型
   - `NeedValue`: 需要的活跃度数值
   - `Reward`: 奖励内容ID

2. **领取条件**：
   - 当前活跃度 >= 需要的活跃度
   - 该奖励未被领取过

3. **每日重置**：
   - 每天00:00重置所有活跃度奖励
   - 可以重新领取所有活跃度奖励

## 数据存储

任务数据存储在独立的 Storage 中：

```json
{
  "date_time": "2025-11-20T12:00:00Z",
  "claimed_tasks": ["20001", "20002", "20003"],
  "claimed_liveness_rewards": ["LivenessReward001", "LivenessReward002"],
  "current_liveness": 60
}
```

- **Collection**: `task`
- **Key**: `data`
- **字段说明**：
  - `date_time`: 最后更新时间，用于判断是否需要每日重置
  - `claimed_tasks`: 已领取的任务ID列表
  - `claimed_liveness_rewards`: 已领取的活跃度奖励ID列表
  - `current_liveness`: 当前活跃度总和

## 配置示例

### TplTasks.json

```json
{
  "20001": {
    "id": "20001",
    "name": "挑战主线关卡",
    "brief": "挑战{0}次主线关卡",
    "TargetType": "PlayMainLevel",
    "TargetValue": 1,
    "Liveness": 20,
    "reward": "Gem005",
    "taskType": 2
  },
  "20002": {
    "id": "20002",
    "name": "击杀怪物",
    "brief": "击杀{0}只怪物",
    "TargetType": "Kill_Monster",
    "TargetValue": 10000,
    "Liveness": 20,
    "reward": "Gem005",
    "taskType": 2
  }
}
```

### TplProgressReward.json

```json
{
  "LivenessReward001": {
    "id": "LivenessReward001",
    "type": 1,
    "needValue": 20,
    "reward": "Reward001"
  },
  "LivenessReward002": {
    "id": "LivenessReward002",
    "type": 1,
    "needValue": 60,
    "reward": "Reward002"
  },
  "LivenessReward003": {
    "id": "LivenessReward003",
    "type": 1,
    "needValue": 100,
    "reward": "Reward003"
  }
}
```

## 客户端集成示例

### C# 调用示例

```csharp
// 获取任务状态
public async Task<GetTaskResponse> GetTaskStatus()
{
    var request = new GetTaskRequest();
    var response = await client.GetTaskAsync(request);
    
    if (response.Code == 0)
    {
        Debug.Log($"当前活跃度：{response.CurrentLiveness}");
        Debug.Log($"已领取任务数：{response.ClaimedTasks.Count}");
        Debug.Log($"已领取活跃度奖励数：{response.ClaimedLivenessRewards.Count}");
        
        // 更新UI显示
        UpdateTaskUI(response);
        return response;
    }
    
    return null;
}

// 检查任务是否已领取
public bool IsTaskClaimed(string taskId, GetTaskResponse taskStatus)
{
    return taskStatus.ClaimedTasks.Contains(taskId);
}

// 检查活跃度奖励是否已领取
public bool IsLivenessRewardClaimed(string rewardId, GetTaskResponse taskStatus)
{
    return taskStatus.ClaimedLivenessRewards.Contains(rewardId);
}

// 检查活跃度奖励是否可领取
public bool CanClaimLivenessReward(string rewardId, int needValue, GetTaskResponse taskStatus)
{
    // 1. 检查活跃度是否足够
    if (taskStatus.CurrentLiveness < needValue)
    {
        return false;
    }
    
    // 2. 检查是否已领取
    if (taskStatus.ClaimedLivenessRewards.Contains(rewardId))
    {
        return false;
    }
    
    return true;
}

// 领取任务奖励
public async Task ClaimTask(string taskId)
{
    var request = new ClaimTaskRewardRequest
    {
        TaskId = taskId
    };

    var response = await client.ClaimTaskRewardAsync(request);
    if (response.Code == 0)
    {
        Debug.Log($"任务完成！获得活跃度：{response.Liveness}");
        UpdateLiveness(response.Liveness);
    }
    else if (response.Code == 3)
    {
        Debug.Log("该任务今天已经领取过了");
    }
}

// 领取活跃度奖励
public async Task ClaimLivenessReward(string rewardId)
{
    var request = new ClaimLivenessRewardRequest
    {
        RewardId = rewardId
    };

    var response = await client.ClaimLivenessRewardAsync(request);
    if (response.Code == 0)
    {
        Debug.Log("活跃度奖励领取成功！");
    }
    else if (response.Code == 5)
    {
        Debug.Log("活跃度不足");
    }
    else if (response.Code == 6)
    {
        Debug.Log("该奖励今天已经领取过了");
    }
}

```

## 使用场景

### 场景1：进入任务界面

```
1. 玩家打开任务界面
2. 调用 GetTask() 获取任务状态
3. 根据返回的数据更新UI：
   - 显示当前活跃度
   - 标记已领取的任务
   - 标记已领取的活跃度奖励
   - 高亮可领取的活跃度奖励
```

### 场景2：完成每日任务

```
1. 玩家完成任务（如：挑战3次主线关卡）
2. 调用 ClaimTaskReward("20001")
3. 获得任务奖励 + 20活跃度
4. 调用 GetTask() 刷新任务状态
5. 更新任务列表UI，标记已完成
6. 更新活跃度进度条
```

### 场景3：领取活跃度奖励

```
1. 玩家完成多个任务，活跃度达到60
2. 调用 GetTask() 获取当前状态
3. 根据活跃度判断，20和60的奖励可领取
4. 玩家点击领取20活跃度奖励
5. 调用 ClaimLivenessReward("LivenessReward001")
6. 获得奖励
7. 调用 GetTask() 刷新状态
8. 继续领取60活跃度奖励
```

### 场景4：每日重置

```
1. 第一天玩家完成了3个任务，活跃度60
2. 领取了20和60的活跃度奖励
3. 第二天00:00，数据自动重置
4. 玩家上线打开任务界面
5. 调用 GetTask()，服务器检测到跨天自动重置数据
6. 返回重置后的状态：
   - current_liveness: 0
   - claimed_tasks: []
   - claimed_liveness_rewards: []
7. UI显示所有任务和活跃度奖励重新可领取
```

## 注意事项

1. **任务进度追踪**：当前版本暂未实现任务进度追踪，建议前端记录或后续扩展
2. **每日重置时间**：使用UTC时间00:00重置，建议转换为本地时区显示
3. **活跃度累加**：活跃度在领取任务奖励时累加，不是完成任务时
4. **奖励顺序**：建议按活跃度值从小到大顺序领取奖励
5. **数据同步**：每次领取后及时更新本地缓存的任务数据

## 测试建议

### 任务奖励测试
1. 测试领取单个任务奖励
2. 测试重复领取同一任务（应该失败）
3. 测试领取不存在的任务（应该失败）
4. 测试活跃度累加是否正确

### 活跃度奖励测试
1. 测试活跃度不足时领取（应该失败）
2. 测试活跃度足够时领取
3. 测试重复领取同一奖励（应该失败）
4. 测试跨多个活跃度档位领取

### 每日重置测试
1. 修改服务器时间到第二天，验证重置
2. 验证重置后任务可以重新领取
3. 验证重置后活跃度归零
4. 验证重置后活跃度奖励可以重新领取

