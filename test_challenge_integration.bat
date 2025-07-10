@echo off
echo =====================================
echo 运行挑战赛集成测试
echo =====================================

echo 这个测试将验证：
echo - 用户认证流程
echo - 挑战赛获取和加入
echo - 成绩提交工作流程
echo - 大规模并发测试
echo.

set /p choice=是否开始集成测试？(Y/N): 
if /i "%choice%"=="Y" (
    echo 开始运行集成测试...
    echo.
    
    cd /d "%~dp0"
    
    echo [%time%] 运行真实挑战赛工作流程测试
    go test -timeout=10m -v -run=TestRealChallengeWorkflow ./server/
    
    echo.
    echo [%time%] 运行竞标赛成绩工作流程测试
    go test -timeout=5m -v -run=TestRealTournamentScoreWorkflow ./server/
    
    echo.
    echo [%time%] 运行挑战赛认证流程测试
    go test -timeout=3m -v -run=TestChallengeAuthentication ./server/
    
    echo.
    echo [%time%] 运行完整集成测试（可选）
    set /p advanced=是否运行完整集成测试？(Y/N): 
    if /i "%advanced%"=="Y" (
        go test -timeout=15m -v -run=TestFullIntegrationWithRealAPI ./server/
    )
    
    echo.
    echo [%time%] 所有集成测试完成！
    echo.
    echo 测试结果说明：
    echo - PASS: 测试通过
    echo - FAIL: 测试失败，请检查日志
    echo - 详细的统计信息在测试输出中
    echo.
) else (
    echo 取消测试。
)

pause 