#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
bash "$SCRIPT_DIR/prepare-storage.sh"
email=$(sed -n 's/^LETSENCRYPT_EMAIL=//p' .env)
if [[ -n "$email" ]]; then
  account_args=(--email "$email")
else
  account_args=(--register-unsafely-without-email)
  echo '未配置运维邮箱，将注册无邮箱账户；自动续期仍然启用。'
fi
compose config --quiet
mkdir -p nginx/conf.d certbot/www certbot/conf
compose pull postgres nakama nginx certbot
compose up -d postgres nakama
wait_nakama
# Bootstrap HTTP without referencing certificate files that do not yet exist.
if [[ -f certbot/conf/live/fruit.sparkgalaxy.cn/fullchain.pem ]]; then
  cp nginx/https.conf.template nginx/conf.d/fruit.conf
else
  cp nginx/http.conf.template nginx/conf.d/fruit.conf
fi
compose up -d nginx
reload_nginx
compose run --rm --no-deps certbot certonly --webroot -w /var/www/certbot \
  --non-interactive --agree-tos "${account_args[@]}" --keep-until-expiring \
  --cert-name fruit.sparkgalaxy.cn -d fruit.sparkgalaxy.cn -d fruit-admin.sparkgalaxy.cn
cp nginx/https.conf.template nginx/conf.d/fruit.conf
reload_nginx
bash "$SCRIPT_DIR/install-renew-timer.sh"
curl --fail --show-error --silent --resolve fruit.sparkgalaxy.cn:443:127.0.0.1 https://fruit.sparkgalaxy.cn/healthcheck
curl --fail --show-error --silent --output /dev/null --resolve fruit-admin.sparkgalaxy.cn:443:127.0.0.1 https://fruit-admin.sparkgalaxy.cn/
echo '部署完成，两个 HTTPS 入口验证通过。'
