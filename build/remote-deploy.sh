#!/bin/sh
# 在 EC2 上执行的部署步骤。由 deploy-hjzx-us.bat 上传后调用，也可以手动跑：
#   ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 'sh /tmp/remote-deploy.sh'
#
# 前提：/opt/nakama/docker-compose.yml 与 /opt/nakama/local.yml 已就位。

set -e

DIR=/opt/nakama
cd "$DIR"

echo "===== 1/5 检查配置文件 ====="
if [ ! -f "$DIR/local.yml" ]; then
    echo "[错误] $DIR/local.yml 不存在，或者它是个目录。"
    echo "       Docker 在挂载点源文件缺失时会把目标创建成目录，必须先修好再启动。"
    exit 1
fi
chmod 600 "$DIR/local.yml"
echo "[OK] local.yml 是文件，权限已设为 600"

echo
echo "===== 2/5 拉取最新镜像 ====="
docker compose pull

echo
echo "===== 3/5 重建容器 ====="
docker compose up -d

echo
echo "===== 4/5 等待健康检查（最多 150 秒）====="
i=0
while [ $i -lt 30 ]; do
    status=$(docker inspect -f '{{.State.Health.Status}}' nakama 2>/dev/null || echo none)
    case "$status" in
        healthy)
            echo "[OK] 容器健康"
            break
            ;;
        unhealthy)
            echo "[错误] 容器进入 unhealthy 状态"
            docker compose logs --tail 60 nakama
            exit 1
            ;;
        *)
            i=$((i + 1))
            printf '.'
            sleep 5
            ;;
    esac
done
if [ $i -ge 30 ]; then
    echo
    echo "[错误] 等待健康检查超时，最近日志："
    docker compose logs --tail 60 nakama
    exit 1
fi

echo
echo "===== 5/5 验证 ====="
docker compose ps

echo
echo "--- 运行的镜像 ---"
docker inspect nakama --format '{{.Config.Image}}  ID={{.Image}}'

echo
echo "--- 服务版本（应等于刚推送的版本号）---"
docker logs nakama 2>&1 | grep -o '"version":"[^"]*"' | grep -v PostgreSQL | tail -1

echo
echo "--- 容器内数据（来自镜像）---"
printf '  Lua 模块: '; docker exec nakama sh -c 'ls /nakama/data/modules/*.lua 2>/dev/null | wc -l'
printf '  模板表  : '; docker exec nakama sh -c 'ls /nakama/data/Tpl*.json 2>/dev/null | wc -l'

echo
echo "部署完成。"
