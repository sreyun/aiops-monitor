# AIOps Monitor 源码安全与质量审查报告

> 审查范围：全量源码（Go 服务端 `cmd/server/`、Agent 端 `cmd/agent/`、共享 `shared/`、前端 `website/`、管理面板 `cmd/server/web/`、Python 插件 `plugins/`）
> 方法：静态逐文件通读 + 4 路并行专项子代理（安全鉴权 / 业务逻辑与协议 / Agent 端 / 前端与插件）+ 本人对全部高危项逐行复核（已确认 7 项高危属实）
> 代码规模：约 16.7k 行 Go + 前端 + 5 个 Python 插件
> 严重级别：🔴 高（可被直接利用，优先级最高） / 🟠 中（需修复，存在现实风险） / 🟡 低（加固/卫生/前瞻）

---

## 〇、整改优先级总表（先修这些）

| 优先级 | 文件 | 问题 | 一句话修复 |
|---|---|---|---|
| 🔴 高 | `cmd/server/forward.go` | TCP 转发监听器无认证 + 默认绑 0.0.0.0 | 默认 `127.0.0.1`；TCP 握手先校验会话令牌 |
| 🔴 高 | `cmd/agent/relay.go` | 中继是无认证的开放反向代理 | 入站鉴权 + 上游路径白名单 |
| 🔴 高 | `cmd/agent/plugins.go` | 插件目录任意文件被直接执行 | 仅允许 `.py` + 清单/签名 |
| 🔴 高 | `cmd/server/recovery_api.go` | legacy 密码重置绕过 MFA | 启用 MFA 的账户禁用 legacy 流程 |
| 🔴 高 | `cmd/server/auth.go` | `/proxy/` 鉴权跳过 RBAC | proxy token 放行后仍需 `routeAllowed` |
| 🔴 高 | `cmd/agent/terminal.go` | 终端/转发通道仅认可预测指纹，无 token | 通道携带并校验注册 token/会话密钥 |
| 🟠 中 | `cmd/server/forward.go` | `copyRule` listener=nil 导致 panic | 复制复用建链逻辑 |
| 🟠 中 | `cmd/server/forward.go` | `toggleRule` 不真正停止监听 | 禁用时 `listener.Close()` |
| 🟠 中 | `cmd/server/forward.go` | `updateRule` 改端口不重绑 | 端口变更重建监听 |
| 🟠 中 | `cmd/server/auth.go` | 密码哈希用加盐 SHA-256（快速哈希） | 迁移 bcrypt/argon2id |
| 🟠 中 | `cmd/server/config.go` | 密钥明文落盘 + 文件 0644 | 权限 0600 + AES-GCM 加密敏感字段 |
| 🟠 中 | `cmd/server/config.go` | 默认 admin/admin 且无强改密 | 首次随机口令 + 强制改密 |
| 🟠 中 | `cmd/server/db.go` | 会话令牌明文持久化 + 宽松权限 | 0600 + 令牌加密 |
| 🟠 中 | `cmd/agent/terminal.go` | 文件上传/下载路径不受限（任意读写） | 沙箱目录 + 路径规范化校验 |
| 🟠 中 | `cmd/server/store.go` | Agent 自报 `Category` 影响 playbook 目标 | 以服务端 override 为准 |
| 🟠 中 | `website/css` / `js` | 无 JS 时核心内容不可见 | 渐进增强 `html.js` 类 |
| 🟠 中 | `website/css` | FAQ 手风琴固定 max-height 截断长答案 | `grid 0fr→1fr` 或 JS 设 scrollHeight |
| 🟠 中 | `website/css` | `--muted2` 对比度不足（<4.5:1） | 调亮至 ≥4.5:1 |
| 🟡 低 | 多处 | CORS `*`、Cookie 缺 HttpOnly/Secure、令牌回退固定值、SSRF、可访问性缺失等 | 见各节 |

---

## 一、服务端安全与鉴权维度

### 1.1 `cmd/server/auth.go`
- 🔴 **【高】`/proxy/` 鉴权绕过 RBAC（`auth.go:402-413`）**
  - 问题：`authMiddleware` 对 `/proxy/` 前缀在 proxy token 校验通过后直接 `next.ServeHTTP` 并 `return`，**完全跳过 `routeAllowed` 的 RBAC 判断**。而 `routeAllowed` 对 `GET /api/v1/proxy-token` 因是 GET 仅要求 viewer+，导致最低权限 viewer 也能 mint 代理令牌。
  - 影响：垂直越权——viewer 获得 operator 专属的 HTTP 代理能力，并绕过 MFA 注册强制与终端二次验证约束。
  - 修复：proxy token 命中后**仍调用 `routeAllowed(r, s.cfg.RoleOf(user))`**；`handleProxyToken` 仅对 operator+ 开放；令牌已记录 `user` 字段，可直接带入 RBAC。
- 🟠 **【中】密码哈希为加盐 SHA-256（`auth.go:20-25`）**
  - 问题：SHA-256 无工作因子，配置文件泄露后 GPU 可大规模并行爆破。
  - 修复：迁移 `golang.org/x/crypto/bcrypt`（或 argon2id）；兼容旧哈希（前缀 `bcrypt$` 走新算法，否则旧算法校验并在登录成功时升级重写）。
- 🟠 **【中】登录口令最小长度仅 4（`auth.go:647`）**
  - 问题：核心登录口令策略（4 位）反而弱于"次要"的终端二次口令（8 位+复杂度，`terminal_auth.go`）。
  - 修复：统一口令策略，登录/账户口令 ≥8 位并支持复杂度；抽象 `validatePasswordStrength` 共用。
- 🟠 **【中】proxy_token 经 URL `?pt=` 传递，cookie 无 HttpOnly/Secure（`auth.go:406`、`forward_api.go:170`）**
  - 影响：令牌进访问日志/Referer/历史；XSS 可窃取；明文 HTTP 下随请求明文发送。
  - 修复：移除 `pt` query 回退，仅用 cookie；cookie 加 `HttpOnly; Secure`；敏感操作 `SameSite=Strict`。
- 🟠 **【中】会话 Cookie 仅 `SameSite=Lax`，`Secure` 依赖可伪造的 `X-Forwarded-Proto`（`auth.go:551-566`、`helpers.go:46-48`）**
  - 修复：增加配置项强制 `Secure`；部署文档强制 TLS 反代。
- 🟡 **【低】随机数失败回退固定值（`auth.go:131`）** → 失败应返回 error，绝不返回可预测固定串。
- 🟡 **【低】配置保存错误回显 `err.Error()`（`auth.go:739`）** → 向客户端返回通用错误，详情仅记日志。

### 1.2 `cmd/server/recovery_api.go`
- 🔴 **【高】legacy 密码重置/找回绕过 MFA（`recovery_api.go:234-258`、`299-341`、`264-295`）**
  - 问题：新流程（recover-send-code → verify → verify-mfa）对已启用 MFA 的账户正确要求 TOTP；但**遗留端点 `handleResetPassword` Path B、 `handleSendResetCode`、`handleRecoverUsername` 仍开放且仅校验邮件码**，启用 MFA 的账户可被仅持邮件码重置口令，完全旁路第二因子。
  - 影响：账户找回是攻击者首选突破口，MFA 核心控制对"账户恢复"失效，可接管管理员。
  - 修复：legacy 流程若 `user.MFAEnabled` 为真则拒绝，统一收敛到带 MFA 的新流程。

### 1.3 `cmd/server/config.go`
- 🟠 **【中】密钥明文落盘 + 文件权限 0644（`config.go:30/19/161`、`646` `os.WriteFile(...,0o644)`）**
  - 影响：SMTP 密码/钉钉密钥/install token/用户 Hash/Salt/MFASecret 同机任意本地用户可读。
  - 修复：`save()` 改 `0o600`；敏感字段 AES-GCM + 随机 salt 加密；install token/MFA secret 不进日志。
- 🟠 **【中】默认管理员 admin/admin，无首次强改密（`config.go:114-123`、`276`）**
  - 修复：首次运行若用默认凭据，生成随机初始口令并一次性打印，强制下次登录改密。
- 🟡 **【低】install token 旋转后旧 token 仍有效 7 天（`config.go:422-439`）** → 提供立即吊销接口，宽限期可配置并缩短。
- 🟡 **【低】回退固定令牌值（`config.go:292`、`email.go:155`）** → 失败时返回 error。

### 1.4 `cmd/server/users_api.go` / `users.go` / `terminal_auth.go`
- 🟠 **【中】`createUser`/`resetUserPassword` 口令最小长度 4（`users_api.go:45,118`）** → 同 1.1，统一策略。
- ✅ `users.go` RBAC 角色等级、禁止降级最后管理员、禁止删除最后用户实现正确；`terminal_auth.go` 终端口令强度与限流设计良好（暂无高危）。

### 1.5 `cmd/server/admin_api.go` / `install.go` / `qrcode.go` / `handlers.go` / `helpers.go` / `main.go`
- 🟠 **【中】install token 泄露给最低 viewer（`admin_api.go:62-68` + `auth.go:385-388`）**
  - 影响：viewer 可读 install_token，据以注册伪造 agent 污染监控。
  - 修复：`/install/info`、`/install/reset-token` 限制为 operator+；或仅返回掩码，重置后一次性展示。
- 🟠 **【中】服务端默认 HTTP 明文监听无 TLS（`main.go:174-233`）** → 增 `-tls-cert/-tls-key` 或强制 TLS 反代。
- 🟡 **【低】CORS `Access-Control-Allow-Origin:*`（`main.go:73-84`）** → 收敛为受信源白名单。
- ✅ `install.go` 安装脚本参数 sanitize、公共端点不回退注入真实 token 设计正确；`qrcode.go` 仅生成 PNG 无问题；`helpers.go` 的 `clientIP` 默认忽略代理头（防伪造）正确。

---

## 二、端口转发维度（`forward.go` / `forward_api.go`）

- 🔴 **【高】TCP 转发监听器完全无认证 + 默认绑 0.0.0.0（`forward.go:698-710`、`config.go:350-355`）**
  - 问题：`serveForwardListener` 直接 `listener.Accept()` 后进入 `handleForwardTCPConn` 建立到 Agent 的隧道，**无任何认证**，而 `ForwardListenAddr()` 默认 `0.0.0.0`。
  - 影响：防火墙放行转发端口后，任意未登录网络客户端即可把流量隧道进被监控主机的 `localhost` 内网服务（Redis/MySQL/SSH 等），实现横向移动与凭据泄露。
  - 修复：①默认 `127.0.0.1`；②`handleForwardTCPConn` 入口要求带会话令牌/一次性票据握手；③UI 创建规则时显式提示暴露风险。
- 🟠 **【中】`copyRule` listener=nil 导致 goroutine panic（`forward.go:575-596` + `forward_api.go:299`）**
  - 问题：`copyRule` 造 `listener:nil` 的规则，`handleForwardCopy` 立即 `go serveForwardListener` → `rule.listener.Accept()` 空指针 panic，且残留孤立规则。
  - 修复：复制时复用 `createRule` 完整建链（真正 `net.Listen`）。
- 🟠 **【中】`toggleRule(false)` 不真正停止监听器（`forward.go:534-548`）**
  - 问题：仅设 `r.enabled`，`serveForwardListener` 循环从不读 `enabled`，隧道照常接受连接。
  - 影响：操作者以为已禁用转发（切断敏感内网访问），实际仍运行。
  - 修复：禁用时 `rule.listener.Close()`，或在 `handleForwardTCPConn` 入口拒绝 `!enabled`。
- 🟠 **【中】`updateRule` 改 `localPort` 后未重绑（`forward.go:553-572`）**
  - 问题：只改字段不重绑 `net.Listener`，实际监听端口不变，配置与实际不一致。
  - 修复：端口变更先 `listener.Close()` 再重建。
- 🟠 **【中】`handleForwardCreate`/`handleHTTPProxy` 不校验 `hostID` 真实存在（`forward.go:623-670`、`806-853`）**
  - 修复：创建前 `GetHost(req.HostID)` 校验，不存在直接 400。
- 🟠 **【中】Agent 流无 body 大小上限（`terminal.go:531` exec `io.ReadAll` 无限制；交互模式单帧上限 100MB）** → exec 加 `io.LimitReader`；交互上限降至 1–4MB。
- 🟠 **【中】Agent 长轮询 POST 无读取截止（slowloris，`forward.go:1318-1376`、`terminal.go:515-607`）** → 设 `ReadDeadline`/`http.Server.ReadTimeout` 或空闲超时。
- 🟡 **【低】`ws.go:99-142` 未强制客户端帧 mask** → RFC 6455 要求拒绝未掩码帧，建议直接关闭连接。
- 🟡 **【低】`check.go` 自定义检查 server-side SSRF（`check.go:324-368`）** → 检查目标加网段白名单，禁止私有/链路本地地址。
- 🟡 **【低】`check.go:178-183` `runSelfCheck` 空分支死代码** → 删除或加注释。

---

## 三、业务逻辑与 Agent 协议维度

### 3.1 `cmd/server/agent_api.go` / `store.go` / `ws.go`
- 🟠 **【中】Agent 自报 `Category` 覆盖执行面（`store.go:160-167`、`playbook.go:130-136`）**
  - 问题：`UpsertAuthenticated` 用 Agent 上报值覆盖 `Category`，`ResolveTargets("category:xxx")` 据此选 playbook 目标，Agent 可自报任意 Category 把自己塞进目标集。
  - 修复：以服务端 `CategoryOverride` 为准，自报值仅作展示。
- 🟠 **【中】`aiops.db` 明文持久化会话令牌 + 权限宽松（`db.go:124-149`、`auth.go:289-300`）** → `os.OpenFile(...,0600)`；令牌加密或仅持久化必要字段。
- 🟡 **【低，防御性】浅拷贝共享底层切片（`store.go:242-250`、`push.go:61-107`、`notify.go:82`）** → `Evaluate` 在持有读锁时执行，或对历史切片深拷贝。
- ✅ `agent_api.go` 注册需 install token、指纹比对逻辑基本正确；`ws.go` 为 WS 服务端，未发现鉴权绕过。

### 3.2 `cmd/server/check.go` / `alerts.go` / `notify.go` / `playbook.go` / `email.go` / `ui_api.go`
- 🟡 **【低】`email.go:32` SMTP `Username` 未校验 CR/LF（理论头部注入）** → 对 `cfg.Username` 同样做 CR/LF 拒绝。
- 🟡 **【低】内部错误 `err.Error()` 直返前端（`admin_api.go:34`、`check_api.go:65`、`forward.go:657`）** → 统一返回脱敏错误。
- 🟡 **【低】自定义检查底层网络错误经 API 暴露（`check.go:347`）** → 仅暴露错误类别（timeout/conn refused）。
- 🟡 **【低】Playbook 经 Agent `sh -c`/`cmd /c` 无沙箱执行（`terminal.go:151-155`、`playbook_api.go:94`）** → 设计层 RCE-by-design，建议最小权限账户运行 + 命令白名单 + UI 警示。
- ✅ `alerts.go` 告警降噪/限流实现良好；`notify.go` 通道逻辑正常；`playbook.go` 目标解析正确。

---

## 四、Agent 端 — 终端 / PTY / ZMODEM 维度

### 4.1 `cmd/agent/terminal.go`
- 🔴 **【高】终端/Exec 通道仅认可预测指纹，无 token（`terminal.go:91-94`、`:123-142`、与 `forward.go` 的 `termWait/forwardWait` 仅带 `host+fp`）**
  - 问题：`runExecSession` 直接 `exec.CommandContext(ctx, "/bin/sh","-c",command)` 以 Agent 权限（常 root/SYSTEM）执行服务端下发命令；而终端/转发通道**只携带 `host+fp`**，不携带 install token。指纹 `sha256(machineID+primaryMAC)[:12]` 输入本机任意用户可读，且明文出现在 URL query。
  - 影响：服务端/中继被控或网络 MITM 时，攻击者可对全部被监控机器获得 RCE。
  - 修复：①默认 `https://` 并支持证书钉扎/mTLS；②终端/转发通道携带并校验 token 或注册时协商的会话密钥；③服务端侧引入命令授权（白名单/危险命令拒绝/审计）。
- 🟠 **【中】文件上传/下载路径不受限（`terminal.go:324-355` 上传 `os.Create(meta.TargetPath)`；`:396-505` 下载 `os.Open(meta.RemotePath)`）**
  - 影响：伪造服务端可在 Agent 任意路径写文件（覆盖启动项/计划任务）与读任意文件（窃取密钥）。
  - 修复：上传/下载限定到沙箱目录，路径规范化并禁止 `..` 逃逸；敏感目录黑名单。
- 🟠 **【中】ZMODEM sz 下载整文件缓存内存无上限（`zmodem.go:434/500`、`terminal.go:792-818`）** → 设体积上限（如 100MB）超限中止；改为流式落盘+分块回传。
- 🟠 **【中】终端会话无并发上限（`terminal.go:82-97`）** → 每目标加信号量（4–8），超限拒绝。
- 🟠 **【中】rx/tx 长连接无超时，强制关闭时只关 shell 不关 HTTP 流（`terminal.go:51-61`、`:218-252`、`:194-198`）** → 用 `context.WithTimeout` 包裹请求，会话结束时统一 cancel 关闭 rx/tx。
- 🟠 **【中】信号退出未优雅关闭，遗留孤儿子 shell/ConPTY（`main.go:133-139` `os.Exit(0)`）** → 捕获信号后 `cancel()` 全局 context，等待各 goroutine 收尾再退出。
- 🟡 **【低】`pipeShell.Close()` 未关闭读取端 fd（`terminal.go:650-656`）** → `Close()` 中显式 `p.out.Close()`。
- ✅ PTY/管道关闭逻辑（ConPTY 双 Once、Unix PTY 关 slave）基本正确；`zmodem` 越界判断有覆盖。

### 4.2 `cmd/agent/pty_*.go` / `collector_*.go` / `gpu.go`
- 🟡 **【低】Windows 采集器忽略全部 syscall 返回值，失败时静默回 0（`collector_windows.go:89/108/137/228/270/302`）** → 检查 BOOL/NTSTATUS，失败打 `slog.Warn`；`readIfTable` 的硬编码 `rowSize=860`/`off*` 偏移改为基于结构体字段偏移，适配不同 Windows 版本。
- ✅ `pty_linux/unix/darwin/windows` 各自平台实现；`gpu.go` 12s TTL+4s 超时缓存良好。

---

## 五、Agent 端 — Relay / 插件 / 采集维度

### 5.1 `cmd/agent/relay.go`
- 🔴 **【高】Relay 是无认证的开放反向代理（`relay.go:41-49`）**
  - 问题：`handler` 仅拦截 `/install.sh|ps1`，其余一切请求（含 `/api/v1/agent/terminal/*`、`forward/*`、`report`、`register`）原样 `proxy.ServeHTTP` 到上游，**无任何入站鉴权**；代码仅对 `0.0.0.0` 绑定打 warn 但仍照常监听转发。
  - 影响：内网隔离被架空，攻击者可经 relay 以 Agent 身份与云服务器交互；误绑公网即成开放跳板。
  - 修复：①入站鉴权（共享密钥/mTLS/与上游一致 token）；②上游路径白名单（仅放行被代理子集）；③默认绑内网 IP，公网强制 TLS。
- 🟡 **【低】上游路径 `upstream + r.URL.Path` 拼接（`relay.go:134`）** → 用 `url.JoinPath` 规范化并校验白名单前缀；改写失败应告警而非静默返回未改写内容。

### 5.2 `cmd/agent/plugins.go`
- 🔴 **【高】插件目录任意文件被直接执行（`plugins.go:60-88` 发现逻辑 + `:137-160` `runOne` 对 non-.py 直接 `exec`）**
  - 问题：`discover()` 仅跳过 `.json/.txt/.md/.yaml/.yml/.conf/.ini/.cfg/.log` 及 `.`/`_` 前缀，其余（含 `.sh/.bin/.exe/.so` 乃至无扩展名文件）一律视为插件；`runOne` 对非 `.py` 文件直接 `exec.CommandContext(ctx, file)`。
  - 影响：插件目录若被低权限用户或部署流程写入，即可以 Agent 权限（可能 root/SYSTEM）本地提权/持久化。
  - 修复：①默认仅允许 `.py`；②非 .py 若支持需可执行位+签名校验；③插件目录权限过宽则拒绝加载；④引入 manifest 清单仅运行列出项。
- 🟡 **【低】插件循环 panic 后无限快速重启无退避（`reporter.go:388-401`）** → 重启前加指数退避并限连续重启次数。

### 5.3 `cmd/agent/identity.go` / `forward.go`（Agent 侧） / `infra.go` / `main.go`
- 🟠 **【中】转发目标端口来自服务端无白名单（`forward.go:48-56`、`89` `localhost:targetPort`）** → Agent 侧配置允许转发的端口白名单，禁止常见管理端口；转发会话审计。
- 🟡 **【低】Token 明文存 `config.json` 且默认无 TLS（`main.go:24-37`）** → 支持环境变量/密钥库读取；配置权限 0600；默认启用 TLS。
- ✅ `identity.go` 指纹生成、`infra.go` 的 `backoff`/`circuitBreaker`（已修复"熔断后不恢复"）、`reporter.go` 重试退避实现完整；`forward.go`(Agent) 的 concopy 逻辑正常。

---

## 六、前端与面板维度

### 6.1 营销站 `website/*.html` / `css/style.css` / `js/i18n.js` / `js/main.js`
- 🟠 **【中】无 JS 时核心内容完全不可见（`.reveal{opacity:0}` + `main.js` IntersectionObserver 才补 `.visible`）**
  - 影响：脚本被 CSP 拦截/网络失败/运行时抛错时整片空白；损害 SEO 与禁用 JS 用户可访问性。
  - 修复：渐进增强——默认 `.reveal{opacity:1}`，`i18n.js/main.js` 顶部 `document.documentElement.classList.add('js')`，仅 `.js .reveal{opacity:0}`；加 `<noscript>` 兜底。
- 🟠 **【中】FAQ 手风琴固定 `max-height:480px` 截断长答案（`style.css:340-342`）** → 改用 `grid-template-rows:0fr→1fr` 过渡或 JS 设 `scrollHeight`。
- 🟠 **【中】`--muted2:#5a6588` 低对比度（深底上约 3:1，低于 WCAG AA 4.5:1，`style.css:9`）** → 调亮至 `#7c89b0` 以上或仅用于大号/加粗文本。
- 🟡 **【低】首页明文展示默认凭据 `admin / admin`（`index.html:82`、`i18n.js` `hero.creds`）** → 保留"首次登录改密/启用 MFA"提示，移除明文凭据，或仅登录后面板内提示。
- 🟡 **【低】移动端汉堡按钮无 `aria-label`/`aria-expanded`（`index.html:56`、`main.js:16-22`）** → 加 ARIA 并同步状态。
- 🟡 **【低】语言 `<select>` 无关联 `<label>`（`i18n.js:1071-1093`）** → 注入 `<label class="sr-only" for="langSelect">语言</label>`。
- 🟡 **【低】未尊重 `prefers-reduced-motion`（`style.css` 动画/`main.js` 数字滚动）** → 加 `@media (prefers-reduced-motion: reduce)` 关闭动画。
- 🟡 **【低】`i18n.js` 单文件 ~30KB 全量三语字典（`index.html` 前置于 `</body>`）** → 按当前语言分片懒加载或构建期按页拆分；加 `preload`+HTTP 缓存。
- ✅ 营销站无第三方 CDN 脚本；`data-i18n-html`/`innerHTML` 内容均来自开发者静态字典（无外部输入），且 `esc()` 转义到位；`?lang=` 经 `SUPPORTED` 白名单；无开放重定向（无高危）。

### 6.2 管理面板 `cmd/server/web/app.js`
- 🟡 **【低】代理令牌以 URL `?pt=` 传递（`app.js:5558` `window.open(...+"pt="+...)`）** → 改用一次性票据/POST/`#hash`，读取后立即失效。
- ✅ 面板 XSS 防护良好：全局 `esc()` 转义 `& < > "`，服务端返回数据统一转义后拼 DOM；WebSocket 优先 `wss`；无 `eval`/`new Function`；终端经 VT100 仿真渲染原始字节（非 innerHTML）。

---

## 七、Python 插件维度（`plugins/*.py`）

- ✅ **已审查，暂无高危。** 五个脚本均无命令注入、无 `eval/exec` 执行不可信代码、无 `subprocess/os.system` 拼接用户输入。`plugin_sdk.py` 仅收集指标经 `json.dump` 输出；`core_metrics.py`/`example_ai_anomaly.py`/`process_monitor.py`/`example_service_check.py` 行为安全（socket 探活、psutil 采集、本地状态文件，均有 try/except 兜底）。
- 🟡 **【信息/低】插件即"任意 Python 执行"的设计风险（责任在加载器）** → 确保 `plugins/` 权限 `0700` 属主独占；Go 侧对插件签名/清单白名单（见 5.2）；插件输出在 Go 侧严格 `json.Unmarshal`+字段校验。
- 🟡 **【低】多插件各自 `time.sleep(0.3)` 采样延迟叠加（`core_metrics.py:38` 等）** → 采样间隔参数化/由 Agent 注入共享时钟，降低多插件叠加延迟。

---

## 八、各模块功能拓展与加强建议

### 鉴权与账户
- 口令策略可配置化（最小长度/复杂度/历史防重用/锁定），与终端口令统一抽象。
- 引入"可信设备/记住设备"降低 MFA 打扰，保留风险登录二次验证。
- 会话表全量存内存+DB 快照（含 token 明文）→ 仅持久化必要字段并对 token 哈希索引。

### 端口转发 / 终端
- 统一"会话级鉴权"：TCP 监听器接受连接后要求会话/一次性令牌握手帧；默认监听 `127.0.0.1`。
- 所有 Agent 流接口（wait/rx/tx）加全局+每主机并发上限、空闲超时、body 总字节硬上限。
- `toggleRule/updateRule/copyRule` 统一复用"建链/拆链"原语，避免监听器状态与规则字段不一致。
- 文件传输沙箱化 + 断点续传 + 校验和；ZMODEM 下载硬性大小上限。

### Agent 上报 / 存储
- 区分"不可信展示字段"与"可信字段"：自报 `Category` 不作为执行目标；执行面以服务端登记为准。
- `aiops.db` 权限 0600 + 令牌加密；DB 体积上限与压缩可配置。

### 检查 / 通知
- 自定义检查目标加网段白名单（禁私有/链路本地/元数据地址）防 SSRF。
- 通知通道失败退避 + 连续失败熔断，避免 webhook 不可达刷屏；`sendCustomWebhook` 模板预编译缓存。

### API / 安全工程
- CORS 收敛为受信源；公开端点（register/install）加速率限制（目前仅 login 有 IP 限流）。
- 敏感接口（install/info、install/reset-token）收敛 operator+/admin；返回脱敏错误。
- 结构化审计：敏感操作补充"操作前后值"快照；增加失败鉴权/越权尝试聚合指标与告警。
- `/proxy/` 与 `/api/v1/forward` 增加 per-user 配额与访问审计明细。

### 可维护性 / 扩展性
- 抽象统一 `Authorizer` 接口（身份+角色+owner 检查），减少"新增端点忘加 RBAC"回归（F2 即此类）。
- 公共安全函数（`sanitizeXxx`/`maskSecret`）集中到 `security.go` 并补单测（install 注入防护边界）。
- `forward.go`（~1400 行）按"管理器/HTTP代理/WS代理/Agent流"拆分；`rawForwardReader` 补截断/EOF 边界单测。
- 配置 schema 版本与迁移框架，便于后续字段演进（如加密字段标记 `encrypted:"true"`）。

### 前端 / 可访问性 / 性能
- 修复无 JS 不可见（渐进增强）；补 ARIA（汉堡按钮、语言 select、FAQ region）；对比度达标；尊重 `prefers-reduced-motion`。
- `i18n.js` 按语言分片懒加载；营销站加 `Cache-Control`/关键 CSS 内联。
- 管理面板大列表考虑虚拟滚动；统一转义工具为共享 `utils.js` 并补 `"`/`'` 与 `javascript:` 防护。
- 对 `data-i18n-html`/SVG `icon`/`s.visual` 等 innerHTML 注入点建立"仅可信/已校验数据"编码规范与 CR 卡点，防未来翻译外部化引入 XSS。

---

## 九、已审查无高危的文件（确认）

`totp.go`、`users.go`、`terminal_auth.go`、`install.go`、`qrcode.go`、`helpers.go`、`handlers.go`、`agent_api.go`、`ws.go`、`alerts.go`、`notify.go`、`playbook.go`、`playbook_api.go`、`ui_api.go`、`check_api.go`、`store.go`(逻辑层)、`shared/wire.go`、`collector_darwin.go`、`gpu.go`、`infra.go`(熔断修复良好)、`identity.go`、管理面板 `app.js`、全部 Python 插件。

> 注：以上结论基于静态审查与高危项逐行复核。建议在修复高危项（转发鉴权、Relay 鉴权、插件白名单、MFA 恢复、proxy RBAC、终端通道 token）后，再补一轮针对修复代码的回归审查。
