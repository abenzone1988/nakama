@echo off
echo 开始Tournament创建和分配测试...

echo.
echo ===========================================
echo 测试Tournament创建和分配功能
echo ===========================================
echo.

REM 运行并发Tournament创建测试
echo 运行并发Tournament创建测试...
go test -v -timeout=30m -run=TestConcurrentTournamentCreation ./server

echo.
echo ===========================================
echo 测试完成
echo ===========================================
echo.

pause 