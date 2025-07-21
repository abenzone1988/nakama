# 钱包签名配置说明

## 概述

本系统支持两种钱包签名模式，以确保向后兼容性：

1. **兼容模式**（默认）：同时支持包含用户ID和不包含用户ID的签名
2. **严格模式**：只支持包含用户ID的签名

## 配置参数

### DefaultSignatureConfig

```go
type SignatureConfig struct {
    SecretKey     string // 签名密钥
    Algorithm     string // 签名算法
    RequireUserID bool   // 是否要求用户ID作为签名的一部分
}
```

### 默认配置

```go
var DefaultSignatureConfig = SignatureConfig{
    SecretKey:     "sparkinfi-game-secret-key",
    Algorithm:     "HMAC-SHA256",
    RequireUserID: false, // 默认不要求用户ID，保持向后兼容
}
```

## 使用方式

### 1. 兼容模式（当前默认）

```go
// 设置配置
server.DefaultSignatureConfig.RequireUserID = false

// 支持两种签名方式：
// 1. 包含用户ID的签名（新版本客户端）
// 2. 不包含用户ID的签名（旧版本客户端）
```

### 2. 严格模式（未来使用）

```go
// 设置配置
server.DefaultSignatureConfig.RequireUserID = true

// 只支持包含用户ID的签名
// 旧版本客户端将无法通过签名验证
```

## 客户端签名生成

### 新版本客户端（推荐）

```csharp
// 包含用户ID的签名
string signature = SignatureHelper.GenerateWalletSignature(
    "gain",           // 操作类型
    100,              // 金币
    50,               // 钻石
    25,               // 广告券
    "测试奖励",        // 原因
    userId            // 用户ID
);
```

### 旧版本客户端（兼容）

```csharp
// 不包含用户ID的签名
string signature = SignatureHelper.GenerateWalletSignatureLegacy(
    "gain",           // 操作类型
    100,              // 金币
    50,               // 钻石
    25,               // 广告券
    "测试奖励"         // 原因
);
```

## 迁移计划

### 阶段1：兼容模式（当前）
- `RequireUserID = false`
- 支持新旧两种签名方式
- 新客户端使用包含用户ID的签名
- 旧客户端继续使用原有签名方式

### 阶段2：严格模式（未来）
- `RequireUserID = true`
- 只支持包含用户ID的签名
- 所有客户端必须升级到新版本

## 安全考虑

1. **用户ID绑定**：包含用户ID的签名可以防止用户伪造其他用户的签名
2. **向后兼容**：兼容模式确保旧版本客户端不会受到影响
3. **平滑迁移**：可以通过配置开关逐步迁移到严格模式

## 测试

运行测试用例验证兼容性：

```bash
# Go测试
go test ./test -v

# C#测试
dotnet test
```

## 配置示例

### 开发环境
```go
server.DefaultSignatureConfig.RequireUserID = false
```

### 生产环境（当前）
```go
server.DefaultSignatureConfig.RequireUserID = false
```

### 生产环境（未来）
```go
server.DefaultSignatureConfig.RequireUserID = true
``` 