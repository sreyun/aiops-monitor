---
kind: error_handling
name: Go 服务错误处理：中间件 + panic/recover + slog 日志策略
category: error_handling
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/server/auth.go
    - cmd/server/auth_core.go
    - cmd/server/handlers.go
    - cmd/agent/forward.go
    - cmd/agent/terminal.go
    - cmd/agent/reporter.go
---

## 1. 整体方法

本仓库采用 Go 标准库 net/http，未引入第三方 Web 框架。错误处理由三层组成：
- HTTP 中间件链：统一的安全头、CORS、gzip、请求体大小限制与认证鉴权；
- panic/recover 兜底：Agent 侧长连接/转发会话 goroutine 内用 recover 捕获异常，避免单点崩溃拖垮整个进程；
- 结构化日志：使用 log/slog 记录启动失败、依赖不可用、优雅关闭等关键路径错误，无统一的错误码枚举或自定义 error 类型。

## 2. 核心文件与位置

- cmd/server/main.go：securityHeadersMiddleware/corsMiddleware/gzipMiddleware/bodyLimitMiddleware/authMiddleware 串联
- cmd/server/auth.go / auth_core.go：登录、MFA、会话、RBAC、速率限制；通过 writeJSON 返回错误 JSON
- cmd/server/handlers.go：writeJSON 统一设置 Content-Type 并写入状态码
- cmd/agent/forward.go / terminal.go / reporter.go：每个转发/终端/上报 goroutine 包裹 defer recover()，仅告警不中断进程
- cmd/server/main.go：mustOpenPG 重试后 log.Fatalf；环境变量缺失直接 log.Fatal

## 3. 架构与约定

### 3.1 中间件分层（从外到内）
securityHeadersMiddleware → corsMiddleware → gzipMiddleware → bodyLimitMiddleware → authMiddleware → Routes()
- 安全头：强制 X-Content-Type-Options: nosniff、X-Frame-Options: DENY、严格 CSP，排除 /proxy/* 隧道；
- CORS：支持白名单 Origin，否则回退为 *；
- Gzip：跳过 WebSocket 升级、terminal/forward/proxy 流式路径；
- Body 限制：默认 100 MiB，防止超大 JSON 耗尽内存；
- 认证鉴权：isPublicPath 放行静态资源、agent 注册上报、login/me 等；其余路径要求有效 session cookie 且满足 RBAC（viewer/operator/admin）。

### 3.2 错误响应格式
所有 API 错误通过 writeJSON(w, status, map[string]string{"error": ...}) 返回，消息走 i18n Tr(r, key)，前端据此展示。没有统一的错误结构体或错误码枚举。

### 3.3 panic/recover 策略
- Server 端：仅在极少数场景使用 recover()，如 sre_api.go 中防御性恢复；
- Agent 端：在 runForwardSession、runTerminalSession、reportLoop 等长期运行的 goroutine 外层 defer recover()，将 panic 降级为 slog.Warn，确保单个会话崩溃不影响 Agent 主循环。

### 3.4 启动期与依赖错误
- PostgreSQL 连接失败：mustOpenPG 指数重试 10 次，仍失败则 log.Fatalf 终止进程；
- 必需环境变量缺失（AIOPS_POSTGRES_DSN、AIOPS_VM_URL）：直接 log.Fatal 退出；
- TLS 未配置时输出警告日志，允许以明文 HTTP 运行（建议置于反向代理之后）。

### 3.5 无全局错误类型
代码中未发现自定义 type XError struct 或 errors.New("sentinel") 模式。业务错误主要通过 HTTP 状态码 + JSON error 字段表达，系统级错误通过 slog 级别区分（info/warn/fatal）。

## 4. 开发者应遵循的规则
1. 不要 panic 业务逻辑：可预期的失败一律返回错误并通过 writeJSON 返回给客户端；
2. goroutine 必须 recover：任何可能长时间运行、涉及外部 I/O 的 goroutine 外层加 defer func(){ if r:=recover();r!=nil{slog.Warn(...)}}()；
3. 错误信息走 i18n：writeJSON 中的 error 值使用 Tr(r, "key")，便于多语言；
4. 启动失败即退出：依赖不可用时使用 log.Fatal/log.Fatalf，让容器编排层感知失败；
5. 敏感错误不上报：密码校验失败、TOTP 错误等只写 warning 级别日志，不暴露具体原因细节。