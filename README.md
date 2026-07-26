<div align="center">

# AIOps Monitor

**一个二进制，替代 5+ 套运维工具栈的开源全栈可观测与 SRE 平台。**

</div>

<div align="center">

[![Version](https://img.shields.io/badge/Version-v0.18.9-blue)](https://github.com/sreyun/aiops-monitor/releases)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#开源与社区)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

**[中文](README.md) · [English](README_EN.md)**

</div>

> **单二进制服务端 + 零依赖 Agent**：一行命令拉起「可观测 · 告警治理 · 自动化自愈 · AI 巡检诊断 · SRE 闭环 · 远程桌面 · SQL 工具 · 安全中心 · 安卓移动控制台」全套能力。100% 开源、私有化自托管、数据完全自持，不依赖任何 SaaS、不上送任何遥测。

**当前版本 [v0.18.9](https://github.com/sreyun/aiops-monitor/releases/tag/v0.18.9)** · 镜像同步：GitHub / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## 为什么需要 AIOps Monitor

监控工具越堆越多，问题反而越来越难查：指标在一个系统、日志在另一个、告警风暴刷屏、根因靠人肉翻。多数商业方案按主机数或功能模块收费，且数据必须留在厂商云上。

AIOps Monitor 的思路不同——**把监控、告警、自动化、AI 诊断、SRE 工作流、远程操控和移动端收敛进一个自托管平台**：

- **少即是多**：一个 Go 二进制服务端 + 一个零依赖 Agent，覆盖 Zabbix / Prometheus / Grafana / Alertmanager / 自动化剧本 / 终端网关 / 远程桌面 的常用能力，少维护 5+ 套组件。
- **一条命令部署**：`docker compose up -d` 即可起全栈；Agent 一键安装、跨平台原生采集。
- **数据自持**：关系数据落 PostgreSQL，时序数据落 VictoriaMetrics，**两个都是你自己掌控的开源数据库**，可随时导出、可审计、可合规。
- **AI 不绑架**：AI 巡检诊断是**可插拔**的增值层，接入任意 OpenAI 兼容大模型即「智能模式」，不接则自动回退「启发式兜底」——零外部依赖也能跑。
- **移动端**：企业级安卓 / 鸿蒙控制台为**外部分发包**（本仓库不包含移动端源码）；Web 面板已支持 PWA / 手机浏览器完成看指标、批告警、终端与 SRE 闭环。

---

## 版本亮点（v0.18 → Year-1 MVP）

| 版本 | 重点 |
|---|---|
| **v0.18.9+ Year-1** | 事件一键闭环；效果运营 KPI（MTTR/MTTA、告警噪声、变更失败率、AI 采纳/验证、闭环率）；Hermes 多轮工具+Fallback+学习沉淀；业务服务树与变更影响面；验收见 [docs/year1-acceptance.md](docs/year1-acceptance.md) |
| **v0.18.9** | ITSM 轻量工单/变更状态机、OpsLink、SQL↔ChangeRecord 双向挂接 |
| **v0.18.2** | Windows 锁屏 **Ctrl+Alt+Del**：Session-0 服务经命名管道注入 SAS，自动启用 `SoftwareSASGeneration` |
| **v0.18.0** | 安全可控闭环（编排审批 / SQL 变更闸门 / 安全总览）；资源搜索；主机深度巡检；远程桌面锁屏工具条 |
| **v0.17.x** | Hyper-V / 容器 / Kubernetes 资源层与跨层 AI 定位 |

完整变更见 [Releases](https://github.com/sreyun/aiops-monitor/releases)。

---

## 核心能力全景

### 1. 全栈可观测（Observe）

- **四平台原生采集**：Linux / Windows / macOS / 麒麟（Kylin），Agent 纯 Go 标准库实现、**零第三方依赖**；含 GPU（NVIDIA / AMD / Apple）、CPU、内存、SWAP、磁盘、网络、TCP 连接、负载、进程、运行时长。
- **主动拨测**：HTTP（状态码 / 延时 / TLS 证书剩余天数）、TCP、Ping（丢包率 / RTT）、UDP、进程存活、OpenAPI 业务拨测、分布式多点探测。
- **硬件巡检（Redfish）**：标准 Redfish/DMTF 协议采集 CPU / 内存 / 磁盘 / RAID / 网卡 / 风扇 / 电源 / 温度，含华为 iBMC 深度兼容；无需在被采集设备装 Agent。
- **流量分析**：NetFlow v5/v9/IPFIX 五元组采集与 TOP-N 排行、流量热力图。
- **存储采集**：华为 OceanStor 存储池 / LUN / 控制器 / 告警纳管。
- **交互式趋势图**：纯 Canvas 实现，悬停十字线、框选放大、双击还原、统一时间跨度（1h~30 天）。
- **日志聚合**：Agent 增量采集日志 → 服务端按主机 / 级别 / 关键字 / 时间全文检索，AES-256-GCM 加密上报。
- **主机深度巡检（host_inspect）**：一键采集 OS / 内核 / 网卡 / 磁盘 / 服务摘要；剧本步骤可回写巡检结果；Windows 中文环境编码纠偏。

### 2. 资源闭环（Resources）

- **Hyper-V**：Windows 宿主自动探测，虚拟机清单与资源摘要。
- **容器**：检测到 Docker / Podman CLI 时自动采集容器清单与资源摘要。
- **Kubernetes**：节点 / Pod / Deployment / 事件面板，支持跨层检索与 AI 定位。
- **全局资源搜索**：主机 / VM / 容器 / K8s 对象统一检索入口。
- **依赖拓扑**：边关系维护 + **自动发现**（`POST /api/v1/topology/auto-discover`）辅助爆炸半径与 RCA。

### 3. 告警治理（Govern）

完整的告警生命周期管理，从源头抑制告警风暴：

- **三档阈值预设**：保守 / 标准 / 宽松，覆盖主机、拨测、API、编排任务、端口转发等维度的 warn/crit 细粒度阈值。
- **告警治理三板斧**：**静默**（时段 / 星期）→ **抑制**（主因告警抑制衍生告警）→ **路由**（按级别 · 主机分流渠道），让严重告警走电话、警告只走飞书。
- **多通道推送**：飞书 / 钉钉 Webhook、邮件 SMTP、以及阿里云 / 华为云 / 腾讯云**多云短信 + 语音电话（TTS）**；触发 / 恢复各推一次，不刷屏。
- **去重防抖**：仅在「新触发」与「恢复」时各推一次。
- **安全发现生命周期**：主机 / Web 扫描 finding 可追踪、可闭环；扫描任务具备看门狗与可取消能力。

### 4. 自动化与自愈（Remediate）

- **自动化剧本**：Shell + 跨平台内置模块编排，确定性预检、并发上限、条件/变量、可配置重试、显式逆序回滚，实时输出 + 全链路执行审计。
- **高风险闸门**：定时高风险剧本进入 `pending_approval`；人工审批 / 拒绝后才继续；执行结果支持 `partial` 与失败原因。
- **SRE 事件闭环**：告警 / SLO / 手动事件汇聚 → 时间线 → 认领 / 解决 / 升级工单，**自动去重与开合**；支持 **On-call 排班与超时升级**。
- **变更管理**：变更窗 / 冻结期 + 变更记录；事件 RCA / 时间线自动关联近期变更；冻结期内未审批自愈强制闸门。
- **自动修复闸门**：告警自动触发剧本修复，内置**人工审批闸门 + 命令白名单 / 危险命令拦截 + 护栏（guardrails）**，高危操作不自动放行。
- **SLO / 错误预算**：多窗口多燃烧率（multi-window multi-burn-rate）算法评估 SLO 突破。
- **工单系统（人机协同闭环）**：
  - 事件可一键升级为工单；状态 / 优先级 / 描述可编辑。
  - **指派给真实账号**：`GET /api/v1/directory/users`（viewer+）提供用户目录，Web / App 下拉选择。
  - **图片与文件附件**：创建工单、工单评论、事件评论统一支持图片与文档；时间线可回看证据。

### 5. AI 巡检诊断（Diagnose）

- **定时 / 手动健康巡检**：综合在线 / 离线主机、活跃告警、SLO 突破、近期错误日志产出健康研判。
- **事件根因诊断**：critical 事件自动触发 AI 根因研判并写入事件时间线；支持流式诊断与追问。
- **依赖拓扑 RCA**：主机 / 分类 / 服务边关系 + 变更关联时间线，辅助定位爆炸半径。
- **RAG 向量学习闭环**：基于 pgvector 的记忆 / 技能检索，对诊断结果做**👍 上浮 / 👎 下沉的反馈重排学习**——越用越准；结案卡与采纳反馈可升格为可复用 Skill。
- **WeKnora 外部文档 RAG**：复杂手册 / Wiki / PDF 由 [WeKnora](https://github.com/Tencent/WeKnora) 维护；本平台在 **AI 设置 → RAG → WeKnora** 填写 API URL + API Key（可选知识库 ID），供 `search_knowledge` 工具检索；不可用时自动降级为本地记忆 / 技能。
- **AI 运维助手（多模态 + 语音）**：
  - 多轮 SSE 流式对话 + Function Calling（查指标 / 检日志 / 列告警 / 相似案例 / 诊断 / 脚本动作）。
  - **Web**：上传图片与文档、抓取 URL；**语音输入（SpeechRecognition）+ 朗读回复（speechSynthesis）**。
  - **Android**：Copilot / 事件诊断追问与 Web 对齐，可发送图片与解析后的文件。
- **可插拔、零绑架**：接入任意 OpenAI 兼容 LLM 即智能模式；**未配置 AI 时自动回退内置启发式兜底**。
- **向量模型解耦**：embedding / 对话 / 可选 rerank 独立配置，一键连通性自检；记忆库浏览与 AI 调用统计可观测。

### 6. 远程桌面与终端（Control）

- **远程终端**：经 Agent 反向隧道免开入站端口；VT 体验 + 会话回放。
- **远程桌面（Web）**：
  - JPEG / H.264（可用时）推流；多显示器选择；画质预设（流畅 / 均衡 / **清晰 15fps**）。
  - 文件传输（Agent 通道，约 100MB 上限）与剪贴板同步。
  - **锁屏 / 注销（Windows）**：以 **Windows 服务 + 桌面 worker** 运行时可跟随 Winlogon；工具条提供 Ctrl+Alt+Del / 唤醒 / 解锁凭据发送 / Esc / Win+L / 任务管理器。
  - Ctrl+Alt+Del 由 **Session-0 服务经命名管道注入 SAS**，并自动配置 `SoftwareSASGeneration`（需 Agent ≥ v0.18.2 且 `--install-service`）。
- **端口转发 / `/proxy`**：无状态 HTTP 反向代理，支持 WebSocket 升级；跳板访问局域网其他服务。

### 7. SQL 工具箱（Data）

- 多数据源连接与 Schema / 历史查询。
- **EXPLAIN 对比**与变更洞察。
- **高风险 SQL 变更工单闸门**：未审批不执行破坏性变更（与 SRE 变更体系对齐）。

### 8. 安全合规（Secure）

- **强会话鉴权**：会话 Cookie 基于 **PBKDF2-HMAC-SHA256（60 万次迭代）**；`HttpOnly` + `SameSite` + HTTPS 下 `Secure`。
- **RBAC 路由矩阵**：admin / operator / viewer 三角色，路由级权限拦截。
- **可选 TOTP MFA**：RFC 6238，单次使用防重放；Google Authenticator 兼容。
- **终端 / 远程桌面二次密码**：敏感操控前二次认证，带限流保护。
- **双维防暴破**：IP + 账户双维度滑动窗口限流。
- **机器指纹防克隆**：`X-Agent-Fingerprint` 绑定设备，克隆镜像自动重生 host_id。
- **安全中心**：安全总览 + 主机安全 / Web 安全分栏；扫描与 finding 生命周期管理。
- **跨平台大模型内容审计**：Linux 原生 AF_PACKET，Windows/macOS 可接入 TShark；支持 DNS/SNI、受控明文 HTTP 重组、主流 LLM 识别与端侧脱敏。HTTPS 正文推荐在 LLM Gateway / SDK 层审计。
- **配置静态加密**：MFA / SMTP / AI / webhook / 中继等密钥经 `AIOPS_SECRET_KEY` 派生 **AES-256-GCM** 落库。
- **出站防护**：AI / Webhook 等出站请求经 SSRF 守卫；可选 `AIOPS_SSRF_STRICT` 拒私网。
- **TLS 可选**：支持 `AIOPS_TLS_CERT/KEY` 启用 HTTPS。

### 9. 安卓移动控制台（Mobile）

配套 **20+ 屏幕的企业级原生安卓控制台**（Kotlin + Jetpack Compose，minSdk 26 / targetSdk 34），非 WebView 套壳：

- **SRE 驾驶舱总览**、主机详情（原生 Canvas 时序图）、告警批处理、**企业级 VT 终端**。
- **运维中心 SRE Hub**：事件闭环 / AI 诊断流式追问 / 剧本 / SLO / 修复审批 / 工单 / On-call / 变更。
- 监控拨测、AI 助手、硬件 / NetFlow / Hyper-V、会话回放、消息中心、告警治理、环境切换等。
- 鉴权：登录 Cookie + MFA 弹窗 + 终端二次密码；自建 `/ws/push` 前台服务 + 系统通知。

### 10. Web 控制台体验

- **统一设计 Token**：深色专业后台，间距 / 圆角 / 状态色 / 附件芯片对齐。
- 顶栏全局 AI 对话入口；语音输入与朗读在 Chrome / Edge 等支持 Web Speech 的浏览器可用。
- **企业运营**：个人信息 →「数据与备份」（admin）可配置数据保留期、自愈命令白名单、PostgreSQL 定时备份 / 下载 / 二次确认还原（VictoriaMetrics 需外部备份）。

### 11. 部署韧性（Resilient）

- **双强制存储**：PostgreSQL + VictoriaMetrics，**任一未配置即拒绝启动**。
- **Schema 版本化迁移**：`schema_migrations` 按版本增量 DDL，失败即中止以免半迁移。
- **网关中继（Relay）**：内网仅一台联网机器代理所有请求到服务端；`X-Relay-Secret` 防 Host 注入。
- **多服务端并发广播**：Agent `servers[]` 采集一次广播所有；**断路器 + 退避 + gzip 降级**。
- **安装令牌轮换 + 7 天宽限**：Token 轮换不影响已装 Agent 持续上报。
- **一键安装 & 开机自启**：面板生成带 Token 命令；Windows 建议以服务安装以启用锁屏远程桌面。
- **跨平台多架构**：amd64 + arm64 预构建镜像，Docker 一行拉起。

---

## 架构概览

```
┌────────────────── 采集端（零依赖 Go Agent） ──────────────────┐
│ 四平台原生采集 → 指标 / GPU / 日志加密上报                      │
│ 主动拨测 · Redfish · NetFlow · OceanStor · Hyper-V / 容器     │
│ 远程终端 · 远程桌面 worker（Windows 服务跟随 Winlogon）         │
│ 机器指纹鉴权 · Relay 中继 · 多服务端广播                        │
└───────┬────────────────────────────────────┬────────────────┘
        │ 上报 / 拨测 / 终端 / 桌面 / 转发      │ servers[] 广播
        ▼                                    ▼
┌──────────────────── 服务端（单 Go 二进制） ────────────────────┐
│ 告警引擎 → 治理(静默/抑制/路由) → 事件 → 剧本审批 → SLO → 工单 │
│ AI 巡检诊断 + RAG（pgvector） · SQL 工具箱 · 安全中心          │
│ 远程桌面中继 · 拓扑/RCA · 资源搜索 · RBAC / MFA                │
│  ┌──────────── 双强制存储（缺一拒启动）────────────┐            │
│  │ PostgreSQL：关系/审计/事件/工单/会话/向量记忆     │            │
│  │ VictoriaMetrics：全部时序指标                   │            │
│  └──────────────────────────────────────────────┘            │
└──────────────────────────┬───────────────────────────────────┘
                           │ REST + WebSocket (/ws/push · desktop)
                           ▼
              ┌──── Web 控制台 / 安卓 / 鸿蒙 ────┐
              │ 总览 · 主机 · 告警 · 终端 · 桌面   │
              │ SRE Hub · AI · SQL · 安全 · K8s   │
              └──────────────────────────────────┘
```

**分工原则**：高频基础采集用 Go 单二进制（零依赖）；Redfish / NetFlow / OceanStor 走标准协议由 Agent 远程轮询；Windows 锁屏远程桌面必须由 **LocalSystem 服务 + 会话内桌面 worker** 协作完成。

---

## 快速开始

### Docker Compose（正式 / 开发）

| 文件 | 场景 | 说明 |
|---|---|---|
| `docker-compose.yml` | **正式环境（默认）** | 拉取华为云 SWR 预构建镜像；Agent 可选（`--profile agent`） |
| `docker-compose.dev.yml` | **开发环境 overlay** | 本地 `docker/Dockerfile.dev` 构建；PG/VM 改用 Docker Hub；默认启 Agent |

**正式环境（推荐）**

```bash
# 一键：下载编排 + 生成强随机密钥到 .env，再启动
bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh)
docker compose up -d

# 或手动：
# curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml
# curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/master/.env.example
# cp .env.example .env   # 务必修改 POSTGRES_PASSWORD / AIOPS_SECRET_KEY
# docker compose up -d
```

**开发环境（源码目录）**

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
# 可选：在 .env 写入 COMPOSE_FILE=docker-compose.yml:docker-compose.dev.yml 后直接 docker compose up -d --build
```

启动后浏览器打开 `http://localhost:8529`，默认凭据 `admin / admin`，**首次登录强制走安全初始化（必须修改用户名 + 密码）**，建议随后启用 MFA。

> `secure-compose.sh` 生成 20 位 PG 密码与 50 位 `AIOPS_SECRET_KEY` 写入 `.env`（勿提交 Git）。本机拉不到 SWR 时，在 `.env` 将 `POSTGRES_IMAGE` / `VM_IMAGE` 改为 Docker Hub 镜像，或直接用开发 overlay。

### 安装 Agent（被监控主机）

面板右上角「安装 Agent」→ 选系统 → 复制命令到目标机执行：

```bash
# Linux（root）
curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows（管理员 PowerShell）— 安装为服务以支持锁屏/注销远程桌面
irm "http://<服务端>:8529/install.ps1?token=<TOKEN>" | iex
# 或升级后显式重装服务：
# aiops-agent --install-service
```

> 服务端**强制依赖** PostgreSQL 与 VictoriaMetrics，缺一拒绝启动。更多部署方式（二进制直跑 / 自编译 / 开机自启 / Nginx 反代 / 网关中继）见 [INSTALL.md](INSTALL.md)。  
> Agent 完整配置项见仓库根目录 [`config.example.yaml`](config.example.yaml)。

### Windows 远程桌面要点

1. Agent **必须以服务安装**（LocalSystem），否则只能抓已登录会话，锁屏/注销不可用。  
2. 升级到 **≥ v0.18.2** 后执行 `aiops-agent --install-service`，以启用 SAS 管道与 `SoftwareSASGeneration`。  
3. Web 远程桌面：先点 **Ctrl+Alt+Del**，再点 **解锁** 发送凭据（凭据不落盘、审计不含明文）。  
4. 若安装时报 Application Control 拦截，先放行二进制再装服务。

---

## 典型场景

| 场景 | AIOps Monitor 怎么用 |
|---|---|
| **中小型机房统一监控** | 单服务端纳管数百台 Linux/Windows/macOS/麒麟主机，原生采集 + 三档阈值预设开箱即用 |
| **告警风暴治理** | 「静默 + 抑制 + 路由」分流渠道；严重走电话，警告走 IM |
| **业务可用性 SLA** | API 黑盒拨测 + SLO 多窗口燃烧率 |
| **故障自愈** | 告警触发剧本；高危动作卡审批闸门，全程审计 |
| **锁屏远程救援** | Windows 服务 Agent + Web 远程桌面：Winlogon 画面、Ctrl+Alt+Del、凭据解锁 |
| **智能根因定位** | LLM 诊断 + 拓扑 RCA + RAG 👍/👎 反馈学习 |
| **证据闭环工单** | 事件升级工单 → 指派真实用户 → 附图评论 → App / Web 同步 |
| **资源与变更联动** | Hyper-V / 容器 / K8s 清单 + SQL 变更闸门 + 变更窗冻结 |
| **安全巡检** | 安全中心聚合主机/Web 扫描 finding，可追踪闭环 |
| **外出应急** | 原生安卓控制台：总览、批告警、VT 终端、SRE / 工单 |
| **跨网段 / 弱网采集** | 网关中继 + 多服务端广播 + 断路器 / gzip 降级 |

---

## 企业服务

AIOps Monitor 本体 100% 开源（MIT），可自由自托管。对于企业级进阶需求，可基于开源版提供：

- **私有化部署咨询**：大规模（万级主机）分片、VictoriaMetrics 外接、保留期调优。
- **定制集成**：对接企业微信 / 钉钉 / 飞书深度能力、CMDB、工单系统、内部大模型网关。
- **安全合规加固**：SSO / LDAP、审计留存、等保适配建议。
- **安卓分发通道**：私有化应用分发与签名托管（见下方诚实边界）。

> 有企业合作需求可在 GitHub / Gitee 仓库提交 Issue 或联系维护者。

---

## 诚实边界与已知限制

我们坚持如实描述能力，以下边界请在使用前知悉：

**后端 / 平台**

- 服务端强制依赖 PostgreSQL 与 VictoriaMetrics；单机建议规模约 3000 台主机（超大规模建议外接 VictoriaMetrics）。
- AI 巡检诊断为可插拔增值能力，未配置大模型时回退启发式兜底，不保证与 LLM 同等深度的语义分析。
- Web 语音输入 / 朗读依赖浏览器 Web Speech API（Chrome / Edge 体验最佳；部分环境需麦克风权限与 HTTPS）。
- 工单 / 评论附件以 JSON 快照持久化，适合证据级截图与中小文档；超大二进制请走对象存储等外部系统。

**远程桌面**

- Windows **锁屏 / 注销 / UAC** 能力依赖 Agent **服务安装**；仅前台用户进程无法可靠注入 Ctrl+Alt+Del。
- 桌面 worker 在安全桌面强制走 GDI/JPEG（避免 ffmpeg 抓到黑屏）；帧率与带宽受网络影响。
- macOS 锁屏可能受 Secure Input / 屏幕录制权限限制；Linux 依赖图形会话 / greeter 是否存在。
- 「解锁」发送的是本次内存中的凭据文本，**不落盘**；仍请仅在受信运维场景使用。

**安卓移动控制台**

- **私有化自托管分发，未上架任何应用商店**；以 APK 方式安装，需自行签名与分发。
- 请以你本地 Android Studio 实际构建结果为准。
- **账号自服务仍在网页端**：MFA 自助绑定、忘记密码、首次登录强制改密等 UI 在 Web 端完成。
- 会话 Cookie 使用**普通 DataStore 持久化（未加密）**；采用**固定轮询**全量拉取，**未接入 FCM**。
- 上述限制不影响其作为「企业级原生移动控制台」在自托管内网场景下的实用价值。

---

## 开源与社区

AIOps Monitor 以 **MIT 协议 100% 开源**，无功能阉割、无用户数限制、无遥测上送。

- **代码规模（约）**：服务端 `cmd/server` 约 250+ 个 Go 文件，Agent `cmd/agent` 约 120+ 文件，配套 **130+** 自动化测试，生产级成熟度。
- **全链路自托管**：关系数据（PostgreSQL）+ 时序数据（VictoriaMetrics）均在你自己掌控的环境。
- **双仓库同步**：GitHub 与 Gitee 同步推送分支与标签。
- **欢迎贡献**：Issue、PR、文档与插件均欢迎。

---

## 相关链接

| 资源 | 链接 |
|---|---|
| GitHub | <https://github.com/sreyun/aiops-monitor> |
| Gitee | <https://gitee.com/bigdatasafe/aiops-monitor> |
| 发布版本 | <https://github.com/sreyun/aiops-monitor/releases> |
| 安装部署 | [INSTALL.md](INSTALL.md) |
| Agent 配置示例 | [config.example.yaml](config.example.yaml) |
| 安卓客户端 | [android/README.md](android/README.md) |
| 鸿蒙客户端 | [harmony/README.md](harmony/README.md) |

---

## License

[MIT](LICENSE)
