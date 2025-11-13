# PowerShell script to generate all Protocol Buffer files
# Copyright 2024 The Nakama Authors

Write-Host "================================" -ForegroundColor Cyan
Write-Host "Generating Protocol Buffer files..." -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot

# 1. Generate nakama-common api
Write-Host "[1/4] Generating nakama-common api..." -ForegroundColor Yellow
Set-Location "$projectRoot\vendor\github.com\heroiclabs\nakama-common\api"
try {
    protoc -I. --go_out=. --go_opt=paths=source_relative api.proto
    if ($LASTEXITCODE -ne 0) {
        throw "protoc failed with exit code $LASTEXITCODE"
    }
    Write-Host "  -> nakama-common api generated successfully" -ForegroundColor Green
} catch {
    Write-Host "  -> nakama-common api generation failed: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""


# 2. Generate game msg
Write-Host "[2/4] Generating game msg..." -ForegroundColor Yellow
Set-Location "$projectRoot\game"
try {
    go generate
    if ($LASTEXITCODE -ne 0) {
        throw "go generate failed with exit code $LASTEXITCODE"
    }
    Write-Host "  -> game msg generated successfully" -ForegroundColor Green
} catch {
    Write-Host "  -> game msg generation failed: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""

# 3. Generate apigrpc
Write-Host "[3/4] Generating apigrpc..." -ForegroundColor Yellow
Set-Location "$projectRoot\apigrpc"
try {
    go generate
    if ($LASTEXITCODE -ne 0) {
        throw "go generate failed with exit code $LASTEXITCODE"
    }
    Write-Host "  -> apigrpc generated successfully" -ForegroundColor Green
} catch {
    Write-Host "  -> apigrpc generation failed: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""

# 4. Generate console
Write-Host "[4/4] Generating console..." -ForegroundColor Yellow
Set-Location "$projectRoot\console"
try {
    go generate
    if ($LASTEXITCODE -ne 0) {
        throw "go generate failed with exit code $LASTEXITCODE"
    }
    Write-Host "  -> console generated successfully" -ForegroundColor Green
} catch {
    Write-Host "  -> console generation failed: $_" -ForegroundColor Red
    Set-Location $projectRoot
    exit 1
}
Write-Host ""

# Return to project root
Set-Location $projectRoot

Write-Host "================================" -ForegroundColor Cyan
Write-Host "All Protocol Buffer files generated successfully!" -ForegroundColor Green
Write-Host "================================" -ForegroundColor Cyan
