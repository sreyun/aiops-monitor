---
kind: configuration_system
name: AIOps 监控平台配置系统（JSON 文件 + AIOPS_* 环境变量）
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - server_config.example.json
    - cmd/agent/main.go
    - config.example.json
    - .env.example
    - cmd/server/install.go
---

## 体系概览

本仓库采用“**静态 JSON 配置文件 + 运行时环境变量覆盖**”的双层配置模型，分别服务于 Agent 与 Server 两个 Go 二进制。配置以人类可读的 JSON 为主，敏感字段支持可选的“可逆加密落盘”，并通过 `AIOPS_*` 环境变量在容器/编排场景下实现无侵入覆盖。

---

## 1. 使用的框架与工具

- **Go 标准库 `encoding/json`**：所有配置结构体使用 struct tag 映射到 JSON 键名，没有引入第三方配置库。
- **`flag` 包**：Agent 启动参数通过命令行 flag 覆盖配置文件值。
- **`os.LookupEnv`**：Server 启动时读取 `AIOPS_*` 环境变量覆盖 JSON 中的对应字段。
- **自定义加解密**：当设置 `AIOPS_SECRET_KEY` 时，对配置中的敏感字段（SMTP/SMS/VoiceCall/AI API Key、RelaySecret 等）进行 AES-GCM 可逆加密后写入磁盘；加载时自动解密回内存明文。

---

## 2. 核心文件与包

| 组件 | 关键文件 | 职责 |
|------|----------|------|
| **Server 配置** | `cmd/server/config.go` | 定义 `ServerConfig`、`ThresholdConfig`、`AccountConfig` 等结构体；实现 `ConfigStore`（带 RWMutex 的线程安全读写、持久化、回滚 Revert()、阈值回填 backfill、环境变量覆盖、密钥加解密）。 |
| **Server 示例配置** | `server_config.example.json` | 告警阈值、飞书/钉钉 Webhook、SMTP/SMS/VoiceCall、账号、转发端口范围等默认模板。 |
| **Agent 配置** | `cmd/agent/main.go` | 解析 `config.json`，合并命令行 flag，构造 `config` 结构体并驱动采集器。 |
| **Agent 示例配置** | `config.example.json` / `cmd/agent/config_example.json` | 上报目标、中继模式、TLS、日志采集、Redfish/BMC、NetFlow、PacketCapture 等。 |
| **Docker 环境** | `.env.example` | Docker Compose 中常用的 `FRONTEND_PORT`、`BACKEND_HOST`、`GOMAXPROCS` 等。 |
| **安装脚本生成配置** | `cmd/server/install.go` | 服务端一键安装命令会动态生成目标机器的 `config.json`（含 log_paths），并写入 systemd/cron 服务单元。 |

---

## 3. 架构与设计约定

### 3.1 分层加载顺序

- **Server**：
  1. 从 `server_config.json` 反序列化为 `ServerConfig`（若存在）。
  2. 调用 `backfillThresholdDefaults` 将缺失/为 0 的阈值回填为标准默认值。
  3. 迁移旧单用户 `Account` → 多用户 `Users` 列表，确保至少有一个 admin。
  4. 应用 `AIOPS_*` 环境变量覆盖（见 §3.3）。
  5. 执行 `Validate()` 校验，失败则拒绝启动。
  6. 如有变更则写回磁盘（权限收紧至 `0o600`）。

- **Agent**：
  1. 默认 `config.json` 路径，可通过 `--config` 指定。
  2. 先读 JSON 文件，再解析命令行 flag（flag 优先级更高）。
  3. 首次启动时若不存在 `config.example.json`，会在同目录自动生成一份注释版模板。

### 3.2 配置结构要点

- **ServerConfig** 是“运营者可编辑”的配置集合，包含：
  - 通知渠道：飞书、钉钉、自定义 Webhook、SMTP、SMS、VoiceCall。
  - 阈值：CPU/Mem/Disk/GPU/Load/Conn/Check/API/Forward 等数十个 warn/crit 级别。
  - 认证：install_token（支持轮换 grace period）、MFA、RBAC Users。
  - 功能开关：terminal_disabled、forward_disabled、allow_anonymous_agents、trust_proxy、mfa_required、cors_origins。
  - 外部存储：VictoriaMetrics remote-write URL、PostgreSQL DSN。
  - 运行时规则：HTTP Proxy 快捷方式、TCP 转发规则、Playbook、SLO、RemediationRule、AI Provider 等。

- **Agent config** 聚焦于“如何连接 Server 以及采集什么”：
  - 单 server 或 servers 数组（多端上报）。
  - relay 网关模式、TLS 证书策略。
  - 日志采集路径、Redfish BMC、NetFlow UDP 接收、nf_conntrack 五元组采集。

### 3.3 环境变量覆盖（AIOPS_*）

Server 启动时在 `applyEnvOverrides()` 中按如下映射覆盖 JSON 值：

| 环境变量 | 覆盖字段 | 说明 |
|----------|----------|------|
| `AIOPS_VM_URL` | `VM.Enabled` + `VM.URL` | 启用 VictoriaMetrics 远程写入 |
| `AIOPS_POSTGRES_DSN` | `PostgresDSN` | 切换为 PostgreSQL 持久化后端 |
| `AIOPS_FORWARD_LISTEN` | `ForwardListen` | 转发监听地址（默认 127.0.0.1） |
| `AIOPS_FORWARD_PORT_RANGE` | `ForwardPortRange` | 转发端口范围（默认 10100-10300） |
| `AIOPS_RELAY_SECRET` | `RelaySecret` | 中继共享密钥 |
| `AIOPS_FORWARD_DISABLED` | `ForwardDisabled` | 禁用端口转发 |
| `AIOPS_TERMINAL_DISABLED` | `TerminalDisabled` | 禁用远程终端 |
| `AIOPS_ALLOW_ANONYMOUS_AGENTS` | `AllowAnonymousAgents` | 允许无 token 的 Agent 注册 |
| `AIOPS_TRUST_PROXY` | `TrustProxy` | 信任反向代理 X-Real-IP |
| `AIOPS_REQUIRE_TOKEN` | `RequireToken` | 强制 Dashboard 登录需要 token |

Agent 侧不读取 `AIOPS_*`，仅依赖 `--config`、`--server`、`--token` 等 flag。

### 3.4 敏感信息保护

- **落盘加密**：当 `AIOPS_SECRET_KEY` 非空时，`save()` 在序列化前调用 `encryptConfigSecrets`，对 SMTP/SMS/VoiceCall/AI Key、RelaySecret 等字段做可逆加密；加载时 `decryptConfigSecrets` 还原为明文供运行期使用。
- **文件权限**：`server_config.json` 写回时强制 `0o600`，避免被其他用户读取。
- **浏览器脱敏**：GET 返回配置时，密码类字段会被替换为 `****`，防止前端泄露。

### 3.5 热更新与回滚

- `ConfigStore.Set()` 在写入前先快照当前配置到 `prev`，随后提供 `Revert()` 恢复上一次成功保存前的状态。
- 阈值回填 `backfillThresholdDefaults` 保证新增阈值字段不会因旧配置缺失而变为 0（0 表示“未配置”，会被修复为默认值）。

---

## 4. 开发者应遵循的规则

1. **新增配置字段**：
   - 在 `ServerConfig` 或 `config` struct 中添加字段，并在 `default*Config()` 中给出合理默认值。
   - 如需支持环境变量覆盖，在 `applyEnvOverrides()` 中增加 `AIOPS_XXX` 映射。
   - 如为敏感字段，确保 `encryptConfigSecrets` / `decryptConfigSecrets` 能识别它。

2. **不要直接操作 JSON 文件**：
   - 所有修改必须通过 `ConfigStore` 的方法（`Set`、`Upsert*`、`Delete*`、`Toggle*`），以保证并发安全、持久化、回滚和字段保护逻辑生效。

3. **阈值字段不得为 0**：
   - 任何阈值字段为 0 都会被视作“未配置”并被回填为默认值；业务代码应按“>= threshold 即触发”语义编写。

4. **环境变量命名规范**：
   - 统一使用 `AIOPS_` 前缀，全大写、下划线分隔；布尔值接受 `"true"` / `"1"`。

5. **Agent 配置变更需重启进程**：
   - Agent 不支持热重载，修改 `config.json` 后需 `systemctl restart aiops-agent`。

6. **Docker 部署优先用 `.env` + `AIOPS_*`**：
   - 不要在镜像里硬编码敏感配置，通过 `docker-compose.yml` 的 `environment` 注入。

