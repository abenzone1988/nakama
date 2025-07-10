@echo off
echo =====================================
echo 运行核心挑战赛测试 (10000个玩家)
echo =====================================

echo 这是一个大规模测试，会模拟：
echo - 10000个玩家加入挑战赛
echo - 自动创建多个竞标赛
echo - 8000个玩家随机提交成绩
echo - 验证竞标赛分组和成绩提交逻辑
echo.
echo 测试预计需要 15-30 分钟...
echo.

set /p choice=是否开始测试？(Y/N): 
if /i "%choice%"=="Y" (
    echo 开始运行测试...
    echo.
    
    cd /d "%~dp0"
    
    echo [%time%] 开始运行 Test10000PlayersChallenge
    go test -timeout=30m -v -run=Test10000PlayersChallenge ./server/
    
    echo.
    echo [%time%] 测试完成！
    echo.
    echo 测试结果说明：
    echo - 如果看到 "PASS" 表示测试通过
    echo - 如果看到 "FAIL" 表示测试失败
    echo - 详细的统计信息会在测试日志中显示
    echo.
) else (
    echo 取消测试。
)

pause 