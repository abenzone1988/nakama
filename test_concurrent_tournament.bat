@echo off
echo 开始并发Tournament创建测试...

echo.
echo ===========================================
echo 测试并发Tournament创建和分配
echo ===========================================
echo.

REM 设置测试环境
set GO_ENV=test
set GOCACHE=%TEMP%\go-cache

REM 运行并发测试
echo 运行并发Tournament创建测试...
go test -v -timeout=30m -run=TestConcurrentTournamentCreation ./server

echo.
echo ===========================================
echo 测试完成
echo ===========================================
echo.

pause 