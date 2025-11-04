# Redis分布式排行榜缓存同步方案

## 问题背景

在多节点环境中，原有的 `LocalLeaderboardRankCache` 存在以下问题：

1. **数据一致性问题**：每个节点维护独立的内存缓存，数据更新时其他节点不同步
2. **排名准确性问题**：不同节点可能返回不同的排名结果  
3. **内存重复问题**：每个节点都缓存完整数据，造成资源浪费

## 解决方案

基于Redis实现分布式排行榜缓存，采用 **Redis + 本地缓存 + 发布/订阅** 的架构：

### 架构设计

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Node 1    │    │   Node 2    │    │   Node 3    │
│             │    │             │    │             │
│ Local Cache │    │ Local Cache │    │ Local Cache │
│      ↓      │    │      ↓      │    │      ↓      │
│Redis Client │    │Redis Client │    │Redis Client │
└─────────────┘    └─────────────┘    └─────────────┘
         ↓                 ↓                 ↓
    ┌─────────────────────────────────────────────────┐
    │              Redis 集群                          │
    │                                                │
    │  ┌─────────────┐  ┌─────────────┐              │
    │  │ Sorted Sets │  │   Pub/Sub   │              │
    │  │(排行榜数据)  │  │  (同步消息)  │              │
    │  └─────────────┘  └─────────────┘              │
    └─────────────────────────────────────────────────┘
```

### 核心机制

1. **Redis有序集合**：存储排行榜核心数据，支持高效的排名查询
2. **本地缓存**：缓存热点排名数据，减少Redis访问
3. **发布/订阅**：实现跨节点的缓存失效通知
4. **代数比较**：基于generation字段避免数据回退

## 技术实现

### 1. 数据存储结构

#### Redis键设计
```
# 排行榜有序集合
leaderboard:rank:{leaderboard_id}:{expiry_unix}

# 排行榜元数据哈希
leaderboard:meta:{leaderboard_id}:{expiry_unix}

# 发布/订阅频道
rank:invalidate  # 缓存失效通知
rank:update      # 数据更新通知
```

#### 数据格式
```go
// Redis有序集合成员
member := owner_id.String()
score := score * 1e9 + subscore  // 复合分数

// 元数据哈希存储
generation_field := member
generation_value := generation_int32
```

### 2. 关键特性

#### 多级缓存策略
1. **L1缓存（本地）**：存储最近查询的排名，5分钟过期
2. **L2缓存（Redis）**：存储完整排行榜数据，1小时过期
3. **L3存储（数据库）**：永久存储，作为数据源

#### 一致性保证
```go
// 原子更新脚本
script := `
local currentGeneration = redis.call('HGET', metaKey, member)
if currentGeneration and tonumber(currentGeneration) >= tonumber(generation) then
    return current_rank  -- 避免旧数据覆盖新数据
end

redis.call('ZADD', redisKey, score, member)
redis.call('HSET', metaKey, member, generation)
return new_rank
`
```

#### 跨节点同步
```go
// 发布缓存失效消息
type RankCacheMessage struct {
    LeaderboardID string    `json:"leaderboard_id"`
    ExpiryUnix    int64     `json:"expiry_unix"`
    OwnerID       uuid.UUID `json:"owner_id"`
    Action        string    `json:"action"`      // insert, delete, clear
    NodeID        string    `json:"node_id"`     // 避免循环通知
}
```

## 配置使用

### 1. 配置文件

```yaml
# config.yml
leaderboard:
  # 启用Redis分布式缓存
  use_redis: true
  redis_address: "192.168.102.223:6379"
  redis_password: ""
  
  # 性能调优
  rank_cache_workers: 4
  callback_queue_size: 65536
  callback_queue_workers: 8
  
  # 可选的黑名单配置
  blacklist_rank_cache: ["test_leaderboard"]  # 禁用特定排行榜
  # blacklist_rank_cache: ["*"]               # 禁用所有排行榜缓存
```

### 2. 启动配置

```bash
# 启动节点1
./nakama --config config-node1.yml

# 启动节点2  
./nakama --config config-node2.yml

# 启动节点3
./nakama --config config-node3.yml
```

## 性能对比

### 单节点 vs 多节点

| 指标 | 本地缓存 | Redis缓存 | 说明 |
|------|----------|-----------|------|
| 查询延迟 | ~0.1ms | ~1-2ms | Redis增加网络开销 |
| 内存使用 | N×全量数据 | 共享存储 | 显著减少内存消耗 |
| 数据一致性 | 不一致 | 强一致 | Redis保证数据同步 |
| 扩展性 | 差 | 优秀 | 水平扩展友好 |
| 可用性 | 单点故障 | 高可用 | Redis集群支持 |

### 性能优化建议

1. **Redis集群**：使用Redis Cluster提高可用性和性能
2. **连接池**：优化Redis连接管理
3. **批量操作**：使用Pipeline减少网络往返
4. **本地缓存**：合理设置本地缓存过期时间
5. **监控告警**：监控Redis性能和缓存命中率

## 监控指标

### 关键指标
```go
// 缓存命中率
local_cache_hit_rate := local_hits / total_requests

// Redis操作延迟
redis_operation_latency := time.Since(start)

// 发布/订阅延迟
pubsub_message_delay := receive_time - publish_time

// 缓存大小
redis_memory_usage := redis_info["used_memory"]
```

### 告警阈值
- 本地缓存命中率 < 70%
- Redis操作延迟 > 10ms
- 发布/订阅延迟 > 100ms
- Redis内存使用率 > 80%

## 故障处理

### 常见问题

1. **Redis连接失败**
   - 降级到数据库查询
   - 禁用排名缓存功能
   - 记录错误日志

2. **数据不一致**
   - 检查发布/订阅是否正常
   - 验证generation字段逻辑
   - 手动清理缓存数据

3. **性能下降**
   - 检查Redis集群状态
   - 优化查询模式
   - 调整缓存策略

### 运维建议

1. **备份策略**：定期备份Redis数据
2. **容量规划**：预估排行榜数据增长
3. **版本升级**：保持Redis版本更新
4. **安全配置**：启用Redis认证和加密

## 迁移指南

### 从本地缓存迁移

1. **准备阶段**
   ```bash
   # 部署Redis集群
   redis-server --cluster-enabled yes
   
   # 更新配置文件
   vi config.yml
   ```

2. **灰度发布**
   ```yaml
   # 先在部分节点启用Redis缓存
   leaderboard:
     use_redis: true
     redis_address: "redis-cluster:6379"
   ```

3. **全量切换**
   ```bash
   # 逐步替换所有节点配置
   # 监控性能和稳定性
   ```

4. **清理工作**
   ```bash
   # 移除本地缓存相关配置
   # 清理历史数据
   ```

## 总结

Redis分布式排行榜缓存方案解决了多节点环境下的数据一致性问题，通过合理的架构设计和性能优化，在保证数据准确性的同时，提供了良好的性能和扩展性。

主要优势：
- ✅ 数据强一致性
- ✅ 水平扩展能力
- ✅ 高可用性
- ✅ 内存使用优化
- ✅ 实时同步更新

适用场景：
- 多节点游戏服务器集群
- 高并发排行榜查询
- 实时竞技游戏
- 大规模用户系统 