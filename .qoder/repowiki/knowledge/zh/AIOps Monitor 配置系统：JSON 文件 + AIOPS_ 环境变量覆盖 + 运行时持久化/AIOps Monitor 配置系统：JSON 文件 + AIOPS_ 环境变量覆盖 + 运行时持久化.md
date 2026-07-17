---
kind: configuration_system
name: AIOps Monitor 配置系统：JSON 文件 + AIOPS_* 环境变量覆盖 + 运行时持久化
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/agent/main.go
    - config.example.json
    - server_config.example.json
---

## 1. 采用的方式与工具

- **Server（服务端）**：基于 Go 标准库 `encoding/json` 自行实现，无第三方配置框架。核心由 `cmd/server/config.go` 中的 `ConfigStore` 提供线程安全的内存配置对象、磁盘/PostgreSQL 持久化、默认值回填、字段校验、敏感信息加解密以及 `AIOPS_*` 环境变量覆盖。
- **Agent（采集端）**：同样使用 `encoding/json` 读取本地 JSON 配置文件，并通过 `flag` 包命令行参数覆盖；无环境变量覆盖机制。
- **Android 客户端**：通过 `data/store/SettingsStore.kt` 以 Android SharedPreferences 形式保存设置，不属于本仓库的 Server/Agent 配置体系。

## 2. 关键文件与包

- `cmd/server/config.go` — Server 配置模型、加载、验证、持久化、环境变量覆盖、阈值回填、安全字段加解密等全部逻辑。
- `cmd/agent/main.go` — Agent 启动入口，负责解析 `config.json` 与命令行 flag，构建多后端上报列表。
- `config.example.json` — Agent 示例配置（单/多 server、插件目录、日志路径等）。
- `server_config.example.json` — Server 示例配置（告警通道、阈值、账户、转发端口范围等）。
- `docker-compose.yml` / `.env.example` — 通过 `AIOPS_*` 环境变量注入 VM URL、Postgres DSN、转发监听地址等。

## 3. 架构与约定

### 3.1 配置来源优先级（Server）

1. 内置默认值（`defaultServerConfig()` / `defaultThresholdConfig()`）
2. 磁盘 JSON 文件或 PostgreSQL JSONB blob（优先 PG，若存在则跳过 JSON 文件）
3. `AIOPS_*` 环境变量覆盖（仅支持一组白名单变量，见下方）
4. 运行时通过 API 修改并调用 `Set()` 写入

加载顺序在 `NewConfigStore` 中体现：先尝试 PG → 再回退到 JSON 文件 → 然后应用 env 覆盖 → 最后执行阈值回填与用户迁移。

### 3.2 支持的 AIOPS_* 环境变量（Server）

| 环境变量 | 对应字段 | 说明 |
|---|---|---|
| `AIOPS_VM_URL` | `VM.URL` + `VM.Enabled=true` | 启用 VictoriaMetrics remote-write |
| `AIOPS_POSTGRES_DSN` | `PostgresDSN` | 切换到 PostgreSQL 持久化模式 |
| `AIOPS_FORWARD_LISTEN` | `ForwardListen` | 端口转发监听地址 |
| `AIOPS_FORWARD_PORT_RANGE` | `ForwardPortRange` | 转发端口范围 |
| `AIOPS_RELAY_SECRET` | `RelaySecret` | 网关中继共享密钥 |
| `AIOPS_TERMINAL_DISABLED` | `TerminalDisabled` | 全局禁用远程终端 |
| `AIOPS_FORWARD_DISABLED` | `ForwardDisabled` | 全局禁用端口转发 |
| `AIOPS_ALLOW_ANONYMOUS_AGENTS` | `AllowAnonymousAgents` | 允许无 token 的 Agent |
| `AIOPS_TRUST_PROXY` | `TrustProxy` | 信任反向代理 X-Real-IP |
| `AIOPS_REQUIRE_TOKEN` | `RequireToken` | 强制 Dashboard 登录需 token |

这些变量在 `applyEnvOverrides()` 中以字符串比较方式解析为布尔或字面量。

### 3.3 配置存储与并发

- `ConfigStore` 内部持有 `sync.RWMutex`，所有读写方法均加锁。
- 写路径 `save()` 会先深拷贝当前配置，对敏感字段做可逆加密后再落盘，避免泄露内存明文。
- 当 `PostgresDSN` 非空时，整个 `ServerConfig` 序列化为一个 JSONB 行存入 Postgres，不再写 JSON 文件。

### 3.4 默认值与自愈

- 阈值字段采用“零值即未配置”的约定：`backfillThresholdDefaults` 会把任何 0 值替换为标准默认值，并在首次加载或保存时自动写回，保证旧配置与新字段兼容。
- 首次运行自动生成随机 `InstallToken`、默认 admin 账户，并进行从单账户到多用户的迁移。

### 3.5 安全字段处理

- SMTP/DingTalk/飞书/Webhook/SMS/VoiceCall/AI Provider 的密码或密钥在 `save()` 前调用 `encryptConfigSecrets` 进行可逆加密；加载后调用 `decryptConfigSecrets` 恢复明文供运行时使用。
- 返回给浏览器的配置接口会对敏感字段脱敏（如 `****`），前端表单提交空值或带 `****` 的值时，`Set()` 会保留原值不被覆盖。
- JSON 文件权限强制设为 `0o600`，防止其他用户读取。

### 3.6 Agent 配置加载流程

1. 构造 `defaultConfig()` 得到默认值。
2. 扫描 `--config` 参数，按路径读取 JSON 并 `json.Unmarshal` 覆盖默认值。
3. 解析 `flag` 命令行参数，覆盖 JSON 中的同名字段。
4. 根据 `servers` 数组或 legacy `server+token` 生成最终上报目标列表。

Agent 不支持环境变量覆盖，也不做运行时持久化。

## 4. 开发者应遵循的规则

- **新增配置字段**：在 `ServerConfig` 结构体上添加字段，必要时在 `Validate()` 增加约束，在 `defaultServerConfig()` 或 `defaultThresholdConfig()` 提供默认值，并在 `Set()` 中决定是否受前端表单覆盖。
- **新增环境变量覆盖**：在 `applyEnvOverrides()` 中添加 `os.LookupEnv("AIOPS_XXX")` 分支，保持命名风格一致。
- **敏感字段**：如需新增密码/密钥类字段，确保在 `encryptConfigSecrets` / `decryptConfigSecrets` 中纳入加解密，并在 GET 配置接口中脱敏返回。
- **向后兼容**：利用“零值=未配置”的阈值回填策略，新字段默认 0 即可被自动修复为合理默认值，无需额外迁移代码。
- **Agent 侧变更**：仅在 `cmd/agent/main.go` 的 `config` 结构体与 `flag` 绑定处修改，不要引入环境变量覆盖以免破坏现有行为。
- **测试**：涉及 `AIOPS_*` 环境变量的测试应使用 `t.Setenv` 隔离，避免污染进程环境。
