---
kind: error_handling
name: 错误处理：哨兵错误 + recover 兜底 + slog 日志的混合模式
category: error_handling
scope:
    - '**'
source_files:
    - cmd/agent/reporter.go
    - cmd/agent/collector_linux.go
    - cmd/agent/terminal.go
    - cmd/agent/forward.go
    - cmd/server/ws.go
    - cmd/server/pgstore.go
    - cmd/server/sre_api.go
    - cmd/server/filext.go
    - cmd/server/auth_core.go
---

## 1. 采用的系统/方法
- Go 标准库 errors 哨兵错误：在 Agent 侧定义包级 var errForbidden、var errBadPayload，并通过 errors.Is 进行分支判断。
- fmt.Errorf 包裹错误：大量使用 fmt.Errorf("HTTP %d: %s", ...) 等直接返回带上下文信息的 error，未统一包装为自定义类型。
- panic/recover 作为最后兜底：在 Agent 的 reporter、terminal、forward、filext 以及 Server 的 SRE API 等处用 defer func(){ _ = recover() }() 捕获 panic，防止协程崩溃导致进程退出。
- slog 结构化日志记录错误：Agent 侧普遍通过 slog.Error("...","err",err) 输出错误上下文，而非仅依赖返回值。
- HTTP 层无全局中间件：Server 未引入 gin/echo 等框架中间件做统一错误码封装，错误多以 http.ErrServerClosed、sql.ErrNoRows 等标准错误直接比较或透传。

## 2. 关键文件与位置
- cmd/agent/reporter.go：定义 errForbidden、errBadPayload 两个哨兵错误，并在上报逻辑中用 errors.Is 区分 403/400 场景；多处 recover() 保护长循环 goroutine。
- cmd/agent/collector_linux.go：定义 errParse 解析错误，配合权限检查逻辑。
- cmd/agent/terminal.go / cmd/agent/forward.go：对 PTY/转发 goroutine 加 recover()，避免单个会话异常拖垮整个 agent。
- cmd/server/ws.go：直接 errors.New 构造 WebSocket 握手失败错误（非升级、缺少 Key、不支持 hijack）。
- cmd/server/pgstore.go：通过 sql.ErrNoRows、sql.ErrConnDone 等标准 SQL 错误控制流程。
- cmd/server/sre_api.go：在回调中使用 defer func(){ _ = recover() }() 隔离外部插件调用异常。
- cmd/server/filext.go：对文件扩展名解析路径加 recover() 防御。
- cmd/server/auth_core.go：在 CSPRNG 不可用时主动 panic，将操作系统安全随机源不可用视为致命错误。

## 3. 架构与约定
- Agent 侧以可恢复错误为主。网络/鉴权类错误用哨兵错误 + errors.Is 做业务分支；IO/采集异常通过 slog.Error 记录并继续运行；对长时间运行的 goroutine 一律加 recover() 兜底。
- Server 侧偏向简单直接。HTTP/WebSocket 层错误多为裸 errors.New 或标准库错误；数据库层依赖 sql.Err* 常量；对外部不可信代码（SRE 插件回调）用 recover() 隔离。
- 未形成统一错误类型体系：没有集中定义的 AIOpsError 结构体，也没有统一的 HTTP 响应错误格式，各模块自行决定返回形式。
- 无全局错误中间件：Server 路由注册分散在多个 _api.go 文件中，未见统一的 middleware 拦截错误并转换为一致 JSON 响应。

## 4. 开发者应遵循的规则
1. 可分支的错误优先用哨兵：若上层需要按错误类型做不同处理（如 403 vs 400），应在包内定义 var errXxx = errors.New(...) 并使用 errors.Is 判断。
2. 不可分支的错误用 fmt.Errorf 包裹：带上必要上下文（URL、host、metric name 等），不要只返回裸错误。
3. 所有长期运行的 goroutine 必须加 recover()：包括采集器、转发器、终端会话、插件回调等，确保单点 panic 不导致进程崩溃。
4. 错误信息通过 slog.Error 输出：至少包含关键字段（addr、target、module 等），便于线上排查。
5. HTTP/WebSocket 层保持简洁：直接使用标准库错误即可，无需额外包装；如需对外暴露统一错误码，应在路由层集中处理。
6. 数据库错误使用标准常量：sql.ErrNoRows、sql.ErrConnDone 等用于流程控制，不要自行字符串匹配。
7. 致命错误可 panic：如 CSPRNG 不可用这类无法恢复的环境问题，允许 panic 让进程快速失败，由 systemd/docker 重启。