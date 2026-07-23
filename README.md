<div align="center">

# AIOps Monitor

**一个二进制，替代 5+ 套运维工具栈的开源全栈可观测与 SRE 平台。**

</div>

<div align="center">

[![Version](https://img.shields.io/badge/Version-v6.8.1-blue)](https://github.com/sreyun/aiops-monitor/releases)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#开源与社区)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

**[中文](README.md) · [English](README_EN.md)**

</div>

> **单二进制服务端 + 零依赖 Agent**：一行命令拉起「可观测 · 告警治理 · 自动化自愈 · AI 巡检诊断 · SRE 闭环 · 安卓移动控制台」全套能力。100% 开源、私有化自托管、数据完全自持，不依赖任何 SaaS、不上送任何遥测。

---

## 为什么需要 AIOps Monitor

监控工具越堆越多，问题反而越来越难查：指标在一个系统、日志在另一个、告警风暴刷屏、根因靠人肉翻。多数商业方案按主机数或功能模块收费，且数据必须留在厂商云上。

AIOps Monitor 的思路不同——**把监控、告警、自动化、AI 诊断、SRE 工作流和移动端收敛进一个自托管平台**：

- **少即是多**：一个 Go 二进制服务端 + 一个零依赖 Agent，覆盖 Zabbix / Prometheus / Grafana / Alertmanager / 自动化剧本 / 终端网关 的常用能力，少维护 5+ 套组件。
- **一条命令部署**：`docker compose up -d` 即可起全栈；Agent 一键安装、跨平台原生采集。
- **数据自持**：关系数据落 PostgreSQL，时序数据落 VictoriaMetrics，**两个都是你自己掌控的开源数据库**，可随时导出、可审计、可合规。
- **AI 不绑架**：AI 巡检诊断是**可插拔**的增值层，接入任意 OpenAI 兼容大模型即「智能模式」，不接则自动回退「启发式兜底」——零外部依赖也能跑。
- **移动优先**：配套企业级原生安卓控制台，以及鸿蒙 NEXT 原生客户端（`harmony/`），手机上即可看指标、批告警、开终端、走 SRE 闭环。

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

### 2. 告警治理（Govern）

完整的告警生命周期管理，从源头抑制告警风暴：

- **三档阈值预设**：保守 / 标准 / 宽松，覆盖主机、拨测、API、编排任务、端口转发五大维度共 27 组 warn/crit 细粒度阈值。
- **告警治理三板斧**：**静默**（时段 / 星期）→ **抑制**（主因告警抑制衍生告警）→ **路由**（按级别 · 主机分流渠道），让严重告警走电话、警告只走飞书。
- **多通道推送**：飞书 / 钉钉 Webhook、邮件 SMTP、以及阿里云 / 华为云 / 腾讯云**多云短信 + 语音电话（TTS）**；触发 / 恢复各推一次，不刷屏。
- **去重防抖**：仅在「新触发」与「恢复」时各推一次。

### 3. 自动化与自愈（Remediate）

- **自动化剧本**：多步骤 Shell 编排，按「全部 / 分类 / 系统 / 主机」批量并行执行，实时输出 + 历史报告。
- **SRE 事件闭环**：告警 / SLO / 手动事件汇聚 → 时间线 → 认领 / 解决 / 升级工单，**自动去重与开合**；支持 **On-call 排班与超时升级**。
- **变更管理**：变更窗 / 冻结期 + 变更记录；事件 RCA / 时间线自动关联近期变更；冻结期内未审批自愈强制闸门。
- **自动修复闸门**：告警自动触发剧本修复，内置**人工审批闸门 + 命令白名单 / 危险命令拦截 + 护栏（guardrails）**，高危操作不自动放行。
- **SLO / 错误预算**：多窗口多燃烧率（multi-window multi-burn-rate）算法评估 SLO 突破。
- **工单系统（人机协同闭环）**：
  - 事件可一键升级为工单；状态 / 优先级 / 描述可编辑。
  - **指派给真实账号**：`GET /api/v1/directory/users`（viewer+）提供用户目录，Web / App 下拉选择，不再依赖「系统」自由文本。
  - **图片与文件附件**：创建工单、工单评论、事件评论统一支持图片（base64）与文档（文本 / Word / Excel / PDF 经 `/hermes/parse` 抽取），时间线可回看证据。
  - App 运维中心同步：工单指派下拉 + 评论；与 Web 共用同一套 API 契约。

### 4. AI 巡检诊断（Diagnose）

- **定时 / 手动健康巡检**：综合在线 / 离线主机、活跃告警、SLO 突破、近期错误日志产出健康研判。
- **事件根因诊断**：critical 事件自动触发 AI 根因研判并写入事件时间线；支持流式诊断与追问。
- **依赖拓扑 RCA**：主机 / 分类 / 服务边关系 + 变更关联时间线，辅助定位爆炸半径。
- **RAG 向量学习闭环**：基于 pgvector 的记忆 / 技能检索，对诊断结果做**👍 上浮 / 👎 下沉的反馈重排学习**——越用越准。
- **AI 运维助手（多模态 + 语音）**：
  - 多轮 SSE 流式对话 + Function Calling（查指标 / 检日志 / 列告警 / 相似案例 / 诊断 / 脚本动作）。
  - **Web**：上传图片与文档、抓取 URL；**语音输入（SpeechRecognition）+ 朗读回复（speechSynthesis）**。
  - **Android**：Copilot / 事件诊断追问与 Web 对齐，可发送图片与解析后的文件。
- **可插拔、零绑架**：接入任意 OpenAI 兼容 LLM 即智能模式；**未配置 AI 时自动回退内置启发式兜底**。
- **向量模型解耦**：embedding / 对话 / 可选 rerank 独立配置，一键连通性自检；记忆库浏览与 AI 调用统计可观测。

### 5. 安全合规（Secure）

- **强会话鉴权**：会话 Cookie 基于 **PBKDF2-HMAC-SHA256（60 万次迭代）**；`HttpOnly` + `SameSite` + HTTPS 下 `Secure`。
- **RBAC 路由矩阵**：admin / operator / viewer 三角色，路由级权限拦截。
- **可选 TOTP MFA**：RFC 6238，单次使用防重放；Google Authenticator 兼容。
- **终端二次密码**：敏感终端操作前二次认证，带限流保护。
- **双维防暴破**：IP + 账户双维度滑动窗口限流。
- **机器指纹防克隆**：`X-Agent-Fingerprint` 绑定设备，克隆镜像自动重生 host_id。
- **配置静态加密**：MFA / SMTP / AI / webhook / 中继等密钥经 `AIOPS_SECRET_KEY` 派生 **AES-256-GCM** 落库。
- **出站防护**：AI / Webhook 等出站请求经 SSRF 守卫，默认拒云元数据与链路本地地址；可选 `AIOPS_SSRF_STRICT` 拒私网。
- **TLS 可选**：支持 `AIOPS_TLS_CERT/KEY` 启用 HTTPS 加密传输。

### 6. 安卓移动控制台（Mobile）

配套 **20+ 屏幕的企业级原生安卓控制台**（Kotlin + Jetpack Compose，minSdk 26 / targetSdk 34），非 WebView 套壳，详见下方「诚实边界」。核心屏幕包括：

- **SRE 驾驶舱总览**：关键指标 + 主机 / 告警汇总，深浅色双主题。
- **主机详情**：原生 Canvas 时序图（点选 / 平移 / 双指缩放），磁盘卷 / GPU 设备明细。
- **告警**：级别 / 状态双维筛选 + 一键确认 / 静默 + AI 诊断。
- **企业级 VT 终端**：VT100 / UTF-8 译码、指数退避重连、软键盘避让、横竖屏不重建。
- **运维中心 SRE Hub**：事件闭环 / AI 诊断流式追问（含图片文件） / 剧本 / SLO / 修复审批 / **工单指派真实用户 + 评论** / **On-call 排班与升级** / **变更窗与变更关联**。
- **监控拨测**、**AI 助手（SSE 流式 + 附件）**、**硬件 / NetFlow / Hyper-V**、**终端会话回放**、**消息中心**、**重复主机清理**、**告警治理**、**终端密码**、**环境切换**等。
- 鉴权：登录 `POST /api/v1/login` → Cookie，`DataStore` 双轨持久化；登录 MFA 动态口令弹窗、终端二次密码 UI；自建 `/ws/push` 长连接前台服务 + 系统通知。
- 关键目录：`GET /api/v1/directory/users`（轻量字段，viewer+），与 Web 工单指派同源。

### 7. Web 控制台体验

- **统一设计 Token**：深色专业后台（蓝主色 + 紫辅色），间距 / 圆角 / 状态色 / 附件芯片 / 评论条对齐，减少模块间视觉割裂。
- **工单 / 事件 / AI** 共用附件条交互；升级工单后可直接打开工单弹窗完成指派与补证。
- 顶栏全局 AI 对话入口；语音输入与朗读在 Chrome / Edge 等支持 Web Speech 的浏览器可用。
- **企业运营**：个人信息 →「数据与备份」（admin）可配置数据保留期、自愈命令白名单、PostgreSQL 定时备份 / 下载 / 二次确认还原（VictoriaMetrics 需外部备份）。

### 8. 部署韧性（Resilient）

- **双强制存储**：PostgreSQL + VictoriaMetrics，**任一未配置即拒绝启动**，从架构上保证数据不丢。
- **Schema 版本化迁移**：`schema_migrations` 按版本增量 DDL（On-call / 变更 / 备份元数据等），失败即中止以免半迁移。
- **网关中继（Relay）**：内网仅一台联网机器代理所有请求到服务端，自动穿透二进制 / 上报 / 终端；`X-Relay-Secret` 防 Host 注入。
- **多服务端并发广播**：Agent `servers[]` 采集一次广播所有，独立鉴权 / 重试 / 连接池；带**断路器 + 退避 + gzip 降级**容灾。
- **安装令牌轮换 + 7 天宽限**：Token 轮换不影响已装 Agent 持续上报。
- **远程终端 + 端口转发**：经 Agent 反向隧道免开端口访问远端服务；`/proxy` 无状态 HTTP 反向代理，支持 WebSocket 升级。
- **一键安装 & 开机自启**：面板生成带 Token 命令，自动下载 + 配置 + 注册 systemd / launchd / 计划任务保活。
- **跨平台多架构**：amd64 + arm64 预构建镜像，Docker 一行拉起。

---

## 架构概览

```
┌──────────── 采集端（零依赖 Go Agent） ────────────┐
│ 四平台原生采集 → 指标 / GPU / 日志加密上报          │
│ 主动拨测：HTTP/TCP/Ping/UDP/进程/OpenAPI/多点探测   │
│ Redfish 硬件巡检 · NetFlow · OceanStor · 远程终端   │
│ 机器指纹鉴权 · Relay 中继 · 多服务端广播          │
└───────┬───────────────────────────┬───────────────┘
        │ 上报 / 拨测 / 终端 / 转发    │ 并发广播 (servers[])
        ▼                           ▼
┌──────────────── 服务端（单 Go 二进制） ────────────────┐
│ 告警引擎 → 告警治理(静默/抑制/路由) → 事件(去重/开合)  │
│ → 自动修复(剧本+审批闸门+护栏) → SLO → 工单          │
│ AI 巡检诊断 + RAG 向量反馈重排学习闭环（pgvector）     │
│ 远程终端 · 端口转发 /proxy · 统一消息中心 · RBAC/MFA  │
│                                                       │
│  ┌─────────────── 双强制存储（缺一拒启动）──────────┐ │
│  │ PostgreSQL：关系/审计/事件/工单/JSONB/AI规则/会话 │ │
│  │ VictoriaMetrics：全部时序指标                     │ │
│  └─────────────────────────────────────────────────┘  │
└───────────────────────┬───────────────────────────────┘
                         │ RESTful API + WebSocket (/ws/push)
                         ▼
            ┌──────── 安卓企业级移动控制台 ────────┐
            │ Kotlin + Jetpack Compose（20+ 屏幕） │
            │ 总览/主机/告警/终端/SRE Hub/AI/拨测  │
            └──────────────────────────────────────┘
```

**分工原则**：高频、性能敏感的基础采集用 Go 单二进制（零依赖）；外部采集器（Redfish / NetFlow / OceanStor）走标准协议，由能连通目标设备的 Agent 远程轮询，被采集设备无需装 Agent。

---

## 快速开始

### Docker Compose 一条命令（推荐）

```bash
# 拉取编排文件并启动（PG + VictoriaMetrics + AIOps Server 三容器一键起全）
curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml
docker compose up -d
```

启动后浏览器打开 `http://localhost:8529`，默认凭据 `admin / admin`，**首次登录强制走安全初始化（必须修改用户名 + 密码）**，建议随后启用 MFA。

> **生产建议**：使用安全编排脚本自动生成强随机密钥并写入 `docker-compose.yml`：
> ```bash
> bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh) && docker compose up -d
> ```
> 该脚本生成 20 位 PG 密码与 50 位 `AIOPS_SECRET_KEY`，并自动回填 `AIOPS_POSTGRES_DSN`，无需手动改配置。

### 安装 Agent（被监控主机）

面板右上角「安装 Agent」→ 选系统 → 复制命令到目标机执行：

```bash
# Linux（root）
curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sudo sh
# Windows（管理员 PowerShell）
irm "http://<服务端>:8529/install.ps1?token=<TOKEN>" | iex
```

> 服务端**强制依赖** PostgreSQL 与 VictoriaMetrics 两个存储，缺一拒绝启动。更多部署方式（二进制直跑 / 自编译 / 开机自启 / 跨网络 Nginx 反代 / 网关中继）见 [INSTALL.md](INSTALL.md)。

---

## 典型场景

| 场景 | AIOps Monitor 怎么用 |
|---|---|
| **中小型机房统一监控** | 单服务端纳管数百台 Linux/Windows/macOS/麒麟主机，原生采集 CPU/内存/磁盘/GPU，三档阈值预设开箱即用 |
| **告警风暴治理** | 用「静默 + 抑制 + 路由」把夜间非关键告警静默、主因离线抑制衍生告警、严重告警走电话，恢复通知照发 |
| **业务可用性 SLA** | API 监控对核心接口批量黑盒拨测，P95 时延 / 可用率 / 吞吐纳入 SLO 多窗口燃烧率评估 |
| **故障自愈** | 告警触发剧本自动修复，高危动作卡人工审批闸门，修复过程全程审计 |
| **智能根因定位** | 接入大模型后事件自动 AI 诊断，拓扑 RCA + RAG 记忆 / 技能检索，👍/👎 反馈让诊断越用越准；Web 可语音提问并朗读结论 |
| **证据闭环工单** | 从事件升级工单 → 指派真实用户 → 附图 / 文档评论 → App / Web 同步跟进，避免「系统」空指派与口头证据丢失 |
| **外出应急** | 手机打开原生安卓控制台，看总览、批告警、开 VT 终端、向 AI 发送截图 / 日志、走 SRE 事件与工单闭环 |
| **硬件资产合规** | Redfish 巡检 + OceanStor 采集统一硬件资产面板，变更漂移可查，支持导出 |
| **跨网段 / 弱网采集** | 网关中继模式单点穿透；多服务端并发广播 + 断路器 + gzip 降级保障弱网下不丢数据 |

---

## 企业服务

AIOps Monitor 本体 100% 开源（MIT），可自由自托管。对于企业级进阶需求，可基于开源版提供：

- **私有化部署咨询**：大规模（万级主机）分片、VictoriaMetrics 外接、保留期调优。
- **定制集成**：对接企业微信 / 钉钉 / 飞书深度能力、CMDB、工单系统、内部大模型网关。
- **安全合规加固**：SSO / LDAP、审计留存、等保适配建议。
- **安卓分发通道**：私有化应用分发与签名托管（见下方诚实边界）。

> 有企业合作需求可在 GitHub 仓库提交 Issue 或联系维护者。

---

## 诚实边界与已知限制

我们坚持如实描述能力，以下边界请在使用前知悉：

**后端 / 平台**

- 服务端强制依赖 PostgreSQL 与 VictoriaMetrics 两个开源数据库；单机建议规模约 3000 台主机（超大规模建议外接 VictoriaMetrics）。
- AI 巡检诊断为可插拔增值能力，未配置大模型时回退启发式兜底，不保证与 LLM 同等深度的语义分析。
- Web 语音输入 / 朗读依赖浏览器 Web Speech API（Chrome / Edge 体验最佳；部分环境需麦克风权限与 HTTPS）。
- 工单 / 评论附件以 JSON 快照持久化，适合证据级截图与中小文档；超大二进制请走对象存储等外部系统。

**安卓移动控制台**

- **私有化自托管分发，未上架任何应用商店**；以 APK 方式安装，需自行签名与分发。
- 仓库内置历史构建产物证明该客户端**曾经可成功构建**；请以你本地 Android Studio 实际构建结果为准。
- AI Copilot / 事件诊断已支持发送图片与可解析文档；语音输入输出以系统能力为准，Web 端优先完善。
- **账号自服务仍在网页端**：MFA 自助绑定、忘记密码、首次登录强制改密等 UI 在 Web 端完成，安卓端复用同一套 RBAC 账户体系。
- 会话 Cookie 使用**普通 DataStore 持久化（未加密）**。
- 采用**固定轮询**拉取数据，主机 / 告警为**全量拉取**，非增量；**未接入系统级后台推送（FCM）**，依赖前台自动刷新。
- 上述限制不影响其作为「企业级原生移动控制台」在自托管内网场景下的实用价值。

---

## 开源与社区

AIOps Monitor 以 **MIT 协议 100% 开源**，无功能阉割、无用户数限制、无遥测上送。

- **代码规模**：服务端 `cmd/server` 约 126 个 Go 文件 / 4 万+ 行，Agent `cmd/agent` 约 69 文件 / 1.8 万+ 行，配套 64 个测试，生产级成熟度。
- **全链路自托管**：关系数据（PostgreSQL）+ 时序数据（VictoriaMetrics）均在你自己掌控的环境。
- **欢迎贡献**：Issue、PR、文档与插件均欢迎。

---

## 相关链接

- **GitHub 仓库**：<https://github.com/sreyun/aiops-monitor>
- **发布版本**：<https://github.com/sreyun/aiops-monitor/releases>
- **安装部署指南**：[INSTALL.md](INSTALL.md)
- **安卓客户端说明**：[android/README.md](android/README.md)
- **鸿蒙原生客户端**：[harmony/README.md](harmony/README.md)（HarmonyOS NEXT API 18 / ArkTS）

---

## License

[MIT](LICENSE)
