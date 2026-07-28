# AIOps · 原生 Android 客户端

纯原生 **Kotlin + Jetpack Compose** 实现的 Android 控制台，直接调用 Go 后端的 REST / WebSocket API。
后端**零改动**——本 APP 只是把现有 `cmd/server` 的接口用原生 UI 重新包了一层。

> v0.7.0 基线已通过 `assembleDebug`、`lintDebug` 和 `testDebugUnitTest` 实际验证。v0.8.0 新增全局深浅色主题与核心指标快捷导航，请在发布前重新执行下文完整构建命令。

## 功能范围

| 页面 | 能力 |
|---|---|
| 设置与账号 | 管理多个后端地址；总览账号菜单支持深色/浅色主题即时切换、服务器设置、切换账号、退出登录，服务端会话与本机 Cookie 同步注销，重新登录继续执行 MFA |
| 登录 | 居中自适应登录卡片、全宽用户名/密码/MFA 输入框，复用 `aiops_session` Cookie 鉴权 |
| 总览 | 居中式 2×2 关键指标（点击快速跳转主机/告警并自动筛选）、主机搜索/分类、10 秒刷新、CPU/内存/磁盘指标及 GPU 主机/设备汇总、计算/显存/温度指标 |
| 主机详情 | 主机信息 + 10 秒实时刷新 + 磁盘卷/GPU 设备 + 原生交互时间序列（点选、平移、缩放、精确时间点）+ 终端入口 |
| 告警 | 默认收集并展示接口返回的全部告警；支持级别/处理状态筛选、确认、静默、即时状态反馈、重复操作保护及 AI 诊断 |
| 终端 | 企业级原生 WebSocket 终端：VT100/xterm、流式 UTF-8、心跳与指数重连、输入法组合态、软键盘避让、无空白行 PTY resize、Ctrl/Alt/导航键与特殊字符；键盘收起后可点击终端区、状态区或键盘按钮再次唤起 26 键布局 |
| AI 助理 | SSE 流式对话、动态快捷问题、主机诊断、工具进度与停止生成；统一检索 Web 端知识记忆及 Loki/Prometheus 数据源，兼容旧版 `/ai/chat` |
| 监控 | 对齐 Web 端 `/checks` 与 `/apimon/systems`，核心指标可点击并自动切换拨测/API 分页及状态/证书筛选；展示 HTTPS 证书剩余有效期、立即探测及可精确定位采样点的 24h 原生趋势 |
| 运维中心 | 事件闭环、事件专用 AI 诊断进度/结果/流式追问/反馈、日志诊断、自动化剧本、SLO、修复审批和工单；剧本执行采用安全的一次性跳转事件，详情页展示主机/步骤状态、耗时与完整输出 |

全局使用统一的 Material 3 设计令牌、语义色、圆角与卡片层级，提供深色运维控制台与浅色互联网产品两套外观；主题选择持久化保存，系统状态栏/手势区图标随主题联动。软键盘弹出时主导航自动隐藏，避免输入空间被二次挤占。

## 环境要求

- **Android Studio**（你已安装即可）
- Android SDK **34**，JDK **17**（Android Studio 自带）
- 一台 Android **8.0+（API 26）** 真机，或模拟器
- 手机能**网络访问**你的 Go 后端（同一 Wi-Fi / VPN）

## 打开与构建

1. Android Studio → `File → Open` → 选择本目录 `android/`。
2. 等待 Gradle Sync 完成（会下载 Compose BOM、Retrofit、OkHttp、DataStore 等依赖）。
3. 连接手机或启动模拟器 → `Run ▶`（或 `Build → Build Bundle(s) / APK(s) → Build APK`）。

也可以在命令行执行：

```bash
./gradlew assembleDebug lintDebug testDebugUnitTest
```

## 使用

1. 打开 APP → 「服务器设置」填入后端地址（推荐 `https://...`；内网 HTTP 可填 `http://192.168.1.10:8080`）→ 保存。
2. 切到「总览」→ 用后端账号登录。
3. 浏览主机、点开详情看指标、在「告警」里确认/静默、在主机详情里「打开终端」。

> 后端为 **http 明文** 时，APP 已开启 `usesCleartextTraffic`，Android 9+ 不会拦截。

## 后端 API 契约（本 APP 已对齐）

| 接口 | 用途 |
|---|---|
| `POST /api/v1/login` | 登录，返回 `aiops_session` Cookie（HttpOnly） |
| `GET /api/v1/me` | 当前会话，401 未登录 |
| `GET /api/v1/hosts` | 主机列表（含 `latest` 指标快照、`online` 状态） |
| `GET /api/v1/hosts/{id}/metrics` | 该主机指标时间序列 |
| `GET /api/v1/hosts/{id}/history` | 指定时间范围的主机历史趋势 |
| `GET /api/v1/summary` | 概览统计 |
| `GET /api/v1/alerts` | 当前告警 |
| `POST /api/v1/alerts/ack` · `/silence` | 确认 / 静默 |
| `GET /api/v1/hosts/{id}/terminal` | 终端 WebSocket（首字节 `'i'`=输入、`'r'`=resize `colsxrows`） |
| `POST /api/user/terminal-password/verify` | 终端二次密码校验 |
| `GET /api/user/terminal-password/status` | 查询终端二次密码状态 |
| `POST /api/user/terminal-password/set` | 设置或修改终端二次密码 |
| `GET /api/v1/checks` · `POST /checks/{id}/run` | 拨测列表与立即执行 |
| `GET /api/v1/checks/{id}/history` | 拨测最近 24h 历史 |
| `GET /api/v1/apimon/systems` · `POST /apimon/systems/{id}/run` | API 业务系统聚合与立即执行 |
| `GET /api/v1/apimon/endpoints/{id}/history` | API 接口最近 24h 性能历史 |
| `POST /api/v1/ai/chat` | 工具化 AI SSE 流式对话（智能体对话能力；不可用时由客户端回退到本接口） |
| `GET /api/v1/ai/suggestions` | 与 Web 端一致的动态快捷问题（由 AI 对话能力提供） |
| `GET /api/v1/datasources` | Loki / Prometheus 数据源能力发现 |
| `GET /api/v1/sre/overview` | SRE 概览指标 |
| `GET/POST /api/v1/incidents` · `POST /incidents/{id}/ack|resolve|ticket` | 事件全生命周期管理 |
| `POST /api/v1/incidents/{id}/diagnose` · `GET/POST /diagnose-chat` · `POST /diagnosis-feedback` | 事件诊断、持久会话、流式追问及效果反馈 |
| `GET /api/v1/logs` · `POST /logs/diagnose` | 日志分页检索与诊断 |
| `GET /api/v1/playbooks` · `POST /playbooks/{id}/execute` | 自动化剧本及发起执行 |
| `GET /api/v1/playbooks/executions` · `/executions/{id}` | 执行历史、主机结果、步骤输出及耗时 |
| `GET /api/v1/slos` | SLO 与错误预算 |
| `GET/POST /api/v1/tickets/{id}` | 工单查询与状态更新 |
| `GET /api/v1/remediation/runs` · `POST /remediation/runs/{id}/approve|reject` | 自动修复审批 |

## 已知限制 / 后续可增强

- 会话 Cookie 保存在应用私有 DataStore 中，并禁用系统备份与设备迁移导出；切换服务器或收到会话失效响应时会自动清除。
- 终端会随 26 键软键盘实时缩放可视区和远端 PTY，并保持输入命令与最新输出可见；VT 状态机支持光标移动、擦除、滚屏、DEC 线框字符、备用屏幕及 `top`/`vim` 等全屏程序。网络断开后会自动建立新 Shell，并明确提示会话边界（后端协议不支持恢复已断开的原 Shell）。
- 登录支持 6 位 MFA 动态口令；首次绑定 MFA、强制修改初始密码仍需在网页端完成。
- 性能趋势使用原生 Compose Canvas 绘制，按真实时间戳定位，支持点选十字线、准确值、平移、双指缩放与重置，不依赖 WebView/H5 或大型图表库。
- **未做系统推送**：总览、告警、监控和运维页面使用前台自动刷新，尚未接入 Firebase/FCM 后台推送。

## 目录结构

```
android/
├── build.gradle                 # 工程级（AGP / Kotlin 插件版本）
├── settings.gradle
├── gradle.properties
├── gradle/wrapper/gradle-wrapper.properties
└── app/
    ├── build.gradle             # 模块级依赖（Compose / Retrofit / OkHttp / DataStore）
    ├── proguard-rules.pro
    └── src/main/
        ├── AndroidManifest.xml
        ├── res/...               # 主题 / 图标 / 字符串
        └── java/com/aiops/monitor/
            ├── MainActivity.kt
            ├── ui/
            │   ├── AIOpsApp.kt          # 导航 + 底部栏
            │   ├── theme/Theme.kt
            │   ├── Util.kt
            │   ├── screens/             # 登录/总览/主机/告警/监控/运维/AI/终端
            │   └── viewmodel/ViewModels.kt
            └── data/
                ├── ApiClient.kt         # OkHttp + Cookie 鉴权 + Retrofit
                ├── ApiService.kt        # 接口定义
                ├── TerminalClient.kt    # 终端 WebSocket 心跳、重连与会话状态
                ├── terminal/            # VT100/xterm 屏幕、UTF-8 流解码、输入编码
                ├── models/Models.kt
                └── store/SettingsStore.kt
```
