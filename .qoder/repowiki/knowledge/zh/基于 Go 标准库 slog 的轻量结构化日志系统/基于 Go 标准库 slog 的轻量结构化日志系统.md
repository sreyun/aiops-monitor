---
kind: logging_system
name: 基于 Go 标准库 slog 的轻量结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/agent/main.go
    - cmd/server/logstore.go
---

## 1. 使用的系统与框架
- **Go 标准库 `log/slog`**：Server 与 Agent 均使用 Go 1.21+ 内置的结构化日志包，未引入第三方日志框架（如 logrus、zap、zerolog）。
- **输出目标**：默认通过 `slog.NewTextHandler(os.Stderr, ...)` 以纯文本形式输出到标准错误流，便于容器/进程管理器收集。
- **日志级别**：统一使用 `slog.LevelDebug / Info / Warn / Error` 四级；在业务层还定义了应用级 level 归一化（error/warn/info/debug），用于服务端内存日志环的存储与检索。

## 2. 关键文件与位置
- `cmd/server/main.go`：服务启动入口，初始化全局 `slog` 默认处理器，设置 Level=Info，并在启动/关闭流程中记录关键事件。
- `cmd/agent/main.go`：Agent 入口，同样在 main 最开头设置全局 slog 默认处理器，随后加载配置并记录运行期信息。
- `cmd/server/logstore.go`：服务端“日志聚合”子系统，负责接收 Agent 上报的应用日志行、维护内存环形缓冲区、提供搜索与统计接口（非运行时程序自身日志，而是被监控对象的应用日志）。
- 各采集器文件（`collector_*.go`、`forward.go`、`reporter.go`、`terminal.go`、`tls.go`、`zmodem.go` 等）：广泛使用 `slog.Info/Warn/Error` 记录采集状态、网络错误、安全模块检测等信息。

## 3. 架构与设计约定
- **全局单例模式**：每个二进制在 `main()` 最早期调用 `slog.SetDefault(slog.New(...))` 设置全局默认 logger，之后所有包直接调用 `slog.Info/...` 即可，无需显式传递 logger 实例。
- **无动态级别切换**：当前未在运行时暴露调整日志级别的 API，Level 固定为 Info；如需调试需重启进程或修改源码。
- **结构化字段**：所有业务日志均以键值对形式附加上下文（如 `host_id`、`hostname`、`err`、`attempt`、`target`、`url`、`path` 等），便于外部日志系统按字段过滤与聚合。
- **多语言文案**：部分启动提示通过 `Tz(...)` 函数进行 i18n 翻译后再写入 slog，保证日志面向中文用户。
- **进程退出路径**：严重错误仍使用 `log.Fatal/Fatalf` 直接终止进程（例如缺少必要环境变量、PostgreSQL 连接失败、Relay 启动失败等），这类属于“启动阶段致命错误”，不经过 slog。
- **与“应用日志采集”解耦**：`logstore.go` 是平台自身的“被监控日志”能力（Agent tail 文件 → Server 内存 ring buffer → Web UI 搜索），与程序自身的运行日志（slog）职责分离，互不影响。

## 4. 开发者应遵循的规则
- **统一使用 `log/slog`**：新增代码一律通过 `slog.Info/Warn/Error` 记录，不要引入新的第三方日志库。
- **附带结构化上下文**：每条日志至少包含可定位上下文的 key-value（如 host_id、operation、err），避免只写纯文本消息。
- **合理选择级别**：启动失败/依赖不可用 → `Error`；可恢复异常/降级 → `Warn`；重要生命周期事件 → `Info`；仅开发调试用的细节 → `Debug`。
- **敏感信息脱敏**：密码、token、证书内容等不应直接写入日志字段；若必须记录，请做掩码处理。
- **启动阶段致命错误可用 `log.Fatal`**：仅在进程初始化阶段、无法继续运行的情况下使用；正常运行中的错误请使用 `slog.Error` 并返回 error。
- **不要自行创建多个 logger 实例**：保持全局默认 logger 的一致性，除非有明确的 sink 隔离需求。