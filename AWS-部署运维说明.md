# AWS 部署与运维说明（hjzx US 环境）

> 配套文档：`AWS-hhxj-迁移说明.md` 讲的是**怎么把 AWS 账号迁到新机器**；
> 本文讲的是**这套服务现在长什么样、怎么更新、出问题怎么查**。
>
> 首次部署：2026-09-24 · 最后更新：2026-09-26

---

## 0. 快速上手（更新并部署线上）

一条命令：

```bat
build\deploy-hjzx-us.bat --version 1.0.2
```

| 你要做的事 | 命令 |
|---|---|
| 改了服务代码 / Lua 模块 / 模板表 | `build\deploy-hjzx-us.bat --version 1.0.2` |
| 只改了 `local.yml` | `build\deploy-hjzx-us.bat --skip-build` |
| 只改了 compose（如健康检查） | `build\deploy-hjzx-us.bat --skip-build` |
| 只看用法 | `build\deploy-hjzx-us.bat --help` |
| 只想构建推送，不动服务器 | `build\build-hjzx-us.bat --version 1.0.2` |

脚本流程：构建并推送镜像 → 上传 compose 与 `local.yml` → 服务器 `pull` + `up -d` →
轮询健康检查 → 打印镜像、版本号、容器内 Lua 模块与模板表数量。
任一步失败即中止并提示，不会把服务器留在半途。

**给后续 Claude 的操作约定**：

1. **部署是对外操作，先确认用户确实要部署再执行**，不要因为"改了就该上线"顺手跑。
2. 本机所有 `aws` 命令必须显式带 `--profile hhxj --region us-east-1`；
   写操作前先 `aws sts get-caller-identity` 自检，**必须是 `581237505531`**。
   `default` profile 是 starhold 生产账号 `746669197317`，误用会直接打到线上。
3. `~/.nakama-deploy/bundle/data/local.yml` 含数据库口令和全部密钥：
   **不要把它的内容打印进对话**。要看结构就逐行打码，且注意 `database.address`
   是 YAML 列表项（行首 `- "`），普通"冒号后打码"的正则会漏掉它，把数据库口令原样打出来。
4. `build/*.bat` **只能写 ASCII**，见 5.1 末尾的说明。
5. 排查"线上行为与代码不符"时，**先核对线上版本号**，命令见 5.1。

---

## 1. 账号与操作约定

任何 `aws` 命令都必须显式带 profile 和 region：

```bash
export AWS_PROFILE=hhxj
R=us-east-1
aws sts get-caller-identity --region $R   # 必须看到 Account = 581237505531
```

> ⚠️ 本机同时存在 `default` profile，那是 **starhold 生产账号 `746669197317`**。
> 漏掉 `--profile hhxj` 会直接打到线上环境。写操作前先自检。

---

## 2. AWS 资源清单

| 类型 | ID / 值 |
|---|---|
| 区域 | `us-east-1` |
| VPC | `vpc-005405572d9a09130`（默认 VPC） |
| EC2 | `i-06c0667ab19d08355` · t3.medium · us-east-1a |
| EC2 公网 IP | `13.221.177.184` |
| EC2 私网 IP | `172.31.12.247` |
| EC2 根卷 | `vol-08a6bcb6d0d628c08` · 50GiB gp3 加密 |
| 密钥对 | `game-app-key` |
| 应用安全组 | `sg-098d10e23337e0812` (`game-app`) |
| 数据库安全组 | `sg-087f36cbceabb3c8e` (`game-db`) |
| IAM Role | `EC2-SSM-Role`（挂 `AmazonSSMManagedInstanceCore`） |
| Instance Profile | `EC2-SSM-Profile` |
| RDS | `game-prod-pg` · PostgreSQL 16.13 · db.t3.medium · 50GiB gp3 加密 |
| RDS 端点 | `game-prod-pg.c01gy2aowawf.us-east-1.rds.amazonaws.com:5432` |
| RDS 子网组 | `game-pg-subnet`（subnet 1a `subnet-0c5efcf3d66995776` + 1b `subnet-0c625c74e736efe91`） |
| 库 / 用户 | `gamedb` / `game_admin` |

RDS：`PubliclyAccessible=false`、`DeletionProtection=true`、备份保留 7 天、单 AZ。

### 端口与安全组（当前实际状态）

| 端口 | 用途 | 放行来源 |
|---|---|---|
| 7350 | 客户端 API + 实时 Socket | `0.0.0.0/0`（业务必须） |
| 7351 | 管理控制台 | `0.0.0.0/0`（**已公开**，见第 7 节风险） |
| 22 | SSH | 单个出口 IP /32，**会随本机出口 IP 变化而失效** |
| 7349 | gRPC | 仅容器内 `127.0.0.1`，不对外 |
| 5432 | RDS | 仅 `sg-098d10e23337e0812` |

---

## 3. 服务器上的目录结构（EC2 内）

```
/opt/nakama/
├── docker-compose.yml        服务定义（仓库 build/docker-compose.prod.yml）
└── local.yml                 Nakama 配置（权限 600，含数据库口令与各密钥）
```

容器内 `/nakama/data/` 的内容**全部来自镜像**（见 `build/Dockerfile`）：

```
/nakama/data/
├── local.yml           ← 由宿主机 /opt/nakama/local.yml 单文件挂载覆盖
├── modules/*.lua       9 个 Lua 运行时模块（镜像内）
└── Tpl*.json           6 张模板表（镜像内）
```

容器行为：

- 镜像 `docker.sparkinfi.com/hjzx/nakama-us:latest`，`restart: unless-stopped`，Docker 开机自启
- 启动命令先跑 `nakama migrate up`（幂等）再拉起服务
- 只做了**单文件**挂载 `/opt/nakama/local.yml:/nakama/data/local.yml:ro`，
  其余数据由镜像提供 —— 所以更新模板表/模块要重新构建镜像，不能靠挂载目录替换
- ⚠️ 第一次部署时宿主机 `/opt/nakama/local.yml` 必须先存在。若不存在，
  Docker 会把它当成目录创建，容器起不来（症状：`local.yml` 变成了一个目录）

---

## 4. 本机运维目录 `C:\Users\Administrator\.nakama-deploy\`

**刻意放在 git 仓库之外**，避免私钥被误提交。内容说明：

| 文件 | 用途 |
|---|---|
| `game-app-key.pem` | EC2 SSH 私钥。**只在创建时下发一次，丢失只能重建密钥对并重开实例** |
| `db_password.txt` | RDS `game_admin` 口令（32 位字母数字） |
| `console_password.txt` | 控制台 `admin` 口令（24 位字母数字） |
| `console_signing_key.txt` | 控制台签名密钥 |
| `session_encryption_key.txt` | session token 加密密钥 |
| `session_refresh_key.txt` | refresh token 加密密钥 |
| `runtime_http_key.txt` | runtime HTTP 调用密钥（换掉了默认值 `defaulthttpkey`） |
| `registry-cred.json` | `docker.sparkinfi.com` 仓库凭证（用户名 `admin`） |
| `rds-create.json` | 建库时的 CLI 参数（含口令，故不进 shell history） |
| `ec2-trust-policy.json` | SSM 用的 IAM 信任策略 |
| `ec2-userdata.sh` / `.lf.sh` | EC2 首次开机脚本（装 Docker / Compose / psql16） |
| `bundle/data/local.yml` | **仍在用**：线上配置的源文件，改配置从这里改（见 5.2） |
| `bundle/` 其余内容 | 首次部署的快照，已被仓库内 `build/docker-compose.prod.yml` 取代，留作参考 |
| `nakama-bundle.tar.gz` | 首次部署的打包产物，已过时 |
| `DEPLOYMENT.md` | 首次部署的完整记录与取舍说明（部分内容已过时） |

> 这些密钥**只有本机这一份**。换机器或重装系统前务必先备份。
> 服务器上的 `local.yml` 是同一批密钥的另一份副本（权限 600）。

---

## 5. 日常更新流程

### 5.1 更新服务代码 / Lua 模块 / 模板表（最常见）

这三样现在都在镜像里，更新流程是同一条。**用一键脚本**：

```bat
build\deploy-hjzx-us.bat --version 1.0.2
```

它依次做：构建并推送镜像 → 上传 `docker-compose.yml` 和 `local.yml` →
服务器 `docker compose pull && up -d` → 等健康检查 → 打印版本号和容器内数据数量。

参数：

| 参数 | 作用 |
|---|---|
| `--version 1.0.2` | 指定版本号（透传给 `build-hjzx-us.bat`），同时打 `:1.0.2` 和 `:latest` |
| `--skip-build` | 跳过构建，只上传配置并重启（改了 `local.yml` 时用） |
| `--skip-config` | 不覆盖服务器上的 compose 与 `local.yml`（远端配置动过时用） |

> 版本号只影响镜像内嵌的 version 字段和 `:版本号` 标签；
> **compose 始终用 `:latest`**，所以本地构建后必须推送，服务器才拉得到。

不想用脚本、要手动分两步的话：

```bash
# 1) 本机构建并推送（打 :版本号 与 :latest 两个标签）
build\build-hjzx-us.bat
# 2) 服务器上拉新镜像并重启
ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 \
  'cd /opt/nakama && docker compose pull && docker compose up -d'
```

> 改了 `data/modules/*.lua` 或 `data/Tpl*.json` 也要走这一条 —— 见 5.3。

> ⚠️ `build/*.bat` **只能写 ASCII**。cmd.exe 按系统 ANSI 代码页（中文机上是 GBK）
> 解析批处理，UTF-8 的中文会把脚本字节拆坏，报 `'xx' is not recognized`。
> 批处理里的提示信息一律用英文；要中文提示就写进 `remote-deploy.sh`（在 Linux 上跑，UTF-8 无碍）。

> **历史坑（2026-09-26 已修复）**：在此之前线上长期跑着 `1.0.0dev+6dbeaf62`，
> 比当时 HEAD `c442137b` 落后 2 个提交，因为没人重新构建推送过。
> 排查「代码里明明是新的、线上行为却是旧的」这类问题前，**先确认线上版本**：
>
> ```bash
> ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 \
>   'docker logs nakama 2>&1 | grep -o "\"version\":\"[^\"]*\"" | grep -v PostgreSQL | tail -1'
> ```
>
> 与 `git rev-parse --short HEAD` 对比。当前线上是 `1.0.1+c442137b`。

### 5.2 更新配置 `local.yml`

改本机 `bundle/data/local.yml` → 上传到 `/opt/nakama/local.yml` → 重建容器：

```bash
DEPLOY=~/.nakama-deploy
scp -i $DEPLOY/game-app-key.pem $DEPLOY/bundle/data/local.yml \
  ec2-user@13.221.177.184:/opt/nakama/local.yml
ssh -i $DEPLOY/game-app-key.pem ec2-user@13.221.177.184 \
  'chmod 600 /opt/nakama/local.yml && cd /opt/nakama && docker compose up -d --force-recreate'
```

> 配置不进镜像（里面有数据库口令和密钥），所以改配置**不需要**重新构建镜像。

> `local.yml` 里含数据库口令和全部密钥，改完**不要再提交进任何仓库**。
> 新密钥的值请同步补进 `.nakama-deploy` 对应文件，否则本机与线上就不一致了。

### 5.3 更新 Lua 模块 / 模板表

`data/modules/*.lua` 和 `data/Tpl*.json` 在构建时被打进镜像（`build/Dockerfile`），
**没有**在服务器上挂载。改完之后必须重新构建镜像，走 5.1 的两步。

```bash
# 改完 data/ 下的文件后：
build\build-hjzx-us.bat
ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 \
  'cd /opt/nakama && docker compose pull && docker compose up -d'
```

验证容器里确实是新文件：

```bash
ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 \
  'docker exec nakama ls -l /nakama/data/modules /nakama/data/Tpl*.json'
```

> **重要：模板表更新不等于发镜像就行。**
> `server/game_template.go` 的 `StorageReadTpl` 是 **DB 优先**：`Tpl` 集合里有数据时用
> 数据库里的，只有为空才回退读本地 JSON 文件。所以线上跑过一段时间后，
> 改了 `Tpl*.json` 再发镜像**不会**改变模板表 —— 真正生效的是 DB 里的那份。
> 要更新模板表得往 `Tpl` 集合写数据（控制台或 runtime 接口），或者先清空该集合。
> 空库首次启动会打 5 条 `Error reading storage object`，属正常回退，不是故障。

### 5.4 首次部署 / 更换服务器

```bash
# 本机：把 compose 和配置传上去
DEPLOY=~/.nakama-deploy
scp -i $DEPLOY/game-app-key.pem build/docker-compose.prod.yml \
  ec2-user@13.221.177.184:/opt/nakama/docker-compose.yml
scp -i $DEPLOY/game-app-key.pem $DEPLOY/bundle/data/local.yml \
  ec2-user@13.221.177.184:/opt/nakama/local.yml

# 服务器：先确认 local.yml 是文件而不是目录，再拉镜像启动
ssh -i $DEPLOY/game-app-key.pem ec2-user@13.221.177.184 \
  'chmod 600 /opt/nakama/local.yml && test -f /opt/nakama/local.yml && \
   cd /opt/nakama && docker compose pull && docker compose up -d'
```

### 5.5 部署脚本一览（仓库 `build/`）

| 文件 | 作用 |
|---|---|
| `deploy-hjzx-us.bat` | **一键部署**：构建推送 → 上传配置 → 远端拉取重启 → 验证 |
| `build-hjzx-us.bat` | 只构建并推送镜像（`deploy-hjzx-us.bat` 会调用它） |
| `remote-deploy.sh` | 在 EC2 上执行的远端步骤，被上传到 `/tmp` 后运行 |
| `docker-compose.prod.yml` | 生产 compose，上传后即服务器上的 `/opt/nakama/docker-compose.yml` |

`remote-deploy.sh` 做的事：检查 `local.yml` 是文件（**不是目录**）→ `chmod 600` →
`docker compose pull` → `up -d` → 轮询健康检查（最多 150 秒）→ 打印镜像、版本号、
容器内 Lua 模块与模板表数量。它**没有** `rm` / `down -v` / `prune` 之类的破坏性命令。

`deploy-hjzx-us.bat` 里的路径常量（SSH 密钥、主机、远端目录）都在文件顶部，
换服务器时改那一段即可。

> 服务器上的旧配置已备份为 `/opt/nakama/docker-compose.yml.bak-20260926` 和
> `/opt/nakama/local.yml.bak-20260926`，回滚时覆盖回去再 `up -d` 即可。

### 5.6 健康检查与自动重启

配置在 `build/docker-compose.prod.yml` 的 `healthcheck` 段。

**为什么要自己实现**：Docker 的 `restart` 策略只在容器**进程退出**时触发，
**不会重启只是「不健康」的容器**。所以 compose 里的健康检查脚本自己数失败次数，
到阈值就 `kill 1`，交给 `restart: unless-stopped` 拉起。

| 项 | 值 | 说明 |
|---|---|---|
| 探针 | `nakama healthcheck`（GET 7350 `/` 判 200） | **浅探针** |
| `interval` | 15s | 检查间隔 |
| `timeout` | 15s | 脚本内部另有 `timeout 8`，保证卡死也能计数 |
| `retries` | 3 | Docker 侧标记 unhealthy 的阈值 |
| `start_period` | 90s | 启动宽限期 |
| `HC_FAIL_THRESHOLD` | 3 | 环境变量，连续失败多少次触发重启 |

**探针是浅的，这点必须清楚**：`nakama healthcheck` 的实现是
`http.Get("http://localhost:7350")` 判 200（`main.go:121`），而该路由无条件返回 200
（`server/api.go:275`）。所以它只证明 HTTP 监听还活着，**数据库断了照样报健康**。
RDS 故障既不会触发重启，也探不出来 —— 只能靠客户端报错发现。

两个内部标记文件（容器内 `/tmp`，重建即清空）：

- `/tmp/.hc_fail` —— 连续失败计数
- `/tmp/.hc_ok` —— 「本容器启动后至少健康过一次」的标记

`.hc_ok` 是**冷启动保护**：没有它，迁移耗时较长时会把还没起来的容器杀掉，
形成重启循环。代价是「起来后从未健康过」的进程不会被重启（Docker 原生策略同样不会）。

**看告警日志**（写入 `/proc/1/fd/1`，即主进程 stdout，复用已有的 json-file 轮转）：

```bash
ssh -i ~/.nakama-deploy/game-app-key.pem ec2-user@13.221.177.184 \
  'docker logs nakama 2>&1 | grep HEALTHCHECK'
```

只有**失败和触发重启**时才写日志，健康时不写，所以正常情况下这个文件是空的。

**改阈值**：改 compose 里 `HC_FAIL_THRESHOLD` 的值，重跑 `deploy-hjzx-us.bat --skip-build`。

> ⚠️ **改健康检查脚本时必须注意**：compose 会对整个 YAML 做变量插值，
> 脚本里的 `$` 必须写成 `$$`，否则会被 compose 当成自己的变量吃掉（`$n` 变成空串），
> 脚本静默失效。改完用 `docker compose config` 复核，注意它输出的是**未还原**的形式
> （仍是 `$$`），实际创建容器时才还原成 `$`。

---

## 6. 三种访问方式

### 6.1 SSH（需要出口 IP 被放行）

```bash
ssh -i C:\Users\Administrator\.nakama-deploy\game-app-key.pem ec2-user@13.221.177.184
```

出口 IP 变了会 `Connection timed out`，两种处理：

```bash
# A. 把安全组更新到当前出口 IP（22 端口）
MYIP=$(curl -s ifconfig.me)
aws ec2 revoke-security-group-ingress --region us-east-1 --group-id sg-098d10e23337e0812 \
  --ip-permissions '[{"IpProtocol":"tcp","FromPort":22,"ToPort":22,"IpRanges":[{"CidrIp":"<旧IP>/32"}]}]'
aws ec2 authorize-security-group-ingress --region us-east-1 --group-id sg-098d10e23337e0812 \
  --ip-permissions "[{\"IpProtocol\":\"tcp\",\"FromPort\":22,\"ToPort\":22,\"IpRanges\":[{\"CidrIp\":\"$MYIP/32\"}]}]"
```

### 6.2 SSM Session Manager（不依赖出口 IP，推荐）

已建好 `EC2-SSM-Role` / `EC2-SSM-Profile` 并挂到实例，Agent 状态 `Online`。
**但本机还没装 Session Manager 插件**，装完才能用交互式 shell 和端口转发：

```powershell
# 装插件（需要管理员权限，会弹 UAC）
curl -o $env:TEMP\smp.exe https://s3.amazonaws.com/session-manager-downloads/plugin/latest/windows/SessionManagerPluginSetup.exe
Start-Process -Wait -FilePath $env:TEMP\smp.exe
```

装好后：

```bash
# 免 SSH 跑单条命令（不需要插件，现在就能用）
aws ssm send-command --region us-east-1 --instance-ids i-06c0667ab19d08355 \
  --document-name AWS-RunShellScript --parameters 'commands=["docker ps"]'

# 交互式 shell
aws ssm start-session --region us-east-1 --target i-06c0667ab19d08355

# 把控制台转发到本机 localhost:7351（即可关掉 7351 的公网放行）
aws ssm start-session --region us-east-1 --target i-06c0667ab19d08355 \
  --document-name AWS-StartPortForwardingSession \
  --parameters '{"portNumber":["7351"],"localPortNumber":["7351"]}'
```

### 6.3 控制台

<http://13.221.177.184:7351/> · 用户名 `admin` · 口令见 `.nakama-deploy/console_password.txt`

---

## 7. 风险与注意事项

1. **控制台 7351 已对 `0.0.0.0/0` 开放**（为绕开动态出口 IP）。控制台可管理用户、
   存储、排行榜等全部游戏数据。口令是 24 位随机串，但一旦泄露即等于交出后台。
   若要收回：用 6.2 的 SSM 端口转发访问，然后把 7351 的安全组规则删掉。
2. **SSH 出口 IP 是动态的**，22 端口的 `/32` 规则会反复失效。长期方案是改用 SSM。
3. **RDS 主密码是显式密码，不是 AWS 托管**。文档 `AWS-hhxj-迁移说明.md` 第 4 节写的
   `--manage-master-user-password` 会让 AWS **每 7 天自动轮换**密码，而 Nakama 只在启动时
   读一次 DSN，轮换后必然 `password authentication failed`。要改回托管方案，必须同时给
   Nakama 加运行时重新读取密码的机制。
4. **`--deletion-protection` 已开启**，删库前必须先
   `aws rds modify-db-instance --no-deletion-protection --apply-immediately`。
5. **社交登录密钥**：`local.yml` 里的 `social.wechat` / `social.tiktok` 沿用仓库
   `config-redis-leaderboard.yml` 的取值。若 US 版是另一套微信/TikTok 应用，需替换。
6. **`console.username` 仍是 `admin`**（启动告警项之一）。密码已是强随机串，如需改名
   改 `local.yml` 后 `--force-recreate`。
7. **密钥只有本机一份**，`.nakama-deploy` 目录务必备份。
8. 镜像仓库凭证存在服务器 `/home/ec2-user/.docker/config.json`，**明文存储**。

---

## 8. 成本参考（us-east-1）

| 资源 | 规格 | 约 |
|---|---|---|
| RDS | db.t3.medium + 50GiB gp3，单 AZ | ~$60/月 |
| EC2 | t3.medium + 50GiB gp3，按需 | ~$35/月 |
| 公网 IPv4 | 实例自动分配 | 单独计费 |

EC2 不用时 `stop`（停止后公网 IP 会变）；RDS 不用时 `delete`（需先关删除保护）。
