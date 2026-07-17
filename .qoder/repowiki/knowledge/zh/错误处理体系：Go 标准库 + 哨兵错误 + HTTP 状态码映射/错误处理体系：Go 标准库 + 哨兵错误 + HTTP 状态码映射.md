---
kind: error_handling
name: 错误处理体系：Go 标准库 + 哨兵错误 + HTTP 状态码映射
category: error_handling
scope:
    - '**'
source_files:
    - cmd/agent/reporter.go
    - cmd/agent/collector_linux.go
    - cmd/agent/security_linux.go
    - cmd/agent/modules.go
    - cmd/server/handlers.go
    - cmd/server/auth_core.go
    - android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt
    - android/app/src/main/java/com/aiops/monitor/ui/viewmodel/ViewModels.kt
---

## 1. 采用的系统/方法
- Go 标准库 errors 与 fmt.Errorf：项目未引入第三方错误包，统一使用 Go 原生错误机制。
- 哨兵错误（sentinel errors）：在关键路径定义包级 var errXxx = errors.New(...)，供上层通过 errors.Is() 判断。目前已发现 errParse、errForbidden、errBadPayload 等。
- 错误包装（wrapping）：广泛使用 %w 将底层错误包装为业务错误，保留调用栈上下文。
- HTTP 层错误：Agent 侧根据服务端返回的 HTTP 状态码构造语义化错误；Server 侧通过 handlers 返回 JSON 响应体中的错误字段给前端。
- panic/recover：未发现全局 recover 中间件或 panic 恢复策略，错误以返回值形式向上传播。
- Android 端：Kotlin 代码中通过 ApiService / ApiClient 封装网络请求，错误以异常或 Result 类型返回给 UI 层展示。

## 2. 核心文件与位置
- cmd/agent/reporter.go：定义 errForbidden、errBadPayload 等哨兵错误，HTTP 状态码到错误映射
- cmd/agent/collector_linux.go：定义 errParse 解析错误
- cmd/agent/security_linux.go：安全模式切换失败时返回结构化 fmt.Errorf
- cmd/agent/modules.go：包管理器检测失败的错误信息
- cmd/server/handlers.go：HTTP handler 入口，负责将错误序列化为 JSON 响应
- cmd/server/auth_core.go：认证相关错误
- android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt：网络请求错误封装
- android/app/src/main/java/com/aiops/monitor/ui/viewmodel/ViewModels.kt：ViewModel 层错误收集与展示

## 3. 架构与约定
- 分层传播：底层 I/O 错误 -> fmt.Errorf("...: %w", err) 包装 -> 业务层哨兵错误 -> HTTP/JSON 响应。
- 哨兵错误集中定义：对可被上层分支处理的错误定义为包级变量，避免字符串匹配。
- HTTP 状态码语义化：Agent 侧将非 200 响应统一转为带描述的错误对象，便于日志记录与重试策略区分。
- 无全局中间件：每个 handler 自行处理错误并写入响应体，未使用统一的 middleware 拦截器。
- i18n 错误消息：Server 侧 i18n 目录仅包含界面文案，错误消息仍以英文/中文硬编码为主。

## 4. 开发者应遵循的规则
1. 优先返回 error：不要在业务函数中打印日志或 panic，统一返回 error 由调用方决定如何处理。
2. 使用 %w 包装：所有需要保留上下文的错误必须用 fmt.Errorf("...: %w", err) 包装，禁止丢失链式信息。
3. 可分支的错误定义为哨兵：如果上层需要根据错误类型做不同处理，请定义 var errXxx = errors.New(...) 并在包内暴露。
4. HTTP 层统一映射：新增 API 时，参照 reporter.go 的模式，将常见 HTTP 状态码映射到语义化错误。
5. Android 端错误收敛：在 ApiClient 层统一捕获网络异常并转换为业务错误，UI 层只消费最终结果。
6. 避免 panic：除非是不可恢复的严重 bug，否则一律使用 error 返回值。