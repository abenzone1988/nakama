#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
if [[ -f .env ]]; then
  echo '.env 已存在，保留现有密码与密钥。'
  exit 0
fi
command -v openssl >/dev/null
umask 077
# Atomic creation prevents a partially written .env being reused after failure.
tmp=$(mktemp .env.XXXXXX.local)
trap 'rm -f "$tmp"' EXIT
while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    POSTGRES_PASSWORD=|CONSOLE_PASSWORD=|SESSION_ENCRYPTION_KEY=|SESSION_REFRESH_ENCRYPTION_KEY=|CONSOLE_SIGNING_KEY=)
      printf '%s%s\n' "$line" "$(openssl rand -hex 24)" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < .env.example > "$tmp"
mv "$tmp" .env
echo '已生成 .env。可设置 LETSENCRYPT_EMAIL 作为证书账户联系邮箱，控制台密码可在此文件查看。'
