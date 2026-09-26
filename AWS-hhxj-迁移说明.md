# AWS 账号 `hhxj` 说明与迁移手册（RDS PostgreSQL + EC2 应用主机）

> 用途：把当前机器上已配置好的 AWS 账号 `hhxj` 迁移到**另一个项目**，用 Claude Code 继续完成
> 「创建 PostgreSQL 数据库 + EC2 主机」的落地操作。
>
> 本文所有资源 ID / 账号 ID 均为 2026-09-24 在本机实测得到，可直接引用。

---

## 0. 一句话结论

`hhxj` 是一个**独立于 starhold 生产账号的、基本空白的 AWS 账号**，已具备 `AdministratorAccess` 全量权限。
迁移时只需搬 **`~/.aws/credentials` 里的 `[hhxj]` 这一段**，然后在所有命令前显式指定
`AWS_PROFILE=hhxj` + `--region us-east-1`，即可在新项目里直接建库建机。

**最大的风险不是权限不够，而是账号串台**——详见第 2 节。

---

## 1. 账号身份与凭证

### 1.1 账号基本信息

| 项 | 值 |
|---|---|
| AWS 账号 ID | `581237505531` |
| 账号别名 | 未设置 |
| IAM 用户 | `hhxj` |
| 用户 ARN | `arn:aws:iam::581237505531:user/hhxj` |
| UserId | `AIDAYOVDUPX56DJLULZ3G` |
| AWS CLI Profile 名 | **`hhxj`** |
| 默认区域 | **未设置（必须手动指定）** |
| 建议区域 | `us-east-1` |

> ⚠️ **拼写陷阱**：本项目里同时存在两个近似拼写，指的是**两个不同的东西**，别混：
>
> | 拼写 | 含义 |
> |---|---|
> | `hhxj` | IAM 用户名 / AWS CLI profile 名（**切换账号时用这个**） |
> | `hhjx` | 账号内已存在的 **EC2 密钥对**名（`KeyName=hhjx`，RSA） |
>
> 用户口头常说的 "hhjx 账号"，实际指的是 profile `hhxj`。

### 1.2 权限

该用户挂载了以下策略，**权限完整，无需再申请**：

```
AdministratorAccess            ← 全量，其余策略其实都是冗余
AmazonRDSFullAccess            ← 建 RDS
AmazonVPCFullAccess            ← 建 VPC / 子网 / 安全组
AmazonEKSClusterPolicy
AmazonEKSWorkerNodePolicy
AmazonEKS_CNI_Policy
ElasticLoadBalancingFullAccess
CloudWatchFullAccessV2
createkeypair                  ← 自定义策略，但 Resource ARN 写错了（.../key-pair/s），实际无效
GameBackend-DeployPolicy       ← 自定义策略
```

> 注：`createkeypair` 策略的 Resource 是 `arn:aws:ec2:us-east-1:581237505531:key-pair/s`（多了个 `/s`），
> 这条策略**本身不生效**。但因为有 `AdministratorAccess`，创建密钥对照样能成功，不用管它。

### 1.3 凭证在哪里

凭证存于本机 **`C:\Users\Administrator\.aws\credentials`**，含两段：

```ini
[default]
aws_access_key_id = AKIA******************
aws_secret_access_key = ******************

[hhxj]
aws_access_key_id = AKIA******************
aws_secret_access_key = ******************
```

配置存于 **`C:\Users\Administrator\.aws\config`**：

```ini
[default]
region = us-east-1

[profile hhxj]
output = json
# ⚠️ 注意：这里没有 region，也没有 output 之外的任何配置
```

> 🔒 **安全提示**：本文档**不写入** Access Key 明文。迁移时直接复制上述文件里的 `[hhxj]` 段即可。
> 更稳妥的做法是：在新机器上 `aws iam create-access-key --user-name hhxj` 重新签一对，
> 旧的那对在迁移完成后 `aws iam update-access-key --status Inactive` 停用。

---

## 2. ⚠️ 最高优先级风险：两个 AWS 账号并存

**本机同时配置了两个 AWS 账号**，且默认走的是**生产账号**：

| Profile | 账号 ID | IAM 用户 | 性质 |
|---|---|---|---|
| `default` | `746669197317` | `starhold-cdn-user` | **starhold 线上生产环境**（EKS + RDS + ECR + Cloudflare CDN） |
| `hhxj` | `581237505531` | `hhxj` | 本次要用的**新账号**（空白） |

**后果**：在新项目里敲 `aws ...` 而不指定 profile，命中的是 **starhold 生产账号**。
建实例、删资源、改安全组都可能直接打到线上环境。这不是理论风险，是本机默认行为。

### 必须做的三件事（在新项目里）

**① 项目级固定 profile**——把下面这行写进新项目的 `CLAUDE.md`：

```markdown
## AWS 约定
- 本项目**只用** profile `hhxj`（账号 581237505531），区域固定 `us-east-1`。
- 任何 aws 命令必须显式带 `AWS_PROFILE=hhxj` 与 `--region us-east-1`。
- **禁止**使用 `default` profile（那是 starhold 生产账号 746669197317）。
- 执行任何写操作（create/delete/modify）前，先跑
  `aws sts get-caller-identity --profile hhxj` 确认 Account 是 581237505531。
```

**② 每次会话先自检**——养成开手第一件事跑这个的习惯：

```bash
aws sts get-caller-identity --profile hhxj
# 期望输出：  "Account": "581237505531"
# 如果看到 746669197317，说明串台了，立刻停下
```

**③ 更彻底的做法**——在新机器/新环境上，**只保留 `[hhxj]` 一段**，
把 `[default]` 整段删掉，并显式设置 region：

```bash
aws configure set region us-east-1 --profile hhxj
aws configure set output json   --profile hhxj
```

### 切换用法速查（三种方式）

```bash
# 方式一：单条命令临时切换（推荐，语义最清晰，不会污染环境）
aws ec2 describe-instances --profile hhxj --region us-east-1

# 方式二：当前 shell 会话内切换（一次设置，后续命令都生效）
export AWS_PROFILE=hhxj        # Git Bash / Linux / macOS
$env:AWS_PROFILE="hhxj"        # PowerShell
# 注意：方式二仍需每条命令带 --region，因为 profile 里没配 region

# 方式三：持久化切换（改配置文件，最省事但最容易忘）
aws configure set region us-east-1 --profile hhxj
export AWS_PROFILE=hhxj        # 仍需每次 shell 设置，profile 不会自动成为默认
```

> **关于 region 的坑**：`[profile hhxj]` 段里**没有 region**。
> 不带 `--region` 时，除了 `sts`、`iam` 这类全局服务，**绝大多数命令会直接报错**
> （`You must specify a region.`）。所以 `--region us-east-1` 不是可选项，是必须项。

---

## 3. 账号现有资源清单（迁移起点）

实测（`us-east-1`，2026-09-24）：

### 3.1 已有

| 类型 | 资源 | 说明 |
|---|---|---|
| VPC | `vpc-005405572d9a09130` | **默认 VPC**，CIDR `172.31.0.0/16` |
| 子网 ×6 | `subnet-0c5efcf3d66995776` (1a)<br>`subnet-0c625c74e736efe91` (1b)<br>`subnet-08f376b041d86f66a` (1c)<br>`subnet-084e139bbd0e52b59` (1d)<br>`subnet-00c56d3a93067ce60` (1e)<br>`subnet-009e60ebe5013ceeb` (1f) | 全部 `MapPublicIpOnLaunch=True`<br>各 4091 个空闲 IP |
| 互联网网关 | `igw-053d28b831dd2fd92` | 已挂到默认 VPC |
| 路由表 | 默认路由 `0.0.0.0/0 → igw-053d28b831dd2fd92` | **出公网正常** |
| 安全组 | `sg-0de29aa875f5ea76d` (`default`) | 默认组 |
| 安全组 | `sg-0ee632ee802ac3ae0` (`hhjy`) | ⚠️ **22 端口对 `0.0.0.0/0` 全开** |
| 密钥对 | `hhjx`（RSA） | 见下方 3.3 说明 |
| 可用 AZ | us-east-1a / b / c / d / e / f | 6 个 |

### 3.2 没有（都需要新建）

| 类型 | 状态 |
|---|---|
| EC2 实例 | ❌ 无（8 个区域扫描全为 0） |
| RDS 实例 | ❌ 无 |
| **RDS 子网组** | ❌ **无**——建库前必须先建，否则 CLI 建库直接报错 |
| ECR 镜像仓库 | ❌ 无（镜像仓库都在另一个账号 746669197317） |
| IAM 角色 / 实例配置文件 | ❌ 无——想用 SSM Session Manager 代替 SSH 的话需要新建 |
| 非默认 VPC | ❌ 无 |



## 4. 目标架构

```
        Internet
           │
           │ 22 (仅限你的出口 IP)
           ▼
   ┌───────────────────┐        5432        ┌──────────────────────┐
   │  EC2 应用主机      │ ─────────────────▶ │  RDS PostgreSQL      │
   │  AL2023 / t3.medium│                    │  16.13 / 私网不可公网 │
   │  默认 VPC 公网子网  │                    │  仅接受来自 EC2 SG     │
   └───────────────────┘                    └──────────────────────┘
```

分层放行策略：**RDS 不开放公网，只允许 EC2 的应用安全组访问 5432**。

---

## 5. 实施步骤

> 以下命令均为**可直接执行**的形式。变量先按需改掉。
> 统一前缀约定：`export AWS_PROFILE=hhxj; R=us-east-1`

### 步骤 1：确认身份（每次操作前必做）

```bash
export AWS_PROFILE=hhxj
R=us-east-1
aws sts get-caller-identity
# 必须看到 "Account": "581237505531"
```

### 步骤 2：创建安全组（两个，职责分离）

```bash
# 2.1 应用主机 SG —— 只为 SSH 开口
APP_SG=$(aws ec2 create-security-group --region $R \
  --group-name game-app --description "App host SG" \
  --vpc-id vpc-005405572d9a09130 \
  --query 'GroupId' --output text)
echo "APP_SG=$APP_SG"

# ⚠️ 把 <你的出口IP> 换成 curl -s ifconfig.me 的结果，不要用 0.0.0.0/0
aws ec2 authorize-security-group-ingress --region $R \
  --group-id $APP_SG --protocol tcp --port 22 --cidr <你的出口IP>/32

# 2.2 数据库 SG —— 只接受来自应用 SG 的 5432
DB_SG=$(aws ec2 create-security-group --region $R \
  --group-name game-db --description "RDS PostgreSQL SG" \
  --vpc-id vpc-005405572d9a09130 \
  --query 'GroupId' --output text)
echo "DB_SG=$DB_SG"

aws ec2 authorize-security-group-ingress --region $R \
  --group-id $DB_SG --protocol tcp --port 5432 --source-group $APP_SG
```

> **不要复用现有的 `sg-0ee632ee802ac3ae0`（`hhjy`）**：它把 22 端口对全世界开放。
> 新建的组按需收窄，这是本账号目前最该修的安全问题。

### 步骤 3：创建 RDS 子网组（**不做这步建库必失败**）

RDS 在 VPC 内建实例**必须**有子网组，而本账号目前一个都没有。

```bash
aws rds create-db-subnet-group --region $R \
  --db-subnet-group-name game-pg-subnet \
  --db-subnet-group-description "Subnets for game PostgreSQL" \
  --subnet-ids subnet-0c5efcf3d66995776 subnet-0c625c74e736efe91
```

> 至少给 2 个**不同 AZ** 的子网。这里选了 1a + 1b。
> 注意：默认 VPC 的子网严格说都是"公网子网"，但配合 `--no-publicly-accessible`
> RDS 依然不会分配公网 IP，实际不可从外网直连，安全上可以接受。

### 步骤 4：创建 RDS PostgreSQL

```bash
aws rds create-db-instance --region $R \
  --db-instance-identifier game-prod-pg \
  --engine postgres --engine-version 16.13 \
  --db-instance-class db.t3.medium \
  --allocated-storage 50 --storage-type gp3 --storage-encrypted \
  --master-username game_admin \
  --manage-master-user-password \
  --db-name gamedb \
  --vpc-security-group-ids $DB_SG \
  --db-subnet-group-name game-pg-subnet \
  --backup-retention-period 7 \
  --preferred-backup-window "18:00-19:00" \
  --no-publicly-accessible \
  --no-multi-az \
  --deletion-protection
```

关键点逐条说明：

| 参数 | 为什么这么选 |
|---|---|
| `--engine-version 16.13` | 与 starhold 线上生产版本一致，避免行为差异。可选版本：15.7–15.19 / 16.3–16.15 / 17.5–17.11 / 18.1–18.6。想上新特性可用 `17.11` |
| `--manage-master-user-password` | 密码交给 Secrets Manager 托管，**不用在命令行传明文**（明文会进 shell history）。返回里会给 Secret ARN |
| `--storage-encrypted` | 加密存储，开建时就要定，事后改要重建 |
| `--no-publicly-accessible` | **关键安全项**，禁止公网访问 |
| `--deletion-protection` | 防误删。注意：真要删除时必须先关掉它 |
| `--backup-retention-period 7` | 自动备份 7 天 |
| `--no-multi-az` | 单 AZ 省钱。要高可用改 `--multi-az`（费用翻倍） |

> **建库耗时约 5–15 分钟**，之后用下面命令查状态，等到 `available`：
> ```bash
> aws rds wait db-instance-available --region $R --db-instance-identifier game-prod-pg
> aws rds describe-db-instances --region $R \
>   --db-instance-identifier game-prod-pg \
>   --query 'DBInstances[0].{Status:DBInstanceStatus,Endpoint:Endpoint.Address,Port:Endpoint.Port}'
> ```

### 步骤 5：创建密钥对

```bash
aws ec2 create-key-pair --region $R \
  --key-name game-app-key \
  --key-type rsa --key-format pem \
  --query 'KeyMaterial' --output text > game-app-key.pem

# Git Bash / Linux / macOS 下收权限；Windows 下 ssh 对权限要求较宽松
chmod 400 game-app-key.pem
```

> ⚠️ **私钥只在创建时返回一次**，务必落盘保存好，丢了只能重建密钥对并重开实例。
> ⚠️ Windows 下用 `--output text > file` 写出的文件有时带 BOM 或 CRLF，导致 SSH 报
> `invalid format`。如果遇到，用 `dos2unix` 或记事本另存为 UTF-8(无 BOM) + LF 修一下。

### 步骤 6：创建 EC2 应用主机

```bash
AMI=ami-0b2c9d1f3edcfd709     # Amazon Linux 2023 (2023.12.20260918.0, kernel 6.1, x86_64)

aws ec2 run-instances --region $R \
  --image-id $AMI \
  --instance-type t3.medium \
  --key-name game-app-key \
  --security-group-ids $APP_SG \
  --subnet-id subnet-0c5efcf3d66995776 \
  --associate-public-ip-address \
  --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=50,VolumeType=gp3,Encrypted=true}' \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=game-app}]' \
  --query 'Instances[0].{Id:InstanceId,AZ:Placement.AvailabilityZone}'
```

拿到 InstanceId 后等它跑起来并取公网 IP：

```bash
aws ec2 wait instance-running --region $R --instance-ids <InstanceId>
aws ec2 describe-instances --region $R --instance-ids <InstanceId> \
  --query 'Reservations[0].Instances[0].{PublicIp:PublicIpAddress,PrivateIp:PrivateIpAddress,State:State.Name}'
```

### 步骤 7：连上去装 PostgreSQL 客户端并验证

```bash
ssh -i game-app-key.pem ec2-user@<PublicIp>

# 实例上：
sudo dnf install -y postgresql16
psql -h <RDS Endpoint> -U game_admin -d gamedb -c "select version();"
# 密码从 Secrets Manager 取：
#   aws secretsmanager get-secret-value --secret-id <SecretArn> --query SecretString
```

> **PG 16 客户端连 RDS PG 16.13** 是匹配的。如果 RDS 选了 17/18，客户端也要装对应版本。

### 步骤 8（可选）：改用 SSM Session Manager，免密钥免 22 端口

本账号**无 IAM 角色和实例配置文件**，需要先建：

```bash
aws iam create-role --role-name EC2-SSM-Role \
  --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}'

aws iam attach-role-policy --role-name EC2-SSM-Role \
  --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore

aws iam create-instance-profile --instance-profile-name EC2-SSM-Profile
aws iam add-role-to-instance-profile \
  --instance-profile-name EC2-SSM-Profile --role-name EC2-SSM-Role

# 然后挂到实例上
aws ec2 associate-iam-instance-profile --region $R \
  --instance-id <InstanceId> \
  --iam-instance-profile Name=EC2-SSM-Profile

# 之后就能免密钥登录，并且可以从 SG 里删掉 22 端口规则
aws ssm start-session --region $R --target <InstanceId>
```

---

## 6. 注意事项汇总

### 6.1 必读（做错会出事）

1. **账号串台**——`default` profile 是 starhold 生产账号 `746669197317`。所有命令必须显式
   `--profile hhxj`，写操作前先 `aws sts get-caller-identity` 自检。
2. **region 必须显式传**——`[profile hhxj]` 里没有 region，漏掉 `--region us-east-1` 多数命令直接报错。
3. **RDS 子网组要先建**——账号里一个都没有，直接 `create-db-instance` 会失败。
4. **不要复用 `sg-0ee632ee802ac3ae0`（hhjy）**——它把 22 端口对 `0.0.0.0/0` 全开。
5. **RDS 主密码别走命令行明文**——用 `--manage-master-user-password` 交给 Secrets Manager；
   明文参数会留在 shell history 和进程列表里。
6. **`--deletion-protection` 是双刃剑**——开了之后想删库必须先
   `aws rds modify-db-instance --no-deletion-protection --apply-immediately`，否则删不掉。
7. **私钥只返回一次**——`create-key-pair` 的 `KeyMaterial` 错过就没了。

### 6.2 成本相关

| 资源 | 规格 | 大致费用（us-east-1，参考） |
|---|---|---|
| RDS | db.t3.medium + 50GiB gp3 | ~$60/月（单 AZ） |
| RDS | 同上 + `--multi-az` | 翻倍 |
| EC2 | t3.medium + 50GiB gp3 | ~$35/月（按需） |
| 公网 IPv4 | 每个弹性/自动公网 IP | 单独计费，注意别留闲置 IP |

> 不用的实例记得 `stop`（EC2）或 `delete`（RDS 需先关删除保护）。

### 6.3 与本项目（nakama-cluster）的差异

- **ECR 镜像仓库不通用**。starhold 的镜像在 `746669197317.dkr.ecr.us-east-1.amazonaws.com/starhold-us/*`，
  本账号 `581237505531` 下没有任何 ECR 仓库。新项目要发镜像需重新建：
  ```bash
  aws ecr create-repository --region us-east-1 --repository-name <name> \
    --image-scanning-configuration scanOnPush=true
  ```
- **EKS 不要照搬**。本账号无 EKS 集群，且本次目标是 EC2 直跑，不走 k8s。
  本项目的 `build/k8s/` 清单、`Starhold-infra.md` 里的 EKS/VPC/子网 ID **全部不适用**于本账号。
- **RDS 端点不同**。本项目 `Starhold-infra.md` 里的 `starhold-prod-pg.cne62kk8qcyy...` 是生产账号的库，
  与本账号无关，别连错。

---

## 7. 在新项目里给 Claude 的开场提示词

把下面这段直接粘到新项目的 Claude Code 会话里：

```text
在这个项目里操作 AWS，严格遵守以下约定：

1. 账号：只能使用 profile `hhxj`（账号 ID 581237505531，IAM 用户 hhxj），
   区域固定 us-east-1。所有 aws 命令必须带 `--profile hhxj --region us-east-1`。
2. 严禁使用 `default` profile —— 那是 starhold 生产账号 746669197317，
   误操作会影响线上。每次执行写操作前，先运行
   `aws sts get-caller-identity --profile hhxj` 并确认 Account == 581237505531。
3. 目标：创建 RDS PostgreSQL + EC2 应用主机。已有基础：
   - 默认 VPC vpc-005405572d9a09130
   - 子网 subnet-0c5efcf3d66995776(1a) / subnet-0c625c74e736efe91(1b)
   - 账号内无 RDS 子网组、无 EC2、无 ECR、无 IAM 实例配置文件，都需要新建
4. 安全底线：
   - 新建安全组，不要复用 sg-0ee632ee802ac3ae0（它 22 端口对 0.0.0.0/0 全开）
   - RDS 必须 --no-publicly-accessible，只放行应用 SG 到 5432
   - RDS 主密码用 --manage-master-user-password，不要在命令行传明文
   - SSH 只放行我的出口 IP /32，不要 0.0.0.0/0
5. 每建一个资源，回报资源 ID；建完给出连接验证命令。
```

---

## 8. 迁移检查清单

- [ ] 新机器上安装 AWS CLI v2，`aws --version` 正常
- [ ] 复制 `~/.aws/credentials` 的 `[hhxj]` 段（或新签一对 Access Key）
- [ ] `aws configure set region us-east-1 --profile hhxj`
- [ ] `aws sts get-caller-identity --profile hhxj` → Account = `581237505531`
- [ ] 新项目 `CLAUDE.md` 里写入第 7 节的 AWS 约定
- [ ] 确认新环境**没有** starhold 生产账号的 `default` profile（防串台）
- [ ] 迁移完成后，评估是否停用旧 Access Key：
      `aws iam update-access-key --user-name hhxj --access-key-id <旧AK> --status Inactive`

---

*生成时间：2026-09-24 · 实测环境：Windows 11 + AWS CLI 2.34.44 · 区域：us-east-1*
