@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo === Redis排行榜缓存一致性测试 ===

REM 设置Redis连接配置
set REDIS_HOST=192.168.102.223
set REDIS_PORT=6379
set REDIS_PASSWORD=

REM 检查是否要修改Redis配置
echo 当前Redis配置: %REDIS_HOST%:%REDIS_PORT%
set /p change_config="是否修改Redis配置？(y/N): "
if /i "%change_config%"=="y" (
    set /p REDIS_HOST="请输入Redis主机地址 (默认: 192.168.102.223): "
    if "!REDIS_HOST!"=="" set REDIS_HOST=192.168.102.223
    
    set /p REDIS_PORT="请输入Redis端口 (默认: 6379): "
    if "!REDIS_PORT!"=="" set REDIS_PORT=6379
    
    set /p use_password="是否使用密码？(y/N): "
    if /i "!use_password!"=="y" (
        set /p REDIS_PASSWORD="请输入Redis密码: "
    )
)

REM 检查Redis是否运行
echo 1. 检查Redis连接...
if "!REDIS_PASSWORD!"=="" (
    redis-cli -h %REDIS_HOST% -p %REDIS_PORT% ping >nul 2>&1
) else (
    redis-cli -h %REDIS_HOST% -p %REDIS_PORT% -a %REDIS_PASSWORD% ping >nul 2>&1
)
if %errorlevel% neq 0 (
    echo 错误: 无法连接到Redis服务器 %REDIS_HOST%:%REDIS_PORT%
    echo 请检查:
    echo 1. Redis服务器是否运行
    echo 2. 网络连接是否正常
    echo 3. 防火墙设置
    echo 4. Redis配置是否允许远程连接
    pause
    exit /b 1
)
echo ✓ Redis连接正常

REM 清理测试数据
echo 2. 清理测试数据...
if "!REDIS_PASSWORD!"=="" (
    redis-cli -h %REDIS_HOST% -p %REDIS_PORT% FLUSHDB >nul 2>&1
) else (
    redis-cli -h %REDIS_HOST% -p %REDIS_PORT% -a %REDIS_PASSWORD% FLUSHDB >nul 2>&1
)
echo ✓ 测试数据已清理

REM 设置环境变量供Go测试使用
set REDIS_ADDRESS=%REDIS_HOST%:%REDIS_PORT%
if not "!REDIS_PASSWORD!"=="" (
    set REDIS_AUTH=!REDIS_PASSWORD!
)

REM 运行基础功能测试
echo 3. 运行基础功能测试...
go test -v -run TestRedisLeaderboardRankCache_BasicOperations ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 基础功能测试失败
    pause
    exit /b 1
)
echo ✓ 基础功能测试通过

REM 运行分布式一致性测试
echo 4. 运行分布式一致性测试...
go test -v -run TestRedisLeaderboardRankCache_DistributedConsistency ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 分布式一致性测试失败
    pause
    exit /b 1
)
echo ✓ 分布式一致性测试通过

REM 运行网络分区测试
echo 5. 运行网络分区测试...
go test -v -run TestRedisLeaderboardRankCache_NetworkPartition ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 网络分区测试失败
    pause
    exit /b 1
)
echo ✓ 网络分区测试通过

REM 运行性能测试
echo 6. 运行性能测试...
go test -v -run TestRedisLeaderboardRankCache_Performance ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 性能测试失败
    pause
    exit /b 1
)
echo ✓ 性能测试通过

REM 运行故障恢复测试
echo 7. 运行故障恢复测试...
go test -v -run TestRedisLeaderboardRankCache_FaultTolerance ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 故障恢复测试失败
    pause
    exit /b 1
)
echo ✓ 故障恢复测试通过

REM 运行代数冲突测试
echo 8. 运行代数冲突测试...
go test -v -run TestRedisLeaderboardRankCache_GenerationConflict ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 代数冲突测试失败
    pause
    exit /b 1
)
echo ✓ 代数冲突测试通过

REM 运行健康检查测试
echo 9. 运行健康检查测试...
go test -v -run TestRedisLeaderboardRankCache_HealthCheck ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 健康检查测试失败
    pause
    exit /b 1
)
echo ✓ 健康检查测试通过

REM 运行基准测试
echo 10. 运行基准测试...
go test -v -bench=BenchmarkRedisLeaderboardRankCache ./server/ -redis-host=%REDIS_HOST% -redis-port=%REDIS_PORT% -redis-password=%REDIS_PASSWORD%
if %errorlevel% neq 0 (
    echo ✗ 基准测试失败
    pause
    exit /b 1
)
echo ✓ 基准测试通过

echo.
echo === 所有测试通过！ ===
echo Redis排行榜缓存一致性测试完成
echo 测试服务器: %REDIS_HOST%:%REDIS_PORT%
echo.
echo 测试总结:
echo - ✓ 基础功能测试
echo - ✓ 分布式一致性测试
echo - ✓ 网络分区测试
echo - ✓ 性能测试
echo - ✓ 故障恢复测试
echo - ✓ 代数冲突测试
echo - ✓ 健康检查测试
echo - ✓ 基准测试

pause 