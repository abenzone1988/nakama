#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
compose run --rm --no-deps certbot renew --webroot -w /var/www/certbot "$@"
reload_nginx
