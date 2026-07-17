---
kind: configuration_system
name: AIOps 平台配置系统：JSON 文件 + AIOPS_* 环境变量覆盖 + 运行时持久化
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/server/main.go
    - server_config.example.json
    - cmd/agent/main.go
    - cmd/agent/config_example.json
    - config.example.json
    - docker-compose.yml
---

## 1. 系统概览
本仓库采用「静态 JSON 配置文件 + AIOPS_* 环境变量覆盖 + 运行时 API 持久化」三层叠加的轻量配置体系，分别服务于 Server 与 Agent 两个进程。Server 侧通过 `ConfigStore` 提供线程安全的读写、默认值回填、阈值校验、密钥解密/加密以及 PostgreSQL 双写能力；Agent 侧以纯 JSON 文件为主，配合命令行 flag 覆盖，无运行时持久化。

## 2. 关键文件与包
- **Server 配置核心**
  - `cmd/server/config.go` — `ServerConfig` / `ThresholdConfig` / `ConfigStore` 定义、加载、验证、默认值回填、环境变量覆盖、PostgreSQL 持久化、安装 Token 轮换等全部逻辑
  - `cmd/server/main.go` — 启动时解析 `-config`、读取 `AIOPS_POSTGRES_DSN` / `AIOPS_VM_URL` / `AIOPS_TLS_CERT` / `AIOPS_TLS_KEY` / `AIOPS_SECRET_KEY` 等环境变量，并初始化 `ConfigStore`
- **Server 示例配置**
  - `server_config.example.json` — 初始化的最小可运行配置（告警开关、飞书/钉钉、阈值、账号）
- **Agent 配置**
  - `cmd/agent/main.go` — 解析 `--config`、`--server`、`--interval` 等 flag，从 `config.json` 加载 `config` 结构体
  - `cmd/agent/config_example.json` / `config.example.json` — 带中文注释的完整示例（多服务端、Relay、TLS、Redfish、NetFlow、五元组采集）
- **Docker Compose 集成**
  - `docker-compose.yml` — 集中声明所有 `AIOPS_*` 环境变量，作为容器部署时的“真实配置源”

## 3. 架构与设计约定
### 3.1 加载顺序（Server）
1. 构建默认值 `defaultServerConfig()` / `defaultThresholdConfig()`
2. 优先从 PostgreSQL 读 `aiops_config_blob`（双 DB 模式），否则回退到本地 JSON 文件
3. 对已加载配置执行：
   - 解密在库中 AES-256-GCM 加密的敏感字段（MFA/SMTP/AI/Webhook 等）
   - 生成缺失的 `install_token`、迁移单用户→多用户、回填零阈值到标准默认值
4. 应用 `applyEnvOverrides()`：用 `AIOPS_*` 环境变量覆盖对应 JSON 字段
5. 调用 `Validate()` 做范围检查（百分比 0–100、端口合法、密码长度等），失败则拒绝启动
6. 如有变更则立即 `save()` 落盘/入库

### 3.2 加载顺序（Agent）
1. 构造 `defaultConfig()`（含 OS 相关的 python 路径、磁盘根目录等）
2. 扫描 `os.Args` 中的 `--config`，若找到文件则 `json.Unmarshal` 覆盖默认值
3. 再解析 `flag.Parse()`，命令行参数覆盖 JSON 文件
4. 根据 `servers` 是否为空决定使用多服务端还是 legacy `server+token` 单服务端模式

### 3.3 环境变量命名规范（AIOPS_*）
| 变量名 | 作用域 | 说明 |
|---|---|---|
| `AIOPS_POSTGRES_DSN` | Server | 强制要求，PostgreSQL DSN（未设置直接退出） |
| `AIOPS_VM_URL` | Server | 启用 VictoriaMetrics remote-write |
| `AIOPS_TLS_CERT` / `AIOPS_TLS_KEY` | Server | 启用 HTTPS 监听 |
| `AIOPS_SECRET_KEY` | Server | 配置密钥静态加密 master key（AES-256-GCM） |
| `AIOPS_FORWARD_LISTEN` / `AIOPS_FORWARD_PORT_RANGE` / `AIOPS_FORWARD_DISABLED` | Server | 端口转发监听地址、端口范围、全局禁用开关 |
| `AIOPS_TERMINAL_DISABLED` | Server | 全局禁用远程终端 |
| `AIOPS_RELAY_SECRET` | Server | 网关中继共享密钥 |
| `AIOPS_ALLOW_ANONYMOUS_AGENTS` / `AIOPS_REQUIRE_TOKEN` | Server | 是否允许无 token 的 Agent 注册 |
| `AIOPS_TRUST_PROXY` | Server | 信任反向代理 X-Real-IP/X-Forwarded-For |

### 3.4 运行时持久化与热更新
- `ConfigStore` 以 `sync.RWMutex` 保护内存中的 `ServerConfig`，所有 Getter/Setter 方法均加锁后返回副本或写入后 `save()`。
- `Set()` 会先快照当前配置到 `prev`，以便后续 `Revert()` 回滚；同时保护由专用 API 管理的字段（如 `Checks`、`Playbooks`、`APISystems`、`Governance`、`SLOs`、`RemediationRules`、`HTTPProxies`、`ForwardRules`、`InstallToken`、`Account` 等）不被通用 Settings 表单覆盖。
- 阈值字段采用「零值=未配置」语义，`backfillThresholdDefaults` 自动回填为 `defaultThresholdConfig()`，保证旧配置和新表单都不会出现危险的全零阈值。
- 支持「安装 Token 轮换」：`ResetToken()` 将当前 token 移入 `PrevInstallToken` 并保留 7 天宽限期，期间 `ValidInstallToken` 用常量时间比较同时接受新旧 token。

### 3.5 安全相关约定
- 所有敏感字段（SMTP 密码、SMS/VoiceCall SecretKey、Webhook Secret、MFA Secret、Terminal Password Hash、RelaySecret）在 GET 接口返回时被掩码（`****`），写入时若为空或包含 `****` 则保留旧值。
- 当 `AIOPS_SECRET_KEY` 存在时，这些字段在 PostgreSQL 中以 AES-256-GCM 静态加密存储；否则明文落库并输出警告日志。
- `TrustProxy` 默认关闭，防止伪造 X-Forwarded-For 绕过登录限流。
- `ForwardListen` 默认 `127.0.0.1`，仅本机可访问；需显式设为 `0.0.0.0` 才对外暴露。

## 4. 开发者应遵循的规则
1. **新增配置项**：在 `ServerConfig` 结构体上添加 JSON tag，并在 `defaultServerConfig()` / `defaultThresholdConfig()` 中给出合理默认值；如需环境变量覆盖，在 `applyEnvOverrides()` 中添加 `AIOPS_XXX` 分支。
2. **阈值类配置**：统一走 `ThresholdConfig`，新增字段后同步在 `defaultThresholdConfig()`、`backfillThresholdDefaults()`、`toThresholds()` 三处补齐，避免零值导致误告警。
3. **敏感字段**：必须实现「GET 掩码 + SET 保留旧值 + 可选静态加密」三件套，参考 SMTP/SMS/VoiceCall/Webhook Secret 的实现模式。
4. **配置校验**：在 `ServerConfig.Validate()` 中对范围、必填字段进行断言，错误信息使用 `Tz(...)` 国际化。
5. **向后兼容**：对废弃字段保持 `omitempty` 且不做破坏性变更；需要迁移时在 `NewConfigStore` 中做一次性迁移（参考单用户→多用户的 `migrateUsers`）。
6. **Agent 侧新增配置**：在 `cmd/agent/main.go` 的 `config` 结构体增加字段，并在 `defaultConfig()` 给出默认值；如需命令行覆盖，在 `flag.StringVar` 处补充。
7. **不要绕过 ConfigStore**：所有对 Server 配置的读写都应通过 `ConfigStore` 的方法，禁止直接操作 `ServerConfig` 实例，以保证并发安全与持久化一致性。
8. **环境变量优先级**：始终记住 `AIOPS_* > JSON 文件 > 代码默认值`，在 Docker Compose 中集中声明所有 `AIOPS_*`，避免硬编码进镜像。