<div align="center">

# AIOps Monitor

**一个二进制，收敛监控 · 告警 · 自愈 · AI 诊断 · SRE 闭环 · 远程操控的开源平台。**

[![Version](https://img.shields.io/badge/Version-v0.19.1-blue)](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.1)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#开源与社区)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

**[中文](README.md) · [English](README_EN.md)**

</div>

> **单二进制服务端 + 零依赖 Agent**：一条命令拉起可观测、告警治理、自动化自愈、AI 巡检诊断、SRE 闭环、远程桌面/终端、SQL 工具与安全中心。100% 开源、私有化自托管、数据完全自持——不依赖 SaaS、不上送遥测。

**当前版本 [v0.19.1](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.1)** · 镜像同步：[GitHub](https://github.com/sreyun/aiops-monitor) / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## 目录

- [为什么选择 AIOps Monitor](#为什么选择-aiops-monitor)
- [v0.19.0 亮点](#v0190-亮点)
- [能力一览](#能力一览)
- [核心能力详解](#核心能力详解)
- [架构概览](#架构概览)
- [快速开始](#快速开始)
- [典型场景](#典型场景)
- [文档索引](#文档索引)
- [诚实边界](#诚实边界)
- [企业服务](#企业服务)
- [开源与社区](#开源与社区)
- [License](#license)

---

## 为什么选择 AIOps Monitor

监控组件越堆越多，排障却越来越难：指标、日志、告警、变更各在一处，根因靠人肉拼接。商业方案常按主机或模块收费，数据还要上云。

本项目把常用能力收敛进**一套自托管平台**：

| 原则 | 做法 |
|---|---|
| **少组件** | 一个 Go 服务端 + 一个零依赖 Agent，覆盖 Zabbix / Prometheus / Grafana / Alertmanager / 剧本 / 终端 / 远程桌面的常用路径 |
| **快部署** | `docker compose up -d` 起全栈；面板一键生成 Agent 安装命令 |
| **数据自持** | PostgreSQL（关系 / 审计 / 向量记忆）+ VictoriaMetrics（时序），均可导出、可审计 |
| **AI 可插拔** | 接任意 OpenAI 兼容模型即智能模式；不接则启发式兜底，平台照样跑 |
| **闭环可度量** | 事件不只「有诊断」，还要 dry-run → 提案 → 批准 → 回验 → 沉淀；效果 KPI 可看板 |

---

## v0.19.0 亮点

相对 v0.18.9，本版本把「能诊断」推进到「能回验、能门禁、能学习」：

| 方向 | 内容 |
|---|---|
| **事件闭环** | 一键 `Dry-run → 提案 → 批准 → 回验 → 沉淀 Skill`；提案需证据闸门；回验检查主机/告警/修复/服务面；案例包导出 |
| **远程门禁** | 冻结窗或危急未决主机时，终端/桌面需变更或闭环授权；`remote-preflight` 统一预检；管理员 break-glass；会话审计挂 `change_id` / `incident_id` |
| **变更与服务** | 循环冻结窗（daily/weekly）；业务服务树与影响面；应急变更 SoD（作者不可自批） |
| **记忆强化** | 记忆带服务/主机类别作用域与 `verified`；检索加权已验证知识；结案/回验强化；UI 可按验证状态过滤 |
| **Agent 学习** | Hermes 多轮工具：高质量轮次才产 **draft** Skill；验证后人工激活；写工具默认审批 |
| **效果运营** | `GET /api/v1/sre/effect`：MTTR/MTTA、告警噪声、变更失败率、闭环率、AI 采纳/验证、Skill·记忆命中与 draft/active |

历史里程碑：`v0.18.9` ITSM 轻量工单/变更 · `v0.18.2` Windows 锁屏 Ctrl+Alt+Del · `v0.18.0` 安全可控闭环 · `v0.17` 资源层（Hyper-V/容器/K8s）。完整说明见 [Releases](https://github.com/sreyun/aiops-monitor/releases)。

POC 验收清单（可选）：[docs/year1-acceptance.md](docs/year1-acceptance.md)

---

## 能力一览

```
Observe          Govern           Remediate         Diagnose
可观测采集        告警治理          自动化自愈         AI 巡检诊断
─────────        ──────           ─────────         ──────────
主机/GPU/日志     静默·抑制·路由     剧本·审批闸门      流式诊断·拓扑 RCA
拨测/Redfish      多通道通知         自动修复护栏       RAG 记忆·Skill
NetFlow/存储      安全 finding       SLO·工单·On-call   WeKnora 外挂文档

Control          Data             Secure            Operate
远程操控          SQL 工具          安全合规           运营与部署
────────         ───────          ──────            ────────
终端·远程桌面     多源·EXPLAIN       RBAC·MFA·指纹     效果 KPI·备份
端口转发/跳板     SQL 变更闸门       安全中心·SSRF     Compose 一键
```

---

## 核心能力详解

### 1. 全栈可观测（Observe）

- **四平台原生采集**：Linux / Windows / macOS / 麒麟；Agent 纯 Go 标准库、**零第三方依赖**；CPU / 内存 / 磁盘 / 网络 / 负载 / 进程 / GPU（NVIDIA·AMD·Apple）等。
- **主动拨测**：HTTP（含 TLS 剩余天数）/ TCP / Ping / UDP / 进程 / OpenAPI / 分布式多点。
- **带外与流量**：Redfish（含华为 iBMC）、NetFlow v5/v9/IPFIX、华为 OceanStor。
- **日志**：Agent 增量采集，AES-256-GCM 加密上报，服务端全文检索。
- **主机深度巡检**（`host_inspect`）：OS / 内核 / 网卡 / 磁盘 / 服务摘要，可写入剧本步骤。
- **交互趋势图**：Canvas 十字线、框选放大、1h～30 天统一跨度。

### 2. 资源闭环（Resources）

- Hyper-V / Docker·Podman 容器 / Kubernetes（节点·Pod·Deployment·事件）自动或直连纳管。
- **全局资源搜索**：主机 / VM / 容器 / K8s 统一入口。
- **依赖拓扑**：边关系 + 自动发现，支撑爆炸半径与 RCA。
- **业务服务树**：绑定主机，查询未决事件与近期变更影响面。

### 3. 告警治理（Govern）

- 三档阈值预设（保守 / 标准 / 宽松）。
- **静默 → 抑制 → 路由**：严重走电话/短信，警告走飞书/钉钉等。
- 多通道：飞书 / 钉钉 / SMTP / 多云短信 + TTS；触发与恢复各推一次。
- 安全扫描 finding 可追踪、可取消、可闭环。

### 4. 自动化与 SRE 闭环（Remediate）

- **剧本**：Shell + 内置模块、预检、并发、条件变量、重试、逆序回滚、实时输出与审计。
- **高风险闸门**：定时高风险进入 `pending_approval`；命令白名单 / 危险命令拦截 / guardrails。
- **事件闭环**：告警 / SLO / 手动事件 → 时间线 → 认领 / 解决 / 升级；**一键闭环条**（dry-run / propose / approve / verify / promote）。
- **变更管理**：变更窗与循环冻结、应急变更 SoD、事件关联近期变更；冻结期内未授权自愈与远程强制闸门。
- **SLO**：多窗口多燃烧率；**On-call** 排班与超时升级。
- **工单**：事件升级、指派真实目录用户、图片/文件附件证据链。
- **ITSM 轻量**：服务请求 / 变更状态机、OpsLink、SQL↔变更单双向挂接。

### 5. AI 巡检诊断（Diagnose）

- 定时 / 手动健康巡检；critical 事件可自动诊断并写入时间线。
- **实时证据刷新** + 强证据闸门（弱心跳 alone 不足以放行提案）。
- **RAG 记忆**：pgvector；作用域（服务 / 类别）过滤；`verified` 加权；👍/👎 强化与惩罚。
- **Skill**：多工具高质量轮次生成 draft；回验/人工激活后参与检索；支持版本、作用域与客户包导入导出。
- **WeKnora** 外挂文档库；不可用时降级本地记忆/技能。
- 多模态助手：SSE、Function Calling、图片/文档/URL；Web 语音输入与朗读（浏览器支持时）。
- 模型可插拔；embedding / chat / rerank 解耦；AI Runs / Fallback / 工具轮次可观测。

### 6. 远程桌面与终端（Control）

- **远程终端**：Agent 反向隧道，免开入站；VT + 会话回放。
- **Web 远程桌面**：JPEG / H.264；多显示器；画质预设；文件传输与剪贴板。
- **Windows 锁屏**：服务 + 桌面 worker 跟随 Winlogon；Ctrl+Alt+Del（Session-0 SAS 管道，Agent ≥ v0.18.2）；解锁凭据不落盘。
- **远程门禁**：冻结或危急主机时需预检通过；管理员 break-glass 可审计放行。
- **端口转发 / `/proxy`**：跳板访问局域网服务。

### 7. SQL 工具箱（Data）

- 多数据源、Schema / 历史、EXPLAIN 对比。
- 高风险 SQL 走变更闸门；PostgreSQL 只读探查与受控运维动作（见 [docs/ci-gate.md](docs/ci-gate.md)）。

### 8. 安全合规（Secure）

- 会话 Cookie：PBKDF2-HMAC-SHA256（60 万次）；`HttpOnly` / `SameSite` / HTTPS 下 `Secure`。
- RBAC：admin / operator / viewer；可选 TOTP MFA。
- 终端 / 桌面二次密码；IP + 账户防暴破；机器指纹防克隆。
- 安全中心（主机 / Web 扫描）；出站 SSRF 守卫；`AIOPS_SECRET_KEY` AES-GCM 落库；可选 TLS。

### 9. 移动端与 Web

- **安卓 / 鸿蒙**：企业级原生控制台为**外部分发包**（本仓库不含移动端源码）；覆盖总览、告警、终端、SRE Hub 等。
- **Web / PWA**：深色专业后台；顶栏全局 AI；效果看板与记忆库 / Skill 管理；管理员「数据与备份」。

### 10. 部署韧性（Resilient）

- **双强制存储**：缺 PostgreSQL 或 VictoriaMetrics 即拒绝启动。
- Schema 版本化迁移；网关中继；Agent 多服务端广播（断路器 + gzip 降级）。
- 安装令牌轮换 + 7 天宽限；amd64 / arm64 预构建镜像。

---

## 架构概览

```
┌────────────────── 采集端（零依赖 Go Agent） ──────────────────┐
│ 四平台原生采集 · GPU · 日志加密上报                              │
│ 拨测 · Redfish · NetFlow · OceanStor · Hyper-V / 容器         │
│ 远程终端 · 远程桌面 worker（Windows 服务跟随 Winlogon）          │
│ 机器指纹 · Relay 中继 · 多服务端广播                             │
└───────┬────────────────────────────────────┬────────────────┘
        │ 上报 / 拨测 / 终端 / 桌面 / 转发      │ servers[] 广播
        ▼                                    ▼
┌──────────────────── 服务端（单 Go 二进制） ────────────────────┐
│ 告警 → 治理 → 事件闭环 → 剧本审批 → SLO → 工单 / 变更           │
│ AI 诊断 + RAG（作用域记忆 / Skill）· SQL · 安全中心             │
│ 远程门禁 · 拓扑 RCA · 资源搜索 · 效果 KPI · RBAC / MFA         │
│  ┌──────────── 双强制存储（缺一拒启动）────────────┐            │
│  │ PostgreSQL：关系 / 审计 / 事件 / 向量记忆 / Skill │            │
│  │ VictoriaMetrics：全部时序指标                     │            │
│  └──────────────────────────────────────────────┘            │
└──────────────────────────┬───────────────────────────────────┘
                           │ REST + WebSocket
                           ▼
              ┌──── Web 控制台 / 安卓 / 鸿蒙 ────┐
              │ 总览 · 主机 · 告警 · 终端 · 桌面   │
              │ SRE Hub · AI · SQL · 安全 · K8s   │
              └──────────────────────────────────┘
```

**分工**：高频采集用纯 Go Agent；带外协议由 Agent 远程轮询；Windows 锁屏桌面需 **LocalSystem 服务 + 会话内 worker**。

---

## 快速开始

### Docker Compose

| 文件 | 场景 |
|---|---|
| `docker-compose.yml` | **正式环境**：拉取预构建镜像；Agent 可选（`--profile agent`） |
| `docker-compose.dev.yml` | **开发 overlay**：本地构建；默认启 Agent |

**正式环境（推荐）**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh)
docker compose up -d
```

**开发环境（源码目录）**

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

浏览器打开 `http://localhost:8529`，默认 `admin / admin`。**首次登录强制安全初始化**（改用户名与密码），建议随后启用 MFA。

> `secure-compose.sh` 会生成强随机 `POSTGRES_PASSWORD` 与 `AIOPS_SECRET_KEY` 写入 `.env`（勿提交 Git）。拉不到默认镜像时，在 `.env` 改 `POSTGRES_IMAGE` / `VM_IMAGE`，或使用开发 overlay。

### 安装 Agent

面板右上角「安装 Agent」→ 选择系统 → 在目标机执行：

```bash
# Linux（root）
curl -fsSL "http://<服务端>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows（管理员 PowerShell）— 装为服务以支持锁屏远程桌面
irm "http://<服务端>:8529/install.ps1?token=<TOKEN>" | iex
# 升级后可重装服务：aiops-agent --install-service
```

服务端**强制依赖** PostgreSQL 与 VictoriaMetrics。更多方式见 [INSTALL.md](INSTALL.md)；Agent 全量配置见 [config.example.yaml](config.example.yaml)。

### Windows 远程桌面要点

1. Agent 必须以 **Windows 服务**（LocalSystem）安装。  
2. Agent ≥ **v0.18.2** 并执行 `--install-service`，启用 SAS 管道。  
3. Web 桌面：先 **Ctrl+Alt+Del**，再 **解锁**（凭据仅内存、审计不含明文）。  
4. 若被 Application Control 拦截，先放行二进制再装服务。

---

## 典型场景

| 场景 | 用法 |
|---|---|
| 中小型机房统一监控 | 单服务端纳管数百台跨平台主机，三档阈值开箱即用 |
| 告警风暴治理 | 静默 + 抑制 + 路由分流渠道 |
| 业务可用性 SLA | API 拨测 + SLO 多窗口燃烧率 |
| 故障自愈 | 告警触发剧本；高危动作卡审批 |
| 事件一键闭环 | 诊断有证据 → dry-run → 提案 → 批准 → 回验 → Skill |
| 变更窗应急 | 循环冻结 + 应急变更 SoD + 远程门禁 |
| 锁屏远程救援 | Windows 服务 Agent + Web 桌面 CAD / 解锁 |
| 智能根因 + 学习 | LLM + 拓扑 RCA + 作用域记忆 / 已验证强化 |
| 证据工单 | 事件升级 → 指派真实用户 → 附图评论 |
| 外出应急 | 原生安卓 / 鸿蒙控制台（外部分发） |
| 跨网段弱网 | 网关中继 + 多服务端广播 + 断路器 |

---

## 文档索引

| 文档 | 说明 |
|---|---|
| [INSTALL.md](INSTALL.md) / [INSTALL_EN.md](INSTALL_EN.md) | 安装、二进制直跑、反代、中继、升级卸载 |
| [USER_GUIDE.md](USER_GUIDE.md) | 安装使用说明书（功能与场景详解） |
| [DEPLOY_GUIDE.md](DEPLOY_GUIDE.md) | 部署进阶 |
| [FORWARD_GUIDE.md](FORWARD_GUIDE.md) | 端口转发 / 跳板 |
| [config.example.yaml](config.example.yaml) | Agent 全量配置示例 |
| [docs/ci-gate.md](docs/ci-gate.md) | CI / 安全与 SQL / AI / 闭环门禁说明 |
| [docs/year1-acceptance.md](docs/year1-acceptance.md) | POC 验收清单与效果 KPI 定义 |
| [Releases](https://github.com/sreyun/aiops-monitor/releases) | 版本发布说明 |

安卓 / 鸿蒙客户端为外部分发包，说明随发布渠道提供（本仓库不含移动端源码）。

---

## 诚实边界

**平台**

- 强制 PostgreSQL + VictoriaMetrics；单机建议规模约 3000 主机（更大建议外接 VictoriaMetrics）。
- 未配置大模型时启发式兜底，语义深度不及 LLM。
- Web 语音依赖浏览器 Web Speech API（Chrome / Edge 体验最佳）。
- 工单附件以 JSON 快照持久化，适合证据级材料，非对象存储替代品。

**远程桌面**

- Windows 锁屏 / 注销 / UAC 依赖 Agent **服务安装**。
- 安全桌面强制 GDI/JPEG；帧率受网络影响。
- macOS / Linux 受系统权限与图形会话限制。
- 「解锁」仅发送内存凭据——仅用于受信运维场景。

**移动端**

- 私有化 APK / 应用分发，不上架应用商店；账号自服务（改密 / MFA 绑定等）仍在 Web。
- 会话 Cookie 普通 DataStore 持久化；固定轮询，未接 FCM。

---

## 企业服务

本体 MIT 开源，可自由自托管。可在开源版之上提供：

- 大规模私有化部署咨询（分片、外接时序、保留期）
- 定制集成（企微 / 钉钉 / 飞书、CMDB、内部大模型网关）
- 安全合规加固（SSO / LDAP、审计留存、等保建议）
- 安卓私有化签名与分发托管

合作需求请在 GitHub / Gitee 提 Issue，或联系维护者。

---

## 开源与社区

- **协议**：MIT —— 无功能阉割、无用户数限制、无遥测。
- **规模（约）**：服务端 `cmd/server` 290+ Go 文件，Agent `cmd/agent` 120+ 文件，**150+** 自动化测试。
- **双远端**：GitHub 与 Gitee 同步分支与标签。
- **欢迎贡献**：Issue、PR、文档与插件。

| 资源 | 链接 |
|---|---|
| GitHub | <https://github.com/sreyun/aiops-monitor> |
| Gitee | <https://gitee.com/bigdatasafe/aiops-monitor> |
| Releases | <https://github.com/sreyun/aiops-monitor/releases> |

---

## License

MIT — 可自由使用、修改与再分发；详见仓库声明与发行说明。
