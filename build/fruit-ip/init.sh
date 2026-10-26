#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
[[ $EUID -eq 0 ]] || { echo '请使用 root 初始化数据目录。' >&2; exit 1; }
compose config --quiet
mkdir -p /data/fruit-ip/postgres /data/fruit-ip/backups
chmod 700 /data/fruit-ip/postgres /data/fruit-ip/backups
compose pull postgres nakama
compose up -d
wait_nakama
curl --fail --show-error --silent http://127.0.0.1:7550/healthcheck
curl --fail --show-error --silent --output /dev/null http://127.0.0.1:7551/
echo 'fruit IP 直连服务已启动。'
