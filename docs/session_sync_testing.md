# Session 跨节点同步测试说明

## 问题分析

### 原始问题

在多节点 Single Session 测试中，发现旧的 session 在大部分情况下仍然有效，只有偶尔会失效。

### 根本原因

**Redis pub/sub 消息传播有延迟**

原始测试代码的执行顺序有误：

```go
// ❌ 错误的顺序
1. 在节点 A 登录 (触发 RemoveAll，发送 Redis 消息)
2. 立即验证节点 A 的新 session (成功)
3. 立即验证节点 B 的旧 session ⚠️ 此时消息可能还没传播到节点 B！
4. sleep 100ms (太晚了)
```

**结果**：由于消息还没传播到节点 B，所以旧 session 看起来仍然有效。

## 解决方案

### 修复后的顺序

```go
// ✅ 正确的顺序
1. 在节点 A 登录 (触发 RemoveAll，发送 Redis 消息)
2. 验证节点 A 的新 session
3. sleep 300ms ⏰ 等待消息传播
4. 验证节点 B 的旧 session (现在消息已经到达)
```

### 关键修改

1. **等待时间增加**：从 100ms 增加到 300ms
   - 考虑网络延迟、Redis 处理延迟、节点处理延迟

2. **等待位置调整**：移到验证旧 session **之前**
   - 确保 RemoveAll 消息已经传播并被处理

3. **错误级别提升**：从警告改为错误
   - 使用 `t.Errorf()` 而不是 `t.Logf()`
   - 测试失败会更明显

4. **添加详细日志**：
   - 显示每个步骤的耗时
   - 方便调试和了解消息传播时间

## 测试流程

### Single Session 工作原理

```
节点 A (7350):                    Redis:                     节点 B (7450):
    |                               |                              |
    | 1. 用户登录                    |                              |
    |    RemoveAll(userID) -------> | 2. Publish                   |
    |                               |    "session_cache:user:sync" |
    |                               |                              |
    |                               | 3. Subscribe <-------------- |
    |                               |                              |
    |                               |                              | 4. handleSyncMessage
    |                               |                              |    更新本地 cache
    |                               |                              |    lastInvalidation = ts
    |                               |                              |
    | 5. 等待 300ms 确保消息传播      |                              |
    |                               |                              |
    | 6. 验证旧 session ----------> |                              |
    |                               | <--------------------------- | 7. IsValidSession
    |                               |                              |    检查 lastInvalidation
    |                               |                              |    返回 false (失效)
```

### 时间戳检查逻辑

```go
// 在 IsValidSession 中
if cache.lastInvalidation > 0 && exp-tokenExpirySec < cache.lastInvalidation {
    // token 签发时间 < lastInvalidation
    // 说明这是一个旧的 token，应该失效
    return false
}
```

**关键点**：
- `exp` 是 token 的过期时间
- `exp - tokenExpirySec` 是 token 的签发时间
- 如果签发时间早于 `lastInvalidation`，则 token 失效

## 测试示例输出

### 成功的测试输出

```
步骤 2: 在节点7450登录用户 sameuser@example.com
  登录完成，耗时: 45ms
  ✓ 客户端 2 (节点7450) session验证成功
  等待 Redis 消息同步...
  同步等待完成，耗时: 300ms
  检查上一个客户端 (节点7350) 的session是否已失效...
  ✓ 上一个客户端 (节点7350) session已失效 (总延迟: 355ms)
    失效原因: session validation failed with status: 401
```

### 失败的测试输出（如果同步有问题）

```
步骤 2: 在节点7450登录用户 sameuser@example.com
  登录完成，耗时: 45ms
  ✓ 客户端 2 (节点7450) session验证成功
  等待 Redis 消息同步...
  同步等待完成，耗时: 300ms
  检查上一个客户端 (节点7350) 的session是否已失效...
  ❌ 错误: 上一个客户端 (节点7350) session仍然有效 (总延迟: 355ms)
    验证耗时: 10ms
```

## 配置要求

### 必须的配置

```yaml
# config1.yml 和 config2.yml 都需要
session:
  single_session: true  # 启用 Single Session

cluster:
  enabled: true         # 启用集群模式
  redis_address: "localhost:6379"
```

### Redis 要求

- Redis 必须运行并可访问
- 两个节点连接到同一个 Redis 实例
- Redis pub/sub 功能正常工作

## 性能考虑

### 消息传播延迟

典型的延迟组成：
- Redis publish：1-5ms
- 网络传输：1-10ms (本地) / 10-50ms (跨机房)
- Redis subscribe：1-5ms
- 节点处理：1-5ms

**总计**：本地环境约 5-25ms，跨机房约 20-70ms

### 测试等待时间选择

- **100ms**：本地环境可能不够（如测试所示）
- **200ms**：本地环境通常足够
- **300ms**：本地环境很安全，跨机房可能需要更长
- **500ms+**：跨地域部署推荐

### 建议

1. **本地测试**：300ms
2. **同机房集群**：500ms
3. **跨机房集群**：1000ms
4. **跨地域集群**：2000ms

## 故障排查

### 如果测试仍然失败

1. **检查 Redis 连接**
   ```bash
   redis-cli ping
   # 应该返回 PONG
   ```

2. **监控 Redis 频道**
   ```bash
   redis-cli SUBSCRIBE "nakama:session_cache:user:sync"
   # 登录时应该看到消息
   ```

3. **检查节点日志**
   - 查找 "Session cache cluster mode enabled"
   - 查找 "Publishing session cache sync message"
   - 查找 "Received session cache sync message"

4. **增加等待时间**
   ```go
   time.Sleep(500 * time.Millisecond) // 或更长
   ```

5. **检查节点 ID**
   - 确保两个节点的 `name` 不同
   - 消息去重依赖节点 ID

## 延伸阅读

- [Redis 排行榜缓存同步方案](Redis排行榜缓存同步方案.md)
- [Redis 缓存一致性测试指南](Redis缓存一致性测试指南.md)

