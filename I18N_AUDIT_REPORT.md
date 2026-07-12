# 国际化（i18n）深度自查报告

> 目标：完成语言切换功能的前置准备——盘点 Go 后端与前端（管理面板 `cmd/server/web/`）中所有用户可见文本，找出未走字典的硬编码项，给出缺失清单、待补充字典结构与代码改造定位。
> 范围：`cmd/server/`（Go 后端 + 内嵌面板）、`cmd/agent/`（Agent 端）。营销站 `website/` 已独立完整三语化，不在本次范围。
> 结论先行：**后端服务端 i18n 已基本完备；前端面板是唯一重大缺口（仅中文、切换被禁用）；Agent 端完全无 i18n。**

---

## 一、现状盘点

### 1.1 后端 i18n 机制（✅ 完善，可直接复用）
- 机制：`cmd/server/i18n.go` 提供 `T(lang,key,args)` / `Tr(r,key,args)` / `Tz(key,args)`，通过 `//go:embed i18n/*.json` 内嵌三语字典。
- 语言探测优先级：`?lang=` > Cookie `aiops_lang` > `Accept-Language` > 默认 `zh-CN`；支持 `zh-CN / zh-TW / en`。
- 字典文件：`cmd/server/i18n/{zh-CN,zh-TW,en}.json`，**三者 key 完全齐平、互为翻译**（已核对 en.json 与 zh-CN.json 一一对应）。
- 覆盖面（已字典化）：`auth / alert / notify / check / user / config / recovery / terminal / playbook / forward / agent / email / db / server / log / common / terminal_auth` 等模块，含 API 错误、表单校验、通知/邮件模板正文、操作日志。
- `Tr`/`T`/`Tz` 已在 **24 个 .go 文件**中被调用。

### 1.2 前端面板 i18n 机制（⚠️ 仅中文、切换被禁用——核心缺口）
- 字典：`cmd/server/web/i18n-dashboard.js`，内部一个 `DICT{}`（~600 条，**仅中文**）。
- 机制：`I18N.t(key)` + `applyTranslations()` 扫描 `data-i18n` / `data-i18n-placeholder` / `data-i18n-title` 三类属性替换文本。
- **关键缺陷（文件头注释明示）**：语言切换已被**主动禁用**——`setLang(){}` 是空实现、`getLang()` 恒返 `"zh-CN"`、`supported: ["zh-CN"]`。无英文词典、无切换 UI、无持久化。
- 因此：面板当前**只能显示中文**；即便补充英文词典，也需先“重新启用切换 + 加载 en 词典 + 增加切换控件”。

### 1.3 key 命名规范
- 后端与前端**均采用** `模块.属性` 点分风格（如 `auth.invalid_credentials`、`ui.install_agent_btn`），规范一致。
- 存在**少量跨端重名**（如 `notify.type_cpu`、`notify.alert_engine`、`empty.no_online_hosts`、`playbook.*`、`forward.*` 在前后端词典各定义一份）。建议明确**职责边界**：后端词典管 API/通知/邮件/日志/服务端；前端词典管 UI 文案，避免重复定义、以一方为准。

---

## 二、后端缺失清单（按模块）

> 说明：服务端其余模块已字典化，以下为**真正未走 i18n 的用户可见硬编码**。日志类（运维侧）单独标注，通常不需随 UI 切换。

### 2.1 `cmd/agent/terminal.go`（🔴 高：经 WebSocket 渲染到浏览器终端 UI）
| 行号 | 原文 | 建议 key |
|---|---|---|
| 338 | `文件超过100MB限制` | `agent.file.upload_too_large` |
| 355 | `无法创建文件: %v` | `agent.file.create_failed` |
| 376 | `上传数据超过声明大小` | `agent.file.upload_oversize` |
| 463 | `文件不存在或无法访问: %v` | `agent.file.not_found` |
| 469 | `不支持下载目录` | `agent.file.dir_unsupported` |
| 475 | `文件超过100MB限制` | `agent.file.download_too_large` |
| 491 | `无法打开文件: %v` | `agent.file.open_failed` |

### 2.2 `cmd/agent/relay.go`（🟡 中：Relay 安装脚本代理的 HTTP 错误，浏览器可见）
| 行号 | 原文 | 建议 key |
|---|---|---|
| 149 | `Relay: 构建请求失败` | `agent.relay.build_req_failed` |
| 155 | `Relay: 无法连接上游服务端 (...)` | `agent.relay.upstream_unreachable` |
| 164 | `Relay: 读取安装脚本失败` | `agent.relay.read_script_failed` |
| 174 | `Relay: 无效的 Host 头` | `agent.relay.invalid_host` |

### 2.3 `cmd/agent/reporter.go`（🟢 低：上报错误，主要进日志）
| 行号 | 原文 | 建议 key |
|---|---|---|
| 182 | `服务端返回状态码 400（请求格式错误）` | `agent.report.bad_request` |
| 226 | `注册失败，跳过本次上报` | `agent.report.register_failed` |

### 2.4 `cmd/agent/main.go`（🟢 低：CLI 帮助/致命错误，终端内可见）
| 行号 | 原文 | 建议 key |
|---|---|---|
| 88 | `服务端地址，如 http://192.168.1.10:8529` | `agent.flag.server` |
| 89 | `基础指标上报间隔(秒)` | `agent.flag.interval` |
| 90 | `插件执行周期(秒)` | `agent.flag.plugin_interval` |
| 91 | `监控的磁盘路径` | `agent.flag.disk_path` |
| 92 | `Python 插件目录` | `agent.flag.plugins_dir` |
| 93 | `运行 .py 插件的解释器` | `agent.flag.python` |
| 94 | `主机分类标签，如 生产/测试/DB/办公终端` | `agent.flag.category` |
| 95 | `安装 Token（由服务端安装命令注入，可选）` | `agent.flag.token` |
| 96 | `网关中继模式：监听本地端口，转发所有请求到 --server 指定的云监控中心` | `agent.flag.relay` |
| 97 | `Relay 监听地址，如 :8529` | `agent.flag.listen` |
| 98 | `Relay 共享密钥，用于上游服务端验证中继请求` | `agent.flag.relay_secret` |
| 99 | `配置文件路径` | `agent.flag.config` |
| 126 | `未配置任何服务端地址（--server 或 servers 字段）` | `agent.fatal.no_server` |

### 2.5 `cmd/server/config.go`（🟡 中：服务端唯一遗漏，渲染到转发列表 UI）
| 行号 | 原文 | 建议 key |
|---|---|---|
| 912 | `newProxy.Name = original.Name + " (副本)"` | `forward.copy_suffix`（值 ` (副本)`） |

> 注：`(副本)` 后缀在**存储时**被写死进名称。理想做法是在**渲染时**本地化（存储中性标记，列表展示时拼接 `T(lang,"forward.copy_suffix")`），否则切换语言后历史副本名不会变。见第五节定位。

### 后端小结
- 服务端本体（24 文件）已基本完成；仅 `config.go:912` 一处遗漏。
- **真正的后端缺口集中在 `cmd/agent/`**（零 i18n，~27 处用户可见中文）。Agent 是独立二进制、不引入服务端 i18n 包，需单独决策（见第六节）。

---

## 三、前端缺失清单（管理面板）

### 3.1 `cmd/server/web/index.html` 硬编码（未接 `data-i18n`，切换英文后仍显示中文）
| 行号 | 原文 | 现状 | 建议处理 |
|---|---|---|---|
| 6 | `<title>AIOps Monitor · 主机监控运维平台</title>` | 写死，未在 JS 中本地化 | 新增 `app.title`，`init` 时 `document.title = I18N.t("app.title")` |
| 26–27 | `AIOps Monitor 需要启用 JavaScript` + 英文 | noscript 写死中文 | 新增 `app.js_required` |
| 90–91 | `概览` / `集群资源、告警与活动总览` | pageTitle 兜底写死 | 改用 `nav.overview` + `section.overview_desc`（已存在） |
| 95 | `title="暂停自动刷新"` | 写死 | 改 `data-i18n-title="ui.pause_refresh"`（已存在） |
| 113 | `未登录` | 写死 | 新增 `ui.not_logged_in` |
| 120 | `主题切换` | 写死 | 改 `data-i18n="ui.theme"`（已存在） |
| 217–218 | 空态“暂无主机上报…” | 写死段落 | 改接 `empty.no_hosts`（已存在，JS 覆盖） |
| 221 | `安装 Agent` | 写死 | 改 `data-i18n="ui.install_agent_btn"`（已存在） |
| 236 / 237 / 242 | `磁盘 IO` / `磁盘 IOPS` / `进程数` | 过滤芯片写死 | 改 `data-i18n="notify.type_diskio/type_iops/type_proc"`（**已存在未用**） |
| 269–272 | `10 条/页` 等 | option 写死 | 新增 `filter.page_size_10/30/50/100` |
| 293 / 296 / 339 | `卡片视图`/`列表视图`/`列表` | title 写死 | 改 `data-i18n-title="ui.card_view/ui.list_view"`（已存在） |
| 309 | `+ 添加检查` | 按钮写死 | 改 `data-i18n="check.add"`（已存在） |
| 314 | 空态“还没有自定义监控…” | 写死段落 | 改接 `empty.no_checks`（已存在） |
| 323 | `+ 新建剧本` | 按钮写死 | 改 `data-i18n="playbook.new"`（已存在） |
| 329–330 | `还没有自动化剧本` / 副文案 | 写死段落 | 改接 `empty.no_playbooks`（已存在） |

> 观察：大量缺口是**“词典已有对应 key 但未接线”**（如 `notify.type_diskio`、`check.add`、`playbook.new`、`ui.theme`），属低风险的接线补全；少数需新增 key（见 3.3）。

### 3.2 `cmd/server/web/app.js` 硬编码（未用 `I18N.t`，共 11 处）
| 行号 | 原文 | 建议处理 |
|---|---|---|
| 1699 | `toast("已复制"/"复制失败")` | 改 `I18N.t("toast.copied"/"toast.copy_failed")`（已存在） |
| 1713 | `toast("终端已连接")` | 新增 `term.connected` |
| 1858 | `errEl.textContent="请填写密码"` | 新增 `valid.fill_password` |
| 1902 | `errEl.textContent="请输入密码"` | 新增 `valid.enter_password` |
| 2184 / 2247 | `toast("终端未连接")` | 新增 `term.not_connected` |
| 2202 | `toast("文件超过 100MB 限制，请使用其他方式传输")` | 新增 `term.file_too_large` |
| 2217 | `toast("终端连接已断开，上传取消")` | 新增 `term.upload_cancelled` |
| 4001 | `errEl.textContent="请填写验证信息和新的终端密码"` | 新增 `term_auth.fill_verify_password` |
| 4020 | `errEl.textContent="请输入 MFA 动态口令"` | 新增 `term_auth.enter_mfa_code` |
| 5089 | 模板 `title="定时触发"` | 新增 `playbook.sched_badge_title` |

> 说明：app.js 已有 **488 处 `I18N.t()` 调用**， adoption 很高；上述是漏网之鱼。另有少量 `I18N.t("x") + "：" + err` 式拼接，中文冒号可并入模板或忽略（低优先级）。

### 3.3 前端需**新增**的 key（当前词典完全没有）
```
app.title, app.js_required, ui.not_logged_in,
term.connected, term.not_connected, term.file_too_large, term.upload_cancelled,
valid.fill_password, valid.enter_password,
term_auth.fill_verify_password, term_auth.enter_mfa_code,
playbook.sched_badge_title,
filter.page_size_10, filter.page_size_30, filter.page_size_50, filter.page_size_100
```
（其中 `app.title`、`ui.not_logged_in`、`term.*`、`valid.*`、`term_auth.*`、`playbook.sched_badge_title`、`filter.page_size_*` 为纯新增；其余多为“已有 key 未接线”）

---

## 四、需补充的字典结构（JSON，区分前后端）

### 4.1 后端字典补充（写入 `cmd/server/i18n/{zh-CN,zh-TW,en}.json` 三语）
> 仅列出**新增**条目；结构沿用现有 `模块.属性`。en 值见下，zh-TW 建议与 zh-CN 同义繁体化。

```json
{
  "agent.file.upload_too_large":   "文件超过100MB限制",
  "agent.file.create_failed":      "无法创建文件: %v",
  "agent.file.upload_oversize":    "上传数据超过声明大小",
  "agent.file.not_found":         "文件不存在或无法访问: %v",
  "agent.file.dir_unsupported":    "不支持下载目录",
  "agent.file.download_too_large": "文件超过100MB限制",
  "agent.file.open_failed":        "无法打开文件: %v",
  "agent.relay.build_req_failed":  "Relay: 构建请求失败",
  "agent.relay.upstream_unreachable": "Relay: 无法连接上游服务端 (%s)",
  "agent.relay.read_script_failed": "Relay: 读取安装脚本失败",
  "agent.relay.invalid_host":      "Relay: 无效的 Host 头",
  "agent.report.bad_request":      "服务端返回状态码 400（请求格式错误）",
  "agent.report.register_failed":  "注册失败，跳过本次上报",
  "agent.fatal.no_server":         "未配置任何服务端地址（--server 或 servers 字段）",
  "agent.flag.server":             "服务端地址，如 http://192.168.1.10:8529",
  "agent.flag.interval":           "基础指标上报间隔(秒)",
  "agent.flag.plugin_interval":    "插件执行周期(秒)",
  "agent.flag.disk_path":          "监控的磁盘路径",
  "agent.flag.plugins_dir":        "Python 插件目录",
  "agent.flag.python":             "运行 .py 插件的解释器",
  "agent.flag.category":           "主机分类标签，如 生产/测试/DB/办公终端",
  "agent.flag.token":              "安装 Token（由服务端安装命令注入，可选）",
  "agent.flag.relay":              "网关中继模式：监听本地端口，转发所有请求到 --server 指定的云监控中心",
  "agent.flag.listen":             "Relay 监听地址，如 :8529",
  "agent.flag.relay_secret":       "Relay 共享密钥，用于上游服务端验证中继请求",
  "agent.flag.config":             "配置文件路径",
  "forward.copy_suffix":           " (副本)"
}
```
对应英文（en.json）：
```json
{
  "agent.file.upload_too_large":   "File exceeds 100MB limit",
  "agent.file.create_failed":      "Failed to create file: %v",
  "agent.file.upload_oversize":    "Uploaded data exceeds declared size",
  "agent.file.not_found":         "File not found or inaccessible: %v",
  "agent.file.dir_unsupported":    "Directory download is not supported",
  "agent.file.download_too_large": "File exceeds 100MB limit",
  "agent.file.open_failed":        "Failed to open file: %v",
  "agent.relay.build_req_failed":  "Relay: failed to build request",
  "agent.relay.upstream_unreachable": "Relay: cannot connect to upstream server (%s)",
  "agent.relay.read_script_failed": "Relay: failed to read install script",
  "agent.relay.invalid_host":      "Relay: invalid Host header",
  "agent.report.bad_request":      "Server returned status 400 (bad request format)",
  "agent.report.register_failed":  "Registration failed, skipping this report",
  "agent.fatal.no_server":         "No server address configured (--server or servers field)",
  "agent.flag.server":             "Server address, e.g. http://192.168.1.10:8529",
  "agent.flag.interval":           "Base metrics report interval (seconds)",
  "agent.flag.plugin_interval":    "Plugin execution interval (seconds)",
  "agent.flag.disk_path":          "Disk path to monitor",
  "agent.flag.plugins_dir":        "Python plugin directory",
  "agent.flag.python":             "Interpreter for .py plugins",
  "agent.flag.category":           "Host category tag, e.g. Prod/Test/DB/Office",
  "agent.flag.token":              "Install token (injected by server install command, optional)",
  "agent.flag.relay":              "Gateway relay mode: listen on local port, proxy all requests to --server",
  "agent.flag.listen":             "Relay listen address, e.g. :8529",
  "agent.flag.relay_secret":       "Relay shared secret for upstream server verification",
  "agent.flag.config":             "Config file path",
  "forward.copy_suffix":           " (copy)"
}
```

### 4.2 前端字典补充（写入 `cmd/server/web/i18n-dashboard.js` 的 `DICT{}`，并同步英文词典）
> zh-CN 值（新增 key）：
```json
{
  "app.title": "AIOps Monitor · 主机监控运维平台",
  "app.js_required": "AIOps Monitor 需要启用 JavaScript",
  "ui.not_logged_in": "未登录",
  "term.connected": "终端已连接",
  "term.not_connected": "终端未连接",
  "term.file_too_large": "文件超过 100MB 限制，请使用其他方式传输",
  "term.upload_cancelled": "终端连接已断开，上传取消",
  "valid.fill_password": "请填写密码",
  "valid.enter_password": "请输入密码",
  "term_auth.fill_verify_password": "请填写验证信息和新的终端密码",
  "term_auth.enter_mfa_code": "请输入 MFA 动态口令",
  "playbook.sched_badge_title": "定时触发",
  "filter.page_size_10": "10 条/页",
  "filter.page_size_30": "30 条/页",
  "filter.page_size_50": "50 条/页",
  "filter.page_size_100": "100 条/页"
}
```
> 英文（落盘为独立文件，如 `i18n-dashboard.en.js`，结构与 `DICT` 完全一致，key 相同、值为英文）：
```json
{
  "app.title": "AIOps Monitor · Host Monitoring & Ops Platform",
  "app.js_required": "AIOps Monitor requires JavaScript",
  "ui.not_logged_in": "Not logged in",
  "term.connected": "Terminal connected",
  "term.not_connected": "Terminal not connected",
  "term.file_too_large": "File exceeds 100MB limit, use another method",
  "term.upload_cancelled": "Terminal disconnected, upload cancelled",
  "valid.fill_password": "Please enter password",
  "valid.enter_password": "Please enter password",
  "term_auth.fill_verify_password": "Please fill verification info and new terminal password",
  "term_auth.enter_mfa_code": "Please enter MFA code",
  "playbook.sched_badge_title": "Scheduled",
  "filter.page_size_10": "10 / page",
  "filter.page_size_30": "30 / page",
  "filter.page_size_50": "50 / page",
  "filter.page_size_100": "100 / page"
}
```

### 4.3 前端 English 词典的落地方式（解决“仅中文/切换禁用”）
当前 `i18n-dashboard.js` 是**单语中文**。要支持切换，建议：
1. 将现有 `DICT`（~600 条）作为 `zh-CN` 源；新增 `i18n-dashboard.en.js` 导出等结构英文 `DICT`（**所有现有 key 均需补英文值**——本报告列出的是“新增 key”，而“已有 key 的英文”需对整个现有 DICT 做翻译补全，属机械性工作量，可后续一次性生成）。
2. 改造 `setLang(lang)`：按语言加载对应 `DICT`，调用 `applyTranslations()` 重新渲染；`getLang()` 返回真实语言。
3. 在顶栏新增语言切换控件（与现有主题切换并列），选择后写入 Cookie `aiops_lang` 并 `location.reload()` 或原地重渲染。
4. 因后端 `i18n.go` 已识别 Cookie `aiops_lang`，面板只要 set 了该 Cookie，API 错误/通知等后端文本会自动随语言返回——**前后端语言由此统一**。

---

## 五、代码改造定位（具体位置，不含实现细节）

| 改造点 | 文件 / 位置 | 说明 |
|---|---|---|
| 补充后端字典 | `cmd/server/i18n/{zh-CN,zh-TW,en}.json` | 写入第四节 4.1 的 `agent.*` + `forward.copy_suffix` |
| Agent 字符串接入 | `cmd/agent/terminal.go:338,355,376,463,469,475,491`、`relay.go:149,155,164,174`、`reporter.go:182,226`、`main.go:88-99,126` | 引入 Agent 侧 `T/Tz`（或复用共享包），替换硬编码中文（见第六节决策） |
| 副本名本地化 | `cmd/server/config.go:912` | 改为存储中性标记，列表渲染时拼接 `T(lang,"forward.copy_suffix")`，避免写死语言 |
| 前端新增 key | `cmd/server/web/i18n-dashboard.js` → `DICT{}` | 写入 4.2 的 17 个新 key |
| 前端英文词典 | 新建 `cmd/server/web/i18n-dashboard.en.js` | 与 `DICT` 同结构，全 key 英文值 |
| 启用切换 | `cmd/server/web/i18n-dashboard.js` → `setLang(){}` / `getLang()` / `supported` | 实现真实语言切换与重渲染 |
| 语言切换 UI | `cmd/server/web/index.html` 顶栏（主题切换旁） | 新增语言下拉/按钮，写 Cookie `aiops_lang` |
| HTML 接线补全 | `cmd/server/web/index.html` 第三节约列的 16 处 | 改 `data-i18n` / `data-i18n-title` / `data-i18n-placeholder`（多数为“已有 key 未接线”） |
| `<title>` 本地化 | `cmd/server/web/app.js` → `init()` | `document.title = I18N.t("app.title")` |
| JS 硬编码改 `I18N.t` | `cmd/server/web/app.js:1699,1713,1858,1902,2184,2202,2217,2247,4001,4020,5089` | 替换为 `I18N.t("对应key")` |
| 动态内容重渲染 | `cmd/server/web/app.js` 各 `render*()` | 语言切换后需对动态生成的列表/弹窗重新调用 `I18N.applyTranslations()` |

---

## 六、范围决策建议（需你确认）

1. **Agent 端是否纳入本次切换？** Agent 的字符串绝大多数出现在**终端/CLI 上下文**（文件传输进度、CLI 帮助、上报日志）。行业惯例中终端输出通常不随 UI 本地化。建议：
   - **高优先级**：`terminal.go` 经 WebSocket 进浏览器终端的提示（用户确实“看得到”）→ 做 Agent 侧轻量 i18n。
   - **低优先级/可延后**：`relay.go` 错误、`reporter.go` 错误、`main.go` CLI 帮助 → 可暂不做，或仅做 CLI 帮助。
   - 若决定做，Agent 需新增自己的 `T/Tz`（可抽共享包或内嵌小词典），因为 Agent 二进制不依赖服务端 i18n 包。
2. **key 命名边界**：建议后端词典专注 API/通知/邮件/日志/服务端；前端词典专注 UI，删除跨端重复定义（如 `notify.type_*`、`playbook.*` 在两端各一份，以一方为准）。
3. **后端已完备**，本次后端实际工作量很小（仅 `config.go:912` + 可选 Agent）；**主要工作量在前端**（启用切换 + 英文词典 + ~30 处接线/硬编码修复）。

---

## 七、统计

- 后端 `.go` 含中文行：186 行（绝大多数为注释/日志）；**真正未 i18n 的用户可见中文 ≈ 27 处，几乎全在 `cmd/agent/`，服务端仅 `config.go:912` 一处**。
- 后端字典：3 语言齐平、结构完整，`Tr/T/Tz` 覆盖 24 文件。
- 前端 HTML：`data-i18n` 引用 241 处（接线基础好），但仍有 **16 处硬编码未接线**、`<title>` 未本地化。
- 前端 JS：`I18N.t()` 488 处（adoption 高），残留 **11 处硬编码**。
- 前端词典：**仅中文、切换禁用**；需补 17 个新 key + 全套英文翻译。
