# Test purchase notify API for iOS
# Access via external network

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$url = "http://39.101.186.196:6000/v2/tiktok/purchase/notify"

# Prepare test data for iOS
$site = "xjsmdyapp_ios"
$key = "0c373fedcec01cff88aacdb8d2e28dc2"
$uid = "test_user_ios_001"
$order_money = "6.00"
$cp_order_id = "CP_IOS_20250104001"
$time = [Math]::Floor([decimal](Get-Date(Get-Date).ToUniversalTime()-uformat "%s"))

# Calculate signature: site + time + key + uid + order_money + cp_order_id
$signStr = "$site$time$key$uid$order_money$cp_order_id"
$md5 = New-Object System.Security.Cryptography.MD5CryptoServiceProvider
$utf8 = New-Object System.Text.UTF8Encoding
$hash = [System.BitConverter]::ToString($md5.ComputeHash($utf8.GetBytes($signStr)))
$sign = $hash.Replace("-", "").ToLower()

Write-Host "=== iOS Platform Test ===" -ForegroundColor Magenta
Write-Host "Sign String: $signStr" -ForegroundColor Yellow
Write-Host "Sign Result: $sign" -ForegroundColor Green
Write-Host ""

# Manually construct form-urlencoded body
$bodyString = "site=$site&order_id=ORDER_IOS_20250104001&uid=$uid&sid=server_001&cp_order_id=$cp_order_id&roleid=role_ios_001&rolename=TestRoleIOS&order_money=$order_money&productid=product_ios_001&time=$time&sign=$sign"

Write-Host "Sending request to: $url" -ForegroundColor Cyan
Write-Host "Request body string: $bodyString" -ForegroundColor Cyan
Write-Host ""

try {
    # Send POST request with manually constructed body
    $response = Invoke-WebRequest -Uri $url -Method POST -Body $bodyString -ContentType "application/x-www-form-urlencoded" -UseBasicParsing
    
    Write-Host "Response Status: $($response.StatusCode)" -ForegroundColor Green
    Write-Host "Response Content: $($response.Content)" -ForegroundColor Green
} catch {
    Write-Host "Request Failed!" -ForegroundColor Red
    Write-Host "Error Message: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response Body: $responseBody" -ForegroundColor Yellow
    }
}

