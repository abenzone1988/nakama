# Test purchase notify API for all platforms
# Test both Android and iOS

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Testing All Platforms" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Test Android
Write-Host ">>> Testing Android Platform <<<" -ForegroundColor Yellow
Write-Host ""
& .\test_purchase_notify_android.ps1

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Test iOS
Write-Host ">>> Testing iOS Platform <<<" -ForegroundColor Yellow
Write-Host ""
& .\test_purchase_notify_ios.ps1

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "All tests completed!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan

