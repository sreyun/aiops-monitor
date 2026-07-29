# 变更日志

本文件记录 AIOps 公开发布版本的重要变更。

**版本线说明（仅保留有序 v0.x）**

- 历史里程碑：`v0.1.0`–`v0.15.0`（由原 v6.x 序号重置而来，仅保留 v0.x tag）
- 正式发布线：`v0.16.0` → `v0.16.8` → `v0.17.0` → `v0.18.0` → `v0.18.9` → `v0.19.0` → …
- 中间补丁已合并归档：`v0.16.1`–`v0.16.7` → **`v0.16.8`**；`v0.18.1`–`v0.18.8` → **`v0.18.9`**；`v0.19.1`–`v0.19.34` → **`v0.19.35`**
- 基线 tag **`v0.19.0`** 保留

---

## [Unreleased]

### 变更

（无）

---

## [v0.19.44] — 2026-07-29

### 变更

- **SQL 查询结果表**：限宽 + 横向/纵向滚动；多字段时单元格省略，悬停看全文。
- **EXPLAIN 详细分析**：在 EXPLAIN JSON 下方输出逐步解读、风险问题、索引 DDL 建议与优化建议（结合元数据）。
- **查询历史不再脱敏**：完整保留字面量，便于重新打开与复跑；慢 SQL 识别 DIGEST `'?'` 并强化从语句历史/slow_log/PROCESSLIST 还原真实参数。
- **Agent 运行日志**：全平台写入安装目录 `agent.log`（桌面 worker 为 `agent-desktop.log`），7×10MB 滚动覆盖；安装脚本避免与同一文件双写冲突。
- **Windows Agent 控制台中文重复**：stderr 改走 `WriteConsoleW`，修复 CP65001 下 CJK「已已加加载载」重复；安装自检日志恢复正常。
- **剧本执行体验**：弹窗标题不再拼出「执行执行中」；轮询延长至约 30 分钟；主机状态逐步从「等待中」→「执行中」并逐步回写步骤。
- **剧本性能**：系统巡检模板改为单步 `host_inspect(quick)`；Windows CIM 磁盘/内存/CPU/型号/开机时间一次 PowerShell 批采 + 20s 缓存；跳过缓慢的 `wmic product`；各只读模板超时收紧。
- **SQL 工作台运行查询**：支持只读运行 SELECT/WITH/SHOW，结果表格展示；拆分 **执行时长**（`exec_ms`）与 **数据返回时长**（`fetch_ms`）；默认限 200 行；Ctrl/⌘+Enter 快捷运行。
- **OpenAPI 导入公共鉴权**：可展开配置公共请求头 / 请求体，写入业务系统级 `common_headers` / `common_body`。
- **OpenAPI 导入方法筛选**：支持全部 / GET / POST / PUT / DELETE / PATCH / HEAD（可多选），默认仅 GET。
- **SQL EXPLAIN 规范化**：去除内置函数名非法引号；**去掉 `SUM (` / `MAX (` / `CAST (` 等与括号间空格**（避免 MySQL Error 1630）；规范化 `COUNT(*)`；日期格式探测值；失败回传实际语句。
- **慢 SQL 列表体验**：汇总卡片（条数/最高均耗/累计耗时/执行次数/类型分布）+ 搜索 + 类型筛选（查询/写入/更新/删除）+ 耗时等降序排序 + `tblPager` 分页；默认按平均耗时降序。
- **OpenAPI/Knife4j 文档导入**：支持直接填 `doc.html#/home`（含 Knife4j 3.0.x 网关聚合页），服务端自动探测 `/v3/api-docs/swagger-config`、`/swagger-resources`、`/v3|/v2/api-docs` 并拉取 JSON；多分组可选（含 `#/SwaggerModels/<group>` 自动选中）；网关 `contextPath` 自动回填基址；切换分组同步系统名与基址。
- **剧本多系统适配**：Agent 只读模块 `disk_usage` / `mem_info` / `cpu_load` / `pkg_list` 与 host_inspect 磁盘/内存/CPU 采集改为 Windows CIM/PowerShell 优先（兼容无 wmic 的 Win11/新 Server），失败再回退 wmic；macOS 磁盘改用 `df -hP`。
- **安装 Agent / 破窗确认 UX**：原生 `confirm` 改为应用内确认对话框；Token 策略区默认折叠，过期改用本地日期时间选择；远程闸门管理员强制放行文案可读化；慢 SQL 限额写入共用同一确认组件。
- **慢 SQL 完整还原**：多源捞最长 `SQL_TEXT`（history_long/history/current/slow_log/processlist）+ 本平台 DIGEST 全文缓存；报告探测 `max_digest_length` / SQL_TEXT 限额并提供复制调参 / 确认后 SET PERSIST；截断态提供粘贴全文入口。无法编造缺失尾部。
- **AI 设置 UX**：信息架构纠偏（记忆/防御/进化迁至安全 Tab）；Provider/RAG/研判/安全统一可折叠卡片与摘要；侧栏状态点；保存默认不关窗 + 脏状态确认；文案与预设高亮减噪。
- **MCP Clients**：AI 设置 → 集成支持接入外部 MCP Server（Streamable HTTP）；工具桥接为 Sreyun `ext_*`，用于 AI 对话、诊断预取与创建看板；危险名工具默认拦截，支持 allow/block list；提供测试连接与同步工具 API。
- **MCP Server**：保留对外暴露能力；集成页文案改为「MCP 集成」双向说明。

---

## [v0.19.43] — 2026-07-29

### 变更

- **企业级治理补齐**：公开 Status Page（`/status` + JSON API）、Playbook 版本快照/diff/还原、工单 SLA（时限 + OnCall 自动指派 + 违约巡检）、备份远程 S3/OSS 上传、配置密钥自动轮换（密钥库 + 定时/手动 ROTATE）。
- **AI 治理增强**：输出层 ops action 白名单校验；TaskModels 智能路由；A/B 实验 CRUD UI + 变体模型覆盖；AI 成本 TCO 看板；高风险助手 MoA/SelfVerify 编排对齐。
- **数据与安全**：`ai_call_events`/`audit_log`/`events` 按月分区；审计链 HMAC + 校验 API；主机趋势/多图表竞态与预测确定性修复。

---

## [v0.19.42] — 2026-07-29

### 变更

- **Linux Agent 安装**：systemd `ProtectHome` 调整为 `read-only`（可读 `$HOME` 供远程终端，同时阻止对 home 的写）；Agent 重装/卸载不再误清网关服务 `aiops-relay`。
- **Windows Agent 自动更新**：Windows 更新一律进入 `pending_verify` 直至版本 ACK；soft-retry 拉长至 180s；不可比对目标不再卡在 `pending_verify`。

---

## [v0.19.41] — 2026-07-29

### 变更

- **仓库治理**：从 Git 历史中彻底清除 `harmony/`、`.android-publish/`（及 `android/` / `.android/` 路径）；本地工程目录保留且继续被 `.gitignore` 忽略。已对远端 `master` 与 tag 强制同步。

---

## [v0.19.40] — 2026-07-29

### 变更

- **仓库**：将 `harmony/`（鸿蒙 App）移出版本库（与 `android/` 一致），本地可保留；推送后远端工作树不再包含该目录。

---

## [v0.19.39] — 2026-07-29

### 变更

- **Windows Agent 自动更新**：替换重启支持服务安装与一键用户态安装（VBS / 计划任务 / `--config`）；拒绝无配置裸启；多服务名与 Win2012 进程名覆盖；按路径兜底杀进程。
- **更新校验闭环**：`restart scheduled` 进入 `pending_verify`，待上报 `agent_version` 后再标成功；超时标失败并缩短 soft-retry，便于二次推送。
- **更新通道**：允许同源 http↔https（TLS 升级/重定向）作为合法更新源；遗留 Windows 更新脚本与模块侧重启路径对齐。
- **文档 / 缓存**：README 多语言去掉旧主版本号表述；Service Worker 缓存升至 `v0.19.39`；源码注释清理残留 v6.x 版本标记。
- **仓库治理**：删除错位 tag（`v0.18.5`/`v0.18.8`）及已归档中间 GitHub Release，远端仅保留有序 v0.x。

---

## [v0.19.38] — 2026-07-29

### 变更

- **Linux Agent / 远程终端**：安装与重装时完整清理残留 systemd 单元及 `*.service.d` drop-in，避免旧 `ProtectHome` / `CapabilityBoundingSet` 覆盖导致 `fork/exec bash: permission denied`。
- **systemd 硬化**：默认 `ProtectHome=false`，SNI/内容审计仅提升 ambient 能力、不再收窄 CapabilityBoundingSet；交互 shell 增强 cwd 可用性检测并回退到可写目录。

---

## [v0.19.37] — 2026-07-29

### 变更

- **AI 设置 · 语音**：语音输入/播报配置区新增「测试语音」按钮；未保存也可按表单配置自检。
- **语音闭环**：`POST /api/v1/ai/test-speech` 合成样例并返回音频供浏览器播放；若配置了 STT，则对同段音频做识别回环验证。

---

## [v0.19.36] — 2026-07-29

### 变更

- **MCP**：`GET/POST/DELETE /api/v1/mcp` 升级为 Streamable HTTP（JSON-RPC + SSE），兼容 Cursor / Claude 等客户端。
- **MCP 工具**：暴露值班/诊断等只读研判工具（`get_duty_context`、`diagnose_incident`、`run_assist_task`、`run_diagnostic`、`analyze_dashboard`）；新增 scope `sre` / `ai`；补充 prompts/resources；不再暴露 `propose_skill` / `remember_preference`。
- **AI 设置 · 集成**：支持作用域令牌、每分钟限流配置，以及一键复制客户端 MCP 配置。
- **质量**：新增 `handleMCP` HTTP 集成测试，更新 `docs/ci-gate.md`。

---

## [v0.19.35] — 2026-07-29

相对 **v0.19.0** 的累计发布（含原 v0.19.1–v0.19.34 全部内容）。

### 亮点摘要

- **AI 能力**：对话闭环调度看板/诊断/导出；附件预览与语音；对话内图表下钻与永久保留/导出；看板生成韧性与多模型预测；AI 设置入口全局可发现（用户菜单 / 告警设置 / 对话窗），非管理员只读。
- **Windows / Agent**：安装上报、自动更新、App Control、http→https 重定向、Win2012 终端乱码/PATH/回显与远程桌面闪屏等一批生产修复；Android 目录移出版本库。
- **安全与基础设施**：主机安全 FIM、威胁情报统一通道、Web 扫描；K8s 探活与 Endpoint 展示；网关中继与多服务加固；Agent 批量远程更新。
- **观测与 UI**：趋势预测自学习与开关策略；趋势图时间切换抖动修复；前端设计系统深度优化；容器库存 `updated_at` 统一为 Unix 秒。

### 按原版本归档（便于对照）

| 原版本 | 变更要点 |
|--------|----------|
| v0.19.1 | 修复 Grafana 大模板导入与仪表盘 AI 应用 |
| v0.19.2 | 深度适配国产与国际 OS 矩阵，完善 ARM/Hyper-V 与巡检编排兼容 |
| v0.19.3 | Agent 批量远程更新与网关中继商业级加固 |
| v0.19.4 | 主机安全文件完整性监控（FIM）与内容差异 |
| v0.19.5 | 加固 K8s 集群探活与配置编辑体验 |
| v0.19.6 | 修复 Agent 自动更新重启导致终端/远程桌面失效 |
| v0.19.7 | 修复 nologin 服务账号导致 Web 终端不可用 |
| v0.19.8 | K8s Endpoint 显示真实地址并修复 Windows Agent 自动更新 |
| v0.19.9 | 加固 Windows 安装绕过 App Control，主机分类省略显示 |
| v0.19.10 | 安装页自动更新默认开启并加固网关中继/多服务，修复终端桌面与 FIM |
| v0.19.11 | 威胁情报源统一更新通道，主机/Web 安全增强与列表布局修复 |
| v0.19.12 | 修复 Web 终端滚动条遮挡底部提示符与新输出 |
| v0.19.13 | 修复 Windows Agent 安装后不上报与静默失败 |
| v0.19.14 | AI 对话闭环统一调度看板/诊断/导出与反馈 |
| v0.19.15 | 修复 Windows 10/11 与 Server 安装后主机不上报 |
| v0.19.16 | 修复 Web 扫描情报源点击无反应 |
| v0.19.17 | AI 对话增强看板链接、附件预览、可配置语音与对话内图表下钻 |
| v0.19.18 | 修复 Agent http→https 重定向注册 404；增强 AI 界面调度/看板组件/安全防御与自我进化 |
| v0.19.19 | 修复 Windows 终端/远程桌面因 http→https 流式通道卡住无法接入 |
| v0.19.20 | 修复终端长输出后输入区不可见；主机列表默认按 IP 升序 |
| v0.19.21 | 前端设计系统深度优化（全局配色/间距/动效/AI 交互/组件统一） |
| v0.19.22 | 增强 AI 看板生成韧性与多模型预测；默认关闭预测开关 |
| v0.19.23 | 修复看板预测关闭后仍预留未来轴；接入预测自学习调校 |
| v0.19.24 | 补齐 AI 编排依赖，修复 release 构建编译失败 |
| v0.19.26 | 修复 Win2012 Agent 构建（无效 x/exp 版本与 slog API） |
| v0.19.27 | 修复趋势图时间切换抖动与 AI 优化看板无法应用 |
| v0.19.28 | 将 Android 目录移出版本库 |
| v0.19.29 | 修复 Win2012 远程终端乱码与提示符阶梯错位 |
| v0.19.30 | 修复 Win2012 终端 PATH 缺失与管道无回显 |
| v0.19.31 | 彻底修复 Win2012 终端 Path 大小写与输入回显 |
| v0.19.32 | 修复 AI 看板趋势图因 node_* 指标导致大面积空白 |
| v0.19.33 | 修复 Win2012 桌面闪屏重连、终端滚动隐藏输入；强化 Agent 自动更新 |
| v0.19.34 | AI 对话图表永久保留与导出；清爽化输入栏与 README 多语言入口 |
| v0.19.35 | 提升 AI 设置可发现性；容器库存 `updated_at` 统一为 Unix 秒 |

> 另含文档与杂项：`docs: 深度完善 README 对齐 v0.19.0`、`README/LICENSE` 重构、以及一次整包入库整理提交。

---

## [v0.19.0] — 基线（tag 保留）

远程门禁、事件闭环与作用域记忆强化：补齐诊断证据闸门/回验学习、Hermes draft 质量门与 AI 可观测，并深度加强记忆作用域、已验证强化与检索 UI。

---

更早版本请参阅对应历史 tag（如 `v0.18.9`、`v0.16.8`）与 git 提交记录。
