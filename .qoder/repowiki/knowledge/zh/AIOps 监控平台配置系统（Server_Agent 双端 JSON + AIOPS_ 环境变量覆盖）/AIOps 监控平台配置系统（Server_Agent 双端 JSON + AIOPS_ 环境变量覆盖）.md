---
kind: configuration_system
name: AIOps 监控平台配置系统（Server/Agent 双端 JSON + AIOPS_* 环境变量覆盖）
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/server/main.go
    - cmd/agent/main.go
    - config.example.json
    - server_config.example.json
    - docker-compose.yml
---

## 系统概览
本仓库采用「JSON 配置文件 + AIOPS_* 环境变量覆盖」的双层配置模型，分别服务于 Server 与 Agent 两个 Go 二进制。配置以 JSON 为持久化格式，通过 `ConfigStore` 提供线程安全的读写、默认值回填、迁移与校验；敏感字段支持 AES-256-GCM 静态加密，运行时由 `AIOPS_SECRET_KEY` 控制。

## 核心文件与包
- **Server 配置**
  - `cmd/server/config.go` — 定义 `ServerConfig`、`ThresholdConfig`、`WebhookConfig`、`SMTPConfig`、`SMSConfig`、`VoiceCallConfig`、`AccountConfig`、`CustomCheck`、`PersistedForwardRule`、`HTTPProxyConfig`、`DataSource` 等结构体，以及 `ConfigStore` 的加载、合并、保存、回滚、验证逻辑。
  - `cmd/server/main.go` — 启动入口，解析 `-config` 参数，强制要求 `AIOPS_POSTGRES_DSN` 与 `AIOPS_VM_URL`，并可选通过 `AIOPS_TLS_CERT/AIOPS_TLS_KEY` 启用 HTTPS。
  - `server_config.example.json` — Server 初始配置模板。
- **Agent 配置**
  - `cmd/agent/main.go` — 解析 `--config` 路径，加载 `config.json`，支持多后端 `servers[]`、Relay 模式、TLS 信任链、日志采集、Redfish/OceanStor/NetFlow/PacketCapture 等扩展采集器。
  - `config.example.json` / `cmd/agent/config_example.json` — Agent 配置示例（含注释说明）。
- **容器编排与环境变量**
  - `docker-compose.yml` — 演示 `AIOPS_FORWARD_LISTEN`、`AIOPS_VM_URL`、`AIOPS_POSTGRES_DSN`、`AIOPS_TLS_CERT/AIOPS_TLS_KEY`、`AIOPS_SECRET_KEY` 等关键环境变量的注入方式。

## 架构与约定
1. **分层加载顺序（Server）**
   - 默认 `ServerConfig` → 读取 `server_config.json`（或 `-config` 指定路径）→ 若存在 PostgreSQL 则优先从 PG 中加载配置 blob → 解密 at-rest 密钥 → 生成缺失的 install token → 回填零阈值到默认值 → 应用 `AIOPS_*` 环境变量覆盖 → 校验 → 如有变更写回磁盘/PG。
2. **分层加载顺序（Agent）**
   - 默认 `config` → 读取 `--config` 指定的 JSON 文件 → 命令行 flag 覆盖（`--server`、`--interval`、`--plugins-dir`、`--token`、`--relay`、`--listen`、`--log-paths`、`--tls-skip-verify`、`--ca-cert` 等）→ 解析 `servers[]`（非空时覆盖单 `server+token`）→ 初始化 TLS 客户端 → 进入 Relay 或正常上报模式。
3. **环境变量覆盖（AIOPS_*）**
   - 仅 Server 侧实现，集中在 `applyEnvOverrides()`，支持的变量包括：`AIOPS_VM_URL`、`AIOPS_POSTGRES_DSN`、`AIOPS_FORWARD_LISTEN`、`AIOPS_FORWARD_PORT_RANGE`、`AIOPS_RELAY_SECRET`、`AIOPS_FORWARD_DISABLED`、`AIOPS_TERMINAL_DISABLED`、`AIOPS_ALLOW_ANONYMOUS_AGENTS`、`AIOPS_TRUST_PROXY`、`AIOPS_REQUIRE_TOKEN`、`AIOPS_TLS_CERT`、`AIOPS_TLS_KEY`、`AIOPS_SECRET_KEY`。
4. **存储后端统一**
   - 关系型数据（配置、用户、审计、事件、工单、会话）全部落 PostgreSQL；时序指标通过 remote-write 写入 VictoriaMetrics。内置 `aiops.db` 已彻底停用，未配置 PG/VM 将直接拒绝启动。
5. **安全设计**
   - 安装 Token 支持轮换（旧 token 保留 7 天宽限期），使用常量时间比较。
   - 密码与 MFA secret 盐值+哈希存储，永不返回浏览器。
   - 敏感字段（SMTP/SMS/VoiceCall/Webhook Secret/MFA/TOTP/RelaySecret/AI Key）可通过 `AIOPS_SECRET_KEY` 启用 AES-256-GCM 静态加密。
   - 默认 `forward_listen=127.0.0.1`，Docker 部署需显式设为 `0.0.0.0`。
6. **向后兼容与自愈**
   - `backfillThresholdDefaults` 自动将任意为零的阈值字段回填为标准默认值，避免“未配置=0”导致误告警。
   - 单账户 `account` 在首次加载时迁移至多用户 `users[]` 列表。

## 开发者应遵循的规则
- **新增配置项**：在 `ServerConfig` 对应结构体中添加 JSON tag，并在 `defaultServerConfig()` / `defaultThresholdConfig()` 中给出合理默认值；如需环境变量覆盖，在 `applyEnvOverrides()` 中增加分支。
- **新增 Agent 配置项**：在 `cmd/agent/main.go` 的 `config` 结构体中添加字段，更新 `defaultConfig()`，并在 `main()` 中注册对应的 `flag.StringVar/BoolVar`；同时更新 `config.example.json` 中的注释说明。
- **敏感字段处理**：所有凭据类字段必须支持 `****` 占位符回读策略（在 `Set()` 中检测并保留原值），并通过 `secretEncryptionEnabled()` + `decryptConfigSecrets()` 配合 `AIOPS_SECRET_KEY` 做静态加解密。
- **禁止绕过校验**：任何配置修改路径必须先调用 `Validate()`，且阈值字段必须在保存前经过 `backfillThresholdDefaults`。
- **环境变量命名规范**：统一使用 `AIOPS_` 前缀，保持全大写、下划线分隔，并在代码注释中列出完整清单。
- **Docker 部署**：所有外部依赖地址（PG DSN、VM URL、TLS 证书路径）必须通过 `environment:` 注入，不得硬编码进镜像或配置文件。