#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
compose config --quiet
mkdir -p nginx/conf.d certbot/www certbot/conf
target=nginx/conf.d/afagame223.conf
backup=$(mktemp)
had_config=false
if [[ -f "$target" ]]; then
  cp "$target" "$backup"
  had_config=true
fi
rollback() {
  if [[ "$had_config" == true ]]; then
    cp "$backup" "$target"
  else
    rm -f "$target"
  fi
  reload_nginx || true
  rm -f "$backup"
}
trap rollback ERR
if [[ -f certbot/conf/live/fruit.afagame223.com/fullchain.pem ]]; then
  cp nginx/afagame223.https.conf.template "$target"
else
  cp nginx/afagame223.http.conf.template "$target"
fi
reload_nginx
email=$(sed -n 's/^LETSENCRYPT_EMAIL=//p' .env)
account_args=(--register-unsafely-without-email)
if [[ -n "$email" ]]; then account_args=(--email "$email"); fi
compose run --rm --no-deps certbot certonly --webroot -w /var/www/certbot \
  --non-interactive --agree-tos "${account_args[@]}" --keep-until-expiring \
  --cert-name fruit.afagame223.com -d fruit.afagame223.com -d fruit-admin.afagame223.com
cp nginx/afagame223.https.conf.template "$target"
reload_nginx
verify_https() {
  local attempt
  for attempt in 1 2 3 4 5; do
    if curl --max-time 15 --fail --show-error --silent --output /dev/null "$@"; then return 0; fi
    sleep 1
  done
  return 1
}
# Reload is asynchronous; wait for workers to load the new SNI certificate.
verify_https --resolve fruit.afagame223.com:443:127.0.0.1 https://fruit.afagame223.com/healthcheck
verify_https --resolve fruit-admin.afagame223.com:443:127.0.0.1 https://fruit-admin.afagame223.com/
trap - ERR
rm -f "$backup"
bash "$SCRIPT_DIR/install-renew-timer.sh"
echo 'afagame223.com 的 API、管理后台 HTTPS 和自动续期已配置。'
