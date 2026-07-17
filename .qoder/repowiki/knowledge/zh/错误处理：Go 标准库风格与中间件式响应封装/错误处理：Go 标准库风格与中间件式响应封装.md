---
kind: error_handling
name: 错误处理：Go 标准库风格与中间件式响应封装
category: error_handling
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/server/handlers.go
    - cmd/server/helpers.go
    - cmd/agent/collector_linux.go
    - android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt
---

## 1. 采用的系统/方法
- Go 标准库 errors、fmt.Errorf（含 %w 包装）作为主要错误表达手段，未引入第三方错误库。
- HTTP 层通过中间件链集中处理横切关注点（CORS、安全头、gzip、Body 大小限制、鉴权），业务 handler 直接返回 JSON 结构体并由统一路由写入响应，不存在全局 panic/recover 兜底。
- 启动期关键依赖（PostgreSQL、VictoriaMetrics）失败时以 log.Fatal 终止进程，避免静默降级；运行时 I/O 错误则记录 slog 并向上返回。

## 2. 核心文件与位置
- cmd/server/main.go：HTTP 中间件链（securityHeadersMiddleware、corsMiddleware、gzipMiddleware、bodyLimitMiddleware、authMiddleware）、优雅关闭、TLS 启动、PG 连接重试（mustOpenPG）。
- cmd/server/handlers.go：通用 JSON 响应写入函数，被各 API handler 复用，实现统一的 code/message 响应格式。
- cmd/server/helpers.go：辅助函数中常见 if err != nil { return fmt.Errorf("...: %w", err) } 的错误包装模式。
- cmd/agent/collector_linux.go 等采集器：使用包级哨兵错误（如 var errParse = errors.New("parse error")）配合 isPermissionError 等判断进行分支处理。
- Android 端 android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt：Retrofit 回调中对网络异常做统一捕获并转换为 UI 可展示的消息。

## 3. 架构与约定
- 错误类型：无自定义 Error struct；使用 errors.New 定义包内哨兵错误，用 fmt.Errorf("...: %w", err) 包装上下文。
- 传播方式：逐层返回 error，由调用方决定是记录日志、包装后继续上抛，还是转为 HTTP 状态码。
- HTTP 响应：Handler 不直接写 http.Error，而是构造结构化 JSON（通常含 code、message 字段），由中间件或统一写入函数输出，保证前端/Android 客户端一致解析。
- 中间件职责：仅处理横切问题（CORS、安全头、压缩、Body 限流、鉴权），不吞掉业务错误；业务错误仍走 handler 返回路径。
- 启动期错误：配置缺失、PG/VM 不可达 → log.Fatal 立即退出，不尝试恢复。
- panic/recover：未发现显式 recover()；生产环境不依赖 panic 恢复，崩溃即视为严重故障。
- Agent 侧：采集器遇到非致命错误（权限不足、设备不存在）→ 记录警告并跳过该指标，不影响其他采集任务。

## 4. 开发者应遵循的规则
1. 不要 panic：业务逻辑中的异常一律返回 error；仅在不可恢复的启动参数/配置错误时使用 log.Fatal。
2. 使用 %w 包装：在跨包边界处用 fmt.Errorf("...: %w", err) 保留原始错误链，便于上层判断与日志关联。
3. 区分哨兵与上下文错误：可被调用方精确匹配的错误用 errors.New 暴露为包级变量；需要附加上下文的错误用 fmt.Errorf 包装。
4. Handler 只负责业务：不要在 handler 里写 CORS、压缩、鉴权逻辑，把它们抽到中间件；handler 只构造数据并返回错误。
5. HTTP 错误必须结构化：返回给前端的错误体至少包含 code（数字）和 message（人类可读字符串），方便 Android/Web 统一渲染。
6. I/O 错误要可观测：所有外部调用失败都要通过 slog.Warn/Error 记录，附带关键上下文（host、metric name、query 等）。
7. Agent 采集器容错：单个采集源失败不应中断整个采集循环；记录告警并 continue，确保平台整体可用性。