# 支付通知测试脚本

## 📋 文件说明

- `test_purchase_notify.ps1` - 统一测试脚本（支持所有平台）

## 🔑 平台配置

平台配置已内置在测试脚本中：

| 平台 | Site ID | Key |
|------|---------|-----|
| Android | `xjsmdyapp_android` | `0c373fedcec01cff88aacdb8d2e28dc2` |
| iOS | `xjsmdyapp_ios` | `234c4ec9c31af40a9a0239b868f10dd8` |

## 🚀 使用方法

```powershell
# 测试Android平台
.\test\test_purchase_notify.ps1 -Platform android

# 测试iOS平台
.\test\test_purchase_notify.ps1 -Platform ios

# 测试所有平台（默认）
.\test\test_purchase_notify.ps1 -Platform all
# 或者直接运行
.\test\test_purchase_notify.ps1

# 指定自定义URL（本地测试）
.\test\test_purchase_notify.ps1 -Platform android -Url "http://localhost:7350/v2/tiktok/purchase/notify"

# 指定自定义URL（测试环境）
.\test\test_purchase_notify.ps1 -Platform all -Url "http://test.example.com:6000/v2/tiktok/purchase/notify"
```

## 📊 预期输出

### 测试单个平台

```
========================================
Purchase Notify Test Script
========================================

=== Android Platform Test ===
Site: xjsmdyapp_android
Key: 0c373fedcec01cff88aacdb8d2e28dc2
Sign String: xjsmdyapp_android1762231409...
Sign Result: [MD5签名]

Sending request to: http://39.101.186.196:6000/v2/tiktok/purchase/notify

[SUCCESS] Response Status: 200
[SUCCESS] Response Content: SUCCESS

========================================
[TEST PASSED]
========================================
```

### 测试所有平台

```
========================================
Purchase Notify Test Script
========================================

=== Android Platform Test ===
...
[SUCCESS] Response Status: 200
[SUCCESS] Response Content: SUCCESS

----------------------------------------

=== iOS Platform Test ===
...
[SUCCESS] Response Status: 200
[SUCCESS] Response Content: SUCCESS

========================================
Test Summary
========================================
Android: [PASS]
iOS: [PASS]
========================================
```

### 常见错误

- `site-error` - 应用ID不在允许列表中
- `sign error` - 签名验证失败
- `amount-error` - 金额格式错误
- `delivery-error` - 发货处理失败

## 🔧 配置外网地址

修改测试脚本中的URL地址：
```powershell
$url = "http://39.101.186.196:6000/v2/tiktok/purchase/notify"
```

## 📝 签名算法

签名计算公式：
```
MD5(site + time + key + uid + order_money + cp_order_id)
```

例如：
```
MD5("xjsmdyapp_android" + "1762231409" + "0c373fedcec01cff88aacdb8d2e28dc2" + "test_user_001" + "6.00" + "CP001")
```

## 🌐 支付平台回调配置

将以下地址配置到支付平台的回调URL：

**Android:**
```
http://39.101.186.196:6000/v2/tiktok/purchase/notify
```

**iOS:**
```
http://39.101.186.196:6000/v2/tiktok/purchase/notify
```

注意：同一个接口，系统会根据 `site` 参数自动选择对应的密钥进行验证。

