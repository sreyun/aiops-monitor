---
kind: logging_system
name: 基于 Go slog 的轻量结构化日志系统
category: logging_system
scope:
    - '**'
source_files:
    - cmd/server/main.go
    - cmd/agent/main.go
---

## 系统概述
本项目采用 Go 标准库 `log/slog` 作为统一的日志框架，Server 与 Agent 两端均使用同一套 API，输出为人类可读的 Text 格式，默认写入 stderr。未引入第三方日志库（如 logrus、zap），保持最小依赖。

## 初始化与配置
- **Server**：`cmd/server/main.go:246` 在启动时调用 `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))`
- **Agent**：`cmd/agent/main.go:81` 同样位置做相同初始化
- 默认级别为 `INFO`，无运行时动态调整开关；如需调试需修改源码后重新编译
- 所有业务模块直接调用全局 `slog.Info/Warn/Error`，无需注入 logger 实例

## 日志结构与字段约定
所有日志均为结构化键值对，常见上下文字段包括：
- `target` — 目标主机/设备名（Redfish/OceanStor 采集器）
- `err` — 错误信息
- `path` / `addr` / `url` — 路径或地址
- `attempt` / `max` — 重试计数
- `distro` / `os` — 操作系统信息
- `module` / `status` — 安全模块状态
- `relational` / `timeseries` — 存储后端标识
- `note` — 补充说明

## 日志级别策略
- `slog.Info`：正常业务流程（服务启动、连接成功、采集完成等）
- `slog.Warn`：可恢复异常或需要关注的事件（TLS 未启用、采集失败、安全模块拦截等）
- `slog.Error`：严重错误（认证失败、连续失败退避、权限不足等）
- `slog.Debug`：未见使用，当前代码库中无 Debug 级别日志

## 特殊处理
- 启动阶段部分致命错误仍使用旧版 `log.Fatal/log.Fatalf`（如 PostgreSQL 未连接、环境变量缺失），这些会直接终止进程并输出到 stderr
- 国际化文本通过 `Tz()` 函数包裹，日志消息支持多语言（zh-CN/en/zh-TW）
- Agent 端还具备日志文件采集能力（`LogPaths` + `LogEncrypt`），将系统日志加密上报至 Server，但这属于被采集的日志而非应用自身运行日志

## 开发者规范
1. 统一使用 `slog.Info/Warn/Error`，避免混用 `fmt.Println` 或 `log.Print`
2. 关键上下文以键值对形式传入（如 `"target", t.Name`），便于后续结构化解析
3. 敏感信息（密码、token）不得出现在日志中
4. 长耗时操作建议记录起止日志，包含耗时上下文字段
5. 错误日志必须携带 `err` 字段，方便聚合分析