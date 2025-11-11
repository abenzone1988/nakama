@echo off
REM 生成所有 Protocol Buffer 文件的批处理脚本
REM Copyright 2024 The Nakama Authors

REM 设置控制台代码页为 UTF-8 以正确显示中文
chcp 65001 >nul 2>&1

echo ================================
echo 开始生成 Protocol Buffer 文件...
echo ================================
echo.

set PROJECT_ROOT=%~dp0
cd /d "%PROJECT_ROOT%"

REM 1. 生成 nakama-common api
echo [1/3] 生成 nakama-common api...
cd vendor\github.com\heroiclabs\nakama-common\api
protoc -I. --go_out=. --go_opt=paths=source_relative api.proto
if %ERRORLEVEL% neq 0 (
    echo X nakama-common api 生成失败
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo √ nakama-common api 生成成功
echo.

REM 2. 生成 apigrpc
echo [2/3] 生成 apigrpc...
cd /d "%PROJECT_ROOT%\apigrpc"
go generate
if %ERRORLEVEL% neq 0 (
    echo X apigrpc 生成失败
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo √ apigrpc 生成成功
echo.

REM 3. 生成 console
echo [3/3] 生成 console...
cd /d "%PROJECT_ROOT%\console"
go generate
if %ERRORLEVEL% neq 0 (
    echo X console 生成失败
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo √ console 生成成功
echo.

REM 返回项目根目录
cd /d "%PROJECT_ROOT%"

echo ================================
echo 所有 Protocol Buffer 文件生成完成!
echo ================================

