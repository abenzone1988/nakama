@echo off
REM ============================================================
REM  Deploy the Nginx TLS layer - hjzx.sfgames.io (AWS hhxj / us-east-1)
REM
REM  Installs or updates nginx on the EC2 host so that
REM    443 -> http://127.0.0.1:7350 (Nakama)
REM    80  -> 301 redirect to https
REM
REM  Usage:
REM    deploy-hjzx-nginx.bat                       full run (default cert files)
REM    deploy-hjzx-nginx.bat --skip-cert           push hjzx.conf only, keep the server cert
REM    deploy-hjzx-nginx.bat --cert-dir DIR        take certs from DIR instead
REM    deploy-hjzx-nginx.bat --fullchain NAME      fullchain filename inside --cert-dir
REM    deploy-hjzx-nginx.bat --key NAME            private key filename inside --cert-dir
REM    deploy-hjzx-nginx.bat --help
REM
REM  Renewing the certificate (expires 2027-04-12, no auto-renewal):
REM    drop the new fullchain + key over the default filenames and just
REM    re-run this script with no arguments. See AWS-部署运维说明.md 5.7.
REM
REM  Notes:
REM    - The private key is uploaded to /tmp on the server, installed as
REM      600 root, then shredded by install-hjzx-nginx.sh.
REM    - Keep this file ASCII-only: cmd.exe parses .bat in the system ANSI
REM      code page, so UTF-8 text corrupts the script.
REM ============================================================

setlocal enabledelayedexpansion
cd /d "%~dp0"

REM ---------- Settings ----------
set SSH_KEY=%USERPROFILE%\.nakama-deploy\game-app-key.pem
set SSH_HOST=ec2-user@13.221.177.184
set STAGE=/tmp/hjzx-nginx-stage

set CERT_DIR=%~dp0..\hjzx.sfgames.io_nginx
set FULLCHAIN_FILE=hjzx.sfgames.io.pem
set KEY_FILE=hjzx.sfgames.io.key
set SKIP_CERT=false

REM ---------- Parse arguments ----------
:parse_args
if "%~1"=="" goto :start
if /I "%~1"=="--cert-dir" (
    set CERT_DIR=%~2
    shift
    shift
    goto :parse_args
)
if /I "%~1"=="--fullchain" (
    set FULLCHAIN_FILE=%~2
    shift
    shift
    goto :parse_args
)
if /I "%~1"=="--key" (
    set KEY_FILE=%~2
    shift
    shift
    goto :parse_args
)
if /I "%~1"=="--skip-cert" (
    set SKIP_CERT=true
    shift
    goto :parse_args
)
if /I "%~1"=="--help" goto :show_help
echo [ERROR] Unknown argument: %~1
echo Run with --help for usage.
exit /b 1

:show_help
echo Usage: deploy-hjzx-nginx.bat [options]
echo.
echo   --cert-dir DIR    Directory holding the certificate files
echo                     (default: build\hjzx.sfgames.io_nginx)
echo   --fullchain NAME  Fullchain filename inside --cert-dir
echo                     (default: hjzx.sfgames.io.pem)
echo   --key NAME        Private key filename inside --cert-dir
echo                     (default: hjzx.sfgames.io.key)
echo   --skip-cert       Push hjzx.conf only; leave the server certificate alone
echo   --help            Show this help
goto :end

:start
echo ============================================================
echo  Nginx TLS layer deploy - hjzx.sfgames.io
echo  Target  : %SSH_HOST%
echo  CertDir : %CERT_DIR%
echo  Push cert: %SKIP_CERT% (false means the certificate is re-uploaded)
echo ============================================================
echo.

REM ---------- Preflight ----------
if not exist "%SSH_KEY%" (
    echo [ERROR] SSH key not found: %SSH_KEY%
    echo         The key is only issued once when the key pair is created.
    exit /b 1
)
if not exist "%~dp0hjzx.conf" (
    echo [ERROR] Config not found: %~dp0hjzx.conf
    exit /b 1
)
if /I "%SKIP_CERT%"=="false" (
    if not exist "%CERT_DIR%\%FULLCHAIN_FILE%" (
        echo [ERROR] Certificate not found: %CERT_DIR%\%FULLCHAIN_FILE%
        exit /b 1
    )
    if not exist "%CERT_DIR%\%KEY_FILE%" (
        echo [ERROR] Private key not found: %CERT_DIR%\%KEY_FILE%
        exit /b 1
    )
)

set SSH_OPTS=-i "%SSH_KEY%" -o StrictHostKeyChecking=no -o ConnectTimeout=20

REM ---------- 1) Prepare the staging directory ----------
echo [1/3] Preparing %STAGE% on the server...
ssh %SSH_OPTS% %SSH_HOST% "mkdir -p %STAGE%"
if errorlevel 1 (
    echo [ERROR] Cannot reach the server. Is the security group 22 rule still
    echo         pointing at this machine's egress IP? See the ops doc section 6.1.
    exit /b 1
)
echo.

REM ---------- 2) Upload config and certificate ----------
echo [2/3] Uploading config and certificate...
scp %SSH_OPTS% "%~dp0hjzx.conf" %SSH_HOST%:%STAGE%/hjzx.conf
if errorlevel 1 (
    echo [ERROR] Failed to upload hjzx.conf
    exit /b 1
)

if /I "%SKIP_CERT%"=="true" (
    echo [SKIP] Certificate upload skipped, the server keeps its current cert
) else (
    scp %SSH_OPTS% "%CERT_DIR%\%FULLCHAIN_FILE%" %SSH_HOST%:%STAGE%/fullchain.pem
    if errorlevel 1 (
        echo [ERROR] Failed to upload the certificate
        exit /b 1
    )
    scp %SSH_OPTS% "%CERT_DIR%\%KEY_FILE%" %SSH_HOST%:%STAGE%/privkey.pem
    if errorlevel 1 (
        echo [ERROR] Failed to upload the private key
        exit /b 1
    )
    echo [OK] hjzx.conf, fullchain.pem, privkey.pem uploaded
)
echo.

REM ---------- 3) Install on the server ----------
echo [3/3] Installing on the server...
echo ------------------------------------------------------------
scp %SSH_OPTS% "%~dp0install-hjzx-nginx.sh" %SSH_HOST%:/tmp/install-hjzx-nginx.sh
if errorlevel 1 (
    echo [ERROR] Failed to upload install-hjzx-nginx.sh
    exit /b 1
)
REM The tr -d '\r' is required: this file has CRLF endings on Windows and
REM sh would choke on the trailing \r with "not found" errors.
ssh %SSH_OPTS% %SSH_HOST% "tr -d '\r' < /tmp/install-hjzx-nginx.sh > /tmp/i.sh && sh /tmp/i.sh"
if errorlevel 1 (
    echo ------------------------------------------------------------
    echo [ERROR] Remote install failed. See the output above.
    echo         The script rolls hjzx.conf back automatically if nginx -t fails.
    exit /b 1
)
echo ------------------------------------------------------------
echo.
echo Done.
echo   https://hjzx.sfgames.io/         (443 -^> Nakama 7350)
echo   http://hjzx.sfgames.io/          (80  -^> 301 to https)
echo.
echo Remember: the certificate expires 2027-04-12. No auto-renewal.

:end
endlocal
