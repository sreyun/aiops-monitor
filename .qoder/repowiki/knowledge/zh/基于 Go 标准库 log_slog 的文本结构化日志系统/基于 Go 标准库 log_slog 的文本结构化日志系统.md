---
kind: logging_system
name: 基于 Go 标准库 log/slog 的文本结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/agent/main.go
    - cmd/server/main.go
---

## 1. 使用的系统与框架
- 日志框架：Go 标准库 `log/slog`（Go 1.21+），无引入第三方日志库。
- 输出格式：纯文本（TextHandler），通过 key=value 键值对实现结构化字段，便于 grep/awk 解析。
- 默认级别：Info；Debug 仅在个别重试路径使用，Warn/Error 用于异常与告警场景。
- 输出目标：stderr（进程管理器如 systemd/docker 可统一收集）。

## 2. 核心文件与初始化位置
- Agent 入口：`cmd/agent/main.go` — 在 main 最开头调用 `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))` 完成全局 logger 初始化。
- Server 入口：`cmd/server/main.go` — 同样在 main 中用相同方式设置全局 slog 实例。
- 业务模块：所有采集器、API handler、通知器等均直接 import `log/slog` 并通过包级 `slog.Info/Warn/Error/Debug` 调用，无需注入 logger 实例。

## 3. 架构与约定
- 单例模式：两个二进制均在启动时设置全局 `slog.Default()`，各模块通过包级函数访问，避免依赖注入开销。
- 结构化字段：所有关键日志都附带上下文键值对，例如 `"target", t.Name`、`"err", err`、`"attempt", i+1`、`"url", ...`，便于按主机/会话/操作维度检索。
- 级别策略：
  - Info：正常生命周期事件（启动、连接成功、配置加载等）。
  - Warn：可恢复异常或需关注状态（安全模块 enforcing、TLS 未启用、转发通道失败等）。
  - Error：不可恢复错误（数据采集权限不足、连续失败退避、PostgreSQL 连接失败等）。
  - Debug：仅用于高频重试/调试路径，默认不输出。
- 国际化结合：Server 侧部分启动日志通过 `Tz(...)` 包装，使提示文案支持多语言，但字段名保持英文不变。
- 无外部 sink：当前未集成 file sink、JSON 序列化、远程日志服务或 APM 上报，日志由宿主环境负责采集。

## 4. 开发者应遵循的规则
- 始终使用 `log/slog` 包级函数（`slog.Info/Warn/Error/Debug`），不要自行创建 logger 实例。
- 为每条日志提供至少一个上下文键值对（如 `"err", err`、`"host", hostID`），禁止只写纯字符串消息。
- 严格区分级别：可恢复问题用 Warn，致命错误用 Error，常规流程用 Info，调试信息用 Debug。
- 敏感信息（token、密码、密钥）不得写入日志字段；如需记录，请脱敏后再传入。
- 若需要 JSON 输出或接入集中式日志平台，应在各自入口替换 TextHandler 为 JSONHandler/FileHandler，而非在各处分散处理。