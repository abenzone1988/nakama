# 购买系统用户映射配置指南

## 🎯 问题说明

第三方支付平台的通知中包含 `uid` 字段，我们需要将这个 `uid` 映射到 Nakama 的 `user_id` (UUID)。

```
第三方平台                     Nakama 数据库
┌──────────────┐              ┌──────────────┐
│ uid: "user123"│    映射?     │ id: UUID     │
└──────────────┘   ───────▶   └──────────────┘
```

## 📊 Users 表结构

根据 `core_user.go`，Nakama 的 `users` 表包含以下字段:

```sql
CREATE TABLE users (
    id               UUID PRIMARY KEY,        -- 主键，Nakama 内部 UUID
    username         TEXT UNIQUE,             -- 用户名
    display_name     TEXT,                    -- 显示名称
    avatar_url       TEXT,
    lang_tag         TEXT,
    location         TEXT,
    timezone         TEXT,
    metadata         JSONB,
    
    -- 第三方平台 ID
    apple_id                TEXT,            -- Apple ID
    facebook_id             TEXT,            -- Facebook ID
    facebook_instant_game_id TEXT,           -- Facebook Instant Game ID
    google_id               TEXT,            -- Google ID
    gamecenter_id           TEXT,            -- GameCenter ID
    steam_id                TEXT,            -- Steam ID
    custom_id               TEXT,            -- 自定义ID (如果有)
    
    edge_count       INT,
    create_time      TIMESTAMPTZ,
    update_time      TIMESTAMPTZ
);
```

## 🔧 配置方案

### **方案1: 使用 username 字段** (推荐 - 最简单)

如果你的第三方平台 `uid` 就是用户注册时使用的用户名:

```go
// game_purchase.go 第 314 行
query := "SELECT id FROM users WHERE username = $1 LIMIT 1"
```

**适用场景**:
- 用户用手机号/用户名注册
- 第三方平台的 `uid` 就是这个用户名

**示例**:
```
第三方通知: uid = "player_12345"
数据库查询: SELECT id FROM users WHERE username = 'player_12345'
```

---

### **方案2: 使用 custom_id 字段** (推荐 - 最灵活)

如果你为每个用户存储了第三方平台的专属ID:

```go
// game_purchase.go 第 314 行
query := "SELECT id FROM users WHERE custom_id = $1 LIMIT 1"
```

**注意**: 需要确保 `users` 表有 `custom_id` 字段！

**添加字段的 SQL**:
```sql
-- 如果表中没有 custom_id 字段，需要先添加
ALTER TABLE users ADD COLUMN IF NOT EXISTS custom_id TEXT;
CREATE INDEX IF NOT EXISTS idx_users_custom_id ON users(custom_id);
```

**适用场景**:
- 第三方平台有自己的用户系统
- 用户在第三方平台的 ID 与 Nakama 的 username 不同

---

### **方案3: 使用现有的第三方平台 ID 字段**

如果你的支付来自特定平台，可以使用对应的字段:

#### 📱 Apple 平台
```go
query := "SELECT id FROM users WHERE apple_id = $1 LIMIT 1"
```

#### 👤 Facebook 平台
```go
query := "SELECT id FROM users WHERE facebook_id = $1 LIMIT 1"
```

#### 🎮 Google 平台
```go
query := "SELECT id FROM users WHERE google_id = $1 LIMIT 1"
```

**适用场景**:
- 支付渠道与登录渠道一致
- 例如: Facebook 登录 + Facebook 支付

---

### **方案4: 多字段查询** (推荐 - 最灵活，但性能稍低)

同时支持多种ID类型:

```go
query := `
SELECT id FROM users 
WHERE username = $1 
   OR custom_id = $1 
   OR apple_id = $1 
   OR facebook_id = $1 
   OR google_id = $1
LIMIT 1
`
```

**优点**: 一个函数支持所有场景
**缺点**: 需要扫描多个字段，性能略低

**性能优化**: 添加索引
```sql
CREATE INDEX IF NOT EXISTS idx_users_custom_id ON users(custom_id);
CREATE INDEX IF NOT EXISTS idx_users_apple_id ON users(apple_id);
CREATE INDEX IF NOT EXISTS idx_users_facebook_id ON users(facebook_id);
CREATE INDEX IF NOT EXISTS idx_users_google_id ON users(google_id);
```

---

### **方案5: 自动创建用户** (可选)

如果允许首次支付时自动注册用户:

```go
err := s.db.QueryRowContext(ctx, query, customID).Scan(&userID)

if err == sql.ErrNoRows {
    // 用户不存在，自动创建
    s.logger.Info("用户不存在，自动创建", zap.String("custom_id", customID))
    return s.createUserWithCustomID(ctx, customID)
}
```

**适用场景**:
- 游客模式支付
- 简化注册流程

**安全建议**:
- 记录详细日志
- 设置默认用户名: `user_<timestamp>_<random>`
- 限制自动创建的频率 (防止恶意刷单)

---

## 🛠️ 实际配置步骤

### Step 1: 确定 uid 的来源

首先确认第三方平台的 `uid` 是什么:

```powershell
# 查看实际的通知数据
Write-Host "第三方通知中的 uid: $uid"

# 在数据库中查找
psql -d nakama -c "SELECT id, username, custom_id FROM users LIMIT 5;"
```

### Step 2: 选择映射方案

根据你的用户系统，在 `game_purchase.go` 第 314 行修改查询:

```go
// 取消对应方案的注释
// query := "SELECT id FROM users WHERE username = $1 LIMIT 1"        // 方案1
// query := "SELECT id FROM users WHERE custom_id = $1 LIMIT 1"       // 方案2
// query := "SELECT id FROM users WHERE facebook_id = $1 LIMIT 1"     // 方案3
```

### Step 3: 测试验证

```powershell
# 使用测试脚本验证
.\test\test_purchase_notify.ps1

# 或手动测试
curl -X POST http://localhost:7350/v2/tiktok/purchase/notify \
  -d "uid=test_user_123" \
  -d "product_id=coin_pack" \
  # ... 其他参数
```

### Step 4: 查看日志

```bash
# 检查日志，确认用户查找是否成功
tail -f nakama.log | grep "findUserByCustomID"
```

---

## 🔍 常见问题排查

### 问题1: 用户不存在

**症状**: 日志显示 "用户不存在: xxx"

**原因**:
- `uid` 与数据库字段不匹配
- 用户还未注册

**解决**:
```sql
-- 1. 检查数据库中有哪些用户
SELECT id, username, custom_id FROM users LIMIT 10;

-- 2. 检查特定用户
SELECT id, username, custom_id, facebook_id, google_id 
FROM users 
WHERE username = 'test_user' OR custom_id = 'test_user';

-- 3. 手动插入测试用户
INSERT INTO users (id, username, create_time, update_time)
VALUES (
    'a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d'::UUID,
    'test_user_123',
    now(),
    now()
);
```

### 问题2: 性能慢

**症状**: 查询用户耗时 > 100ms

**原因**: 缺少索引

**解决**:
```sql
-- 添加索引
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_custom_id ON users(custom_id);

-- 查看索引使用情况
EXPLAIN ANALYZE 
SELECT id FROM users WHERE username = 'test_user';
```

### 问题3: 重复用户

**症状**: 同一个 `uid` 在数据库中有多条记录

**原因**: 
- 字段没有唯一约束
- 用户多次注册

**解决**:
```sql
-- 1. 检查重复
SELECT username, COUNT(*) 
FROM users 
WHERE username IS NOT NULL 
GROUP BY username 
HAVING COUNT(*) > 1;

-- 2. 添加唯一约束
ALTER TABLE users ADD CONSTRAINT unique_custom_id UNIQUE (custom_id);
```

---

## 📊 数据流示意图

```
┌─────────────────────────────────────────────────────────────┐
│  第三方支付通知                                              │
│  {                                                           │
│    "uid": "player_12345",        ← 第三方平台的用户ID        │
│    "order_id": "ORD001",                                     │
│    "product_id": "coin_pack",                                │
│    ...                                                       │
│  }                                                           │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  game_purchase.go::findUserByCustomID("player_12345")       │
│                                                             │
│  根据配置的查询方式:                                           │
│  ├─ 方案1: WHERE username = 'player_12345'                  │
│  ├─ 方案2: WHERE custom_id = 'player_12345'                 │
│  ├─ 方案3: WHERE facebook_id = 'player_12345'               │
│  └─ 方案4: WHERE username = ... OR custom_id = ...          │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  数据库查询结果                                              │
│  ├─ 找到: 返回 UUID (a1b2c3d4-e5f6-...)                     │
│  └─ 未找到: 返回错误 或 自动创建用户                         │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  继续处理购买                                                │
│  1. 记录到 purchase 表                                       │
│  2. 执行发货                                                 │
└─────────────────────────────────────────────────────────────┘
```

---

## 🎯 推荐配置

根据不同场景，推荐的配置:

| 场景 | 推荐方案 | 配置难度 | 灵活性 |
|------|---------|---------|--------|
| 手机号/用户名注册 | 方案1 (username) | ⭐ 简单 | ⭐⭐ 中等 |
| 专属第三方平台 | 方案2 (custom_id) | ⭐⭐ 中等 | ⭐⭐⭐ 高 |
| Facebook/Apple 登录 | 方案3 (platform_id) | ⭐ 简单 | ⭐⭐ 中等 |
| 支持多种登录方式 | 方案4 (多字段) | ⭐⭐⭐ 复杂 | ⭐⭐⭐ 高 |
| 游客模式 | 方案5 (自动创建) | ⭐⭐ 中等 | ⭐⭐⭐ 高 |

---

## 🔐 安全建议

1. **验证 uid 格式**
```go
func (s *ApiServer) validateUID(uid string) bool {
    // 检查长度
    if len(uid) < 3 || len(uid) > 64 {
        return false
    }
    
    // 检查字符 (只允许字母数字和下划线)
    matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, uid)
    return matched
}
```

2. **防止 SQL 注入**
```go
// ✅ 正确: 使用参数化查询
db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = $1", uid)

// ❌ 错误: 拼接 SQL 字符串
db.QueryRowContext(ctx, fmt.Sprintf("SELECT id FROM users WHERE username = '%s'", uid))
```

3. **限制查询频率**
```go
// 使用缓存避免频繁查询
userIDCache := make(map[string]uuid.UUID)
if cachedID, exists := userIDCache[uid]; exists {
    return cachedID, nil
}
```

---

## 📞 需要帮助?

如果还不确定如何配置，可以:

1. 查看你的用户表结构:
```sql
\d users
```

2. 查看第三方通知的实际数据:
```powershell
# 启用调试日志
tail -f nakama.log | grep "收到购买通知请求"
```

3. 根据实际情况选择上述方案之一。

