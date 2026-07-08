# AIOps Monitor

[中文](README.md) | [English](README_EN.md)

> **轻量级主机监控运维平台** —— Go 原生采集 + Python 插件层 + 实时面板 + 阈值告警 + 远程终端 + 自动化剧本

单二进制服务端、零依赖 Agent、三平台原生采集（含 GPU）、一条命令安装、开箱即用。内置交互式趋势图、自定义拨测、远程终端（免开端口）、自动化剧本编排、多用户 RBAC、MFA 两步验证、内嵌持久化、PWA 安装。

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
git clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor
docker compose up -d
# 浏览器打开 http://localhost:8529
```

> 单容器模式：Go 二进制内嵌前端（`//go:embed`），开箱即用。

> **默认凭据**：`admin / admin`。首次登录后请立即修改用户名与密码，并建议启用 MFA。

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
| **交互式趋势图** | 纯 Canvas，悬停十字线 + 数值气泡、框选放大、双击还原、放大预览 |
| **自定义拨测** | HTTP（状态码/延时/TLS 证书天数）/ TCP / Ping（丢包率/RTT）/ 进程存活；历史曲线回看 |
| **远程终端** | 浏览器全 TTY，经 Agent 反向连接（免开端口）；多标签、会话录制回放、只读旁观、命令审计 |
| **自动化剧本** | 多步骤编排 + 按 全部/分类/系统/主机 选目标 → 批量并行执行 → 实时输出 + 历史报告 |
| **告警推送** | 飞书 / 钉钉 Webhook + 邮件 SMTP，触发/恢复各推一次，不刷屏 |
| **多用户 RBAC** | admin / operator / viewer 三角色，路由级权限拦截，用户管理界面 |
| **MFA 两步验证** | TOTP（RFC 6238），Google Authenticator 兼容，扫码入网 |
| **账户找回** | 忘记用户名 / 忘记密码（邮箱验证码）/ 邮箱解除 MFA，防枚举 |
| **多服务端推送** | 单 Agent 同时向多服务端推送，采集一次广播所有，独立鉴权/重试 |
| **网关中继模式** | 内网仅一台联网机器代理所有请求到云端，二进制/上报/终端自动穿透 |
| **机器指纹鉴权** | machine-id + MAC 哈希指纹绑定，Token 轮换不影响已装 Agent |
| **持久化** | 内嵌轻量库（gzip+JSON 落盘），重启不丢历史/日志/会话 |
| **PWA 安装** | 可安装到桌面、Service Worker 离线缓存、独立窗口运行 |
| **gzip 压缩** | API/静态资源自动 gzip，多主机轮询带宽 ~8-10 倍压缩 |
| **一键安装** | 面板生成带 Token 命令，自动下载 + 配置 + 注册开机自启 |

---

## 安装部署指南

### 方式一：Docker 部署

```bash
git clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor
docker compose up -d aiops-server
```

- 服务端数据通过 volume 持久化（`/app/data`），配置文件在 `./server_config.json`
- 默认端口 `8529`，可在 `docker-compose.yml` 中修改映射
- Agent 容器默认不启动，取消注释 `docker-compose.yml` 中 `aiops-agent` 段即可启用
- Docker 镜像支持 `amd64` 和 `arm64` 双架构，`docker pull` 自动匹配

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
| `thresholds.cpu_crit` | float | `90` | CPU 严重阈值（%） |
| `thresholds.mem_warn` | float | `80` | 内存警告阈值（%） |
| `thresholds.mem_crit` | float | `90` | 内存严重阈值（%） |
| `thresholds.disk_warn` | float | `85` | 磁盘警告阈值（%） |
| `thresholds.disk_crit` | float | `95` | 磁盘严重阈值（%） |
| `thresholds.offline_after_sec` | int | `30` | 主机失联判定秒数 |
| `require_token` | bool | `false` | 强制 Agent Token |
| `allow_anonymous_agents` | bool | `false` | 允许无 Token Agent |
| `terminal_disabled` | bool | `false` | 全局禁用远程终端 |
| `install_token` | string | 自动生成 | Agent 安装 Token |
| `trust_proxy` | bool | `false` | 反代后设 `true`：采信 `X-Real-IP` 做限流 |
| `smtp.smtp_enabled` | bool | `false` | 邮件推送开关 |
| `smtp.smtp_host` | string | `""` | SMTP 服务器地址 |
| `smtp.smtp_port` | int | `0` | SMTP 端口（465 隐式 TLS / 587 STARTTLS） |
| `smtp.smtp_username` | string | `""` | 发件邮箱账号 |
| `smtp.smtp_password` | string | `""` | SMTP 授权码/密码（脱敏回显） |
| `smtp.smtp_from_name` | string | `"AIOps Monitor"` | 发件人显示名称 |
| `smtp.smtp_use_tls` | bool | `false` | 启用隐式 TLS（465 选 `true`，587 选 `false`） |

### 服务端命令行参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `-addr` | 监听地址 | `:8529` |
| `-config` | 配置文件路径 | `server_config.json` |
| `-dist` | Agent 下载目录 | 自动探测 `./dist` 或程序所在目录 |

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

## 远程终端

- **多标签**：主机卡片一键打开，可同时开多台主机/多个终端
- **会话录制与回放**：自动录制（带时间戳帧），支持进度条拖拽、倍速播放
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

### 账户找回

- **忘记用户名**：输入绑定邮箱 → 发送用户名通知邮件（防枚举）
- **忘记密码**：输入用户名 → 邮箱收 6 位验证码（10 分钟有效）→ 验证后重置
- **邮箱解除 MFA**：丢失手机时通过绑定邮箱验证码解除 MFA 绑定
- 验证码安全：6 位随机数、10 分钟 TTL、单次使用、60 秒发送间隔限制

### Agent 与数据安全

- **强制 Agent Token**（默认开启）：`register`/`report` 必须携带有效 Token（常数时间比较）
- **请求体上限**：2 MiB，防超大 JSON 内存耗尽
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

- 默认 30 秒未上报即判离线，可在告警设置中调整 `offline_after_sec`
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

## 项目目录结构

```
aiops-monitor/
├── cmd/                    # Go 源代码
│   ├── server/            #   服务端（API + WebSocket + 告警 + 终端）
│   │   └── web/           #     前端静态资源（//go:embed 嵌入）
│   ├── agent/             #   Agent（采集器 + 上报 + 终端 + 插件）
│   └── ...
├── shared/                # 共享类型定义（通信协议层）
├── vendor/                # Go 依赖（go-qrcode）
├── plugins/               # Python 插件（进程监控/服务探活/AI 异常检测）
├── docker/                # Docker 配置
│   ├── Dockerfile         #   多阶段构建（server + agent）
│   └── nginx/             #   nginx 反代配置参考（未来分离时可用）
│       └── nginx-frontend.conf
├── deploy/                # 部署示例
│   └── nginx-aiops.conf   #   Nginx 反向代理示例（SSL/HTTPS）
├── generated/             # 自动生成的文件（.gitignore 忽略内容）
│   ├── reports/           #   检测报告、覆盖率报告
│   ├── logs/              #   运行日志
│   └── test-output/      #   测试输出
├── bin/                   # 预编译二进制（.gitignore 忽略）
├── dist/                  # 交叉编译分发产物（.gitignore 忽略）
├── docker-compose.yml     # 根级 Docker Compose 编排
├── .env.example           # 环境变量模板
├── .dockerignore           # Docker 构建上下文排除项
├── .gitignore
├── go.mod / go.sum
├── Dockerfile             # （已迁移至 docker/）
├── README.md / README_EN.md
├── INSTALL.md
└── config.example.json    # Agent 配置模板
```

> **目录说明**：
> - `docker/` — 所有 Docker 相关配置统一存放，支持独立构建部署
> - `generated/` — 检测报告、日志、测试输出集中管理，已加入 `.gitignore`
> - `cmd/server/web/` — 前端 SPA 源码（原生 JS，未来可迁移至 React/Vue）

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
- **多级降采样**：原始（≈1.5h）/ 1 分钟聚合（48h）/ 5 分钟聚合（7 天）三层
- **内嵌持久化**：gzip+JSON 原子落盘 `aiops.db`，定时保存 + 退出前 flush
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
| **告警与事件** | | |
| GET | `/api/v1/alerts` | 阈值告警 + 自定义监控告警 |
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
| POST | `/api/v1/account/recover-username` | 找回用户名 |
| POST | `/api/v1/account/send-reset-code` | 发送重置验证码 |
| POST | `/api/v1/account/reset-password` | 重置密码 |
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
- [x] 交互式趋势图：悬停十字线 + 框选放大 + 放大预览
- [x] 登录认证 + 安全加固：加盐口令 + 限流 + 强制 Token + 安全头 + 密钥脱敏 + 防克隆
- [x] MFA 两步验证（TOTP）+ 账户找回（邮箱验证码）+ 邮箱解除 MFA
- [x] 邮件告警推送（SMTP）
- [x] 实时面板：概览 + TOP10 + 分类分组/搜索/分页 + 卡片·列表双视图 + 宽屏切换
- [x] 告警推送：飞书 / 钉钉 + 邮件，去重 + 状态转换
- [x] gzip 压缩 + PWA 安装 + 移动端响应式
- [x] 分类多选筛选 + 折叠 + 键盘快捷键
- [x] 远程终端：反向连接 + 全 TTY + 多标签 + 录制回放 + 只读旁观 + 命令审计
- [x] 自动化剧本：多步骤编排 + 批量并行 + 专用执行通道 + 中文乱码三层修复
- [x] 多用户 RBAC：三角色 + 用户管理界面 + 路由级拦截
- [x] 多服务端推送：采集一次广播所有 + 独立鉴权/重试/连接池
- [x] 网关中继模式：自动穿透二进制/上报/终端
- [x] 机器指纹鉴权：Token 轮换不影响已装 Agent
- [x] 一键安装：自动检测架构 + 下载 + 配置 + 开机自启

### 进行中 / 计划中

- [ ] 超大规模（万级）：历史外接 VictoriaMetrics、`/hosts` 服务端分页/增量、保留期可配置
- [ ] 插件增强：每插件独立周期、插件级配置、指标类型（counter/histogram）
- [ ] AIOps 演进层：时序异常检测（Prophet / statsmodels）、告警降噪/关联、根因分析、容量预测
- [ ] 智能运维助手：对接 RAGFlow + Dify + 本地 vLLM

---

## License

MIT
