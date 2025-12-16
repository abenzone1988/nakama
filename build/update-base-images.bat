@echo off
setlocal enabledelayedexpansion

REM Build & push prepped base images (builder/runtime)
REM Run from any path; it will cd to repo root.

set REGISTRY=docker.sparkinfi.com:8443
set BUILDER_IMAGE=%REGISTRY%/base-img/nakama-builder:bookworm-prepped
set RUNTIME_IMAGE=%REGISTRY%/base-img/nakama-runtime:bookworm-prepped

set SCRIPT_DIR=%~dp0
pushd "%SCRIPT_DIR%\.." 1>nul

if not exist "C:\tmp\.buildx-cache" mkdir "C:\tmp\.buildx-cache"
if not exist "C:\tmp\.buildx-cache-new" mkdir "C:\tmp\.buildx-cache-new"

echo === Build & Push builder base ===
docker buildx build --file build/Dockerfile.base-builder --platform linux/amd64 --cache-from type=local,src=C:\tmp\.buildx-cache --cache-to type=local,dest=C:\tmp\.buildx-cache-new,mode=max -t %BUILDER_IMAGE% --push .
if errorlevel 1 exit /b 1

echo === Build & Push runtime base ===
docker buildx build --file build/Dockerfile.base-runtime --platform linux/amd64 --cache-from type=local,src=C:\tmp\.buildx-cache --cache-to type=local,dest=C:\tmp\.buildx-cache-new,mode=max -t %RUNTIME_IMAGE% --push .
if errorlevel 1 exit /b 1

popd 1>nul

echo [OK] Base images updated: %BUILDER_IMAGE% , %RUNTIME_IMAGE%