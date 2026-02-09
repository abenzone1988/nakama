# encoding: utf-8
# 微信消息推送测试脚本 (PowerShell)

Write-Host "=== 微信消息推送接口测试 ===" -ForegroundColor Cyan
Write-Host ""

# 配置
$BASE_URL = "http://118.145.182.217:7350"
$API_PATH = "/v2/wechat/message/verify"
$TOKEN = "sparkinfi"
$USER_OPENID = "oOeFY7E2tGQqNnKOaa88mP4nMBfk"
$ITEM_ID = "60000"

# 测试结果统计
$TOTAL_TESTS = 0
$PASSED_TESTS = 0
$FAILED_TESTS = 0

# 生成签名函数
function Generate-Signature {
    param (
        [string]$timestamp,
        [string]$nonce
    )

    # 将token、timestamp、nonce按字典序排序后拼接
    $params = @($TOKEN, $timestamp, $nonce) | Sort-Object
    $str = $params -join ""

    # 计算SHA1
    $sha1 = [System.Security.Cryptography.SHA1]::Create()
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($str)
    $hash = $sha1.ComputeHash($bytes)
    $signature = ($hash | ForEach-Object { $_.ToString("x2") }) -join ""

    return $signature
}

# 测试函数
function Run-Test {
    param ([string]$testName)
    $script:TOTAL_TESTS++
    Write-Host "Test $script:TOTAL_TESTS`: $testName" -ForegroundColor Yellow
}

function Test-Passed {
    $script:PASSED_TESTS++
    Write-Host "[PASS] Test passed" -ForegroundColor Green
    Write-Host ""
}

function Test-Failed {
    param ([string]$reason)
    $script:FAILED_TESTS++
    Write-Host "[FAIL] Test failed: $reason" -ForegroundColor Red
    Write-Host ""
}

# ========================================
# Test 1: GET Signature Verification
# ========================================
Run-Test "GET Signature Verification"

$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "test_nonce_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$signature = Generate-Signature -timestamp $timestamp -nonce $nonce
$echostr = "hello_world"

Write-Host "  Timestamp: $timestamp"
Write-Host "  Nonce: $nonce"
Write-Host "  Signature: $signature"

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}&echostr={5}' -f $BASE_URL, $API_PATH, $signature, $timestamp, $nonce, $echostr
    $response = Invoke-RestMethod -Uri $url -Method Get

    if ($response -eq $echostr) {
        Test-Passed
    } else {
        Test-Failed "Expected '$echostr', got '$response'"
    }
} catch {
    Test-Failed "Request failed: $_"
}

# ========================================
# Test 2: Deliver Goods (minigame_deliver_goods)
# ========================================
Run-Test "Deliver Goods Test"

$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "deliver_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$signature = Generate-Signature -timestamp $timestamp -nonce $nonce

$deliverJson = @{
    ToUserName = "game_server"
    FromUserName = "wechat"
    CreateTime = [int][double]::Parse((Get-Date -UFormat %s))
    MsgType = "event"
    Event = "minigame_deliver_goods"
    MiniGame = @{
        OrderId = "test_order_$(Get-Date -Format 'yyyyMMddHHmmss')"
        IsPreview = 0
        ToUserOpenid = $USER_OPENID
        GoodsList = @(
            @{
                Id = $ITEM_ID
                Num = 10
            },
            @{
                Id = "10001"
                Num = 5
            }
        )
        Zone = 1
        GiftTypeId = 1
        GiftId = "CBgAAoXb6-hx2kb4vrq9mMP2tXCYy-Cmca2VjoRJHnCv4wtyo9B1YoJmvh14frdIM7Gfwk6sV_dfQ2n4"
        SendTime = [int][double]::Parse((Get-Date -UFormat %s))
    }
} | ConvertTo-Json -Depth 10

Write-Host "  Request JSON:"
Write-Host $deliverJson

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}' -f $BASE_URL, $API_PATH, $signature, $timestamp, $nonce
    $response = Invoke-RestMethod -Uri $url -Method Post -Body $deliverJson -ContentType "application/json"

    Write-Host "  Response: $($response | ConvertTo-Json -Compress)"

    if ($response.ErrCode -eq 0) {
        Test-Passed
    } else {
        Test-Failed "Delivery failed, ErrCode: $($response.ErrCode)"
    }
} catch {
    Test-Failed "Request failed: $_"
}

# ========================================
# Test 3: Query Reward (query_challenge_reward)
# ========================================
Run-Test "Query Reward Test"

$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "query_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$signature = Generate-Signature -timestamp $timestamp -nonce $nonce

$queryJson = @{
    ToUserName = "game_server"
    FromUserName = "wechat"
    CreateTime = [int][double]::Parse((Get-Date -UFormat %s))
    MsgType = "event"
    Event = "query_challenge_reward"
    QueryReward = @{
        ToUserOpenid = $USER_OPENID
        ItemID = $ITEM_ID
    }
} | ConvertTo-Json -Depth 10

Write-Host "  Request JSON:"
Write-Host $queryJson

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}' -f $BASE_URL, $API_PATH, $signature, $timestamp, $nonce
    $response = Invoke-RestMethod -Uri $url -Method Post -Body $queryJson -ContentType "application/json"

    Write-Host "  Response: $($response | ConvertTo-Json -Compress)"
    Write-Host "  Today Count: $($response.TodayCount)"
    Write-Host "  Total Count: $($response.TotalCount)"

    if ($response.ErrCode -eq 0) {
        Test-Passed
    } else {
        Test-Failed "Query failed, ErrCode: $($response.ErrCode)"
    }
} catch {
    Test-Failed "Request failed: $_"
}

# ========================================
# Test 4: Invalid Signature Test
# ========================================
Run-Test "Invalid Signature Test (Expected to Fail)"

$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "bad_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$badSignature = "0000000000000000000000000000000000000000"

$deliverJson = @{
    Event = "minigame_deliver_goods"
    MiniGame = @{
        OrderId = "test_bad"
        ToUserOpenid = $USER_OPENID
        GoodsList = @()
    }
} | ConvertTo-Json -Depth 10

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}' -f $BASE_URL, $API_PATH, $badSignature, $timestamp, $nonce
    $response = Invoke-RestMethod -Uri $url -Method Post -Body $deliverJson -ContentType "application/json" -ErrorAction Stop

    Write-Host "  Response: $($response | ConvertTo-Json -Compress)"

    if ($response.ErrCode -ne 0) {
        Test-Passed
    } else {
        Test-Failed "Should reject invalid signature"
    }
} catch {
    # Expected to fail
    Write-Host "  Response: Signature verification failed (expected behavior)"
    Test-Passed
}

# ========================================
# Test 5: Multiple Delivery Accumulation Test
# ========================================
Run-Test "Multiple Delivery Accumulation Test"

# First delivery
$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "multi1_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$signature = Generate-Signature -timestamp $timestamp -nonce $nonce

$deliverJson1 = @{
    Event = "minigame_deliver_goods"
    MiniGame = @{
        OrderId = "multi_order_1_$(Get-Date -Format 'yyyyMMddHHmmss')"
        ToUserOpenid = $USER_OPENID
        GoodsList = @(
            @{
                Id = $ITEM_ID
                Num = 5
            }
        )
        GiftId = "test_gift"
        SendTime = [int][double]::Parse((Get-Date -UFormat %s))
    }
} | ConvertTo-Json -Depth 10

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}' -f $BASE_URL, $API_PATH, $signature, $timestamp, $nonce
    $response1 = Invoke-RestMethod -Uri $url -Method Post -Body $deliverJson1 -ContentType "application/json"
    Write-Host "  First delivery: $($response1 | ConvertTo-Json -Compress)"
} catch {
    Write-Host "  First delivery failed: $_"
}

Start-Sleep -Seconds 1

# Second delivery
$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "multi2_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$signature = Generate-Signature -timestamp $timestamp -nonce $nonce

$deliverJson2 = @{
    Event = "minigame_deliver_goods"
    MiniGame = @{
        OrderId = "multi_order_2_$(Get-Date -Format 'yyyyMMddHHmmss')"
        ToUserOpenid = $USER_OPENID
        GoodsList = @(
            @{
                Id = $ITEM_ID
                Num = 3
            }
        )
        GiftId = "test_gift"
        SendTime = [int][double]::Parse((Get-Date -UFormat %s))
    }
} | ConvertTo-Json -Depth 10

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}' -f $BASE_URL, $API_PATH, $signature, $timestamp, $nonce
    $response2 = Invoke-RestMethod -Uri $url -Method Post -Body $deliverJson2 -ContentType "application/json"
    Write-Host "  Second delivery: $($response2 | ConvertTo-Json -Compress)"
} catch {
    Write-Host "  Second delivery failed: $_"
}

Start-Sleep -Seconds 1

# Query accumulation result
$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$nonce = "query_multi_$(Get-Date -Format 'yyyyMMddHHmmssfff')"
$signature = Generate-Signature -timestamp $timestamp -nonce $nonce

$queryJson = @{
    Event = "query_challenge_reward"
    QueryReward = @{
        ToUserOpenid = $USER_OPENID
        ItemID = $ITEM_ID
    }
} | ConvertTo-Json -Depth 10

try {
    $url = '{0}{1}?signature={2}&timestamp={3}&nonce={4}' -f $BASE_URL, $API_PATH, $signature, $timestamp, $nonce
    $response3 = Invoke-RestMethod -Uri $url -Method Post -Body $queryJson -ContentType "application/json"

    Write-Host "  Query result: $($response3 | ConvertTo-Json -Compress)"
    Write-Host "  Today accumulated: $($response3.TodayCount) (expected >= 8)"
    Write-Host "  Total accumulated: $($response3.TotalCount) (expected >= 8)"

    if ($response3.TodayCount -ge 8 -and $response3.TotalCount -ge 8) {
        Test-Passed
    } else {
        Test-Failed "Accumulation result incorrect"
    }
} catch {
    Test-Failed "Query failed: $_"
}

# ========================================
# Test Summary
# ========================================
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Test Summary" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Total Tests: $TOTAL_TESTS"
Write-Host "Passed: $PASSED_TESTS" -ForegroundColor Green
Write-Host "Failed: $FAILED_TESTS" -ForegroundColor Red
Write-Host ""

if ($FAILED_TESTS -eq 0) {
    Write-Host "All tests passed!" -ForegroundColor Green
    exit 0
} else {
    Write-Host "Some tests failed, please check" -ForegroundColor Red
    exit 1
}
