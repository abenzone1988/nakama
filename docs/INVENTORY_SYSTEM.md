# 背包系统 (Inventory System) - 完整文档

## 概述

背包系统是一个完整的物品管理系统，参考了钱包系统的设计，提供了物品的添加、扣除、查询和历史记录功能。

## 系统架构

### 1. 数据库层

#### 迁移文件
- `migrate/sql/20251110120000_create_inventory.sql`
  - 添加 `inventory` 字段到 `users` 表（JSONB 类型）
  - 创建 `inventory_ledger` 表记录背包变化历史
  - 添加索引优化查询性能

#### 数据结构

**users.inventory 字段格式：**
```json
{
  "item_001": 100,
  "sword_legendary": 1,
  "potion_hp": 50
}
```

**inventory_ledger 表：**
- `id`: UUID，唯一标识
- `user_id`: UUID，用户ID
- `changeset`: JSONB，物品变化集
- `metadata`: JSONB，元数据（如原因、来源等）
- `create_time`: TIMESTAMPTZ，创建时间
- `update_time`: TIMESTAMPTZ，更新时间

### 2. 后端核心代码

#### server/core_inventory.go

主要功能：

1. **UpdateInventories** - 更新玩家背包
   - 支持批量更新多个玩家
   - 支持添加（正数）和扣除（负数）物品
   - 自动检查物品数量，防止负数
   - 事务保证数据一致性
   - 可选记录到 ledger

2. **ListInventoryLedger** - 查询背包变化记录
   - 支持分页（前向/后向翻页）
   - 使用游标（cursor）高效分页
   - 返回详细变化记录

3. **UpdateInventoryLedger** - 更新 ledger 元数据
   - 支持更新已有记录的元数据

4. **InventoryNegativeError** - 自定义错误类型
   - 当物品不足时返回详细错误信息

#### server/core_inventory_test.go

测试用例：
- 单用户并发更新测试
- 多用户批量更新测试
- 物品不足错误测试
- Ledger 查询测试
- 多种物品同时操作测试

#### server/console_account.go

Console API 接口：
- `GetInventoryLedger` - 查询用户背包变化历史
- `DeleteInventoryLedger` - 删除背包变化记录

### 3. Console API (Proto)

#### console/console.proto

添加的 RPC 方法：
```protobuf
rpc GetInventoryLedger (GetInventoryLedgerRequest) returns (InventoryLedgerList);
rpc DeleteInventoryLedger (DeleteInventoryLedgerRequest) returns (google.protobuf.Empty);
```

添加的消息类型：
- `InventoryLedger` - 背包变化记录
- `InventoryLedgerList` - 背包变化记录列表
- `GetInventoryLedgerRequest` - 查询请求
- `DeleteInventoryLedgerRequest` - 删除请求

### 4. 前端管理界面

#### console/ui/src/app/account/inventory/

组件文件：
- `inventory.component.ts` - TypeScript 组件逻辑
- `inventory.component.html` - HTML 模板
- `inventory.component.scss` - 样式文件

功能特性：
- JSON 编辑器编辑背包数据
- 查看背包变化历史（分页）
- 删除背包变化记录
- 展开/折叠元数据查看

#### 路由配置

- `app-routing.module.ts` - 添加 inventory 路由
- `app.module.ts` - 注册 InventoryComponent
- `account.component.ts` - 添加 Inventory 菜单项

## 使用方式

### 1. 数据库迁移

```bash
# 应用迁移
migrate up

# 回滚迁移
migrate down
```

### 2. 后端 Go 代码使用示例

```go
// 添加物品
updates := []*inventoryUpdate{
    {
        UserID: userUUID,
        Changeset: map[string]int64{
            "sword_001": 1,
            "potion_hp": 10,
            "gold": 1000,
        },
        Metadata: `{"reason": "quest_reward", "quest_id": "main_001"}`,
    },
}
results, err := UpdateInventories(ctx, logger, db, updates, true)

// 扣除物品
updates := []*inventoryUpdate{
    {
        UserID: userUUID,
        Changeset: map[string]int64{
            "sword_001": -1,  // 扣除 1 把剑
            "gold": -100,     // 扣除 100 金币
        },
        Metadata: `{"reason": "purchase", "item_bought": "shield_001"}`,
    },
}
results, err := UpdateInventories(ctx, logger, db, updates, true)

// 查询变化记录
limit := 20
ledgers, nextCursor, prevCursor, err := ListInventoryLedger(ctx, logger, db, userUUID, &limit, "")
```

### 3. 前端访问

1. 登录 Nakama Console
2. 进入 Accounts 页面
3. 选择一个用户
4. 点击 "Inventory" 标签
5. 可以：
   - 编辑背包数据（JSON 格式）
   - 查看背包变化历史
   - 删除历史记录

## API 端点

### Console REST API

- `GET /v2/console/account/{account_id}/inventory` - 获取用户背包变化历史
  - 参数：`limit` (1-100), `cursor` (分页游标)
  
- `DELETE /v2/console/account/{id}/inventory/{inventory_id}` - 删除背包变化记录

## 数据安全

1. **事务保证**：所有背包更新操作都在事务中执行
2. **负数检查**：自动检查物品数量，防止出现负数
3. **用户隔离**：每个用户的背包数据完全隔离
4. **权限控制**：Console 操作需要相应权限

## 性能优化

1. **批量更新**：支持一次更新多个用户的背包
2. **索引优化**：在 `inventory_ledger` 表创建了合理的索引
3. **分页查询**：使用游标分页，高效查询大量历史记录
4. **排序优化**：按 user_id 自然顺序更新，避免死锁

## 扩展性

1. **物品类型无限制**：使用 JSONB 存储，可以添加任意类型的物品
2. **元数据灵活**：ledger 的 metadata 可以存储任意 JSON 数据
3. **易于集成**：API 设计简洁，易于与游戏逻辑集成

## 测试

运行测试：
```bash
# 运行所有背包测试
go test -v ./server -run TestUpdateInventory

# 运行特定测试
go test -v ./server -run TestUpdateInventoryInsufficientItems

# 运行 ledger 测试
go test -v ./server -run TestListInventoryLedger
```

## 注意事项

1. **Proto 生成**：修改 `console.proto` 后需要重新生成代码
   ```bash
   cd console
   go generate
   ```

2. **前端构建**：修改前端代码后需要重新构建
   ```bash
   cd console/ui
   npm run build
   ```

3. **数据库迁移**：确保在启动服务前执行数据库迁移

## 文件清单

### 数据库
- `migrate/sql/20251110120000_create_inventory.sql`

### 后端核心
- `server/core_inventory.go`
- `server/core_inventory_test.go`
- `server/console_account.go` (修改)

### Proto 定义
- `console/console.proto` (修改)

### 前端组件
- `console/ui/src/app/account/inventory/inventory.component.ts`
- `console/ui/src/app/account/inventory/inventory.component.html`
- `console/ui/src/app/account/inventory/inventory.component.scss`

### 路由配置
- `console/ui/src/app/app-routing.module.ts` (修改)
- `console/ui/src/app/app.module.ts` (修改)
- `console/ui/src/app/account/account.component.ts` (修改)

### 文档
- `docs/INVENTORY_SYSTEM.md` (本文档)

## 版本历史

- **v1.0** (2024-11-10)
  - 初始版本
  - 完整的背包系统实现
  - Console 管理界面
  - 完整的测试用例




