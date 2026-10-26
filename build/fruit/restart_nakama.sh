#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
compose restart nakama
wait_nakama
reload_nginx
compose logs --tail=100 nakama
