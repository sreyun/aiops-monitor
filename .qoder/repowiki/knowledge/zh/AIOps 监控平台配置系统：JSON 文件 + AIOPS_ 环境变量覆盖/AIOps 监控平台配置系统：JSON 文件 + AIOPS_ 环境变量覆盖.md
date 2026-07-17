---
kind: configuration_system
name: AIOps 监控平台配置系统：JSON 文件 + AIOPS_* 环境变量覆盖
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/agent/main.go
    - server_config.example.json
    - config.example.json
    - cmd/server/install.go
---

## 1. 系统概览

AIOps 监控平台的配置系统采用 JSON 配置文件加环境变量覆盖的轻量方案，不使用第三方配置库（如 Viper、koanf）。Server 端与 Agent 端各自维护独立的 JSON 配置，并通过 AIOPS_* 前缀的环境变量在容器化部署场景下实现运行时覆盖。

- Server 端配置：server_config.json（由安装脚本生成），通过 ConfigStore 统一管理，支持热更新、回滚、持久化到文件或 PostgreSQL。
- Agent 端配置：config.json，启动时加载，支持命令行参数覆盖。
- 环境变量覆盖：仅 Server 端支持 AIOPS_* 环境变量覆盖 JSON 字段，用于 Docker Compose 或 Kubernetes 等无状态部署。

## 2. 核心文件与包

- cmd/server/config.go：ServerConfig、ThresholdConfig、AccountConfig 等结构体定义，以及 ConfigStore 持久化逻辑
- server_config.example.json：初始配置模板，包含告警阈值、通知渠道、账户等
- cmd/agent/main.go：config 结构体定义，defaultConfig() 提供默认值
- config.example.json：完整的 Agent 配置示例，含 Redfish、NetFlow、PacketCapture 等可选采集器
- cmd/server/install.go：自动生成 config.json（Agent）和 server_config.json（Server），注入 install token

## 3. 架构与设计决策

### 3.1 分层加载顺序

Server 端（NewConfigStore）：
1. 优先从 PostgreSQL 读取（当 pg != nil 时）
2. 否则从本地 JSON 文件读取
3. 若两者均不存在，使用 defaultServerConfig() 初始化
4. 解密 at-rest 加密的密钥（当设置 AIOPS_SECRET_KEY）
5. 自动补全缺失的阈值字段（backfillThresholdDefaults）
6. 迁移旧版单用户到多用户列表（migrateUsers）
7. 应用 AIOPS_* 环境变量覆盖（applyEnvOverrides）
8. 校验配置（Validate），失败则拒绝启动

Agent 端（main）：
1. 解析 --config 参数确定配置文件路径（默认 config.json）
2. 读取并反序列化 JSON 配置
3. 确保 config.example.json 存在（首次启动时生成）
4. 命令行参数覆盖 JSON 配置（flag.Parse）

### 3.2 配置持久化策略

- 默认存储：本地 JSON 文件，权限强制为 0o600（仅 owner 可读写）
- PostgreSQL 模式：当设置 AIOPS_POSTGRES_DSN 时，整个 ServerConfig 序列化为 JSONB 行存储
- 内存快照：每次 Set() 前保存上一份配置到 prev，支持 Revert() 回滚
- 并发安全：所有读写操作通过 sync.RWMutex 保护

### 3.3 环境变量覆盖机制

Server 端支持的 AIOPS_* 环境变量（按优先级高于 JSON 文件）：
- AIOPS_VM_URL：启用 VictoriaMetrics 远程写入
- AIOPS_POSTGRES_DSN：切换到 PostgreSQL 持久化
- AIOPS_FORWARD_LISTEN：端口转发监听地址
- AIOPS_FORWARD_PORT_RANGE：端口范围（如 "10100-10300"）
- AIOPS_RELAY_SECRET：中继网关共享密钥
- AIOPS_FORWARD_DISABLED：禁用端口转发功能
- AIOPS_TERMINAL_DISABLED：禁用远程终端功能
- AIOPS_ALLOW_ANONYMOUS_AGENTS：允许无 token 的 Agent 连接
- AIOPS_TRUST_PROXY：信任反向代理的 X-Real-IP 头
- AIOPS_REQUIRE_TOKEN：强制 Agent 必须携带安装 token

### 3.4 安全设计

- 密钥加密：当设置 AIOPS_SECRET_KEY 时，敏感字段（SMTP 密码、SMS/VoiceCall secret_key、Webhook secret、RelaySecret、MFA secret）在磁盘上以 AES-GCM 加密存储，内存中解密
- Token 轮换：ResetToken() 将当前 token 降级为 PrevInstallToken，保留 7 天宽限期，避免瞬间导致所有 Agent 离线
- 常量时间比较：使用 subtle.ConstantTimeCompare 验证 token，防止时序攻击
- 最小权限：配置文件默认 0o600 权限，仅进程所有者可读

## 4. 开发者规范

### 4.1 新增配置项的步骤

1. 定义结构体字段：在 ServerConfig 或 config 中添加新字段，附带 json:"field_name" tag
2. 添加默认值：在 defaultServerConfig() 或 defaultConfig() 中提供合理默认值
3. 处理环境变量（可选）：如需支持运行时覆盖，在 applyEnvOverrides() 中添加映射
4. 更新示例配置：同步修改 server_config.example.json 或 config.example.json
5. 安全性考虑：敏感字段应加入 encryptConfigSecrets / decryptConfigSecrets 白名单；布尔开关建议使用反向标志（如 terminal_disabled 而非 terminal_enabled），使零值为启用状态，兼容旧配置
6. 测试覆盖：为新字段添加单元测试，特别是边界值和默认值回填逻辑

### 4.2 配置变更的最佳实践

- 向后兼容：新增字段必须提供默认值，旧配置不应因缺少字段而崩溃
- 阈值回填：所有阈值字段遵循零值视为未配置原则，启动时自动回填标准默认值
- 表单提交保护：前端表单不提交的字段（如 Users、CORSOrigins、ForwardRules）在 Set() 中显式保留原值，避免被清空
- 错误反馈：Validate() 返回国际化错误消息（通过 Tz() 函数），便于 UI 展示

### 4.3 部署建议

- Docker Compose：优先使用 AIOPS_* 环境变量注入敏感配置，避免硬编码到 JSON 文件
- Kubernetes：通过 Secret 挂载为环境变量，结合 ConfigMap 管理非敏感配置
- 生产环境：务必设置 AIOPS_SECRET_KEY 启用密钥加密，并限制配置文件访问权限
- 多实例部署：每个 Server 实例独立持有自己的 server_config.json，或通过 PostgreSQL 共享配置
