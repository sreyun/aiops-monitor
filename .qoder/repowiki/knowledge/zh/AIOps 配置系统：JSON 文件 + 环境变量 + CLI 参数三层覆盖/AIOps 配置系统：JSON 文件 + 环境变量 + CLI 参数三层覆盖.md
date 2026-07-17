---
kind: configuration_system
name: AIOps 配置系统：JSON 文件 + 环境变量 + CLI 参数三层覆盖
category: configuration_system
scope:
    - '**'
source_files:
    - cmd/server/config.go
    - cmd/server/main.go
    - cmd/agent/main.go
    - config.example.json
    - server_config.example.json
---

## 体系概览

AIOps 平台采用“配置文件 + 环境变量 + 命令行参数”的三层覆盖模型，Server 与 Agent 各自维护独立的 JSON 配置文件，并通过 AIOPS_* 前缀的环境变量进行运行时覆盖。所有敏感字段（SMTP/短信/语音密钥、MFA 密钥、安装 Token 等）在持久化时支持 AES-256-GCM 可逆加密。

## 加载顺序与优先级

默认值由 Go 代码中的 default*Config() / defaultThresholdConfig() 提供；配置文件通过 -config 指定路径（Server 默认 server_config.json，Agent 默认 config.json）；AIOPS_* 环境变量仅作用于 Server，用于 Docker Compose 注入；命令行参数最高优先级，覆盖文件和 env。

Server 启动流程（cmd/server/main.go → NewConfigStore）：读取 PostgreSQL 中存储的配置 blob（若启用外部存储）→ 回退到磁盘 JSON 文件 → 解密内存中的可逆密钥 → 回填缺失阈值（backfillThresholdDefaults）→ 迁移旧单用户为多用户列表 → 应用 AIOPS_* 环境变量覆盖 → 校验并持久化变更。

Agent 启动流程（cmd/agent/main.go）：扫描 os.Args 手动解析 --config 路径（早于 flag.Parse）→ 读取 JSON 文件 → flag.Parse() 以命令行参数覆盖 → 根据 relay 字段决定是否进入中继模式。

## 核心数据结构

ServerConfig（cmd/server/config.go）：包含告警开关、飞书/钉钉/自定义 Webhook、SMTP、SMS、语音通话、阈值、分类、安装 Token、RBAC 用户、拨测检查、SLO、AI 配置、VictoriaMetrics 写入器、PostgreSQL DSN、终端/转发开关、CORS 白名单、代理信任、MFA 策略等。

ThresholdConfig：按 CPU/Mem/Disk/GPU/Load/Conn/Ping/TCP/HTTP/API/Task/Forward 等维度定义 warn/crit 两级阈值，零值自动回填默认值。

config（Agent）：server/servers 多后端上报、插件周期、日志采集路径、Redfish/BMC、NetFlow、五元组包捕获等。

## 持久化与热更新

ConfigStore 封装 sync.RWMutex，提供 Get/Set/Revert 原子操作，每次 Set 前快照 prev 支持一键回滚。写盘时先对可逆密钥执行 encryptConfigSecrets，再以 0o600 权限落盘；当配置了 PostgreSQL 则整体序列化为 JSONB 行保存。运行时通过 API 修改阈值、用户、拨测规则等，立即生效，无需重启。

## 环境变量清单（Server）

AIOPS_VM_URL → vm.url + vm.enabled=true
AIOPS_POSTGRES_DSN → postgres_dsn
AIOPS_FORWARD_LISTEN → forward_listen
AIOPS_FORWARD_PORT_RANGE → forward_port_range
AIOPS_RELAY_SECRET → relay_secret
AIOPS_FORWARD_DISABLED → forward_disabled
AIOPS_TERMINAL_DISABLED → terminal_disabled
AIOPS_ALLOW_ANONYMOUS_AGENTS → allow_anonymous_agents
AIOPS_TRUST_PROXY → trust_proxy
AIOPS_REQUIRE_TOKEN → require_token
AIOPS_TLS_CERT/AIOPS_TLS_KEY → 启用 HTTPS
AIOPS_SECRET_KEY → 配置静态加密主密钥

## 安全约定

安装 Token 支持轮换：ResetToken 将当前 token 降级为 PrevInstallToken，保留 7 天宽限期，验证使用常量时间比较。SMTP/SMS/VoiceCall/AI 等密钥在 GET 返回时被脱敏（含 ****），表单提交空值或脱敏值时保留原值。未设置 AIOPS_SECRET_KEY 时密钥明文落库，启动会输出警告。端口转发默认绑定 127.0.0.1，需显式设为 0.0.0.0 才对外暴露。

## 开发者规范

新增配置字段应在 ServerConfig/config 结构体上添加 json tag，并在 default*Config 中给出合理默认值。若该字段需要环境覆盖，在 applyEnvOverrides 中添加对应 AIOPS_* 分支。若字段包含敏感信息，确保在 encryptConfigSecrets/decryptConfigSecrets 中处理，并在 GET 接口中脱敏。阈值类字段遵循“零值=未配置→回填默认”的模式，避免误触发告警。所有写操作走 ConfigStore.Set，利用其保护机制（保留受管字段、校验、持久化）。