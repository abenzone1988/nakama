#!/bin/bash

# Redis排行榜缓存一致性测试脚本

echo "=== Redis排行榜缓存一致性测试 ==="

# 检查Redis是否运行
echo "1. 检查Redis连接..."
redis-cli ping > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "错误: Redis未运行，请先启动Redis服务器"
    echo "启动命令: redis-server"
    exit 1
fi
echo "✓ Redis连接正常"

# 清理测试数据
echo "2. 清理测试数据..."
redis-cli FLUSHDB > /dev/null 2>&1
echo "✓ 测试数据已清理"

# 运行基础功能测试
echo "3. 运行基础功能测试..."
go test -v -run TestRedisLeaderboardRankCache_BasicOperations ./server/
if [ $? -ne 0 ]; then
    echo "✗ 基础功能测试失败"
    exit 1
fi
echo "✓ 基础功能测试通过"

# 运行分布式一致性测试
echo "4. 运行分布式一致性测试..."
go test -v -run TestRedisLeaderboardRankCache_DistributedConsistency ./server/
if [ $? -ne 0 ]; then
    echo "✗ 分布式一致性测试失败"
    exit 1
fi
echo "✓ 分布式一致性测试通过"

# 运行网络分区测试
echo "5. 运行网络分区测试..."
go test -v -run TestRedisLeaderboardRankCache_NetworkPartition ./server/
if [ $? -ne 0 ]; then
    echo "✗ 网络分区测试失败"
    exit 1
fi
echo "✓ 网络分区测试通过"

# 运行性能测试
echo "6. 运行性能测试..."
go test -v -run TestRedisLeaderboardRankCache_Performance ./server/
if [ $? -ne 0 ]; then
    echo "✗ 性能测试失败"
    exit 1
fi
echo "✓ 性能测试通过"

# 运行故障恢复测试
echo "7. 运行故障恢复测试..."
go test -v -run TestRedisLeaderboardRankCache_FaultTolerance ./server/
if [ $? -ne 0 ]; then
    echo "✗ 故障恢复测试失败"
    exit 1
fi
echo "✓ 故障恢复测试通过"

# 运行代数冲突测试
echo "8. 运行代数冲突测试..."
go test -v -run TestRedisLeaderboardRankCache_GenerationConflict ./server/
if [ $? -ne 0 ]; then
    echo "✗ 代数冲突测试失败"
    exit 1
fi
echo "✓ 代数冲突测试通过"

# 运行健康检查测试
echo "9. 运行健康检查测试..."
go test -v -run TestRedisLeaderboardRankCache_HealthCheck ./server/
if [ $? -ne 0 ]; then
    echo "✗ 健康检查测试失败"
    exit 1
fi
echo "✓ 健康检查测试通过"

# 运行基准测试
echo "10. 运行基准测试..."
go test -v -bench=BenchmarkRedisLeaderboardRankCache ./server/
if [ $? -ne 0 ]; then
    echo "✗ 基准测试失败"
    exit 1
fi
echo "✓ 基准测试通过"

echo ""
echo "=== 所有测试通过！ ==="
echo "Redis排行榜缓存一致性测试完成"
echo ""
echo "测试总结:"
echo "- ✓ 基础功能测试"
echo "- ✓ 分布式一致性测试"
echo "- ✓ 网络分区测试"
echo "- ✓ 性能测试"
echo "- ✓ 故障恢复测试"
echo "- ✓ 代数冲突测试"
echo "- ✓ 健康检查测试"
echo "- ✓ 基准测试" 