---
kind: logging_system
name: 基于 Go 标准库 slog 的结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/agent/main.go
    - cmd/agent/collector_netflow.go
    - cmd/agent/collector_redfish.go
    - cmd/agent/forward.go
---

## 系统概述

本项目采用 Go 1.21+ 内置的 `log/slog` 作为统一日志框架，在 Agent 与 Server 两个二进制入口中分别初始化全局默认 logger，输出格式为纯文本（TextHandler），目标输出为 stderr。未引入第三方日志库（如 zap、zerolog、logrus），保持最小依赖。

## 关键文件与包

- **Server 端初始化**：`cmd/server/main.go` — 在启动早期调用 `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))`，将全局日志级别设为 Info，输出到 stderr。
- **Agent 端初始化**：`cmd/agent/main.go` — 同样在 main 开头设置全局 slog，配置一致。
- **日志采集上报**：`cmd/agent/logcollect.go` — 负责 tail 本地日志文件并通过加密通道上报到服务端，属于“被采集的日志”而非应用自身日志。
- **使用点示例**：`cmd/agent/collector_netflow.go`、`cmd/agent/collector_redfish.go`、`cmd/agent/forward.go` 等大量业务模块直接调用 `slog.Info/Warn/Error/Debug`。

## 架构与约定

### 日志级别策略
- 全局默认级别为 `slog.LevelInfo`，即 Debug 级别消息默认不输出。
- 各模块按语义选择级别：
  - `slog.Info`：正常业务流程事件（服务启动、连接建立、采集成功）。
  - `slog.Warn`：可恢复异常或需关注的情形（网络错误、安全模块拦截、连续失败退避）。
  - `slog.Error`：需要立即关注的故障（认证失败、端口监听失败）。
  - `slog.Debug`：调试级信息（转发重试延迟），生产环境默认不可见。
  - `slog.Fatal`：仅用于致命错误导致进程退出（如缺少必需环境变量），通过标准库 `log.Fatal` 调用。

### 结构化字段
所有 slog 调用均采用键值对形式附加上下文，常见字段包括：
- `err` / `error`：错误对象
- `target` / `server`：目标主机或服务地址
- `path` / `addr`：路径或监听地址
- `attempt` / `retry`：重试计数
- `os` / `distro`：操作系统信息
- `session` / `sid`：会话标识
- `interval` / `delay`：时间参数

### 输出格式与路由
- 输出格式：纯文本（TextHandler），便于直接在终端查看或通过 systemd/docker 收集。
- 输出目标：stderr，符合云原生容器日志收集惯例（stdout/stderr 由容器运行时接管）。
- 无自定义 Handler 或 sink 抽象，日志无法在运行时切换 JSON 格式或重定向到文件。

### 遗留代码混合
部分启动阶段和工具命令仍使用标准库 `log.Fatal` / `fmt.Println` 输出一次性提示（如 admin 密码重置、缺失环境变量），这些不属于结构化日志范畴，主要用于 CLI 交互场景。

## 开发者应遵循的规则

1. **统一使用 `slog`**：新增业务逻辑一律通过 `slog.Info/Warn/Error/Debug` 记录日志，禁止再使用 `fmt.Printf` 输出运行期日志。
2. **附带结构化字段**：每条日志至少包含 `err` 字段（如有错误），并补充能定位问题的上下文键（如 `target`、`path`、`session`）。
3. **合理选择级别**：可恢复问题用 Warn，不可恢复错误用 Error，调试信息用 Debug；Info 仅记录真正重要的业务事件。
4. **避免敏感信息**：不要在日志中输出密码、token、证书内容等敏感数据。
5. **进程退出使用 `log.Fatal`**：仅在必须终止进程时使用，其余情况返回错误由上层处理。
6. **如需 JSON 格式或远程 sink**：应在各自 main 中替换 TextHandler 为 JSONHandler 或自定义 Handler，目前项目未提供运行时切换机制。