@echo off
REM Nakama Docker Build Script - HJZX (Alloy Frontline) US Version (Windows Batch Version)
REM Based on build-xjsm-us.bat, adapted to this project's single-stage Dockerfile
REM Also pre-pulls the base images: BuildKit resolves FROM tags from the CLI
REM process, which does not use the Docker Desktop proxy, so on hosts with
REM unreliable Docker Hub DNS the build dies with "failed to fetch anonymous token".
REM Author: Jiangp
REM Version: v1.0.0

setlocal enabledelayedexpansion

cd /d "%~dp0"

echo === Nakama Docker Build Script - HJZX US (Windows) ===
echo Image: hjzx/nakama-us
echo.

REM Get version information
set VERSION=
for /f "tokens=*" %%i in ('git describe --tags --abbrev=0 2^>nul') do set VERSION=%%i
if "%VERSION%"=="" set VERSION=1.0.0dev
REM Remove 'v' prefix from version only if it starts with 'v'
IF /I "%VERSION:~0,1%"=="v" set VERSION=%VERSION:~1%

REM Get commit information
set COMMIT=
for /f "tokens=*" %%i in ('git rev-parse --short HEAD 2^>nul') do set COMMIT=%%i
if "%COMMIT%"=="" set COMMIT=unknown

REM Set default parameters
set REGISTRY=docker.sparkinfi.com
set IMAGE_NAME=hjzx/nakama-us
set PLATFORM=linux/amd64
set NO_PUSH=false

REM Parse command line arguments
:parse_args
if "%1"=="" goto :start_build
if "%1"=="--version" (
    set VERSION=%2
    shift
    shift
    goto :parse_args
)
if "%1"=="--registry" (
    set REGISTRY=%2
    shift
    shift
    goto :parse_args
)
if "%1"=="--image-name" (
    set IMAGE_NAME=%2
    shift
    shift
    goto :parse_args
)
if "%1"=="--no-push" (
    set NO_PUSH=true
    shift
    goto :parse_args
)
if "%1"=="--platform" (
    REM NOTE: cmd splits unquoted arguments on commas, so multi-platform values
    REM must be quoted, e.g. --platform "linux/amd64,linux/arm64". Strip the
    REM quotes again so docker receives a clean value.
    set PLATFORM=%2
    set PLATFORM=!PLATFORM:"=!
    shift
    shift
    goto :parse_args
)
if "%1"=="--help" (
    goto :show_help
)
shift
goto :parse_args

:show_help
echo Usage: build-hjzx-us.bat [options]
echo.
echo Options:
echo   --version VERSION     Set version number (default: from git tag)
echo   --registry REGISTRY   Set registry address (default: docker.sparkinfi.com)
echo   --image-name NAME     Set image name (default: hjzx/nakama-us)
echo   --platform PLATFORM   Set target platform (default: linux/amd64)
echo                         Multi-platform must be quoted: "linux/amd64,linux/arm64"
echo   --no-push             Build only, do not push
echo   --help                Show this help message
echo.
echo Examples:
echo   build-hjzx-us.bat
echo   build-hjzx-us.bat --version 2.1.0 --registry myregistry.com
echo   build-hjzx-us.bat --no-push
goto :end

:start_build
echo Version: %VERSION%
echo Commit: %COMMIT%
echo Registry: %REGISTRY%
echo Platform: %PLATFORM%
if /I "%NO_PUSH%"=="true" (
    echo Push: disabled ^(build only^)
) else (
    echo Push: enabled
)
echo.

REM Check if Docker is available
echo Checking Docker environment...
docker --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker not installed or not available
    exit /b 1
)
echo [OK] Docker is available

REM Check Docker Buildx
docker buildx version >nul 2>&1
if errorlevel 1 (
    echo [WARNING] Docker Buildx not available, trying to create builder...
    docker buildx create --name nakama-builder --use >nul 2>&1
    if errorlevel 1 (
        echo [ERROR] Cannot create builder
        exit /b 1
    )
    echo [OK] Builder created successfully
) else (
    echo [OK] Docker Buildx is available
)

REM Set full image name
set FULL_IMAGE_NAME=%REGISTRY%/%IMAGE_NAME%

echo.
echo Setting up build cache...
if not exist "C:\tmp\.buildx-cache" mkdir "C:\tmp\.buildx-cache"
if not exist "C:\tmp\.buildx-cache-new" mkdir "C:\tmp\.buildx-cache-new"
echo [OK] Build cache directories created

REM Pre-pull the base images declared in the Dockerfile.
REM BuildKit resolves every FROM tag to a digest from the CLI process, which does
REM NOT use the Docker Desktop proxy (that setting only affects dockerd). Where
REM Docker Hub DNS is unreliable this hangs and ends in
REM "failed to fetch anonymous token". `docker pull` goes through dockerd and
REM therefore works; once an image is in the local store BuildKit resolves its
REM metadata locally instead of over the network.
echo.
echo Pre-pulling base images declared in Dockerfile...
REM docker pull takes a single platform, so use the first of a comma separated list.
for /f "tokens=1 delims=," %%p in ("%PLATFORM%") do set PULL_PLATFORM=%%p
for /f "usebackq tokens=1,2" %%a in (`findstr /b /i "FROM" ".\Dockerfile"`) do call :prepull_base "%%b"

REM Preflight the push target so a permission problem does not surface only after
REM a full build. Harbor answers 401 both for a project the account cannot see and
REM for one that does not exist yet, while a repository that simply has not been
REM created reports 404. Only the 401 is treated as fatal here.
REM Credentials are deliberately NOT handled by this script: run
REM "docker login %REGISTRY%" once by hand and Docker Desktop stores the login in
REM the Windows credential manager. Do not put passwords in this file - it is
REM tracked by git.
if /I not "%NO_PUSH%"=="true" (
    echo.
    echo Preflight: checking push access to %FULL_IMAGE_NAME%...
    docker buildx imagetools inspect %FULL_IMAGE_NAME%:latest >"%TEMP%\nakama_preflight.txt" 2>&1
    findstr /c:"401 Unauthorized" "%TEMP%\nakama_preflight.txt" >nul 2>&1
    if not errorlevel 1 (
        echo [ERROR] Registry refused access to %REGISTRY%/%IMAGE_NAME%
        echo         Harbor returns 401 when the project is missing or the account
        echo         cannot see it. Create the project and grant push rights, then
        echo         retry. If this machine never logged in, run:
        echo             docker login %REGISTRY%
        del "%TEMP%\nakama_preflight.txt" >nul 2>&1
        exit /b 1
    )
    echo [OK] Push access looks usable ^(repository may not exist yet, that is fine^)
    del "%TEMP%\nakama_preflight.txt" >nul 2>&1
)

echo.
echo Starting build process...

REM Build main image
echo.
echo Building Nakama HJZX US image...
echo Dockerfile: ./Dockerfile
echo Image tag: %FULL_IMAGE_NAME%

set BUILD_ARGS=buildx build .. --platform %PLATFORM% --file ./Dockerfile --build-arg COMMIT="%COMMIT%" --build-arg VERSION="%VERSION%" --cache-from type=local,src=C:\tmp\.buildx-cache --cache-to type=local,dest=C:\tmp\.buildx-cache-new,mode=max -t %FULL_IMAGE_NAME%:%VERSION% -t %FULL_IMAGE_NAME%:latest

if /I not "%NO_PUSH%"=="true" (
    set BUILD_ARGS=!BUILD_ARGS! --push
)

docker !BUILD_ARGS!
if errorlevel 1 (
    echo [ERROR] Nakama HJZX US image build failed
    exit /b 1
)
echo [OK] Nakama HJZX US image built successfully!

REM Rotate build cache
if exist "C:\tmp\.buildx-cache" rmdir /s /q "C:\tmp\.buildx-cache"
move "C:\tmp\.buildx-cache-new" "C:\tmp\.buildx-cache" >nul 2>&1

REM Display the built image info
echo.
echo Build completed successfully!
echo Image: %FULL_IMAGE_NAME%:%VERSION%
echo Tags: %FULL_IMAGE_NAME%:latest
if /I not "%NO_PUSH%"=="true" (
    echo Status: Pushed to registry
) else (
    echo Status: Built locally ^(not pushed^)
)

:end
endlocal
exit /b 0

REM ---------------------------------------------------------------------------
REM Subroutines
REM ---------------------------------------------------------------------------

REM docker pull one base image. A failed pre-pull is not fatal: BuildKit falls
REM back to resolving the tag itself, which is what happened before this step
REM existed. Skips "scratch" and FROM lines carrying flags, e.g.
REM "FROM --platform=... image".
:prepull_base
set BASE_IMAGE=%~1
if "%BASE_IMAGE%"=="" exit /b 0
if "%BASE_IMAGE:~0,1%"=="-" exit /b 0
if /I "%BASE_IMAGE%"=="scratch" exit /b 0
echo   - %BASE_IMAGE%
docker pull --platform %PULL_PLATFORM% %BASE_IMAGE% >nul 2>&1
if errorlevel 1 (
    echo     [WARNING] pre-pull failed, BuildKit will resolve it over the network
) else (
    echo     [OK] cached locally
)
exit /b 0
