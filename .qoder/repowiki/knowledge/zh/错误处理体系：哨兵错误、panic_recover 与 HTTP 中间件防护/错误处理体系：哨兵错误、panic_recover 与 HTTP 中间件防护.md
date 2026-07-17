---
kind: error_handling
name: 错误处理体系：哨兵错误、panic/recover 与 HTTP 中间件防护
category: error_handling
scope:
    - '**'
source_files:
    - cmd/agent/reporter.go
    - cmd/agent/forward.go
    - cmd/agent/terminal.go
    - cmd/agent/security_linux.go
    - cmd/server/main.go
    - cmd/server/handlers.go
    - cmd/server/auth_core.go
---

## 1. 整体方案概览

本仓库采用 Go 标准库原生的错误处理方式，结合少量自定义哨兵错误（sentinel errors）和 panic/recover 兜底策略，在 Agent 侧形成“可恢复的长驻进程”模型；Server 侧通过 HTTP 中间件链统一做安全/限流/压缩等横切处理，业务 handler 内部以 `errors.Is` 匹配哨兵错误驱动重试、降级与断路器。Android 客户端使用 Kotlin 协程 + Retrofit，异常由上层 UI 层捕获并提示用户，无全局错误类型定义。

## 2. 关键文件与位置

- **Agent 上报与错误分类**
  - `cmd/agent/reporter.go`：定义 `errForbidden`、`errBadPayload` 两个包级哨兵错误，配合 `sendWithRetry` 实现 403 自动重注册、400 gzip 损坏时禁用压缩、指数退避 + 熔断器。
  - `cmd/agent/forward.go`：转发会话 goroutine 用 `defer recover()` 包裹，确保单个会话 panic 不拖垮整个 Agent。
  - `cmd/agent/terminal.go`、`cmd/agent/zmodem.go`：终端与 ZModem 子协议同样在每个 I/O goroutine 内 recover。
  - `cmd/agent/security_linux.go`：封装 `isPermissionError` 判断权限类错误，供 SELinux/KySec 切换逻辑分支处理。
  - `cmd/agent/modules.go`、`collector_redfish.go`：对平台差异/外部 API 失败返回 `fmt.Errorf(...)` 带 `%w` 包装，便于上层 `errors.Is` 或日志诊断。

- **Server 启动与中间件链**
  - `cmd/server/main.go`：组装中间件链 `securityHeadersMiddleware → corsMiddleware → gzipMiddleware → bodyLimitMiddleware → authMiddleware → Routes`；`mustOpenPG` 在启动阶段对 PG 连接做有限次重试，失败则 `log.Fatalf` 终止进程。
  - `cmd/server/handlers.go`：集中路由注册，所有业务 handler 均通过上述中间件链，统一获得 CORS、CSP、BodySize 限制、gzip 压缩等保护。
  - `cmd/server/auth_core.go`：密码哈希迁移（SHA-256→PBKDF2）、登录 IP/账号双维度暴力破解限流、TOTP 单码防重放、session 滑动过期；其中 `crypto/rand.Read` 失败直接 `panic`，因为生成可预测 token 比崩溃更危险。

- **Android 客户端**
  - `android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt`、`ApiService.kt`：基于 Retrofit 的网络层，异常由 ViewModel/UI 层捕获并展示 Toast/Snackbar，无全局错误类型定义。

## 3. 架构与设计约定

### 3.1 哨兵错误 + 语义化重试
- 将“服务端拒绝（403）”、“gzip 被代理损坏（400）”抽象为包级 `var errForbidden = errors.New(...)`、`var errBadPayload = errors.New(...)`，调用方通过 `errors.Is(err, errForbidden)` 精确分支，避免字符串比较。
- `sendWithRetry` 在同一报告周期内最多重试 3 次，间隔 1s；遇到 403 先重新注册再重试，遇到 400+gzip 则关闭该 target 的 gzip 开关后重试，其他网络/5xx 错误走通用重试路径。

### 3.2 每目标独立熔断器 + 指数退避
- 每个 `serverTarget` 持有独立的 `circuitBreaker`（8 次连续失败打开，冷却 15s）和 `backoff`（1s~60s），一个后端故障不会阻塞对其他健康后端的上报。
- 熔断器打开时会重置 `registered=false`，下次成功恢复后触发重新注册，保证服务端状态一致性。

### 3.3 panic/recover 作为最后防线
- Agent 的核心循环（`reportOnceSafe`、`pluginLoop`、`runForwardSession`、终端各 goroutine）全部用 `defer func(){ if r:=recover(); r!=nil{ slog.Error(...) } }()` 包裹，确保采集/上报/转发的任何 nil dereference 或越界都不会杀死进程。
- Server 仅在不可恢复的安全场景（CSPRNG 不可用）中主动 `panic`，其余业务错误一律返回 error。

### 3.4 HTTP 中间件链式防护
- `bodyLimitMiddleware`：`http.MaxBytesReader(100MB)` 防止超大 JSON 打爆内存。
- `securityHeadersMiddleware`：注入 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、严格 CSP（script-src 'self'，禁止 inline script）。
- `corsMiddleware`：按白名单回显 `Access-Control-Allow-Origin`，未配置时兼容旧版 `*`。
- `gzipMiddleware`：复用 `sync.Pool` 中的 `gzip.Writer`，跳过 WebSocket/terminal/forward/proxy 等流式端点。

### 3.5 启动期致命错误
- `main` 中对环境变量缺失（DSN、VM URL）、PostgreSQL 连接失败（重试 10 次后 `log.Fatalf`）直接终止进程，避免服务在“半可用”状态下对外暴露。

## 4. 开发者应遵循的规则

1. **优先返回 error，不要 panic**
   业务逻辑错误一律返回 `error`，仅当状态已不可恢复且继续运行会引入安全风险时才 `panic`（如随机数源失效）。

2. **使用哨兵错误表达可分支的错误**
   需要让调用方区分“认证失败”、“请求体损坏”、“网络超时”等语义时，定义包级 `var errXXX = errors.New(...)`，并通过 `errors.Is` 匹配。

3. **错误包装保留上下文**
   使用 `fmt.Errorf("...: %w", err)` 包装底层错误，方便上层 `errors.Is` 同时匹配原始哨兵错误。

4. **长生命周期 goroutine 必须 recover**
   任何可能长期运行的 goroutine（采集循环、转发会话、终端 I/O）应在入口处 `defer recover()`，记录堆栈后继续运行。

5. **HTTP 入口统一经中间件链**
   新增路由通过 `Routes()` 注册，自动获得安全头、CORS、Body 大小限制、gzip 压缩；不要在 handler 内重复实现这些横切逻辑。

6. **启动阶段失败即退出**
   依赖的外部服务（PostgreSQL、VictoriaMetrics）在启动时检查并失败则 `log.Fatal`，避免静默降级。

7. **Agent 侧幂等与去重**
   事件上报失败时仅在“所有目标都失败”的情况下才重新入队，允许同一事件在不同后端重复投递，但避免对同一目标重复重发。
