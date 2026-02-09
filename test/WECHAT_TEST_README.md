# 微信消息推送接口测试文档

## 概述

本测试套件用于测试微信小游戏消息推送接口的功能，包括：
- 签名验证
- 发货功能
- 查询奖励功能
- 数据累加

## 文件说明

- `test_wechat_push.ps1` - Windows PowerShell脚本
- `wechat_test_config.json` - 测试配置文件

## 前置条件

1. **服务器运行中**
   - 线上测试请求地址
   - 默认地址：`http://118.145.182.217:7350`

2. **用户已创建**
   - 测试用户的OpenID必须存在于数据库中

3. **Token配置正确**
   - 服务器端Token：`sparkinfi

## 使用方法


### Windows (PowerShell)

```powershell
# 直接运行PowerShell脚本
.\test\test_wechat_push.ps1

```

## 测试项目

### 测试1：GET请求签名验证
测试服务器验证签名的基本功能（用于微信后台配置验证）。

**预期结果**：返回echostr参数的值

### 测试2：发货请求测试
模拟微信发送发货请求，包含：
- 擂台赛奖励凭证（ID: 60000）
- 普通道具（ID: 10001）

**预期结果**：
- ErrCode = 0
- 擂台赛奖励保存到storage
- 普通道具发送到邮件

### 测试3：查询奖励请求测试
查询用户获得的擂台赛奖励凭证数量。

**预期结果**：
- ErrCode = 0
- 返回TodayCount（今日数量）
- 返回TotalCount（历史总数）

### 测试4：错误签名验证测试
测试签名验证的安全性。

**预期结果**：
- ErrCode != 0
- 拒绝请求

### 测试5：多次发货累加测试
测试多次发货是否正确累加。

**预期结果**：
- 两次发货都成功
- 查询结果正确累加

## 配置文件说明

修改 `wechat_test_config.json` 可以自定义测试参数：

```json
{
  "base_url": "http://localhost:7350",    // 服务器地址
  "api_path": "/v2/wechat/message/verify", // API路径
  "token": "sparkinfi",                    // 签名Token
  "test_users": [
    {
      "openid": "e730dd1d-6f31-416b-ab9a-e7db5b069116",
      "description": "测试用户1"
    }
  ]
}
```

## 签名算法说明

微信消息推送使用SHA1签名算法：

```
1. 将 token、timestamp、nonce 三个参数按字典序排序
2. 拼接成一个字符串
3. 对拼接后的字符串进行SHA1哈希
4. 将哈希结果与微信传来的signature比较
```

**示例：**
```
token = "sparkinfi"
timestamp = "1234567890"
nonce = "abc123"

排序后：["1234567890", "abc123", "sparkinfi"]
拼接：  "1234567890abc123sparkinfi"
SHA1：  计算结果即为signature
```

## 故障排查

### 错误：连接被拒绝
- 检查服务器是否运行
- 检查端口是否正确（默认7350）

### 错误：User not found
- 确认测试用户已在数据库中创建
- 检查OpenID是否正确

### 错误：Signature verification failed
- 检查服务器端Token配置
- 检查时间戳是否正确

### 错误：Failed to parse message
- 检查JSON格式是否正确
- 检查字段名是否匹配

#
## API接口说明

### 路由地址
```
GET/POST /v2/wechat/message/verify
```

### 查询参数
- `signature` - 签名
- `timestamp` - 时间戳
- `nonce` - 随机数
- `echostr` - 验证字符串（仅GET请求）

### 请求体格式（POST）

**发货请求：**
```json
{
  "Event": "minigame_deliver_goods",
  "MiniGame": {
    "OrderId": "订单ID",
    "ToUserOpenid": "用户OpenID",
    "GoodsList": [
      {"Id": "商品ID", "Num": 数量}
    ],
    "GiftId": "礼包ID",
    "SendTime": 时间戳
  }
}
```

**查询请求：**
```json
{
  "Event": "query_challenge_reward",
  "QueryReward": {
    "ToUserOpenid": "用户OpenID",
    "ItemID": "商品ID"
  }
}
```

### 响应格式

**通用响应：**
```json
{
  "ErrCode": 0,      // 0表示成功，-1表示失败
  "ErrMsg": "Success"
}
```

**查询响应：**
```json
{
  "ErrCode": 0,
  "ErrMsg": "Success",
  "TodayCount": 10,   // 今日获得数量
  "TotalCount": 100   // 历史总数量
}
```

## 注意事项

1. 测试前确保服务器正常运行
2. 测试用户必须提前创建
3. 签名算法必须与服务器端一致
4. 建议在开发环境测试，避免影响生产数据
5. 测试会实际操作数据库，请注意数据清理


