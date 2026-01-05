# 水晶装备洗炼日志说明

## 日志输出顺序

洗炼一次水晶装备会输出以下日志：

### 1. 开始洗炼日志

```
INFO  开始洗炼水晶装备
  user_id: "uuid-123"
  equipment_id: "equipment-uuid-456"
  equipment_tpl_id: "CEQ1001001"
  equipment_quality: 8
  locked_rare_count: 1
```

**说明**：
- `user_id`: 玩家ID
- `equipment_id`: 装备实例ID
- `equipment_tpl_id`: 装备模板ID
- `equipment_quality`: 装备品质（1=白, 2=绿, 4=蓝, 8=紫, 16=橙）
- `locked_rare_count`: 锁定的稀有词条数量

---

### 2. 稀有词条洗炼配置日志

```
INFO  稀有词条洗炼配置
  rare_entry_num: 2
  locked_rare_count: 1
  remaining_slots: 1
  rare_entry_weight: 5000
  probability_percent: 50.0
```

**说明**：
- `rare_entry_num`: 稀有词条最大配额
- `locked_rare_count`: 已锁定的稀有词条数
- `remaining_slots`: 剩余可生成的稀有词条配额
- `rare_entry_weight`: 稀有词条触发权重（0-10000）
- `probability_percent`: 触发概率百分比（weight/100）

**如何验证概率**：
- 如果 `rare_entry_weight = 5000`，则 `probability_percent = 50.0%`
- 如果 `rare_entry_weight = 3000`，则 `probability_percent = 30.0%`
- 如果 `rare_entry_weight = 10000`，则 `probability_percent = 100.0%`（必定触发）

---

### 3. 稀有词条独立槽位判断日志

#### 3A. 开始槽位判断

```
INFO  开始独立判断每个稀有词条槽位
  remaining_slots: 2
  weight_per_slot: 2000
  probability_per_slot_percent: 20.0
```

**说明**：
- `remaining_slots`: 剩余可用槽位数
- `weight_per_slot`: 每个槽位的触发权重
- `probability_per_slot_percent`: 每个槽位的触发概率

#### 3B. 单个槽位触发成功

```
INFO  稀有词条槽位触发成功
  slot_index: 1
  random_value: 1500
  threshold: 2000
  triggered: true
```

**说明**：
- `slot_index`: 当前判断的槽位索引（从1开始）
- `random_value < threshold`: 该槽位触发成功

#### 3C. 单个槽位未触发

```
INFO  稀有词条槽位未触发
  slot_index: 2
  random_value: 8500
  threshold: 2000
  triggered: false
```

**说明**：
- `random_value >= threshold`: 该槽位未触发

#### 3D. 槽位判断完成汇总

```
INFO  稀有词条槽位判断完成
  total_slots: 2
  triggered_slots: 1
  failed_slots: 1
```

**说明**：
- `total_slots`: 总共判断的槽位数
- `triggered_slots`: 触发成功的槽位数
- `failed_slots`: 未触发的槽位数

#### 3E. 配额已满

```
INFO  稀有词条配额已满，跳过概率判断
  locked_rare_count: 2
  rare_entry_num: 2
```

**说明**：
- 锁定的稀有词条数已达到最大配额
- 跳过概率判断，不再生成新稀有词条

---

### 4. 添加锁定的稀有词条日志（如果有锁定）

```
DEBUG 添加锁定的稀有词条
  locked_rare_count: 1

DEBUG 锁定的稀有词条详情
  affix_id: "AF10013"
  value: 5
```

**说明**：
- 显示每个锁定的稀有词条的ID和数值
- 这些词条会被直接保留到新生成的词条列表中

---

### 5. 生成新稀有词条日志（如果触发成功）

```
INFO  生成新的稀有词条
  affix_id: "AF10014"
  min_value: 1
  max_value: 10
  random_value: 7
  max_ratio: 100
  weight: 125
```

**说明**：
- `affix_id`: 新生成的稀有词条ID
- `min_value`: 词条最小值
- `max_value`: 词条最大值（已应用maxRatio）
- `random_value`: 实际生成的随机值
- `max_ratio`: 数值上限百分比
- `weight`: 词条权重（影响被选中的概率）

---

### 6. 词条生成完成汇总日志

```
INFO  词条生成完成
  total_affixes: 5
  normal_count: 4
  locked_rare_count: 1
  new_rare_count: 1
  total_rare_count: 2
```

**说明**：
- `total_affixes`: 生成的总词条数
- `normal_count`: 普通词条数
- `locked_rare_count`: 锁定保留的稀有词条数
- `new_rare_count`: 新生成的稀有词条数
- `total_rare_count`: 稀有词条总数（锁定+新生成）

---

### 7. 洗炼成功日志

```
INFO  洗炼水晶装备成功
  user_id: "uuid-123"
  equipment_id: "equipment-uuid-456"
  total_affix_count: 5
  normal_affix_count: 4
  rare_affix_count: 2
  locked_rare_count: 1
  new_rare_count: 1
```

**说明**：
- `total_affix_count`: 备用词条总数
- `normal_affix_count`: 普通词条数（统计PendingAffixes中的普通词条）
- `rare_affix_count`: 稀有词条总数（统计PendingAffixes中的稀有词条）
- `locked_rare_count`: 其中锁定保留的数量
- `new_rare_count`: 其中新生成的数量

---

## 概率验证方法

### 方法1：通过日志验证单次洗炼

查看日志中的关键字段：

1. **查看配置概率**：
   ```
   weight_per_slot: 2000
   probability_per_slot_percent: 20.0
   ```
   确认每个槽位的概率设置正确（20%）

2. **查看每个槽位的判断结果**：
   ```
   槽位1: random_value: 1500, threshold: 2000, triggered: true  ✓
   槽位2: random_value: 8500, threshold: 2000, triggered: false ✗
   ```
   验证：
   - 槽位1：1500 < 2000 ✓ 触发成功
   - 槽位2：8500 > 2000 ✗ 未触发

3. **查看汇总结果**：
   ```
   total_slots: 2
   triggered_slots: 1
   failed_slots: 1
   ```
   2个槽位中，1个成功，1个失败

### 方法2：统计多次洗炼验证单槽位概率

使用日志分析工具统计槽位触发情况：

```bash
# 统计单个槽位触发成功的次数
grep "稀有词条槽位触发成功" logs.txt | wc -l

# 统计单个槽位未触发的次数
grep "稀有词条槽位未触发" logs.txt | wc -l

# 计算实际单槽位触发率
单槽位触发率 = 触发成功次数 / (触发成功次数 + 未触发次数)
```

**示例（单槽位20%概率）**：
- 配置概率：20%（rare_entry_weight=2000）
- 测试500个槽位（可能来自250次洗炼，每次2个槽位）
- 槽位触发成功：98次
- 槽位未触发：402次
- 实际单槽位触发率：98/500 = 19.6% ≈ 20% ✓

**验证多槽位分布**：
假设2个槽位，每个20%概率，测试100次洗炼：
- 0个稀有：约64次（理论64%）
- 1个稀有：约32次（理论32%）
- 2个稀有：约4次（理论4%）

### 方法3：验证随机值分布

检查 `random_value` 是否在 0-9999 范围内均匀分布：

```bash
# 提取所有random_value
grep "random_value" logs.txt | awk '{print $2}' | sort -n

# 统计分布情况
# 0-2499: 应该约占25%
# 2500-4999: 应该约占25%
# 5000-7499: 应该约占25%
# 7500-9999: 应该约占25%
```

---

## 常见问题排查

### Q1: 概率显示正确，但从不触发稀有词条

**检查**：
```
INFO  稀有词条配额已满，跳过概率判断
  locked_rare_count: 2
  rare_entry_num: 2
```

**原因**：锁定的稀有词条数已达到最大配额，无法生成新的。

**解决**：解锁一些稀有词条，或者不锁定。

---

### Q2: 有槽位触发成功但最终 new_rare_count=0

**检查**：
```
INFO  稀有词条槽位触发成功 (某个槽位)
INFO  稀有词条槽位判断完成
  triggered_slots: 0
```

**不可能情况**：如果有任何槽位 `triggered: true`，则 `triggered_slots` 必定 >= 1。

如果出现矛盾，说明代码有bug。

---

### Q3: 想要100%触发稀有词条

**配置**：设置 `rare_entry_weight = 10000`

**日志验证**：
```
rare_entry_weight: 10000
probability_percent: 100.0
```

所有 `random_value` 都会 < 10000，必定触发。

---

## 完整日志示例

### 示例1：2个槽位，1个触发成功

```
2025-01-05 10:15:23 INFO  开始洗炼水晶装备
  user_id: "550e8400-e29b-41d4-a716-446655440000"
  equipment_id: "7c9e6679-7425-40de-944b-e07fc1249cd0"
  equipment_tpl_id: "CEQ1004001"
  equipment_quality: 8
  locked_rare_count: 0

2025-01-05 10:15:23 INFO  稀有词条洗炼配置
  rare_entry_num: 2
  locked_rare_count: 0
  remaining_slots: 2
  rare_entry_weight: 2000
  probability_percent: 20.0

2025-01-05 10:15:23 INFO  开始独立判断每个稀有词条槽位
  remaining_slots: 2
  weight_per_slot: 2000
  probability_per_slot_percent: 20.0

2025-01-05 10:15:23 INFO  稀有词条槽位触发成功
  slot_index: 1
  random_value: 1500
  threshold: 2000
  triggered: true

2025-01-05 10:15:23 INFO  稀有词条槽位未触发
  slot_index: 2
  random_value: 8500
  threshold: 2000
  triggered: false

2025-01-05 10:15:23 INFO  稀有词条槽位判断完成
  total_slots: 2
  triggered_slots: 1
  failed_slots: 1

2025-01-05 10:15:23 INFO  生成新的稀有词条
  affix_id: "AF10015"
  min_value: 1
  max_value: 5
  random_value: 4
  max_ratio: 100
  weight: 125

2025-01-05 10:15:23 INFO  词条生成完成
  total_affixes: 5
  normal_count: 4
  locked_rare_count: 0
  new_rare_count: 1
  total_rare_count: 1

2025-01-05 10:15:23 INFO  洗炼水晶装备成功
  user_id: "550e8400-e29b-41d4-a716-446655440000"
  equipment_id: "7c9e6679-7425-40de-944b-e07fc1249cd0"
  total_affix_count: 5
  normal_affix_count: 4
  rare_affix_count: 1
  locked_rare_count: 0
  new_rare_count: 1
```

**分析**：
- 每个槽位20%概率 ✓
- 槽位1：1500 < 2000，触发成功 ✓
- 槽位2：8500 > 2000，未触发 ✗
- 最终生成1个稀有词条 ✓
- 总共生成4个普通+1个稀有=5个词条 ✓

