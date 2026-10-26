#!/usr/bin/env bash
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo '请以 root 配置数据盘目录。' >&2; exit 1; }
mountpoint -q /data || { echo '/data 数据盘未挂载，拒绝创建数据库目录。' >&2; exit 1; }
mkdir -p /data/fruit/postgres /data/fruit/backups
chmod 700 /data/fruit/postgres /data/fruit/backups
# Preserve existing PostgreSQL ownership; its image initializes ownership on first use.
mkdir -p /etc/systemd/system/docker.service.d
cat > /etc/systemd/system/docker.service.d/fruit-data.conf <<'EOF'
[Unit]
RequiresMountsFor=/data
EOF
systemctl daemon-reload
echo '数据目录已就绪，Docker 开机启动将等待 /data 数据盘挂载。'
