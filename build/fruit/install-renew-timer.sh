#!/usr/bin/env bash
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo '请以 root 运行以安装续期定时器。' >&2; exit 1; }
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ "$dir" != *[[:space:]]* ]] || { echo '部署路径不能包含空格。' >&2; exit 1; }
cat > /etc/systemd/system/fruit-cert-renew.service <<EOF
[Unit]
Description=Renew Fruit HTTPS certificates and reload Nginx
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=oneshot
WorkingDirectory=$dir
ExecStart=/bin/bash $dir/renew-cert.sh
EOF
cat > /etc/systemd/system/fruit-cert-renew.timer <<'EOF'
[Unit]
Description=Check Fruit certificates twice daily

[Timer]
OnCalendar=*-*-* 03,15:17:00
RandomizedDelaySec=1800
Persistent=true

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now fruit-cert-renew.timer
systemctl list-timers fruit-cert-renew.timer --no-pager
