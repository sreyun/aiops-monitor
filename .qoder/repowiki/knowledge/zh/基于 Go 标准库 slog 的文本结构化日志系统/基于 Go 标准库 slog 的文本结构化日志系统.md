---
kind: logging_system
name: 基于 Go 标准库 slog 的文本结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/agent/main.go
    - cmd/server/main.go
    - cmd/server/logstore.go
    - cmd/agent/logcollect.go
---

## 1. 使用的框架与工具
- 统一采用 Go 标准库 `log/slog`，未引入第三方日志框架（如 logrus、zap、zerolog）。
- 输出格式为**纯文本**（`slog.NewTextHandler`），非 JSON；默认级别为 `INFO`。
- 日志通过 `slog.SetDefault(...)` 在进程启动时设置全局默认 logger，所有包直接调用 `slog.Info/Warn/Error/Debug` 即可使用。

## 2. 核心文件与位置
- **Agent 侧初始化**：`cmd/agent/main.go` 中 `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))`
- **Server 侧初始化**：`cmd/server/main.go` 中相同模式设置全局 slog，并写入 PostgreSQL 连接、TLS、存储后端等关键运行信息。
- **应用日志采集子系统**：`cmd/server/logstore.go` 提供内存环形缓冲 + 可选 PG 持久化的“日志聚合”能力，用于接收 Agent 上报的应用日志行，供 Dashboard 搜索与 AI 分析。
- **Agent 日志采集模块**：`cmd/agent/logcollect.go` 负责 tail 本地 `.log/.out/.err/.txt` 及轮转文件，加密后 POST 到 Server 的 `/api/v1/logs`。

## 3. 架构与约定
- **双轨日志**：
  - **平台自身运行日志**：由 `slog` 输出到 stderr，文本格式，便于容器 stdout/stderr 收集或 systemd journal 抓取。
  - **被监控应用日志**：通过 Agent 的 `LogPaths` 配置 tail 目标文件，经 gzip+AES-256-GCM 加密后批量上报，Server 端以 `StoredLog{Ts, HostID, Hostname, Source, Level, Message}` 结构入内存环（上限 50000 条），并按 `logPersistCap=8000` 周期性落盘 PG，重启后可恢复最近窗口。
- **级别规范化**：Server 端的 `normalizeLevel` 将 error/warn/info/debug 以外的拼写归一化到四档，保证前端筛选一致。
- **字段规范**：`slog` 调用一律以 key-value 形式附加结构化字段（如 `server`, `session`, `target`, `err`, `distro`, `module` 等），便于后续按字段过滤。
- **安全脱敏**：`maskToken` 等辅助函数确保 token 等敏感值不会原样出现在日志中。
- **Windows 控制台兼容**：`console_windows.go` 处理 UTF-8 编码问题，避免 Windows 终端下中文日志乱码。

## 4. 开发者应遵循的规则
- **禁止使用 `fmt.Print*` / `log.Printf` 输出业务日志**，统一使用 `slog.Info/Warn/Error/Debug`。
- **必须附带结构化字段**：至少包含上下文键（如 `host_id`、`session`、`err`），不要只写纯字符串消息。
- **级别选择**：启动/配置类信息用 `Info`，可恢复异常用 `Warn`，不可恢复错误用 `Error`，调试细节用 `Debug`。
- **不要在日志中输出明文 token、密码、密钥**，如需记录请走 `maskToken` 或自行截断。
- **新增日志采集源**：在 Agent 的 `config.LogPaths` 中声明路径，保持 `.log/.out/.err/.txt` 命名约定，以便 `logcollect.go` 自动发现轮转文件。
- **Server 扩展点**：若需持久化更多审计日志，参考 `logstore.go` 的 ring buffer + PG export/import 模式，避免无界增长。