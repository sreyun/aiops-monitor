---
kind: logging_system
name: 基于 Go 标准库 log/slog 的文本结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/agent/main.go
    - cmd/agent/collector_netflow.go
    - cmd/agent/collector_redfish.go
    - cmd/agent/collector_packet.go
    - cmd/agent/forward.go
    - cmd/agent/reporter.go
---

## 1. 使用的框架与工具
- 统一采用 Go 1.21+ 内置 `log/slog` 作为日志框架，未引入第三方日志库（如 zap、logrus、zerolog）。
- 所有进程（Server 与 Agent）在各自 `main()` 中通过 `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))` 设置全局默认 logger，输出到 stderr，格式为纯文本（TextHandler），默认级别 `INFO`。
- 启动前/初始化阶段仍使用标准库 `log.Fatal/Fatalf` 输出致命错误并退出；业务运行期全部走 `slog.Info/Warn/Error`。

## 2. 核心文件与位置
- Server 端：
  - `cmd/server/main.go` — 入口，设置 slog 默认 handler，输出服务启动、PG/VM 连接、TLS、dist 目录等关键信息。
  - `cmd/server/config.go` 及其他业务文件 — 通过 `slog.Warn/Info/Error` 记录配置加载、阈值回填、告警评估等事件。
- Agent 端：
  - `cmd/agent/main.go` — 入口，设置 slog 默认 handler，输出安全模块检测、操作系统发行版、采集器状态等。
  - `cmd/agent/collector_*.go`、`forward.go`、`reporter.go`、`terminal.go`、`tls.go`、`zmodem.go` 等 — 广泛使用 `slog.Info/Warn/Error` 记录采集、转发、终端会话、TLS 握手等运行时事件。

## 3. 架构与约定
- **全局单例**：每个二进制在 `main` 中调用 `slog.SetDefault(...)`，后续任意包直接 `slog.Info(...)` 即可，无需依赖注入。
- **输出目标**：`os.Stderr` + `TextHandler`，适合容器化场景下由 Docker/编排系统收集 stderr 流；无 JSON 结构化输出、无独立日志文件 sink。
- **日志级别策略**：
  - `INFO`：正常业务流程（服务启动、配置加载、采集器启动、成功完成）。
  - `WARN`：可恢复异常或需关注的问题（TLS 未启用、安全模块 enforcing、Redfish 失败重试、PostgreSQL 连接重试）。
  - `ERROR`：不可恢复错误（权限不足、连续失败退避、切换安全模式失败）。
  - 未见 `DEBUG` 级别的使用，当前默认 Level 即 `INFO`，无法动态降级到 DEBUG。
- **结构化字段**：大量使用 `slog` 键值对形式，如 `"target", t.Name`、`"err", err`、`"attempt", i+1`、`"url", "http://localhost"+*addr` 等，便于外部日志聚合系统按字段检索。
- **国际化结合**：部分启动信息通过 `Tz("server.started")` 等函数获取本地化字符串后再传入 `slog.Info`，实现多语言日志提示。
- **与旧 `log` 包的混用**：仅保留在 `log.Fatal/Fatalf` 用于进程级致命错误退出，以及 `admin_reset.go` 中一次性打印重置结果；业务路径已迁移至 `slog`。

## 4. 开发者应遵循的规则
1. **统一使用 `slog`**：新增日志一律通过 `slog.Info/Warn/Error`，不要再用 `fmt.Println` / `log.Printf` 输出业务日志。
2. **提供结构化字段**：每条日志至少包含上下文键值对（如 `"host"`, `"module"`, `"err"`），避免纯字符串消息，便于聚合分析。
3. **合理选择级别**：
   - 正常流程 → `Info`
   - 可恢复异常/需关注 → `Warn`
   - 不可恢复错误/需要立即干预 → `Error`
4. **敏感信息脱敏**：密码、token、密钥等不应直接写入日志字段；如需记录，请做掩码处理。
5. **不依赖本地日志文件**：当前无文件 sink，若需落盘应由部署层（Docker stdout/stderr 重定向、systemd journal、fluent-bit 等）负责。
6. **调试级别**：如需更详细日志，可在启动时修改 `HandlerOptions.Level` 为 `slog.LevelDebug`，但仓库内尚未出现 DEBUG 日志点，建议先补充再开启。