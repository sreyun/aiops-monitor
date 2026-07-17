---
kind: error_handling
name: 错误处理体系：哨兵错误 + defer/recover 兜底 + slog 结构化日志
category: error_handling
scope:
    - '**'
source_files:
    - cmd/agent/reporter.go
    - cmd/agent/forward.go
    - cmd/agent/terminal.go
    - cmd/agent/collector_linux.go
    - cmd/server/ws.go
    - cmd/server/filext.go
    - cmd/server/auth_core.go
    - cmd/server/sre_api.go
    - cmd/server/handlers.go
---

## 1. 采用的错误处理方案
- **标准库 error 语义**：使用 `errors.New` 定义少量**哨兵错误**（sentinel errors），通过 `errors.Is` 在调用方做分支判断，而非字符串匹配。
- **fmt.Errorf 包装**：业务层用 `%w` 包装底层错误，保留错误链，便于上层 `errors.Is` 探测。
- **defer/recover 兜底**：对不可信第三方库、插件执行、长生命周期 goroutine 等场景，统一用 `recover()` 捕获 panic，避免进程崩溃。
- **slog 结构化日志**：所有可恢复异常均通过 `slog.Error/Warn/Info` 输出，附带 key-value 字段（如 `err`、`server`、`target`），无集中式 logger 初始化，依赖 Go 1.21+ 默认 slog。
- **HTTP 层**：Server 侧没有统一的 HTTP 中间件，各 handler 直接 `writeJSON` 返回 JSON 错误体；Agent 的 relay 模块直接用 `http.Error` 返回文本错误。

## 2. 关键文件与位置
- 哨兵错误定义
  - `cmd/agent/reporter.go`：`errForbidden`（403）、`errBadPayload`（gzip 被代理破坏导致 400）
  - `cmd/agent/collector_linux.go`：`errParse`（解析失败）
  - `cmd/server/ws.go`：握手阶段返回裸 `errors.New` 错误（非升级、缺少 Key、不支持 Hijack、帧过大）
- recover 兜底点
  - `cmd/agent/forward.go:103`：端口转发会话 panic 恢复
  - `cmd/agent/reporter.go:409,443`：上报循环与插件执行 panic 恢复
  - `cmd/agent/terminal.go:260,342,603`：终端 PTY 读写 panic 恢复
  - `cmd/server/filext.go:285`：PDF 解析 panic 恢复（外部库不可信）
  - `cmd/server/sre_api.go:219`：AI 自动诊断异步 goroutine panic 恢复
- 必须 panic 的场景
  - `cmd/server/auth_core.go:292`：`crypto/rand.Read` 失败时直接 `panic`，因为此时生成可预测 token 比崩溃更危险
- HTTP 响应封装
  - `cmd/server/handlers.go:363`：`writeJSON` 统一设置 Content-Type 并编码 JSON 响应体
  - `cmd/agent/relay.go`：直接使用 `http.Error` 返回纯文本错误

## 3. 架构与约定
- **Agent 端**：以“健壮优先”为原则。采集器、上报循环、终端会话等长时间运行的 goroutine 都包裹 `defer recover`，确保单个组件 panic 不拖垮整个 Agent 进程。网络请求失败通过重试 + 指数退避 + 熔断（circuit breaker）组合处理，并在 slog 中记录上下文。
- **Server 端**：对不可信输入（PDF、URL 抓取、WebSocket 帧大小）使用 recover 隔离风险；对安全相关的关键路径（CSPRNG 失败）选择 panic，让进程快速失败而非继续运行在不安全状态。HTTP handler 内部自行处理错误并返回 JSON，未引入全局中间件。
- **错误分类**：通过哨兵错误区分“需要重注册”、“需要降级 gzip”、“权限不足”等可恢复情形，其余一律包装为普通 error 上抛。

## 4. 开发者应遵循的规则
1. **可恢复的业务错误**：使用 `fmt.Errorf("...: %w", err)` 包装底层错误，不要丢弃原始错误链。
2. **需要分支处理的错误**：在包级定义 `var errXxx = errors.New(...)` 哨兵错误，调用方用 `errors.Is(err, errXxx)` 判断。
3. **不可信第三方库调用**：在函数入口加 `defer func(){ if r:=recover();r!=nil{ err=... } }()`，将 panic 转为 error 返回。
4. **长生命周期 goroutine**（采集循环、上报循环、终端会话）：外层包裹 `defer recover()`，记录 `slog.Warn` 后继续运行。
5. **安全关键路径**（随机数源、加密密钥生成）：失败时直接 `panic`，不应静默降级。
6. **HTTP 响应**：Server 侧统一使用 `writeJSON(w, status, map[string]string{"error": ...})`；Agent 侧 relay 可直接用 `http.Error`。
7. **日志**：所有错误路径必须通过 `slog.Error/Warn` 输出，带上 `err` 字段及必要上下文键（server、target、host_id 等），禁止仅 `fmt.Println`。