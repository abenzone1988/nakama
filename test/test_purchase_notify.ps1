# Purchase Notify Test Script - Universal Version
# Usage: .\test_purchase_notify.ps1 -Platform android
#        .\test_purchase_notify.ps1 -Platform ios
#        .\test_purchase_notify.ps1 -Platform all

param(
    [Parameter(Mandatory=$false)]
    [ValidateSet("android", "ios", "all")]
    [string]$Platform = "all",

    [Parameter(Mandatory=$false)]
    [string]$Url = "http://127.0.0.1:7350/v2/tiktok/purchase/notify"
)

# Platform configurations
$platforms = @{
    "android" = @{
        Site = "xjsmdyapp_android"
        Key = "0c373fedcec01cff88aacdb8d2e28dc2"
        DisplayName = "Android"
        PayType = 2  # 支付宝
    }
    "ios" = @{
        Site = "xjsmdyapp_ios"
        Key = "234c4ec9c31af40a9a0239b868f10dd8"
        DisplayName = "iOS"
        PayType = 1  # 苹果内购
    }
}

# Test function
function Test-Platform {
    param(
        [string]$PlatformKey
    )

    $config = $platforms[$PlatformKey]
    $timestamp = [Math]::Floor([decimal](Get-Date(Get-Date).ToUniversalTime()-uformat "%s"))

    # Test data
    $site = $config.Site
    $key = $config.Key
    $uid = "test_user_${PlatformKey}_001"
    $order_money = "6.00"
    $cp_order_id = "CP_${PlatformKey}_${timestamp}"
    $order_id = "ORDER_${PlatformKey}_${timestamp}"
    $pay_type = $config.PayType
    $ext = "test_ext_${PlatformKey}_${timestamp}"

    # Calculate signature: site + time + key + uid + order_money + cp_order_id
    $signStr = "$site$timestamp$key$uid$order_money$cp_order_id"
    $md5 = New-Object System.Security.Cryptography.MD5CryptoServiceProvider
    $utf8 = New-Object System.Text.UTF8Encoding
    $hash = [System.BitConverter]::ToString($md5.ComputeHash($utf8.GetBytes($signStr)))
    $sign = $hash.Replace("-", "").ToLower()

    Write-Host "=== $($config.DisplayName) Platform Test ===" -ForegroundColor Magenta
    Write-Host "Site: $site" -ForegroundColor Cyan
    Write-Host "Key: $key" -ForegroundColor Cyan
    Write-Host "Sign String: $signStr" -ForegroundColor Yellow
    Write-Host "Sign Result: $sign" -ForegroundColor Green
    Write-Host ""

    # Build request body
    $bodyString = "site=$site&order_id=$order_id&uid=$uid&sid=server_001&cp_order_id=$cp_order_id&roleid=role_${PlatformKey}_001&rolename=TestRole${PlatformKey}&order_money=$order_money&productid=product_${PlatformKey}_001&pay_type=$pay_type&ext=$ext&time=$timestamp&sign=$sign"

    Write-Host "Sending request to: $Url" -ForegroundColor Cyan
    Write-Host ""

    try {
        # Send POST request
        $response = Invoke-WebRequest -Uri $Url -Method POST -Body $bodyString -ContentType "application/x-www-form-urlencoded" -UseBasicParsing

        Write-Host "[SUCCESS] Response Status: $($response.StatusCode)" -ForegroundColor Green
        Write-Host "[SUCCESS] Response Content: $($response.Content)" -ForegroundColor Green
        return $true
    } catch {
        Write-Host "[FAILED] Request Failed!" -ForegroundColor Red
        Write-Host "Error Message: $($_.Exception.Message)" -ForegroundColor Red
        if ($_.Exception.Response) {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $responseBody = $reader.ReadToEnd()
            Write-Host "Response Body: $responseBody" -ForegroundColor Yellow
        }
        return $false
    }
}

# Main logic
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Purchase Notify Test Script" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$results = @{}

if ($Platform -eq "all") {
    # Test all platforms
    foreach ($key in $platforms.Keys) {
        $results[$key] = Test-Platform -PlatformKey $key
        Write-Host ""
        Write-Host "----------------------------------------" -ForegroundColor Gray
        Write-Host ""
        Start-Sleep -Milliseconds 500
    }

    # Display summary
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "Test Summary" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    foreach ($key in $results.Keys) {
        $status = if ($results[$key]) { "[PASS]" } else { "[FAIL]" }
        $color = if ($results[$key]) { "Green" } else { "Red" }
        Write-Host "$($platforms[$key].DisplayName): $status" -ForegroundColor $color
    }
} else {
    # Test specified platform
    $result = Test-Platform -PlatformKey $Platform
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    $status = if ($result) { "[TEST PASSED]" } else { "[TEST FAILED]" }
    $color = if ($result) { "Green" } else { "Red" }
    Write-Host $status -ForegroundColor $color
}

Write-Host "========================================" -ForegroundColor Cyan

