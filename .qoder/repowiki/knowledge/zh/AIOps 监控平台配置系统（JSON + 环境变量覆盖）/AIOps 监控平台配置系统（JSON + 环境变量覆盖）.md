---
kind: configuration_system
name: AIOps 监控平台配置系统（JSON + 环境变量覆盖）
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/agent/main.go
    - cmd/agent/embed.go
    - config.example.json
    - server_config.example.json
---

## 系统概述

AIOps 监控平台采用 **轻量级 JSON 配置文件 + AIOPS_* 环境变量覆盖** 的配置体系，Server 端提供 ConfigStore 内存态管理、持久化与热更新能力，Agent 端通过命令行参数与 JSON 文件加载配置。整体设计遵循“默认值 → 示例文件 → JSON 文件 → 环境变量/CLI 参数”的优先级覆盖模型。

## 核心架构与组件

### Server 端配置系统 (`cmd/server/config.go`)
- **ConfigStore**：线程安全的配置存储封装，持有 `ServerConfig` 内存副本，支持并发读写（`sync.RWMutex`）
- **双后端持久化**：优先从 PostgreSQL 读取（`pgStore.loadConfigBlob`），回退到本地 JSON 文件（`server_config.json`），写操作统一经 `save()` 完成
- **运行时加密**：在 `save()` 前调用 `encryptConfigSecrets()` 对敏感字段（SMTP/SMS/VoiceCall SecretKey、AI API Key、RelaySecret 等）进行可逆加密；启动时 `decryptConfigSecrets()` 解密为明文供运行使用
- **默认值回填**：`backfillThresholdDefaults()` 自动将未配置的阈值字段填充为标准默认值，保证告警引擎始终有安全阈值可用
- **迁移机制**：`migrateUsers()` 将旧版单用户 `Account` 迁移为新版多用户 `Users` 列表，并保证至少存在一个 admin 账户
- **撤销能力**：每次 `Set()` 前快照当前配置到 `prev`，支持 `Revert()` 回滚最近一次变更

### Agent 端配置系统 (`cmd/agent/main.go`) 
- **配置加载顺序**：`defaultConfig()` → 解析 `--config` 指定的 JSON 文件 → 命令行 flag 覆盖
- **多服务端上报**：支持 `servers` 数组（每个含独立 server+token），当非空时覆盖旧的 `server`+`token` 单点模式
- **中继模式**：`relay=true` 时 Agent 作为网关监听本地端口，反向代理到云端 Server，需配合 `relay_secret` 鉴权
- **TLS 信任链**：支持 `tls_skip_verify`（仅自签环境）和 `ca_cert`（自定义 CA PEM 包）两种证书校验策略
- **示例文件自动生成**：首次启动时通过 `embed.go` 中的 `go:embed config_example.json` 在配置目录生成 `config.example.json` 参考文件

### 环境变量覆盖层 (`applyEnvOverrides`)
Server 启动后按以下顺序应用 `AIOPS_*` 环境变量覆盖（优先级高于 JSON 文件）：
- `AIOPS_VM_URL` → 启用 VictoriaMetrics remote-write
- `AIOPS_POSTGRES_DSN` → 切换配置持久化为 PostgreSQL
- `AIOPS_FORWARD_LISTEN` / `AIOPS_FORWARD_PORT_RANGE` / `AIOPS_FORWARD_DISABLED`
- `AIOPS_TERMINAL_DISABLED` / `AIOPS_RELAY_SECRET` / `AIOPS_ALLOW_ANONYMOUS_AGENTS`
- `AIOPS_TRUST_PROXY` / `AIOPS_REQUIRE_TOKEN`

## 配置结构分层

| 层级 | 内容 | 来源 | 说明 |
|------|------|------|------|
| L0 默认值 | 所有字段的 `default*Config()` 返回值 | Go 代码 | 保证零配置也能运行 |
| L1 示例文件 | `config.example.json` / `server_config.example.json` | 源码嵌入/仓库根目录 | 部署时复制为实际配置文件 |
| L2 JSON 文件 | `config.json` (Agent) / `server_config.json` (Server) | 磁盘持久化 | 主要配置入口，权限 0600 |
| L3 环境变量 | `AIOPS_*` 变量 | 进程环境 | Docker Compose/K8s 注入首选方式 |
| L4 CLI 参数 | `--server`, `--interval`, `--config` 等 | 命令行 | 临时覆盖，仅影响本次进程 |

## 关键设计决策

1. **向后兼容优先**：保留 `server` 单字段的同时新增 `servers` 数组，旧配置自动转换为新结构
2. **安全默认值**：`forward_listen` 默认 `127.0.0.1`（仅本机）、`require_token` 默认 true、`log_encrypt` 默认 true
3. **字段保护机制**：`Set()` 时显式保留由专用 API 管理的字段（InstallToken、Account、Checks、Playbooks、SLOs、AI、VM、PostgresDSN、ForwardRules、HTTPProxies 等），防止前端表单误清零
4. **令牌轮换窗口**：`ResetToken()` 不直接替换而是进入 7×24h 宽限期，旧 token 仍有效，避免全量 Agent 同时掉线
5. **配置验证**：`Validate()` 在保存前检查阈值范围、SMTP 端口合法性等，拒绝明显错误的配置

## 开发者规范

- **新增配置字段**：必须在 `default*Config()` 中提供默认值，并在 `backfillThresholdDefaults` 或对应迁移函数中处理历史配置
- **敏感字段**：必须走 `encryptConfigSecrets`/`decryptConfigSecrets` 流程，禁止明文落盘
- **环境变量映射**：新增 `AIOPS_*` 变量需在 `applyEnvOverrides` 中注册，命名遵循 `AIOPS_` 前缀 + 小写下划线
- **字段保护**：若某字段应由专用 API 管理而非通用设置表单，需在 `Set()` 中显式保留其现有值
- **示例文件同步**：修改 `config.example.json` 或 `server_config.example.json` 后需同步更新 `cmd/agent/embed.go` 中的 go:generate 指令
- **权限控制**：配置文件写入时必须确保 0600 权限，避免共享主机上的信息泄露

## 关键文件清单

- `cmd/server/config.go` — Server 配置核心逻辑（ConfigStore、ServerConfig、阈值管理、环境变量覆盖）
- `cmd/agent/main.go` — Agent 配置加载与命令行参数解析
- `cmd/agent/embed.go` — 示例配置文件嵌入与自动生成
- `config.example.json` — Agent 配置示例（含 Redfish/NetFlow/PacketCapture 等高级采集器）
- `server_config.example.json` — Server 配置示例（通知渠道、阈值、账户）
- `.env.example` — 环境变量模板（Docker Compose 使用）
- `docker-compose.yml` — 容器编排中的配置注入示例