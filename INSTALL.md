# AIOps Monitor 安装部署指南

本指南覆盖**服务端**与**不同平台客户端(Agent)**的安装、开机自启与验证。

- **服务端**:1 台,收集数据 + 展示面板 + 发告警,监听 `:8529`。
- **客户端 Agent**:装在每台**被监控主机**上,支持 **Windows / Linux / macOS**。

> **依赖说明**:基础指标(CPU/内存/SWAP/多磁盘/网络/负载/进程数/TCP 连接)由 Go 核心**原生采集,零依赖**,任何平台都不需要装 Python。
> 只有**插件层**(服务探活 `example_service_check`、CPU 异常检测 `example_ai_anomaly`、进程监控 `process_monitor`)需要 **Python 3 + psutil**,可选。

---

## 目录

- [一、部署服务端](#一部署服务端)
- [二、客户端 Agent 通用说明](#二客户端-agent-通用说明)
- [三、Windows 客户端](#三windows-客户端)
- [四、Linux 客户端](#四linux-客户端)
- [五、macOS 客户端](#五macos-客户端)
- [六、验证](#六验证)
- [七、告警推送配置](#七告警推送配置)
- [八、升级与卸载](#八升级与卸载)
- [九、常见问题 FAQ](#九常见问题-faq)

---

## 一、部署服务端

服务端是单个 Go 二进制,内置面板,无外部依赖。`bin/` 下已附预编译产物。

**Linux 服务器(推荐)**
```bash
mkdir -p /opt/aiops-server && cd /opt/aiops-server
cp /path/to/bin/aiops-server .            # 或 go build -o aiops-server ./cmd/server
./aiops-server                             # 默认监听 :8529
# 指定地址/端口: ./aiops-server -addr 0.0.0.0:8529
```

**Windows 服务器**
```powershell
.\bin\aiops-server.exe                     # 默认 :8529
# .\bin\aiops-server.exe -addr 0.0.0.0:9000
```

**放行端口**(否则客户端连不上、浏览器打不开面板):
```bash
# Linux firewalld
firewall-cmd --add-port=8529/tcp --permanent && firewall-cmd --reload
# Linux ufw
ufw allow 8529/tcp
```
```powershell
# Windows 防火墙
New-NetFirewallRule -DisplayName "AIOps Monitor" -Direction Inbound -Protocol TCP -LocalPort 8529 -Action Allow
```

**开机自启(Linux systemd)**:用 [`deploy/aiops-server.service`](deploy/aiops-server.service),改好路径后:
```bash
cp deploy/aiops-server.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now aiops-server
```

浏览器打开 `http://<服务端IP>:8529` 即为面板。服务端配置(告警 webhook / 阈值 / 分类覆盖)持久化在其工作目录的 `server_config.json`。

---

## 二、客户端 Agent 通用说明

> **✅ 推荐：一条命令安装（Token 模式）**
>
> 打开监控面板右上角 **「安装 Agent」** → 选择目标系统 → 复制其中一条命令到被监控主机执行即可。命令已内置**服务端地址**与 **Token**，会自动下载 Agent + 插件、写好配置、注册开机自启并上线：
>
> - **Linux**（root/sudo）：`curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sudo sh`
> - **macOS**：`curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sh`
> - **Windows**（管理员 PowerShell）：`irm "http://<服务端>:8529/install.ps1?token=<TOKEN>" | iex`
>
> 弹窗里填「主机分类」可让新机自动归入对应分组；点「重置」可轮换 Token（旧命令随即失效）。
> 服务端用 `-dist ./dist`（默认值）指向存放各平台 Agent 二进制与 `plugins.zip` 的目录，仓库 `dist/` 已备好。
> 需要强制校验时，把 `server_config.json` 的 `require_token` 设为 `true`——此后未携带正确 Token 的上报会被拒绝。
>
> 下面的**手动安装**适用于内网隔离、需自定义路径或不便联网下载的场景。

**分发内容**:每台被监控主机需要两样东西,放在同一目录:

```
aiops-agent(.exe / -mac)     # 对应平台的 Agent 二进制
plugins/                     # 插件目录(整个拷过去)
```

**Agent 从"包含 plugins/ 的目录"运行**,或用 `--plugins-dir` 指定 plugins 的**绝对路径**(服务化时推荐,避免依赖工作目录)。

**常用参数**:

| 参数 | 说明 | 默认 |
|---|---|---|
| `--server` | 服务端地址,如 `http://10.0.0.5:8529` | `http://localhost:8529` |
| `--category` | **主机分类**,面板按此分组/筛选,如 `生产`、`DB`、`办公终端` | 空(未分类) |
| `--interval` | 基础指标上报间隔(秒) | `5` |
| `--plugin-interval` | 插件执行周期(秒) | `15` |
| `--plugins-dir` | 插件目录(可用绝对路径) | `plugins` |
| `--python` | 运行 `.py` 插件的解释器 | `python3`(Win 为 `python`) |
| `--disk-path` | 主磁盘路径(概览用;所有本地盘会自动全部识别) | `/`(Win 为系统盘) |
| `--config` | 配置文件路径(见 `config.example.json`) | `config.json` |

也可以用配置文件代替命令行:`cp config.example.json config.json`,改好后直接运行 `aiops-agent`。

**Python 插件依赖(可选)**:想用服务探活 / 异常检测 / 进程监控插件时:
```bash
pip install -r plugins/requirements.txt      # 即 psutil
```
不装也能跑——基础指标照常原生采集,只是这几个插件会静默跳过。

**进程监控**:编辑 `plugins/process_monitor.json`,把要盯的进程名填进 `processes`(子串匹配),进程消失即产 critical 事件:
```json
{ "processes": ["nginx", "mysqld", "redis-server", "java"] }
```

---

## 三、Windows 客户端

假设放在 `C:\aiops-agent\`(内含 `aiops-agent.exe` 和 `plugins\`)。

**① 手动前台运行(先验证连通)**
```powershell
cd C:\aiops-agent
.\aiops-agent.exe --server http://<服务端IP>:8529 --category 生产
```

**② 开机自启 —— 方式 A:NSSM(推荐,带自动重启)**

下载 [NSSM](https://nssm.cc/),然后:
```powershell
nssm install AIOps-Agent C:\aiops-agent\aiops-agent.exe "--server http://<服务端IP>:8529 --category 生产"
nssm set AIOps-Agent AppDirectory C:\aiops-agent     # 关键:设工作目录,才能找到 plugins\
nssm start AIOps-Agent
```

**② 开机自启 —— 方式 B:任务计划程序(原生,无第三方)**

用仓库自带的包装脚本 [`deploy/start-agent.bat`](deploy/start-agent.bat)(它会先 `cd` 到自身目录,再启动 Agent),放到 `C:\aiops-agent\`,改好里面的服务端 IP 与分类,然后:
```powershell
schtasks /Create /TN "AIOps-Agent" /TR "C:\aiops-agent\start-agent.bat" /SC ONSTART /RU SYSTEM /RL HIGHEST /F
schtasks /Run /TN "AIOps-Agent"                      # 立即启动一次
```
> 直接用 `schtasks` 指向 exe 会因工作目录是 `System32` 而找不到 `plugins\`,所以务必通过 `start-agent.bat`(或改用 NSSM 的 `AppDirectory`)。

**Python(可选)**:装 [Python 3](https://www.python.org/) 并勾选加入 PATH,然后 `pip install psutil`。Agent 会自动用 `python` 运行 `.py` 插件。

---

## 四、Linux 客户端

假设放在 `/opt/aiops-agent/`(内含 `aiops-agent` 和 `plugins/`)。

**① 手动前台运行**
```bash
cd /opt/aiops-agent
chmod +x aiops-agent
./aiops-agent --server http://<服务端IP>:8529 --category 生产
```

**② 开机自启(systemd,推荐)**

用仓库自带的 [`deploy/aiops-agent.service`](deploy/aiops-agent.service),改好 `WorkingDirectory`、服务端 IP、分类:
```bash
cp deploy/aiops-agent.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now aiops-agent
systemctl status aiops-agent          # 查看状态
journalctl -u aiops-agent -f          # 跟踪日志
```
`WorkingDirectory` 指向含 `plugins/` 的目录即可;崩溃自动重启(`Restart=always`)。

**Python(可选)**:`sudo apt install python3-psutil` 或 `pip3 install psutil`。

---

## 五、macOS 客户端

假设放在 `/usr/local/aiops-agent/`(内含 `aiops-agent-mac` 和 `plugins/`)。macOS 基础指标由零依赖采集器(`sysctl` + 系统命令)提供。

**① 手动前台运行**
```bash
cd /usr/local/aiops-agent
chmod +x aiops-agent-mac
./aiops-agent-mac --server http://<服务端IP>:8529 --category 办公终端
```
> 若提示"无法验证开发者",在 `系统设置 → 隐私与安全性` 里点"仍要打开",或执行 `xattr -d com.apple.quarantine ./aiops-agent-mac`。

**② 开机自启(launchd)**

用仓库自带的 [`deploy/com.aiops.agent.plist`](deploy/com.aiops.agent.plist),改好路径 / 服务端 IP / 分类:
```bash
cp deploy/com.aiops.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.aiops.agent.plist
launchctl start com.aiops.agent
```
(需开机即启、无需登录的系统级守护,放 `/Library/LaunchDaemons/` 并用 `sudo launchctl load`。)

**Python(可选)**:`pip3 install psutil`。

---

## 六、验证

1. 浏览器打开 `http://<服务端IP>:8529`,几秒后应看到该主机卡片出现在对应**分类分组**下。
2. 卡片上应有真实的 CPU / 内存 / SWAP / **每个磁盘一条** / 负载 1·5·15 / 网络收发 / TCP 连接数 / 进程数。
3. 点主机名可看**趋势弹窗**(CPU/内存/磁盘 sparkline)。
4. 装了 psutil 的话,底部会出现 `svc.*`、`proc.*`、`cpu.anomaly_zscore` 等自定义指标 chips。

命令行快速自检(在服务端上):
```bash
curl http://localhost:8529/api/v1/hosts | python3 -m json.tool
```

---

## 七、告警推送配置

告警(飞书 / 钉钉)在**面板**上配置,无需改文件:

1. 面板右上角点 **告警设置**。
2. 填入**飞书**或**钉钉**机器人的 Webhook 地址(钉钉若开"加签"再填 Secret),勾选启用。
3. 点**发送测试**确认通道连通(失败会返回真实错误码,便于排查关键词/签名问题)。
4. 点**保存**——保存后会**立即把当前未恢复的告警补推一次**,之后仅在告警"新触发/恢复"时各推一次,不刷屏。

> - **飞书**自定义机器人若设了"自定义关键词",请把关键词设为 `AIOps` 或 `告警`(推送内容含【AIOps Monitor】与"告警"字样)。
> - **钉钉**建议用"加签"安全设置,把 Secret 填进面板即可自动签名。

---

## 八、升级与卸载

**升级 Agent**:停服务 → 替换二进制(和有改动的 `plugins/`)→ 起服务。`host_id` 存在 `agent_state.json`,升级不会改变主机身份。
```bash
systemctl stop aiops-agent && cp new/aiops-agent /opt/aiops-agent/ && systemctl start aiops-agent
```

**卸载**:
```bash
# Linux
systemctl disable --now aiops-agent && rm /etc/systemd/system/aiops-agent.service
# Windows(NSSM)
nssm stop AIOps-Agent && nssm remove AIOps-Agent confirm
# Windows(任务计划)
schtasks /Delete /TN "AIOps-Agent" /F
# macOS
launchctl unload ~/Library/LaunchAgents/com.aiops.agent.plist
```
在面板上对下线的主机点卡片右上角 **✕** 可删除(约 60 秒内不会因残留上报而复活)。

---

## 九、常见问题 FAQ

**面板上看不到主机?**
- 检查 Agent 日志有没有"上报成功";没有多半是连不上服务端。
- 服务端 `8529` 端口是否放行;`--server` 地址(IP/端口/http 前缀)是否正确。
- 服务端与客户端网络是否互通:`curl http://<服务端IP>:8529/api/v1/summary`。

**主机出现了,但基础指标是 0?**
- 正常情况下不该发生(三平台均原生采集)。若某平台异常,把 Agent 日志贴出来。
- 极少数最小化系统缺少 `/proc`(Linux)或权限受限时,可装 psutil 让 `core_metrics.py` 兜底。

**没有自定义指标 / 插件事件?**
- `plugins/` 目录没跟 Agent 放一起,或没用 `--plugins-dir` 指对路径。
- 没装 psutil(`svc.*`、`proc.*`、异常检测都依赖它)。

**磁盘只显示一个 / 少了盘?**
- 本地固定盘会全部自动识别。Windows 默认**不含**移动 U 盘、网络映射盘(避免弹窗/超时);需要的话告诉维护者开启。
- Linux 只统计 `/dev/` 真实挂载,网络挂载(NFS/CIFS)默认不计。

**告警没收到推送?**
- 面板"告警设置"里 Webhook 是否**已保存**且**已勾选启用**;点"发送测试"看返回。
- 飞书关键词 / 钉钉加签是否匹配(见[第七节](#七告警推送配置))。

**主机分类不对 / 想改?**
- Agent 端用 `--category` 设定;也可在面板点卡片上的分类标签**手动覆盖**(覆盖优先于 Agent 上报)。
