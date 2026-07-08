/* ============================================================
   AIOps Monitor · 多语言 i18n 系统
   支持简体中文(zh-CN) / 繁体中文(zh-TW) / 英文(en)
   ============================================================ */
"use strict";
(function(){

var SUPPORTED = ["zh-CN", "zh-TW", "en"];
var DEFAULT_LANG = "zh-CN";

/* 语言显示名称 */
var LANG_NAMES = {
  "zh-CN": "简体中文",
  "zh-TW": "繁體中文",
  "en": "English"
};

/* ============================================================
   翻译字典 — 按页面分区
   ============================================================ */
var T = {

/* ---------- 通用（导航栏 + 页脚）---------- */
"_common": {
  "zh-CN": {
    "nav.home": "首页", "nav.features": "功能详情", "nav.solutions": "解决方案", "nav.comparison": "产品对比",
    "nav.cta": "免费部署", "nav.deploy": "立即部署 →", "nav.seePain": "了解痛点",
    "footer.desc": "轻量级主机监控运维平台，为中小企业设计。开源免费，零依赖，开箱即用。",
    "footer.product": "产品", "footer.resources": "资源",
    "footer.docs": "使用文档", "footer.install": "安装指南",
    "footer.github": "GitHub 仓库",
    "footer.copy": "© 2026 AIOps Monitor · MIT License · Built with Go"
  },
  "zh-TW": {
    "nav.home": "首頁", "nav.features": "功能詳情", "nav.solutions": "解決方案", "nav.comparison": "產品對比",
    "nav.cta": "免費部署", "nav.deploy": "立即部署 →", "nav.seePain": "了解痛點",
    "footer.desc": "輕量級主機監控運維平台，為中小企業設計。開源免費，零依賴，開箱即用。",
    "footer.product": "產品", "footer.resources": "資源",
    "footer.docs": "使用文檔", "footer.install": "安裝指南",
    "footer.github": "GitHub 倉庫",
    "footer.copy": "© 2026 AIOps Monitor · MIT License · Built with Go"
  },
  "en": {
    "nav.home": "Home", "nav.features": "Features", "nav.solutions": "Solutions", "nav.comparison": "Comparison",
    "nav.cta": "Deploy Free", "nav.deploy": "Deploy Now →", "nav.seePain": "See Pain Points",
    "footer.desc": "Lightweight host monitoring & ops platform, built for SMBs. Open source, zero dependencies, ready out of the box.",
    "footer.product": "Product", "footer.resources": "Resources",
    "footer.docs": "Docs", "footer.install": "Install Guide",
    "footer.github": "GitHub",
    "footer.copy": "© 2026 AIOps Monitor · MIT License · Built with Go"
  }
},

/* ---------- 首页 ---------- */
"index": {
  "zh-CN": {
    "page.title": "AIOps Monitor — 轻量级主机监控运维平台",
    "page.desc": "轻量级主机监控运维平台 — Go 原生采集 + Python 插件层 + 实时面板 + 阈值告警 + 远程终端 + 自动化剧本。单二进制服务端、零依赖 Agent、三平台原生采集（含 GPU）、一条命令安装、开箱即用。",
    "hero.badge": "开源免费 · 单二进制 · 3 分钟完成部署",
    "hero.title": '别再让运维团队<br>被<span class="gradient-text">告警疲劳和手工排查</span>拖垮',
    "hero.desc": "轻量级主机监控运维平台 —— Go 原生采集 + Python 插件层 + 实时面板 + 阈值告警 + 远程终端 + 自动化剧本。单二进制服务端、零依赖 Agent、三平台原生采集（含 GPU）、一条命令安装、开箱即用。",
    "hero.creds": '默认凭据 <code style="background:var(--surface2);padding:2px 8px;border-radius:4px;color:var(--accent2)">admin / admin</code> · 首次登录后请立即修改并启用 MFA',
    "hero.stat1.num": "3 min", "hero.stat1.label": "完成部署",
    "hero.stat2.num": "0", "hero.stat2.label": "外部依赖",
    "hero.stat3.num": "3 平台", "hero.stat3.label": "Linux/Win/macOS",
    "hero.stat4.num": "100%", "hero.stat4.label": "开源免费",
    "pain.tag": "痛点聚焦", "pain.title": "中小企业运维的四大难题",
    "pain.desc": "人力有限、工具分散、告警轰炸、排查靠人肉 —— 这些问题正在吞噬你团队的效率",
    "pain1.title": "人力严重不足",
    "pain1.desc": "1-2 个运维管几十台到上百台机器，日常巡检、故障处理、安全补丁全靠人堆，加班成常态。",
    "pain1.sol": "一条命令自动部署 Agent，批量纳管，运维人力立省 70%",
    "pain2.title": "告警疲劳轰炸",
    "pain2.desc": "每台机器一堆监控项，告警铺天盖地却分不清轻重缓急，真正的严重故障被淹没在噪音里。",
    "pain2.sol": "分级告警（严重/警告）+ 去重冷却 + 桌面通知，只推真正需要处理的",
    "pain3.title": "故障排查耗时",
    "pain3.desc": "出问题先 SSH 上去敲命令查日志，定位全靠经验。多人协作时谁做了什么完全没记录。",
    "pain3.sol": "远程终端免开端口 + 会话回放 + 操作审计，故障 5 分钟定位",
    "pain4.title": "监控工具碎片化",
    "pain4.desc": "Prometheus 管指标、Grafana 看图、Alertmanager 告警、Jira 工单 —— 五六个工具拼起来，部署和维护成本高昂。",
    "pain4.sol": "一个二进制搞定监控+告警+终端+自动化，替代 5+ 工具栈",
    "feat.tag": "核心能力", "feat.title": "一个平台，覆盖运维全链路",
    "feat.desc": "从采集到告警，从终端到自动化，不需要拼接多个工具",
    "feat1.title": "实时监控", "feat1.desc": "CPU / 内存 / SWAP / 多磁盘 / 网络收发 / TCP 连接数 / 负载 / 进程数 / 运行时长 —— 5 秒级采集，多级降采样保留 7 天历史趋势。", "feat1.val": "运维人员无需逐台 SSH 查看状态",
    "feat2.title": "智能告警", "feat2.desc": "分级告警（严重/警告）+ 去重冷却 + 飞书/钉钉/邮件推送 + 桌面通知。告警噪音降低 80%。", "feat2.val": "告警疲劳问题彻底解决",
    "feat3.title": "远程终端", "feat3.desc": "浏览器直连主机终端，Agent 反向连接免开端口。会话全程录制，支持回放追溯和实时旁观。", "feat3.val": "故障排查从 30 分钟缩短到 5 分钟",
    "feat4.title": "自动化运维", "feat4.desc": "可视化剧本编排，批量执行命令到多台主机。执行结果实时回传，历史可追溯。", "feat4.val": "100 台主机的补丁更新，10 分钟搞定",
    "feat5.title": "安全与合规", "feat5.desc": "多用户 RBAC（管理员/操作员/观察员）+ MFA 两步验证 + 会话审计 + 操作日志全记录。支持账户找回（邮箱验证码）和邮箱解除 MFA。", "feat5.val": "满足等保审计要求",
    "feat6.title": "跨平台原生采集", "feat6.desc": "Linux（/proc + syscall）、Windows（Win32 API）、macOS（sysctl）三平台原生采集，AMD64 + ARM64 全覆盖。支持 NVIDIA、AMD、Apple GPU 监控。", "feat6.val": "混合环境一套工具统管",
    "cta.title": "三分钟，让运维变得简单",
    "cta.desc": "一条命令部署服务端，一条命令安装 Agent。无需数据库，无需消息队列，无需任何外部依赖。",
    "cta.cmd": "# 启动服务端\ngit clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor\ndocker compose up -d\n# 浏览器打开 http://localhost:8529",
    "cta.btn2": "查看功能详情"
  },
  "zh-TW": {
    "page.title": "AIOps Monitor — 輕量級主機監控運維平台",
    "page.desc": "輕量級主機監控運維平台 — Go 原生採集 + Python 插件層 + 即時面板 + 閾值告警 + 遠程終端 + 自動化劇本。單二進制服務端、零依賴 Agent、三平台原生採集（含 GPU）、一條命令安裝、開箱即用。",
    "hero.badge": "開源免費 · 單二進制 · 3 分鐘完成部署",
    "hero.title": '別再讓運維團隊<br>被<span class="gradient-text">告警疲勞和手工排查</span>拖垮',
    "hero.desc": "輕量級主機監控運維平台 —— Go 原生採集 + Python 插件層 + 即時面板 + 閾值告警 + 遠程終端 + 自動化劇本。單二進制服務端、零依賴 Agent、三平台原生採集（含 GPU）、一條命令安裝、開箱即用。",
    "hero.creds": '預設憑據 <code style="background:var(--surface2);padding:2px 8px;border-radius:4px;color:var(--accent2)">admin / admin</code> · 首次登入後請立即修改並啟用 MFA',
    "hero.stat1.num": "3 min", "hero.stat1.label": "完成部署",
    "hero.stat2.num": "0", "hero.stat2.label": "外部依賴",
    "hero.stat3.num": "3 平台", "hero.stat3.label": "Linux/Win/macOS",
    "hero.stat4.num": "100%", "hero.stat4.label": "開源免費",
    "pain.tag": "痛點聚焦", "pain.title": "中小企業運維的四大難題",
    "pain.desc": "人力有限、工具分散、告警轟炸、排查靠人肉 —— 這些問題正在吞噬你團隊的效率",
    "pain1.title": "人力嚴重不足",
    "pain1.desc": "1-2 個運維管幾十台到上百台機器，日常巡檢、故障處理、安全補丁全靠人堆，加班成常態。",
    "pain1.sol": "一條命令自動部署 Agent，批量納管，運維人力立省 70%",
    "pain2.title": "告警疲勞轟炸",
    "pain2.desc": "每台機器一堆監控項，告警鋪天蓋地卻分不清輕重緩急，真正的嚴重故障被淹沒在噪音裡。",
    "pain2.sol": "分級告警（嚴重/警告）+ 去重冷卻 + 桌面通知，只推真正需要處理的",
    "pain3.title": "故障排查耗時",
    "pain3.desc": "出問題先 SSH 上去敲命令查日誌，定位全靠經驗。多人協作時誰做了什麼完全沒記錄。",
    "pain3.sol": "遠程終端免開端口 + 會話回放 + 操作審計，故障 5 分鐘定位",
    "pain4.title": "監控工具碎片化",
    "pain4.desc": "Prometheus 管指標、Grafana 看圖、Alertmanager 告警、Jira 工單 —— 五六個工具拼起來，部署和維護成本高昂。",
    "pain4.sol": "一個二進制搞定監控+告警+終端+自動化，替代 5+ 工具棧",
    "feat.tag": "核心能力", "feat.title": "一個平台，覆蓋運維全鏈路",
    "feat.desc": "從採集到告警，從終端到自動化，不需要拼接多個工具",
    "feat1.title": "即時監控", "feat1.desc": "CPU / 記憶體 / SWAP / 多磁碟 / 網路收發 / TCP 連接數 / 負載 / 進程數 / 運行時長 —— 5 秒級採集，多級降採樣保留 7 天歷史趨勢。", "feat1.val": "運維人員無需逐台 SSH 查看狀態",
    "feat2.title": "智能告警", "feat2.desc": "分級告警（嚴重/警告）+ 去重冷卻 + 飛書/釘釘/郵件推送 + 桌面通知。告警噪音降低 80%。", "feat2.val": "告警疲勞問題徹底解決",
    "feat3.title": "遠程終端", "feat3.desc": "瀏覽器直連主機終端，Agent 反向連接免開端口。會話全程錄製，支持回放追溯和即時旁觀。", "feat3.val": "故障排查從 30 分鐘縮短到 5 分鐘",
    "feat4.title": "自動化運維", "feat4.desc": "可視化劇本編排，批量執行命令到多台主機。執行結果即時回傳，歷史可追溯。", "feat4.val": "100 台主機的補丁更新，10 分鐘搞定",
    "feat5.title": "安全與合規", "feat5.desc": "多使用者 RBAC（管理員/操作員/觀察員）+ MFA 兩步驗證 + 會話審計 + 操作日誌全記錄。支持帳號找回（郵箱驗證碼）和郵箱解除 MFA。", "feat5.val": "滿足等保審計要求",
    "feat6.title": "跨平台原生採集", "feat6.desc": "Linux（/proc + syscall）、Windows（Win32 API）、macOS（sysctl）三平台原生採集，AMD64 + ARM64 全覆蓋。支持 NVIDIA、AMD、Apple GPU 監控。", "feat6.val": "混合環境一套工具統管",
    "cta.title": "三分鐘，讓運維變得簡單",
    "cta.desc": "一條命令部署服務端，一條命令安裝 Agent。無需資料庫，無需消息佇列，無需任何外部依賴。",
    "cta.cmd": "# 啟動服務端\ngit clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor\ndocker compose up -d\n# 瀏覽器打開 http://localhost:8529",
    "cta.btn2": "查看功能詳情"
  },
  "en": {
    "page.title": "AIOps Monitor — Lightweight Host Monitoring & Ops Platform",
    "page.desc": "Lightweight host monitoring & ops platform — Go native collection + Python plugins + real-time dashboard + threshold alerts + remote terminal + automation playbooks. Single binary, zero-dependency agent, one-command install.",
    "hero.badge": "Open Source · Single Binary · Deploy in 3 Minutes",
    "hero.title": 'Stop letting your ops team<br>drown in <span class="gradient-text">alert fatigue and manual troubleshooting</span>',
    "hero.desc": "Lightweight host monitoring & ops platform — Go native collection + Python plugins + real-time dashboard + threshold alerts + remote terminal + automation playbooks. Single binary server, zero-dependency agent, native collection on 3 platforms (incl. GPU), one-command install.",
    "hero.creds": 'Default credentials <code style="background:var(--surface2);padding:2px 8px;border-radius:4px;color:var(--accent2)">admin / admin</code> · Change immediately after first login and enable MFA',
    "hero.stat1.num": "3 min", "hero.stat1.label": "To deploy",
    "hero.stat2.num": "0", "hero.stat2.label": "Dependencies",
    "hero.stat3.num": "3 platforms", "hero.stat3.label": "Linux/Win/macOS",
    "hero.stat4.num": "100%", "hero.stat4.label": "Open source",
    "pain.tag": "Pain Points", "pain.title": "Four Headaches in SMB Operations",
    "pain.desc": "Limited staff, fragmented tools, alert bombardment, manual troubleshooting — these problems are eating your team's efficiency",
    "pain1.title": "Severely Understaffed",
    "pain1.desc": "1-2 ops engineers managing dozens to hundreds of machines. Daily inspections, incident response, security patches — all manual, overtime is the norm.",
    "pain1.sol": "One-command agent deployment, bulk onboarding, save 70% ops effort",
    "pain2.title": "Alert Fatigue",
    "pain2.desc": "Each machine has dozens of metrics. Alerts flood in with no way to distinguish critical from noise. Real incidents get buried.",
    "pain2.sol": "Tiered alerts (critical/warning) + dedup cooldown + desktop notifications",
    "pain3.title": "Slow Troubleshooting",
    "pain3.desc": "SSH in, run commands, check logs — diagnosis depends on experience. No record of who did what during collaborative debugging.",
    "pain3.sol": "Port-free remote terminal + session replay + command audit, 5-min root cause",
    "pain4.title": "Fragmented Tooling",
    "pain4.desc": "Prometheus for metrics, Grafana for dashboards, Alertmanager for alerts, Jira for tickets — five or six tools stitched together at high cost.",
    "pain4.sol": "One binary replaces 5+ tools: monitoring + alerts + terminal + automation",
    "feat.tag": "Core Capabilities", "feat.title": "One Platform, Full Ops Coverage",
    "feat.desc": "From collection to alerts, from terminal to automation — no need to stitch multiple tools",
    "feat1.title": "Real-time Monitoring", "feat1.desc": "CPU / Memory / SWAP / Disk / Network / Load / GPU — 5-second collection, multi-tier downsampling retains 7-day trends.", "feat1.val": "No more SSH top/free/df on every host",
    "feat2.title": "Smart Alerts", "feat2.desc": "Tiered alerts (critical/warning) + dedup cooldown + Feishu/DingTalk/Email push + desktop notifications. Alert noise reduced 80%.", "feat2.val": "Alert fatigue solved completely",
    "feat3.title": "Remote Terminal", "feat3.desc": "Browser-to-host terminal via agent reverse connection (no inbound ports). Full session recording, replay, and live observation.", "feat3.val": "Troubleshooting from 30 min to 5 min",
    "feat4.title": "Automation Playbooks", "feat4.desc": "Visual playbook orchestration, batch-execute commands to multiple hosts. Real-time results, full execution history.", "feat4.val": "Patch 100 hosts in 10 minutes",
    "feat5.title": "Security & Compliance", "feat5.desc": "Multi-user RBAC (admin/operator/viewer) + MFA two-step verification + session audit + full operation logging. Account recovery via email.", "feat5.val": "Meets compliance audit requirements",
    "feat6.title": "Cross-Platform Native", "feat6.desc": "Linux (/proc + syscall), Windows (Win32 API), macOS (sysctl) — AMD64 + ARM64. NVIDIA, AMD, Apple GPU support.", "feat6.val": "One tool for mixed environments",
    "cta.title": "Make Operations Simple in 3 Minutes",
    "cta.desc": "One command to start the server, one command to install the agent. No database, no message queue, no external dependencies.",
    "cta.cmd": "# Start the server\ngit clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor\ndocker compose up -d\n# Open http://localhost:8529",
    "cta.btn2": "View Features"
  }
},

/* ---------- 功能详情页 ---------- */
"features": {
  "zh-CN": {
    "page.title": "功能详情 — AIOps Monitor",
    "page.desc": "三平台原生采集、全面指标、GPU 监控、交互式趋势图、自定义拨测、远程终端、自动化剧本、告警推送、多用户 RBAC、MFA、账户找回、多服务端推送、网关中继、机器指纹鉴权、PWA 安装 — 全功能详解",
    "head.tag": "功能详情", "head.title": "每个功能都为运维效率而生",
    "head.desc": "不是技术特性的罗列，而是解决实际运维问题的业务能力",
    "f1.title": "实时指标监控", "f1.desc1": "CPU 使用率、内存/SWAP、多磁盘、网络收发速率、系统负载、进程数、TCP 连接数、运行时间 —— 5 秒级采集，全面覆盖。", "f1.desc2": "GPU 监控：NVIDIA（nvidia-smi）、AMD（Linux sysfs）、Apple（macOS ioreg），best-effort + 缓存。多级降采样保留 7 天历史趋势。", "f1.val": "告别逐台 SSH top/free/df 查看指标",
    "f2.title": "自定义拨测监控", "f2.desc1": "HTTP 状态码检测、TCP 端口连通性、Ping 延迟、关键进程存活 —— 四种拨测类型覆盖所有服务可用性场景。", "f2.desc2": "内置历史曲线图，支持框选放大和全屏预览。", "f2.val": "服务不可达第一时间发现",
    "f3.title": "多渠道智能告警", "f3.desc1": "CPU/内存/磁盘/负载/GPU/离线 六类阈值告警，分级严重/警告。飞书 Webhook、钉钉 Webhook（HMAC 签名）、SMTP 邮件三渠道推送。", "f3.desc2": "事件去重冷却（5 分钟内相同事件不重复推送），桌面通知 + 声音提醒。", "f3.val": "告警噪音降低 80%，只看需要处理的",
    "f4.title": "远程终端（免开端口）", "f4.desc1": "浏览器直接打开主机终端，Agent 反向连接服务端，无需在主机上开放任何入站端口。支持多标签页、窗口大小自适应。", "f4.desc2": "完整 VT100 仿真器，支持 vim/top 等全屏程序。移动端虚拟键盘支持。", "f4.val": "故障排查不用 VPN + SSH，浏览器直达",
    "f5.title": "自动化剧本编排", "f5.desc1": "可视化定义命令序列，一键批量执行到多台主机。执行结果实时回传，支持成功/失败状态统计和步骤级输出查看。", "f5.desc2": "执行历史完整保留，操作者、时间、结果全部可追溯。", "f5.val": "100 台主机批量打补丁，10 分钟完成",
    "f6.title": "多用户 RBAC + MFA", "f6.desc1": "三级角色权限控制：管理员（全部操作）、操作员（终端+告警）、观察员（只读查看）。路由级权限拦截，用户管理界面。", "f6.desc2": "TOTP 两步验证（RFC 6238，Google Authenticator 兼容）。支持账户找回：忘记用户名/忘记密码（邮箱验证码）/ 邮箱解除 MFA，防枚举。", "f6.val": "团队协作安全合规，满足等保要求",
    "f7.title": "终端会话回放", "f7.desc1": "所有远程终端会话全程录制（时间戳帧），支持 1x/2x/4x/8x 倍速回放。实时旁观功能让多人同时查看活跃会话。", "f7.desc2": "会话列表支持按操作者用户名、主机名、IP 地址三维搜索。", "f7.val": "谁做了什么，什么时候做的，完整可追溯",
    "f8.title": "Python 插件扩展", "f8.desc1": "内置插件 SDK，几行代码即可采集自定义指标（MySQL 连接数、Nginx 请求量、Redis 内存等）。", "f8.desc2": "内置示例：进程监控、服务端口探活、轻量 AI 异常检测（z-score）。", "f8.val": "监控什么你说了算，不限于内置指标",
    "f9.title": "PWA 离线访问", "f9.desc1": "支持安装到桌面（PWA），独立窗口运行。App Shell 离线缓存，断网时仍可查看最后已知状态。", "f9.desc2": "WebSocket 实时推送 + 轮询降级，网络恢复自动重连。", "f9.val": "手机也能装，随时随地查看监控",
    "f10.title": "多服务端推送", "f10.desc1": "单个 Agent 同时向多个服务端推送数据，采集一次广播所有。各服务端独立鉴权/重试，适合容灾或多团队共享监控。", "f10.val": "一套 Agent，多套监控面板",
    "f11.title": "网关中继模式", "f11.desc1": "内网仅一台联网机器代理所有请求到云端，二进制/上报/终端自动穿透。适合跨网段或防火墙后的主机纳管。", "f11.val": "无需每台机器都开外网",
    "f12.title": "机器指纹鉴权", "f12.desc1": "machine-id + MAC 哈希指纹绑定，Agent 终端通道按指纹鉴权（非 Token）。Token 轮换不影响已装 Agent，7 天宽限期。", "f12.val": "Token 轮换不中断已部署 Agent",
    "cta.title": "这些功能，一个二进制全部包含", "cta.desc": "不需要 Prometheus + Grafana + Alertmanager + 堡垒机 + 工单系统 —— AIOps Monitor 一个搞定", "cta.btn2": "查看解决方案"
  },
  "zh-TW": {
    "page.title": "功能詳情 — AIOps Monitor",
    "page.desc": "三平台原生採集、全面指標、GPU 監控、互動式趨勢圖、自定義撥測、遠程終端、自動化劇本、告警推送、多使用者 RBAC、MFA、帳號找回、多服務端推送、網關中繼、機器指紋鑑權、PWA 安裝 — 全功能詳解",
    "head.tag": "功能詳情", "head.title": "每個功能都為運維效率而生",
    "head.desc": "不是技術特性的羅列，而是解決實際運維問題的業務能力",
    "f1.title": "即時指標監控", "f1.desc1": "CPU 使用率、記憶體/SWAP、多磁碟、網路收發速率、系統負載、進程數、TCP 連接數、運行時間 —— 5 秒級採集，全面覆蓋。", "f1.desc2": "GPU 監控：NVIDIA（nvidia-smi）、AMD（Linux sysfs）、Apple（macOS ioreg），best-effort + 快取。多級降採樣保留 7 天歷史趨勢。", "f1.val": "告別逐台 SSH top/free/df 查看指標",
    "f2.title": "自定義撥測監控", "f2.desc1": "HTTP 狀態碼檢測、TCP 端口連通性、Ping 延遲、關鍵進程存活 —— 四種撥測類型覆蓋所有服務可用性場景。", "f2.desc2": "內建歷史曲線圖，支持框選放大和全螢幕預覽。", "f2.val": "服務不可達第一時間發現",
    "f3.title": "多渠道智能告警", "f3.desc1": "CPU/記憶體/磁碟/負載/GPU/離線 六類閾值告警，分級嚴重/警告。飛書 Webhook、釘釘 Webhook（HMAC 簽名）、SMTP 郵件三渠道推送。", "f3.desc2": "事件去重冷卻（5 分鐘內相同事件不重複推送），桌面通知 + 聲音提醒。", "f3.val": "告警噪音降低 80%，只看需要處理的",
    "f4.title": "遠程終端（免開端口）", "f4.desc1": "瀏覽器直接打開主機終端，Agent 反向連接服務端，無需在主機上開放任何入站端口。支持多分頁、視窗大小自適應。", "f4.desc2": "完整 VT100 模擬器，支持 vim/top 等全螢幕程式。行動端虛擬鍵盤支持。", "f4.val": "故障排查不用 VPN + SSH，瀏覽器直達",
    "f5.title": "自動化劇本編排", "f5.desc1": "可視化定義命令序列，一鍵批量執行到多台主機。執行結果即時回傳，支持成功/失敗狀態統計和步驟級輸出查看。", "f5.desc2": "執行歷史完整保留，操作者、時間、結果全部可追溯。", "f5.val": "100 台主機批量打補丁，10 分鐘完成",
    "f6.title": "多使用者 RBAC + MFA", "f6.desc1": "三級角色權限控制：管理員（全部操作）、操作員（終端+告警）、觀察員（唯讀查看）。路由級權限攔截，使用者管理介面。", "f6.desc2": "TOTP 兩步驗證（RFC 6238，Google Authenticator 相容）。支持帳號找回：忘記使用者名/忘記密碼（郵箱驗證碼）/ 郵箱解除 MFA，防枚舉。", "f6.val": "團隊協作安全合規，滿足等保要求",
    "f7.title": "終端會話回放", "f7.desc1": "所有遠程終端會話全程錄製（時間戳幀），支持 1x/2x/4x/8x 倍速回放。即時旁觀功能讓多人同時查看活躍會話。", "f7.desc2": "會話列表支持按操作者使用者名、主機名、IP 地址三維搜索。", "f7.val": "誰做了什麼，什麼時候做的，完整可追溯",
    "f8.title": "Python 插件擴展", "f8.desc1": "內建插件 SDK，幾行程式碼即可採集自定義指標（MySQL 連接數、Nginx 請求量、Redis 記憶體等）。", "f8.desc2": "內建範例：進程監控、服務端口探活、輕量 AI 異常檢測（z-score）。", "f8.val": "監控什麼你說了算，不限於內建指標",
    "f9.title": "PWA 離線訪問", "f9.desc1": "支持安裝到桌面（PWA），獨立視窗運行。App Shell 離線快取，斷網時仍可查看最後已知狀態。", "f9.desc2": "WebSocket 即時推送 + 輪詢降級，網路恢復自動重連。", "f9.val": "手機也能裝，隨時隨地查看監控",
    "f10.title": "多服務端推送", "f10.desc1": "單個 Agent 同時向多個服務端推送資料，採集一次廣播所有。各服務端獨立鑑權/重試，適合容災或多團隊共享監控。", "f10.val": "一套 Agent，多套監控面板",
    "f11.title": "網關中繼模式", "f11.desc1": "內網僅一台聯網機器代理所有請求到雲端，二進制/上報/終端自動穿透。適合跨網段或防火牆後的主機納管。", "f11.val": "無需每台機器都開外網",
    "f12.title": "機器指紋鑑權", "f12.desc1": "machine-id + MAC 雜湊指紋綁定，Agent 終端通道按指紋鑑權（非 Token）。Token 輪換不影響已裝 Agent，7 天寬限期。", "f12.val": "Token 輪換不中斷已部署 Agent",
    "cta.title": "這些功能，一個二進制全部包含", "cta.desc": "不需要 Prometheus + Grafana + Alertmanager + 堡壘機 + 工單系統 —— AIOps Monitor 一個搞定", "cta.btn2": "查看解決方案"
  },
  "en": {
    "page.title": "Features — AIOps Monitor",
    "page.desc": "Real-time monitoring, custom probes, smart alerts, remote terminal, automation playbooks, RBAC, MFA, session replay, multi-server push, gateway relay, fingerprint auth, PWA — full feature list",
    "head.tag": "Features", "head.title": "Every Feature Built for Ops Efficiency",
    "head.desc": "Not a list of technical specs — but business capabilities that solve real operations problems",
    "f1.title": "Real-time Metrics", "f1.desc1": "CPU, Memory/SWAP, disk space, network rates, system load, process count, TCP connections, uptime — 5-second collection.", "f1.desc2": "GPU monitoring: NVIDIA (nvidia-smi), AMD (Linux sysfs), Apple (macOS ioreg). Multi-tier downsampling retains 7-day trends.", "f1.val": "No more SSH top/free/df on every host",
    "f2.title": "Custom Health Probes", "f2.desc1": "HTTP status code, TCP port connectivity, Ping latency, process liveness — four probe types covering all availability scenarios.", "f2.desc2": "Built-in history charts with box-select zoom and full-screen preview.", "f2.val": "Detect service outages instantly",
    "f3.title": "Multi-Channel Alerts", "f3.desc1": "CPU/Memory/Disk/Load/GPU/Offline threshold alerts, tiered critical/warning. Feishu Webhook, DingTalk (HMAC), SMTP email.", "f3.desc2": "Event dedup cooldown + desktop notifications + sound alerts.", "f3.val": "80% less alert noise, only what matters",
    "f4.title": "Remote Terminal (Port-Free)", "f4.desc1": "Browser-to-host terminal via agent reverse connection — no inbound ports needed. Multi-tab, auto-resize.", "f4.desc2": "Full VT100 emulator supporting vim/top. Mobile virtual keyboard support.", "f4.val": "No VPN + SSH, browser direct",
    "f5.title": "Automation Playbooks", "f5.desc1": "Visual command sequence orchestration, one-click batch execution to multiple hosts. Real-time results, step-level output.", "f5.desc2": "Full execution history with operator, timing, and results.", "f5.val": "Patch 100 hosts in 10 minutes",
    "f6.title": "RBAC + MFA", "f6.desc1": "Three-tier roles: admin (full), operator (terminal+alerts), viewer (read-only). Route-level permission enforcement.", "f6.desc2": "TOTP two-step verification (RFC 6238, Google Authenticator compatible). Account recovery: username/password reset via email, MFA unbind. Brute-force protection.", "f6.val": "Team collaboration, security compliant",
    "f7.title": "Session Replay", "f7.desc1": "All remote terminal sessions recorded (timestamped frames). 1x/2x/4x/8x playback speed. Live observation for multi-user viewing.", "f7.desc2": "Session list searchable by operator username, hostname, IP address.", "f7.val": "Full audit trail — who did what, when",
    "f8.title": "Python Plugin SDK", "f8.desc1": "Built-in plugin SDK — collect custom metrics in a few lines of code (MySQL connections, Nginx requests, Redis memory, etc.).", "f8.desc2": "Built-in examples: process monitor, service port probe, lightweight AI anomaly detection (z-score).", "f8.val": "Monitor what you want, not just built-in",
    "f9.title": "PWA Offline Access", "f9.desc1": "Installable to desktop (PWA), standalone window. App Shell offline cache — view last-known state even when disconnected.", "f9.desc2": "WebSocket real-time push + polling fallback, auto-reconnect on network recovery.", "f9.val": "Install on your phone, monitor anywhere",
    "f10.title": "Multi-Server Push", "f10.desc1": "Single agent pushes to multiple servers simultaneously — collect once, broadcast to all. Independent auth/retry per server.", "f10.val": "One agent, multiple dashboards",
    "f11.title": "Gateway Relay Mode", "f11.desc1": "One internet-connected machine proxies all requests to the cloud — binaries, reporting, and terminal auto-tunnel through.", "f11.val": "No need to expose every machine",
    "f12.title": "Machine Fingerprint Auth", "f12.desc1": "machine-id + MAC hash fingerprint binding. Terminal channel authenticates by fingerprint, not token. Token rotation doesn't affect deployed agents.", "f12.val": "Token rotation never breaks deployed agents",
    "cta.title": "All These Features in One Binary", "cta.desc": "No need for Prometheus + Grafana + Alertmanager + Bastion + Ticketing — AIOps Monitor does it all", "cta.btn2": "View Solutions"
  }
}

}; /* end T */

/* ============================================================
   语言检测 / 切换 / 持久化
   ============================================================ */

/* 从 URL ?lang= 参数获取语言 */
function detectFromURL() {
  var params = new URLSearchParams(window.location.search);
  var lang = params.get("lang");
  if (lang && SUPPORTED.indexOf(lang) >= 0) return lang;
  return null;
}

/* 从 localStorage 获取语言 */
function detectFromStorage() {
  try {
    var lang = localStorage.getItem("aiops_lang");
    if (lang && SUPPORTED.indexOf(lang) >= 0) return lang;
  } catch(e) {}
  return null;
}

/* 从浏览器语言偏好检测 */
function detectFromBrowser() {
  var nav = navigator.language || navigator.userLanguage || "";
  nav = nav.toLowerCase();
  if (nav.indexOf("zh-tw") >= 0 || nav.indexOf("zh-hk") >= 0 || nav.indexOf("zh-mo") >= 0 || nav.indexOf("zh-hant") >= 0) return "zh-TW";
  if (nav.indexOf("zh") >= 0) return "zh-CN";
  return "en";
}

/* 获取当前页面名 */
function getPageName() {
  var path = window.location.pathname.split("/").pop() || "index.html";
  return path.replace(".html", "").replace("-en", "");
}

/* 获取翻译 */
function t(key) {
  var page = getPageName();
  var common = T["_common"] || {};
  var pageT = T[page] || {};
  var dict = common[CURRENT_LANG] || {};
  var pageDict = pageT[CURRENT_LANG] || {};
  return pageDict[key] || dict[key] || key;
}

/* 应用所有翻译 */
function applyTranslations() {
  /* 更新 <html lang> */
  document.documentElement.lang = CURRENT_LANG;

  /* 更新 <title> 和 meta description */
  var titleEl = document.querySelector("title");
  if (titleEl && titleEl.hasAttribute("data-i18n")) {
    titleEl.textContent = t(titleEl.getAttribute("data-i18n"));
  }
  var descEl = document.querySelector('meta[name="description"]');
  if (descEl && descEl.hasAttribute("data-i18n")) {
    descEl.setAttribute("content", t(descEl.getAttribute("data-i18n")));
  }

  /* 更新所有 data-i18n 元素 */
  document.querySelectorAll("[data-i18n]").forEach(function(el) {
    var key = el.getAttribute("data-i18n");
    var val = t(key);
    if (val) el.textContent = val;
  });

  /* 更新所有 data-i18n-html 元素（含 HTML 标签）*/
  document.querySelectorAll("[data-i18n-html]").forEach(function(el) {
    var key = el.getAttribute("data-i18n-html");
    var val = t(key);
    if (val) el.innerHTML = val;
  });

  /* 更新所有 data-i18n-attr 元素（属性翻译）*/
  document.querySelectorAll("[data-i18n-attr]").forEach(function(el) {
    var pairs = el.getAttribute("data-i18n-attr").split(",");
    pairs.forEach(function(pair) {
      var parts = pair.split(":");
      if (parts.length === 2) {
        var val = t(parts[1].trim());
        if (val) el.setAttribute(parts[0].trim(), val);
      }
    });
  });

  /* 更新 hreflang 标签 */
  updateHreflang();

  /* 更新语言切换器当前选项 */
  var switcher = document.getElementById("langSelect");
  if (switcher) switcher.value = CURRENT_LANG;
}

/* 更新 hreflang alternate 标签 */
function updateHreflang() {
  /* 移除旧的 hreflang 标签 */
  document.querySelectorAll('link[rel="alternate"][hreflang]').forEach(function(el) {
    el.remove();
  });
  /* 添加新的 hreflang 标签 */
  SUPPORTED.forEach(function(lang) {
    var link = document.createElement("link");
    link.rel = "alternate";
    link.hreflang = lang;
    link.href = updateURLLang(lang);
    document.head.appendChild(link);
  });
  /* x-default 指向默认语言 */
  var def = document.createElement("link");
  def.rel = "alternate";
  def.hreflang = "x-default";
  def.href = updateURLLang(DEFAULT_LANG);
  document.head.appendChild(def);
}

/* 更新 URL 中的 lang 参数 */
function updateURLLang(lang) {
  var url = new URL(window.location.href);
  url.searchParams.set("lang", lang);
  return url.toString();
}

/* 切换语言 */
function setLang(lang) {
  if (SUPPORTED.indexOf(lang) < 0) lang = DEFAULT_LANG;
  CURRENT_LANG = lang;
  try { localStorage.setItem("aiops_lang", lang); } catch(e) {}
  /* 更新 URL（不刷新页面）*/
  var url = new URL(window.location.href);
  url.searchParams.set("lang", lang);
  window.history.replaceState({}, "", url.toString());
  applyTranslations();
}

/* 初始化 */
var CURRENT_LANG = detectFromURL() || detectFromStorage() || detectFromBrowser();

/* 注入语言切换器 */
function injectLangSwitcher() {
  var nav = document.querySelector(".nav-inner");
  if (!nav) return;
  /* 检查是否已注入 */
  if (document.getElementById("langSelect")) return;
  /* 创建下拉选择器 */
  var wrap = document.createElement("div");
  wrap.style.cssText = "display:flex;align-items:center;gap:8px;margin-right:4px";
  var select = document.createElement("select");
  select.id = "langSelect";
  select.className = "lang-toggle";
  select.style.cssText = "cursor:pointer;font-family:inherit";
  SUPPORTED.forEach(function(lang) {
    var opt = document.createElement("option");
    opt.value = lang;
    opt.textContent = LANG_NAMES[lang];
    if (lang === CURRENT_LANG) opt.selected = true;
    select.appendChild(opt);
  });
  select.addEventListener("change", function() {
    setLang(this.value);
  });
  wrap.appendChild(select);
  /* 插入到 nav-cta 之前 */
  var cta = nav.querySelector(".nav-cta");
  if (cta) {
    nav.insertBefore(wrap, cta);
  } else {
    nav.appendChild(wrap);
  }
}

/* DOM 就绪后执行 */
function init() {
  injectLangSwitcher();
  applyTranslations();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}

})();
