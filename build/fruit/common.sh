#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
compose() { docker compose --project-directory "$SCRIPT_DIR" --env-file "$SCRIPT_DIR/.env" -f "$SCRIPT_DIR/docker-compose.yml" "$@"; }
[[ -f .env ]] || { echo '请先运行 bash prepare.sh 并填写 .env。' >&2; exit 1; }
docker compose version >/dev/null
# Prevent concurrent init, restart, update and certificate renewal operations.
exec 9>"$SCRIPT_DIR/.deploy.lock"
flock -n 9 || { echo '另一个部署或续期操作正在进行。' >&2; exit 1; }
wait_nakama() {
  local deadline=$((SECONDS + ${HEALTH_TIMEOUT_SEC:-240})) id status
  while (( SECONDS < deadline )); do
    id=$(compose ps -q nakama)
    status=unknown
    if [[ -n "$id" ]]; then
      status=$(docker inspect --format '{{.State.Health.Status}}' "$id" 2>/dev/null || true)
    fi
    if [[ "$status" == healthy ]]; then
      echo 'Nakama 健康检查通过。'
      return 0
    fi
    sleep 3
  done
  compose logs --tail=100 nakama >&2
  echo '等待 Nakama 健康检查超时。' >&2
  return 1
}
reload_nginx() {
  compose exec -T nginx nginx -t
  compose exec -T nginx nginx -s reload
}
