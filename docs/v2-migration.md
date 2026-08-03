# Web v2（Vue）迁移说明

## 切流策略

| 入口 | 行为 |
|------|------|
| `/` | 默认 Vue SPA（`cmd/server/web/v2/index.html`，Hash 路由 `#/...`） |
| `/?ui=legacy` | 经典 JS UI（`web/index.html`） |
| `/?ui=legacy#view` | 经典深链，如 `#hosts`、`#alerts`、`#sql-toolkit`、`#security-overview` |
| `/?ui=legacy#dashboard/{id}` | 打开指定看板 |
| `/?ui=legacy#settings` | 打开设置弹层 |
| `/v2/` | Vue 静态资源（Vite `base=/v2/`） |

Vue 顶栏「经典版」会按当前路由生成对应深链（见 `frontend/src/shared/legacy.ts`）。

## 技术栈

- Vue 3.5 + TypeScript + Vite
- Element Plus + Vue Router + Pinia + TanStack Vue Query
- ECharts（`vue-echarts`）
- `@xterm/xterm` 终端 Dock
- `fetch`（无 Axios）；列表信封用 `asArr` / `objectToRows` 归一化

源码目录 `frontend/` 本地构建，产物嵌入 `cmd/server/web/v2`（`//go:embed all:web`）。

## 路线图完成度（自测口径）

### Commercial delivery（本轮）

- [x] 深色优先 + 经典色板（accent `#4c8dff`、侧栏 236px、顶栏毛玻璃、内容区 padding）
- [x] Settings 通知通道补齐（alerts_enabled / webhook / SMS / 语音 / 测试推送）+ 角色门控
- [x] Security：AI 工具审计、审计外发
- [x] Overview：WS KPI、插件事件、健康条；Hosts：卡片/列表+分页+能力开关
- [x] Alerts：阈值内嵌编辑、治理行内编辑；Dashboard 不支持类型降级 + 撤销
- [x] 监控 Tab 内嵌原生 APIMon / Scrape；Token 遮罩；Markdown XSS 加固
- [x] `vue-tsc` 零错误 + production build + `:8529` 冒烟（未登录 API 401）

### IA parity（本轮）

- [x] 侧栏对齐经典：首页 + 可观测 / 告警响应 / 运维工具 / 安全 / 系统
- [x] 文案对齐：首页、监控、仪表盘、编排、SRE 中枢、安全中心、审计日志、SQL 工具
- [x] 监控子 Tab：拨测 / API 业务监控 / 采集配置；资源含 K8s；告警含子 Tab 治理/阈值
- [x] 顶栏：安装 Agent、AI 对话、主题（用户菜单）、快捷键 1–6、侧栏分组折叠与徽章
- [x] Viewer 隐藏安全中心

### i18n（本轮）

- [x] 词库拆到 `frontend/src/i18n/messages.ts`（zh-CN / zh-TW / en 同构）
- [x] 路由 `meta.titleKey` + 侧栏分组 / 面包屑随语言切换
- [x] Element Plus `el-config-provider` 随 `aiops-lang` 同步（zh-cn / zh-tw / en）
- [x] 高流量与次级页面全面 `t()`；APIMon / Scrape 经典桥接
- [x] `document.documentElement.lang` 随语言切换

### IA parity（本轮）

- [x] 侧栏分组与顺序对齐经典：`首页` → 可观测 → 告警响应 → 运维工具 → 安全 → 系统
- [x] `/apimon`、`/scrape` 并入 ChecksView Tab；`/k8s` 重定向至 `/resources?tab=k8s`
- [x] 终端 / 桌面 / Hermes 移出侧栏；Hermes 顶栏入口；终端 Dock 保留
- [x] 顶栏：安装 Agent 对话框、主题切换（用户菜单）、AI 按钮、快捷键 1–6
- [x] 侧栏折叠分组持久化 `aiops_nav_group_collapsed`；hosts/alerts/log 徽标
- [x] 三语 `layout.*` / `nav.*` / `install.*` / `checks.tab*` / `resources.tabK8s` 更新

### Phase 6（本轮）

- [x] Security：Content Audit（主机选择 / 事件筛选 / 敏感词配置）
- [x] Settings：完整 Thresholds 分组 + conservative/standard/relaxed 预设
- [x] Resources：硬件详情抽屉（events + history）
- [x] Network：NetFlow Flows（hosts / summary top-N / flow 明细）
- [x] Dashboards：面板 resize 手柄（E/S/SE）+ Compact 压缩布局

### Phase 5（本轮）

- [x] Dashboards：模板变量（custom/query/constant/textbox）+ 面板拖拽定位
- [x] Alerts：Governance 抑制规则 + 通知路由（与静默同抽屉）
- [x] Security：Web/Nuclei 目标 CRUD、触发扫描、任务详情/取消
- [x] Network：SNMP 主机/设备列表、Trap 流、设备删除
- [x] 修复：`v-loading` 需读 `query.isLoading.value`（vue-query Ref 不解包）

### Phase 4（本轮）

- [x] Dashboards：Grafana 导入（ID/JSON）+ AI 生成（job 轮询）
- [x] Alerts：Governance 静默规则编辑
- [x] Settings：OIDC SSO 配置 + AI usage 成本曲线/按用户
- [x] Security：主机扫描触发 + 任务列表 + 主机摘要
- [x] Login：OIDC SSO 按钮（启用时显示）

### Phase 3（本轮）

- [x] Hermes `/hermes`：会话、建议提问、SSE 流式、额度错误可读化
- [x] 登录修复：登录后强制 refreshUser；强制改密表单
- [x] Settings：语音 STT/TTS + 测试播报；Users CRUD；账号改密
- [x] Hosts：分组增删改 + 主机归组
- [x] SRE：Duty 晨报上下文、Incident Loop（dry-run/propose/approve/verify/promote）
- [x] 性能：Element Plus 按需、xterm/echarts 懒加载、KeepAlive/预取

### Phase 2（本轮）

- [x] Remote Desktop：`/desktop` JPEG 画布、质量预设、上传/下载、会话回放
- [x] Automation：可视化步骤 + YAML、执行日志抽屉、取消、主机体检
- [x] Terminal Dock：搜索、命令历史、文件上传/下载（0xFF/0xFE 帧）
- [x] `vue-tsc` 零错误 + embed 构建

### Phase 1（本轮已收口）

- [x] `npx vue-tsc -b --noEmit` 零错误（`@types/node` 已在 `package.json`）
- [x] Overview：刷新脉冲 / 空状态
- [x] Hosts：详情 Drawer（metrics）/ 批量删除
- [x] Alerts：批量 ack·silence·clear / 高级筛选抽屉
- [x] Dashboard 面板：timeseries / gauge / **stat** / **table**
- [x] Settings：AI + **Notify**（飞书/钉钉/SMTP）+ **Thresholds**
- [x] API 模块封装起步：`src/api/modules.ts`

### Week 1–2 Foundation — 已完成

- Layout 分组导航、折叠、主题、用户菜单
- 鉴权：`/me` bootstrap、登录/MFA 提示、登出
- API client：`ApiError`、cookie session、`asArr`/`num`/`objectToRows`
- 经典深链桥接：`legacyHref` / `LegacyBridgeView` / 顶栏按路由跳转

### Week 2–5 P0 — 可用（日运维主路径）

| 页 | Vue 能力 | 经典版保留 |
|----|----------|------------|
| 总览 | KPI 可点跳转、CPU/Mem/Disk TOP、告警/活动 | 健康卡插件事件、WS 推送 |
| 主机 | 分组树 + 列表 + 终端 Dock | 卡片视图、桌面、文件夹 CRUD |
| 告警 | `level` 筛选、status、搜索、ack/silence/clear **带 scope**、**静默/抑制/路由治理** | 阈值子页等 |
| 设置 | AI 保存/测试、巡检间隔、主题、账号 | 通知/用户/OIDC/完整配置 |

### Week 5–8 P1 — 并跑验收

| 页 | Vue | 经典 |
|----|-----|------|
| 看板 | 列表/新建/编辑、timeseries/gauge/stat、**变量**、**拖拽**、Grafana 导入、AI 生成 | 经典完整编辑器/ZMODEM 等 |
| 终端 Dock | 多会话、resize `0x72`、心跳 | ZMODEM、预检门控 |
| SRE | Assist SSE、feedback、事件 ack/resolve/diagnose、**Duty 上下文**、**Loop 动作**、On-call/Changes | 完整编排 UI、晨报一键生成 |
| 设置 | AI + **Speech 测试** + Notify + Thresholds + **Users** + 改密 | OIDC / 完整配置项 |

### Week 8–12 P2 — 按用量

- **可用**：拨测、自动化剧本、转发、数据源、SQL 工作台（美化/审计/分析/查询）
- **可用（摘要+操作）**：安全 KPI + 主机扫描 + **Web/Nuclei** + **内容审计**；网络队列 + **NetFlow Flows** + **SNMP**；资源硬件详情/Hyper-V；K8s 概览；日志筛选；事件流
- **经典桥接保留**：Agent 侧 SNMP OID 配置、ZMODEM 全协议、内容审计超深筛选等

### Week 12–14 切流 — 服务端已完成

- 默认 `/` → Vue；`/?ui=legacy` 保留
- React 构建链已移除（现为 Vue）
- 本文档作为发布说明基线；CHANGELOG 同步更新

## 并跑验收清单（P0/P1）

- [x] 登录后进入 Vue 壳，刷新不掉会话
- [x] `/v2/assets/_plugin-vue_*` 返回 200（`all:web` embed）
- [x] 总览 KPI → 主机/告警跳转正确
- [x] 主机树切换分组，过滤列表；打开终端可输入
- [x] **主机 Agent 舰队更新**：更新模式栏 / 选中落后 / 确认推送 / 任务轮询 / 失败回滚
- [x] **顶栏消息中心**：列表、未读角标、单条/全部已读、跳转 view
- [x] 告警 ack / silence / clear（含带 scope 的磁盘告警）
- [x] 设置 AI：保存 + 测试连接
- [x] **设置账号**：个人资料、MFA 启用/关闭、强制改密；管理员备份与全局 MFA / 重置用户 MFA
- [x] 登录 `require_mfa_setup` → Vue `/settings?tab=account`（不再跳经典版）
- [x] 看板：新建 → 编辑面板 → 保存 → 查询出图（含 alertlist / bargauge / logs）
- [x] SRE：Assist 流式输出；值班「生成晨报」；SLO / 工单；Helpful/Applied 需有 `assist_id`
- [x] 登录：用户名/手机号切换；忘记用户名/密码恢复流程
- [x] 全局搜索：经典 view id 正确映射到 Vue 路由（hardware/hyperv/containers/k8s 等）
- [x] 主机详情：历史趋势图（1h/6h/24h/7d）+ 分类编辑；重复主机清理
- [x] 远程桌面：编码切换 / CAD / 剪贴板 / 全屏
- [x] 资源：Hyper-V 电源操作；容器 start/stop/restart/logs
- [x] 顶栏「经典版」落到正确 `#view`（`legacyHrefForRoute`）
- [x] `/?ui=legacy` 经典功能不受影响

## 商业级交付核对（本轮已落地）

| 能力 | Vue 入口 | 关键 API |
|------|---------|---------|
| Agent 舰队更新 | 主机 → 更多操作 / 更新栏 | `/agent-dist/manifest`, `/agents/update`, `/agents/update/jobs` |
| 消息中心 | 顶栏铃铛 | `/messages`, `/messages/read`, `/messages/read-all` |
| 账号 MFA / 资料 | 设置 → 账号 | `/profile`, `/mfa/setup|enable|disable` |
| 备份 | 设置 → 备份（管理员） | `/admin/backups`, `/admin/backup-config` |
| 值班晨报 | SRE → 值班 | `/ai/duty-context` + `/ai/assist` `task=duty_report` |
| SRE SLO / 工单 | SRE → SLO / 工单 | `/slos`, `/tickets` |
| 看板扩展面板 | 看板编辑器 | `/alerts`, `/dashboards/query-logs`, instant query |
| 主机历史图 | 主机详情抽屉 | `/hosts/{id}/history` |
| 远程桌面增强 | `/desktop` | Desktop WS `Q/H/C/A` |
| 资源控制 | 资源 → Hyper-V / 容器 | `/hyperv/.../power`, `/containers/.../action|logs` |
| 登录恢复 | 登录页 | `/account/recover-*` |
| 全局搜索映射 | Ctrl+K | `/resources/search` → Vue path |
| 多 Provider SSO | 设置 → SSO；登录按钮 | `/auth/sso/config|info|identities` |
| AI RAG/MCP/WeKnora/MoA | 设置 → AI | `/ai/config` 扩展字段 |
| SRE 拓扑 / 修复 | SRE → 拓扑 / 修复 | `/topology/*`, `/remediation/*` |
| K8s 控制面 | 资源 → K8s | scale/restart/undo/apply/delete/log |
| 安全情报源 | 安全 → Feeds | `/security/feeds*` |
| Agent 自动更新策略 | 设置 → Agent | `/agents/auto-update-policy` |
| 看板 pie/bar/heatmap | 看板编辑器 | query / queryInstant |
| 主机预测叠加 | 主机详情 → 预测开关 | `POST /metrics/forecast` |
| 剧本模块/定时/版本 | 编排 | schedule + `/revisions` + preflight |
| Ops 动作计划 | Assist / Hermes 一键执行 | `/ops/actions/validate|apply` |
| Hermes 附件/语音 | Hermes / SRE Assist | `/hermes/chat` + `/ai/speech/*` |
| 看板冷门图型 | 看板编辑器 | candlestick/radar/sankey/state-timeline |
| K8s pod exec | 资源 → K8s | `POST .../pods/.../exec` |
| 顶栏经典版深链 | 顶栏「经典版」 | `legacyHrefForRoute` → `/?ui=legacy#view` |
| Status Page 配置 | 设置 → 状态页 | `/admin/status-page` |
| AI A/B 实验 | 设置 → AI | `/ai/experiments*` + `active_experiment_id` |
| SQL 运维观测 | SQL → 运维/慢查询/变更 | processlist / locks / slow-sql / change-requests |
| Scrape 规则 / Write | 指标抓取 → 规则 / Write | `/prom-rules*` `/prom/write-config` |
| ApiMon 接口编辑 | API 监控 → 系统抽屉 | endpoints CRUD in upsert |
| 主机安全详情/配置 | 安全 → 扫描任务 / 配置 | host scan detail + findings + `/security/host/config` |
| 深链映射对齐 | 搜索 / 消息 / 经典版链接 | `view-routes.ts` + tab sync |

### 已知保留（经典桥接 / 后续加深）

- 终端原生 ZMODEM（rz/sz）全协议（自定义上传/下载帧已可用；Terminal 页有经典版入口）
- K8s 交互式长连接 exec（短命令 `POST .../exec` 已可用）
- 内容审计超深筛选 / Agent 侧 SNMP OID 细配（网络页「OID 查询（经典版）」桥接）

### Classic Parity Waves（多轮还原）

- [x] **Wave 0**：默认浅色主题（对齐经典 `theme-init`）+ EP 密度压制 + AppLayout 顶栏/侧栏/Dock 偏移
- [x] **Wave 1**：Overview KPI/健康条压缩；Hosts 卡片密度/在线点/点击详情；Terminal Dock token 化；Desktop 舞台占满视口；远程闸门 break-glass 修复
- [x] **Wave 2–3**：Checks/Alerts/Logs/Resources/Network 工具条与空态密度；Checks 高级字段（json_path/json_expect/keyword_is_regex/cert_warn_days）保存与启停不丢；阈值/OID 经典入口为可选高级链接
- [x] **Wave 4–6**：Dashboard/SRE/Automation/Forward/Datasource/SQL/Security/Events/Settings/Hermes 密度与抽屉底栏保存/测试
- [x] **Wave 7**：ZMODEM / K8s 交互式长 exec / OID 深配仍经典桥接（Terminal/K8s/Network 页入口明确）

### UX Overhaul（1A + 2C）

- [x] **F0 共享底座**：`api` 401→登录、错误字段/timeout/Abort；Vue Query mutation 默认错误；App 错误边界；StateView 骨架；FilterBar / ConfirmDialog / useQueryState；realtime 降级条；冷色 accent-2
- [x] **F1 核心路径**：Overview 真错误≠引导空态；Hosts FilterBar/筛选空态/批删确认；终端仅激活会话保活；桌面重连不重复 POST；break-glass 预检失败不伪装；Alerts clear 确认 + Checks 按行 loading
- [x] **F2 侧栏收敛**：Forward/Datasource/Terminal 等主列表 StateView；其余页沿用既有密度与错误 toast

### Design System + 业务态补齐（2026-07）

- [x] **配色对齐经典**：暗色 `#4c8dff` / 浅色 `#1677ff`；墨色侧栏；去掉 cyan 模板色与浅色方格底纹
- [x] **壳层**：AppLayout 激活轨/徽章用 token；Login 大气晕阴影；StatCard / SectionCard / PageHeader 统一层次
- [x] **StateView 收敛**：Network / SRE / Automation / Security / Settings / K8s / Scrape 主列表；配置对象查询 `:empty="false"`（避免表单被误判空态）；`#header` 归还 `el-card`
- [x] **Hermes**：状态/会话错误告警 + 重试；顶栏刷新
- [x] **已知保留**：K8s Nodes/Events 深页、ZMODEM 全协议、OID 细配 → 经典桥接入口

## 本地构建

```bash
cd frontend && npm install && npm run build
go build -o bin/aiops-server.exe ./cmd/server
```

环境变量与经典版相同（Postgres / VictoriaMetrics / `AIOPS_SECRET_KEY` 等）。
