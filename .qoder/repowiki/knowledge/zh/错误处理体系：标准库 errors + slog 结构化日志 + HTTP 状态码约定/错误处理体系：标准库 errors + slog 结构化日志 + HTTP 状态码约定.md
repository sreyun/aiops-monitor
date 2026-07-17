---
kind: error_handling
name: 错误处理体系：标准库 errors + slog 结构化日志 + HTTP 状态码约定
category: error_handling
scope:
    - '**'
source_files:
    - cmd/server/handlers.go
    - cmd/server/main.go
    - cmd/agent/reporter.go
    - cmd/agent/collector_redfish.go
    - cmd/agent/forward.go
    - cmd/agent/terminal.go
    - cmd/server/auth_core.go
---

## 1. 采用的系统/方法
- Go 标准库 `errors` 与 `fmt.Errorf("%w", err)` 包装，用于业务层可检测的错误值。
- 全局结构化日志使用 `log/slog`（TextHandler 输出到 stderr），作为统一的错误/告警记录通道。
- HTTP API 通过自定义 `writeJSON(w, status, v)` 统一返回 JSON 响应体，错误场景以 `http.StatusBadRequest / Forbidden / InternalServerError` 等状态码区分。
- 关键 goroutine 使用 `defer recover()` 捕获 panic，避免单点崩溃影响整个进程。
- 启动期依赖缺失（PostgreSQL、VictoriaMetrics）直接 `log.Fatal` 终止进程，不静默降级。

## 2. 核心文件与位置
- `cmd/server/handlers.go`：`writeJSON` 统一 JSON 响应封装；`Server.Routes()` 集中注册路由。
- `cmd/server/main.go`：中间件链 `securityHeadersMiddleware → corsMiddleware → gzipMiddleware → bodyLimitMiddleware → authMiddleware`；优雅关停与信号处理。
- `cmd/agent/reporter.go`：定义哨兵错误 `errForbidden`、`errBadPayload`，并通过 `errors.Is` 分支处理。
- `cmd/agent/collector_redfish.go`：`classifyError` 将底层错误分类为人类可读提示。
- `cmd/agent/forward.go`、`cmd/agent/terminal.go`：在转发/终端会话 goroutine 中 `defer recover()` 捕获异常并记录 `slog.Warn`。
- `cmd/server/auth_core.go`：对不可恢复的 CSPRNG 失败使用 `panic` 主动终止。

## 3. 架构与约定
- **错误类型**：未引入第三方 error 库，采用“包级 `var errXxx = errors.New(...)` 哨兵 + `fmt.Errorf("%w", err)` 包装”的组合。调用方用 `errors.Is` 判断语义错误。
- **HTTP 错误响应**：所有 handler 通过 `writeJSON` 返回 `{"error": "..."}` 或业务字段，配合明确的 HTTP 状态码；无全局 HTTP 错误中间件，每个 handler 自行判断并写回。
- **日志即错误载体**：`slog.Error`/`Warn`/`Info` 是跨组件错误传播的主要手段，包含结构化 key-value（如 `target`, `err`, `session`），便于外部采集。
- **panic/recover 策略**：仅包裹不可控边界（插件执行、转发会话、终端 I/O），内部逻辑不 panic；初始化阶段遇到严重不一致直接 `panic` 或 `log.Fatal`。
- **中间件式防护**：`bodyLimitMiddleware` 限制请求体大小防 OOM，`securityHeadersMiddleware` 注入安全头，CORS 白名单控制跨域——这些属于“输入校验型错误预防”。

## 4. 开发者应遵循的规则
1. **返回 error 而非 panic**：业务函数一律返回 `error`，上层用 `errors.Is` 匹配哨兵错误做分支处理。
2. **使用 `%w` 包装**：在错误路径上层层 `fmt.Errorf("...: %w", err)`，保留原始错误链，禁止吞掉原错误。
3. **HTTP 错误走 writeJSON**：handler 内错误统一通过 `writeJSON(w, code, map[string]string{"error": ...})` 返回，不要直接 `json.Marshal` + `WriteHeader`。
4. **记录结构化日志**：对外部依赖失败、网络超时、权限不足等使用 `slog.Error`/`Warn`，附带上下文键（host、endpoint、session 等）。
5. **仅在边界 recover**：插件执行、长连接转发、终端流等易崩溃 goroutine 外层加 `defer recover()`，记录后继续运行；业务逻辑不 panic。
6. **启动期致命错误直接退出**：缺少 PG/VM 配置、CSPRNG 不可用等不可恢复场景使用 `log.Fatal` 或 `panic`，让容器编排负责重启。
7. **避免裸 fmt.Printf/print**：统一使用 `slog`，以便被日志系统收集与检索。