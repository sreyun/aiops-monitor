# AIOps Monitor

> **轻量级主机监控运维平台** —— Go 原生采集核心 + Python 插件层 + 实时面板 + 阈值告警 + 飞书/钉钉推送
>
> 单二进制服务端、零依赖 Agent、三平台原生采集、一条命令安装、开箱即用。

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
| **Python 插件层** | 子进程 + JSON 契约、并发执行、超时隔离、崩溃跳过；可自定义采集 / 服务探活 / AI 异常检测 |
| **实时 Web 面板** | 概览卡片 + 主机列表（分类分组/筛选）+ 阈值告警 + 插件事件 + 操作日志 + 主机趋势弹窗 |
| **阈值告警** | CPU / 内存 / 磁盘越限 + 主机失联检测，支持自定义阈值，面板可视化配置 |
| **告警推送** | 飞书 / 钉钉机器人 Webhook，仅在触发/恢复时各推一次，不刷屏 |
| **一键安装** | 面板生成带 Token 的安装命令，Agent 二进制 + 插件自动下载，注册开机自启 |
| **主机分类** | Agent 上报分类标签，面板按分组展示，支持面板手动覆盖 |
| **操作日志** | 操作 / 系统 / 插件三类日志统一呈现，方便审计与排查 |
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
│   │   ├── main.go                 # 入口、路由、CORS
│   │   ├── handlers.go             # API 处理器
│   │   ├── store.go                # 内存存储
│   │   ├── alerts.go               # 阈值告警引擎
│   │   ├── auth.go                 # 登录认证 + session
│   │   ├── check.go                # 自定义监控（HTTP/TCP 拨测）
│   │   ├── notify.go               # 飞书/钉钉推送（去重 + 状态转换）
│   │   ├── config.go               # 配置持久化
│   │   ├── install.go              # 一键安装脚本生成
│   │   └── web/                    # 面板前端（编译时 embed）
│   │       ├── index.html
│   │       ├── app.js
│   │       └── style.css
│   └── agent/                      # ★ Go Agent 核心
│       ├── main.go                 # 配置 / flag / 信号
│       ├── collector.go            # Collector 接口
│       ├── collector_linux.go      # Linux 原生采集（/proc + syscall）
│       ├── collector_windows.go    # Windows 原生采集（Win32 API）
│       ├── collector_darwin.go     # macOS 原生采集（sysctl + 系统命令）
│       ├── collector_other.go      # 其他平台桩
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
| `--interval` | 基础指标上报间隔（秒） | `5` |
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

**三平台均零第三方依赖**——Go 核心通过 syscall / 系统命令直接采集，不需要安装 Python 或任何 agent 框架。

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
3. 点 **发送测试** 确认通道连通
4. 点 **保存** —— 保存后会立即把当前未恢复的告警补推一次

默认阈值：CPU/内存 80% 警告、90% 严重；磁盘 85%/95%；失联 30s 判离线。所有阈值可在面板中调整。

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
| GET | `/api/v1/hosts/{id}/metrics` | 单主机基础指标历史序列 |
| POST | `/api/v1/hosts/{id}/category` | 设置主机分类覆盖 |
| DELETE | `/api/v1/hosts/{id}` | 删除主机 |
| **告警与事件** | | |
| GET | `/api/v1/alerts` | 阈值告警 + 自定义监控告警 |
| GET | `/api/v1/events` | 插件事件 |
| GET | `/api/v1/activity` | 操作与系统日志 |
| GET | `/api/v1/summary` | 汇总统计 |
| **自定义监控** | | |
| GET | `/api/v1/checks` | 获取自定义监控列表 |
| POST | `/api/v1/checks` | 添加/更新自定义监控 |
| DELETE | `/api/v1/checks/{id}` | 删除自定义监控 |
| **配置管理** | | |
| GET | `/api/v1/config` | 获取告警配置（脱敏） |
| POST | `/api/v1/config` | 更新告警配置 |
| POST | `/api/v1/config/test` | 发送告警测试消息 |
| **认证与账户** | | |
| POST | `/api/v1/login` | 登录（获取 session cookie） |
| POST | `/api/v1/logout` | 退出登录 |
| GET | `/api/v1/me` | 当前用户信息 |
| POST | `/api/v1/profile` | 更新个人资料 |
| POST | `/api/v1/password` | 修改密码 |
| **安装分发** | | |
| GET | `/api/v1/install/info` | 一键安装信息（URL + Token） |
| POST | `/api/v1/install/reset-token` | 重置安装 Token |
| GET | `/install.sh` / `/install.ps1` | 一键安装脚本 |
| GET | `/uninstall.sh` / `/uninstall.ps1` | 一键卸载脚本 |
| **面板与资源** | | |
| GET | `/` | Web 面板 |
| GET | `/dl/*` | Agent 二进制下载 |

---

## Docker 部署

```bash
# 一键启动服务端
docker compose up -d aiops-server

# 启动服务端 + 本机 Agent
docker compose up -d

# 浏览器打开 http://localhost:8080
```

服务端数据通过 volume 持久化，配置文件挂载在 `./server_config.json`。Agent 容器默认不启动，取消注释 `docker-compose.yml` 中 `aiops-agent` 段即可启用。

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

## 关键设计说明

- **共享代码**：`shared/wire.go` 被 server 与 agent 同时 import——改一处，两端同步，契约不会漂移。
- **双心跳上报**：基础指标（Go 原生，便宜）高频上报；插件（可能较重）按更低频率执行，结果缓存后随基础上报一并发送。事件采用"缓冲队列 + 每次上报清空"语义，发送失败会重新排队。
- **进程级隔离**：插件跑在子进程里，`context` 超时可强杀；一个坏插件不会拖垮采集核心。
- **告警去重**：Notifier 追踪告警状态转换，仅在"新触发"和"恢复"时各推一次，持久告警不刷屏。配置变更后自动重置状态，确保新通道能收到当前告警。
- **内存存储的边界**：当前重启后历史清零，适合演示与中小规模验证；上生产可把 `Store` 换成时序库。

---

## 已实现 vs. 演进路线

**已实现（均已实测）**
- [x] 单 Go module + `shared/` 共享类型
- [x] Go Agent 核心：三平台原生采集（Linux/Windows/macOS）、稳定身份、注册、双心跳上报、断连事件重排队
- [x] 插件运行器：子进程 + JSON 契约、并发执行、超时隔离、崩溃跳过
- [x] Python 插件层 + SDK + 示例（服务探活 / CPU 异常检测 / 进程监控 / psutil 兜底）
- [x] Go 服务端：内存存储、阈值告警、自定义指标与插件事件入库
- [x] 自定义监控：HTTP 网站探测 / TCP 端口拨测，异常自动告警并推送
- [x] 登录认证：用户名 + 密码 + session cookie，默认 admin/admin
- [x] 实时面板：概览卡片 + 主机分类分组 + 阈值告警 + 插件事件 + 操作日志 + 趋势弹窗
- [x] 告警推送：飞书 / 钉钉 Webhook，去重 + 状态转换推送
- [x] 一键安装：Token 模式、面板生成命令、自动下载安装、开机自启
- [x] 主机管理：分类标签、面板手动覆盖、主机删除

**下一步（生产化）**
- [ ] **持久化**：内存 `Store` → 时序库（推荐 VictoriaMetrics）+ 元数据入 PostgreSQL
- [ ] **鉴权多租户**：Agent Token 强制校验、后台 RBAC
- [ ] **告警进阶**：持续 N 分钟才触发、静默/升级、多渠道通知（邮件/Webhook）
- [ ] **历史趋势图**：面板接 `/hosts/{id}/metrics` 画曲线（接口已就绪）
- [ ] **自动化运维**：Agent 侧安全命令通道 + 剧本编排 + 批量执行
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
| 告警推送 | 飞书 / 钉钉 Webhook |

---

## License

MIT

