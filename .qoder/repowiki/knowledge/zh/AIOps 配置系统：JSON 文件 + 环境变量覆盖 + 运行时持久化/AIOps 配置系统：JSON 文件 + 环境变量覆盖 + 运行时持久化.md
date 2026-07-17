---
kind: configuration_system
name: AIOps 配置系统：JSON 文件 + 环境变量覆盖 + 运行时持久化
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

## 系统概览

AIOps 平台采用轻量级、自包含的配置方案，不使用外部配置中心或框架（如 Viper），而是基于 Go 标准库 encoding/json + flag + os.LookupEnv 实现。Server 与 Agent 各自维护独立的 JSON 配置文件，并通过环境变量提供运行时覆盖能力。

## 核心架构

### Server 端配置 (cmd/server/config.go)
- 主配置结构体: ServerConfig，包含告警阈值、通知渠道（飞书/钉钉/自定义 Webhook/SMTP/SMS/语音）、用户账户、拨测规则、SLO、AI 配置等
- 配置存储: ConfigStore 封装了内存中的 ServerConfig，提供线程安全的读写访问
- 加载顺序: PostgreSQL（如果配置了 AIOPS_POSTGRES_DSN）→ 本地 JSON 文件 → 默认值 (defaultServerConfig())
- 环境覆盖: applyEnvOverrides() 支持通过 AIOPS_* 环境变量覆盖配置项
- 安全特性: 配置文件权限强制为 0600，敏感字段（密码、密钥）支持可逆加密存储（AIOPS_SECRET_KEY），安装 Token 支持轮换机制（保留旧 token 7 天宽限期）

### Agent 端配置 (cmd/agent/main.go)
- 主配置结构体: config，包含服务端地址、采集间隔、插件目录、TLS 设置等
- 加载顺序: 命令行参数解析（--server, --interval 等）→ 配置文件 config.json（--config 指定路径）→ 默认值 (defaultConfig())
- 多服务端支持: servers 数组可配置多个上报目标，优先于单 server 字段
- 示例生成: 首次启动自动在配置目录生成 config.example.json

## 配置优先级

命令行参数 > 配置文件 > 环境变量 > 默认值

## 关键设计决策

1. 零依赖: 不引入第三方配置库，降低部署复杂度
2. 向后兼容: 通过 omitempty 和零值回退机制保证旧配置文件的兼容性
3. 热更新友好: 配置变更通过 API 接口触发，内部使用 RWMutex 保护并发访问
4. 安全优先: 敏感信息加密存储、最小权限原则、Token 轮换机制
5. 运维友好: 丰富的默认值、详细的错误提示、配置验证机制

## 开发者规范

- 新增配置字段时，必须同时更新 default*Config() 函数
- 敏感字段应标记 omitempty 并考虑加密存储
- 所有配置变更都应通过 ConfigStore 的方法进行，确保线程安全
- 环境变量覆盖的字段应在注释中明确说明
- 配置验证逻辑应放在 Validate() 方法中集中处理