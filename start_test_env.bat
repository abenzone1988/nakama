@echo off
echo 启动 Nakama 测试环境 (包含 Redis)...
echo.

echo 停止现有容器...
docker-compose -f docker-compose-tests.yml down

echo.
echo 启动服务...
docker-compose -f docker-compose-tests.yml up -d

echo.
echo 等待服务启动...
timeout /t 10 /nobreak > nul

echo.
echo 检查服务状态...
docker-compose -f docker-compose-tests.yml ps

echo.
echo 测试 Redis 连接...
docker exec redis redis-cli ping

echo.
echo 测试 Nakama 健康检查...
docker exec nakama /nakama/nakama healthcheck

echo.
echo 环境启动完成！
echo.
echo 服务访问地址:
echo - Nakama API: http://localhost:7350
echo - Nakama Console: http://localhost:7351
echo - Redis: localhost:6379
echo.
echo 使用以下命令查看日志:
echo docker-compose -f docker-compose-tests.yml logs -f nakama
echo.
pause
