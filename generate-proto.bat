@echo off
REM Batch script to generate all Protocol Buffer files
REM Copyright 2024 The Nakama Authors

echo ================================
echo Generating Protocol Buffer files...
echo ================================
echo.

set PROJECT_ROOT=%~dp0
cd /d "%PROJECT_ROOT%"

REM 1. Generate nakama-common api
echo [1/4] Generating nakama-common api...
cd vendor\github.com\heroiclabs\nakama-common\api
protoc -I. --go_out=. --go_opt=paths=source_relative api.proto
if %ERRORLEVEL% neq 0 (
    echo   [X] nakama-common api generation failed
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo   [OK] nakama-common api generated successfully
echo.

REM 2. Generate game msg
echo [2/4] Generating game msg...
cd /d "%PROJECT_ROOT%\game"
go generate
if %ERRORLEVEL% neq 0 (
    echo   [X] game msg generation failed
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo   [OK] game msg generated successfully
echo.

REM 3. Generate apigrpc
echo [3/4] Generating apigrpc...
cd /d "%PROJECT_ROOT%\apigrpc"
go generate
if %ERRORLEVEL% neq 0 (
    echo   [X] apigrpc generation failed
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo   [OK] apigrpc generated successfully
echo.

REM 4. Generate console
echo [4/4] Generating console...
cd /d "%PROJECT_ROOT%\console"
go generate
if %ERRORLEVEL% neq 0 (
    echo   [X] console generation failed
    cd /d "%PROJECT_ROOT%"
    exit /b 1
)
echo   [OK] console generated successfully
echo.

REM Return to project root
cd /d "%PROJECT_ROOT%"

echo ================================
echo All Protocol Buffer files generated successfully!
echo ================================
