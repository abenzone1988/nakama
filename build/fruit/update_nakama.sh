#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
compose config --quiet
mountpoint -q /data || { echo '/data 数据盘未挂载，拒绝更新。' >&2; exit 1; }

# 备份开关：默认不备份，显式传 --backup 才在更新前导出数据库。
backup=0
case "${1:-}" in
  --backup) backup=1 ;;
  "") ;;
  *) echo "用法: bash $(basename "$0") [--backup]" >&2; exit 2 ;;
esac

# Pull failure leaves the running container untouched. Credentials come from docker login.
compose pull nakama

if (( backup )); then
  mkdir -p /data/fruit/backups
  chmod 700 /data/fruit/backups
  umask 077
  backup_file="/data/fruit/backups/fruit-$(date +%Y%m%d-%H%M%S).sql"
  # wallet_ledger 只保留表结构，不导数据（流水表体积增长最快）。
  compose exec -T postgres pg_dump -U fruit -d fruit \
    --exclude-table-data=wallet_ledger > "$backup_file.tmp"
  mv "$backup_file.tmp" "$backup_file"
  echo "更新前数据库已备份（wallet_ledger 仅保留结构）：$backup_file"
fi

# Stop the old binary before the new binary migrates the schema.
compose stop nakama
compose up -d --no-deps --force-recreate nakama
wait_nakama
# Nginx must resolve the recreated container's new IP.
reload_nginx
compose logs --tail=100 nakama
