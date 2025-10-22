#!/usr/bin/env bash
# 更新并重启 Nakama（latest），检查健康并输出日志

set -euo pipefail

# 可选：开启调试
# set -x

# 变量（如脚本放在 deploy/game/ 下，可保持默认）
COMPOSE_DIR="${COMPOSE_DIR:-$(cd "$(dirname "$0")" && pwd)}"
SERVICE_NAME="${SERVICE_NAME:-nakama}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:7350/healthcheck}"
HEALTH_TIMEOUT_SEC="${HEALTH_TIMEOUT_SEC:-180}"   # 健康检查超时
SLEEP_BETWEEN_CHECKS="${SLEEP_BETWEEN_CHECKS:-3}" # 健康检查间隔秒

# 如果需要 Harbor 登录，默认账号密码为 admin/Aben6290209，也可通过环境变量覆盖
HARBOR_USER="${HARBOR_USER:-admin}"
HARBOR_PASS="${HARBOR_PASS:-Aben6290209}"

# 选择 docker compose 命令
if docker compose version >/dev/null 2>&1; then
  COMPOSE_BIN="docker compose"
elif docker-compose version >/dev/null 2>&1; then
  COMPOSE_BIN="docker-compose"
else
  echo "[ERROR] docker compose/docker-compose 未安装" >&2
  exit 1
fi

cd "$COMPOSE_DIR"

# 可选：Harbor 登录
if [[ -n "$HARBOR_USER" && -n "$HARBOR_PASS" ]]; then
  echo "$HARBOR_PASS" | docker login "${REGISTRY:-docker.sparkinfi.com:8443}" --username "$HARBOR_USER" --password-stdin
fi

echo "[INFO] 拉取最新镜像..."
$COMPOSE_BIN pull "$SERVICE_NAME"

echo "[INFO] 以 latest 更新并重启服务..."
$COMPOSE_BIN up -d --pull=always --no-deps --force-recreate "$SERVICE_NAME"

# 健康检查
echo "[INFO] 等待服务通过健康检查: $HEALTH_URL"
deadline=$(( $(date +%s) + HEALTH_TIMEOUT_SEC ))
for i in {1..3}; do
  code=$(curl -sS -o /dev/null -w "%{http_code}" --connect-timeout 2 --max-time 5 "$HEALTH_URL" || true)
  if [[ "$code" == "200" ]]; then
    echo "[INFO] 健康检查通过 (HTTP 200)"
    break
  fi
  if [[ $i -eq 3 ]]; then
    echo "[ERROR] 健康检查共尝试3次均失败。输出最近日志以便排查..." >&2
    $COMPOSE_BIN logs --tail=200 "$SERVICE_NAME" || true
    exit 1
  fi
  sleep "$SLEEP_BETWEEN_CHECKS"
done

# 输出并跟随日志
echo "[INFO] 输出最近 200 行日志并持续跟随（Ctrl+C 退出跟随）"
$COMPOSE_BIN logs --tail=200 -f "$SERVICE_NAME"
