#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
compose config --quiet
mountpoint -q /data || { echo '/data 数据盘未挂载，拒绝更新。' >&2; exit 1; }
# Pull failure leaves the running container untouched. Credentials come from docker login.
compose pull nakama
mkdir -p /data/fruit/backups
chmod 700 /data/fruit/backups
umask 077
backup="/data/fruit/backups/fruit-$(date +%Y%m%d-%H%M%S).sql"
compose exec -T postgres pg_dump -U fruit -d fruit > "$backup.tmp"
mv "$backup.tmp" "$backup"
echo "更新前数据库已备份：$backup"
# Stop the old binary before the new binary migrates the schema.
compose stop nakama
compose up -d --no-deps --force-recreate nakama
wait_nakama
# Nginx must resolve the recreated container's new IP.
reload_nginx
compose logs --tail=100 nakama
