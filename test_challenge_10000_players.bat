@echo off
echo =====================================
echo 运行10000个玩家挑战赛大规模测试
echo =====================================

echo 开始运行测试...
echo 这可能需要几分钟时间，请耐心等待...

cd /d "%~dp0"

echo 运行主要测试: Test10000PlayersChallenge
go test -timeout=30m -v -run=Test10000PlayersChallenge ./server/

echo.
echo 运行并发测试: TestConcurrentChallengeJoin
go test -timeout=10m -v -run=TestConcurrentChallengeJoin ./server/

echo.
echo 运行成绩提交测试: TestTournamentScoreSubmission
go test -timeout=5m -v -run=TestTournamentScoreSubmission ./server/

echo.
echo 运行真实锦标赛测试: TestRealTournamentScoreSubmission
go test -timeout=10m -v -run=TestRealTournamentScoreSubmission ./server/

echo.
echo 运行成绩验证测试: TestScoreSubmissionValidation
go test -timeout=2m -v -run=TestScoreSubmissionValidation ./server/

echo.
echo =====================================
echo 测试完成！
echo =====================================
pause 