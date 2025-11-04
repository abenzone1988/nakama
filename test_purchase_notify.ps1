# Test purchase notify API
# Access via external network

# Set console encoding to UTF-8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$url = "http://39.101.186.196:6000/v2/tiktok/purchase/notify"

# Prepare test data
$site = "xjsmdyapp_android"
$key = "0c373fedcec01cff88aacdb8d2e28dc2"
$uid = "test_user_001"
$order_money = "6.00"
$cp_order_id = "CP20250104001"
$time = [Math]::Floor([decimal](Get-Date(Get-Date).ToUniversalTime()-uformat "%s"))

# Calculate signature: site + time + key + uid + order_money + cp_order_id
$signStr = "$site$time$key$uid$order_money$cp_order_id"
$md5 = New-Object System.Security.Cryptography.MD5CryptoServiceProvider
$utf8 = New-Object System.Text.UTF8Encoding
$hash = [System.BitConverter]::ToString($md5.ComputeHash($utf8.GetBytes($signStr)))
$sign = $hash.Replace("-", "").ToLower()

Write-Host "Sign String: $signStr" -ForegroundColor Yellow
Write-Host "Sign Result: $sign" -ForegroundColor Green
Write-Host ""

# Build request body
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

Write-Host "`nNote: Sending as form-urlencoded format" -ForegroundColor Gray

try {
    # Send POST request with form-urlencoded format
    # PowerShell will automatically convert hashtable to proper format
    $response = Invoke-WebRequest -Uri $url -Method POST -Body $body -ContentType "application/x-www-form-urlencoded" -UseBasicParsing
    
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
