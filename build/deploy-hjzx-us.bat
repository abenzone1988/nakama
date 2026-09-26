@echo off
REM ============================================================
REM  One-click deploy - Nakama HJZX US (AWS hhxj / us-east-1)
REM
REM  Three steps:
REM    1. Build and push the image locally (calls build-hjzx-us.bat)
REM    2. Upload docker-compose.yml / local.yml / remote-deploy.sh
REM    3. On the server: docker compose pull && up -d, wait for health
REM
REM  Usage:
REM    deploy-hjzx-us.bat                    full flow (version from git tag)
REM    deploy-hjzx-us.bat --version 1.0.1    full flow with explicit version
REM    deploy-hjzx-us.bat --skip-build       no build; upload config and restart only
REM    deploy-hjzx-us.bat --skip-config      keep the server's compose and local.yml
REM    deploy-hjzx-us.bat --help
REM
REM  Notes:
REM    - The image tag is :latest, so the local build MUST be pushed for the
REM      server to pick it up. The version only sets the embedded version
REM      string and the :<version> tag; compose always uses :latest.
REM    - local.yml holds the DB password and every key. It is pushed from
REM      .nakama-deploy on this machine and must never enter the repo.
REM    - Keep this file ASCII-only: cmd.exe parses .bat in the system ANSI
REM      code page, so UTF-8 text corrupts the script.
REM ============================================================

setlocal enabledelayedexpansion
cd /d "%~dp0"

REM ---------- Settings ----------
set SSH_KEY=%USERPROFILE%\.nakama-deploy\game-app-key.pem
set DEPLOY_DIR=%USERPROFILE%\.nakama-deploy
set SSH_HOST=ec2-user@13.221.177.184
set REMOTE_DIR=/opt/nakama

set VERSION_ARG=
set SKIP_BUILD=false
set SKIP_CONFIG=false

REM ---------- Parse arguments ----------
:parse_args
if "%~1"=="" goto :start
if /I "%~1"=="--version" (
    set VERSION_ARG=--version %~2
    shift
    shift
    goto :parse_args
)
if /I "%~1"=="--skip-build" (
    set SKIP_BUILD=true
    shift
    goto :parse_args
)
if /I "%~1"=="--skip-config" (
    set SKIP_CONFIG=true
    shift
    goto :parse_args
)
if /I "%~1"=="--help" goto :show_help
echo [ERROR] Unknown argument: %~1
echo Run with --help for usage.
exit /b 1

:show_help
echo Usage: deploy-hjzx-us.bat [options]
echo.
echo   --version VERSION   Version number, passed through to build-hjzx-us.bat
echo   --skip-build        Skip the local build and push
echo   --skip-config       Do not overwrite docker-compose.yml / local.yml on the server
echo   --help              Show this help
goto :end

:start
echo ============================================================
echo  Nakama HJZX US deploy
echo  Target : %SSH_HOST%
echo  Build  : %SKIP_BUILD%
echo  Config : %SKIP_CONFIG%
echo ============================================================
echo.

REM ---------- Preflight ----------
if not exist "%SSH_KEY%" (
    echo [ERROR] SSH key not found: %SSH_KEY%
    echo         The key is only issued once when the key pair is created.
    exit /b 1
)

if /I "%SKIP_CONFIG%"=="false" (
    if not exist "%DEPLOY_DIR%\bundle\data\local.yml" (
        echo [ERROR] Config not found: %DEPLOY_DIR%\bundle\data\local.yml
        exit /b 1
    )
)

REM ---------- 1) Build and push ----------
if /I "%SKIP_BUILD%"=="true" (
    echo [1/3] Skipping build, using the existing :latest image
) else (
    echo [1/3] Building and pushing the image...
    call "%~dp0build-hjzx-us.bat" %VERSION_ARG%
    if errorlevel 1 (
        echo.
        echo [ERROR] Build or push failed. Aborted; the server was not touched.
        exit /b 1
    )
)
echo.

REM ---------- 2) Upload config ----------
set SSH_OPTS=-i "%SSH_KEY%" -o StrictHostKeyChecking=no -o ConnectTimeout=20

if /I "%SKIP_CONFIG%"=="true" (
    echo [2/3] Skipping config upload, keeping the server's compose and local.yml
) else (
    echo [2/3] Uploading config...
    scp %SSH_OPTS% "%~dp0docker-compose.prod.yml" %SSH_HOST%:%REMOTE_DIR%/docker-compose.yml
    if errorlevel 1 (
        echo [ERROR] Failed to upload docker-compose.yml
        exit /b 1
    )

    scp %SSH_OPTS% "%DEPLOY_DIR%\bundle\data\local.yml" %SSH_HOST%:%REMOTE_DIR%/local.yml
    if errorlevel 1 (
        echo [ERROR] Failed to upload local.yml
        exit /b 1
    )
    echo [OK] docker-compose.yml and local.yml uploaded
)

REM remote-deploy.sh is uploaded every run so the logic always matches the repo
scp %SSH_OPTS% "%~dp0remote-deploy.sh" %SSH_HOST%:/tmp/remote-deploy.sh
if errorlevel 1 (
    echo [ERROR] Failed to upload remote-deploy.sh
    exit /b 1
)
echo.

REM ---------- 3) Pull and restart on the server ----------
echo [3/3] Pulling the image and restarting on the server...
echo ------------------------------------------------------------
REM The tr -d '\r' is required: this file may have CRLF endings on Windows,
REM and sh would choke on the trailing \r with "not found" errors.
ssh %SSH_OPTS% %SSH_HOST% "tr -d '\r' < /tmp/remote-deploy.sh > /tmp/rd.sh && sh /tmp/rd.sh"
if errorlevel 1 (
    echo ------------------------------------------------------------
    echo [ERROR] Remote deploy failed. See the output above.
    echo         Rollback: *.bak-* backups live in /opt/nakama on the server.
    exit /b 1
)
echo ------------------------------------------------------------
echo.
echo Deploy succeeded.
echo Console: http://13.221.177.184:7351/

:end
endlocal
