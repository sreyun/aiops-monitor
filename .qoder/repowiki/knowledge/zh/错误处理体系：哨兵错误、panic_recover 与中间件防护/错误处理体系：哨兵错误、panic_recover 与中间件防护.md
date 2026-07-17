---
kind: error_handling
name: 错误处理体系：哨兵错误、panic/recover 与中间件防护
category: error_handling
scope:
    - '**'
source_files:
    - cmd/agent/reporter.go
    - cmd/agent/forward.go
    - cmd/agent/terminal.go
    - cmd/server/main.go
    - cmd/server/auth_core.go
    - cmd/server/ws.go
    - cmd/server/pgstore.go
    - cmd/server/forward.go
---

## 1. 整体策略

本仓库采用分层兜底与明确传播的错误处理模型：
- 启动/配置阶段：使用 log.Fatal / log.Fatalf 直接终止进程，避免在不可恢复状态下继续运行。
- 运行时关键路径（Agent 采集/上报循环、转发会话）：用 defer recover() 包裹，捕获 panic 并记录日志后自愈重启，保证守护进程不崩溃。
- 业务错误：通过返回 error 向上层传播；对需要跨层判断的语义化错误，定义包级哨兵变量并用 errors.Is 匹配。
- HTTP 层：统一通过中间件做安全加固与请求体限制，具体路由内部以 http.Error 返回标准状态码。

## 2. 核心文件与位置

- Agent 侧
  - cmd/agent/reporter.go：上报循环、重试、gzip 降级、断路器；定义 errForbidden、errBadPayload 两个哨兵错误；reportOnceSafe、pluginLoop 内用 recover 兜底。
  - cmd/agent/forward.go：端口转发会话 goroutine 内 recover，防止单个会话异常拖垮整个 Agent。
  - cmd/agent/terminal.go：终端相关 goroutine 多处 recover，确保 PTY/WS 异常不影响主循环。
  - cmd/agent/main.go、cmd/agent/relay.go：启动参数缺失或上游地址非法时 log.Fatal 退出。
- Server 侧
  - cmd/server/main.go：全局中间件链 securityHeadersMiddleware → corsMiddleware → gzipMiddleware → bodyLimitMiddleware → authMiddleware；优雅关闭；TLS 监听失败走 log.Fatal。
  - cmd/server/auth_core.go：密码学随机数失败时 panic（拒绝生成可预测 token），登录/会话校验集中实现。
  - cmd/server/ws.go：WebSocket 升级失败返回 errors.New(...) 给上层。
  - cmd/server/pgstore.go：显式比较 sql.ErrNoRows 等标准库哨兵错误。
  - cmd/server/forward.go：按场景返回 http.StatusForbidden/BadRequest/GatewayTimeout/ServiceUnavailable 等。

## 3. 架构与约定

- 哨兵错误（Sentinel Errors）
  - 面向跨层语义：如 errForbidden（403）、errBadPayload（400+gzip 损坏）、errParse（解析失败）。
  - 使用 errors.Is(err, sentinel) 进行分支决策，而非字符串比较。
  - 标准库错误（sql.ErrNoRows、http.ErrServerClosed）也直接 == 比较。
- Panic/Recover 边界
  - 仅用于不应发生但必须自愈的场景：长生命周期循环（上报、插件执行、转发/终端会话）。
  - 恢复后记录 slog.Error("...已恢复") 并继续运行，必要时重启子协程（如 pluginLoop）。
  - 安全敏感路径（CSPRNG 失败）选择 panic 而不是回退到不安全实现。
- HTTP 错误响应
  - 业务错误优先返回具体 HTTP 状态码（400/401/403/404/500/502/503/504），消息通过 i18n 翻译键输出。
  - 非业务错误（如构建请求失败、连接上游失败）用 http.Error 快速返回。
- 中间件防护
  - bodyLimitMiddleware：MaxBytesReader 限制请求体大小，防内存耗尽。
  - securityHeadersMiddleware：强制 X-Content-Type-Options: nosniff、X-Frame-Options: DENY、严格 CSP，阻断常见 Web 攻击面。
  - corsMiddleware：按需放行 Origin，否则不回写 CORS 头。
  - gzipMiddleware：对静态/JSON 响应压缩，跳过 WS/代理/流式路径。
- 启动期错误
  - 缺少必要环境变量（PostgreSQL DSN、VM URL）、证书未配置、数据库连接失败等，一律 log.Fatal 终止，避免半可用服务上线。

## 4. 开发者应遵循的规则

- 不要吞掉 error：调用处至少用 slog.Error/Warn 记录上下文（server、attempt、err 等）。
- 需要跨层判断的错误，优先定义为包级 var errXxx = errors.New(...) 并通过 errors.Is 检查。
- 仅在守护进程级长循环中用 defer recover() 包裹，普通函数/HTTP handler 不要滥用 recover。
- HTTP handler 内部错误统一通过 http.Error 返回对应状态码，不要自行拼接 HTML。
- 涉及密码学随机数的路径，失败时应 panic，禁止回退到弱随机源。
- 启动阶段无法恢复的错误使用 log.Fatal，让容器编排层感知并重启。
- 对外暴露的错误信息需经 i18n 翻译键包装，避免泄露内部细节。