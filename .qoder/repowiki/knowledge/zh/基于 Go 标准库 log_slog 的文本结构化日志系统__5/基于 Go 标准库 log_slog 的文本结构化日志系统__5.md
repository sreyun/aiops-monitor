---
kind: logging_system
name: 基于 Go 标准库 log/slog 的文本结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/agent/main.go
    - cmd/server/main.go
    - cmd/agent/collector_netflow.go
    - cmd/agent/collector_redfish.go
    - cmd/agent/collector_packet.go
---

## 1. 使用的系统与框架
- 统一采用 Go 1.21+ 标准库 `log/slog`，未引入第三方日志框架（如 zap、logrus、zerolog）。
- 两个二进制入口（Agent 与 Server）均在各自 `main()` 中通过 `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, ...)))` 设置全局默认 logger，输出为人类可读的 Text 格式到 stderr。
- 日志级别使用 slog 内置四级：`Debug` / `Info` / `Warn` / `Error`，默认过滤阈值为 `LevelInfo`，即 Debug 级别在默认运行模式下不会输出。

## 2. 关键文件与位置
- Agent 初始化：`cmd/agent/main.go` — 启动时设置默认 slog handler，并输出 OS 发行版、安全模块检测等上下文信息。
- Server 初始化：`cmd/server/main.go` — 同样设置默认 slog handler，并在 PG 连接、TLS 启用、dist 目录解析等关键路径打点。
- 业务调用方遍布各采集器与 API 处理文件，例如：
  - `cmd/agent/collector_netflow.go`、`collector_packet.go`、`collector_redfish.go`、`collector_linux.go`
  - `cmd/server/*.go`（认证、告警、存储、终端等模块均直接使用 `slog.Info/Warn/Error`）

## 3. 架构与约定
- **全局默认 Logger**：每个进程只设置一次全局默认 slog，所有包直接调用 `slog.Info(...)` 等，无需显式注入 logger 实例。
- **统一 Handler**：`NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: LevelInfo})`，意味着所有日志都输出到进程 stderr，由外部容器/进程管理器（systemd、Docker、supervisor）负责收集与转发。
- **结构化字段**：所有日志调用均采用 key-value 参数形式，如 `"err", err`、`"target", t.Name`、`"attempt", i+1`，便于被 JSON 化或按字段检索。
- **日志级别策略**：
  - `Info`：正常业务流程事件（服务启动、配置加载、连接成功、采集完成）。
  - `Warn`：可恢复异常或需关注状态（安全模块 enforcing、重试、跳过平台不支持功能）。
  - `Error`：不可恢复错误（网络失败、权限不足、连续失败触发退避）。
  - `Debug`：仅用于开发调试（如 BMC 事件日志采样），默认不输出。
- **国际化结合**：Server 侧部分启动日志通过 `Tz("server.started")` 等键值从 i18n 资源加载中文文案，再作为消息体传入 slog，实现多语言日志文案。

## 4. 开发者应遵循的规则
1. **始终使用 `log/slog`**：新增代码不得再使用 `fmt.Println`、`log.Print` 或第三方日志库；需要输出时使用 `slog.Info/Warn/Error(Debug)`。
2. **提供结构化字段**：每条日志都应包含至少一个能定位上下文的 key（如 `"err"`、`"target"`、`"attempt"`、`"path"`），避免纯字符串日志。
3. **合理选择级别**：
   - 用户可见的“发生了什么”用 `Info`；
   - 可能影响功能但可自动恢复的用 `Warn`；
   - 明确失败且需要人工介入的用 `Error`；
   - 仅在本地调试时开启的细粒度信息用 `Debug`。
4. **不要自行创建 logger 实例**：沿用全局默认 slog，除非有明确的子进程/插件隔离需求。
5. **敏感信息脱敏**：密码、token、证书路径等不应直接写入日志字段；如需记录，请做掩码处理。
6. **i18n 文案优先**：面向用户的提示性日志建议使用 `Tz("...")` 包装，以便后续支持多语言。