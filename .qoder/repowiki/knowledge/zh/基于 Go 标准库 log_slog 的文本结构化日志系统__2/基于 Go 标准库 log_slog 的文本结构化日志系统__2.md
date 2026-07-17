---
kind: logging_system
name: 基于 Go 标准库 log/slog 的文本结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/agent/main.go
    - cmd/server/main.go
---

## 1. 使用的系统与框架
- 日志框架：Go 标准库 `log/slog`（Go 1.21+），无引入第三方日志库。
- 输出格式：纯文本（TextHandler），未启用 JSON 模式，所有日志直接写入 `os.Stderr`。
- 默认级别：`slog.LevelInfo`，Debug 级日志默认不输出。

## 2. 核心文件与初始化位置
- Agent 端初始化：`cmd/agent/main.go:79`
  - `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))`
- Server 端初始化：`cmd/server/main.go:246`
  - 同样的 TextHandler + LevelInfo 配置。
- 业务代码中通过全局 `slog.Info/Warn/Error/Debug` 调用，无需显式注入 logger 实例。

## 3. 架构与约定
- **全局默认 Logger**：两个入口进程在 main 最开头设置 `slog.Default()`，后续各模块直接使用包级函数，没有自定义 logger 封装层或中间件。
- **统一输出到 stderr**：所有日志均输出到标准错误流，便于容器编排（Docker / systemd）收集 stdout/stderr。
- **结构化字段**：广泛使用键值对形式记录上下文，如 `"target"`, `"err"`, `"addr"`, `"attempt"`, `"max"`, `"distro"`, `"module"`, `"status"` 等，便于外部日志采集器按字段解析。
- **日志级别策略**：
  - `Info`：启动、连接成功、常规状态变更（如 "已加载配置文件"、"PostgreSQL 已连接"、"Redfish 采集成功"）。
  - `Warn`：可恢复异常或需关注的问题（如安全模块 enforcing、NetFlow 读取错误、TLS 未配置）。
  - `Error`：失败且需要干预的错误（如 Redfish 连续失败退避、/proc 路径被拦截）。
  - `Debug`：调试信息，默认关闭，仅在个别采集器中使用（如 BMC 事件日志、转发通道重试延迟）。
- **与 `log` 包的混用**：启动阶段的致命错误仍使用 `log.Fatal/Fatalf` 直接退出进程；运行时业务逻辑一律走 `slog`。`fmt.Println` 仅用于一次性管理命令（admin reset）的人机交互输出。

## 4. 开发者应遵循的规则
- **统一使用 `slog`**：新增日志请使用 `slog.Info/Warn/Error/Debug`，不要再用 `fmt.Print*` 或 `log.Printf` 记录运行期日志。
- **提供结构化字段**：每条日志至少包含能定位问题的关键键值对（如 `"err"`, `"target"`, `"path"`, `"attempt"`），避免只写纯字符串消息。
- **合理选择级别**：
  - 正常流程 → `Info`
  - 可恢复异常/潜在风险 → `Warn`
  - 不可恢复错误/需要人工介入 → `Error`
  - 仅调试时可见的详细信息 → `Debug`（默认不输出）
- **敏感信息脱敏**：当前实现未做统一的敏感字段过滤，记录密码、token、证书路径等时需自行注意脱敏。
- **如需 JSON 输出或分级文件输出**：应在各自 main 中替换 `NewTextHandler` 为 `NewJSONHandler` 或自定义 Handler，目前仓库内未提供此类开关。