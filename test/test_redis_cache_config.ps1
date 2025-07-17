# Redis排行榜缓存一致性测试脚本 (配置文件版本)

param(
    [string]$ConfigFile = "redis_test_config.json"
)

Write-Host "=== Redis排行榜缓存一致性测试 (配置文件版本) ===" -ForegroundColor Green

# 检查配置文件是否存在
if (-not (Test-Path $ConfigFile)) {
    Write-Host "错误: 配置文件 $ConfigFile 不存在" -ForegroundColor Red
    Write-Host "请创建配置文件或使用默认配置" -ForegroundColor Yellow
    exit 1
}

# 读取配置文件
try {
    $config = Get-Content $ConfigFile -Raw | ConvertFrom-Json
    Write-Host "✓ 配置文件加载成功: $ConfigFile" -ForegroundColor Green
} catch {
    Write-Host "错误: 配置文件格式错误" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# 设置Redis连接配置
$redisHost = $config.redis.host
$redisPort = $config.redis.port
$redisPassword = $config.redis.password
$redisDB = $config.redis.db
$redisTimeout = $config.redis.timeout

Write-Host "Redis配置:" -ForegroundColor Cyan
Write-Host "  主机: $redisHost" -ForegroundColor Gray
Write-Host "  端口: $redisPort" -ForegroundColor Gray
Write-Host "  数据库: $redisDB" -ForegroundColor Gray
Write-Host "  超时: ${redisTimeout}ms" -ForegroundColor Gray

# 检查是否要修改Redis配置
$changeConfig = Read-Host "`n是否修改Redis配置？(y/N)"
if ($changeConfig -eq "y" -or $changeConfig -eq "Y") {
    $inputHost = Read-Host "请输入Redis主机地址 (当前: $redisHost)"
    if ($inputHost) { $redisHost = $inputHost }
    
    $inputPort = Read-Host "请输入Redis端口 (当前: $redisPort)"
    if ($inputPort) { $redisPort = $inputPort }
    
    $usePassword = Read-Host "是否使用密码？(y/N)"
    if ($usePassword -eq "y" -or $usePassword -eq "Y") {
        $redisPassword = Read-Host "请输入Redis密码" -AsSecureString
        $redisPassword = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($redisPassword))
    }
}

# 检查Redis是否运行
Write-Host "`n1. 检查Redis连接..." -ForegroundColor Yellow
try {
    if ($redisPassword) {
        $redisResponse = redis-cli -h $redisHost -p $redisPort -a $redisPassword ping 2>$null
    } else {
        $redisResponse = redis-cli -h $redisHost -p $redisPort ping 2>$null
    }
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ Redis连接正常" -ForegroundColor Green
    } else {
        throw "Redis连接失败"
    }
} catch {
    Write-Host "错误: 无法连接到Redis服务器 $redisHost`:$redisPort" -ForegroundColor Red
    Write-Host "请检查:" -ForegroundColor Yellow
    Write-Host "1. Redis服务器是否运行" -ForegroundColor Yellow
    Write-Host "2. 网络连接是否正常" -ForegroundColor Yellow
    Write-Host "3. 防火墙设置" -ForegroundColor Yellow
    Write-Host "4. Redis配置是否允许远程连接" -ForegroundColor Yellow
    Read-Host "按任意键退出"
    exit 1
}

# 清理测试数据
if ($config.test.cleanup_before_test) {
    Write-Host "2. 清理测试数据..." -ForegroundColor Yellow
    if ($redisPassword) {
        redis-cli -h $redisHost -p $redisPort -a $redisPassword FLUSHDB >$null 2>&1
    } else {
        redis-cli -h $redisHost -p $redisPort FLUSHDB >$null 2>&1
    }
    Write-Host "✓ 测试数据已清理" -ForegroundColor Green
}

# 设置环境变量供Go测试使用
$env:REDIS_HOST = $redisHost
$env:REDIS_PORT = $redisPort
$env:REDIS_PASSWORD = $redisPassword
$env:REDIS_DB = $redisDB
$env:REDIS_TIMEOUT = $redisTimeout

# 定义测试函数
function Run-Test {
    param(
        [string]$TestName,
        [string]$TestPattern,
        [string]$Description
    )
    
    Write-Host "$TestName..." -ForegroundColor Yellow
    Write-Host "  描述: $Description" -ForegroundColor Gray
    Write-Host "  服务器: $redisHost`:$redisPort" -ForegroundColor Gray
    
    $testArgs = @("-v", "-run", $TestPattern, "./server/")
    if ($redisHost -ne "localhost") {
        $testArgs += "-redis-host=$redisHost"
        $testArgs += "-redis-port=$redisPort"
        if ($redisPassword) {
            $testArgs += "-redis-password=$redisPassword"
        }
    }
    
    $result = & go test @testArgs 2>&1
    $exitCode = $LASTEXITCODE
    
    if ($exitCode -eq 0) {
        Write-Host "  ✓ $TestName 通过" -ForegroundColor Green
        return $true
    } else {
        Write-Host "  ✗ $TestName 失败" -ForegroundColor Red
        Write-Host "  错误输出:" -ForegroundColor Red
        Write-Host $result -ForegroundColor Red
        return $false
    }
}

# 运行所有测试
$tests = @(
    @{
        Name = "基础功能测试"
        Pattern = "TestRedisLeaderboardRankCache_BasicOperations"
        Description = "验证基本的插入、查询、删除功能"
    },
    @{
        Name = "分布式一致性测试"
        Pattern = "TestRedisLeaderboardRankCache_DistributedConsistency"
        Description = "验证多节点环境下的数据一致性"
    },
    @{
        Name = "网络分区测试"
        Pattern = "TestRedisLeaderboardRankCache_NetworkPartition"
        Description = "验证网络分区情况下的系统行为"
    },
    @{
        Name = "性能测试"
        Pattern = "TestRedisLeaderboardRankCache_Performance"
        Description = "验证系统在高负载下的性能表现"
    },
    @{
        Name = "故障恢复测试"
        Pattern = "TestRedisLeaderboardRankCache_FaultTolerance"
        Description = "验证系统在故障情况下的恢复能力"
    },
    @{
        Name = "代数冲突测试"
        Pattern = "TestRedisLeaderboardRankCache_GenerationConflict"
        Description = "验证代数机制防止数据回退"
    },
    @{
        Name = "健康检查测试"
        Pattern = "TestRedisLeaderboardRankCache_HealthCheck"
        Description = "验证健康检查和监控功能"
    }
)

$testNumber = 3
$allTestsPassed = $true
$testResults = @()

foreach ($test in $tests) {
    Write-Host "`n$testNumber. 运行$($test.Name)..." -ForegroundColor Cyan
    $startTime = Get-Date
    $success = Run-Test -TestName $test.Name -TestPattern $test.Pattern -Description $test.Description
    $endTime = Get-Date
    $duration = $endTime - $startTime
    
    $testResults += @{
        Name = $test.Name
        Success = $success
        Duration = $duration
    }
    
    if (-not $success) {
        $allTestsPassed = $false
        Write-Host "`n测试失败，是否继续运行其他测试？" -ForegroundColor Yellow
        $continue = Read-Host "输入 'y' 继续，其他键退出"
        if ($continue -ne 'y') {
            exit 1
        }
    }
    
    $testNumber++
}

# 运行基准测试
Write-Host "`n$testNumber. 运行基准测试..." -ForegroundColor Cyan
Write-Host "  描述: 验证系统性能基准" -ForegroundColor Gray
Write-Host "  服务器: $redisHost`:$redisPort" -ForegroundColor Gray

$benchmarkArgs = @("-v", "-bench=BenchmarkRedisLeaderboardRankCache", "./server/")
if ($redisHost -ne "localhost") {
    $benchmarkArgs += "-redis-host=$redisHost"
    $benchmarkArgs += "-redis-port=$redisPort"
    if ($redisPassword) {
        $benchmarkArgs += "-redis-password=$redisPassword"
    }
}

$benchmarkStartTime = Get-Date
$benchmarkResult = & go test @benchmarkArgs 2>&1
$benchmarkExitCode = $LASTEXITCODE
$benchmarkEndTime = Get-Date
$benchmarkDuration = $benchmarkEndTime - $benchmarkStartTime

if ($benchmarkExitCode -eq 0) {
    Write-Host "  ✓ 基准测试通过" -ForegroundColor Green
    $testResults += @{
        Name = "基准测试"
        Success = $true
        Duration = $benchmarkDuration
    }
} else {
    Write-Host "  ✗ 基准测试失败" -ForegroundColor Red
    Write-Host "  错误输出:" -ForegroundColor Red
    Write-Host $benchmarkResult -ForegroundColor Red
    $allTestsPassed = $false
    $testResults += @{
        Name = "基准测试"
        Success = $false
        Duration = $benchmarkDuration
    }
}

# 输出测试总结
Write-Host "`n=== 测试总结 ===" -ForegroundColor Green
if ($allTestsPassed) {
    Write-Host "✓ 所有测试通过！" -ForegroundColor Green
    Write-Host "Redis排行榜缓存一致性测试完成" -ForegroundColor Green
    Write-Host "测试服务器: $redisHost`:$redisPort" -ForegroundColor Green
    
    Write-Host "`n测试结果:" -ForegroundColor Cyan
    foreach ($result in $testResults) {
        $status = if ($result.Success) { "✓" } else { "✗" }
        $color = if ($result.Success) { "Green" } else { "Red" }
        Write-Host "  - $status $($result.Name) ($($result.Duration.TotalSeconds.ToString('F2'))s)" -ForegroundColor $color
    }
} else {
    Write-Host "✗ 部分测试失败" -ForegroundColor Red
    Write-Host "请检查失败原因并修复后重新运行测试" -ForegroundColor Yellow
    
    Write-Host "`n失败测试:" -ForegroundColor Red
    foreach ($result in $testResults) {
        if (-not $result.Success) {
            Write-Host "  - ✗ $($result.Name)" -ForegroundColor Red
        }
    }
}

# 性能统计（如果Redis可用）
try {
    Write-Host "`n=== Redis性能统计 ===" -ForegroundColor Cyan
    if ($redisPassword) {
        $redisInfo = redis-cli -h $redisHost -p $redisPort -a $redisPassword INFO 2>$null
    } else {
        $redisInfo = redis-cli -h $redisHost -p $redisPort INFO 2>$null
    }
    
    if ($LASTEXITCODE -eq 0) {
        $lines = $redisInfo -split "`n"
        $memoryInfo = $lines | Where-Object { $_ -match "^used_memory:" }
        $connectedClients = $lines | Where-Object { $_ -match "^connected_clients:" }
        $totalCommands = $lines | Where-Object { $_ -match "^total_commands_processed:" }
        $uptime = $lines | Where-Object { $_ -match "^uptime_in_seconds:" }
        
        Write-Host "  内存使用: $memoryInfo" -ForegroundColor Gray
        Write-Host "  连接数: $connectedClients" -ForegroundColor Gray
        Write-Host "  总命令数: $totalCommands" -ForegroundColor Gray
        Write-Host "  运行时间: $uptime" -ForegroundColor Gray
    }
} catch {
    Write-Host "  无法获取Redis性能统计" -ForegroundColor Yellow
}

# 保存测试报告
$reportFile = "test_report_$(Get-Date -Format 'yyyyMMdd_HHmmss').json"
$report = @{
    timestamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
    redis_config = @{
        host = $redisHost
        port = $redisPort
        db = $redisDB
    }
    test_results = $testResults
    overall_success = $allTestsPassed
}

$report | ConvertTo-Json -Depth 3 | Out-File $reportFile -Encoding UTF8
Write-Host "`n测试报告已保存: $reportFile" -ForegroundColor Green

Write-Host "`n测试完成！" -ForegroundColor Green
Read-Host "按任意键退出" 