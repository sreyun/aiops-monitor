---
kind: error_handling
name: 错误处理体系：哨兵错误、panic/recover 与中间件兜底
category: error_handling
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/server/handlers.go
    - cmd/agent/reporter.go
    - cmd/agent/forward.go
    - cmd/agent/terminal.go
    - cmd/agent/collector_redfish.go
    - cmd/server/terminal_auth.go
---

## 1. 整体方法

本仓库采用 Go 原生 error 接口，结合包级哨兵错误变量、自定义错误类型、defer/recover 兜底以及 HTTP 中间件统一包装的模式。Server 端以中间件链集中处理安全/压缩/限流等横切问题；Agent 端在关键长生命周期 goroutine 上广泛使用 recover，保证单个采集器或会话崩溃不拖垮整个进程。

## 2. 核心文件与位置

- Server 启动与中间件链：cmd/server/main.go（securityHeadersMiddleware、bodyLimitMiddleware、gzipMiddleware、corsMiddleware、authMiddleware）
- HTTP 路由与响应辅助：cmd/server/handlers.go（writeJSON、Server.Routes()）
- Agent 上报与重试：cmd/agent/reporter.go（errForbidden、errBadPayload、sendWithRetry、circuit breaker）
- Agent 会话 panic 恢复：cmd/agent/forward.go、cmd/agent/terminal.go、cmd/agent/reporter.go（多处 defer recover）
- 领域错误类型：cmd/agent/collector_redfish.go（rfHTTPError）、cmd/server/terminal_auth.go（validationError）

## 3. 架构与约定

### 3.1 哨兵错误（sentinel errors）
- cmd/agent/reporter.go 定义包级 var errForbidden = errors.New("forbidden")、var errBadPayload = errors.New("bad payload (server returned 400)")，通过 errors.Is 判断并触发重注册或 gzip 降级。
- cmd/agent/collector_oceanstor.go 中 osError 结构体用于区分存储设备认证失败。

### 3.2 自定义错误类型
- cmd/agent/collector_redfish.go 的 rfHTTPError 携带 HTTP status/path/body，并通过 errors.As 提取状态码，便于上层按 404/401/5xx 分类处理。
- cmd/server/terminal_auth.go 的 validationError 实现 Error() 返回 i18n key，由调用方用 Tr/Tz 渲染为多语言消息。

### 3.3 panic/recover 策略
- Agent 侧对每个独立会话/循环包裹 defer func(){ if r := recover(); r != nil { slog.Warn(...); } }()，包括转发会话（forward.go）、终端会话（terminal.go）、插件循环（reporter.go），确保单点 panic 仅记录日志而不终止进程。
- Server 侧仅在极少数不可恢复场景使用 panic（如 crypto/rand.Read 失败），其余路径一律返回 error。

### 3.4 HTTP 层错误处理
- 所有响应经 handlers.go 中的 writeJSON(status, v) 统一写入 JSON body，避免手写 http.Error。
- 中间件链顺序：securityHeaders -> cors -> gzip -> bodyLimit -> auth -> Routes，其中 bodyLimitMiddleware 用 MaxBytesReader 防止超大请求体 OOM；securityHeadersMiddleware 注入 CSP/X-Frame-Options 等头，非 /proxy/ 路径强制严格策略。
- 启动期依赖缺失直接 log.Fatal（PG DSN、VM URL 未配置时），拒绝静默降级。

### 3.5 网络错误与重试
- Agent 上报链路内置指数退避加熔断器（每目标独立），403 自动重注册，400+gzip 自动禁用压缩并重试，连续失败打开断路器冷却 15s。
- Server 启动时对 PG 连接做有限次重试，超时后 log.Fatalf 终止。

## 4. 开发者应遵循的规则

1. 优先返回 error，而非 panic：业务逻辑错误一律 return error；仅在程序状态已损坏且无法继续时使用 panic。
2. 可被 errors.Is 判断的错误定义为包级哨兵变量，并在注释说明触发条件（参考 errForbidden、errBadPayload）。
3. 需要携带结构化信息的错误定义新类型并实现 Error()，必要时提供 As 友好的字段（参考 rfHTTPError）。
4. 长生命周期 goroutine 必须 recover：任何可能因外部数据异常而崩溃的循环/会话，外层加 defer recover 并记录 slog.Warn/Error。
5. HTTP 响应统一走 writeJSON，不要直接 fmt.Fprint 或裸 http.Error，以保证 Content-Type 和编码一致。
6. 敏感错误信息不要原样输出到客户端：i18n key 通过 validationError 间接翻译，避免泄露内部细节。
7. 对外部依赖（DB、VM、第三方 API）失败要可重试/可降级，但启动阶段的关键依赖缺失应直接终止，避免半可用状态。