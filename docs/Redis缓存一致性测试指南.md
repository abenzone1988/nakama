# Redis排行榜缓存一致性测试指南

## 概述

本文档描述了如何测试Redis排行榜缓存在分布式环境中的一致性。测试覆盖了基本功能、分布式一致性、网络分区、性能、故障恢复等关键场景。

## 测试环境要求

### 1. 系统要求
- Go 1.19+
- Redis 6.0+
- 至少2GB可用内存
- 网络连接正常

### 2. 依赖安装

#### Linux/macOS
```bash
# 安装Redis（Ubuntu/Debian）
sudo apt-get install redis-server

# 安装Redis（CentOS/RHEL）
sudo yum install redis

# 启动Redis服务
sudo systemctl start redis
sudo systemctl enable redis

# 验证Redis运行状态
redis-cli ping
```

#### Windows
```cmd
# 方法1: 使用Chocolatey安装
choco install redis-64

# 方法2: 下载Windows版本
# 访问 https://github.com/microsoftarchive/redis/releases
# 下载 Redis-x64-xxx.msi 并安装

# 启动Redis服务
redis-server

# 验证Redis运行状态
redis-cli ping
```

## 测试用例详解

### 1. 基础功能测试 (TestRedisLeaderboardRankCache_BasicOperations)

**测试目标**: 验证基本的插入、查询、删除功能

**测试内容**:
- 插入排行榜数据
- 查询用户排名
- 删除用户数据
- 删除整个排行榜

**预期结果**:
- 插入操作返回正确的排名
- 查询操作返回正确的排名
- 删除操作成功执行
- 数据一致性正确

### 2. 分布式一致性测试 (TestRedisLeaderboardRankCache_DistributedConsistency)

**测试目标**: 验证多节点环境下的数据一致性

**测试内容**:
- 基本插入和查询一致性
- 并发更新一致性
- 删除一致性

**测试场景**:
```go
// 创建3个缓存实例模拟不同节点
cache1, cache2, cache3 := setupRedisLeaderboardCache()

// 在节点1插入数据
rank := cache1.Insert(leaderboardID, score, ownerID)

// 验证所有节点都能获取相同结果
rank1 := cache1.Get(leaderboardID, ownerID)
rank2 := cache2.Get(leaderboardID, ownerID)
rank3 := cache3.Get(leaderboardID, ownerID)
assert.Equal(rank1, rank2, rank3)
```

**预期结果**:
- 所有节点返回相同的排名结果
- 并发操作不会导致数据不一致
- 删除操作在所有节点同步生效

### 3. 网络分区测试 (TestRedisLeaderboardRankCache_NetworkPartition)

**测试目标**: 验证网络分区情况下的系统行为

**测试内容**:
- 正常情况下的数据同步
- 网络分区模拟
- 网络恢复测试

**测试场景**:
```go
// 模拟网络分区：停止cache2的发布/订阅
cache2.pubsub.Close()

// 在cache1插入数据
rank := cache1.Insert(leaderboardID, score, ownerID)

// cache2应该无法获取新数据
rank2 := cache2.Get(leaderboardID, ownerID)
assert.Equal(0, rank2) // 网络分区时返回0
```

**预期结果**:
- 网络分区时，分区内的节点正常工作
- 分区外的节点无法获取新数据
- 网络恢复后，数据能正常同步

### 4. 性能测试 (TestRedisLeaderboardRankCache_Performance)

**测试目标**: 验证系统在高负载下的性能表现

**测试内容**:
- 批量插入性能
- 批量查询性能
- 并发性能测试

**性能指标**:
- 插入吞吐量: ≥1000 ops/sec
- 查询吞吐量: ≥2000 ops/sec
- 并发操作: 支持10个goroutine并发

**测试方法**:
```bash
# 运行性能测试
go test -v -run TestRedisLeaderboardRankCache_Performance ./server/

# 运行基准测试
go test -v -bench=BenchmarkRedisLeaderboardRankCache ./server/
```

### 5. 故障恢复测试 (TestRedisLeaderboardRankCache_FaultTolerance)

**测试目标**: 验证系统在故障情况下的恢复能力

**测试内容**:
- Redis连接失败时的降级处理
- 数据一致性验证
- 连接恢复后的功能验证

**测试场景**:
```go
// 模拟Redis连接失败
cache.client.Close()

// 验证降级处理
rank := cache.Get(leaderboardID, ownerID)
assert.Equal(0, rank) // 连接失败时返回0

// 重新连接后验证功能
cache.client = redis.NewClient(...)
rank = cache.Insert(leaderboardID, score, ownerID)
assert.Greater(0, rank) // 重新连接后正常工作
```

### 6. 代数冲突测试 (TestRedisLeaderboardRankCache_GenerationConflict)

**测试目标**: 验证代数机制防止数据回退

**测试内容**:
- 新数据覆盖旧数据
- 旧数据不覆盖新数据

**测试场景**:
```go
// 插入低代数数据
rank1 := cache.Insert(leaderboardID, score1, generation1, ownerID)

// 插入高代数数据
rank2 := cache.Insert(leaderboardID, score2, generation2, ownerID)
assert.Equal(rank1, rank2) // 高代数数据覆盖低代数数据

// 尝试插入低代数数据
rank3 := cache.Insert(leaderboardID, score3, generation1, ownerID)
assert.Equal(rank2, rank3) // 低代数数据不覆盖高代数数据
```

## 运行测试

### 1. 运行所有测试

#### Linux/macOS
```bash
# 给测试脚本执行权限
chmod +x test_redis_cache.sh

# 运行完整测试套件
./test_redis_cache.sh
```

#### Windows
```cmd
# 使用批处理脚本
test_redis_cache.bat

# 或使用PowerShell脚本（推荐）
powershell -ExecutionPolicy Bypass -File test_redis_cache.ps1

# 使用配置文件版本（推荐）
powershell -ExecutionPolicy Bypass -File test_redis_cache_config.ps1
```

#### 远程Redis配置
```cmd
# 修改配置文件 redis_test_config.json
{
  "redis": {
    "host": "192.168.102.223",
    "port": 6379,
    "password": "",
    "db": 1
  }
}

# 运行测试
powershell -ExecutionPolicy Bypass -File test_redis_cache_config.ps1
```

### 2. 运行单个测试
```bash
# 运行分布式一致性测试
go test -v -run TestRedisLeaderboardRankCache_DistributedConsistency ./server/

# 运行性能测试
go test -v -run TestRedisLeaderboardRankCache_Performance ./server/

# 运行基准测试
go test -v -bench=BenchmarkRedisLeaderboardRankCache ./server/
```

### 3. 运行基准测试
```bash
# 运行所有基准测试
go test -v -bench=. ./server/

# 运行特定基准测试
go test -v -bench=BenchmarkRedisLeaderboardRankCache_Insert ./server/
go test -v -bench=BenchmarkRedisLeaderboardRankCache_Get ./server/
```

## 测试结果分析

### 1. 一致性指标
- **强一致性**: 所有节点在正常网络条件下数据完全一致
- **最终一致性**: 网络分区恢复后，数据在5分钟内达到一致
- **可用性**: 单节点故障不影响其他节点正常工作

### 2. 性能指标
- **延迟**: 查询操作 < 5ms，插入操作 < 10ms
- **吞吐量**: 插入 ≥ 1000 ops/sec，查询 ≥ 2000 ops/sec
- **并发**: 支持100+并发连接

### 3. 可靠性指标
- **故障恢复**: Redis连接失败时自动降级
- **数据持久性**: 数据在Redis中正确存储
- **监控告警**: 健康检查功能正常

## 常见问题排查

### 1. Redis连接失败
```bash
# 检查Redis服务状态
sudo systemctl status redis

# 检查Redis端口
netstat -tlnp | grep 6379

# 测试Redis连接
redis-cli ping
```

### 2. 测试失败排查
```bash
# 查看详细测试日志
go test -v -run TestRedisLeaderboardRankCache_DistributedConsistency ./server/ -logtostderr

# 检查Redis数据
redis-cli KEYS "leaderboard:*"

# 清理测试数据
redis-cli FLUSHDB
```

### 3. 性能问题排查
```bash
# 监控Redis性能
redis-cli INFO

# 检查内存使用
redis-cli INFO memory

# 检查连接数
redis-cli INFO clients
```

## 测试报告模板

### 测试环境信息
- Redis版本: 6.0+
- Go版本: 1.19+
- 测试时间: YYYY-MM-DD HH:MM:SS
- 测试节点数: 3

### 测试结果汇总
| 测试用例 | 状态 | 执行时间 | 备注 |
|---------|------|----------|------|
| 基础功能测试 | ✅ | XXms | 通过 |
| 分布式一致性测试 | ✅ | XXms | 通过 |
| 网络分区测试 | ✅ | XXms | 通过 |
| 性能测试 | ✅ | XXms | 通过 |
| 故障恢复测试 | ✅ | XXms | 通过 |
| 代数冲突测试 | ✅ | XXms | 通过 |
| 健康检查测试 | ✅ | XXms | 通过 |

### 性能测试结果
- 插入吞吐量: XXXX ops/sec
- 查询吞吐量: XXXX ops/sec
- 平均延迟: XXms
- 并发支持: XXX连接

### 结论
Redis排行榜缓存一致性测试全部通过，系统在分布式环境下表现良好，满足生产环境要求。

## 持续集成

### 1. GitHub Actions配置
```yaml
name: Redis Cache Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:6.0
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.19'
      - run: go test -v ./server/
```

### 2. 自动化测试脚本
```bash
#!/bin/bash
# 自动化测试脚本
set -e

echo "开始Redis缓存一致性测试..."

# 启动Redis（如果未运行）
if ! redis-cli ping > /dev/null 2>&1; then
    echo "启动Redis服务..."
    redis-server --daemonize yes
    sleep 2
fi

# 运行测试
./test_redis_cache.sh

echo "测试完成"
```

## 总结

Redis排行榜缓存一致性测试确保了系统在分布式环境下的可靠性和性能。通过全面的测试覆盖，我们验证了：

1. ✅ **数据一致性**: 多节点环境下数据强一致性
2. ✅ **故障恢复**: 网络分区和节点故障的恢复能力
3. ✅ **性能表现**: 高并发下的稳定性能
4. ✅ **监控告警**: 完善的健康检查和监控机制

这些测试为生产环境部署提供了可靠的质量保证。 