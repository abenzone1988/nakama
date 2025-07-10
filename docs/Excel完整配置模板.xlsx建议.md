# Excel完整配置模板建议

## 基于您当前配置的改进建议

### 当前配置分析
您的配置已经包含了核心字段，建议添加以下字段以获得完整功能：

## 完整的Excel表格列配置

| 列 | 字段名 | 类型 | 必需 | 您的示例值 | 建议值 |
|----|--------|------|------|------------|--------|
| A | id | string | ✅ | C001 | ✅ 正确 |
| B | name | string | ✅ | 夏季挑战赛01 | ✅ 正确 |
| C | activity_id | string | ✅ | A1001 | ✅ 正确（驼峰命名法） |
| D | start_date | string | ✅ | 7/9/2025 | 建议改为 `2025-07-09` |
| E | start_time | string | ✅ | 10:00 | ✅ 正确 |
| F | end_date | string | ✅ | 7/10/2025 | 建议改为 `2025-07-10` |
| G | end_time | string | ✅ | 22:00 | ✅ 正确 |
| H | max_part | int | ✅ | 20 | ✅ 正确 |
| I | status | int | ✅ | 1 | ✅ 正确 |
| J | description | string | 推荐 | - | 夏季限时挑战活动 |
| K | duration_seconds | int | 推荐 | - | 129600 (36小时) |
| L | category | int | 可选 | - | 1 |
| M | operator | string | 可选 | - | best |
| N | sort_order | int | 可选 | - | 1 |
| O | reward_config | string | 推荐 | - | JSON格式奖励 |
| P | requirements | string | 可选 | - | JSON格式要求 |

## 建议的完整Excel配置

```
id          name            activity_id   start_date    start_time   end_date      end_time   max_part   status   description              duration_seconds   category   operator   sort_order   reward_config                                                                      requirements
C001        夏季挑战赛01    A1001        2025-07-09   10:00       2025-07-10   22:00     20        1        夏季限时挑战活动01       129600            1          best       1           {"first":{"gem":1000,"coin":5000},"second":{"gem":500,"coin":3000}}              {"level":10,"vip":false}
C002        夏季挑战赛02    L10102       2025-07-12   10:00       2025-07-13   22:00     30        1        夏季限时挑战活动02       129600            1          best       1           {"first":{"gem":1500,"coin":7500},"second":{"gem":750,"coin":4500}}              {"level":15,"vip":false}
C003        夏季挑战赛03    L10051       2025-07-14   10:00       2025-07-16   22:00     40        1        夏季限时挑战活动03       216000            1          best       1           {"first":{"gem":2000,"coin":10000},"second":{"gem":1000,"coin":6000}}            {"level":20,"vip":true}
```

## 字段格式建议

### 1. 日期格式统一
- **当前**: `7/9/2025`
- **建议**: `2025-07-09` (ISO 8601标准)
- **原因**: 避免地区差异，确保解析一致性

### 2. 字段名统一
- **当前**: `activity_id`
- **建议**: `activity_id` (下划线分隔)
- **原因**: 与代码中的JSON标签保持一致

### 3. 添加持续时间
- **字段**: `duration_seconds`
- **C001**: 129600 (36小时)
- **C002**: 129600 (36小时) 
- **C003**: 216000 (60小时，跨周末)

### 4. 奖励配置示例
```json
{
  "first": {"gem": 1000, "coin": 5000, "items": ["legendary_sword"]},
  "second": {"gem": 500, "coin": 3000, "items": ["rare_shield"]},
  "third": {"gem": 200, "coin": 1000},
  "participation": {"gem": 50, "coin": 200}
}
```

### 5. 参与要求示例
```json
{
  "level": 10,
  "vip": false,
  "items": ["access_key"],
  "achievements": ["summer_2024"]
}
```

## 时间计算示例

### C001 挑战赛
- **开始**: 2025-07-09 10:00
- **结束**: 2025-07-10 22:00
- **持续**: 36小时 = 129600秒

### C003 挑战赛（跨周末）
- **开始**: 2025-07-14 10:00
- **结束**: 2025-07-16 22:00  
- **持续**: 60小时 = 216000秒

## 常用时间配置

| 持续时间 | 秒数 | 适用场景 |
|----------|------|----------|
| 12小时 | 43200 | 半日挑战 |
| 24小时 | 86400 | 每日挑战 |
| 36小时 | 129600 | 周末挑战 |
| 72小时 | 259200 | 三日挑战 |
| 7天 | 604800 | 周活动 |

## 操作符选择建议

- **best**: 保留最佳成绩（推荐用于竞技类）
- **set**: 覆盖成绩（用于任务完成类）
- **incr**: 累积分数（用于收集类活动）
- **decr**: 减少分数（用于消耗类挑战）

## 分类建议

- **1**: 竞技类挑战赛
- **2**: 收集类挑战赛  
- **3**: 任务类挑战赛
- **4**: 社交类挑战赛 