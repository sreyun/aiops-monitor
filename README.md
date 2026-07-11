<div align="center">

# AIOps Monitor

**企业级主机监控与 SRE 运维平台** —— Go 原生采集 + Python 插件层 + 实时面板 + 阈值告警 + 远程终端 + 自动化剧本 + SRE 中枢（事件/自动修复/SLO/工单）+ 日志采集检索 + AI 巡检诊断

[![Version](https://img.shields.io/badge/Version-v5.5.5-blue)](https://github.com/sreyun/aiops-monitor/releases)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#license)
[![Docker](https://img.shields.io/badge/Docker-multi--arch-blue?logo=docker&logoColor=white)](docker-compose.yml)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

[中文](README.md) · [English](README_EN.md)

</div>

> 单二进制服务端、零第三方依赖 Agent、三平台原生采集（含 GPU）、一条命令安装。内置交互式趋势图、自定义拨测、远程终端（免开端口 + 二次密码认证）、自动化剧本编排、SRE 中枢（事件/自动修复/SLO/工单）、日志采集与全文检索、AI 巡检与事件诊断、多用户 RBAC、MFA 两步验证、PWA 安装、端口转发与 HTTP 代理、i18n 国际化（中/英/繁中）。
>
> **v5.5.0 架构升级**：存储统一为 **PostgreSQL（全部关系数据）+ VictoriaMetrics（全部时序数据）**，内置 `aiops.db` 单文件库已彻底停用；新增配置密钥 **AES-256-GCM 静态加密**、可选 **TLS 加密传输**、首次登录 **强制安全初始化**、跨平台 **开机自启 + 保活**（systemd / launchd / 计划任务）。

## 目录

- [平台与架构支持](#平台与架构支持)
- [快速开始](#快速开始)
- [核心特性](#核心特性)
- [安装部署指南](#安装部署指南)
- [配置参考](#配置参考)
- [监控指标](#监控指标)
- [自定义监控（拨测）](#自定义监控拨测)
- [自动化剧本（Playbook）](#自动化剧本playbook)
- [端口转发与 HTTP 代理](#端口转发与-http-代理)
- [远程终端](#远程终端)
- [插件开发](#插件开发)
- [告警配置](#告警配置)
- [高级功能](#高级功能)
- [安全机制](#安全机制)
- [跨网络部署](#跨网络部署)
- [FAQ / 故障排查](#faq--故障排查)
- [技术栈与架构](#技术栈与架构)
- [性能与规模](#性能与规模)
- [API 参考](#api-参考)
- [路线图](#路线图)
- [License](#license)

---

## 平台与架构支持

| 处理器架构 | Linux | Windows | macOS |
|---|:---:|:---:|:---:|
| **AMD64 / x86_64** | ✅ | ✅ | ✅ Intel Mac |
| **ARM64 / aarch64** | ✅ | — | ✅ Apple Silicon (M1/M2/M3/M4) |

> **Apple Silicon 原生支持**：`GOARCH=arm64` + `GOOS=darwin`，无需 Rosetta 转译。  
> **Intel Mac 原生支持**：`GOARCH=amd64` + `GOOS=darwin`。  
> Docker 镜像已配置 `amd64` + `arm64` 多架构交叉编译，`docker pull` 自动获取匹配架构。

### Agent 交叉编译产物

| 文件名 | 平台 | 架构 |
|---|---|---|
| `aiops-agent-linux-amd64` | Linux | AMD64 |
| `aiops-agent-linux-arm64` | Linux | ARM64 |
| `aiops-agent-darwin-amd64` | macOS | Intel |
| `aiops-agent-darwin-arm64` | macOS | Apple Silicon |
| `aiops-agent.exe` | Windows | AMD64 |

安装脚本自动检测 CPU 架构并下载对应二进制，无需手动选择。

---

## 快速开始

### Docker 一键启动（推荐）

```bash
# 直接拉取预构建镜像启动（无需克隆仓库、无需编译）
curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/main/docker-compose.yml
docker compose up -d
# 浏览器打开 http://localhost:8529
```

> 三容器编排：`aiops-server`（Go 单二进制 + `//go:embed` 内嵌前端）+ `postgres` + `victoriametrics`，compose 一键起全。服务端强制依赖 PG + VM，缺一拒绝启动。
>
> 镜像托管于华为云 SWR（`swr.cn-east-3.myhuaweicloud.com/sreyun/`），每次 Release 自动构建 `linux/amd64` + `linux/arm64` 双架构镜像，`docker pull` 自动匹配。

> **默认凭据**：`admin / admin`。**首次登录会强制弹出「安全初始化」，必须修改用户名 + 密码后方可进入**，建议随后启用 MFA。生产请务必修改 `docker-compose.yml` 中的 `POSTGRES_PASSWORD` / `AIOPS_SECRET_KEY`。

### 二进制直接运行

```bash
# 启动服务端（默认监听 :8529）
./bin/aiops-server

# 启动 Agent（从仓库根目录运行以找到 plugins/）
./bin/aiops-agent --server http://<服务端IP>:8529 --category 生产
```

浏览器打开 `http://localhost:8529`，几秒后即可看到主机卡片与指标。

---

## 核心特性

| 能力 | 说明 |
|---|---|
| **三平台原生采集** | Linux（`/proc` + `syscall`）、Windows（Win32 API）、macOS（`sysctl`），均零第三方依赖 |
| **全面指标** | CPU / 内存 / SWAP / 多磁盘 / 网络收发 / TCP 连接数 / 负载 / 进程数 / 运行时长 / **GPU** |
| **GPU 监控** | NVIDIA（`nvidia-smi`）、AMD（Linux sysfs）、Apple（macOS `ioreg`），best-effort + 缓存 |
| **交互式趋势图** | 纯 Canvas，悬停十字线 + 数值气泡、框选放大、双击还原、放大预览；渐变填充、统一时间跨度控件（1h~30天）、水平图例 |
| **自定义拨测** | HTTP（状态码/延时/TLS 证书天数）/ TCP / Ping（丢包率/RTT）/ 进程存活；历史曲线回看 |
| **远程终端** | 浏览器全 TTY，经 Agent 反向连接（免开端口）；多标签、会话录制回放、只读旁观、命令审计、二次认证 |
| **自动化剧本** | 多步骤编排 + 按 全部/分类/系统/主机 选目标 → 批量并行执行 → 实时输出 + 历史报告 |
| **告警推送** | 飞书 / 钉钉 Webhook + 邮件 SMTP，触发/恢复各推一次，不刷屏 |
| **多用户 RBAC** | admin / operator / viewer 三角色，路由级权限拦截，用户管理界面 |
| **MFA 两步验证** | TOTP（RFC 6238），Google Authenticator 兼容，扫码入网 |
| **账户找回** | 未登录双重验证：邮箱验证码 + 可选 MFA（TOTP 动态口令）→ 找回用户名/重置密码 |
| **多服务端推送** | 单 Agent 同时向多服务端推送，采集一次广播所有，独立鉴权/重试 |
| **网关中继模式** | 内网仅一台联网机器代理所有请求到云端，二进制/上报/终端自动穿透 |
| **机器指纹鉴权** | machine-id + MAC 哈希指纹绑定，Token 轮换不影响已装 Agent |
| **SRE 中枢** | 事件（告警/SLO/手动汇聚 + 时间线）· 告警→剧本闭环自动修复（护栏 + 审批）· SLO/错误预算（长窗口查 VM）· 工单 |
| **日志采集与检索** | Agent `--log-paths` 增量采集 → 服务端按主机/级别/关键字/时间全文检索；自动分级 error/warn/info |
| **AI 巡检与诊断** | 定时健康巡检 + 事件根因研判；接入 AI Provider 时智能体级分析，未配置时启发式兜底；**错误/告警日志纳入分析上下文** |
| **统一存储（PG + VM）** | 关系数据（配置/用户/审计/事件/工单/会话）落 PostgreSQL，时序数据（指标/趋势）落 VictoriaMetrics；内置 aiops.db 已彻底停用，二者缺一拒绝启动 |
| **静态加密与 TLS** | 配置密钥（MFA/SMTP/AI/webhook/relay）AES-256-GCM 落库加密（`AIOPS_SECRET_KEY`）；可选 HTTPS/TLS 加密传输 |
| **PWA 安装** | 可安装到桌面、Service Worker 离线缓存、独立窗口运行 |
| **侧栏实时时钟** | 左侧导航栏底部显示当前日期与精确到秒的实时时间（`YYYY-MM-DD HH:mm:ss`），适配浅色/深色主题，侧栏折叠时竖排保持可见 |
| **gzip 压缩** | API/静态资源自动 gzip，多主机轮询带宽 ~8-10 倍压缩 |
| **端口转发（TCP）** | 经 Agent 隧道将远端主机的 TCP 端口映射到服务端本地端口，支持持久规则 + 启停/编辑/复制 |
| **HTTP 反向代理** | 无状态代理：`/proxy/{hostID}/{port}/{path}` 直通目标主机 Web 服务，支持 WebSocket 升级 |
| **一键安装** | 面板生成带 Token 命令，自动下载 + 配置 + 注册开机自启 |
| **告警阈值分级** | 保守 / 标准 / 宽松三档预设，面板一键切换，适配不同部署场景 |
| **i18n 国际化** | 中文简体 / English / 中文繁体，全链路覆盖前端面板与后端 API |

---

## 安装部署指南

### 方式一：Docker 部署（预构建镜像）

```bash
# 下载 docker-compose.yml
curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/main/docker-compose.yml

# 拉取预构建镜像并启动（无需克隆仓库、无需编译）
docker compose up -d
```

- 镜像托管于华为云 SWR：`swr.cn-east-3.myhuaweicloud.com/sreyun/aiops-server:latest`
- 每次打 tag 推送后 GitHub Actions 自动构建 `linux/amd64` + `linux/arm64` 双架构镜像
- 服务端数据通过 volume 持久化（`/app/data`），配置文件在 `./data/server_config.json`
- 默认端口 `8529`，可在 `docker-compose.yml` 中修改映射
- 默认映射 TCP 转发端口范围 `10100-10300`，`forward_listen` 已通过 `AIOPS_FORWARD_LISTEN` 环境变量设为 `0.0.0.0`
- Agent 容器默认不启动，取消注释 `docker-compose.yml` 中 `aiops-agent` 段即可启用
- 如需本地构建，将 `docker-compose.yml` 中 `image:` 替换为注释的 `build:` 配置后执行 `docker compose up -d --build`

<details>
<summary>Docker 手动构建</summary>

```bash
# 构建服务端镜像
docker build --target server -t aiops-server .

# 构建 Agent 镜像
docker build --target agent -t aiops-agent .

# 运行
docker run -d -p 8529:8529 -v aiops-data:/app/data --name aiops-server aiops-server
```
</details>

### 方式二：一键安装脚本（推荐生产使用）

面板右上角点 **「安装 Agent」** → 选择目标系统 → 复制命令到被监控主机执行：

```bash
# Linux（root/sudo）— 自动检测 amd64/arm64
curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows（管理员 PowerShell）
irm "http://<服务端>:8529/install.ps1?token=<TOKEN>" | iex

# macOS — 自动检测 Intel/Apple Silicon
curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sh
```

命令已内置服务端地址与 Token，自动下载对应架构的 Agent 二进制 + 插件、写好配置、注册开机自启。

### 方式三：二进制直接运行

**启动服务端**：

```bash
./bin/aiops-server                          # 默认监听 :8529
./bin/aiops-server -addr 0.0.0.0:9000       # 指定地址/端口
./bin/aiops-server -config /path/to/config  # 指定配置文件
```

**启动 Agent**（从仓库根目录运行以找到 `plugins/`）：

```bash
# Linux AMD64
./bin/aiops-agent-linux-amd64 --server http://<IP>:8529 --category 生产

# Linux ARM64
./bin/aiops-agent-linux-arm64 --server http://<IP>:8529 --category 生产

# macOS Apple Silicon
./bin/aiops-agent-darwin-arm64 --server http://<IP>:8529 --category 生产

# macOS Intel
./bin/aiops-agent-darwin-amd64 --server http://<IP>:8529 --category 生产

# Windows AMD64
.\bin\aiops-agent.exe --server http://<IP>:8529 --category 生产
```

### 方式四：自行编译

```bash
# 需 Go 1.22+
go build -o bin/aiops-server ./cmd/server
go build -o bin/aiops-agent  ./cmd/agent

# 交叉编译各架构 Agent
GOOS=linux   GOARCH=amd64 go build -o bin/aiops-agent-linux-amd64   ./cmd/agent
GOOS=linux   GOARCH=arm64 go build -o bin/aiops-agent-linux-arm64   ./cmd/agent
GOOS=darwin  GOARCH=amd64 go build -o bin/aiops-agent-darwin-amd64  ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o bin/aiops-agent-darwin-arm64  ./cmd/agent
GOOS=windows GOARCH=amd64 go build -o bin/aiops-agent.exe           ./cmd/agent
```

**Windows 一键构建**（自动注入 Git tag 版本号）：

```powershell
# 自动获取 git describe --tags 并通过 ldflags 注入版本号
powershell -File build.ps1

# 含交叉编译 Linux/macOS 产物
powershell -File build.ps1 -CrossCompile
```

> 也可手动注入版本号：`go build -ldflags "-X main.appVersion=$(git describe --tags)" ./cmd/server ./cmd/agent`

### 开机自启

<details>
<summary>Linux（systemd）</summary>

```bash
cp deploy/aiops-agent.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now aiops-agent
```
</details>

<details>
<summary>Windows（NSSM）</summary>

```powershell
nssm install AIOps-Agent C:\aiops-agent\aiops-agent.exe "--server http://<IP>:8529 --category 生产"
nssm set AIOps-Agent AppDirectory C:\aiops-agent
nssm start AIOps-Agent
```
</details>

<details>
<summary>Windows（任务计划）</summary>

```powershell
schtasks /Create /TN "AIOps-Agent" /TR "C:\aiops-agent\start-agent.bat" /SC ONSTART /RU SYSTEM /RL HIGHEST /F
```
</details>

<details>
<summary>macOS（launchd）</summary>

```bash
cp deploy/com.aiops.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.aiops.agent.plist
```
</details>

详细部署说明（含防火墙配置、升级卸载）见 [INSTALL.md](INSTALL.md)。

---

## 配置参考

### Agent 配置（`config.example.json`）

```json
{
  "server": "http://localhost:8529",
  "servers": [
    {"server": "https://monitor-a:8529", "token": "token-a"},
    {"server": "https://monitor-b:8529", "token": "token-b"}
  ],
  "report_interval": 10,
  "plugin_interval": 15,
  "disk_path": "/",
  "plugins_dir": "plugins",
  "python": "python3",
  "state_file": "agent_state.json",
  "category": "",
  "token": ""
}
```

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `server` | string | `http://localhost:8529` | 单服务端地址（`servers` 为空时回退到此） |
| `servers` | array | `[]` | 多服务端列表，每项含 `server` + `token`；非空时优先使用 |
| `report_interval` | int | `10` | 基础指标上报间隔（秒） |
| `plugin_interval` | int | `15` | 插件执行周期（秒） |
| `disk_path` | string | `/` | 主磁盘路径（概览用，所有本地盘自动识别） |
| `plugins_dir` | string | `plugins` | 插件目录（可用绝对路径） |
| `python` | string | `python3` | Python 解释器（Windows 为 `python`） |
| `state_file` | string | `agent_state.json` | Agent 状态文件（含 host_id） |
| `category` | string | `""` | 主机分类（面板按此分组） |
| `token` | string | `""` | 安装 Token（可选） |

### Agent 命令行参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--server` | 服务端地址 | `http://localhost:8529` |
| `--category` | 主机分类 | 空 |
| `--interval` | 基础指标上报间隔（秒） | `10` |
| `--plugin-interval` | 插件执行周期（秒） | `15` |
| `--plugins-dir` | 插件目录 | `plugins` |
| `--python` | Python 解释器 | `python3`（Win 为 `python`） |
| `--disk-path` | 主磁盘路径 | `/`（Win 为系统盘） |
| `--token` | 安装 Token | 空 |
| `--relay` | 网关中继模式 | `false` |
| `--listen` | Relay 监听地址 | `:8529` |
| `--config` | 配置文件路径 | `config.json` |

> Flag 覆盖配置文件，配置文件覆盖默认值。`servers` 数组非空时优先于 `server` + `token`。

### 服务端配置（`server_config.example.json`）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `alerts_enabled` | bool | `true` | 启用告警推送 |
| `feishu.enabled` | bool | `false` | 飞书推送开关 |
| `feishu.webhook` | string | `""` | 飞书机器人 Webhook |
| `dingtalk.enabled` | bool | `false` | 钉钉推送开关 |
| `dingtalk.webhook` | string | `""` | 钉钉机器人 Webhook |
| `dingtalk.secret` | string | `""` | 钉钉加签 Secret |
| `thresholds.cpu_warn` | float | `80` | CPU 警告阈值（%） |
| `thresholds.cpu_crit` | float | `95` | CPU 严重阈值（%） |
| `thresholds.mem_warn` | float | `85` | 内存警告阈值（%） |
| `thresholds.mem_crit` | float | `95` | 内存严重阈值（%） |
| `thresholds.disk_warn` | float | `80` | 磁盘警告阈值（%） |
| `thresholds.disk_crit` | float | `90` | 磁盘严重阈值（%） |
| `thresholds.diskio_warn` | float | `80` | 磁盘 IO 利用率警告阈值（%） |
| `thresholds.diskio_crit` | float | `95` | 磁盘 IO 利用率严重阈值（%） |
| `thresholds.iops_warn` | float | `50000` | IOPS 警告阈值（总读写 IOPS） |
| `thresholds.iops_crit` | float | `100000` | IOPS 严重阈值 |
| `thresholds.gpu_warn` | float | `80` | GPU 利用率警告阈值（%） |
| `thresholds.gpu_crit` | float | `95` | GPU 利用率严重阈值（%） |
| `thresholds.load_warn` | float | `4.0` | 系统负载警告倍率（× CPU 核心数） |
| `thresholds.load_crit` | float | `8.0` | 系统负载严重倍率（× CPU 核心数） |
| `thresholds.proc_warn` | float | `0.5` | 进程数异常变化比例（50% = 突增/突降一半） |
| `thresholds.offline_after_sec` | int | `60` | 主机失联判定秒数 |
| `require_token` | bool | `false` | 强制 Agent Token |
| `allow_anonymous_agents` | bool | `false` | 允许无 Token Agent |
| `terminal_disabled` | bool | `false` | 全局禁用远程终端 |
| `install_token` | string | 自动生成 | Agent 安装 Token |
| `trust_proxy` | bool | `false` | 反代后设 `true`：采信 `X-Real-IP` 做限流 |
| `forward_disabled` | bool | `false` | 全局禁用端口转发与 HTTP 代理 |
| `forward_listen` | string | `127.0.0.1` | TCP 转发监听地址（Docker 部署需设为 `0.0.0.0`） |
| `forward_port_range` | string | `10100-10300` | TCP 转发端口范围（需与 Docker `ports` 映射一致） |
| `relay_secret` | string | `""` | 中继节点共享密钥（v5.4.1，与 Agent 端 `-relay-secret` 一致） |
| `smtp.smtp_enabled` | bool | `false` | 邮件推送开关 |
| `smtp.smtp_host` | string | `""` | SMTP 服务器地址 |
| `smtp.smtp_port` | int | `0` | SMTP 端口（465 隐式 TLS / 587 STARTTLS） |
| `smtp.smtp_username` | string | `""` | 发件邮箱账号 |
| `smtp.smtp_password` | string | `""` | SMTP 授权码/密码（脱敏回显） |
| `smtp.smtp_from_name` | string | `"AIOps Monitor"` | 发件人显示名称 |
| `smtp.smtp_use_tls` | bool | `false` | 启用隐式 TLS（465 选 `true`，587 选 `false`） |

#### 告警阈值三档预设（v5.4.1）

默认使用「标准」档，用户可根据部署环境通过 `server_config.json` 的 `thresholds` 字段切换：

| 指标 | 保守（敏感） | 标准（推荐·默认） | 宽松（低噪） |
|---|---|---|---|
| CPU 警告/严重 | 70 / 85 | 80 / 95 | 90 / 98 |
| 内存 警告/严重 | 75 / 90 | 85 / 95 | 90 / 98 |
| 磁盘 警告/严重 | 75 / 85 | 80 / 90 | 90 / 97 |
| 磁盘 IO 警告/严重 | 70 / 85 | 80 / 95 | 90 / 98 |
| IOPS 警告/严重 | 20K / 50K | 50K / 100K | 100K / 200K |
| GPU 警告/严重 | 70 / 85 | 80 / 95 | 90 / 98 |
| 负载 警告/严重 | 2.0× / 4.0× | 4.0× / 8.0× | 6.0× / 12.0× |
| 进程数变化 | 0.3 (30%) | 0.5 (50%) | 0.8 (80%) |
| 离线判定 | 30s | 60s | 120s |

> **保守**：生产关键系统，尽早发现异常 → 误报较多。  
> **标准**：多数场景推荐，平衡灵敏度与噪音 → 新安装默认值。  
> **宽松**：开发/测试环境，减少告警疲劳 → 阈值最宽容。

### 服务端命令行参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `-addr` | 监听地址 | `:8529` |
| `-config` | 配置文件路径 | `server_config.json` |
| `-dist` | Agent 下载目录 | 自动探测 `./dist` 或程序所在目录 |

### 环境变量覆盖（v5.4.1）

以下环境变量可覆盖 `server_config.json` 中对应字段，Docker Compose 部署时无需修改配置文件即可调整安全策略：

| 环境变量 | 对应配置项 | 类型 | 说明 |
|---|---|---|---|
| `AIOPS_POSTGRES_DSN` | —（**必填**） | string | PostgreSQL 连接串，如 `postgres://user:pwd@host:5432/db?sslmode=disable`。全部关系数据入 PG；**未配置将拒绝启动** |
| `AIOPS_VM_URL` | —（**必填**） | string | VictoriaMetrics 地址，如 `http://victoriametrics:8428`。全部时序数据入 VM；**未配置将拒绝启动** |
| `AIOPS_SECRET_KEY` | —（强烈建议） | string | 配置密钥落库主密钥：对 MFA/SMTP/AI/webhook/relay 做 AES-256-GCM 静态加密（**务必妥善备份，丢失将无法解密已存密钥**） |
| `AIOPS_TLS_CERT` / `AIOPS_TLS_KEY` | —（可选） | string | TLS 证书 / 私钥路径，配置后以 HTTPS 加密对外服务；否则明文 HTTP（建议置于 TLS 终止代理之后） |
| `AIOPS_FORWARD_LISTEN` | `forward_listen` | string | TCP 转发监听地址（Docker 部署必须设为 `0.0.0.0`） |
| `AIOPS_FORWARD_PORT_RANGE` | `forward_port_range` | string | TCP 转发端口范围，如 `10100-10300` |
| `AIOPS_RELAY_SECRET` | `relay_secret` | string | 中继节点共享密钥 |
| `AIOPS_FORWARD_DISABLED` | `forward_disabled` | bool | 设为 `true` 全局禁用端口转发 |
| `AIOPS_TERMINAL_DISABLED` | `terminal_disabled` | bool | 设为 `true` 全局禁用远程终端 |
| `AIOPS_ALLOW_ANONYMOUS_AGENTS` | `allow_anonymous_agents` | bool | 设为 `true` 允许无 Token Agent |
| `AIOPS_TRUST_PROXY` | `trust_proxy` | bool | 设为 `true` 采信反向代理客户端 IP 头 |
| `AIOPS_REQUIRE_TOKEN` | `require_token` | bool | 设为 `true` 强制 Agent Token 校验 |

> **优先级**：环境变量 > `server_config.json` 文件。布尔类型支持 `true`/`false` 或 `1`/`0`。

---

## 监控指标

| 指标 | Linux | Windows | macOS |
|---|---|---|---|
| CPU 使用率 / 核数 | `/proc/stat` | `GetSystemTimes` | `top -l 2` |
| 内存 / SWAP | `/proc/meminfo` | `GlobalMemoryStatusEx` | `sysctl` + `vm_stat` |
| 磁盘（全部本地盘） | `/proc/mounts` + `statfs` | `GetDiskFreeSpaceExW` | `syscall.Statfs` + `df` |
| 网络收发速率 | `/proc/net/dev` | `GetIfTable` | `netstat -ibn` |
| TCP 连接数 | `/proc/net/tcp` | `GetTcpTable` | `netstat -an` |
| 负载 1/5/15 | `/proc/loadavg` | EWMA 近似 | `sysctl vm.loadavg` |
| 进程数 | `/proc` 枚举 | `EnumProcesses` | `ps -A` |
| 运行时长 | `/proc/uptime` | `GetTickCount64` | `sysctl kern.boottime` |
| **GPU 使用率/显存/温度** | `nvidia-smi` / amdgpu sysfs | `nvidia-smi` | `ioreg`（IOAccelerator） |

**三平台均零第三方依赖**。GPU 为 best-effort：有对应厂商工具或 OS 接口时上报，结果缓存约 12s；无 GPU/无工具时不显示，不影响其他指标。

---

## 自定义监控（拨测）

面板「监控」页可添加主动拨测，定时探测网站、端口、主机连通性、进程存活，异常时自动告警：

| 类型 | 需要填写 | 判定为异常 |
|---|---|---|
| **HTTP 网站** | URL（如 `https://example.com`） | 状态码 ≥ 400，或超时/请求失败 |
| **TCP 端口** | 主机:端口（如 `10.0.0.5:3306`） | 无法建立连接 |
| **Ping 主机** | 主机地址 / IP（如 `8.8.8.8`） | 100% 丢包（不可达） |
| **进程存活** | ① 目标主机 ＋ ② 进程名称 | 目标主机未上报该进程（或离线） |

> 进程监控需先选目标主机再填进程名，因为服务端核对的是该主机 Agent 上报的进程列表。匹配规则为不区分大小写的子串匹配。每项支持列表/胶囊双视图 + 历史曲线回看。

---

## 自动化剧本（Playbook）

面板「自动化」页可编排剧本——一组按顺序在目标主机上批量执行的 shell 命令：

**创建剧本**：填名称 + 若干步骤，每步包含：
- **命令**：一行 shell 命令（Linux `sh -c`、Windows `cmd /c`）
- **目标**：`全部` / `分类:xxx` / `系统:linux|windows|macos` / `主机:<ID>`
- **超时**（秒）与**失败后是否继续**

**执行原理**：命令经 Agent 反向通道下发，以一次性子进程执行、回传输出与退出码。所有匹配在线主机并行执行，每台按步骤顺序运行。执行历史保留最近 100 次。

> 命令为非交互式，不要用 `vim`/`top`/`ssh` 等需交互的程序。每步是独立进程，`cd`/`export` 不跨步骤保留——连续操作写同一步内用 `&&` 串联。

---

## 端口转发与 HTTP 代理

通过 Agent 反向隧道，无需目标主机开放端口即可访问其内部服务。两种模式：

### TCP 端口映射

将远端主机的 TCP 端口持久映射到服务端本地端口，适合数据库、SSH 等长连接协议：

```bash
# 例：将 Agent 所在主机的 MySQL 3306 映射到服务端 13306 端口
# 面板「转发」页创建规则，或 API：
curl -X POST http://<服务端>:8529/api/v1/forward \
  -d '{"host_id":"abc123","target_port":3306,"local_port":13306}'

# 然后用本地客户端直连
mysql -h 127.0.0.1 -P 13306 -u root -p
```

- 支持自动分配端口（`local_port: 0`）或指定端口
- 规则可启用/禁用/编辑/复制/删除
- 监听地址可配置（`forward_listen`），默认 `127.0.0.1`（仅限本机），Docker 部署需设为 `0.0.0.0` 或通过 `AIOPS_FORWARD_LISTEN` 环境变量覆盖
- 端口范围可配置（`forward_port_range`），Docker 部署需与 `ports` 映射一致

### HTTP 反向代理

无状态代理，无需创建规则，直接通过 URL 访问目标主机的 Web 服务：

```bash
# 访问 Agent 主机 abc123 上 8080 端口的 /api/health
curl http://<服务端>:8529/proxy/abc123/8080/api/health

# 支持所有 HTTP 方法 + WebSocket 升级
ws://<服务端>:8529/proxy/abc123/8080/ws
```

- 支持 GET/POST/PUT/DELETE/PATCH 全方法
- 支持 WebSocket 升级（需 Nginx 配置 Upgrade 头）
- 面板可保存常用代理为快捷入口
- `window.open()` 场景使用一次性 proxy_token 鉴权

> 端口转发默认开启，可在告警设置中通过 `forward_disabled: true` 全局关闭。

---

## 远程终端

- **多标签**：主机卡片一键打开，可同时开多台主机/多个终端
- **收起悬浮卡片**：点击「收起」按钮将终端最小化到右下角悬浮卡片，WebSocket 保持连接；支持多窗口并行收起、垂直堆叠；点击卡片即可展开恢复，不影响会话
- **会话录制与回放**：自动录制（带时间戳帧 + 终端尺寸变化），支持进度条拖拽、倍速播放；回放时自动还原录制时的终端尺寸，避免排版错乱
- **只读旁观**：多名管理员可同时旁观活跃会话，用于协作排障
- **命令级审计**：执行的命令自动提取写入操作日志
- **跨平台 TTY**：Windows ConPTY（chcp 65001 + GBK→UTF-8）、Linux/macOS openpty
- **免开端口**：经 Agent 反向连接，被控端无需开放 22/入站端口

> 终端/剧本共用 Agent 反向通道，同一主机同一时刻只服务一个会话。跨外网使用需按 [Nginx 配置](#跨网络部署) 放行 WebSocket。

---

## 插件开发

插件 = 一个可执行脚本，向 stdout 打印 JSON 对象。用 SDK 只需几行：

```python
# plugins/my_check.py
from plugin_sdk import Plugin

p = Plugin()
p.metric("mysql.connections", 42)          # 自定义指标（gauge）
p.metric("mysql.qps", 1350.5)
p.event("warning", "主从延迟 8s")           # 事件（info | warning | critical）
p.emit()                                   # 输出 JSON
```

放进 `plugins/` 目录即自动发现并按 `--plugin-interval` 周期执行。插件崩溃/超时/坏 JSON 只记录跳过，不影响核心。非 `.py` 可执行文件也能作为插件，可用任意语言编写。

---

## 告警配置

告警在面板可视化配置，无需改文件：

1. 面板右上角 **告警设置**
2. 填飞书或钉钉 Webhook（钉钉加签需填 Secret），勾选启用
3. **邮件推送**：展开 SMTP 区域，填服务器/端口/账号/授权码，465 端口选隐式 TLS，587 不选
4. 点 **发送测试** 确认通道连通
5. 点 **保存** — 保存后立即补推当前未恢复告警

| 告警类型 | 触发条件 | 级别 |
|---|---|---|
| CPU / 内存 / 磁盘 | 超过设定阈值 | 警告 / 严重 |
| 主机失联 | 超过设定时长未上报 | 严重 |
| GPU 使用率 | ≥ 80% 警告，≥ 90% 严重 | 警告 / 严重 |
| 系统负载 | 5min 负载 ≥ 核数×2 | 警告 / 严重 |
| HTTP / TCP / Ping / 进程 | 拨测异常 | 自定义 |

> 飞书自定义机器人关键词设为 `AIOps` 或 `告警`。钉钉建议用"加签"安全设置。

---

## 高级功能

### 多服务端推送

单 Agent 实例同时向多个监控服务端推送数据和建立终端通道。**采集只执行一次，结果广播到所有服务端**。

**配置方法**：在 `config.json` 中使用 `servers` 数组（见上方配置参考），或面板「安装 Agent」弹窗勾选「多服务端推送」输入多个地址。

| 维度 | 说明 |
|---|---|
| 采集 | 基础指标 + 插件指标只执行一次，结果广播所有服务端 |
| 上报 | 各服务端并发推送，8s 超时隔离，互不阻塞 |
| 鉴权 | 每个服务端独立校验指纹 |
| 终端通道 | 每个服务端独立长轮询 |
| 事件重试 | 所有服务端都失败才重新排队；至少一个成功即视为已投递 |
| 连接池 | 每个服务端独立 `http.Client` + 连接池 |

> 当 `servers` 非空时优先使用；为空时回退到 `server` + `token`（完全向后兼容）。

### 网关中继模式（Relay）

内网仅一台机器可联网时，在该机器以 Relay 模式安装 Agent：中继服务监听本地端口，将内网 Agent 的所有请求反向代理到云监控中心。

```bash
# ① 网关机器（能联网的机器）
curl -fsSL "https://cloud-server/install-relay.sh?token=TOKEN" | sudo sh

# ② 内网机器（经网关间接上报）
curl -fsSL "http://<网关IP>:8529/install.sh?token=TOKEN" | sudo sh
```

> Relay 与多服务端推送互斥：Relay 是「一台机器代理所有请求到单一上游」，多服务端是「一台机器主动推送到多个上游」。

### 机器指纹鉴权

Agent 注册时将机器指纹（machine-id + 主 MAC 地址的 SHA-256 前 12 位）发送给服务端绑定。后续所有上报与终端通道请求均携带指纹鉴权，**不再依赖安装 Token**——Token 轮换后已装 Agent 无需更新配置。多服务端场景下每个服务端独立校验指纹。

---

## 安全机制

### 登录与认证

- **登录鉴权**：用户名 + 密码（加盐 SHA-256）+ 会话 Cookie；登录框不预填默认 admin
- **MFA 两步验证**：Google Authenticator 兼容 TOTP，启用后需密码 + 6 位动态码
- **登录限流**：按 IP 滑动窗口限制失败次数（默认 5 分钟 8 次）
- **会话安全**：`HttpOnly` + `SameSite=Lax`；HTTPS 下自动加 `Secure`；改密清除所有会话

### 多用户 RBAC

- **admin**：全部权限，含用户管理（创建/编辑/删除/重置密码/解绑 MFA）
- **operator**：除用户管理外的所有操作（终端/剧本/配置/主机删除）
- **viewer**：仅查看；可管理自己的资料/密码/MFA
- 路由级拦截：每个 API 请求经 `authMiddleware` → `routeAllowed` 检查权限

### 账户找回（双重验证）

账户找回流程在**未登录**状态下即可完成，无需事先获得会话令牌，采用双重验证确保安全：

| 步骤 | 说明 |
|---|---|
| **① 邮箱验证码** | 输入已绑定邮箱 → 系统发送 6 位验证码 → 输入验证码进行第一步验证 |
| **② MFA 动态口令**（可选） | 若账户已启用 TOTP 两步验证，则进一步要求输入 Google Authenticator 生成的 6 位动态口令作为第二因素 |
| **③ 获取结果** | 双重验证全部通过后方可显示用户名（找回用户名）或签发一次性重置令牌（重置密码） |

**找回用户名流程**：登录页点击「忘记用户名」→ 输入绑定邮箱 → 收验证码 → 输入验证码 →（若启用 MFA）输入动态口令 → 显示用户名。

**忘记密码流程**：登录页点击「忘记密码」→ 输入绑定邮箱 → 收验证码 → 输入验证码 →（若启用 MFA）输入动态口令 → 设置新密码。重置密码使用一次性令牌（15 分钟有效），无需知晓原密码。

- 验证码安全：6 位随机数、10 分钟 TTL、单次使用、错误 5 次自动作废、60 秒发送间隔限制
- 邮箱解除 MFA：已登录用户丢失手机时，通过绑定邮箱验证码解除 MFA 绑定
- 防枚举：无论邮箱/用户名是否存在，服务端均返回统一响应

### Agent 与数据安全

- **强制 Agent Token**（默认开启）：`register`/`report` 必须携带有效 Token（常数时间比较）
- **请求体上限**：100 MiB（覆盖端口转发文件传输），防超大 JSON 内存耗尽
- **静态加密**：配置中的 MFA/SMTP/AI/webhook/relay 密钥经 `AIOPS_SECRET_KEY` 派生 AES-256-GCM 落库加密
- **加密传输**：可选 TLS（`AIOPS_TLS_CERT/KEY`）；Agent 支持自签 CA 信任（`--ca-cert` / `tls_skip_verify`）
- **首次登录强制安全初始化**：默认 admin/admin 首登强制走「修改用户名 + 密码」弹窗，不可跳过
- **安全响应头**：`nosniff`、`DENY`（防点击劫持）、`no-referrer`
- **密钥脱敏**：Webhook/SMTP 密码掩码回显，空值保持原值
- **主机身份防克隆**：克隆镜像导致 `agent_state.json` 被复制时自动重生 `host_id`
- **远程终端双鉴权**：浏览器需登录会话 + Agent 需 Token；开关闭入审计日志
- **面向公网请置于反向代理之后并启用 HTTPS**

---

## 跨网络部署

### 反向代理 / 域名接入（Nginx）

用域名 + HTTPS 对外时走 Nginx 反代。普通监控走默认 HTTP 代理即可；**远程终端**用到 WebSocket 升级 + 长连接实时流，Nginx 默认不转发 `Upgrade` 头且会缓冲，导致「指标正常、终端连不上」。

```nginx
# http {} 层，全局一次
map $http_upgrade $connection_upgrade { default upgrade; '' close; }

location / {
    proxy_pass http://127.0.0.1:8529;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host  $host;         # 让安装命令自动用域名
    proxy_set_header X-Real-IP         $remote_addr;  # 真实客户端 IP（配合 trust_proxy）

    # —— 远程终端必需（缺一不可）——
    proxy_set_header Upgrade    $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_buffering         off;
    proxy_request_buffering off;
    proxy_read_timeout  3600s;
    proxy_send_timeout  3600s;
}
```

> 完整示例见 [deploy/nginx-aiops.conf](deploy/nginx-aiops.conf)。改完 `nginx -t && nginx -s reload`。  
> 反代后在 `server_config.json` 设 `"trust_proxy": true` 以采信 `X-Real-IP` 做登录限流。  
> 云负载均衡（ALB/CLB/K8s Ingress）同理：需开启 WebSocket 支持、关闭响应缓冲、空闲超时 ≥1h。

### 终端通道穿透

Agent 采用**主动反向连接**：安装时把服务端地址固化到 `--server`。跨网络时 Agent 必须用**公网可达的域名或 IP**。面板「安装 Agent」弹窗的一键命令自动使用当前访问地址作为 Agent 的 `--server`——通过域名访问面板即可，无需手填。

---

## FAQ / 故障排查

<details>
<summary><b>Agent 上报失败</b></summary>

- 检查 `--server` 地址是否正确，确保服务端已启动
- 检查防火墙/安全组是否放行了服务端端口（默认 8529）
- 查看 Agent 日志中的错误信息（`上报失败: ...`）
</details>

<details>
<summary><b>远程终端连不上</b></summary>

- **Nginx 反代时**：必须配置 WebSocket 升级头和关闭缓冲（见上方 Nginx 配置）
- **跨网络时**：安装 Agent 时务必填写公网可达的服务端地址
- 确认服务端未设置 `terminal_disabled: true`
</details>

<details>
<summary><b>终端中文乱码</b></summary>

- Windows ConPTY 已自动 `chcp 65001` + GBK→UTF-8 转换
- 剧本执行有三层编码保障：chcp 65001 + locale 环境变量 + GBK→UTF-8 API 兜底
- Linux/macOS 终端默认 UTF-8，无需额外处理
</details>

<details>
<summary><b>面板显示连接失败</b></summary>

- 检查服务端是否运行：`curl http://localhost:8529/healthz`
- 检查浏览器控制台是否有 CORS 或认证错误
- 尝试强制刷新（Ctrl+Shift+R）
</details>

<details>
<summary><b>主机显示离线</b></summary>

- 默认 60 秒未上报即判离线，可在告警设置中调整 `offline_after_sec`
- 检查 Agent 进程：`ps aux | grep aiops-agent`（Linux）或任务管理器（Windows）
- 检查 Agent 到服务端的网络连通性
</details>

<details>
<summary><b>GPU 信息不显示</b></summary>

- NVIDIA GPU 需安装 `nvidia-smi` 工具
- AMD GPU（Linux）需要 sysfs 权限
- macOS 仅支持 Apple Silicon 的 GPU 监控
- GPU 为 best-effort，无工具时不显示，不影响其他指标
</details>

---

## 技术栈与架构

### 技术选型

| 组件 | 技术 |
|---|---|
| Agent 核心 | Go 1.22+，纯标准库，零第三方依赖 |
| 服务端 | Go 1.22+，`net/http`（Go 1.22 路由），`embed` 内嵌面板 |
| 前端面板 | 原生 HTML/CSS/JS，无框架依赖 |
| 插件层 | Python 3 + psutil（可选） |
| 告警推送 | 飞书/钉钉 Webhook + 邮件 SMTP（`net/smtp` + `crypto/tls`） |
| PWA | manifest.json + Service Worker + icon.svg |

### 架构图

```
                ┌─────────────── Go Agent 核心 ───────────────┐
                │  Collector（三平台原生采集）→ 基础指标          │
                │  PluginRunner → 并发调度 Python 插件           │
                │  Reporter → 广播到所有服务端（独立连接池）      │
  Report ─HTTP─►│  Terminal → 每服务端独立反向通道               │
                │  与后端共享 shared/ 类型                       │
                └──┬──────────────────────────┬─────────────────┘
                   │                          │
              ┌────┴────┐               ┌─────┴─────┐
              │ 服务端 A │               │  服务端 B  │  （多服务端推送）
              └─────────┘               └───────────┘
                                               │ 子进程 + JSON
                    ┌──────────────────────────┼──────────────────────┐
              ┌─────┴───────┐          ┌───────┴───────┐       ┌──────┴───────┐
              │ 自定义采集   │          │ AI / 异常检测  │       │ 进程监控      │
              │ (.py)       │          │ (.py)         │       │ (.py)        │
              └─────────────┘          └───────────────┘       └──────────────┘
```

**分工原则**：高频、通用、对性能敏感的基础采集用 Go（单二进制、无依赖）；多变、需要生态的自定义/AI 逻辑用 Python。进程边界隔离，各自演进。

### 目录结构

```
aiops-monitor/
├── go.mod                          # Go module
├── shared/
│   └── wire.go                     # ★ 共享类型（Agent ↔ Server 契约）
├── cmd/
│   ├── server/                     # Go 服务端
│   │   ├── main.go                 # 入口、路由、中间件
│   │   ├── handlers.go             # API 处理器
│   │   ├── store.go                # 内存存储 + 多级降采样历史
│   │   ├── db.go                   # 内嵌轻量库（gzip+JSON 落盘）
│   │   ├── alerts.go               # 阈值告警引擎
│   │   ├── auth.go                 # 登录认证 + MFA + RBAC
│   │   ├── users.go                # 多用户管理
│   │   ├── check.go                # 自定义监控（HTTP/TCP/Ping/进程）
│   │   ├── ws.go                   # 手写 WebSocket（远程终端）
│   │   ├── terminal.go             # 远程终端中转
│   │   ├── notify.go               # 飞书/钉钉/邮件推送
│   │   ├── email.go                # SMTP + 验证码管理
│   │   ├── playbook.go             # 自动化剧本引擎
│   │   ├── totp.go                 # TOTP 两步验证
│   │   ├── config.go               # 配置持久化
│   │   ├── install.go              # 一键安装脚本生成
│   │   └── web/                    # 面板前端（编译时 embed）
│   │       ├── index.html / app.js / style.css
│   │       ├── manifest.json / sw.js / icon.svg
│   └── agent/                      # ★ Go Agent 核心
│       ├── main.go                 # 配置 / flag / 信号
│       ├── collector.go            # Collector 接口
│       ├── collector_linux.go      # Linux 原生采集
│       ├── collector_windows.go    # Windows 原生采集
│       ├── collector_darwin.go     # macOS 原生采集
│       ├── collector_other.go      # 其他平台桩
│       ├── gpu.go                  # GPU 采集（三平台共用）
│       ├── terminal.go             # 远程终端 Agent 侧
│       ├── pty_windows.go          # Windows ConPTY
│       ├── pty_unix.go             # Linux/macOS openpty
│       ├── pty_linux.go / pty_darwin.go
│       ├── relay.go                # 网关中继模式
│       ├── plugins.go              # 插件运行器
│       ├── identity.go             # 稳定 host_id / 指纹
│       └── reporter.go             # 双心跳上报
├── plugins/                        # ★ Python 插件层
│   ├── plugin_sdk.py               # 插件 SDK
│   ├── core_metrics.py             # psutil 兜底
│   ├── example_service_check.py    # 示例：服务探活
│   ├── example_ai_anomaly.py       # 示例：异常检测
│   ├── process_monitor.py          # 进程监控
│   └── requirements.txt
├── deploy/
│   └── nginx-aiops.conf            # Nginx 反代示例
├── dist/                           # Agent 分发（各平台二进制）
├── bin/                            # 预编译产物
├── config.example.json             # Agent 配置示例
├── server_config.example.json      # 服务端配置示例
├── Dockerfile                      # 多阶段构建
├── docker-compose.yml              # Docker Compose
└── INSTALL.md                      # 详细安装指南
```

### 关键设计

- **共享代码**：`shared/wire.go` 被 server 与 agent 同时 import，契约不会漂移
- **双心跳上报**：基础指标高频上报；插件低频执行，结果随基础上报一并发送
- **进程级隔离**：插件跑在子进程里，超时可强杀，一个坏插件不拖垮核心
- **告警去重**：仅在"新触发"和"恢复"时各推一次，持久告警不刷屏
- **多级降采样**：原始（≈1.5h）/ 1 分钟聚合（48h）/ 5 分钟聚合（30 天）三层
- **统一存储**：关系数据（配置/用户/审计/事件/工单/会话）落 PostgreSQL，时序数据（指标/趋势/SLO）落 VictoriaMetrics；内置 aiops.db 已彻底停用（内存三级窗口仅作热缓存）
- **gzip 压缩**：多主机轮询 JSON 可压 ~8-10 倍，WebSocket 升级自动跳过

---

## 性能与规模

- **带宽**：gzip 压缩 ~8-10 倍，3000 台每 3s 轮询 `/hosts` 下行从 MB/s 降到百 KB/s 级
- **上报吞吐**：3000 台 × 每 10s ≈ 300 次/s，`Upsert` 仅短暂持写锁
- **内存**：每台三层历史 ~1-2 MB，3000 台约需 4-7 GB（可下调保留常量换取更低内存）
- **渲染**：主机列表分页（每页 9），DOM 只渲染当前页
- **调优**：主机多时增大 `--interval`（如 10-15s）降低上报/带宽

> **结论**：gzip + 分页 + 多级降采样 + 持久化使单实例可稳定支撑约 3000 台。万级建议历史外接 VictoriaMetrics 等时序库。

---

## API 参考

<details>
<summary>展开完整 API 列表</summary>

| 方法 | 路径 | 说明 |
|---|---|---|
| **Agent 通信** | | |
| POST | `/api/v1/agent/register` | Agent 注册 |
| POST | `/api/v1/agent/report` | 上报（base + custom + events） |
| **主机管理** | | |
| GET | `/api/v1/hosts` | 主机列表（含最新指标、在线状态） |
| GET | `/api/v1/hosts/meta` | 主机精简元数据 |
| GET | `/api/v1/hosts/{id}/metrics` | 单主机基础指标历史 |
| GET | `/api/v1/hosts/{id}/history` | 单主机时序历史（自动选层） |
| POST | `/api/v1/hosts/{id}/category` | 设置主机分类 |
| DELETE | `/api/v1/hosts/{id}` | 删除主机 |
| **告警管理** | | |
| GET | `/api/v1/alerts` | 阈值告警 + 自定义监控告警 |
| POST | `/api/v1/alerts/ack` | 确认告警 |
| POST | `/api/v1/alerts/silence` | 静默告警 |
| POST | `/api/v1/alerts/clear` | 清除告警 |
| GET | `/api/v1/events` | 插件事件 |
| GET | `/api/v1/activity` | 操作与系统日志 |
| GET | `/api/v1/summary` | 汇总统计 |
| **自定义监控** | | |
| GET | `/api/v1/checks` | 自定义监控列表 |
| POST | `/api/v1/checks` | 添加/更新监控 |
| POST | `/api/v1/checks/{id}/run` | 立即检测 |
| GET | `/api/v1/checks/{id}/history` | 检查历史时序 |
| DELETE | `/api/v1/checks/{id}` | 删除监控 |
| **自动化运维** | | |
| GET | `/api/v1/playbooks` | 剧本列表 |
| POST | `/api/v1/playbooks` | 创建/更新剧本 |
| DELETE | `/api/v1/playbooks/{id}` | 删除剧本 |
| POST | `/api/v1/playbooks/{id}/execute` | 执行剧本 |
| GET | `/api/v1/playbooks/executions` | 执行历史 |
| GET | `/api/v1/playbooks/executions/{id}` | 执行详情 |
| **终端** | | |
| GET | `/api/v1/terminal/sessions` | 活跃会话列表 |
| GET | `/api/v1/terminal/sessions/{id}/replay` | 会话录制回放 |
| GET | `/api/v1/terminal/sessions/{id}/observe` | 只读旁观（WebSocket） |
| GET | `/api/v1/hosts/{id}/terminal` | 浏览器 WebSocket 终端 |
| GET | `/api/v1/agent/terminal/wait` | Agent 长轮询 |
| GET | `/api/v1/agent/terminal/rx` | Server → Agent 帧流 |
| POST | `/api/v1/agent/terminal/tx` | Agent → Server 输出流 |
| **配置管理** | | |
| GET | `/api/v1/config` | 获取告警配置（脱敏） |
| POST | `/api/v1/config` | 更新告警配置 |
| POST | `/api/v1/config/test` | 发送测试消息 |
| **认证与账户** | | |
| POST | `/api/v1/login` | 登录 |
| POST | `/api/v1/logout` | 退出 |
| GET | `/api/v1/me` | 当前用户信息 |
| POST | `/api/v1/profile` | 更新个人资料 |
| POST | `/api/v1/password` | 修改密码 |
| POST | `/api/v1/mfa/setup` | 生成 MFA 密钥 + QR URI |
| POST | `/api/v1/mfa/enable` | 启用 MFA |
| POST | `/api/v1/mfa/disable` | 关闭 MFA |
| POST | `/api/v1/mfa/unbind-via-email` | 邮箱解除 MFA |
| **账户找回** | | |
| POST | `/api/v1/account/recover-send-code` | 发送验证码（邮箱 + 目的：找回用户名/密码） |
| POST | `/api/v1/account/recover-verify` | 验证邮箱验证码（若启用 MFA 返回 `mfa_required`） |
| POST | `/api/v1/account/recover-verify-mfa` | 验证 TOTP 动态口令（MFA 第二因素） |
| POST | `/api/v1/account/recover-username` | [兼容] 找回用户名 |
| POST | `/api/v1/account/send-reset-code` | [兼容] 发送重置验证码 |
| POST | `/api/v1/account/reset-password` | 重置密码（支持 `reset_token` 或 `username+email+code`） |
| **用户管理（admin）** | | |
| GET | `/api/v1/users` | 用户列表 |
| POST | `/api/v1/users` | 创建用户 |
| POST | `/api/v1/users/{username}` | 更新用户 |
| DELETE | `/api/v1/users/{username}` | 删除用户 |
| POST | `/api/v1/users/{username}/reset-password` | 重置密码 |
| POST | `/api/v1/users/{username}/reset-mfa` | 解绑 MFA |
| **安装分发** | | |
| GET | `/api/v1/install/info` | 安装信息 |
| POST | `/api/v1/install/reset-token` | 重置 Token |
| GET | `/install.sh` / `/install.ps1` | 安装脚本 |
| GET | `/uninstall.sh` / `/uninstall.ps1` | 卸载脚本 |
| **端口转发** | | |
| GET | `/api/v1/forward` | 转发规则列表 |
| POST | `/api/v1/forward` | 创建 TCP 转发规则 |
| DELETE | `/api/v1/forward/{id}` | 删除转发规则 |
| PUT | `/api/v1/forward/{id}` | 编辑转发规则 |
| PUT | `/api/v1/forward/{id}/toggle` | 启用/禁用规则 |
| POST | `/api/v1/forward/{id}/copy` | 复制转发规则 |
| GET | `/api/v1/forward/stats` | 转发统计 |
| GET | `/api/v1/forward/health` | 转发健康检查 |
| **HTTP 代理** | | |
| GET | `/api/v1/http-proxy` | 代理快捷入口列表 |
| POST | `/api/v1/http-proxy` | 创建代理快捷入口 |
| DELETE | `/api/v1/http-proxy/{id}` | 删除代理快捷入口 |
| PUT | `/api/v1/http-proxy/{id}` | 编辑代理快捷入口 |
| PUT | `/api/v1/http-proxy/{id}/toggle` | 启用/禁用代理 |
| POST | `/api/v1/http-proxy/{id}/copy` | 复制代理快捷入口 |
| GET | `/api/v1/proxy-token` | 获取一次性代理鉴权 Token |
| GET/POST/PUT/DELETE/PATCH | `/proxy/{hostID}/{port}/{path...}` | HTTP 反向代理（透传到目标主机） |
| **Agent 转发通道** | | |
| GET | `/api/v1/agent/forward/wait` | Agent 长轮询等待转发任务 |
| GET | `/api/v1/agent/forward/rx` | Server → Agent 转发数据流 |
| POST | `/api/v1/agent/forward/tx` | Agent → Server 转发数据流 |
| **实时推送** | | |
| GET | `/ws/push` | WebSocket 实时推送（主机状态/告警） |
| **SRE · 事件** | | |
| GET | `/api/v1/incidents` | 事件列表 |
| POST | `/api/v1/incidents` | 手动创建事件 |
| GET | `/api/v1/incidents/{id}` | 事件详情（含时间线） |
| POST | `/api/v1/incidents/{id}/ack` | 认领事件 |
| POST | `/api/v1/incidents/{id}/resolve` | 解决事件 |
| POST | `/api/v1/incidents/{id}/comment` | 追加评论 |
| POST | `/api/v1/incidents/{id}/ticket` | 升级为工单 |
| POST | `/api/v1/incidents/{id}/diagnose` | AI / 启发式根因诊断 |
| **SRE · 自动修复** | | |
| GET | `/api/v1/remediation/rules` | 修复规则列表 |
| POST | `/api/v1/remediation/rules` | 创建 / 更新规则 |
| DELETE | `/api/v1/remediation/rules/{id}` | 删除规则 |
| GET | `/api/v1/remediation/runs` | 执行记录 |
| POST | `/api/v1/remediation/runs/{id}/approve` | 审批通过并执行 |
| POST | `/api/v1/remediation/runs/{id}/reject` | 驳回待审批修复 |
| **SRE · SLO** | | |
| GET | `/api/v1/slos` | SLO 列表（含 SLI / 错误预算） |
| POST | `/api/v1/slos` | 创建 / 更新 SLO |
| DELETE | `/api/v1/slos/{id}` | 删除 SLO |
| **SRE · 工单** | | |
| GET | `/api/v1/tickets` | 工单列表 |
| POST | `/api/v1/tickets` | 创建工单 |
| GET | `/api/v1/tickets/{id}` | 工单详情 |
| POST | `/api/v1/tickets/{id}` | 更新工单（状态 / 指派等） |
| POST | `/api/v1/tickets/{id}/comment` | 追加评论 |
| DELETE | `/api/v1/tickets/{id}` | 删除工单 |
| **日志聚合** | | |
| POST | `/api/v1/agent/logs` | Agent 日志上报（指纹鉴权） |
| GET | `/api/v1/logs` | 日志检索（`host` / `level` / `q` / `since_min` / `limit`） |
| **AI 巡检与诊断** | | |
| GET | `/api/v1/ai/config` | 获取 AI Provider 配置 |
| POST | `/api/v1/ai/config` | 保存 AI Provider 配置 |
| GET | `/api/v1/ai/inspections` | 巡检报告列表 |
| POST | `/api/v1/ai/inspect` | 立即执行一次巡检 |
| **消息中心** | | |
| GET | `/api/v1/messages` | 消息列表 + 未读数（事件 / AI / 自动修复 / 工单） |
| POST | `/api/v1/messages/read` | 标记指定消息已读 |
| POST | `/api/v1/messages/read-all` | 全部标记已读 |
| **其他** | | |
| GET | `/` | Web 面板 |
| GET | `/healthz` | 健康检查 |
| GET | `/dl/*` | Agent 二进制下载 |

</details>

---

## 路线图

### 已实现

- [x] Go Agent 核心：三平台原生采集 + 稳定身份 + 双心跳上报 + 断连重排队
- [x] GPU 监控：NVIDIA / AMD / Apple，best-effort + 缓存
- [x] Python 插件层 + SDK + 示例（服务探活 / 异常检测 / 进程监控 / psutil 兜底）
- [x] Go 服务端：内存存储 + 多级降采样 + 内嵌持久化（重启不丢）
- [x] 自定义监控：HTTP / TCP / Ping / 进程；列表·胶囊双视图 + 历史曲线
- [x] 交互式趋势图：悬停十字线 + 框选放大 + 放大预览 + 渐变填充 + 30 天历史跨度
- [x] 登录认证 + 安全加固：加盐口令 + 限流 + 强制 Token + 安全头 + 密钥脱敏 + 防克隆
- [x] MFA 两步验证（TOTP）+ 账户找回双重验证（邮箱验证码 + 可选 MFA）+ 邮箱解除 MFA
- [x] 邮件告警推送（SMTP）
- [x] 实时面板：概览 + TOP10 + 分类分组/搜索/分页 + 卡片·列表双视图 + 宽屏切换
- [x] 告警推送：飞书 / 钉钉 + 邮件，去重 + 状态转换 + 服务端防抖（连续 2 次才切换状态）
- [x] gzip 压缩 + PWA 安装 + 移动端响应式 + 版本号自动注入（Git tag → ldflags）
- [x] 分类多选筛选 + 折叠 + 键盘快捷键
- [x] 远程终端：反向连接 + 全 TTY + 多标签 + 收起悬浮卡片 + 录制回放（含终端尺寸还原） + 只读旁观 + 命令审计
- [x] 自动化剧本：多步骤编排 + 批量并行 + 专用执行通道 + 中文乱码三层修复
- [x] 多用户 RBAC：三角色 + 用户管理界面 + 路由级拦截
- [x] 多服务端推送：采集一次广播所有 + 独立鉴权/重试/连接池
- [x] 网关中继模式：自动穿透二进制/上报/终端
- [x] 机器指纹鉴权：Token 轮换不影响已装 Agent
- [x] 一键安装：自动检测架构 + 下载 + 配置 + 开机自启
- [x] 侧栏实时时钟：YYYY-MM-DD HH:mm:ss，每秒更新，适配浅色/深色主题
- [x] 端口转发（TCP 映射）+ HTTP 反向代理：经 Agent 隧道免开端口访问远端服务
- [x] 告警确认/静默/清除 + WebSocket 实时推送
- [x] 全局强制 MFA 策略 + 浅色主题
- [x] 终端文件传输（ZMODEM/lrzsz）+ 终端悬浮卡片最小化
- [x] 资源热力图仪表盘 + 全链路 i18n 国际化（中/英/繁中）
- [x] 终端二次认证：访问终端前需再次验证密码或 MFA 动态口令
- [x] 安全协议确认流程：终端/剧本使用前需阅读并同意安全协议
- [x] 告警阈值三档预设：保守 / 标准 / 宽松，一键切换适配不同场景
- [x] 管理员密码重置 CLI 子命令 + 环境变量覆盖配置（`AIOPS_*`）
- [x] TCP 转发默认监听 127.0.0.1 + 可配置监听地址与端口范围

### 进行中 / 计划中

- [ ] 超大规模（万级）：历史外接 VictoriaMetrics、`/hosts` 服务端分页/增量、保留期可配置
- [ ] 插件增强：每插件独立周期、插件级配置、指标类型（counter/histogram）
- [ ] AIOps 演进层：时序异常检测（Prophet / statsmodels）、告警降噪/关联、根因分析、容量预测
- [ ] 智能运维助手：对接 RAGFlow + Dify + 本地 vLLM

---

## 更新日志

<details>
<summary>v5.3.0 — 终端二次认证 · 安全协议 · 告警阈值分级</summary>

- 终端二次认证：访问终端前需再次验证密码或 MFA 动态口令，提升敏感操作安全性
- 安全协议确认流程：首次使用终端/剧本需阅读并同意安全协议，记录同意时间戳
- 告警阈值三档预设：保守/标准/宽松，面板一键切换，适配不同部署场景
- 管理员密码重置 CLI 子命令（`aiops-server reset-password`）
- 环境变量覆盖配置：`AIOPS_FORWARD_LISTEN`、`AIOPS_TERMINAL_DISABLED` 等 8 个变量
- TCP 转发默认监听地址改为 `127.0.0.1`（提升安全性），Docker 部署通过环境变量设为 `0.0.0.0`
- i18n 国际化完善：补齐 en/zh-TW 缺失翻译，前端字典新增补充
- 多项 UI/UX 修复与样式优化
</details>

<details>
<summary>v5.2.7 — Windows Agent 卸载修复</summary>

- 修复卸载脚本未终止 `wscript.exe` VBS 启动器导致文件删除失败
- 清理 Relay 模式注册表残留（`AIOpsRelay`）
- 文件删除增加重试机制（递增延迟），避免句柄未释放导致静默失败
</details>

<details>
<summary>v5.2.6 — Agent 服务端重启后自动重连</summary>

- 允许已知指纹主机免 Install Token 重注册（服务端 DB 恢复场景）
- 禁用 HTTP/2 避免单连接死亡导致全部请求失败
- 断路器打开时重置注册状态，半开时自动重注册
</details>

<details>
<summary>v5.2.5 — HTTP 代理竞态修复</summary>

- 新增 `pendingSessions` 队列解决 Agent 在 poll 间隙时通知丢失
- 修复 `handleAgentForwardTx/Rx` select 竞态导致最后一帧丢失
- 修复 HTTP 请求 Host 头重复 + 缺少 Content-Length
</details>

<details>
<summary>v5.2.4 — 移动端登录修复</summary>

- 修复移动端登录网络错误 + 表单红框 UI 问题
</details>

<details>
<summary>v5.2.3 — 批量执行与 GPU 面板修复</summary>

- 修复批量剧本执行不稳定问题
- 修复 GPU 面板显示闪烁
</details>

<details>
<summary>v5.2.2 — 外网 Agent 离线修复</summary>

- 修复外网环境下 Agent 频繁离线问题
</details>

<details>
<summary>v5.2.0 — GPU TOP10 + 告警设置重构</summary>

- GPU TOP10 过滤（仅显示有 GPU 硬件的主机）
- 告警设置重构：Tab 切换、自定义 Webhook、阈值扩展、i18n
</details>

<details>
<summary>v5.1.0 — 深度性能/可靠性优化</summary>

- Agent 深度性能优化、可靠性增强、网络优化、安全加固
- 登录红框与 Ping 面板 i18n 修复
</details>

<details>
<summary>v5.0.0 — 主题/图表/统计面板/告警/国际化</summary>

- P0：主题系统 + 图表重绘（Canvas 渐变/十字线/框选放大）
- P1：统计面板（KPI 卡片 + TOP10 横向条形图）
- P2：告警确认/静默 + 告警去重防抖
- P3：TOP10 i18n 中文化 + 视觉优化
</details>

<details>
<summary>v3.10.x — 端口转发/i18n/终端增强</summary>

- TCP 端口映射 + HTTP 反向代理（经 Agent 隧道免开端口）
- 全链路 i18n 国际化（中/英/繁中）
- 终端 ZMODEM 文件传输、悬浮卡片最小化、右键菜单
- 资源热力图仪表盘、全局强制 MFA、浅色主题
- 可配置 TCP 转发监听地址与端口范围
</details>

<details>
<summary>v3.9.x — 终端回放/版本注入/终端 UX</summary>

- 终端录制回放（含终端尺寸还原）
- 版本号自动注入（Git tag → ldflags）
- 终端悬浮卡片最小化 + 主题切换顶栏
</details>

<details>
<summary>v3.8.x — 浅色主题/推送/骨架屏/防闪烁</summary>

- 浅色主题 + 模块拆分 + WebSocket 推送
- 骨架屏加载 + 空状态 + 差分更新防闪烁
- 告警延迟移除宽限期 + 淡出动画
</details>

<details>
<summary>v3.7.x — 全局 MFA / 移动端终端 / UI 打磨</summary>

- 全局强制 MFA 策略（管理员一键开关）
- 移动端终端输入修复 + 深度 UI 审查打磨
</details>

<details>
<summary>v3.6.x — MFA 二维码修复 / Docker 离线化</summary>

- MFA 二维码服务端生成（QR 码格式修复）
- Docker 构建离线化（go mod vendor）
</details>

<details>
<summary>v3.5.x — 全局 MFA / 默认端口变更</summary>

- 全局默认端口 8080 → 8529
- MFA 二维码格式修复 + UI 优化
</details>

<details>
<summary>v3.4.x — 剧本系统类型筛选</summary>

- 剧本目标主机系统类型筛选（Linux/macOS/Windows）
</details>

<details>
<summary>v3.3.x — 网关中继 / 剧本执行通道 / 中文编码</summary>

- Agent 网关中继模式 + 机器指纹鉴权
- 自动化剧本专用一次性执行通道
- 剧本执行中文乱码三层编码修复
</details>

<details>
<summary>v3.2.x — 单进程多服务端推送</summary>

- 单 Agent 同时向多服务端推送，采集一次广播所有
- 独立鉴权/重试/连接池隔离
</details>

<details>
<summary>v3.1.x — 多用户 RBAC / 终端增强</summary>

- 多用户账户与角色权限管理（admin/operator/viewer）
- 远程终端：会话录制回放、多标签、只读旁观、命令审计
- 自动化剧本编排 + 批量并行执行
</details>

<details>
<summary>v3.0.x — 终端增强 / 自动化运维</summary>

- 远程终端增强（录制回放 + 多标签 + 旁观模式）
- 自动化剧本编排（多步骤 + 批量并行 + 执行历史）
</details>

<details>
<summary>v2.x — PWA / 邮件 / 账户找回 / 移动端</summary>

- PWA 安装 + Service Worker 离线缓存
- 邮件 SMTP 推送 + 账户找回双重验证
- MFA 两步验证（TOTP）+ 邮箱解除 MFA
- 深度移动端响应式适配
</details>

<details>
<summary>v1.x — 自定义监控 / 远程终端 / GPU</summary>

- 自定义监控（HTTP/TCP/Ping/进程存活）
- 远程终端（WebSocket + 全 TTY + 跨平台 PTY）
- GPU 监控（NVIDIA/AMD/Apple）
- 多平台采集器增强
</details>

<details>
<summary>v0.x — 初始版本 → 主机监控平台</summary>

- 基础指标采集（CPU/内存/磁盘/网络/负载）
- 阈值告警 + 飞书/钉钉推送
- 内存存储 + 多级降采样 + 内嵌持久化
- 交互式趋势图 + 概览面板
- Docker 部署 + 一键安装脚本
</details>

---

## License

MIT
