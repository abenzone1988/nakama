# 生成所有 Protocol Buffer 文件的 PowerShell 脚本
# Copyright 2024 The Nakama Authors

# 设置控制台输出编码为 UTF-8 以正确显示中文
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "================================" -ForegroundColor Cyan
Write-Host "开始生成 Protocol Buffer 文件..." -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot

# 1. 生成 nakama-common api
Write-Host "[1/3] 生成 nakama-common api..." -ForegroundColor Yellow
Set-Location "$projectRoot\vendor\github.com\heroiclabs\nakama-common\api"
try {
    protoc -I. --go_out=. --go_opt=paths=source_relative api.proto
    Write-Host "✓ nakama-common api 生成成功" -ForegroundColor Green
} catch {
    Write-Host "✗ nakama-common api 生成失败: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""

# 2. 生成 apigrpc
Write-Host "[2/3] 生成 apigrpc..." -ForegroundColor Yellow
Set-Location "$projectRoot\apigrpc"
try {
    go generate
    Write-Host "✓ apigrpc 生成成功" -ForegroundColor Green
} catch {
    Write-Host "✗ apigrpc 生成失败: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""

# 3. 生成 console
Write-Host "[3/3] 生成 console..." -ForegroundColor Yellow
Set-Location "$projectRoot\console"
try {
    go generate
    Write-Host "✓ console 生成成功" -ForegroundColor Green
} catch {
    Write-Host "✗ console 生成失败: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""

# 返回项目根目录
Set-Location $projectRoot

Write-Host "================================" -ForegroundColor Cyan
Write-Host "所有 Protocol Buffer 文件生成完成!" -ForegroundColor Green
Write-Host "================================" -ForegroundColor Cyan

