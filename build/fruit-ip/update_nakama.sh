#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
compose config --quiet
# Pull failure leaves the running container untouched. Credentials come from docker login.
compose pull nakama
mkdir -p /data/fruit-ip/backups
chmod 700 /data/fruit-ip/backups
umask 077
backup="/data/fruit-ip/backups/fruit-$(date +%Y%m%d-%H%M%S).sql"
compose exec -T postgres pg_dump -U fruit -d fruit > "$backup.tmp"
mv "$backup.tmp" "$backup"
echo "更新前数据库已备份：$backup"
# Stop the old binary before the new binary migrates the schema.
compose stop nakama
compose up -d --no-deps --force-recreate nakama
wait_nakama
compose logs --tail=100 nakama
