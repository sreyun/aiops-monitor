---
kind: configuration_system
name: AIOps 监控平台配置系统（JSON 文件 + 环境变量覆盖）
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/agent/main.go
    - config.example.json
    - server_config.example.json
    - cmd/server/install.go
---

## 1. 系统概览

AIOps 监控平台的配置系统采用 **轻量级 JSON 配置文件 + 环境变量覆盖** 的模式，Server 端和 Agent 端各自维护独立的配置结构，通过 `ConfigStore` 提供线程安全的读写、持久化与回滚能力。

- **配置文件格式**：纯 JSON（无 YAML/TOML/INI），字段名使用 snake_case
- **加载顺序**：默认值 → JSON 文件 → 命令行 flag → 环境变量（仅 Server 端支持）
- **持久化存储**：本地文件（0o600 权限）或 PostgreSQL JSONB（双后端模式）
- **安全特性**：敏感字段可启用 AES 加密封装（AIOPS_SECRET_KEY），密码哈希存储，MFA 支持

## 2. 核心架构

### Server 端配置（cmd/server/config.go）

```
ServerConfig (主配置结构)
├── ConfigStore (线程安全包装器)
│   ├── Get/Set/Revert (原子操作)
│   ├── save() (加密→序列化→持久化)
│   └── applyEnvOverrides() (AIOPS_* 环境变量覆盖)
├── ThresholdConfig (告警阈值)
├── AccountConfig (管理员账户+MFA)
├── WebhookConfig (飞书/钉钉)
├── SMTPConfig/SMSConfig/VoiceCallConfig (通知渠道)
└── 运行时配置项 (checks/playbooks/SLOs/forward_rules 等)
```

**关键设计决策：**
- `ConfigStore` 使用 `sync.RWMutex` 保护并发访问
- `Set()` 前自动 backfill 零值阈值为标准默认值
- `save()` 前对敏感字段执行 `encryptConfigSecrets()`（需设置 AIOPS_SECRET_KEY）
- 支持 Revert() 回滚最近一次 Set() 变更

### Agent 端配置（cmd/agent/main.go）

```
config (Agent 配置结构)
├── defaultConfig() (默认值)
├── 手动扫描 --config 参数
├── JSON 反序列化为 config
├── flag.Parse() 覆盖
└── 多服务端上报 (servers 数组优先于 server+token)
```

## 3. 配置来源与优先级

| 来源 | Server 端 | Agent 端 | 说明 |
|------|-----------|----------|------|
| 默认值 | `defaultServerConfig()` / `defaultThresholdConfig()` | `defaultConfig()` | Go 代码中硬编码 |
| JSON 文件 | `server_config.json` (路径由启动参数决定) | `config.json` (当前目录) | 首次启动自动生成示例 |
| 命令行 flag | 部分字段支持 | 广泛支持 (--server, --interval, --plugins-dir 等) | 覆盖 JSON 值 |
| 环境变量 | `AIOPS_*` 变量 | 不支持 | 仅 Server 端，Docker Compose 友好 |

**支持的 AIOPS_* 环境变量（Server 端）：**
- `AIOPS_VM_URL` - VictoriaMetrics 远程写入地址
- `AIOPS_POSTGRES_DSN` - PostgreSQL 连接串
- `AIOPS_FORWARD_LISTEN` - 端口转发监听地址
- `AIOPS_FORWARD_PORT_RANGE` - 端口范围
- `AIOPS_RELAY_SECRET` - 中继共享密钥
- `AIOPS_TERMINAL_DISABLED` - 禁用终端功能
- `AIOPS_ALLOW_ANONYMOUS_AGENTS` - 允许匿名 Agent
- `AIOPS_TRUST_PROXY` - 信任反向代理头
- `AIOPS_REQUIRE_TOKEN` - 强制安装令牌

## 4. 安全机制

- **文件权限**：配置文件以 0o600 权限写入，防止其他用户读取
- **敏感字段加密**：当设置 `AIOPS_SECRET_KEY` 时，Webhook Secret、SMTP 密码、SMS/VoiceCall 密钥等会被 AES 加密封装后落盘
- **密码哈希**：管理员密码使用 salted hash 存储，从不以明文形式保存
- **Token 轮换**：Install Token 支持平滑轮换，旧 Token 在 7 天宽限期内仍有效
- **TLS 验证**：Agent 支持自定义 CA 证书或跳过验证（仅限内网测试）

## 5. 开发者规范

1. **新增配置字段**：
   - 在对应结构体中添加 JSON tag（snake_case）
   - 在 `default*Config()` 中提供合理默认值
   - 如需环境变量覆盖，在 `applyEnvOverrides()` 中添加映射
   - 更新 `Validate()` 进行基本校验

2. **敏感字段处理**：
   - 使用 `omitempty` 标记可选字段
   - 在 `encryptConfigSecrets()` / `decryptConfigSecrets()` 中注册新字段
   - 前端展示时使用 `****` 脱敏

3. **向后兼容**：
   - 使用 `backfillThresholdDefaults()` 模式填充缺失字段
   - 迁移逻辑放在 `NewConfigStore()` 初始化阶段
   - 保留旧字段至少一个版本周期

4. **配置验证**：
   - 在 `Validate()` 中实现业务规则校验
   - 返回国际化错误消息（使用 Tz() 函数）
   - 拒绝明显错误的配置阻止启动

## 6. 关键文件

- `cmd/server/config.go` - Server 配置核心逻辑（1450 行）
- `cmd/agent/main.go` - Agent 配置加载流程
- `config.example.json` - Agent 配置示例
- `server_config.example.json` - Server 配置示例
- `cmd/server/install.go` - 安装脚本生成配置片段
- `.env.example` - Docker Compose 环境变量示例