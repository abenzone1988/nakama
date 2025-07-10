@echo off
echo ==========================================
echo 挑战赛系统测试批处理脚本
echo ==========================================

echo.
echo 1. 运行基础挑战赛功能测试
echo 2. 运行综合挑战赛系统测试
echo 3. 运行1000玩家大规模测试
echo 4. 运行所有挑战赛相关测试
echo 5. 运行测试并生成详细报告
echo.

set /p choice=请选择要运行的测试 (1-5): 

if "%choice%"=="1" (
    echo.
    echo 运行基础挑战赛功能测试...
    go test -v -run "TestParse|TestShould|TestUserMatch|TestChallenge" ./server/
    goto :end
)

if "%choice%"=="2" (
    echo.
    echo 运行综合挑战赛系统测试...
    go test -v -run "TestChallengeSystemIntegration" ./server/
    goto :end
)

if "%choice%"=="3" (
    echo.
    echo 运行1000玩家大规模测试...
    go test -v -run "TestChallengeSystemIntegration" -timeout 30m ./server/
    goto :end
)

if "%choice%"=="4" (
    echo.
    echo 运行所有挑战赛相关测试...
    go test -v -run "TestChallenge|TestParse|TestShould|TestUser|TestTournament|TestScore|TestConcurrent|Test10000" ./server/
    goto :end
)

if "%choice%"=="5" (
    echo.
    echo 运行测试并生成详细报告...
    echo 创建测试报告目录...
    if not exist "test_reports" mkdir test_reports
    
    echo 运行测试并生成JSON报告...
    go test -v -json -run "TestChallenge|TestParse|TestShould|TestUser|TestTournament|TestScore|TestConcurrent|Test10000" ./server/ > test_reports/challenge_test_report.json
    
    echo 运行测试并生成文本报告...
    go test -v -run "TestChallenge|TestParse|TestShould|TestUser|TestTournament|TestScore|TestConcurrent|Test10000" ./server/ > test_reports/challenge_test_report.txt 2>&1
    
    echo 测试报告已生成在 test_reports 目录中
    goto :end
)

echo 无效的选择，请重新运行脚本
goto :end

:end
echo.
echo ==========================================
echo 测试完成！
echo ==========================================
pause 