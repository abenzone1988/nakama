@echo off
chcp 65001
echo ===============================================
echo 挑战赛测试用例执行脚本
echo ===============================================
echo.

echo 设置测试环境...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

echo.
echo 开始执行挑战赛测试...
echo.

echo [1/5] 执行综合时间段测试...
go test -v -run TestChallengeComprehensive server/game_challenge_test.go server/game_challenge_test_impl.go -timeout=30m
if errorlevel 1 (
    echo 时间段测试失败！
    pause
    exit /b 1
)

echo.
echo [2/5] 执行奖励场景测试...
go test -v -run TestChallengeRewardScenarios server/game_challenge_test.go server/game_challenge_test_impl.go -timeout=30m
if errorlevel 1 (
    echo 奖励场景测试失败！
    pause
    exit /b 1
)

echo.
echo [3/5] 执行边界条件测试...
go test -v -run TestChallengeBoundaryConditions server/game_challenge_test.go server/game_challenge_test_impl.go -timeout=30m
if errorlevel 1 (
    echo 边界条件测试失败！
    pause
    exit /b 1
)

echo.
echo [4/5] 执行排名场景测试...
go test -v -run TestChallengeRankingScenarios server/game_challenge_test.go server/game_challenge_test_impl.go -timeout=30m
if errorlevel 1 (
    echo 排名场景测试失败！
    pause
    exit /b 1
)

echo.
echo [5/5] 执行完整工作流程测试...
go test -v -run TestChallengeCompleteWorkflow server/game_challenge_test.go server/game_challenge_test_impl.go -timeout=30m
if errorlevel 1 (
    echo 完整工作流程测试失败！
    pause
    exit /b 1
)

echo.
echo ===============================================
echo 所有测试执行完成！
echo ===============================================
echo.

echo 执行具体实现测试...
echo.
echo [具体实现 1/4] 时间场景测试...
go test -v -run TestChallengeTimeScenarioImpl server/game_challenge_test_impl.go -timeout=30m

echo.
echo [具体实现 2/4] 奖励场景测试...
go test -v -run TestChallengeRewardScenarioImpl server/game_challenge_test_impl.go -timeout=30m

echo.
echo [具体实现 3/4] 边界条件测试...
go test -v -run TestChallengeBoundaryConditionsImpl server/game_challenge_test_impl.go -timeout=30m

echo.
echo [具体实现 4/4] 时间边界测试...
go test -v -run TestChallengeTimeBoundaryImpl server/game_challenge_test_impl.go -timeout=30m

echo.
echo ===============================================
echo 完整测试套件执行完成！
echo ===============================================
echo.

echo 生成测试报告...
go test -v -run TestChallengeExecutionGuide server/game_challenge_test.go -timeout=5m

echo.
echo 测试执行完成！请查看上面的输出结果。
echo 如果有测试失败，请检查相应的错误信息。
echo.
pause 