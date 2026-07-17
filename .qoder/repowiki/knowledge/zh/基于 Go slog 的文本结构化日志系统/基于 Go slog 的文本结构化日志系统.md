---
kind: logging_system
name: 基于 Go slog 的文本结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/agent/main.go
    - cmd/server/main.go
    - cmd/agent/logcollect.go
---

## 系统概述

AIOps 监控平台在 Agent 与 Server 两端统一使用 Go 标准库 `log/slog` 作为日志框架，采用**纯文本（TextHandler）输出到 stderr** 的形式，不引入第三方日志库。日志通过结构化键值对记录关键上下文，便于外部日志采集器（如 systemd journal、Docker log driver、ELK/Fluentd）进行解析。

## 初始化与配置

- **Agent 端**：`cmd/agent/main.go:79` 启动时设置全局默认 logger，级别固定为 `slog.LevelInfo`，输出到 `os.Stderr`。
- **Server 端**：`cmd/server/main.go:246` 同样以 `LevelInfo` 写入 stderr。
- 当前代码中**没有提供运行时调整日志级别的机制**（无 `-log-level` flag），也未实现 JSON handler 或文件 sink；所有日志均为进程标准错误流输出，依赖容器化或进程管理器收集。

## 日志级别约定

| 级别 | 使用场景 | 示例 |
|------|----------|------|
| `Debug` | 调试信息，如转发通道重试延迟 | `forward.go:65` |
| `Info` | 正常业务流程事件：启动、连接成功、周期任务刷新等 | `collector_netflow.go:215`、`main.go:325` |
| `Warn` | 可恢复异常或潜在风险：读取失败、安全模块 enforcing、未启用 TLS 等 | `collector_redfish.go:78`、`main.go:350` |
| `Error` | 严重错误：权限不足、连续失败退避、PostgreSQL 连接失败等 | `collector_linux.go:197`、`main.go:223` |

## 结构化字段规范

所有 `slog.Info/Warn/Error` 调用均采用**命名参数键值对**形式附加上下文，常见键包括：
- `err` / `error` — 错误对象
- `server` / `target` — 远端地址或目标标识
- `path` / `file` — 文件路径
- `attempt` / `max` — 重试计数
- `session` / `sid` — 会话标识
- `hostID` / `fingerprint` — 主机身份
- `level` / `version` / `protocol` — 协议/版本信息

这些键名保持语义一致，便于下游日志分析工具按字段聚合。

## 日志采集与上报（Agent → Server）

除程序自身运行日志外，Agent 还具备**业务日志采集能力**（`cmd/agent/logcollect.go`）：
- 支持 tail 指定文件或目录下的 `.log/.out/.err/.txt` 及轮转文件
- 每 10 秒批量收集新行，自动检测文件旋转（size < offset）并重新从头读取
- 每条日志经 `classifyLogLevel()` 推断 level（error/warn/debug/info），并附带 `Source` 原始路径
- 可选 gzip + AES-256-GCM 加密后 POST 至 `/api/v1/agent/logs`
- 服务端接收后落库 PostgreSQL，供审计与检索

## 设计决策与约束

1. **统一使用标准库 slog**：避免引入第三方依赖，降低二进制体积与供应链风险。
2. **纯文本输出**：不内嵌 JSON formatter，由部署环境（systemd/Docker/K8s）负责结构化解析。
3. **无动态级别切换**：日志级别在进程启动时固化，不支持热更新。
4. **无本地文件落盘**：进程日志仅走 stderr，持久化完全交由外部日志系统。
5. **业务日志独立通道**：被采集的应用日志通过专用 HTTP API 上报，与程序运行日志解耦。

## 开发者规范

- 新增日志请使用 `slog.Info/Warn/Error` 并附带有意义的键值对，遵循现有键名约定。
- 敏感信息（token、密钥、密码）**不得**直接写入日志字段。
- 高频路径（如网络请求循环）优先使用 `Debug` 级别，避免刷屏。
- 需要持久化的业务日志应通过 `sendLogBatch` 上报而非写本地文件。