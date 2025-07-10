@echo off
echo 运行挑战赛功能测试...
echo.

:: 设置Go环境
set GO111MODULE=on
set GOPROXY=https://goproxy.cn,direct

:: 运行基础测试
echo 运行基础时间逻辑测试...
go test -v ./server -run TestShouldShowChallenge
echo.

echo 运行挑战赛加入逻辑测试...
go test -v ./server -run TestShouldJoinChallenge
echo.

echo 运行时间解析测试...
go test -v ./server -run TestParseDateTime
echo.

echo 运行扩展测试...
go test -v ./server -run TestChallengeTimeScenarios
echo.

echo 运行多用户测试...
go test -v ./server -run TestMultipleUsersChallenge
echo.

echo 运行综合测试...
go test -v ./server -run TestChallengeFullScenario
echo.

:: 运行性能测试
echo 运行性能测试...
go test -v ./server -bench=BenchmarkChallengeTimeLogic -benchtime=3s
echo.

echo 运行数据序列化性能测试...
go test -v ./server -bench=BenchmarkChallengeDataSerialization -benchtime=3s
echo.

echo 运行所有挑战赛测试...
go test -v ./server -run TestChallenge
echo.

echo 测试完成！
pause 