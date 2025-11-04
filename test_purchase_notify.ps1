# 测试购买通知接口
# 使用外网访问

# 设置控制台编码为UTF-8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001 > $null

$url = "http://39.101.186.196:6000/v2/tiktok/purchase/notify"

# 准备测试数据
$site = "cjyx_cn"
$key = "11b18290a34e03da78900824fa59b140"
$uid = "test_user_001"
$order_money = "6.00"
$cp_order_id = "CP20250104001"
$time = [Math]::Floor([decimal](Get-Date(Get-Date).ToUniversalTime()-uformat "%s"))

# 计算签名: site + time + key + uid + order_money + cp_order_id
$signStr = "$site$time$key$uid$order_money$cp_order_id"
$md5 = New-Object System.Security.Cryptography.MD5CryptoServiceProvider
$utf8 = New-Object System.Text.UTF8Encoding
$hash = [System.BitConverter]::ToString($md5.ComputeHash($utf8.GetBytes($signStr)))
$sign = $hash.Replace("-", "").ToLower()

Write-Host "Sign String: $signStr" -ForegroundColor Yellow
Write-Host "Sign Result: $sign" -ForegroundColor Green
Write-Host ""

# 构造请求数据
$body = @{
    site = $site
    order_id = "ORDER20250104001"
    uid = $uid
    sid = "server_001"
    cp_order_id = $cp_order_id
    roleid = "role_001"
    rolename = "TestRole"
    order_money = $order_money
    productid = "product_001"
    time = $time
    sign = $sign
}

Write-Host "Sending request to: $url" -ForegroundColor Cyan
Write-Host "Request body:" -ForegroundColor Cyan
$body | ConvertTo-Json

try {
    # Send POST request
    $response = Invoke-WebRequest -Uri $url -Method POST -Body $body -ContentType "application/x-www-form-urlencoded"
    
    Write-Host "`nResponse Status: $($response.StatusCode)" -ForegroundColor Green
    Write-Host "Response Content: $($response.Content)" -ForegroundColor Green
} catch {
    Write-Host "`nRequest Failed!" -ForegroundColor Red
    Write-Host "Error Message: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response Body: $responseBody" -ForegroundColor Yellow
    }
}

