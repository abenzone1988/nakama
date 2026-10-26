#!/bin/sh
# ============================================================================
#  在 EC2 上安装 / 更新 Nginx TLS 层（443 -> Nakama 7350）
#
#  由 deploy-hjzx-nginx.bat 上传到 /tmp 后调用，也可手动跑：
#    ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 \
#      'tr -d "\r" < /tmp/install-hjzx-nginx.sh > /tmp/i.sh && sh /tmp/i.sh'
#
#  前提：以下三个文件已上传到 $STAGE
#    hjzx.conf        Nginx server 配置（仓库 build/nginx/hjzx.conf）
#    fullchain.pem    证书链（leaf + intermediate）
#    privkey.pem      私钥
#
#  幂等：重复执行只覆盖配置与证书并 reload。
#  **不会**修改 /etc/nginx/nginx.conf —— 发行版自带的默认 server 块靠
#  conf.d 里的 `server_name ""` + default_server 压住，见 hjzx.conf 顶部注释。
# ============================================================================

set -e

STAGE=/tmp/hjzx-nginx-stage
CONF_DEST=/etc/nginx/conf.d/hjzx.conf
SSL_DIR=/etc/nginx/ssl/hjzx.sfgames.io
SERVER_NAME=hjzx.sfgames.io
STAMP=$(date +%Y%m%d-%H%M%S)

echo "===== 1/6 检查暂存文件 ====="
for f in hjzx.conf fullchain.pem privkey.pem; do
    if [ ! -f "$STAGE/$f" ]; then
        echo "[错误] 缺少 $STAGE/$f"
        exit 1
    fi
done
echo "[OK] 三个文件都在 $STAGE"

echo
echo "===== 2/6 校验证书与私钥是否配对 ====="
# 用公钥比对，RSA / ECDSA 都适用
cert_pub=$(openssl x509 -in "$STAGE/fullchain.pem" -noout -pubkey | openssl md5)
key_pub=$(openssl pkey -in "$STAGE/privkey.pem" -pubout 2>/dev/null | openssl md5)
if [ -z "$key_pub" ] || [ "$cert_pub" != "$key_pub" ]; then
    echo "[错误] 证书与私钥不匹配，拒绝部署（服务器未被改动）。"
    exit 1
fi
echo "[OK] 证书与私钥匹配"

echo
echo "--- 证书信息 ---"
openssl x509 -in "$STAGE/fullchain.pem" -noout -subject -issuer -dates

echo
echo "===== 3/6 确认 nginx 已安装 ====="
if command -v nginx >/dev/null 2>&1; then
    echo "[OK] $(nginx -v 2>&1)"
else
    echo "未安装，开始安装..."
    sudo dnf install -y nginx
    echo "[OK] $(nginx -v 2>&1)"
fi

echo
echo "===== 4/6 安装配置与证书 ====="
if [ -f "$CONF_DEST" ]; then
    sudo cp "$CONF_DEST" "$CONF_DEST.bak-$STAMP"
    echo "[备份] $CONF_DEST.bak-$STAMP"
fi
sudo mkdir -p "$SSL_DIR" /var/www/html
sudo install -o root -g root -m 644 "$STAGE/hjzx.conf"     "$CONF_DEST"
sudo install -o root -g root -m 644 "$STAGE/fullchain.pem" "$SSL_DIR/fullchain.pem"
sudo install -o root -g root -m 600 "$STAGE/privkey.pem"   "$SSL_DIR/privkey.pem"
sudo chmod 700 "$SSL_DIR"
echo "[OK] 配置 -> $CONF_DEST"
echo "[OK] 证书 -> $SSL_DIR （privkey 600 root，目录 700）"

# SELinux 目前是 Permissive；若哪天改成 Enforcing，没有这个布尔值 nginx 连不上 7350
sudo setsebool -P httpd_can_network_connect on 2>/dev/null || true

echo
echo "===== 5/6 语法检查并生效 ====="
if ! sudo nginx -t; then
    echo "[错误] nginx 配置检查未通过。"
    if [ -f "$CONF_DEST.bak-$STAMP" ]; then
        sudo cp "$CONF_DEST.bak-$STAMP" "$CONF_DEST"
        echo "[回滚] 已恢复上一版 $CONF_DEST，服务器仍按原配置运行。"
    fi
    exit 1
fi
sudo systemctl enable nginx >/dev/null 2>&1
if systemctl is-active --quiet nginx; then
    sudo systemctl reload nginx
    echo "[OK] 已 reload"
else
    sudo systemctl start nginx
    echo "[OK] 已启动"
fi
sleep 1

echo
echo "===== 6/6 验证 ====="
printf '  监听端口: '; sudo ss -lntp | grep -cE ':80 |:443 ' | tr -d '\n'; echo " 个 (80/443 各一，期望 4 含 IPv6)"

echo
echo "--- 本机经 nginx 访问（SNI 指向 127.0.0.1）---"
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 \
    --resolve "$SERVER_NAME:443:127.0.0.1" "https://$SERVER_NAME/" || echo "000")
echo "  https://$SERVER_NAME/  ->  $code"

echo
echo "--- 代理是否真的到 Nakama（期望 Nakama 的 401 JSON）---"
curl -s --max-time 10 --resolve "$SERVER_NAME:443:127.0.0.1" "https://$SERVER_NAME/v2/account" || true
echo

echo
echo "--- 证书有效期 ---"
end=$(openssl x509 -in "$SSL_DIR/fullchain.pem" -noout -enddate | cut -d= -f2)
echo "  到期: $end"
days=$(( ( $(date -d "$end" +%s) - $(date +%s) ) / 86400 ))
echo "  剩余: $days 天"
if [ "$days" -lt 30 ]; then
    echo "  ！！！ 不足 30 天，尽快续期（见 AWS-部署运维说明.md 第 5.7 节）！！！"
elif [ "$days" -lt 60 ]; then
    echo "  ！ 不足 60 天，安排续期。"
fi

# 清理暂存：私钥必须抹掉，其余一并删除
shred -u "$STAGE/privkey.pem" 2>/dev/null || rm -f "$STAGE/privkey.pem"
rm -f "$STAGE/hjzx.conf" "$STAGE/fullchain.pem"
rmdir "$STAGE" 2>/dev/null || true

echo
echo "Nginx TLS 层部署完成。"
