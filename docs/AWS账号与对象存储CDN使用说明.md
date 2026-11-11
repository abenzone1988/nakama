# AWS 账号与对象存储 / CDN 使用说明

> 用途：把当前项目在用的 AWS 账号 `hhxj` 复用到**其他项目**，用于「上传静态资源到对象存储（S3）+ 通过 CDN 分发」。
> 账号与权限信息为 2026-11-05 实测，命令均可直接执行。
>
> **本文不写入任何 Access Key 明文**。密钥只存在于本机 `~/.aws/credentials`，复制/迁移方式见第 2 节。

---

## 0. 一句话结论

- 账号：`581237505531`，IAM 用户 `hhxj`，Profile 名 **`hhxj`**，权限 `AdministratorAccess`（S3 / CloudFront / ACM 都能用）。
- 区域：**`us-east-1`**。`[profile hhxj]` 里**没有配置 region**，除 S3/STS/IAM 外的多数命令必须显式带 `--region us-east-1`。
- ⚠️ 本机 `default` profile 是**另一个生产账号**（starhold，`746669197317`）。所有命令必须显式 `--profile hhxj`，否则会静默打到别人线上。
- 对象存储用 **S3**，CDN 用 **CloudFront**（推荐）或 Cloudflare 回源（备选）。上传是往 S3 传，CDN 只会缓存回源内容，不接收"上传"。

---

## 1. 账号信息与现有资源（2026-11-05 实测）

| 项 | 值 |
|---|---|
| AWS 账号 ID | `581237505531` |
| IAM 用户 | `hhxj` |
| 用户 ARN | `arn:aws:iam::581237505531:user/hhxj` |
| CLI Profile | `hhxj` |
| 权限 | `AdministratorAccess`（全量），另有 RDS/VPC/EKS 等策略，均冗余 |
| 默认区域 | **未配置**，命令里必须写 `--region us-east-1` |
| AWS CLI | 2.34.44（Windows） |

账号内与 CDN/对象存储相关的现状：

| 资源 | 现状 |
|---|---|
| S3 桶 | 只有 1 个 CloudTrail 日志桶 `aws-cloudtrail-logs-581237505531-9c02c29a`（us-east-1），**没有业务桶** |
| CloudFront 分发 | **0 个**（服务可用，直接新建即可） |
| ACM 证书 | **0 张**（自定义域名需新申请，且必须在 us-east-1） |
| Route53 托管区 | **无**（域名在 GoDaddy，解析要在域名商那边加 CNAME） |
| 账号级 S3 公共访问块 | **未设置**（新桶沿用桶级默认：阻止公共访问） |

> 域名坑：真实域名是 **`sfgames.io`**（GoDaddy 托管）。`sfgame.io` 是 NXDOMAIN，不要拼错。

---

## 2. 在新机器 / 新项目里配置凭证

### 2.1 复制已有凭证（最省事）

本机凭证文件：

- Windows：`C:\Users\Administrator\.aws\credentials`
- 配置：`C:\Users\Administrator\.aws\config`

只需要复制 `[hhxj]` 这一段（**不要**复制 `[default]`，那是另一个生产账号）：

```ini
[hhxj]
aws_access_key_id = AKIA...
aws_secret_access_key = ...
```

同时建议把 region 补进 profile（新机器上执行一次）：

```bash
aws configure set region us-east-1 --profile hhxj
aws configure set output json   --profile hhxj
```

### 2.2 或者重新签一对密钥（更安全，推荐）

```bash
# 用现有 hhxj 凭证签一对新的
aws iam create-access-key --user-name hhxj --profile hhxj --region us-east-1
```

拿到后在新机器上：

```bash
aws configure --profile hhxj
# AWS Access Key ID     : 粘贴新 AK
# AWS Secret Access Key : 粘贴新 SK
# Default region name   : us-east-1
# Default output format : json
```

迁移完成后评估停用旧密钥：

```bash
aws iam update-access-key --user-name hhxj --access-key-id <旧AK> --status Inactive --profile hhxj --region us-east-1
```

### 2.3 安全红线

- 密钥**只放** `~/.aws/credentials`，不进任何 git 仓库、不进文档、不进聊天记录。
- 别人的项目 / CI 不要直接共用这把管理员密钥——见第 8 节的最小权限子账号方案。
- 新项目的 `.gitignore` 里确认有 `.aws/`、`*.pem`、`.env`。

---

## 3. 账号串台防护（最高优先级）

本机两个 profile 并存：

| Profile | 账号 ID | IAM 用户 | 性质 |
|---|---|---|---|
| `default` | `746669197317` | `starhold-cdn-user` | **starhold 线上生产环境**（EKS + RDS + ECR + Cloudflare CDN） |
| `hhxj` | `581237505531` | `hhxj` | 本项目要用的账号 |

**漏写 `--profile hhxj` 不会报错**，会静默落到 `default`，写操作直接改到生产账号。所以：

**① 每条命令都带 `--profile hhxj --region us-east-1`。**

**② 写操作前先自检：**

```bash
aws sts get-caller-identity --profile hhxj --region us-east-1
# 必须看到 "Account": "581237505531"
# 看到 746669197317 就是串台了，立刻停手
```

**③ 新项目的 `CLAUDE.md` 里固定写死这段约定：**

```markdown
## AWS 约定
- 只用 profile `hhxj`（账号 581237505531），区域固定 us-east-1。
- 所有 aws 命令必须带 `--profile hhxj --region us-east-1`。
- 禁止使用 `default` profile（那是 starhold 生产账号 746669197317）。
- 写操作前先 `aws sts get-caller-identity --profile hhxj` 确认 Account。
```

**④ 更彻底**：新机器上只保留 `[hhxj]`，把 `[default]` 整段删掉。

---

## 4. 区域说明

| 服务 | 区域要求 |
|---|---|
| S3（创建桶） | 桶所在区域创建后不可改；建议统一 `us-east-1` |
| CloudFront | 全局服务，命令带 `--region us-east-1` 即可 |
| ACM（证书） | **必须 us-east-1**，否则 CloudFront 不能引用 |
| STS / IAM | 全局，region 可省 |

如果主要用户在中国大陆，`us-east-1` 的 S3/CloudFront 回源和边缘延迟都偏高：

- 可把桶建在 `ap-northeast-1`（东京）或 `ap-southeast-1`（新加坡），CloudFront 选 `PriceClass_200`；
- 对大陆用户更现实的是用国内 CDN（阿里云/腾讯云）+ 国内 OSS，CloudFront 在大陆没有优质边缘节点。

---

## 5. 对象存储（S3）

### 5.1 命名与约定

- 桶名**全球唯一**，只能小写字母、数字、连字符，3–63 字符，不能有下划线和大写。
- 建议按「项目-用途-环境」命名，例如 `hhxj-game-assets-prod`、`myproj-web-static`。
- 2023 年 4 月后新建的桶默认**禁用 ACL**，`--acl public-read` 会直接报错；公开读取只能用「桶策略」或「CloudFront OAC」。

### 5.2 建桶

```bash
aws s3api create-bucket \
  --bucket myproj-web-static \
  --profile hhxj --region us-east-1
```

（非 us-east-1 需加 `--create-bucket-configuration LocationConstraint=<region>`）

建议顺手开启版本控制（防误删、可回滚）：

```bash
aws s3api put-bucket-versioning \
  --bucket myproj-web-static \
  --versioning-configuration Status=Enabled \
  --profile hhxj --region us-east-1
```

### 5.3 上传文件

单文件：

```bash
aws s3 cp ./dist/app.js "s3://myproj-web-static/app/app.js" \
  --profile hhxj --region us-east-1 \
  --content-type "application/javascript" \
  --cache-control "public, max-age=31536000, immutable"
```

目录同步（**发布静态站点最常用**）：

```bash
aws s3 sync ./dist "s3://myproj-web-static/web" \
  --profile hhxj --region us-east-1 \
  --delete \
  --exclude "*.map" --exclude ".DS_Store"
```

> `--delete` 会**删除目标端多余文件**，第一次务必先加 `--dryrun` 看清单：
> `aws s3 sync ./dist s3://bucket/web --delete --dryrun ...`

缓存策略要按文件类型分开（这是最容易做错的地方）：

```bash
# 带内容哈希的构建产物：一年 + immutable
aws s3 sync ./dist/assets "s3://myproj-web-static/assets" \
  --profile hhxj --region us-east-1 \
  --cache-control "public, max-age=31536000, immutable"

# 入口文件：不缓存，保证发版立即生效
aws s3 cp ./dist/index.html "s3://myproj-web-static/index.html" \
  --profile hhxj --region us-east-1 \
  --content-type "text/html; charset=utf-8" \
  --cache-control "no-cache"
```

常用 Content-Type 对照（`aws s3 sync` 按扩展名猜，猜错浏览器就会变成"下载文件"）：

| 扩展名 | Content-Type |
|---|---|
| `.html` | `text/html; charset=utf-8` |
| `.js` / `.mjs` | `application/javascript` |
| `.css` | `text/css` |
| `.json` | `application/json` |
| `.wasm` | `application/wasm` |
| `.unityweb` | `application/octet-stream`（若已 gzip，加 `--content-encoding gzip`） |
| `.glb` / `.gltf` | `model/gltf-binary` / `model/gltf+json` |
| `.woff2` | `font/woff2` |

只改元数据、不重新上传：

```bash
aws s3 cp "s3://bucket/path/x.js" "s3://bucket/path/x.js" \
  --metadata-directive REPLACE \
  --content-type "application/javascript" \
  --cache-control "public, max-age=31536000, immutable" \
  --profile hhxj --region us-east-1
```

大文件（>100MB）自动走分片上传，无需额外参数；想要更快可调并发：

```bash
aws configure set default.s3.max_concurrent_requests 20 --profile hhxj
aws configure set default.s3.multipart_threshold 64MB --profile hhxj
```

### 5.4 查询与预签名

```bash
# 看桶内容
aws s3 ls "s3://myproj-web-static/" --recursive --human-readable --summarize \
  --profile hhxj --region us-east-1

# 生成临时下载链接（私有对象，默认 GET）
aws s3 presign "s3://myproj-web-static/private/a.png" \
  --expires-in 3600 --profile hhxj --region us-east-1
```

> CLI 只能生成 **GET** 预签名。若要客户端直传（PUT 预签名），需用 SDK（boto3 / aws-sdk）在服务端生成。

### 5.5 CORS（网页加载字体 / WebGL 资源必备）

`cors.json`：

```json
{
  "CORSRules": [
    {
      "AllowedOrigins": ["*"],
      "AllowedMethods": ["GET", "HEAD"],
      "AllowedHeaders": ["*"],
      "ExposeHeaders": ["ETag"],
      "MaxAgeSeconds": 86400
    }
  ]
}
```

```bash
aws s3api put-bucket-cors --bucket myproj-web-static \
  --cors-configuration file://cors.json \
  --profile hhxj --region us-east-1
```

### 5.6 生命周期（清理临时文件 / 旧版本）

`lifecycle.json`：

```json
{
  "Rules": [
    {
      "ID": "expire-tmp",
      "Filter": { "Prefix": "tmp/" },
      "Status": "Enabled",
      "Expiration": { "Days": 7 }
    },
    {
      "ID": "expire-old-versions",
      "Filter": {},
      "Status": "Enabled",
      "NoncurrentVersionExpiration": { "NoncurrentDays": 30 }
    }
  ]
}
```

```bash
aws s3api put-bucket-lifecycle-configuration --bucket myproj-web-static \
  --lifecycle-configuration file://lifecycle.json \
  --profile hhxj --region us-east-1
```

---

## 6. CDN

### 6.1 方案 A：CloudFront + S3（推荐，AWS 内闭环）

架构：`浏览器 → CloudFront（缓存/HTTPS/自定义域名）→ S3（私有桶，OAC 鉴权）`

#### 步骤 1：创建 OAC（Origin Access Control）

```bash
OAC_ID=$(aws cloudfront create-origin-access-control \
  --origin-access-control-config \
  "Name=assets-oac,OriginAccessControlOriginType=s3,SigningBehavior=always,SigningProtocol=sigv4" \
  --profile hhxj --region us-east-1 \
  --query 'OriginAccessControl.Id' --output text)
echo "$OAC_ID"
```

#### 步骤 2：桶保持私有，只允许该 CloudFront 分发读取

`bucket-policy.json`（把 `BUCKET`、`DIST_ID` 换掉）：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowCloudFrontOAC",
      "Effect": "Allow",
      "Principal": { "Service": "cloudfront.amazonaws.com" },
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::BUCKET/*",
      "Condition": {
        "StringEquals": {
          "AWS:SourceArn": "arn:aws:cloudfront::581237505531:distribution/DIST_ID"
        }
      }
    }
  ]
}
```

```bash
aws s3api put-bucket-policy --bucket myproj-web-static \
  --policy file://bucket-policy.json \
  --profile hhxj --region us-east-1
```

> 分发创建后才能拿到 `DIST_ID`，所以顺序是：建 OAC → 建分发 → 回填桶策略。

#### 步骤 3：创建分发

`cf-dist.json`：

```json
{
  "CallerReference": "myproj-web-20261105",
  "Comment": "myproj web assets",
  "Enabled": true,
  "DefaultRootObject": "index.html",
  "Origins": {
    "Quantity": 1,
    "Items": [
      {
        "Id": "s3-assets",
        "DomainName": "myproj-web-static.s3.us-east-1.amazonaws.com",
        "OriginPath": "",
        "S3OriginConfig": { "OriginAccessIdentity": "" },
        "OriginAccessControlId": "OAC_ID"
      }
    ]
  },
  "DefaultCacheBehavior": {
    "TargetOriginId": "s3-assets",
    "ViewerProtocolPolicy": "redirect-to-https",
    "TrustedSigners": { "Enabled": false, "Quantity": 0 },
    "AllowedMethods": {
      "Quantity": 2,
      "Items": ["GET", "HEAD"],
      "CachedMethods": { "Quantity": 2, "Items": ["GET", "HEAD"] }
    },
    "ForwardedValues": {
      "QueryString": false,
      "Cookies": { "Forward": "none" }
    },
    "MinTTL": 0,
    "Compress": true
  },
  "PriceClass": "PriceClass_100",
  "HttpVersion": "http2and3",
  "IsIPV6Enabled": true,
  "ViewerCertificate": { "CloudFrontDefaultCertificate": true }
}
```

```bash
aws cloudfront create-distribution \
  --distribution-config file://cf-dist.json \
  --profile hhxj --region us-east-1 \
  --query 'Distribution.{Id:Id,Domain:DomainName}'
```

拿到 `dxxxx.cloudfront.net` 即可访问。SPA 如果要用前端路由，加一段 `CustomErrorResponses`：

```json
"CustomErrorResponses": {
  "Quantity": 2,
  "Items": [
    { "ErrorCode": 403, "ResponseCode": "200", "ResponsePagePath": "/index.html", "ErrorCachingMinTTL": 0 },
    { "ErrorCode": 404, "ResponseCode": "200", "ResponsePagePath": "/index.html", "ErrorCachingMinTTL": 0 }
  ]
}
```

#### 步骤 4：绑定自定义域名（可选）

1. 在 **us-east-1** 申请 ACM 证书（DNS 验证）：

```bash
aws acm request-certificate \
  --domain-name cdn.yourdomain.com \
  --validation-method DNS \
  --profile hhxj --region us-east-1 \
  --query CertificateArn --output text

aws acm describe-certificate --certificate-arn <ARN> \
  --profile hhxj --region us-east-1 \
  --query 'Certificate.DomainValidationOptions'
```

2. 把返回的 `ResourceRecord`（CNAME）加到 GoDaddy 的 DNS 里，等状态变 `ISSUED`。
3. 修改分发：`Aliases` 加域名，`ViewerCertificate` 换成：

```json
"ViewerCertificate": {
  "AcmCertificateArn": "arn:aws:acm:us-east-1:581237505531:certificate/xxx",
  "SslSupportMethod": "sni-only",
  "MinimumProtocolVersion": "TLSv1.2_2021"
}
```

4. 域名商处加 CNAME：`cdn.yourdomain.com → dxxxx.cloudfront.net`。

#### 步骤 5：发布后刷新缓存

```bash
aws cloudfront create-invalidation \
  --distribution-id DIST_ID \
  --paths "/*" \
  --profile hhxj --region us-east-1
```

> 每月前 1000 条失效路径免费，之后 $0.005/条。文件多时尽量**用带哈希的文件名 + 只失效 `index.html` / `/*` 中的少数路径**，不要每条都刷。
> 更好的做法：静态资源文件名带 hash（`app.a1b2c3.js`）并设一年缓存，发版只上传新文件、只刷新入口页。

### 6.2 方案 B：Cloudflare 在最外层

现有生产环境用的是 Cloudflare（域名解析也在 GoDaddy），有两种接法：

**B1（推荐）：Cloudflare DNS 指向 CloudFront**

- 在 Cloudflare 加 CNAME：`cdn.yourdomain.com → dxxxx.cloudfront.net`。
- 如果只做 DNS（灰云 / Proxied 关闭），浏览器直连 CloudFront，HTTPS 由 CloudFront 的 ACM 证书负责，**最简单、最稳**。
- 如果要开橙云（Cloudflare 代理）：SSL/TLS 模式设为 **Full (strict)**，并在 CloudFront 的 `Aliases` 里加上该域名、配好 ACM 证书，否则会 526/525 证书错误。
- 此时 S3 仍是私有桶 + OAC，安全模型不变。

**B2（不推荐）：Cloudflare 直接回源 S3**

- S3 桶必须**公开读**（Cloudflare 回源时没有 OAC 身份）：
  1. 关掉桶级公共访问块：
     ```bash
     aws s3api put-public-access-block --bucket BUCKET \
       --public-access-block-configuration \
       "BlockPublicAcls=false,IgnorePublicAcls=false,BlockPublicPolicy=false,RestrictPublicBuckets=false" \
       --profile hhxj --region us-east-1
     ```
  2. 加公开读策略：
     ```json
     {
       "Version": "2012-10-17",
       "Statement": [{
         "Sid": "PublicReadGetObject",
         "Effect": "Allow",
         "Principal": "*",
         "Action": "s3:GetObject",
         "Resource": "arn:aws:s3:::BUCKET/*"
       }]
     }
     ```
- **关键坑**：Cloudflare 代理会保留自定义域名的 Host 头，而 S3 按 Host 找桶。
  只有当**桶名与自定义域名完全一致**（如桶 `cdn.example.com`）时，CNAME 到
  `s3.us-east-1.amazonaws.com` 才能命中；否则 S3 返回 404/403。
  绕法是用 Cloudflare Transform Rule 改写 Host，或者干脆用 B1。
- 回源地址用区域端点 `BUCKET.s3.us-east-1.amazonaws.com`，**不要**用 website endpoint（只支持 HTTP）。
- 公开桶意味着任何人拿到 URL 都能读，**不要放私有资源**；发版后记得 purge Cloudflare 缓存，或使用带哈希的文件名。

### 6.3 方案 C：Cloudflare R2（如果主要目标是省流量费）

R2 完全兼容 S3 API，出网流量免费，适合纯静态资源。用法上只需把 CLI 的 endpoint 换掉：

```bash
export AWS_ACCESS_KEY_ID=<R2 AK>
export AWS_SECRET_ACCESS_KEY=<R2 SK>
aws s3 sync ./dist s3://my-bucket --endpoint-url https://<account-id>.r2.cloudflarestorage.com
```

新版本 AWS CLI 默认校验和算法可能让 R2/MinIO 报错，加：

```bash
export AWS_REQUEST_CHECKSUM_CALCULATION=when_required
```

---

## 7. 一键上传脚本（可直接复制到新项目）

### 7.1 PowerShell（Windows）

`publish-cdn.ps1`：

```powershell
<#
.SYNOPSIS
  上传静态资源到 S3 并刷新 CloudFront。
.EXAMPLE
  .\publish-cdn.ps1 -Bucket myproj-web-static -Source .\dist -Prefix web -DistributionId E1234567890
#>
param(
    [Parameter(Mandatory = $true)][string]$Bucket,
    [Parameter(Mandatory = $true)][string]$Source,
    [string]$Prefix = "",
    [string]$DistributionId = "",
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$AwsProfile = "hhxj"   # 不要用 $Profile，它是 PowerShell 的自动变量
$Region     = "us-east-1"

# 1) 账号自检，防止串到 default 生产账号
$account = aws sts get-caller-identity --profile $AwsProfile --region $Region --query Account --output text
if ($account -ne "581237505531") {
    throw "当前账号是 $account，不是 hhxj(581237505531)，已中止。"
}

$target = if ($Prefix) { "s3://$Bucket/$Prefix" } else { "s3://$Bucket" }
$common = @("--profile", $AwsProfile, "--region", $Region)

# 2) 带哈希的静态资源：长缓存
$assetsDir = Join-Path $Source "assets"
if (Test-Path $assetsDir) {
    $targetAssets = if ($Prefix) { "s3://$Bucket/$Prefix/assets" } else { "s3://$Bucket/assets" }
    $awsArgs = @("s3", "sync", $assetsDir, $targetAssets) + $common +
               @("--cache-control", "public, max-age=31536000, immutable",
                 "--exclude", "*.map")
    if ($DryRun) { $awsArgs += "--dryrun" }
    & aws @awsArgs
}

# 3) 其余文件：短缓存 + 删除多余文件（先 DryRun 确认）
$awsArgs = @("s3", "sync", $Source, $target) + $common +
           @("--delete",
             "--cache-control", "public, max-age=300",
             "--exclude", "assets/*",
             "--exclude", "*.map")
if ($DryRun) { $awsArgs += "--dryrun" }
& aws @awsArgs

# 4) 刷新 CDN
if ($DistributionId -and -not $DryRun) {
    aws cloudfront create-invalidation `
        --distribution-id $DistributionId `
        --paths "/*" `
        --profile $AwsProfile --region $Region | Out-Null
    Write-Host "CloudFront 缓存已刷新：$DistributionId"
}

Write-Host "发布完成：$target"
```

### 7.2 Bash（Linux / macOS / CI）

`publish-cdn.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

BUCKET="${BUCKET:?需要设置 BUCKET}"
SOURCE="${SOURCE:-./dist}"
PREFIX="${PREFIX:-}"
DISTRIBUTION_ID="${DISTRIBUTION_ID:-}"
# 本地默认用 hhxj profile；CI 里用环境变量凭证时把 AWS_PROFILE 设为空字符串
PROFILE="${AWS_PROFILE-hhxj}"
REGION="${AWS_REGION:-us-east-1}"
DRY_RUN="${DRY_RUN:-0}"

PROFILE_ARGS=()
if [ -n "$PROFILE" ]; then PROFILE_ARGS=(--profile "$PROFILE"); fi

# 账号自检
account=$(aws sts get-caller-identity "${PROFILE_ARGS[@]}" --region "$REGION" --query Account --output text)
if [ "$account" != "581237505531" ]; then
  echo "当前账号是 $account，不是 hhxj(581237505531)，已中止。" >&2
  exit 1
fi

aws_sync() {
  local args=("$@")
  if [ "$DRY_RUN" = "1" ]; then args+=(--dryrun); fi
  aws s3 sync "${args[@]}" "${PROFILE_ARGS[@]}" --region "$REGION"
}

target="s3://$BUCKET${PREFIX:+/$PREFIX}"

# 带哈希资源：长缓存
if [ -d "$SOURCE/assets" ]; then
  aws_sync "$SOURCE/assets" "$target/assets" \
    --cache-control "public, max-age=31536000, immutable" \
    --exclude "*.map"
fi

# 其余文件：短缓存，--delete 清掉旧文件
aws_sync "$SOURCE" "$target" \
  --delete \
  --cache-control "public, max-age=300" \
  --exclude "assets/*" \
  --exclude "*.map"

if [ -n "$DISTRIBUTION_ID" ] && [ "$DRY_RUN" != "1" ]; then
  aws cloudfront create-invalidation \
    --distribution-id "$DISTRIBUTION_ID" --paths "/*" \
    "${PROFILE_ARGS[@]}" --region "$REGION" >/dev/null
  echo "CloudFront 缓存已刷新：$DISTRIBUTION_ID"
fi

echo "发布完成：$target"
```

> 脚本里的账号自检是刻意加的：本机 `default` 是生产账号，任何自动脚本都值得先确认 `Account == 581237505531`。

---

## 8. CI/CD 接入（GitHub Actions 等）

**不要把 hhxj 的管理员密钥塞进 CI。** 给发布流程单建一个最小权限 IAM 用户或 OIDC 角色。

### 8.1 最小权限策略

`deploy-policy.json`：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
      "Resource": "arn:aws:s3:::myproj-web-static"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:GetObject",
        "s3:AbortMultipartUpload",
        "s3:ListMultipartUploadParts"
      ],
      "Resource": "arn:aws:s3:::myproj-web-static/*"
    },
    {
      "Effect": "Allow",
      "Action": ["cloudfront:CreateInvalidation", "cloudfront:GetInvalidation"],
      "Resource": "arn:aws:cloudfront::581237505531:distribution/DIST_ID"
    }
  ]
}
```

```bash
aws iam create-policy --policy-name MyProj-CdnDeploy \
  --policy-document file://deploy-policy.json \
  --profile hhxj --region us-east-1

aws iam create-user --user-name myproj-cdn-deploy --profile hhxj --region us-east-1
aws iam attach-user-policy --user-name myproj-cdn-deploy \
  --policy-arn arn:aws:iam::581237505531:policy/MyProj-CdnDeploy \
  --profile hhxj --region us-east-1
aws iam create-access-key --user-name myproj-cdn-deploy --profile hhxj --region us-east-1
```

### 8.2 GitHub Actions 示例（密钥方式）

```yaml
name: publish-cdn
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
      AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
      AWS_DEFAULT_REGION: us-east-1
      AWS_PROFILE: ""          # CI 用环境变量凭证，不读本地 profile
    steps:
      - uses: actions/checkout@v4
      - name: 账号自检
        run: |
          account=$(aws sts get-caller-identity --query Account --output text)
          [ "$account" = "581237505531" ] || { echo "账号错误: $account"; exit 1; }
      - name: 发布
        run: |
          BUCKET=myproj-web-static DISTRIBUTION_ID=E1234567890 ./publish-cdn.sh
```

更安全的做法是 GitHub OIDC 联合身份（`AssumeRoleWithWebIdentity`），无需长期密钥；首次配置稍多，建议后续单独做。

---

## 9. 费用参考（us-east-1，2026 年，实际以官网为准）

| 项 | 大致单价 |
|---|---|
| S3 标准存储 | $0.023 / GB-月 |
| S3 PUT 请求 | $0.005 / 1000 次 |
| S3 GET 请求 | $0.0004 / 1000 次 |
| S3 直接公网出流量 | 约 $0.09 / GB |
| CloudFront 出流量 | 约 $0.085 / GB（每月 1TB + 1000 万请求有免费额度） |
| CloudFront 失效 | 前 1000 条路径/月免费，之后 $0.005/条 |
| ACM 证书 | 免费 |

省钱要点：

- 静态资源全部走 CloudFront，避免 S3 直接出公网；
- 文件名带哈希 + 长缓存，减少回源和失效次数；
- 不用的旧版本用 Lifecycle 清掉；
- 大流量场景优先考虑 Cloudflare R2（出网免费）或国内 CDN。

---

## 10. 常见坑

| # | 坑 | 现象 / 处理 |
|---|---|---|
| 1 | 漏写 `--profile hhxj` | 静默落到 starhold 生产账号 `746669197317`，不报错。命令永远带 profile + 自检 |
| 2 | 漏写 `--region us-east-1` | `You must specify a region.`；S3 有时能过，但别依赖 |
| 3 | `--acl public-read` | 新桶 ACL 已禁用，报 `AccessControlListNotSupported`，改用桶策略 / OAC |
| 4 | 公开桶策略报 AccessDenied | 桶级「阻止公共访问」没关；先 `put-public-access-block` 再放策略 |
| 5 | 桶名冲突 | 桶名全球唯一，被占就换名 |
| 6 | `s3 sync --delete` | 会删目标端多余文件，先 `--dryrun` |
| 7 | Content-Type 错 | 浏览器把 js/css 当文件下载；用 `--content-type` 显式指定 |
| 8 | 发版后看到的还是旧页面 | CloudFront 缓存；刷新 `/*` 或给入口页设 `no-cache` |
| 9 | 自定义域名 403 / 证书错 | ACM 证书必须在 **us-east-1**；CNAME 没加或还没签发 |
| 10 | 大陆访问慢 | us-east-1 边缘远；换区域或使用国内 CDN |
| 11 | 第三方 S3（R2/MinIO）校验和报错 | `export AWS_REQUEST_CHECKSUM_CALCULATION=when_required` |
| 12 | 公开桶放私有文件 | 公开桶任何拿到 URL 的人都能读；私有资源用预签名 URL 或 CloudFront 签名 |
| 13 | 域名拼写 | 真实域名是 `sfgames.io`，`sfgame.io` 不存在 |
| 14 | `--delete` 配合 `--exclude` | 被 exclude 的文件不会删，但同步逻辑容易误判，先在测试桶验证 |

---

## 11. 新项目接入检查清单

- [ ] `aws --version` 正常（建议 v2）
- [ ] 新机器上只剩 `[hhxj]`（或至少不再默认使用 `default`）
- [ ] `aws sts get-caller-identity --profile hhxj` → `581237505531`
- [ ] `aws configure set region us-east-1 --profile hhxj`
- [ ] 新项目 `CLAUDE.md` 写入第 3 节的 AWS 约定
- [ ] 创建业务桶（版本控制可选）
- [ ] 配置 CORS
- [ ] 创建 OAC + CloudFront 分发（或接入 Cloudflare 回源）
- [ ] （可选）申请 ACM 证书并绑定自定义域名
- [ ] 用 `--dryrun` 跑一次上传脚本，再正式发布
- [ ] 确认缓存策略：哈希资源长缓存、入口文件短缓存
- [ ] CI 使用独立的最小权限账号，不共用管理员密钥
- [ ] `.gitignore` 排除密钥、`*.pem`、`.env`

---

## 附录：本文事实来源

| 事实 | 验证方式（2026-11-05） |
|---|---|
| 账号 `581237505531` / 用户 `hhxj` | `aws sts get-caller-identity --profile hhxj` |
| 管理员权限 | `aws iam list-attached-user-policies --user-name hhxj` |
| 仅有 CloudTrail 桶 | `aws s3api list-buckets --profile hhxj` |
| 无 CloudFront 分发 | `aws cloudfront list-distributions` 返回空 |
| 无 ACM 证书 | `aws acm list-certificates --region us-east-1` 返回空 |
| 无 Route53 托管区 | `aws route53 list-hosted-zones` 返回空 |
| CLI 版本 2.34.44 | `aws --version` |
