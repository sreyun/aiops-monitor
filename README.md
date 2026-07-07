# AIOps Monitor

[中文](README.md) | [English](README_EN.md)

> **轻量级主机监控运维平台** —— Go 原生采集核心 + Python 插件层 + 实时面板 + 阈值告警 + 飞书/钉钉/邮件推送
>
> 单二进制服务端、零依赖 Agent、三平台原生采集（含 **GPU**）、一条命令安装、开箱即用。
>
> 面板内置：**交互式趋势图**（悬停十字线 / 框选放大 / 放大预览）、**自定义拨测**（HTTP / TCP / **Ping** / 进程，含历史曲线回看）、**远程终端**（经 Agent 反连的浏览器全 TTY，免开端口）、**内嵌轻量库持久化**（历史/日志/会话重启不丢）、**gzip 响应压缩**、**PWA 可安装到桌面**、登录鉴权与安全加固（**MFA 两步验证** + **账户找回**）。

---

## 项目简介

AIOps Monitor 是一套面向中小规模场景的**主机监控与运维平台**，采用 **Go + Python 混合架构**：

- **Go Agent 核心**负责高频、贴近系统的基础指标采集——Linux 读 `/proc` + `syscall`、Windows 调 Win32 API、macOS 用 `sysctl` + 系统命令——**三平台均原生零依赖**，同时负责主机注册、双心跳上报、插件调度。
- **Python 插件层**负责自定义采集、业务/中间件探活、异常检测等 AI/自动化逻辑。插件以**子进程 + JSON**方式被 Go 核心调用，天然解耦：插件崩溃/超时只会被记录跳过，绝不影响核心。
- **Go 服务端**单二进制运行，内置实时 Web 面板、阈值告警引擎、飞书/钉钉推送、一键安装脚本分发、主机分类管理。

Agent 与服务端共享 `shared/` 中的类型定义，采集端与服务端的数据契约永不漂移。

---

## 核心特性

| 能力 | 说明 |
|---|---|
| **三平台原生采集** | Linux（`/proc` + `syscall`）、Windows（Win32 API）、macOS（`sysctl` + 系统命令），均零第三方依赖 |
| **全面指标** | CPU / 内存 / SWAP / 多磁盘 / 网络收发速率 / TCP 连接数 / 负载 1·5·15 / 进程数 / 运行时长 |
| **GPU 显卡监控** | 使用率 / 显存 / 温度；NVIDIA（`nvidia-smi`，Linux/Windows）、AMD（Linux sysfs）、Apple/其他（macOS `ioreg`），best-effort 且带缓存 |
| **交互式趋势图** | 纯 Canvas 绘制，悬停十字线 + 数值气泡、拖拽框选放大区间、双击还原、点击放大预览；CPU/内存/磁盘/GPU/网络多图 |
| **自定义拨测** | HTTP 网站（状态码 / 延时 / **TLS 证书剩余天数**）、TCP 端口、**Ping 主机存活（丢包率 / RTT）**、进程存活；**列表 / 胶囊双视图**，每项支持**历史曲线回看** |
| **Python 插件层** | 子进程 + JSON 契约、并发执行、超时隔离、崩溃跳过；可自定义采集 / 服务探活 / AI 异常检测 |
| **实时 Web 面板** | 概览卡片 + 资源 TOP10（CPU/内存/磁盘/GPU）+ 主机列表（分类分组/搜索/筛选/分页）+ 阈值告警 + 操作日志（可选分页 10/30/50/100）+ 标准/宽屏切换 |
| **阈值告警** | CPU / 内存 / 磁盘越限 + 主机失联 + **GPU 过载** + **系统负载过高** 检测，支持自定义阈值，面板可视化配置 |
| **告警推送** | 飞书 / 钉钉机器人 Webhook + **邮件 SMTP 推送**，仅在触发/恢复时各推一次，不刷屏；推送内容含**主机名 / IP / 详细异常 / 时间** |
| **持久化** | 内嵌轻量库（gzip+JSON 落盘 `aiops.db`）—— 历史 / 日志 / 会话重启不丢，无需外部数据库 |
| **远程终端** | 主机卡片一键打开浏览器终端，经 Agent **反向连接**（免在被控端开放 22/入站端口）；完整交互式 TTY（Windows ConPTY、Linux/macOS openpty），支持颜色 / vim·top / 窗口放大·还原·关闭；登录 + Token 双鉴权 + 审计 |
| **分类多选筛选** | 右上角分类下拉支持**多选**，可同时选择多个分类查看；概览页 KPI 卡片、资源 TOP10、告警等**自动联动**筛选 |
| **分类折叠** | 主机列表按分类分组，每组支持**点击收起/展开**，快速聚焦关注分组 |
| **PWA 可安装** | 面板支持 **PWA**——可安装到桌面/主屏幕、独立窗口运行、Service Worker 离线缓存；长按图标快速访问主机/告警/监控 |
| **键盘快捷键** | 数字键 **1–5** 快速切换视图（概览/主机/监控/告警/日志） |
| **一键安装** | 面板生成带 Token 的安装命令，Agent 二进制 + 插件自动下载，注册用户级/系统级开机自启 |
| **安全与性能** | **强制 Agent Token 接入**（默认，常数时间比较）+ 登录限流 + 会话 Cookie（HttpOnly/SameSite/HTTPS 下 Secure）+ 安全响应头 + 请求体大小限制 + 密钥脱敏 + 主机身份防克隆 + **MFA 两步验证（TOTP）** + **账户找回（邮箱验证码）** + **通过邮箱解除 MFA**；**gzip 响应压缩**大幅降低多主机轮询带宽 |
| **共享类型** | `shared/wire.go` 被 server 与 agent 同时 import，契约统一不会漂移 |

---

## 架构

```
                    ┌─────────────── Go Agent 核心（高性能 / 高频） ───────────────┐
                    │  Collector（三平台原生采集）→ 基础指标                         │
   Report           │  PluginRunner → 并发调度 Python 插件、按 JSON 契约合并结果      │
  (base+custom      │  Reporter（双心跳）→ 基础指标高频上报 + 插件低频执行             │
   +events) ─HTTP─► │  与后端共享 shared/ 类型                                       │
                    └───────────────────────────────┬───────────────────────────────┘
                                                     │ 子进程 + JSON（低频）
                          ┌──────────────────────────┼──────────────────────────┐
                    ┌─────┴───────┐          ┌────────┴────────┐         ┌────────┴────────┐
                    │ 自定义采集   │          │  AI / 异常检测   │         │ 进程监控        │
                    │ service_check│          │  ai_anomaly     │         │ process_monitor │
                    │  (.py)      │          │  (.py)          │         │  (.py)          │
                    └─────────────┘          └─────────────────┘         └─────────────────┘
```

**分工原则**：高频、通用、对性能敏感的基础采集用 Go（单二进制、无依赖、可密集轮询）；多变、需要生态和快速迭代的自定义/AI 逻辑用 Python。两者用进程边界隔离，各自演进、互不拖累。

---

## 目录结构

```
aiops-monitor/
├── go.mod                          # Go module: aiops-monitor
├── shared/
│   └── wire.go                     # ★ 共享类型（Metrics/Sample/Event/Report）
├── cmd/
│   ├── server/                     # Go 服务端（纯标准库，单二进制，内置面板）
│   │   ├── main.go                 # 入口、路由、CORS、gzip / 请求体限制中间件
│   │   ├── handlers.go             # API 处理器
│   │   ├── store.go                # 内存存储 + 多级降采样历史
│   │   ├── db.go                   # 内嵌轻量库（gzip+JSON 落盘，自动保存/退出落盘）
│   │   ├── alerts.go               # 阈值告警引擎
│   │   ├── auth.go                 # 登录认证 + session + 登录限流 + MFA(TOTP)
│   │   ├── check.go                # 自定义监控（HTTP / TCP / Ping / 进程 + 历史序列）
│   │   ├── ws.go                   # 手写 WebSocket（远程终端浏览器侧，零依赖）
│   │   ├── terminal.go             # 远程终端中转（Agent 反向通道 + 会话管理）
│   │   ├── notify.go               # 飞书/钉钉/邮件推送（去重 + 状态转换）
│   │   ├── email.go                # SMTP 邮件发送 + 验证码/重置 Token 管理
│   │   ├── totp.go                 # TOTP (RFC 6238) 两步验证
│   │   ├── config.go               # 配置持久化
│   │   ├── install.go              # 一键安装脚本生成
│   │   └── web/                    # 面板前端（编译时 embed）
│   │       ├── index.html
│   │       ├── app.js
│   │       ├── style.css
│   │       ├── manifest.json        # PWA 清单
│   │       ├── sw.js                # Service Worker（离线缓存）
│   │       └── icon.svg             # 应用图标
│   └── agent/                      # ★ Go Agent 核心
│       ├── main.go                 # 配置 / flag / 信号
│       ├── collector.go            # Collector 接口
│       ├── collector_linux.go      # Linux 原生采集（/proc + syscall）
│       ├── collector_windows.go    # Windows 原生采集（Win32 API）
│       ├── collector_darwin.go     # macOS 原生采集（sysctl + 系统命令）
│       ├── collector_other.go      # 其他平台桩
│       ├── gpu.go                  # GPU 采集（nvidia-smi 解析 + 缓存，三平台共用）
│       ├── terminal.go             # 远程终端 Agent 侧（反连通道 + 帧化 rx + shell）
│       ├── pty_windows.go          # Windows 伪终端（ConPTY）
│       ├── pty_unix.go             # Linux/macOS 伪终端（openpty，公共部分）
│       ├── pty_linux.go / pty_darwin.go # 各自 ioctl 打开 pts
│       ├── plugins.go              # 插件运行器（子进程 + JSON，并发+超时）
│       ├── identity.go             # 稳定 host_id / 主机身份
│       └── reporter.go             # 双心跳循环 + 注册 + 上报
├── plugins/                        # ★ Python 插件层
│   ├── plugin_sdk.py               # 极简插件 SDK
│   ├── core_metrics.py             # 基础指标兜底（psutil）
│   ├── example_service_check.py    # 示例：服务探活
│   ├── example_ai_anomaly.py       # 示例：CPU 异常检测（z-score）
│   ├── process_monitor.py          # 进程存活监控
│   ├── process_monitor.json        # 进程监控配置
│   └── requirements.txt            # psutil（可选）
├── dist/                           # Agent 分发目录（各平台二进制 + plugins.zip）
├── bin/                            # 预编译产物
├── config.example.json             # Agent 配置示例
├── server_config.example.json      # 服务端配置示例
├── INSTALL.md                      # 详细安装部署指南
├── Dockerfile                      # 多阶段构建（服务端 + Agent）
└── docker-compose.yml              # Docker Compose 一键部署
```

---

## Docker 部署（推荐）

```bash
# 克隆仓库
git clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor

# 一键启动服务端
docker compose up -d aiops-server

# 浏览器打开 http://localhost:8080
```

> **默认登录凭据**：用户名 `admin` / 密码 `admin`。首次登录后请立即在「个人信息」中修改用户名与密码，并建议启用两步验证（MFA）。

服务端数据通过 volume 持久化（`/app/data`），配置文件在 `./server_config.json`。Agent 容器默认不启动，取消注释 `docker-compose.yml` 中 `aiops-agent` 段即可启用本机 Agent。

---

## 快速开始

### 1. 启动服务端

```bash
# 使用预编译二进制
./bin/aiops-server                     # 默认监听 :8080

# 或自行编译（需 Go 1.22+）
go build -o bin/aiops-server ./cmd/server
./bin/aiops-server

# 指定地址/端口
./bin/aiops-server -addr 0.0.0.0:9000
```

浏览器打开 `http://localhost:8080` 即可看到监控面板。

> **首次登录**：默认账号 **`admin / admin`**。登录后请立即在左下角「个人信息」**修改用户名与密码**，并按需启用**两步验证（TOTP）**。出于防探测考虑，登录框不会预填该默认账号。

### 2. 启动 Agent

**从仓库根目录运行**（这样能找到 `plugins/` 目录）：

```bash
# 插件用到 psutil（可选，基础指标不需要）
pip install -r plugins/requirements.txt

./bin/aiops-agent --server http://<服务端IP>:8080 --category 生产
```

几秒后刷新面板，即可看到主机卡片及各项指标。

### 3. 一键安装（推荐生产使用）

面板右上角点 **「安装 Agent」** → 选择目标系统 → 复制命令到被监控主机执行。命令已内置服务端地址与 Token，会自动下载 Agent + 插件、写好配置、注册开机自启：

```bash
# Linux（root/sudo）
curl -fsSL "http://<服务端>:8080/install.sh?token=<TOKEN>" | sudo sh

# Windows（管理员 PowerShell）
irm "http://<服务端>:8080/install.ps1?token=<TOKEN>" | iex

# macOS
curl -fsSL "http://<服务端>:8080/install.sh?token=<TOKEN>" | sh
```

### 常用参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--server` | 服务端地址 | `http://localhost:8080` |
| `--category` | 主机分类（面板按此分组） | 空 |
| `--interval` | 基础指标上报间隔（秒） | `10` |
| `--plugin-interval` | 插件执行周期（秒） | `15` |
| `--plugins-dir` | 插件目录（可用绝对路径） | `plugins` |
| `--python` | 运行 `.py` 插件的解释器 | `python3`（Win 为 `python`） |
| `--disk-path` | 主磁盘路径（概览用，所有本地盘自动识别） | `/`（Win 为系统盘） |
| `--token` | 安装 Token（可选） | 空 |
| `--config` | 配置文件路径 | `config.json` |

也可用配置文件：`cp config.example.json config.json`，改好后直接运行。

### 自行编译

```bash
go build -o bin/aiops-server ./cmd/server
go build -o bin/aiops-agent  ./cmd/agent

# 交叉编译 Agent
GOOS=windows GOARCH=amd64 go build -o bin/aiops-agent.exe ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o bin/aiops-agent-mac ./cmd/agent
```

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

**三平台均零第三方依赖**——Go 核心通过 syscall / 系统命令直接采集，不需要安装 Python 或任何 agent 框架。

> GPU 为 best-effort：有对应厂商工具（NVIDIA 的 `nvidia-smi`）或 OS 接口时上报，结果缓存约 12s 避免每个上报周期都拉起进程；无 GPU/无工具时该主机不显示 GPU，不影响其它指标。

---

## 自定义监控（拨测）使用方法

除 Agent 自动上报的基础指标外，面板「监控」页可添加**主动拨测**：定时探测网站、端口、主机连通性、进程存活，异常时自动产生告警并按「告警设置」推送。四种类型只需填不同目标：

| 类型 | 需要填写 | 说明 | 判定为异常 |
|---|---|---|---|
| **HTTP 网站** | URL（如 `https://example.com`） | 服务端发起 HTTP(S) 请求，展示状态码 / 响应延时 / HTTPS 证书剩余天数 | 状态码 ≥ 400，或超时/请求失败 |
| **TCP 端口** | 主机:端口（如 `10.0.0.5:3306`） | 服务端尝试建立 TCP 连接，展示连通状态与连接延时 | 无法建立连接 |
| **Ping 主机** | 主机地址 / IP（如 `8.8.8.8`） | 服务端 ICMP ping，展示丢包率与平均 RTT | 100% 丢包（不可达） |
| **进程存活** | **① 目标主机 ＋ ② 进程名称** | 见下方说明 | 目标主机未上报该进程（或主机离线） |

**操作步骤**：面板 →「监控」→「＋ 添加检查」→ 选类型 → 填目标 → 设检测间隔与告警级别 → 保存。每项支持「立即检测 ▶ / 历史曲线（可按 1h/6h/24h/全部 筛选）/ 编辑 / 删除」，并可在**列表 / 胶囊**两种视图间切换。

### 进程存活监控为什么不是只填进程名？

进程监控需要 **①先选「目标主机」＋ ②再填「进程名称」**（如 `nginx`、`mysql`、`aiops-agent`），原因：

- **HTTP / TCP / Ping** 都是**服务端主动去探测目标地址**，跟被监控端无关，所以只填地址即可；
- **进程存活**是**核对某台主机的 Agent 上报的进程列表**里有没有这个进程——服务端并不运行在被监控机上，必须知道「查哪一台机器」。

所以一条进程检查的完整语义是「**主机 A 上的进程 X 是否在运行**」。匹配规则：**不区分大小写的子串匹配**（填 `nginx` 可命中 `nginx.exe` / `nginx: master`）。

> 前提：目标主机需已安装并在线运行 Agent（Agent 周期上报进程名列表）；主机离线或暂无进程数据时，该检查显示异常。

---

## 写一个插件

插件 = 一个可执行脚本，向 **stdout 打印一个 JSON 对象**。用 SDK 只需几行：

```python
# plugins/my_check.py
from plugin_sdk import Plugin

p = Plugin()
p.metric("mysql.connections", 42)          # 自定义指标（gauge）
p.metric("mysql.qps", 1350.5)
p.event("warning", "主从延迟 8s")           # 事件（info | warning | critical）
p.emit()                                   # 输出 JSON
```

放进 `plugins/` 目录即被自动发现并按 `--plugin-interval` 周期执行。JSON 契约：

```json
{
  "metrics": { "自定义指标名": 数值, ... },
  "events":  [ {"level": "warning", "message": "..."} ],
  "base":    { "cpu_percent": ..., ... }
}
```

- `metrics` 的 key 建议自带命名空间（`mysql.`、`nginx.`）避免冲突
- `events` 的 `source` 不填会自动补成插件名
- 插件崩溃/超时/坏 JSON 只会被记录跳过，不影响核心
- 可执行文件（非 `.py`）也能作为插件，即插件可用任意语言编写

**AI / 自动化逻辑就放在这一层**：`example_ai_anomaly.py` 用 z-score 做 CPU 异常检测，真实场景可替换为 Prophet / sklearn，或对接 RAGFlow + Dify + 本地 vLLM 等智能分析平台。

---

## 告警配置

告警在**面板**上可视化配置，无需改文件：

1. 面板右上角点 **告警设置**
2. 填入飞书或钉钉机器人 Webhook 地址（钉钉若开"加签"再填 Secret），勾选启用
3. **邮件推送**：展开「邮件服务（SMTP）」区域，填写 SMTP 服务器地址、端口、发件邮箱账号、授权码/密码、发件人名称，勾选「启用 TLS/SSL」（465 端口选隐式 TLS，587 端口不选），勾选「启用邮件推送」
4. 点 **发送测试** 确认通道连通（同时测试飞书/钉钉/邮件三通道）
5. 点 **保存** —— 保存后会立即把当前未恢复的告警补推一次

> SMTP 授权码/密码与 Webhook Secret 采用相同的脱敏策略：存储明文、回显掩码、提交空值保持原值不变。邮件告警推送到「个人信息」中绑定的邮箱地址。

默认阈值：CPU/内存 80% 警告、90% 严重；磁盘 85%/95%；失联 30s 判离线；GPU 80%/90%；系统负载（5分钟均值 ≥ 核数×2）警告。所有阈值可在面板中调整。

告警类型覆盖范围：
| 告警类型 | 触发条件 | 级别 |
|---|---|---|
| CPU 使用率 | 超过设定阈值 | 警告 / 严重 |
| 内存使用率 | 超过设定阈值 | 警告 / 严重 |
| 磁盘使用率 | 超过设定阈值（支持多分区） | 警告 / 严重 |
| 主机失联 | 超过设定失联时长未上报 | 严重 |
| GPU 使用率 | ≥ 80% 警告，≥ 90% 严重 | 警告 / 严重 |
| 系统负载 | 5min 负载 ≥ 核数×2 | 警告 / 严重 |
| HTTP 拨测 | 状态码 ≥ 400、超时、请求失败 | 自定义 |
| TCP 拨测 | 无法建立连接 | 自定义 |
| Ping 拨测 | 100% 丢包（不可达） | 自定义 |
| 进程存活 | 目标进程未在主机上运行 | 自定义 |

> - 飞书自定义机器人关键词请设为 `AIOps` 或 `告警`
> - 钉钉建议用"加签"安全设置，把 Secret 填进面板即可自动签名

---

## API 参考

| 方法 | 路径 | 说明 |
|---|---|---|
| **Agent 通信** | | |
| POST | `/api/v1/agent/register` | Agent 注册 |
| POST | `/api/v1/agent/report` | 上报（base + custom + events） |
| **主机管理** | | |
| GET | `/api/v1/hosts` | 主机列表（含最新指标、在线状态） |
| GET | `/api/v1/hosts/meta` | 主机精简元数据（id + 主机名，供进程监控选择） |
| GET | `/api/v1/hosts/{id}/metrics` | 单主机基础指标历史序列（近期原始） |
| GET | `/api/v1/hosts/{id}/history?from=&to=` | 单主机时序历史（按跨度自动选原始/1 分钟/5 分钟聚合层） |
| POST | `/api/v1/hosts/{id}/category` | 设置主机分类覆盖 |
| DELETE | `/api/v1/hosts/{id}` | 删除主机 |
| **告警与事件** | | |
| GET | `/api/v1/alerts` | 阈值告警 + 自定义监控告警 |
| GET | `/api/v1/events` | 插件事件 |
| GET | `/api/v1/activity` | 操作与系统日志 |
| GET | `/api/v1/summary` | 汇总统计 |
| **自定义监控** | | |
| GET | `/api/v1/checks` | 获取自定义监控列表（含状态码/延时/证书天数/丢包等运行态） |
| POST | `/api/v1/checks` | 添加/更新自定义监控（type: http / tcp / ping / process） |
| POST | `/api/v1/checks/{id}/run` | 立即触发一次检测 |
| GET | `/api/v1/checks/{id}/history` | 该检查的历史时序（延时/状态/状态码/丢包，用于曲线回看） |
| DELETE | `/api/v1/checks/{id}` | 删除自定义监控 |
| **远程终端** | | |
| GET | `/api/v1/hosts/{id}/terminal` | 浏览器 WebSocket 终端（需登录会话） |
| GET | `/api/v1/agent/terminal/wait` | Agent 长轮询等待会话（Token 鉴权） |
| GET | `/api/v1/agent/terminal/rx` | 服务端 → Agent 键入/尺寸帧流（Token） |
| POST | `/api/v1/agent/terminal/tx` | Agent → 服务端 shell 输出流（Token） |
| **配置管理** | | |
| GET | `/api/v1/config` | 获取告警配置（脱敏） |
| POST | `/api/v1/config` | 更新告警配置 |
| POST | `/api/v1/config/test` | 发送告警测试消息 |
| **认证与账户** | | |
| POST | `/api/v1/login` | 登录（获取 session cookie） |
| POST | `/api/v1/logout` | 退出登录 |
| GET | `/api/v1/me` | 当前用户信息 |
| POST | `/api/v1/profile` | 更新个人资料（含邮箱绑定） |
| POST | `/api/v1/password` | 修改密码 |
| POST | `/api/v1/mfa/setup` | 生成 MFA 密钥 + 二维码 URI |
| POST | `/api/v1/mfa/enable` | 启用两步验证（验证动态码后生效） |
| POST | `/api/v1/mfa/disable` | 关闭两步验证（需密码确认） |
| POST | `/api/v1/mfa/unbind-via-email` | 通过邮箱验证码解除 MFA（防手机丢失锁定） |
| **账户找回** | | |
| POST | `/api/v1/account/recover-username` | 通过绑定邮箱找回用户名（公开端点） |
| POST | `/api/v1/account/send-reset-code` | 发送密码重置验证码到绑定邮箱（公开端点） |
| POST | `/api/v1/account/reset-password` | 验证邮箱验证码后重置密码（公开端点） |
| **安装分发** | | |
| GET | `/api/v1/install/info` | 一键安装信息（URL + Token） |
| POST | `/api/v1/install/reset-token` | 重置安装 Token |
| GET | `/install.sh` / `/install.ps1` | 一键安装脚本 |
| GET | `/uninstall.sh` / `/uninstall.ps1` | 一键卸载脚本 |
| **面板与资源** | | |
| GET | `/` | Web 面板 |
| GET | `/healthz` | 健康检查（服务端内置自监控也用它） |
| GET | `/dl/*` | Agent 二进制下载 |

---

## 服务端配置参数

服务端配置文件 `server_config.json`（与服务端同目录自动生成）支持以下参数：

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `alerts_enabled` | bool | `true` | 是否启用告警推送 |
| `feishu.enabled` | bool | `false` | 飞书推送开关 |
| `feishu.webhook` | string | `""` | 飞书机器人 Webhook 地址 |
| `dingtalk.enabled` | bool | `false` | 钉钉推送开关 |
| `dingtalk.webhook` | string | `""` | 钉钉机器人 Webhook 地址 |
| `dingtalk.secret` | string | `""` | 钉钉加签 Secret |
| `thresholds.cpu_warn` | float | `80` | CPU 警告阈值（%） |
| `thresholds.cpu_crit` | float | `90` | CPU 严重阈值（%） |
| `thresholds.mem_warn` | float | `80` | 内存警告阈值（%） |
| `thresholds.mem_crit` | float | `90` | 内存严重阈值（%） |
| `thresholds.disk_warn` | float | `85` | 磁盘警告阈值（%） |
| `thresholds.disk_crit` | float | `95` | 磁盘严重阈值（%） |
| `thresholds.offline_after_sec` | int | `30` | 主机失联判定秒数 |
| `require_token` | bool | `false` | 是否强制 Agent Token |
| `allow_anonymous_agents` | bool | `false` | 允许无 Token Agent 接入 |
| `terminal_disabled` | bool | `false` | 全局禁用远程终端 |
| `install_token` | string | 自动生成 | Agent 安装 Token |
| `trust_proxy` | bool | `false` | 置于可信反代(Nginx)后设为 `true`：据 `X-Real-IP`/`X-Forwarded-For` 记录真实客户端 IP 并据此做登录限流；直连公网时保持 `false`（这些头可被伪造） |
| `smtp.smtp_enabled` | bool | `false` | 邮件推送开关 |
| `smtp.smtp_host` | string | `""` | SMTP 服务器地址（如 `smtp.gmail.com`） |
| `smtp.smtp_port` | int | `0` | SMTP 端口（465 隐式 TLS / 587 STARTTLS） |
| `smtp.smtp_username` | string | `""` | 发件邮箱账号 |
| `smtp.smtp_password` | string | `""` | SMTP 授权码/密码（脱敏回显，空值保持原值） |
| `smtp.smtp_from_name` | string | `"AIOps Monitor"` | 发件人显示名称 |
| `smtp.smtp_use_tls` | bool | `false` | 启用隐式 TLS（端口 465 选 `true`，587 选 `false`） |

---

## 常见问题

### Agent 上报失败
- 检查 `--server` 地址是否正确，确保服务端已启动
- 检查防火墙/安全组是否放行了服务端端口
- 查看 Agent 日志中的错误信息（`上报失败: ...`）

### 远程终端连不上
- **Nginx 反代时**：必须配置 WebSocket 升级头和关闭缓冲（见上方“反向代理”章节）
- **跨网络时**：安装 Agent 时务必填写公网可达的服务端地址
- 确认服务端未设置 `terminal_disabled: true`

### 面板显示连接失败
- 检查服务端是否正常运行：`curl http://localhost:8080/healthz`
- 检查浏览器控制台是否有 CORS 或认证错误
- 尝试清除浏览器缓存或强制刷新（Ctrl+Shift+R）

### 主机显示离线
- 默认 30 秒未上报即判离线，可在告警设置中调整 `offline_after_sec`
- 检查 Agent 进程是否存活：`ps aux | grep aiops-agent`
- 检查 Agent 到服务端的网络连通性

### GPU 信息不显示
- NVIDIA GPU 需要安装 `nvidia-smi` 工具
- AMD GPU（Linux）需要 sysfs 权限
- macOS 仅支持 Apple Silicon 的 GPU 监控
- GPU 信息为 best-effort，无对应工具时不显示，不影响其他指标

---

## 部署与运维

### 开机自启

**Linux（systemd）**：
```bash
cp deploy/aiops-agent.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now aiops-agent
```

**Windows（NSSM）**：
```powershell
nssm install AIOps-Agent C:\aiops-agent\aiops-agent.exe "--server http://<IP>:8080 --category 生产"
nssm set AIOps-Agent AppDirectory C:\aiops-agent
nssm start AIOps-Agent
```

**Windows（任务计划）**：用 `deploy/start-agent.bat` 包装，`schtasks /Create /TN "AIOps-Agent" /TR "C:\aiops-agent\start-agent.bat" /SC ONSTART /RU SYSTEM /RL HIGHEST /F`

**macOS（launchd）**：
```bash
cp deploy/com.aiops.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.aiops.agent.plist
```

详细部署说明（含防火墙配置、升级卸载、FAQ）见 [INSTALL.md](INSTALL.md)。

---

## 反向代理 / 域名接入（Nginx）

用域名 + HTTPS 对外时通常走 Nginx 反代。**普通监控（指标上报、面板）走普通 HTTP，Nginx 默认就能转发**；但 **远程终端**用到 **WebSocket 升级 + 长连接实时流**，Nginx 默认**不转发 `Upgrade` 头、且会缓冲**，于是会出现「**指标正常、终端连不上**」。

这不是本项目特有——所有 WebSocket 应用（Grafana / Jupyter / code-server 等）在 Nginx 后都要加这几行。服务端已对下行流自动发送 `X-Accel-Buffering: no`（Nginx 见此会对该流关缓冲），所以你要加的很少：

```nginx
# http {} 层，全局一次
map $http_upgrade $connection_upgrade { default upgrade; '' close; }

location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host  $host;         # 让安装命令自动用域名
    proxy_set_header X-Real-IP         $remote_addr;  # 真实客户端 IP（配合 trust_proxy）

    # —— 远程终端必需（缺一不可）——
    proxy_set_header Upgrade    $http_upgrade;         # 转发 WebSocket 升级
    proxy_set_header Connection $connection_upgrade;
    proxy_buffering         off;                       # 关缓冲，实时收发
    proxy_request_buffering off;
    proxy_read_timeout  3600s;                          # 长连接不被切断
    proxy_send_timeout  3600s;
}
```

> 完整可用示例见 **[deploy/nginx-aiops.conf](deploy/nginx-aiops.conf)**。改完 `nginx -t && nginx -s reload`，终端即可跨外网使用。
>
> **说明**：Agent 的 `--server` 地址在安装时由服务端按请求 Host 自动识别（配了 `X-Forwarded-Host` 就是你的域名），**无需手填**——指标能正常上报即代表 Agent 已能通过域名连到服务端，终端连不上纯粹是上面 WebSocket/缓冲的 Nginx 配置问题。
>
> 云负载均衡（ALB/CLB/K8s Ingress）同理：需开启 WebSocket 支持、关闭响应缓冲、把空闲超时调到 ≥1h。
>
> **真实来源 IP**：反代后请在 `server_config.json` 设 `"trust_proxy": true`，服务端才会采信 `X-Real-IP`/`X-Forwarded-For` 记录真实客户端 IP 并据此做登录限流；直连公网时保持默认 `false`（否则这些头可被伪造以绕过限流）。

---

## 关键设计说明

- **共享代码**：`shared/wire.go` 被 server 与 agent 同时 import——改一处，两端同步，契约不会漂移。
- **双心跳上报**：基础指标（Go 原生，便宜）高频上报；插件（可能较重）按更低频率执行，结果缓存后随基础上报一并发送。事件采用"缓冲队列 + 每次上报清空"语义，发送失败会重新排队。
- **进程级隔离**：插件跑在子进程里，`context` 超时可强杀；一个坏插件不会拖垮采集核心。
- **告警去重**：Notifier 追踪告警状态转换，仅在"新触发"和"恢复"时各推一次，持久告警不刷屏。配置变更后自动重置状态，确保新通道能收到当前告警。
- **多级降采样历史**：每台主机保留原始（≈1.5h）/ 1 分钟聚合（48h）/ 5 分钟聚合（7 天）三层；`/history` 按查询跨度自动选层，兼顾细粒度与内存。
- **内嵌轻量库持久化**：`db.go` 将主机历史、日志、会话以 gzip+JSON 原子落盘 `aiops.db`（与配置同目录），定时自动保存、退出前 flush——**重启后历史/日志/登录态都不丢**，无需外部数据库。
- **gzip 响应压缩**：API/静态资源按 `Accept-Encoding` 自动 gzip，多主机轮询下 JSON 通常可压 ~8–10 倍，是大规模部署的首要带宽优化；WebSocket 升级请求自动跳过。

---

## 性能与规模

面向多主机的优化与容量建议：

- **带宽**：服务端对所有 JSON/静态响应做 gzip 压缩（约 8–10 倍）。3000 台、面板每 3s 轮询 `/hosts` 的场景下，这是最关键的一项——压缩后单面板下行通常从 MB/s 级降到百 KB/s 级。
- **上报吞吐**：3000 台 × 每 5s 上报 ≈ 600 次/s 写入，`Upsert` 仅短暂持写锁；1/5 分钟聚合按主机周期性执行，均为常数级开销，采集侧不是瓶颈。
- **内存**：历史保留在内存（并持久化落盘）。每台三层历史合计数千个采样点，粗估每台 ~1–2 MB，**3000 台约需 4–7 GB 内存**，主要由历史层决定。大规模可按需下调 `store.go` 的保留常量（`histRawMax`/`hist1mMax`/`hist5mMax`）换取更低内存，或将历史外接时序库。
- **渲染**：主机列表默认分页（每页 9），DOM 只渲染当前页；概览 TOP、趋势图按需计算，前端在数千主机下仍流畅。
- **调优**：主机很多时可增大 Agent `--interval`（如 10–15s）降低上报/带宽；面板右上角可暂停自动刷新便于排查。

> 结论：**gzip + 分页 + 多级降采样 + 持久化**使单实例可稳定支撑约 3000 台的采集与展示；再往上（万级）建议历史外接 VictoriaMetrics 等时序库，并对 `/hosts` 增加服务端分页/增量下发。

---

## 安全

### 登录与认证

- **登录鉴权**：用户名 + 密码（加盐 SHA-256）+ 会话 Cookie；登录框**不预填默认 admin**（防暴力破解探测）；首次登录请使用部署时设置的管理员账号，登录后请及时修改用户名与密码。
- **用户名可修改**：在「个人信息」弹窗中可修改登录用户名（2–32 位字母/数字/-_.，常数时间比较）。
- **两步验证（MFA / TOTP）**：支持启用 **Google Authenticator** 动态口令作为第二因子。启用后登录需密码 + 6 位 TOTP 验证码；MFA 是否启用的信息仅在密码验证通过后才返回，防止探测。
- **登录限流**：按客户端 IP 滑动窗口限制失败次数（默认 5 分钟 8 次），失败写系统日志，抵御暴力破解。
- **会话 Cookie 安全**：`HttpOnly` + `SameSite=Lax`；经 HTTPS（含反代 `X-Forwarded-Proto`）时自动加 `Secure`；密码修改后清除所有会话。

### 账户找回

- **忘记用户名**：登录页点「忘记用户名」→ 输入绑定的邮箱 → 系统向该邮箱发送用户名通知邮件（防枚举：无论邮箱是否匹配都返回相同成功响应）。
- **忘记密码**：登录页点「忘记密码」→ 输入用户名 → 系统向绑定邮箱发送 6 位验证码（有效期 10 分钟，单次使用）→ 输入验证码 + 新密码完成重置。重置后清除所有会话，旧 Cookie 失效。
- **邮箱验证码安全**：验证码 6 位随机数，有效期 10 分钟，验证后立即删除；同一邮箱 60 秒内最多发送 1 次（防滥用）；用户名比较使用常数时间比较。

### 通过邮箱解除 MFA

- 当用户丢失手机无法生成 TOTP 验证码时，可通过**绑定邮箱**解除 MFA 绑定：
  1. 在「关闭两步验证」弹窗中点「通过邮箱解除」
  2. 系统向绑定邮箱发送 6 位验证码（有效期 10 分钟，单次使用）
  3. 输入正确验证码后关闭 MFA 并记录操作日志
- 若账户未绑定邮箱，提示需先绑定邮箱才能使用此功能。

### Agent 与数据安全

- **请求体上限**：全局 `MaxBytesReader`（2 MiB），防超大 JSON 内存耗尽。
- **安全响应头**：全站 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`（防点击劫持）、`Referrer-Policy: no-referrer`。
- **密钥脱敏**：配置读取接口对 Webhook / 加签 Secret / SMTP 密码掩码回显；提交空值或掩码值则保持原值不变。
- **强制 Agent Token（默认开启）**：`register` 与 `report` **必须携带有效安装 Token 才能接入**（**常数时间比较**），无 Token / 错 Token 一律 `403`。仅在明确设置 `allow_anonymous_agents: true` 时才允许匿名接入（不推荐）。
- **Token 不外泄**：`/install.sh`、`/install.ps1` 为公开端点，但**不再在缺省 token 参数时回填真实 Token**——面板生成的一键命令已带 Token（来自需登录的 `/install/info`），故合法安装照常，而直接 `curl /install.sh` 无法读到 Token。
- **主机身份防克隆**：Agent 身份绑定机器指纹（machine-id + MAC）；克隆母盘/镜像导致 `agent_state.json` 被复制时会被检测并重生 `host_id`，避免不同机器撞同一 ID 造成监控互抢掉线。
- **远程终端**：本质是对被控端的远程命令执行，采用**双重鉴权**——操作侧浏览器 WebSocket 需有效登录会话，Agent 反向通道需安装 Token（常数时间比较）；每次开/关终端写入审计日志；可在服务端配置 `terminal_disabled: true` 一键全局禁用。**强烈建议仅在可信网络启用，并置于 HTTPS 反代之后。**
- **面向公网请置于反向代理之后并启用 HTTPS。**

### PWA 安全

- Service Worker 仅缓存静态资源（HTML/CSS/JS），API 请求始终走网络（实时数据不缓存）；离线时显示面板框架但数据为上次在线快照。

---

## 跨网络部署与远程终端

Agent 采用**主动反向连接**：安装时把服务端地址固化到 `--server`。若被监控主机与服务端**不在同一内网**（走外网/域名接入），必须让 Agent 用**公网可达的域名或 IP**——否则内网 IP 只能内网连通，远程终端也就只在内网可用（这正是"终端只有局域网能连"的根因）。

**安装时指定外网地址**：面板「安装 Agent」弹窗的一键命令自动使用当前访问地址作为 Agent 的 `--server`。跨外网/域名接入时，通过域名访问面板即可——安装命令中的服务端地址会自动推导为当前域名（服务端对该参数做严格白名单校验，防脚本注入）。

**反向代理（nginx / Caddy 等）**：远程终端走 WebSocket + 长连接流式中转，反代必须放行 WebSocket 升级并**关闭缓冲**，否则终端连不上或无输出。nginx 示例：

```nginx
location /api/v1/hosts/ {            # 浏览器 WebSocket
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 3600s;
}
location /api/v1/agent/terminal/ {   # Agent 反向流（必须关闭缓冲）
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
location / {                         # 其余 API / 面板
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
}
```

> 若直接以公网 `IP:端口` 暴露服务端（无反代），Agent 用该地址即可，无需上述配置。升级已装 Agent：在被控端重新执行一次带新地址的安装命令即可覆盖。

---

## 已实现 vs. 演进路线

**已实现（均已实测）**
- [x] 单 Go module + `shared/` 共享类型
- [x] Go Agent 核心：三平台原生采集（Linux/Windows/macOS）、稳定身份、注册、双心跳上报、断连事件重排队
- [x] **GPU 显卡监控**：使用率 / 显存 / 温度（NVIDIA / AMD / Apple，best-effort + 缓存）
- [x] 插件运行器：子进程 + JSON 契约、并发执行、超时隔离、崩溃跳过
- [x] Python 插件层 + SDK + 示例（服务探活 / CPU 异常检测 / 进程监控 / psutil 兜底）
- [x] Go 服务端：内存存储 + **多级降采样历史** + **内嵌轻量库持久化**（重启不丢）
- [x] **自定义监控**：HTTP（状态码/延时/证书天数）/ TCP / **Ping（丢包率/RTT）**/ 进程存活；列表·胶囊双视图；**每项历史曲线回看**
- [x] **交互式趋势图**：悬停十字线 + 数值气泡、框选放大、双击还原、放大预览（CPU/内存/磁盘/GPU/网络）
- [x] 登录认证与安全加固：加盐口令 + 会话 Cookie（HttpOnly/SameSite/Secure）、**登录限流**、**强制 Agent Token 接入（默认，常数时间比较）**、安全响应头、请求体上限、密钥脱敏、**主机身份防克隆**、**登录框无默认 admin**、**用户名可修改**
- [x] **两步验证（MFA / TOTP）**：Google Authenticator 兼容，启用/关闭，二维码扫码入网
- [x] **账户找回**：忘记用户名（邮箱接收）、忘记密码（邮箱验证码重置）、防枚举
- [x] **通过邮箱解除 MFA**：防手机丢失导致账户锁定
- [x] **邮件告警推送（SMTP）**：HTML 邮件模板，支持隐式 TLS / STARTTLS，密码脱敏
- [x] 实时面板：概览卡片 + 资源 TOP10（CPU/内存/磁盘/GPU + **HTTP/TCP/Ping/进程**）+ 主机分类分组/搜索/分页 + **卡片·列表双视图** + 阈值告警 + 操作日志分页 + 标准/宽屏切换
- [x] 告警推送：飞书 / 钉钉 Webhook + **邮件 SMTP**，去重 + 状态转换推送
- [x] **gzip 响应压缩**：多主机轮询带宽 ~8–10 倍压缩
- [x] **PWA 可安装**：manifest + Service Worker + 离线缓存 + 图标 + 快捷入口
- [x] **移动端响应式**：手机竖屏/横屏适配、侧栏抽屉、触摸优化、安全区域适配
- [x] **分类多选筛选 + 折叠**：多选下拉、概览联动、分类收起/展开
- [x] **键盘快捷键**：数字键 1–5 切换视图
- [x] **远程终端**：主机卡片一键打开浏览器终端，经 Agent 反向连接（**免在被控端开放 22/入站端口**）+ 服务端中转；**完整交互式 TTY**（Windows ConPTY、Linux/macOS openpty），支持颜色/行编辑/vim·top 等全屏程序、**窗口放大·还原·关闭**与尺寸自适应；登录会话 + 安装 Token 双鉴权 + 开关闭/审计
- [x] 一键安装：Token 模式、面板生成命令、自动下载安装、开机自启
- [x] 主机管理：分类标签、面板手动覆盖、主机删除

**进行中 / 下一步**
- [ ] **超大规模（万级）**：历史外接时序库（VictoriaMetrics）、`/hosts` 服务端分页/增量、历史保留期可配置化
- [ ] **终端增强**：会话录制/回放、多标签、命令级审计、只读旁观
- [ ] **鉴权多租户**：后台 RBAC、多用户
- [ ] **自动化运维**：剧本编排 + 批量执行
- [ ] **插件增强**：每插件独立周期、插件级配置、指标类型（counter/histogram）

**AIOps 演进层（可作为 Python 插件接入）**
- [ ] 时序异常检测（Prophet / statsmodels）、告警降噪/关联、根因分析、容量预测
- [ ] 智能运维助手 —— 对接 RAGFlow + Dify + 本地 vLLM 知识库栈

---

## 技术栈

| 组件 | 技术 |
|---|---|
| Agent 核心 | Go 1.22+，纯标准库，零第三方依赖 |
| 服务端 | Go 1.22+，`net/http`（Go 1.22 路由），`embed` 内嵌面板 |
| 前端面板 | 原生 HTML/CSS/JS，无框架依赖 |
| 插件层 | Python 3 + psutil（可选） |
| 告警推送 | 飞书 / 钉钉 Webhook + 邮件 SMTP（`net/smtp` + `crypto/tls`，零依赖） |
| PWA | manifest.json + Service Worker + icon.svg |

---

## License

MIT

