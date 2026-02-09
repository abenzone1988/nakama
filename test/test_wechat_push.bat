@echo off
REM Wechat Push Test Script (Windows Batch)

REM Set console to UTF-8 encoding
chcp 65001 >nul

echo Running Wechat Push Tests...
echo.

REM Execute PowerShell script
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0test_wechat_push.ps1"

REM 获取退出码
set exit_code=%errorlevel%

echo.
if %exit_code% equ 0 (
    echo Tests completed successfully!
) else (
    echo Tests failed!
)

exit /b %exit_code%
