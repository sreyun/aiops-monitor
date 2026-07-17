---
kind: error_handling
name: 错误处理体系：哨兵错误 + fmt.Errorf 包装 + recover 兜底
category: error_handling
scope:
    - '**'
source_files:
    - cmd/agent/reporter.go
    - cmd/agent/collector_redfish.go
    - cmd/agent/modules.go
    - cmd/agent/forward.go
    - cmd/agent/gpu.go
    - cmd/server/auth.go
    - cmd/server/auth_core.go
    - android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt
---

## 1. 采用的系统/方法
- **Go 原生错误处理**：使用 `errors.New` 定义包级哨兵错误（sentinel errors），通过 `errors.Is` 进行判断；业务错误统一用 `fmt.Errorf("...: %w", err)` 包装传播。
- **recover 兜底**：在 Agent 的长生命周期 goroutine（collector、reporter、forward）中，对关键循环体包裹 `defer func(){ if r := recover(); r != nil {...} }()`，防止单个采集器崩溃导致整个进程退出。
- **HTTP 层错误码映射**：Agent reporter 将服务端返回的 HTTP 状态码转换为结构化错误（400→errBadPayload、403→errForbidden），上层据此决定是否重试或跳过上报。
- **Android 端**：基于 Retrofit/Kotlin 协程，错误以异常形式抛出，由 ViewModel 捕获后转为 UI 提示，未见到统一的错误类型封装。

## 2. 核心文件与位置
- `cmd/agent/reporter.go` — 定义 `errForbidden`、`errBadPayload` 两个哨兵错误，并在上报循环中使用 `errors.Is` 分支处理；多处 `recover` 保护上报 goroutine。
- `cmd/agent/collector_redfish.go` — 使用 `fmt.Errorf` 包装底层错误并附带上下文信息，同时提供 `classifyError` 辅助函数将常见 Redfish 错误转为用户可读提示。
- `cmd/agent/modules.go` — 包管理器探测失败时返回带中文描述的错误字符串，便于日志定位缺失依赖。
- `cmd/agent/forward.go`、`cmd/agent/gpu.go` — 在转发/命令执行入口使用 `recover` 做最外层兜底。
- `cmd/server/*.go` — Server 侧未发现集中式错误类型定义，主要依赖标准库 error 和 HTTP 响应码；认证/权限相关错误集中在 `auth.go`、`auth_core.go` 等文件中。
- `android/app/src/main/java/com/aiops/monitor/data/ApiClient.kt` — Android 网络请求错误处理，未找到统一错误枚举。

## 3. 架构与约定
- **哨兵错误仅用于“可被调用方显式区分”的关键路径**（如注册被拒、载荷格式错误），普通业务错误一律用 `fmt.Errorf` 包装传递，不单独定义类型。
- **错误消息语言**：Agent 内部错误消息使用中文（如“未找到受支持的包管理器”、“package 模块不支持当前系统”），面向运维人员直接阅读；Server 侧 i18n 资源位于 `cmd/server/i18n/*.json`，但错误对象本身未走 i18n 框架。
- **recover 的使用范围严格限定在“守护型 goroutine”**（采集器、上报器、转发器），避免滥用 recover 掩盖真正的 bug。
- **HTTP 错误分层**：Agent 将远端状态码降级为本地哨兵错误，再向上层暴露语义化错误，调用方可据此决定重试策略（例如 403 时跳过本次上报而非无限重试）。

## 4. 开发者应遵循的规则
1. **新增错误优先用 `fmt.Errorf("%w", err)` 包装**，仅在需要被 `errors.Is` 精确匹配时才定义新的哨兵错误变量。
2. **不要在 handler 或短生命周期函数中使用 recover**；仅在长期运行的采集/上报 goroutine 入口处使用 recover 做隔离。
3. **错误消息保持简洁且包含上下文**（至少带上来源组件名、关键参数），以便日志检索。
4. **对外 API 错误码应与内部哨兵错误一一对应**，避免在调用链中间随意转换状态码。
5. **Android 端建议引入统一的 Result/Error 封装类**，目前各 Screen 自行处理异常，风格不够一致。