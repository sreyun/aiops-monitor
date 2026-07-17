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
- 日志框架：Go 标准库 `log/slog`（Go 1.21+），无第三方日志库依赖。
- 输出格式：纯文本（TextHandler），未启用 JSON 模式。
- 默认级别：Info，所有关键运行信息、错误、告警均通过 Info/Warn/Error 输出；Debug 仅在少量重试/合并场景使用。
- 输出目标：stderr（进程管理器可统一收集）。

## 2. 核心文件与初始化位置
- Agent 端初始化：`cmd/agent/main.go:79`
  - `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))`
- Server 端初始化：`cmd/server/main.go:246`
  - 同样的 TextHandler + LevelInfo 配置。
- 全局调用点：两个二进制的所有业务模块直接通过 `slog.Info / Warn / Error / Debug` 调用，无需注入 logger 实例。

## 3. 架构与约定
- 单例全局 Logger：每个进程在 main 入口设置一次 `slog.Default()`，后续各包直接使用包级函数，避免传递 logger 参数。
- 结构化字段：所有日志调用均采用 key-value 对形式（如 `"err", err`、`"target", t.Name`、`"session", sid`），便于外部解析或 grep。
- 级别策略：
  - Info：启动、连接成功、周期性统计刷新等正常流程。
  - Warn：可恢复异常（网络读取错误、版本不支持、转发超时、安全模块 enforcing 提示）。
  - Error：不可恢复或需要立即关注的错误（权限不足、监听失败、连续失败退避）。
  - Debug：仅用于调试性重试间隔、AI 记忆合并细节等高频低价值信息。
- 国际化消息：Server 侧部分启动日志通过 `Tz(...)` 包装 i18n 键，但字段仍为英文 key，保持机器可读性。
- 无独立日志轮转/分级文件输出：当前实现全部写 stderr，由部署层（systemd/docker/nginx）负责落盘与轮转。

## 4. 开发者应遵循的规则
- 统一使用 `log/slog`，禁止再引入 zap/logrus 等第三方日志库。
- 不要绕过 `slog.SetDefault` 自定义 Handler；如需 JSON 输出或按级别分流，应在进程入口处集中修改。
- 日志消息使用中文描述，但结构化字段名必须使用英文 key（如 `err`、`addr`、`target`、`session`），确保可被工具解析。
- 合理使用级别：
  - 正常业务流程用 `Info`；
  - 可自动恢复的异常用 `Warn` 并附带上下文字段；
  - 导致功能失效的错误用 `Error`；
  - 高频调试信息用 `Debug`，默认关闭不影响生产。
- 避免在热路径中打印过长消息或大量字段，防止影响性能。
- 敏感信息（密码、token、密钥）不得写入日志字段。