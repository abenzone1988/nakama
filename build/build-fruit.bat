@echo off
setlocal EnableExtensions DisableDelayedExpansion
cd /d "%~dp0.."
set "REGISTRY=docker.sparkinfi.com"
set "IMAGE_NAME=nakama/fruit"
set "PLATFORM=linux/amd64"
set "NO_PUSH=false"
set "COMMIT="
for /f %%i in ('git rev-parse --short HEAD') do set "COMMIT=%%i"
if not defined COMMIT exit /b 1
set "VERSION=%COMMIT%"
:parse
if "%~1"=="" goto build
if /i "%~1"=="--help" goto help
if /i "%~1"=="--no-push" (set "NO_PUSH=true"& shift& goto parse)
if "%~2"=="" goto badarg
if /i "%~1"=="--version" (set "VERSION=%~2"& goto next)
if /i "%~1"=="--registry" (set "REGISTRY=%~2"& goto next)
if /i "%~1"=="--platform" (set "PLATFORM=%~2"& goto next)
:badarg
echo [ERROR] Invalid or missing argument: %~1
exit /b 1
:next
shift
shift
goto parse
:build
docker info >nul 2>&1
if errorlevel 1 (
  echo [ERROR] Start Docker Desktop with Linux containers first.
  exit /b 1
)
docker buildx version >nul 2>&1
if errorlevel 1 exit /b 1
if /i "%NO_PUSH%"=="true" if not "%PLATFORM:,=%"=="%PLATFORM%" (
  echo [ERROR] --no-push supports one platform only.
  exit /b 1
)
set "OUTPUT=--push"
if /i "%NO_PUSH%"=="true" set "OUTPUT=--load"
echo Building current working tree, including uncommitted changes.
git branch --show-current
echo Image: %REGISTRY%/%IMAGE_NAME%:%VERSION% and :latest
REM Credentials stay in Docker's credential store. Run docker login first.
REM Pull through the daemon before BuildKit resolves the base images.
for /f "tokens=1 delims=," %%p in ("%PLATFORM%") do set "PULL_PLATFORM=%%p"
docker pull --platform "%PULL_PLATFORM%" golang:1.23.5-bookworm
if errorlevel 1 exit /b 1
docker pull --platform "%PULL_PLATFORM%" debian:bookworm-slim
if errorlevel 1 exit /b 1
docker buildx build . --file build/Dockerfile.fruit --platform "%PLATFORM%" --build-arg "COMMIT=%COMMIT%" --build-arg "VERSION=%VERSION%" --label "org.opencontainers.image.revision=%COMMIT%" -t "%REGISTRY%/%IMAGE_NAME%:%VERSION%" -t "%REGISTRY%/%IMAGE_NAME%:latest" %OUTPUT%
if errorlevel 1 exit /b 1
echo [OK] Image build completed: %REGISTRY%/%IMAGE_NAME%:%VERSION%
exit /b 0
:help
echo Usage: build-fruit.bat [--version TAG] [--registry HOST] [--platform PLATFORM] [--no-push]
echo Default image: docker.sparkinfi.com/nakama/fruit
echo Default version: current git commit. Default platform: linux/amd64.
echo Quote multi-platform values, for example "linux/amd64,linux/arm64".
exit /b 0
