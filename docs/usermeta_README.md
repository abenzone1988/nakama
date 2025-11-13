# UserMeta 自动管理系统

## 概述

UserMeta 自动管理系统提供了延迟加载、自动保存的用户元数据管理功能。

## 核心特性

1. **延迟加载**：首次访问时才从数据库加载
2. **自动保存**：请求结束时自动保存修改
3. **Context 传递**：通过 context 在整个请求生命周期中传递
4. **线程安全**：使用互斥锁保证并发安全

## 使用方式

### 1. 读取 UserMeta

```go
func Handler(ctx context.Context, logger *zap.Logger) error {
    userMeta, user, err := GetUserMeta(ctx)
    if err != nil {
        return err
    }
    
    // 使用 userMeta...
    logger.Info("Last sync", zap.Int64("time", userMeta.LastSyncNotice))
    return nil
}
```

### 2. 修改 UserMeta

```go
func Handler(ctx context.Context) error {
    // 使用 UpdateUserMeta 修改
    err := UpdateUserMeta(ctx, func(userMeta *game.UserMeta) error {
        userMeta.LastSyncNotice = time.Now().Unix()
        
        if userMeta.Stamina != nil {
            userMeta.Stamina.Stamina += 10
        }
        
        return nil
    })
    
    // UserMeta 会在请求结束时自动保存
    return err
}
```

### 3. 使用业务函数

体力管理示例（`game_stamina.go`）：

```go
// 获取当前体力（自动计算恢复）
stamina, err := GetCurrentStamina(ctx, logger)

// 消耗体力
err := ConsumeStamina(ctx, logger, 10)

// 补充体力
err := RefillStamina(ctx, logger, 50)

// 重置体力
err := ResetStamina(ctx, logger)
```

## 工作原理

### 请求流程

1. **拦截器**（`api.go`）在请求开始时创建 `UserMetaManager` 并添加到 context
2. **延迟加载**：首次调用 `GetUserMeta` 时从数据库加载
3. **修改标记**：调用 `UpdateUserMeta` 时标记为 dirty
4. **自动保存**：拦截器在请求结束时自动保存修改过的 UserMeta

### 核心组件

- `UserMetaManager`：管理 UserMeta 的生命周期
- `GetUserMeta(ctx)`：获取 UserMeta
- `UpdateUserMeta(ctx, func)`：修改 UserMeta
- `SaveUserMeta(ctx)`：手动保存（通常不需要）

## 性能优化

- 延迟加载：不访问就不加载
- 按需保存：只保存被修改的数据
- 单次保存：无论修改多少次，请求结束时只保存一次

## 注意事项

1. **必须有 UserMetaManager**：所有函数都需要 context 中存在 UserMetaManager
2. **拦截器自动添加**：已认证的 gRPC 请求会自动添加
3. **错误处理**：保存失败时整个请求返回错误，确保数据一致性

