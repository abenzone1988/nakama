#!/bin/bash
# 测试购买通知接口

URL="http://39.101.186.196:6000/v2/tiktok/purchase/notify"
SITE="cjyx_cn"
KEY="11b18290a34e03da78900824fa59b140"
UID="test_user_001"
ORDER_MONEY="6.00"
CP_ORDER_ID="CP20250104001"
TIME=$(date +%s)

# 计算签名: site + time + key + uid + order_money + cp_order_id
SIGN_STR="${SITE}${TIME}${KEY}${UID}${ORDER_MONEY}${CP_ORDER_ID}"
SIGN=$(echo -n "$SIGN_STR" | md5sum | awk '{print $1}')

echo "签名字符串: $SIGN_STR"
echo "签名结果: $SIGN"
echo ""
echo "发送请求到: $URL"
echo ""

# 发送POST请求
curl -X POST "$URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "site=$SITE" \
  -d "order_id=ORDER20250104001" \
  -d "uid=$UID" \
  -d "sid=server_001" \
  -d "cp_order_id=$CP_ORDER_ID" \
  -d "roleid=role_001" \
  -d "rolename=测试角色" \
  -d "order_money=$ORDER_MONEY" \
  -d "productid=product_001" \
  -d "time=$TIME" \
  -d "sign=$SIGN" \
  -v

echo ""
echo "测试完成"

