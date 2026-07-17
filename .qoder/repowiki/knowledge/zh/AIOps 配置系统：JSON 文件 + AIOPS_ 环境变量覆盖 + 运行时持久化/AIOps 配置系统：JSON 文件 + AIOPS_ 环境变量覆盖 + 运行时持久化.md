---
kind: configuration_system
name: AIOps 配置系统：JSON 文件 + AIOPS_* 环境变量覆盖 + 运行时持久化
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

## 1. 整体方案

AIOps 平台采用“**静态 JSON 配置文件 + 启动时 AIOPS_* 环境变量覆盖 + 运行时 API 持久化**”的三层配置模型，Server 与 Agent 各自维护独立的配置结构。

- **Server（cmd/server）**：核心配置为 `server_config.json`，由 `ConfigStore` 统一管理；支持可选 PostgreSQL 持久化（`AIOPS_POSTGRES_DSN`）和 VictoriaMetrics 远程写入（`AIOPS_VM_URL`）。
- **Agent（cmd/agent）**：核心配置为 `config.json`，通过 `--config` 命令行参数指定路径；支持多服务端上报、Relay 网关模式、TLS CA 信任等。

## 2. 关键文件与包

| 组件 | 核心文件 | 作用 |
|------|----------|------|
| Server 配置结构 | `cmd/server/config.go` | `ServerConfig`、`ThresholdConfig`、`AccountConfig`、`ConfigStore` 等全部定义与加载逻辑 |
| Server 示例配置 | `server_config.example.json` | 最小可运行配置模板 |
| Agent 配置结构 | `cmd/agent/main.go` | `config` 结构体、默认值、`--config` 解析 |
| Agent 示例配置 | `config.example.json` | 完整 Agent 配置说明（含 Redfish/NetFlow/PacketCapture） |
| 安装脚本生成配置 | `cmd/server/install.go` | 自动在目标主机生成 `config.json` 并注入 `log_paths` |

## 3. 架构与约定

### 3.1 加载顺序（Server）

```
NewConfigStore(path, pg)
  ├─ 优先从 PostgreSQL 读取（当 pg != nil）
  ├─ 否则回退到本地 JSON 文件
  ├─ decryptConfigSecrets() 解密 at-rest 加密的密钥
  ├─ backfillThresholdDefaults() 回填缺失阈值
  ├─ migrateUsers() 迁移旧单用户 → 多用户
  ├─ applyEnvOverrides() 应用 AIOPS_* 环境变量覆盖
  └─ Validate() 校验后保存脏标记
```

### 3.2 加载顺序（Agent）

```
main()
  ├─ defaultConfig() 填充默认值
  ├─ 扫描 os.Args 查找 --config 路径
  ├─ 读取 config.json 合并
  ├─ ensureConfigExample() 首次启动生成示例
  ├─ flag.Parse() 命令行覆盖
  └─ configureServerTLS() 设置 TLS 信任
```

### 3.3 环境变量覆盖（AIOPS_*）

Server 支持的覆盖变量（按功能分组）：

| 类别 | 变量名 | 映射字段 |
|------|--------|----------|
| 外部存储 | `AIOPS_VM_URL` | `VM.Enabled`, `VM.URL` |
| 外部存储 | `AIOPS_POSTGRES_DSN` | `PostgresDSN` |
| 端口转发 | `AIOPS_FORWARD_LISTEN` | `ForwardListen` |
| 端口转发 | `AIOPS_FORWARD_PORT_RANGE` | `ForwardPortRange` |
| 安全开关 | `AIOPS_FORWARD_DISABLED` | `ForwardDisabled` |
| 安全开关 | `AIOPS_TERMINAL_DISABLED` | `TerminalDisabled` |
| 中继认证 | `AIOPS_RELAY_SECRET` | `RelaySecret` |
| 匿名 Agent | `AIOPS_ALLOW_ANONYMOUS_AGENTS` | `AllowAnonymousAgents` |
| 反向代理 | `AIOPS_TRUST_PROXY` | `TrustProxy` |
| Token 强制 | `AIOPS_REQUIRE_TOKEN` | `RequireToken` |

### 3.4 运行时持久化与回滚

- `ConfigStore.Set()` 先执行 `backfillThresholdDefaults()` 再 `Validate()`，确保零值被修复而非报错。
- 每次 `Set()` 前快照当前配置到 `prev`，提供 `Revert()` 一键回滚。
- 敏感字段（SMTP/SMS/VoiceCall/AI API Key）在保存前调用 `encryptConfigSecrets()`，使用 `AIOPS_SECRET_KEY` 做可逆加密；读取时 `decryptConfigSecrets()` 还原明文。
- 磁盘文件权限固定为 `0o600`，防止其他用户读取。

### 3.5 向后兼容与迁移

- 阈值零值视为“未配置”，自动回填标准默认值。
- 旧版单 `Account` 自动迁移到 `Users` 列表，并确保至少一个 admin。
- `InstallToken` 支持旋转：`ResetToken()` 将旧 token 放入 `PrevInstallToken`，保留 7 天宽限期。

## 4. 开发者规则

1. **新增配置字段必须三处同步更新**：
   - 结构体 tag（`json:"..."`）
   - `default*Config()` 中的默认值
   - `applyEnvOverrides()` 中对应的 `AIOPS_*` 覆盖（如适用）

2. **敏感字段必须走加密流程**：
   - 在 `save()` 之前调用 `encryptConfigSecrets()` / `decryptConfigSecrets()`
   - 前端提交空或脱敏值（含 `****`）时，`SetXxx()` 方法应保留原值

3. **阈值类字段禁止直接写 0**：
   - 所有百分比/计数阈值都经过 `backfillThresholdDefaults()` 修复，业务代码不应依赖 0 表示“禁用”

4. **环境变量命名遵循 `AIOPS_<FIELD>` 大写下划线风格**，且仅用于覆盖，不替代 JSON 主配置

5. **Agent 侧新增采集器配置**应在 `config.example.json` 中添加注释块，保持文档与代码一致

6. **安装脚本生成的配置**需保证 JSON 语法正确（特别是数组字段），参考 `install.go` 中对 `log_paths` 的处理方式