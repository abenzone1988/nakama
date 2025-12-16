@echo off
REM Nakama Docker Build Script (Windows Batch Version)
REM Based on original build.sh script, supports multi-platform build and push
REM Author: Jiangp
REM Version: v1.0.0

setlocal enabledelayedexpansion

echo === Nakama Docker Build Script (Windows) ===
echo Based on original build.sh script
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
set REGISTRY=docker.sparkinfi.com:8443
set IMAGE_NAME=xjsm/nakama
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
    set PLATFORM=%2
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
echo Usage: build.bat [options]
echo.
echo Options:
echo   --version VERSION     Set version number (default: from git tag)
echo   --registry REGISTRY   Set registry address (default: docker.sparkinfi.com:8443)
echo   --image-name NAME     Set image name (default: nakama)
echo   --platform PLATFORM   Set target platform (default: linux/amd64)
echo   --no-push             Build only, do not push
echo   --help                Show this help message
echo.
echo Examples:
echo   build.bat
echo   build.bat --version 2.1.0 --registry myregistry.com
echo   build.bat --no-push
goto :end

:start_build
echo Version: %VERSION%
echo Commit: %COMMIT%
echo Registry: %REGISTRY%
echo Platform: %PLATFORM%
echo Push: %NO_PUSH%
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

echo.
echo Starting build process...

REM Build main image
echo.
echo Building Nakama main image...
echo Dockerfile: ./Dockerfile
echo Image tag: %FULL_IMAGE_NAME%

set BUILD_ARGS=buildx build .. --platform %PLATFORM% --file ./Dockerfile --build-arg COMMIT="%COMMIT%" --build-arg VERSION="%VERSION%" --cache-from type=local,src=C:\tmp\.buildx-cache --cache-to type=local,dest=C:\tmp\.buildx-cache-new,mode=max -t %FULL_IMAGE_NAME%:%VERSION% -t %FULL_IMAGE_NAME%:latest

if "%NO_PUSH%"=="false" (
    set BUILD_ARGS=%BUILD_ARGS% --push
)

docker %BUILD_ARGS%
if errorlevel 1 (
    echo [ERROR] Nakama main image build failed
    exit /b 1
)
echo [OK] Nakama main image built successfully!

