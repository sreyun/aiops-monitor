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
    "nav.home": "首页", "nav.features": "功能详情", "nav.solutions": "解决方案", "nav.comparison": "产品对比", "nav.faq": "常见问题", "nav.contact": "联系我们",
    "nav.cta": "免费部署", "nav.deploy": "立即部署 →", "nav.seePain": "了解痛点",
    "footer.desc": "轻量级主机监控运维平台，为中小企业设计。开源免费，零依赖，开箱即用。",
    "footer.product": "产品", "footer.resources": "资源",
    "footer.docs": "使用文档", "footer.install": "安装指南",
    "footer.github": "GitHub 仓库",
    "footer.copy": "© 2026 AIOps Monitor · MIT License · Built with Go"
  },
  "zh-TW": {
    "nav.home": "首頁", "nav.features": "功能詳情", "nav.solutions": "解決方案", "nav.comparison": "產品對比", "nav.faq": "常見問題", "nav.contact": "聯絡我們",
    "nav.cta": "免費部署", "nav.deploy": "立即部署 →", "nav.seePain": "了解痛點",
    "footer.desc": "輕量級主機監控運維平台，為中小企業設計。開源免費，零依賴，開箱即用。",
    "footer.product": "產品", "footer.resources": "資源",
    "footer.docs": "使用文檔", "footer.install": "安裝指南",
    "footer.github": "GitHub 倉倉",
    "footer.copy": "© 2026 AIOps Monitor · MIT License · Built with Go"
  },
  "en": {
    "nav.home": "Home", "nav.features": "Features", "nav.solutions": "Solutions", "nav.comparison": "Comparison", "nav.faq": "FAQ", "nav.contact": "Contact",
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
    "page.oglocale": "zh_CN",
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
    "arch.tag": "工作原理", "arch.title": "一套架构，覆盖从采集到运维的闭环", "arch.desc": "Agent 反向连接免开端口，数据汇聚到单二进制服务端，告警 / 终端 / 剧本一站完成",
    "arch.linux": "Linux Agent", "arch.linuxSub": "/proc + syscall 原生采集",
    "arch.win": "Windows Agent", "arch.winSub": "Win32 API + ConPTY 终端",
    "arch.mac": "macOS Agent", "arch.macSub": "sysctl + Apple GPU",
    "arch.serverTitle": "AIOps Monitor 服务端", "arch.serverSub": "单二进制 · 零外部依赖",
    "arch.cap1": "指标存储", "arch.cap2": "告警引擎", "arch.cap3": "远程终端", "arch.cap4": "剧本编排", "arch.cap5": "RBAC / MFA",
    "arch.panel": "浏览器实时面板", "arch.panelSub": "PWA · 多端访问",
    "arch.notify": "飞书 / 钉钉 / 邮件", "arch.notifySub": "分级告警推送",
    "arch.multi": "多服务端 / 中继", "arch.multiSub": "容灾 · 跨网段穿透",
    "cta.title": "三分钟，让运维变得简单",
    "cta.desc": "一条命令部署服务端，一条命令安装 Agent。无需数据库，无需消息队列，无需任何外部依赖。",
    "cta.cmd": "# 启动服务端\ngit clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor\ndocker compose up -d\n# 浏览器打开 http://localhost:8529",
    "cta.btn2": "查看功能详情",
    "trust.tag": "技术生态",
    "trust.title": "完美融入你现有的技术栈",
    "trust.desc": "原生支持主流操作系统、容器化部署与团队协作工具，开箱即用，无需改造你的基础设施",
    "integrations": [
      {"name":"Linux","icon":"M9 3v2M15 3v2M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z"},
      {"name":"Windows","icon":"M3 5h8v6H3zM13 5h8v6h-8zM3 13h8v6H3zM13 13h8v6h-8z"},
      {"name":"macOS","icon":"M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zM2 12h20M12 2a15 15 0 0 1 0 20 15 15 0 0 1 0-20z"},
      {"name":"Docker","icon":"M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"},
      {"name":"Python","icon":"M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"},
      {"name":"飞书","icon":"M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"},
      {"name":"钉钉","icon":"M8 12a4 4 0 1 0 8 0 4 4 0 0 0-8 0zM3 12h2M19 12h2M12 3v2M12 19v2"},
      {"name":"SMTP 邮件","icon":"M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M22 6l-10 7L2 6"}
    ],
    "fwd.tag": "独家能力",
    "fwd.title": "浏览器内端口转发，安全暴露内网服务",
    "fwd.desc": "无需在公网开放任何端口，通过 Agent 反向隧道把内网 Web / 数据库 / 调试接口安全地映射到本地浏览器。支持 TCP 与 HTTP 两种模式、列表与卡片双视图，启用 / 禁用 / 复制 / 编辑 / 删除一键完成。",
    "fwd.points": [
      "TCP + HTTP 双模式转发，覆盖数据库、Web 后台、微服务调试",
      "Agent 反向连接，内网服务零公网暴露",
      "列表 / 卡片双视图，转发状态一目了然",
      "启用 / 禁用 / 复制 / 编辑 / 删除，运维操作闭环",
      "转发统计与健康检测，异常及时感知"
    ],
    "fwd.cta": "查看全部功能",
    "faq.tag": "常见问题",
    "faq.title": "关于 AIOps Monitor，你可能想问",
    "faq.desc": "部署、安全、性能、扩展 —— 我们整理了最常见的疑问"
  },
  "zh-TW": {
    "page.title": "AIOps Monitor — 輕量級主機監控運維平台",
    "page.desc": "輕量級主機監控運維平台 — Go 原生採集 + Python 插件層 + 即時面板 + 閾值告警 + 遠程終端 + 自動化劇本。單二進制服務端、零依賴 Agent、三平台原生採集（含 GPU）、一條命令安裝、開箱即用。",
    "page.oglocale": "zh_TW",
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
    "arch.tag": "運作原理", "arch.title": "一套架構，覆蓋從採集到運維的閉環", "arch.desc": "Agent 反向連接免開端口，數據匯聚到單二進制服務端，告警 / 終端 / 劇本一站完成",
    "arch.linux": "Linux Agent", "arch.linuxSub": "/proc + syscall 原生採集",
    "arch.win": "Windows Agent", "arch.winSub": "Win32 API + ConPTY 終端",
    "arch.mac": "macOS Agent", "arch.macSub": "sysctl + Apple GPU",
    "arch.serverTitle": "AIOps Monitor 服務端", "arch.serverSub": "單二進制 · 零外部依賴",
    "arch.cap1": "指標存儲", "arch.cap2": "告警引擎", "arch.cap3": "遠程終端", "arch.cap4": "劇本編排", "arch.cap5": "RBAC / MFA",
    "arch.panel": "瀏覽器即時面板", "arch.panelSub": "PWA · 多端訪問",
    "arch.notify": "飛書 / 釘釘 / 郵件", "arch.notifySub": "分級告警推送",
    "arch.multi": "多服務端 / 中繼", "arch.multiSub": "容災 · 跨網段穿透",
    "cta.title": "三分鐘，讓運維變得簡單",
    "cta.desc": "一條命令部署服務端，一條命令安裝 Agent。無需資料庫，無需消息佇列，無需任何外部依賴。",
    "cta.cmd": "# 啟動服務端\ngit clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor\ndocker compose up -d\n# 瀏覽器打開 http://localhost:8529",
    "cta.btn2": "查看功能詳情",
    "trust.tag": "技術生態",
    "trust.title": "完美融入你現有的技術棧",
    "trust.desc": "原生支持主流作業系統、容器化部署與團隊協作工具，開箱即用，無需改造你的基礎設施",
    "integrations": [
      {"name":"Linux","icon":"M9 3v2M15 3v2M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z"},
      {"name":"Windows","icon":"M3 5h8v6H3zM13 5h8v6h-8zM3 13h8v6H3zM13 13h8v6h-8z"},
      {"name":"macOS","icon":"M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zM2 12h20M12 2a15 15 0 0 1 0 20 15 15 0 0 1 0-20z"},
      {"name":"Docker","icon":"M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"},
      {"name":"Python","icon":"M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"},
      {"name":"飛書","icon":"M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"},
      {"name":"釘釘","icon":"M8 12a4 4 0 1 0 8 0 4 4 0 0 0-8 0zM3 12h2M19 12h2M12 3v2M12 19v2"},
      {"name":"SMTP 郵件","icon":"M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M22 6l-10 7L2 6"}
    ],
    "fwd.tag": "獨家能力",
    "fwd.title": "瀏覽器內端口轉發，安全暴露內網服務",
    "fwd.desc": "無需在公網開放任何端口，透過 Agent 反向隧道把內網 Web / 資料庫 / 除錯介面安全地映射到本地瀏覽器。支援 TCP 與 HTTP 兩種模式、列表與卡片雙視圖，啟用 / 停用 / 複製 / 編輯 / 刪除一鍵完成。",
    "fwd.points": [
      "TCP + HTTP 雙模式轉發，覆蓋資料庫、Web 後台、微服務除錯",
      "Agent 反向連接，內網服務零公網暴露",
      "列表 / 卡片雙視圖，轉發狀態一目了然",
      "啟用 / 停用 / 複製 / 編輯 / 刪除，運維操作閉環",
      "轉發統計與健康檢測，異常及時感知"
    ],
    "fwd.cta": "查看全部功能",
    "faq.tag": "常見問題",
    "faq.title": "關於 AIOps Monitor，你可能想問",
    "faq.desc": "部署、安全、效能、擴展 —— 我們整理了最常見的疑問"
  },
  "en": {
    "page.title": "AIOps Monitor — Lightweight Host Monitoring & Ops Platform",
    "page.desc": "Lightweight host monitoring & ops platform — Go native collection + Python plugins + real-time dashboard + threshold alerts + remote terminal + automation playbooks. Single binary, zero-dependency agent, one-command install.",
    "page.oglocale": "en_US",
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
    "arch.tag": "How It Works", "arch.title": "One Architecture, Full Ops Loop", "arch.desc": "Agents connect reversely (no open ports). Data converges to a single binary server. Alerts, terminal, and playbooks — all in one place.",
    "arch.linux": "Linux Agent", "arch.linuxSub": "/proc + syscall native collection",
    "arch.win": "Windows Agent", "arch.winSub": "Win32 API + ConPTY terminal",
    "arch.mac": "macOS Agent", "arch.macSub": "sysctl + Apple GPU",
    "arch.serverTitle": "AIOps Monitor Server", "arch.serverSub": "Single binary · zero dependencies",
    "arch.cap1": "Metrics Store", "arch.cap2": "Alert Engine", "arch.cap3": "Remote Terminal", "arch.cap4": "Playbooks", "arch.cap5": "RBAC / MFA",
    "arch.panel": "Browser Dashboard", "arch.panelSub": "PWA · multi-device",
    "arch.notify": "Feishu / DingTalk / Email", "arch.notifySub": "Tiered alert push",
    "arch.multi": "Multi-Server / Relay", "arch.multiSub": "DR · cross-subnet tunnel",
    "cta.title": "Make Operations Simple in 3 Minutes",
    "cta.desc": "One command to start the server, one command to install the agent. No database, no message queue, no external dependencies.",
    "cta.cmd": "# Start the server\ngit clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor\ndocker compose up -d\n# Open http://localhost:8529",
    "cta.btn2": "View Features",
    "trust.tag": "Tech Ecosystem",
    "trust.title": "Fits Right Into Your Existing Stack",
    "trust.desc": "Native support for mainstream operating systems, containerized deployment, and team-collaboration tools — ready out of the box, no infrastructure changes needed",
    "integrations": [
      {"name":"Linux","icon":"M9 3v2M15 3v2M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z"},
      {"name":"Windows","icon":"M3 5h8v6H3zM13 5h8v6h-8zM3 13h8v6H3zM13 13h8v6h-8z"},
      {"name":"macOS","icon":"M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zM2 12h20M12 2a15 15 0 0 1 0 20 15 15 0 0 1 0-20z"},
      {"name":"Docker","icon":"M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"},
      {"name":"Python","icon":"M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"},
      {"name":"Feishu","icon":"M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"},
      {"name":"DingTalk","icon":"M8 12a4 4 0 1 0 8 0 4 4 0 0 0-8 0zM3 12h2M19 12h2M12 3v2M12 19v2"},
      {"name":"SMTP Email","icon":"M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M22 6l-10 7L2 6"}
    ],
    "fwd.tag": "Exclusive Capability",
    "fwd.title": "In-Browser Port Forwarding — Safely Expose Internal Services",
    "fwd.desc": "No public ports required. Through the agent's reverse tunnel, map internal web apps, databases, and debug endpoints securely to your local browser. Supports both TCP and HTTP modes, list and card dual views, with enable/disable/copy/edit/delete at your fingertips.",
    "fwd.points": [
      "TCP + HTTP dual-mode forwarding — covers databases, web backends, microservice debugging",
      "Agent reverse connection — zero public exposure for internal services",
      "List + card dual views — forwarding status at a glance",
      "Enable / disable / copy / edit / delete — a closed-loop ops workflow",
      "Forwarding stats and health checks — catch anomalies early"
    ],
    "fwd.cta": "View All Features",
    "faq.tag": "FAQ",
    "faq.title": "Common Questions About AIOps Monitor",
    "faq.desc": "Deployment, security, performance, extensibility — we've compiled the most common questions"
  }
},

/* ---------- 功能详情页 ---------- */
"features": {
  "zh-CN": {
    "page.title": "功能详情 — AIOps Monitor",
    "page.desc": "三平台原生采集、全面指标、GPU 监控、交互式趋势图、自定义拨测、远程终端、自动化剧本、告警推送、多用户 RBAC、MFA、账户找回、多服务端推送、网关中继、机器指纹鉴权、PWA 安装 — 全功能详解",
    "page.oglocale": "zh_CN",
    "head.tag": "功能详情", "head.title": "每个功能都为运维效率而生",
    "head.desc": "不是技术特性的罗列，而是解决实际运维问题的业务能力",
    "groups": [
      {"tag":"01","title":"监控与指标","desc":"从操作系统到业务服务的全栈可见性","items":[
        {"title":"实时指标监控","color":"accent","icon":"M22 12h-4l-3 9L9 3l-3 9H2","desc1":"CPU / 内存 / SWAP / 多磁盘 / 网络收发 / 系统负载 / 进程数 / TCP 连接数 / 运行时长 —— 5 秒级采集，全面覆盖。","desc2":"多级降采样保留 7 天历史趋势，重启后续传不丢点。","val":"告别逐台 SSH top/free/df 查看指标"},
        {"title":"GPU 监控","color":"accent","icon":"M9 3v2M15 3v2M9 19v2M15 19v2M3 9h2M3 15h2M19 9h2M19 15h2M9 9h6v6H9z","desc1":"NVIDIA（nvidia-smi）、AMD（Linux sysfs）、Apple（macOS ioreg）三平台 GPU 采集，best-effort + 缓存。","val":"训练 / 渲染场景的显卡负载一目了然"},
        {"title":"自定义拨测","color":"accent","icon":"M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z","desc1":"HTTP 状态码、TCP 端口、Ping 延迟、关键进程存活 —— 四种拨测覆盖所有可用性场景。","desc2":"内置历史曲线，支持框选放大与全屏预览。","val":"服务不可达第一时间发现"},
        {"title":"交互式趋势图","color":"accent","icon":"M3 3v18h18M7 14l4-4 3 3 5-6","desc1":"Canvas 自绘图表，支持悬停数值、框选缩放、全屏预览，深 / 浅主题自适应。","val":"点一下就能下钻，不用切到 Grafana"},
        {"title":"主机分组与概览","color":"accent","icon":"M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z","desc1":"按业务 / 机房自定义分组；概览 KPI 卡片实时显示在线 / 离线 / 严重告警 / 警告数量。","val":"几百台机器的健康状况，一屏掌握"}
      ]},
      {"tag":"02","title":"告警与通知","desc":"只推你真正需要处理的","items":[
        {"title":"多渠道智能告警","color":"warn","icon":"M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9","desc1":"CPU / 内存 / 磁盘 / 负载 / GPU / 离线 六类阈值告警；飞书 Webhook、钉钉 Webhook（HMAC 签名）、SMTP 邮件三渠道推送。","val":"告警直接进你已经在用的协作工具"},
        {"title":"分级与降噪","color":"warn","icon":"M3 12h4l3-9 4 18 3-9h4","desc1":"严重 / 警告两级，事件去重冷却（5 分钟内相同事件不重复推送），结合噪音抑制，告警量降低 80%。","val":"真正的故障不再被淹没"},
        {"title":"桌面通知","color":"ok","icon":"M9 3v2M15 3v2M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z","desc1":"浏览器 Notification API 桌面弹窗 + 声音提醒，无需打开页面也能第一时间感知。","val":"人不在电脑前也能被叫醒"},
        {"title":"离线即告警","color":"warn","icon":"M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z","desc1":"Agent 30 秒无上报即触发严重离线告警，分布式环境下的主机失联无所遁形。","val":"机器挂了，你比同事先知道"}
      ]},
      {"tag":"03","title":"远程访问与审计","desc":"免开端口，安全可达可追溯","items":[
        {"title":"远程终端","color":"ok","icon":"M4 17l6-6-6-6M12 19h8","desc1":"浏览器直连主机终端，Agent 反向连接免开入站端口。多标签页、窗口自适应、完整 VT100 仿真（vim/top 全屏可用），移动端虚拟键盘。","val":"不用 VPN + SSH，浏览器直达"},
        {"title":"终端会话回放","color":"ok","icon":"M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15","desc1":"所有会话全程录制（时间戳帧），1x/2x/4x/8x 倍速回放；实时旁观让多人同时查看活跃会话；列表支持按操作者 / 主机 / IP 三维搜索。","val":"谁做了什么、何时做的，完整可追溯"},
        {"title":"端口转发（TCP/HTTP）","color":"ok","icon":"M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8M16 6l-4-4-4 4M12 2v13","desc1":"通过 Agent 反向隧道把内网 Web / 数据库 / 调试接口映射到本地浏览器，零公网暴露。TCP + HTTP 双模式，列表 / 卡片双视图。","desc2":"启用 / 禁用 / 复制 / 编辑 / 删除一键完成；转发统计与健康检测接口实时感知异常。","val":"内网服务，随手就能本地访问"},
        {"title":"操作日志与审计","color":"ok","icon":"M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z M14 2v6h6 M16 13H8M16 17H8M10 9H8","desc1":"全量操作日志（操作 / 系统 / 插件三类），支持筛选与 CSV 导出；与终端录制、命令审计共同构成完整审计闭环。","val":"等保测评的审计材料，一键导出"}
      ]},
      {"tag":"04","title":"自动化运维","desc":"把重复劳动交给剧本","items":[
        {"title":"自动化剧本编排","color":"purple","icon":"M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z","desc1":"可视化定义命令序列，一键批量执行到多台主机；执行结果实时回传，支持成功 / 失败统计与步骤级输出。","desc2":"执行历史完整保留，操作者、时间、结果全部可追溯。","val":"100 台主机批量打补丁，10 分钟完成"}
      ]},
      {"tag":"05","title":"安全与权限","desc":"团队协作也要安全合规","items":[
        {"title":"多用户 RBAC","color":"purple","icon":"M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 7a4 4 0 1 0 0 8 4 4 0 0 0 0-8z M23 21v-2a4 4 0 0 0-3-3.87","desc1":"三级角色：管理员（全部操作）、操作员（终端+告警）、观察员（只读）。路由级权限拦截 + 用户管理界面。","val":"不同人看到不同的能力边界"},
        {"title":"MFA 两步验证","color":"purple","icon":"M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5","desc1":"TOTP 两步验证（RFC 6238，兼容 Google Authenticator），登录与敏感操作二次确认。","val":"账密泄露也进不来"},
        {"title":"账户找回","color":"purple","icon":"M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M22 6l-10 7L2 6","desc1":"支持忘记用户名 / 忘记密码（邮箱验证码）/ 邮箱解除 MFA，全程防枚举保护。","val":"管理员离职也不怕锁死"},
        {"title":"机器指纹鉴权","color":"purple","icon":"M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z M9 12l2 2 4-4","desc1":"machine-id + MAC 哈希指纹绑定，Agent 终端通道按指纹鉴权（非 Token）。Token 轮换不影响已装 Agent，7 天宽限期。","val":"Token 轮换不中断已部署 Agent"},
        {"title":"合规审计闭环","color":"purple","icon":"M9 2h6a2 2 0 0 1 2 2v0h3a2 2 0 0 1 2 2v13a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h3a2 2 0 0 1 2-2z M9 14l2 2 4-4","desc1":"终端录制回放 + 操作日志 + MFA + RBAC，满足等保对可追溯、可管控、有记录的要求。","val":"等保测评材料，开箱即得"}
      ]},
      {"tag":"06","title":"部署与架构","desc":"极简到极致，弹性到生产","items":[
        {"title":"单二进制零依赖","color":"accent","icon":"M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z","desc1":"服务端单个 Go 二进制（~15MB），Agent 更小；内存即存储，gzip+JSON 快照持久化，无需 MySQL/Redis/Kafka/时序库。","val":"一台 1 核 1G 的小机器全搞定"},
        {"title":"多服务端推送","color":"accent","icon":"M12 2L2 7v10l10 5 10-5V7L12 2z","desc1":"单个 Agent 同时向多个服务端推送，采集一次广播所有；各端独立鉴权 / 重试，适合容灾或多团队共享监控。","val":"一套 Agent，多套监控面板"},
        {"title":"网关中继模式","color":"accent","icon":"M17 1l4 4-4 4 M3 11V9a4 4 0 0 1 4-4h14 M7 23l-4-4 4-4 M21 13v2a4 4 0 0 1-4 4H3","desc1":"内网仅一台联网机器代理所有请求到云端，二进制 / 上报 / 终端自动穿透，适合跨网段或防火墙后主机。","val":"无需每台机器都开外网"},
        {"title":"PWA 离线访问","color":"accent","icon":"M12 18h.01M8 21h8a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2z M9 7h.01","desc1":"可安装到桌面（PWA），独立窗口运行；App Shell 离线缓存，断网仍看最后已知状态。","val":"手机也能装，随时随地查看"},
        {"title":"安装向导","color":"accent","icon":"M12 3v12M7 10l5 5 5-5 M5 21h14","desc1":"一条 docker compose 启动服务端；install.sh 自动检测 CPU 架构（AMD64/ARM64）并下载对应 Agent 二进制，一条 curl 完成安装。","val":"运维小白也能 3 分钟上线"},
        {"title":"内嵌持久化","color":"accent","icon":"M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z M17 21v-8H7v8 M7 3v5h8","desc1":"gzip + JSON 快照内嵌持久化，无外部数据库；支持配置迁移与一键回滚（Revert），Token 轮换 7 天宽限期。","val":"重启不丢数据，改错能回退"},
        {"title":"实时数据推送","color":"accent","icon":"M13 2L3 14h9l-1 8 10-12h-9l1-8z","desc1":"WebSocket 实时推送，网络异常自动降级为轮询，恢复后无缝重连；gzip 8-10 倍压缩降低带宽。","val":"面板数据永远是最新的"}
      ]},
      {"tag":"07","title":"扩展能力","desc":"监控的边界由你定义","items":[
        {"title":"Python 插件 SDK","color":"purple","icon":"M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4","desc1":"内置插件 SDK，几行代码采集自定义指标（MySQL 连接数、Nginx 请求量、Redis 内存等）。","desc2":"内置示例：进程监控、服务端口探活等。","val":"监控什么你说了算"},
        {"title":"轻量 AI 异常检测","color":"purple","icon":"M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z","desc1":"插件内置 z-score 轻量异常检测，无需额外机器学习平台即可发现指标突变。","val":"异常自动标红，不用盯图表"}
      ]}
    ],
    "cta.title": "这些功能，一个二进制全部包含", "cta.desc": "不需要 Prometheus + Grafana + Alertmanager + 堡垒机 + 工单系统 —— AIOps Monitor 一个搞定", "cta.btn2": "查看解决方案"
  },
  "zh-TW": {
    "page.title": "功能詳情 — AIOps Monitor",
    "page.desc": "三平台原生採集、全面指標、GPU 監控、互動式趨勢圖、自定義撥測、遠程終端、自動化劇本、告警推送、多使用者 RBAC、MFA、帳號找回、多服務端推送、網關中繼、機器指紋鑑權、PWA 安裝 — 全功能詳解",
    "page.oglocale": "zh_TW",
    "head.tag": "功能詳情", "head.title": "每個功能都為運維效率而生",
    "head.desc": "不是技術特性的羅列，而是解決實際運維問題的業務能力",
    "groups": [
      {"tag":"01","title":"監控與指標","desc":"從作業系統到業務服務的全棧可視性","items":[
        {"title":"即時指標監控","color":"accent","icon":"M22 12h-4l-3 9L9 3l-3 9H2","desc1":"CPU / 記憶體 / SWAP / 多磁碟 / 網路收發 / 系統負載 / 進程數 / TCP 連接數 / 運行時間 —— 5 秒級採集，全面覆蓋。","desc2":"多級降採樣保留 7 天歷史趨勢，重啟後續傳不丟點。","val":"告別逐台 SSH top/free/df 查看指標"},
        {"title":"GPU 監控","color":"accent","icon":"M9 3v2M15 3v2M9 19v2M15 19v2M3 9h2M3 15h2M19 9h2M19 15h2M9 9h6v6H9z","desc1":"NVIDIA（nvidia-smi）、AMD（Linux sysfs）、Apple（macOS ioreg）三平台 GPU 採集，best-effort + 快取。","val":"訓練 / 渲染場景的顯卡負載一目了然"},
        {"title":"自定義撥測","color":"accent","icon":"M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z","desc1":"HTTP 狀態碼、TCP 端口、Ping 延遲、關鍵進程存活 —— 四種撥測覆蓋所有可用性場景。","desc2":"內建歷史曲線，支持框選放大與全螢幕預覽。","val":"服務不可達第一時間發現"},
        {"title":"互動式趨勢圖","color":"accent","icon":"M3 3v18h18M7 14l4-4 3 3 5-6","desc1":"Canvas 自繪圖表，支持懸停數值、框選縮放、全螢幕預覽，深 / 淺主題自適應。","val":"點一下就能下鑽，不用切到 Grafana"},
        {"title":"主機分組與概覽","color":"accent","icon":"M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z","desc1":"按業務 / 機房自定義分組；概覽 KPI 卡片即時顯示在線 / 離線 / 嚴重告警 / 警告數量。","val":"幾百台機器的健康狀況，一屏掌握"}
      ]},
      {"tag":"02","title":"告警與通知","desc":"只推你真正需要處理的","items":[
        {"title":"多渠道智能告警","color":"warn","icon":"M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9","desc1":"CPU / 記憶體 / 磁碟 / 負載 / GPU / 離線 六類閾值告警；飛書 Webhook、釘釘 Webhook（HMAC 簽名）、SMTP 郵件三渠道推送。","val":"告警直接進你已經在用的協作工具"},
        {"title":"分級與降噪","color":"warn","icon":"M3 12h4l3-9 4 18 3-9h4","desc1":"嚴重 / 警告兩級，事件去重冷卻（5 分鐘內相同事件不重複推送），結合噪音抑制，告警量降低 80%。","val":"真正的故障不再被淹沒"},
        {"title":"桌面通知","color":"ok","icon":"M9 3v2M15 3v2M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z","desc1":"瀏覽器 Notification API 桌面彈窗 + 聲音提醒，無需打開頁面也能第一時間感知。","val":"人不在電腦前也能被叫醒"},
        {"title":"離線即告警","color":"warn","icon":"M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z","desc1":"Agent 30 秒無上報即觸發嚴重離線告警，分散式環境下的主機失聯無所遁形。","val":"機器掛了，你比同事先知道"}
      ]},
      {"tag":"03","title":"遠程訪問與審計","desc":"免開端口，安全可達可追溯","items":[
        {"title":"遠程終端","color":"ok","icon":"M4 17l6-6-6-6M12 19h8","desc1":"瀏覽器直連主機終端，Agent 反向連接免開入站端口。多分頁、視窗自適應、完整 VT100 模擬（vim/top 全螢幕可用），行動端虛擬鍵盤。","val":"不用 VPN + SSH，瀏覽器直達"},
        {"title":"終端會話回放","color":"ok","icon":"M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15","desc1":"所有會話全程錄製（時間戳幀），1x/2x/4x/8x 倍速回放；即時旁觀讓多人同時查看活躍會話；列表支持按操作者 / 主機 / IP 三維搜索。","val":"誰做了什麼、何時做的，完整可追溯"},
        {"title":"端口轉發（TCP/HTTP）","color":"ok","icon":"M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8M16 6l-4-4-4 4M12 2v13","desc1":"透過 Agent 反向隧道把內網 Web / 資料庫 / 除錯介面映射到本地瀏覽器，零公網暴露。TCP + HTTP 雙模式，列表 / 卡片雙視圖。","desc2":"啟用 / 停用 / 複製 / 編輯 / 刪除一鍵完成；轉發統計與健康檢測接口即時感知異常。","val":"內網服務，隨手就能本地訪問"},
        {"title":"操作日誌與審計","color":"ok","icon":"M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z M14 2v6h6 M16 13H8M16 17H8M10 9H8","desc1":"全量操作日誌（操作 / 系統 / 插件三類），支持篩選與 CSV 匯出；與終端錄製、命令審計共同構成完整審計閉環。","val":"等保測評的審計材料，一鍵匯出"}
      ]},
      {"tag":"04","title":"自動化運維","desc":"把重複勞動交給劇本","items":[
        {"title":"自動化劇本編排","color":"purple","icon":"M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z","desc1":"可視化定義命令序列，一鍵批量執行到多台主機；執行結果即時回傳，支持成功 / 失敗統計與步驟級輸出。","desc2":"執行歷史完整保留，操作者、時間、結果全部可追溯。","val":"100 台主機批量打補丁，10 分鐘完成"}
      ]},
      {"tag":"05","title":"安全與權限","desc":"團隊協作也要安全合規","items":[
        {"title":"多使用者 RBAC","color":"purple","icon":"M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 7a4 4 0 1 0 0 8 4 4 0 0 0 0-8z M23 21v-2a4 4 0 0 0-3-3.87","desc1":"三級角色：管理員（全部操作）、操作員（終端+告警）、觀察員（唯讀）。路由級權限攔截 + 使用者管理介面。","val":"不同人看到不同的能力邊界"},
        {"title":"MFA 兩步驗證","color":"purple","icon":"M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5","desc1":"TOTP 兩步驗證（RFC 6238，相容 Google Authenticator），登入與敏感操作二次確認。","val":"帳密洩露也進不來"},
        {"title":"帳號找回","color":"purple","icon":"M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M22 6l-10 7L2 6","desc1":"支持忘記使用者名 / 忘記密碼（郵箱驗證碼）/ 郵箱解除 MFA，全程防枚舉保護。","val":"管理員離職也不怕鎖死"},
        {"title":"機器指紋鑑權","color":"purple","icon":"M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z M9 12l2 2 4-4","desc1":"machine-id + MAC 雜湊指紋綁定，Agent 終端通道按指紋鑑權（非 Token）。Token 輪換不影響已裝 Agent，7 天寬限期。","val":"Token 輪換不中斷已部署 Agent"},
        {"title":"合規審計閉環","color":"purple","icon":"M9 2h6a2 2 0 0 1 2 2v0h3a2 2 0 0 1 2 2v13a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h3a2 2 0 0 1 2-2z M9 14l2 2 4-4","desc1":"終端錄製回放 + 操作日誌 + MFA + RBAC，滿足等保對可追溯、可管控、有記錄的要求。","val":"等保測評材料，開箱即得"}
      ]},
      {"tag":"06","title":"部署與架構","desc":"極簡到極致，彈性到生產","items":[
        {"title":"單二進制零依賴","color":"accent","icon":"M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z","desc1":"服務端單個 Go 二進制（~15MB），Agent 更小；記憶體即儲存，gzip+JSON 快照持久化，無需 MySQL/Redis/Kafka/時序庫。","val":"一台 1 核 1G 的小機器全搞定"},
        {"title":"多服務端推送","color":"accent","icon":"M12 2L2 7v10l10 5 10-5V7L12 2z","desc1":"單個 Agent 同時向多個服務端推送，採集一次廣播所有；各端獨立鑑權 / 重試，適合容災或多團隊共享監控。","val":"一套 Agent，多套監控面板"},
        {"title":"網關中繼模式","color":"accent","icon":"M17 1l4 4-4 4 M3 11V9a4 4 0 0 1 4-4h14 M7 23l-4-4 4-4 M21 13v2a4 4 0 0 1-4 4H3","desc1":"內網僅一台聯網機器代理所有請求到雲端，二進制 / 上報 / 終端自動穿透，適合跨網段或防火牆後主機。","val":"無需每台機器都開外網"},
        {"title":"PWA 離線訪問","color":"accent","icon":"M12 18h.01M8 21h8a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2z M9 7h.01","desc1":"可安裝到桌面（PWA），獨立視窗運行；App Shell 離線快取，斷網仍看最後已知狀態。","val":"手機也能裝，隨時隨地查看"},
        {"title":"安裝精靈","color":"accent","icon":"M12 3v12M7 10l5 5 5-5 M5 21h14","desc1":"一條 docker compose 啟動服務端；install.sh 自動檢測 CPU 架構（AMD64/ARM64）並下載對應 Agent 二進制，一條 curl 完成安裝。","val":"運維小白也能 3 分鐘上線"},
        {"title":"內嵌持久化","color":"accent","icon":"M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z M17 21v-8H7v8 M7 3v5h8","desc1":"gzip + JSON 快照內嵌持久化，無外部資料庫；支持配置遷移與一鍵回滾（Revert），Token 輪換 7 天寬限期。","val":"重啟不丟數據，改錯能回退"},
        {"title":"即時數據推送","color":"accent","icon":"M13 2L3 14h9l-1 8 10-12h-9l1-8z","desc1":"WebSocket 即時推送，網路異常自動降級為輪詢，恢復後無縫重連；gzip 8-10 倍壓縮降低頻寬。","val":"面板數據永遠是最新的"}
      ]},
      {"tag":"07","title":"擴展能力","desc":"監控的邊界由你定義","items":[
        {"title":"Python 插件 SDK","color":"purple","icon":"M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4","desc1":"內建插件 SDK，幾行程式碼採集自定義指標（MySQL 連接數、Nginx 請求量、Redis 記憶體等）。","desc2":"內建範例：進程監控、服務端口探活等。","val":"監控什麼你說了算"},
        {"title":"輕量 AI 異常檢測","color":"purple","icon":"M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z","desc1":"插件內建 z-score 輕量異常檢測，無需額外機器學習平台即可發現指標突變。","val":"異常自動標紅，不用盯圖表"}
      ]}
    ],
    "cta.title": "這些功能，一個二進制全部包含", "cta.desc": "不需要 Prometheus + Grafana + Alertmanager + 堡壘機 + 工單系統 —— AIOps Monitor 一個搞定", "cta.btn2": "查看解決方案"
  },
  "en": {
    "page.title": "Features — AIOps Monitor",
    "page.desc": "Real-time monitoring, custom probes, smart alerts, remote terminal, automation playbooks, RBAC, MFA, session replay, multi-server push, gateway relay, fingerprint auth, PWA — full feature list",
    "page.oglocale": "en_US",
    "head.tag": "Features", "head.title": "Every Feature Built for Ops Efficiency",
    "head.desc": "Not a list of technical specs — but business capabilities that solve real operations problems",
    "groups": [
      {"tag":"01","title":"Monitoring & Metrics","desc":"Full-stack visibility from OS to business services","items":[
        {"title":"Real-time Metrics","color":"accent","icon":"M22 12h-4l-3 9L9 3l-3 9H2","desc1":"CPU, Memory/SWAP, multi-disk, network I/O, load, process count, TCP connections, uptime — 5-second collection, full coverage.","desc2":"Multi-tier downsampling retains 7-day trends; points resume after restart without gaps.","val":"No more SSH top/free/df on every host"},
        {"title":"GPU Monitoring","color":"accent","icon":"M9 3v2M15 3v2M9 19v2M15 19v2M3 9h2M3 15h2M19 9h2M19 15h2M9 9h6v6H9z","desc1":"NVIDIA (nvidia-smi), AMD (Linux sysfs), Apple (macOS ioreg) GPU collection across platforms, best-effort + cached.","val":"GPU load for training/rendering at a glance"},
        {"title":"Custom Health Probes","color":"accent","icon":"M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z","desc1":"HTTP status, TCP port, Ping latency, process liveness — four probe types cover every availability scenario.","desc2":"Built-in history charts with box-select zoom and full-screen preview.","val":"Detect service outages instantly"},
        {"title":"Interactive Trend Charts","color":"accent","icon":"M3 3v18h18M7 14l4-4 3 3 5-6","desc1":"Canvas-rendered charts with hover values, box-zoom, full-screen preview, and dark/light theme adaptation.","val":"Drill down in one click — no need to switch to Grafana"},
        {"title":"Host Groups & Overview","color":"accent","icon":"M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z","desc1":"Group hosts by business/DC; overview KPI cards show online/offline/critical/warning counts in real time.","val":"Health of hundreds of hosts, on one screen"}
      ]},
      {"tag":"02","title":"Alerting & Notifications","desc":"Only what you actually need to act on","items":[
        {"title":"Multi-Channel Alerts","color":"warn","icon":"M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9","desc1":"CPU/Memory/Disk/Load/GPU/Offline threshold alerts; Feishu Webhook, DingTalk (HMAC), SMTP email channels.","val":"Alerts land in the collaboration tools you already use"},
        {"title":"Tiered & De-noised","color":"warn","icon":"M3 12h4l3-9 4 18 3-9h4","desc1":"Critical/warning tiers, event dedup cooldown (no repeat within 5 min), plus noise suppression — 80% less alert volume.","val":"Real incidents no longer buried in noise"},
        {"title":"Desktop Notifications","color":"ok","icon":"M9 3v2M15 3v2M5 7h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z","desc1":"Browser Notification API pop-ups + sound, so you're alerted even without the page open.","val":"Woken up even when you're away from the keyboard"},
        {"title":"Offline = Alert","color":"warn","icon":"M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z","desc1":"30s of no report from an agent triggers a critical offline alert — host loss in distributed setups never goes unnoticed.","val":"You know a machine died before your colleagues do"}
      ]},
      {"tag":"03","title":"Remote Access & Audit","desc":"Reachable without open ports, auditable end to end","items":[
        {"title":"Remote Terminal","color":"ok","icon":"M4 17l6-6-6-6M12 19h8","desc1":"Browser-to-host terminal via agent reverse connection — no inbound ports. Multi-tab, auto-resize, full VT100 (vim/top), mobile keyboard.","val":"No VPN + SSH — straight from the browser"},
        {"title":"Session Replay","color":"ok","icon":"M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15","desc1":"All sessions recorded (timestamped frames), 1x/2x/4x/8x playback; live observation for multiple viewers; searchable by operator/host/IP.","val":"Full audit trail — who did what, when"},
        {"title":"Port Forwarding (TCP/HTTP)","color":"ok","icon":"M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8M16 6l-4-4-4 4M12 2v13","desc1":"Map internal web apps, databases, and debug endpoints to your local browser via the agent's reverse tunnel — zero public exposure. TCP + HTTP, list + card views.","desc2":"Enable/disable/copy/edit/delete in one click; stats and health endpoints catch anomalies early.","val":"Internal services, accessible from your laptop in seconds"},
        {"title":"Operation Logs & Audit","color":"ok","icon":"M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z M14 2v6h6 M16 13H8M16 17H8M10 9H8","desc1":"Full operation logs (operation/system/plugin), filterable and CSV-exportable; together with session recording and command audit, a complete audit loop.","val":"Compliance audit materials, exported in one click"}
      ]},
      {"tag":"04","title":"Automation","desc":"Hand the repetitive work to playbooks","items":[
        {"title":"Automation Playbooks","color":"purple","icon":"M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z","desc1":"Visual command-sequence orchestration, one-click batch execution to multiple hosts; real-time results, success/failure stats, step-level output.","desc2":"Full execution history with operator, timing, and results.","val":"Patch 100 hosts in 10 minutes"}
      ]},
      {"tag":"05","title":"Security & Access Control","desc":"Collaboration that's still secure and compliant","items":[
        {"title":"Multi-User RBAC","color":"purple","icon":"M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 7a4 4 0 1 0 0 8 4 4 0 0 0 0-8z M23 21v-2a4 4 0 0 0-3-3.87","desc1":"Three-tier roles: admin (all), operator (terminal+alerts), viewer (read-only). Route-level enforcement + user management UI.","val":"Different people see different capability boundaries"},
        {"title":"MFA Two-Step","color":"purple","icon":"M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5","desc1":"TOTP two-step verification (RFC 6238, Google Authenticator compatible) for login and sensitive actions.","val":"Credential leaks still can't get in"},
        {"title":"Account Recovery","color":"purple","icon":"M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M22 6l-10 7L2 6","desc1":"Forgot username / forgot password (email code) / email unbind MFA — all with brute-force/enumeration protection.","val":"An admin leaving doesn't lock you out"},
        {"title":"Machine Fingerprint Auth","color":"purple","icon":"M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z M9 12l2 2 4-4","desc1":"machine-id + MAC hash fingerprint binding; terminal channel authenticates by fingerprint, not token. Token rotation doesn't break deployed agents.","val":"Token rotation never breaks deployed agents"},
        {"title":"Compliance Audit Loop","color":"purple","icon":"M9 2h6a2 2 0 0 1 2 2v0h3a2 2 0 0 1 2 2v13a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h3a2 2 0 0 1 2-2z M9 14l2 2 4-4","desc1":"Session replay + operation logs + MFA + RBAC together satisfy compliance requirements for traceability, control, and record-keeping.","val":"Compliance materials, out of the box"}
      ]},
      {"tag":"06","title":"Deployment & Architecture","desc":"Minimal to the extreme, elastic to production","items":[
        {"title":"Single Binary, Zero Deps","color":"accent","icon":"M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z","desc1":"Server is one Go binary (~15MB); agent is smaller. Memory is storage, gzip+JSON snapshot persistence — no MySQL/Redis/Kafka/TSDB.","val":"A single 1-core 1GB box does it all"},
        {"title":"Multi-Server Push","color":"accent","icon":"M12 2L2 7v10l10 5 10-5V7L12 2z","desc1":"One agent pushes to multiple servers simultaneously — collect once, broadcast to all; independent auth/retry per server.","val":"One agent, multiple dashboards"},
        {"title":"Gateway Relay Mode","color":"accent","icon":"M17 1l4 4-4 4 M3 11V9a4 4 0 0 1 4-4h14 M7 23l-4-4 4-4 M21 13v2a4 4 0 0 1-4 4H3","desc1":"One internet-connected machine proxies all requests to the cloud — binaries, reporting, and terminal auto-tunnel through. Ideal for cross-subnet or firewalled hosts.","val":"No need to expose every machine"},
        {"title":"PWA Offline Access","color":"accent","icon":"M12 18h.01M8 21h8a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2z M9 7h.01","desc1":"Installable to desktop (PWA), standalone window; App Shell offline cache shows last-known state even when disconnected.","val":"Install on your phone, monitor anywhere"},
        {"title":"Install Wizard","color":"accent","icon":"M12 3v12M7 10l5 5 5-5 M5 21h14","desc1":"One docker compose starts the server; install.sh auto-detects CPU arch (AMD64/ARM64) and downloads the matching agent binary — one curl to install.","val":"Even a beginner ships in 3 minutes"},
        {"title":"Embedded Persistence","color":"accent","icon":"M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z M17 21v-8H7v8 M7 3v5h8","desc1":"gzip+JSON snapshot persistence, no external DB; config migration and one-click Revert; 7-day token-rotation grace period.","val":"Data survives restarts; mistakes can be rolled back"},
        {"title":"Real-Time Push","color":"accent","icon":"M13 2L3 14h9l-1 8 10-12h-9l1-8z","desc1":"WebSocket real-time push with automatic polling fallback and seamless reconnect; gzip 8-10x compression cuts bandwidth.","val":"Dashboard data is always fresh"}
      ]},
      {"tag":"07","title":"Extensibility","desc":"You define the boundaries of monitoring","items":[
        {"title":"Python Plugin SDK","color":"purple","icon":"M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4","desc1":"Built-in plugin SDK — collect custom metrics in a few lines (MySQL connections, Nginx requests, Redis memory, etc.).","desc2":"Built-in examples: process monitor, service port probe, and more.","val":"Monitor what you want, not just built-ins"},
        {"title":"Lightweight AI Anomaly","color":"purple","icon":"M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5z","desc1":"Plugin ships z-score lightweight anomaly detection — spot metric spikes without a separate ML platform.","val":"Anomalies auto-flagged; no need to stare at charts"}
      ]}
    ],
    "cta.title": "All These Features in One Binary", "cta.desc": "No need for Prometheus + Grafana + Alertmanager + Bastion + Ticketing — AIOps Monitor does it all", "cta.btn2": "View Solutions"
  }
},

/* ---------- 产品对比页 ---------- */
"comparison": {
  "zh-CN": {
    "page.title": "产品对比 — AIOps Monitor",
    "page.desc": "与 Zabbix / Prometheus+Grafana / 商业 APM 全面对比：轻量级、零依赖、一键部署的差异化优势。",
    "page.oglocale": "zh_CN",
    "head.tag": "产品对比", "head.title": "为什么选择 AIOps Monitor？", "head.desc": "同样是监控运维，但部署成本、学习曲线和维护负担天差地别",
    "adv.tag": "核心优势", "adv.title": "中小企业选择 AIOps Monitor 的三个理由",
    "cta.title": "别再为监控工具的部署和维护买单了", "cta.desc": "把省下来的时间和预算，用在真正创造业务价值的事情上",
    "cta.btn1": "免费部署 →", "cta.btn2": "返回首页",
    "table": {
      "headers": ["能力维度","AIOps Monitor","Zabbix","Prometheus + Grafana","商业 APM"],
      "rows": [
        [["部署方式",""],["单二进制 + 一条命令","yes"],["Server + DB + Agent 多组件","no"],["Prometheus + Grafana + AlertManager","no"],["Agent + SaaS 账号","no"]],
        [["外部依赖",""],["零（仅 go-qrcode）","yes"],["MySQL / PostgreSQL","no"],["时序数据库（TSDB）","no"],["无（云端托管）",""]],
        [["部署时间",""],["3 分钟","yes"],["30-60 分钟","no"],["1-2 小时（含配置）","no"],["10-30 分钟","no"]],
        [["学习曲线",""],["低（开箱即用）","yes"],["中高（模板/触发器/Low-level discovery）","no"],["高（PromQL/YAML/Grafana 面板）","no"],["低-中",""]],
        [["远程终端",""],["内置（免开端口 + 会话录制回放 + 命令审计）","yes"],["需额外部署堡垒机","no"],["无","no"],["无","no"]],
        [["自动化运维",""],["内置剧本编排","yes"],["无（需 Ansible 等配合）","no"],["无","no"],["无","no"]],
        [["告警推送",""],["飞书/钉钉/邮件 + 桌面通知","yes"],["邮件/Webhook（需配置）",""],["AlertManager（需单独部署）","no"],["邮件/Slack/Webhook",""]],
        [["用户权限",""],["RBAC + MFA（内置）","yes"],["用户组（无 MFA）","no"],["无原生（需 Grafana 企业版）","no"],["有",""]],
        [["操作审计",""],["终端录制 + 回放 + 命令审计","yes"],["无终端审计","no"],["无","no"],["无终端审计","no"]],
        [["GPU 监控",""],["NVIDIA + AMD + Apple","yes"],["需自定义模板","no"],["需 DCGM Exporter","no"],["部分支持",""]],
        [["跨平台 Agent",""],["Linux/Win/macOS + ARM64","yes"],["Linux/Win/macOS",""],["Linux/Win（macOS 社区）","no"],["Linux/Win","no"]],
        [["PWA 移动端",""],["支持（可安装到手机桌面）","yes"],["仅 Web","no"],["仅 Web","no"],["有 App",""]],
        [["多服务端推送",""],["单 Agent 多服务端广播","yes"],["不支持","no"],["需 Remote Write","no"],["不支持","no"]],
        [["网关中继模式",""],["内置（跨网段穿透）","yes"],["需 Proxy/Agent 主动","no"],["需 Pushgateway","no"],["不支持","no"]],
        [["机器指纹鉴权",""],["machine-id + MAC 绑定","yes"],["PSK/Token","no"],["mTLS","no"],["Agent Key","no"]],
        [["gzip 压缩",""],["内置（8-10 倍压缩）","yes"],["需 Nginx 配置","no"],["需 Nginx 配置","no"],["有",""]],
        [["数据持久化",""],["内嵌 gzip+JSON（无 DB）","yes"],["MySQL / PostgreSQL","no"],["本地磁盘 TSDB","no"],["云端托管",""]],
        [["价格",""],["免费开源（MIT）","yes"],["免费开源（GPL）",""],["免费开源（Apache）",""],["按主机数收费","no"]],
        [["适合规模",""],["1-5000 台主机","yes"],["50-5000+ 台",""],["100-10000+ 台",""],["任意（按需付费）",""]]
      ]
    },
    "advantages": [
      {"title":"轻量到极致","color":"ok","icon":"M13 2L3 14h9l-1 8 10-12h-9l1-8z","desc":["服务端单个 Go 二进制（~15MB），Agent 更小。不需要 MySQL、Redis、Kafka、时序数据库 —— 内存即存储，gzip+JSON 快照持久化。","Go 原生编译，启动即就绪。1 核 1G 的云服务器就能跑。"],"value":"运维成本从“买服务器跑监控”变成“一台小机器全搞定”"},
      {"title":"部署零门槛","color":"accent","icon":"M5 13l4 4L19 7","desc":["一条 docker compose up 启动服务端，一条 curl | bash 安装 Agent。自动检测 CPU 架构（AMD64/ARM64），自动下载对应二进制。","不支持 Docker？直接下载二进制运行也行。Windows 用 NSSM 或任务计划自启，Linux 用 systemd，macOS 用 launchd。"],"value":"不需要 DevOps 工程师，运维小白也能部署"},
      {"title":"免费且开源","color":"purple","icon":"M12 1v22M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6","desc":["MIT 开源协议，无商业限制。代码托管在 GitHub，透明可信。无主机数限制、无功能阉割、无“企业版”套路。","Python 插件 SDK 自由扩展，几行代码接入自定义指标。社区贡献持续迭代。"],"value":"零授权费、零主机数限制、零功能锁定"}
    ]
  },
  "zh-TW": {
    "page.title": "產品對比 — AIOps Monitor",
    "page.desc": "與 Zabbix / Prometheus+Grafana / 商業 APM 全面對比：輕量級、零依賴、一鍵部署的差异化優勢。",
    "page.oglocale": "zh_TW",
    "head.tag": "產品對比", "head.title": "為什麼選擇 AIOps Monitor？", "head.desc": "同樣是監控運維，但部署成本、學習曲線和維護負擔天差地別",
    "adv.tag": "核心優勢", "adv.title": "中小企業選擇 AIOps Monitor 的三個理由",
    "cta.title": "別再為監控工具的部署和維護買單了", "cta.desc": "把省下來的時間和預算，用在真正創造業務價值的事情上",
    "cta.btn1": "免費部署 →", "cta.btn2": "返回首頁",
    "table": {
      "headers": ["能力維度","AIOps Monitor","Zabbix","Prometheus + Grafana","商業 APM"],
      "rows": [
        [["部署方式",""],["單二進制 + 一條命令","yes"],["Server + DB + Agent 多組件","no"],["Prometheus + Grafana + AlertManager","no"],["Agent + SaaS 帳號","no"]],
        [["外部依賴",""],["零（僅 go-qrcode）","yes"],["MySQL / PostgreSQL","no"],["時序資料庫（TSDB）","no"],["無（雲端託管）",""]],
        [["部署時間",""],["3 分鐘","yes"],["30-60 分鐘","no"],["1-2 小時（含配置）","no"],["10-30 分鐘","no"]],
        [["學習曲線",""],["低（開箱即用）","yes"],["中高（模板/觸發器/Low-level discovery）","no"],["高（PromQL/YAML/Grafana 面板）","no"],["低-中",""]],
        [["遠程終端",""],["內建（免開端口 + 會話錄製回放 + 命令審計）","yes"],["需額外部署堡壘機","no"],["無","no"],["無","no"]],
        [["自動化運維",""],["內建劇本編排","yes"],["無（需 Ansible 等配合）","no"],["無","no"],["無","no"]],
        [["告警推送",""],["飛書/釘釘/郵件 + 桌面通知","yes"],["郵件/Webhook（需配置）",""],["AlertManager（需單獨部署）","no"],["郵件/Slack/Webhook",""]],
        [["用戶權限",""],["RBAC + MFA（內建）","yes"],["用戶組（無 MFA）","no"],["無原生（需 Grafana 企業版）","no"],["有",""]],
        [["操作審計",""],["終端錄製 + 回放 + 命令審計","yes"],["無終端審計","no"],["無","no"],["無終端審計","no"]],
        [["GPU 監控",""],["NVIDIA + AMD + Apple","yes"],["需自定義模板","no"],["需 DCGM Exporter","no"],["部分支持",""]],
        [["跨平台 Agent",""],["Linux/Win/macOS + ARM64","yes"],["Linux/Win/macOS",""],["Linux/Win（macOS 社群）","no"],["Linux/Win","no"]],
        [["PWA 移動端",""],["支援（可安裝到手機桌面）","yes"],["僅 Web","no"],["僅 Web","no"],["有 App",""]],
        [["多服務端推送",""],["單 Agent 多服務端廣播","yes"],["不支援","no"],["需 Remote Write","no"],["不支援","no"]],
        [["網關中繼模式",""],["內建（跨網段穿透）","yes"],["需 Proxy/Agent 主動","no"],["需 Pushgateway","no"],["不支援","no"]],
        [["機器指紋鑑權",""],["machine-id + MAC 綁定","yes"],["PSK/Token","no"],["mTLS","no"],["Agent Key","no"]],
        [["gzip 壓縮",""],["內建（8-10 倍壓縮）","yes"],["需 Nginx 配置","no"],["需 Nginx 配置","no"],["有",""]],
        [["資料持久化",""],["內嵌 gzip+JSON（無 DB）","yes"],["MySQL / PostgreSQL","no"],["本地磁碟 TSDB","no"],["雲端託管",""]],
        [["價格",""],["免費開源（MIT）","yes"],["免費開源（GPL）",""],["免費開源（Apache）",""],["按主機數收費","no"]],
        [["適合規模",""],["1-5000 台主機","yes"],["50-5000+ 台",""],["100-10000+ 台",""],["任意（按需付費）",""]]
      ]
    },
    "advantages": [
      {"title":"輕量到極致","color":"ok","icon":"M13 2L3 14h9l-1 8 10-12h-9l1-8z","desc":["服務端單個 Go 二進制（~15MB），Agent 更小。不需要 MySQL、Redis、Kafka、時序資料庫 —— 記憶體即儲存，gzip+JSON 快照持久化。","Go 原生編譯，啟動即就緒。1 核 1G 的雲伺服器就能跑。"],"value":"運維成本從「買伺服器跑監控」變成「一台小機器全搞定」"},
      {"title":"部署零門檻","color":"accent","icon":"M5 13l4 4L19 7","desc":["一條 docker compose up 啟動服務端，一條 curl | bash 安裝 Agent。自動檢測 CPU 架構（AMD64/ARM64），自動下載對應二進制。","不支援 Docker？直接下載二進制運行也行。Windows 用 NSSM 或任務計畫自啟，Linux 用 systemd，macOS 用 launchd。"],"value":"不需要 DevOps 工程師，運維小白也能部署"},
      {"title":"免費且開源","color":"purple","icon":"M12 1v22M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6","desc":["MIT 開源協議，無商業限制。代碼託管在 GitHub，透明可信。無主機數限制、無功能閹割、無「企業版」套路。","Python 插件 SDK 自由擴展，幾行代碼接入自定義指標。社區貢獻持續迭代。"],"value":"零授權費、零主機數限制、零功能鎖定"}
    ]
  },
  "en": {
    "page.title": "Comparison — AIOps Monitor",
    "page.desc": "A full comparison with Zabbix / Prometheus+Grafana / commercial APM: the lightweight, zero-dependency, one-command-deploy difference.",
    "page.oglocale": "en_US",
    "head.tag": "Comparison", "head.title": "Why Choose AIOps Monitor?", "head.desc": "Same monitoring job — but deployment cost, learning curve, and maintenance burden are worlds apart",
    "adv.tag": "Core Advantages", "adv.title": "Three Reasons SMBs Choose AIOps Monitor",
    "cta.title": "Stop Paying for Monitoring Tooling and Maintenance", "cta.desc": "Spend the saved time and budget on what actually creates business value",
    "cta.btn1": "Deploy Free →", "cta.btn2": "Back to Home",
    "table": {
      "headers": ["Capability","AIOps Monitor","Zabbix","Prometheus + Grafana","Commercial APM"],
      "rows": [
        [["Deployment",""],["Single binary + one command","yes"],["Server + DB + Agent components","no"],["Prometheus + Grafana + AlertManager","no"],["Agent + SaaS account","no"]],
        [["Dependencies",""],["Zero (only go-qrcode)","yes"],["MySQL / PostgreSQL","no"],["TSDB","no"],["None (cloud-hosted)",""],],
        [["Deploy time",""],["3 minutes","yes"],["30-60 minutes","no"],["1-2 hours (config incl.)","no"],["10-30 minutes","no"]],
        [["Learning curve",""],["Low (works out of the box)","yes"],["Medium-High (templates/triggers/LLD)","no"],["High (PromQL/YAML/Grafana)","no"],["Low-Medium",""]],
        [["Remote terminal",""],["Built-in (port-free + session recording & replay + command audit)","yes"],["Needs separate bastion host","no"],["None","no"],["None","no"]],
        [["Automation",""],["Built-in playbook orchestration","yes"],["None (needs Ansible etc.)","no"],["None","no"],["None","no"]],
        [["Alert delivery",""],["Feishu/DingTalk/Email + desktop notifications","yes"],["Email/Webhook (needs config)",""],["AlertManager (separate deploy)","no"],["Email/Slack/Webhook",""]],
        [["User permissions",""],["RBAC + MFA (built-in)","yes"],["User groups (no MFA)","no"],["Not native (needs Grafana Enterprise)","no"],["Yes",""]],
        [["Operation audit",""],["Terminal recording + replay + command audit","yes"],["No terminal audit","no"],["None","no"],["No terminal audit","no"]],
        [["GPU monitoring",""],["NVIDIA + AMD + Apple","yes"],["Custom template required","no"],["Needs DCGM Exporter","no"],["Partial",""]],
        [["Cross-platform agent",""],["Linux/Win/macOS + ARM64","yes"],["Linux/Win/macOS",""],["Linux/Win (macOS community)","no"],["Linux/Win","no"]],
        [["PWA mobile",""],["Yes (installable to phone home screen)","yes"],["Web only","no"],["Web only","no"],["Has app",""]],
        [["Multi-server push",""],["Single agent broadcasts to multiple servers","yes"],["Not supported","no"],["Needs Remote Write","no"],["Not supported","no"]],
        [["Gateway relay",""],["Built-in (cross-subnet tunnel)","yes"],["Needs Proxy/active Agent","no"],["Needs Pushgateway","no"],["Not supported","no"]],
        [["Machine fingerprint auth",""],["machine-id + MAC binding","yes"],["PSK/Token","no"],["mTLS","no"],["Agent Key","no"]],
        [["gzip compression",""],["Built-in (8-10x compression)","yes"],["Needs Nginx config","no"],["Needs Nginx config","no"],["Yes",""]],
        [["Data persistence",""],["Embedded gzip+JSON (no DB)","yes"],["MySQL / PostgreSQL","no"],["Local disk TSDB","no"],["Cloud-hosted",""]],
        [["Pricing",""],["Free open source (MIT)","yes"],["Free open source (GPL)",""],["Free open source (Apache)",""],["Per-host pricing","no"]],
        [["Best for",""],["1-5000 hosts","yes"],["50-5000+ hosts",""],["100-10000+ hosts",""],["Any (pay as you go)",""]]
      ]
    },
    "advantages": [
      {"title":"Extreme Lightweight","color":"ok","icon":"M13 2L3 14h9l-1 8 10-12h-9l1-8z","desc":["Server is a single Go binary (~15MB); the agent is even smaller. No MySQL, Redis, Kafka, or TSDB — memory is storage, gzip+JSON snapshot persistence.","Native Go build, ready on launch. Runs on a 1-core 1GB cloud server."],"value":'Ops cost shifts from "servers just to run monitoring" to "one small box does it all"'},
      {"title":"Zero-Friction Deploy","color":"accent","icon":"M5 13l4 4L19 7","desc":["One docker compose up starts the server; one curl | bash installs the agent. Auto-detects CPU arch (AMD64/ARM64) and downloads the right binary.","No Docker? Just run the binary directly. Windows via NSSM or Task Scheduler, Linux via systemd, macOS via launchd."],"value":"No DevOps engineer needed — anyone can deploy"},
      {"title":"Free and Open Source","color":"purple","icon":"M12 1v22M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6","desc":["MIT licensed, no commercial restrictions. Code on GitHub, transparent and trustworthy. No host limits, no feature cuts, no enterprise-edition gimmicks.","Free Python plugin SDK — add custom metrics in a few lines. Community-driven iteration."],"value":'Zero license fees, zero host limits, zero lock-in'}
    ]
  }
},

/* ---------- 解决方案页 ---------- */
"solutions": {
  "zh-CN": {
    "page.title": "解决方案 — AIOps Monitor",
    "page.desc": "单机监控快速部署、多机房集中监控、团队协作运维、等保合规审计 — 四大真实场景解决方案。",
    "page.oglocale": "zh_CN",
    "head.tag": "解决方案", "head.title": "真实场景，真实价值", "head.desc": "从单机到多机房，从个人到团队，从日常运维到合规审计",
    "cta.title": "无论你的运维场景是什么", "cta.desc": "单机也好，多机房也罢，团队协作或合规审计 —— AIOps Monitor 都能 3 分钟搞定",
    "cta.btn1": "免费部署 →", "cta.btn2": "查看产品对比",
    "scenarios": [
      {"num":"场景 01","title":"单机监控快速部署","desc":"一台服务器跑业务，一个人管运维。没有专业监控团队，也没有预算买商业 APM。",
       "points":["一条 docker compose 命令启动监控服务端","一条 curl 命令在目标主机安装 Agent","3 分钟内完成全部部署，浏览器打开即用","无需 MySQL / Redis / Kafka，零外部依赖","飞书/钉钉 Webhook 配置后即收告警"],
       "visual":'<span style="color:var(--muted)"># 1. 启动服务端</span><br>git clone https://github.com/sreyun/aiops-monitor.git<br>cd aiops-monitor && docker compose up -d<br><br><span style="color:var(--muted)"># 2. 在目标主机安装 Agent</span><br><span style="color:var(--muted)"># Linux（自动检测 amd64/arm64）</span><br>curl -fsSL "http://server:8529/install.sh?token=XXX" | sudo sh<br><br><span style="color:var(--ok)">✓ 浏览器打开 http://localhost:8529</span><br><span style="color:var(--muted)">默认凭据 admin / admin</span>'},
      {"num":"场景 02","title":"多机房集中监控","desc":"多个机房、几十上百台机器，分散管理看不到全局。某台机器挂了半天才发现。",
       "points":["所有主机统一纳管到单一面板，按分类分组展示","Agent 支持多服务端广播，双活容灾不丢数据","网关中继模式：跨网段/防火墙后主机也能纳管","离线告警：主机 30 秒无上报即触发严重告警","概览页 KPI 卡片：在线/离线/严重告警/警告一目了然"],
       "visual":'<span style="color:var(--accent2)">[概览]</span> 15 台主机 · 14 在线 · 1 离线<br><span style="color:var(--ok)">●</span> web-01    CPU 23%  MEM 45%<br><span style="color:var(--ok)">●</span> web-02    CPU 18%  MEM 52%<br><span style="color:var(--ok)">●</span> db-master  CPU 67%  MEM 81%<br><span style="color:var(--crit)">●</span> db-slave   CPU  0%  MEM  0%<br><span style="color:var(--warn)">⚠</span> 告警: db-slave 已失联 120s<br><span style="color:var(--accent2)">[终端]</span> 点击 db-slave → 远程排查'},
      {"num":"场景 03","title":"团队协作运维","desc":"多人运维但没有堡垒机，谁登了哪台机器、做了什么操作，完全没有记录。出了问题互相甩锅。",
       "points":["三级 RBAC：管理员 / 操作员 / 观察员权限隔离","远程终端全程录制，支持倍速回放追溯","操作日志：谁在什么时候对哪台主机做了什么","实时旁观：多人同时查看同一终端会话","MFA 两步验证 + 暴力破解防护"],
       "visual":'<span style="color:var(--accent2)">[终端会话]</span><br>操作者: <span style="color:var(--ok)">zhangsan</span><br>主机: db-master (10.0.1.5)<br>时间: 14:23:08<br><span style="color:var(--muted)">─ 命令审计 ─</span><br>$ top<br>$ systemctl restart nginx<br>$ tail -f /var/log/error.log<br><span style="color:var(--ok)">✓ 会话已录制 · 可回放</span>'},
      {"num":"场景 04","title":"等保合规审计","desc":"等保测评要求操作可追溯、权限可管控、告警有记录。传统方式靠手动整理日志，费时费力。",
       "points":["全量操作日志：操作/系统/插件三类，支持筛选和 CSV 导出","终端会话录制 + 回放，满足操作可追溯要求","MFA + RBAC，满足访问控制要求","告警推送记录可查（飞书/钉钉/邮件）","内嵌持久化：重启不丢历史数据"],
       "visual":'<span style="color:var(--accent2)">[操作日志]</span><br>14:23  操作  zhangsan  打开远程终端<br>14:25  操作  zhangsan  终端命令: systemctl restart<br>14:30  系统  告警引擎  CPU 恢复正常<br>14:31  系统  通知引擎  飞书推送: 告警恢复<br>15:00  操作  lisi      执行剧本: 批量更新补丁<br>15:05  操作  lisi      剧本完成: 12/12 成功<br><span style="color:var(--muted)">─ 可导出 CSV ─</span>'}
    ]
  },
  "zh-TW": {
    "page.title": "解決方案 — AIOps Monitor",
    "page.desc": "單機監控快速部署、多機房集中監控、團隊協作運維、等保合規審計 — 四大真實場景解決方案。",
    "page.oglocale": "zh_TW",
    "head.tag": "解決方案", "head.title": "真實場景，真實價值", "head.desc": "從單機到多機房，從個人到團隊，從日常運維到合規審計",
    "cta.title": "無論你的運維場景是什麼", "cta.desc": "單機也好，多機房也罷，團隊協作或合規審計 —— AIOps Monitor 都能 3 分鐘搞定",
    "cta.btn1": "免費部署 →", "cta.btn2": "查看產品對比",
    "scenarios": [
      {"num":"場景 01","title":"單機監控快速部署","desc":"一台伺服器跑業務，一個人管運維。沒有專業監控團隊，也沒有預算買商業 APM。",
       "points":["一條 docker compose 命令啟動監控服務端","一條 curl 命令在目標主機安裝 Agent","3 分鐘內完成全部部署，瀏覽器打開即用","無需 MySQL / Redis / Kafka，零外部依賴","飛書/釘釘 Webhook 配置後即收告警"],
       "visual":'<span style="color:var(--muted)"># 1. 啟動服務端</span><br>git clone https://github.com/sreyun/aiops-monitor.git<br>cd aiops-monitor && docker compose up -d<br><br><span style="color:var(--muted)"># 2. 在目標主機安裝 Agent</span><br><span style="color:var(--muted)"># Linux（自動檢測 amd64/arm64）</span><br>curl -fsSL "http://server:8529/install.sh?token=XXX" | sudo sh<br><br><span style="color:var(--ok)">✓ 瀏覽器打開 http://localhost:8529</span><br><span style="color:var(--muted)">預設憑據 admin / admin</span>'},
      {"num":"場景 02","title":"多機房集中監控","desc":"多個機房、幾十上百台機器，分散管理看不到全局。某台機器掛了半天才發現。",
       "points":["所有主機統一納管到單一面板，按分類分組展示","Agent 支援多服務端廣播，雙活容災不丟數據","網關中繼模式：跨網段/防火牆後主機也能納管","離線告警：主機 30 秒無上報即觸發嚴重告警","概覽頁 KPI 卡片：在線/離線/嚴重告警/警告一目了然"],
       "visual":'<span style="color:var(--accent2)">[概覽]</span> 15 台主機 · 14 在線 · 1 離線<br><span style="color:var(--ok)">●</span> web-01    CPU 23%  MEM 45%<br><span style="color:var(--ok)">●</span> web-02    CPU 18%  MEM 52%<br><span style="color:var(--ok)">●</span> db-master  CPU 67%  MEM 81%<br><span style="color:var(--crit)">●</span> db-slave   CPU  0%  MEM  0%<br><span style="color:var(--warn)">⚠</span> 告警: db-slave 已失聯 120s<br><span style="color:var(--accent2)">[終端]</span> 點擊 db-slave → 遠程排查'},
      {"num":"場景 03","title":"團隊協作運維","desc":"多人運維但沒有堡壘機，誰登了哪台機器、做了什麼操作，完全沒有記錄。出了問題互相甩鍋。",
       "points":["三級 RBAC：管理員 / 操作員 / 觀察員權限隔離","遠程終端全程錄製，支援倍速回放追溯","操作日誌：誰在什麼時候對哪台主機做了什麼","即時旁觀：多人同時查看同一終端會話","MFA 兩步驗證 + 暴力破解防護"],
       "visual":'<span style="color:var(--accent2)">[終端會話]</span><br>操作者: <span style="color:var(--ok)">zhangsan</span><br>主機: db-master (10.0.1.5)<br>時間: 14:23:08<br><span style="color:var(--muted)">─ 命令審計 ─</span><br>$ top<br>$ systemctl restart nginx<br>$ tail -f /var/log/error.log<br><span style="color:var(--ok)">✓ 會話已錄製 · 可回放</span>'},
      {"num":"場景 04","title":"等保合規審計","desc":"等保測評要求操作可追溯、權限可管控、告警有記錄。傳統方式靠手動整理日誌，費時費力。",
       "points":["全量操作日誌：操作/系統/插件三類，支援篩選和 CSV 匯出","終端會話錄製 + 回放，滿足操作可追溯要求","MFA + RBAC，滿足存取控制要求","告警推送記錄可查（飛書/釘釘/郵件）","內嵌持久化：重啟不丟歷史數據"],
       "visual":'<span style="color:var(--accent2)">[操作日誌]</span><br>14:23  操作  zhangsan  打開遠程終端<br>14:25  操作  zhangsan  終端命令: systemctl restart<br>14:30  系統  告警引擎  CPU 恢復正常<br>14:31  系統  通知引擎  飛書推送: 告警恢復<br>15:00  操作  lisi      執行劇本: 批量更新補丁<br>15:05  操作  lisi      劇本完成: 12/12 成功<br><span style="color:var(--muted)">─ 可匯出 CSV ─</span>'}
    ]
  },
  "en": {
    "page.title": "Solutions — AIOps Monitor",
    "page.desc": "Single-host quick deploy, multi-DC centralized monitoring, team collaboration, compliance audit — four real-world scenarios.",
    "page.oglocale": "en_US",
    "head.tag": "Solutions", "head.title": "Real Scenarios, Real Value", "head.desc": "From single host to multi-DC, from solo to team, from daily ops to compliance audit",
    "cta.title": "Whatever Your Ops Scenario", "cta.desc": "Single host or multi-DC, team collaboration or compliance audit — AIOps Monitor gets it done in 3 minutes",
    "cta.btn1": "Deploy Free →", "cta.btn2": "View Comparison",
    "scenarios": [
      {"num":"Scenario 01","title":"Quick Single-Host Deploy","desc":"One server runs your business, one person handles ops. No dedicated monitoring team, no budget for commercial APM.",
       "points":["Start the server with one docker compose command","Install the agent on the target host with one curl command","Fully deployed in 3 minutes, open in the browser","No MySQL / Redis / Kafka — zero external dependencies","Receive alerts right after configuring Feishu/DingTalk webhooks"],
       "visual":'# 1. Start the server<br>git clone https://github.com/sreyun/aiops-monitor.git<br>cd aiops-monitor && docker compose up -d<br><br># 2. Install the agent on the target host<br># Linux (auto-detects amd64/arm64)<br>curl -fsSL "http://server:8529/install.sh?token=XXX" | sudo sh<br><br><span style="color:var(--ok)">✓ Open http://localhost:8529 in your browser</span><br><span style="color:var(--muted)">Default credentials admin / admin</span>'},
      {"num":"Scenario 02","title":"Centralized Multi-DC Monitoring","desc":"Multiple data centers, dozens to hundreds of hosts — fragmented management hides the big picture. A host goes down and you find out half an hour later.",
       "points":["All hosts unified in one dashboard, grouped by category","Agents broadcast to multiple servers — active-active DR, no data loss","Gateway relay: hosts behind firewalls/subnets still onboarded","Offline alert: 30s of no reporting triggers a critical alert","Overview KPI cards: online/offline/critical/warning at a glance"],
       "visual":'[Overview] 15 hosts · 14 online · 1 offline<br><span style="color:var(--ok)">●</span> web-01    CPU 23%  MEM 45%<br><span style="color:var(--ok)">●</span> web-02    CPU 18%  MEM 52%<br><span style="color:var(--ok)">●</span> db-master  CPU 67%  MEM 81%<br><span style="color:var(--crit)">●</span> db-slave   CPU  0%  MEM  0%<br><span style="color:var(--warn)">⚠</span> Alert: db-slave unreachable for 120s<br><span style="color:var(--accent2)">[Terminal]</span> click db-slave → remote troubleshoot'},
      {"num":"Scenario 03","title":"Team Collaboration Ops","desc":"Multi-admin ops without a bastion host — who logged into which machine and did what is completely unrecorded. When something breaks, everyone points fingers.",
       "points":["3-tier RBAC: admin / operator / viewer isolation","Full remote-terminal recording with speed-replay","Operation logs: who did what, when, on which host","Live observation: multiple people view the same session","MFA two-step + brute-force protection"],
       "visual":'[Terminal Session]<br>Operator: <span style="color:var(--ok)">zhangsan</span><br>Host: db-master (10.0.1.5)<br>Time: 14:23:08<br><span style="color:var(--muted)">─ Command Audit ─</span><br>$ top<br>$ systemctl restart nginx<br>$ tail -f /var/log/error.log<br><span style="color:var(--ok)">✓ Session recorded · replayable</span>'},
      {"num":"Scenario 04","title":"Compliance Audit","desc":"Compliance assessments require traceable operations, controllable permissions, and logged alerts. The traditional way — manually collating logs — is slow and painful.",
       "points":["Full operation logs: operation/system/plugin, filterable and CSV-exportable","Terminal session recording + replay meets traceability requirements","MFA + RBAC meet access-control requirements","Alert delivery logs auditable (Feishu/DingTalk/Email)","Embedded persistence: history survives restarts"],
       "visual":'[Operation Log]<br>14:23  op    zhangsan  opened remote terminal<br>14:25  op    zhangsan  terminal cmd: systemctl restart<br>14:30  sys   alert engine  CPU back to normal<br>14:31  sys   notify engine  Feishu push: alert recovered<br>15:00  op    lisi      ran playbook: batch patch update<br>15:05  op    lisi      playbook done: 12/12 succeeded<br><span style="color:var(--muted)">─ Exportable to CSV ─</span>'}
    ]
  }
},

/* ---------- 常见问题页 ---------- */
"faq": {
  "zh-CN": {
    "page.title": "常见问题 — AIOps Monitor",
    "page.desc": "关于 AIOps Monitor 的部署、安全、性能、扩展与端口转发，我们整理了最常见的疑问。",
    "page.oglocale": "zh_CN",
    "head.tag": "常见问题",
    "head.title": "关于 AIOps Monitor，你可能想问",
    "head.desc": "部署、安全、性能、扩展 —— 我们整理了最常见的疑问",
    "items": [
      {"q":"AIOps Monitor 免费吗？有功能限制吗？","a":"完全免费，采用 MIT 开源协议，无主机数限制、无功能阉割、无「企业版」套路。代码托管在 GitHub，透明可信。"},
      {"q":"需要额外安装数据库或中间件吗？","a":"不需要。服务端是单个 Go 二进制，数据以内嵌 gzip+JSON 快照持久化，内存即存储。没有 MySQL、Redis、Kafka 或时序数据库依赖。"},
      {"q":"支持哪些操作系统和架构？","a":"Agent 原生支持 Linux、Windows、macOS，覆盖 AMD64 与 ARM64。服务端为单一 Go 二进制，可在 1 核 1G 的小型云服务器上运行。"},
      {"q":"不开放端口，远程终端和端口转发怎么工作？","a":"Agent 主动反向连接服务端，所有通信（终端、转发、上报）都走这条已建立的隧道，主机无需开放任何入站端口，天然适配防火墙 / NAT 环境。"},
      {"q":"能监控多少台主机？性能如何？","a":"设计目标是 1–5000+ 台主机。采集 5 秒级、gzip 8–10 倍压缩、多级降采样，单台服务端资源占用极低，横向可通过多服务端推送扩展。"},
      {"q":"和 Zabbix / Prometheus 相比优势在哪？","a":"一个二进制同时内置监控、告警、远程终端、自动化剧本与审计，替代 5+ 工具栈；零外部依赖，3 分钟上线，学习曲线远低于 PromQL/YAML 体系。"},
      {"q":"端口转发（TCP/HTTP）是用来做什么的？","a":"把内网 Web 后台、数据库、调试接口通过反向隧道安全地映射到你的本地浏览器，无需在公网暴露服务。适合临时排查、内网联调与演示。"},
      {"q":"数据存在哪里？会上传到云端吗？","a":"全部数据落在你部署的服务端本地，自托管、不依赖任何云服务，也不会外传。适合对数据主权有要求的场景。"}
    ],
    "cta.title": "还有疑问？直接动手试试",
    "cta.desc": "3 分钟部署，所有功能开箱即用，不满意随时卸载，不留任何外部依赖。",
    "cta.btn1": "免费部署 →", "cta.btn2": "查看功能详情"
  },
  "zh-TW": {
    "page.title": "常見問題 — AIOps Monitor",
    "page.desc": "關於 AIOps Monitor 的部署、安全、效能、擴展與端口轉發，我們整理了最常見的疑問。",
    "page.oglocale": "zh_TW",
    "head.tag": "常見問題",
    "head.title": "關於 AIOps Monitor，你可能想問",
    "head.desc": "部署、安全、效能、擴展 —— 我們整理了最常見的疑問",
    "items": [
      {"q":"AIOps Monitor 免費嗎？有功能限制嗎？","a":"完全免費，採用 MIT 開源協議，無主機數限制、無功能閹割、無「企業版」套路。代碼託管在 GitHub，透明可信。"},
      {"q":"需要額外安裝資料庫或中間件嗎？","a":"不需要。服務端是單個 Go 二進制，數據以內嵌 gzip+JSON 快照持久化，記憶體即儲存。沒有 MySQL、Redis、Kafka 或時序資料庫依賴。"},
      {"q":"支援哪些作業系統和架構？","a":"Agent 原生支援 Linux、Windows、macOS，覆蓋 AMD64 與 ARM64。服務端為單一 Go 二進制，可在 1 核 1G 的小型雲伺服器上運行。"},
      {"q":"不開放端口，遠程終端和端口轉發怎麼工作？","a":"Agent 主動反向連接服務端，所有通信（終端、轉發、上報）都走這條已建立的隧道，主機無需開放任何入站端口，天然適配防火牆 / NAT 環境。"},
      {"q":"能監控多少台主機？效能如何？","a":"設計目標是 1–5000+ 台主機。採集 5 秒級、gzip 8–10 倍壓縮、多級降採樣，單台服務端資源占用極低，橫向可透過多服務端推送擴展。"},
      {"q":"和 Zabbix / Prometheus 相比優勢在哪？","a":"一個二進制同時內建監控、告警、遠程終端、自動化劇本與審計，替代 5+ 工具棧；零外部依賴，3 分鐘上線，學習曲線遠低於 PromQL/YAML 體系。"},
      {"q":"端口轉發（TCP/HTTP）是用來做什麼的？","a":"把內網 Web 後台、資料庫、除錯介面透過反向隧道安全地映射到你的本地瀏覽器，無需在公網暴露服務。適合臨時排查、內網聯調與示範。"},
      {"q":"數據存在哪裡？會上傳到雲端嗎？","a":"全部數據落在你部署的服務端本地，自託管、不依賴任何雲服務，也不會外傳。適合對數據主權有要求的場景。"}
    ],
    "cta.title": "還有疑問？直接動手試試",
    "cta.desc": "3 分鐘部署，所有功能開箱即用，不滿意隨時卸載，不留任何外部依賴。",
    "cta.btn1": "免費部署 →", "cta.btn2": "查看功能詳情"
  },
  "en": {
    "page.title": "FAQ — AIOps Monitor",
    "page.desc": "Common questions about AIOps Monitor: deployment, security, performance, extensibility, and port forwarding.",
    "page.oglocale": "en_US",
    "head.tag": "FAQ",
    "head.title": "Common Questions About AIOps Monitor",
    "head.desc": "Deployment, security, performance, extensibility — the questions we hear most",
    "items": [
      {"q":"Is AIOps Monitor free? Any limits?","a":"Completely free under the MIT license — no host limits, no feature cuts, no enterprise-edition gimmicks. Code is on GitHub, transparent and trustworthy."},
      {"q":"Do I need a database or middleware?","a":"No. The server is a single Go binary; data persists as embedded gzip+JSON snapshots — memory is storage. No MySQL, Redis, Kafka, or TSDB."},
      {"q":"Which OS and architectures are supported?","a":"Agents natively support Linux, Windows, macOS on both AMD64 and ARM64. The server is one Go binary that runs on a 1-core 1GB cloud instance."},
      {"q":"No open ports — how do terminal and forwarding work?","a":"The agent connects to the server reversely; all traffic (terminal, forwarding, reporting) flows over that established tunnel, so hosts need no inbound ports — ideal behind firewalls/NAT."},
      {"q":"How many hosts can it monitor? Performance?","a":"Designed for 1–5000+ hosts. 5-second collection, 8–10x gzip compression, multi-tier downsampling keep server overhead minimal; scale out via multi-server push."},
      {"q":"How is it better than Zabbix / Prometheus?","a":"One binary bundles monitoring, alerting, remote terminal, automation, and audit — replacing a 5+ tool stack. Zero external deps, 3-minute deploy, far lower learning curve than PromQL/YAML."},
      {"q":"What is port forwarding (TCP/HTTP) for?","a":"Securely map internal web backends, databases, and debug endpoints to your local browser via the reverse tunnel — no public exposure. Great for troubleshooting, internal testing, and demos."},
      {"q":"Where is my data? Does it leave my network?","a":"All data stays on your self-hosted server — no cloud dependency, nothing sent externally. Ideal when data sovereignty matters."}
    ],
    "cta.title": "Still Curious? Just Try It",
    "cta.desc": "Deploy in 3 minutes, every feature works out of the box, uninstall anytime with zero leftover dependencies.",
    "cta.btn1": "Deploy Free →", "cta.btn2": "View Features"
  }
},

/* ---------- 联系我们 ---------- */
"contact": {
  "zh-CN": {
    "page.title": "联系我们 — AIOps Monitor",
    "page.desc": "有合作、部署、定制或反馈需求？通过邮件或 GitHub 与 AIOps Monitor 团队取得联系。",
    "page.oglocale": "zh_CN",
    "head.tag": "联系我们",
    "head.title": "我们随时乐意为你提供帮助",
    "head.desc": "无论是部署咨询、功能建议、商务合作还是问题反馈，都欢迎与我们联系",
    "c.email.title": "电子邮件",
    "c.email.desc": "商务合作、部署咨询、定制开发与一般问题，邮件是最可靠的联系方式，我们通常在 1–2 个工作日内回复。",
    "c.email.btn": "发送邮件",
    "c.issue.title": "问题反馈",
    "c.issue.desc": "发现 Bug 或有明确的功能需求？在 GitHub Issues 提交，可追踪、可讨论，团队会公开跟进处理。",
    "c.issue.btn": "提交 Issue",
    "c.repo.title": "开源社区",
    "c.repo.desc": "关注项目动态、查阅文档、参与讨论或贡献代码，欢迎 Star 与 Fork，一起把产品做得更好。",
    "c.repo.btn": "访问仓库",
    "resp.title": "我们承诺",
    "resp.i1.t": "1–2 个工作日", "resp.i1.d": "邮件通常在两个工作日内回复",
    "resp.i2.t": "公开透明", "resp.i2.d": "Issues 与讨论全程公开可追踪",
    "resp.i3.t": "认真对待", "resp.i3.d": "每一条建议与反馈都会被评估",
    "cta.title": "准备好开始了吗？",
    "cta.desc": "3 分钟完成部署，所有功能开箱即用。有任何问题，随时给我们发邮件。",
    "cta.btn1": "免费部署 →", "cta.btn2": "查看功能详情"
  },
  "zh-TW": {
    "page.title": "聯絡我們 — AIOps Monitor",
    "page.desc": "有合作、部署、客製或回饋需求？透過電子郵件或 GitHub 與 AIOps Monitor 團隊取得聯繫。",
    "page.oglocale": "zh_TW",
    "head.tag": "聯絡我們",
    "head.title": "我們隨時樂意為你提供協助",
    "head.desc": "無論是部署諮詢、功能建議、商務合作還是問題回饋，都歡迎與我們聯繫",
    "c.email.title": "電子郵件",
    "c.email.desc": "商務合作、部署諮詢、客製開發與一般問題，電子郵件是最可靠的聯繫方式，我們通常在 1–2 個工作日內回覆。",
    "c.email.btn": "發送郵件",
    "c.issue.title": "問題回饋",
    "c.issue.desc": "發現 Bug 或有明確的功能需求？在 GitHub Issues 提交，可追蹤、可討論，團隊會公開跟進處理。",
    "c.issue.btn": "提交 Issue",
    "c.repo.title": "開源社群",
    "c.repo.desc": "關注專案動態、查閱文檔、參與討論或貢獻代碼，歡迎 Star 與 Fork，一起把產品做得更好。",
    "c.repo.btn": "造訪倉庫",
    "resp.title": "我們的承諾",
    "resp.i1.t": "1–2 個工作日", "resp.i1.d": "電子郵件通常在兩個工作日內回覆",
    "resp.i2.t": "公開透明", "resp.i2.d": "Issues 與討論全程公開可追蹤",
    "resp.i3.t": "認真對待", "resp.i3.d": "每一條建議與回饋都會被評估",
    "cta.title": "準備好開始了嗎？",
    "cta.desc": "3 分鐘完成部署，所有功能開箱即用。有任何問題，隨時給我們發郵件。",
    "cta.btn1": "免費部署 →", "cta.btn2": "查看功能詳情"
  },
  "en": {
    "page.title": "Contact — AIOps Monitor",
    "page.desc": "Partnership, deployment, customization, or feedback? Reach the AIOps Monitor team by email or GitHub.",
    "page.oglocale": "en_US",
    "head.tag": "Contact",
    "head.title": "We're Always Happy to Help",
    "head.desc": "Deployment questions, feature ideas, business partnerships, or feedback — we'd love to hear from you",
    "c.email.title": "Email",
    "c.email.desc": "For partnerships, deployment questions, custom development, and general inquiries, email is the most reliable way to reach us. We typically reply within 1–2 business days.",
    "c.email.btn": "Send Email",
    "c.issue.title": "Report an Issue",
    "c.issue.desc": "Found a bug or have a concrete feature request? File it on GitHub Issues — trackable, discussable, and publicly followed up by the team.",
    "c.issue.btn": "Open an Issue",
    "c.repo.title": "Open Source Community",
    "c.repo.desc": "Follow the project, read the docs, join discussions, or contribute code. Star and fork us — let's make it better together.",
    "c.repo.btn": "Visit Repo",
    "resp.title": "Our Commitment",
    "resp.i1.t": "1–2 Business Days", "resp.i1.d": "Emails are usually answered within two business days",
    "resp.i2.t": "Open & Transparent", "resp.i2.d": "Issues and discussions are public and trackable",
    "resp.i3.t": "Taken Seriously", "resp.i3.d": "Every suggestion and report gets evaluated",
    "cta.title": "Ready to Get Started?",
    "cta.desc": "Deploy in 3 minutes with every feature out of the box. Got questions? Just drop us an email.",
    "cta.btn1": "Deploy Free →", "cta.btn2": "View Features"
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

/* HTML 转义 */
function esc(s) {
  return String(s == null ? "" : s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

/* 渲染对比表格 + 优势卡片 */
function renderComparison(d) {
  var tbl = document.getElementById("cmpTable");
  if (tbl && d && d.table) {
    var h = d.table.headers, rows = d.table.rows;
    var thead = "<thead><tr>" + h.map(function(hh, i) {
      return "<th" + (i === 1 ? ' class="col-highlight"' : "") + ">" + esc(hh) + "</th>";
    }).join("") + "</tr></thead>";
    var tbody = "<tbody>" + rows.map(function(r) {
      return "<tr>" + r.map(function(cell, i) {
        var cls = cell[1];
        if (i === 0) return '<td class="feat-name">' + esc(cell[0]) + "</td>";
        var c = cls ? ' class="' + cls + '"' : "";
        return "<td" + c + ">" + esc(cell[0]) + "</td>";
      }).join("") + "</tr>";
    }).join("") + "</tbody>";
    tbl.innerHTML = thead + tbody;
  }
  var adv = document.getElementById("cmpAdvantages");
  if (adv && d && d.advantages) {
    var softMap = { ok: "var(--ok-soft)", accent: "var(--accent-soft)", purple: "var(--purple-soft)" };
    var borderMap = { ok: "rgba(16,185,129,.2)", accent: "rgba(59,130,246,.2)", purple: "rgba(139,92,246,.2)" };
    adv.innerHTML = d.advantages.map(function(a) {
      var bg = softMap[a.color] || "var(--accent-soft)";
      var bd = borderMap[a.color] || "rgba(59,130,246,.2)";
      var descs = a.desc.map(function(p) { return '<p class="desc">' + esc(p) + "</p>"; }).join("");
      return '<div class="feature-card">'
        + '<div class="glow"></div>'
        + '<div class="feature-icon" style="background:' + bg + ';border-color:' + bd + '">'
        + '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="' + a.icon + '"/></svg></div>'
        + "<h3>" + esc(a.title) + "</h3>"
        + descs
        + '<div class="value"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>' + esc(a.value) + "</div>"
        + "</div>";
    }).join("");
  }
}

/* 渲染解决方案场景 */
function renderSolutions(d) {
  var list = document.getElementById("scnList");
  if (list && d && d.scenarios) {
    list.innerHTML = d.scenarios.map(function(s, i) {
      var reverse = (i % 2 === 1);
      var text = '<div><div class="scenario-num">' + esc(s.num) + "</div>"
        + "<h3>" + esc(s.title) + "</h3>"
        + "<p>" + esc(s.desc) + "</p>"
        + "<ul>" + s.points.map(function(p) { return "<li>" + esc(p) + "</li>"; }).join("") + "</ul></div>";
      var vis = '<div class="scenario-visual"><div class="mockup">' + s.visual + "</div></div>";
      return '<div class="scenario">' + (reverse ? vis + text : text + vis) + "</div>";
    }).join("");
  }
}

/* 渲染功能分组（features 页） */
function renderFeatures(d) {
  var wrap = document.getElementById("featGroups");
  if (!wrap || !d || !d.groups) return;
  var softMap = { ok: "var(--ok-soft)", accent: "var(--accent-soft)", warn: "var(--warn-soft)", purple: "var(--purple-soft)" };
  var borderMap = { ok: "rgba(16,185,129,.25)", accent: "rgba(59,130,246,.25)", warn: "rgba(245,158,11,.25)", purple: "rgba(139,92,246,.25)" };
  wrap.innerHTML = d.groups.map(function(g) {
    var cards = g.items.map(function(it) {
      var bg = softMap[it.color] || "var(--accent-soft)";
      var bd = borderMap[it.color] || "rgba(59,130,246,.25)";
      var descs = [it.desc1, it.desc2].filter(function(x) { return x; })
        .map(function(p) { return '<p class="desc">' + esc(p) + "</p>"; }).join("");
      return '<div class="feature-card reveal">'
        + '<div class="glow"></div>'
        + '<div class="feature-icon" style="background:' + bg + ';border-color:' + bd + '">'
        + '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="' + it.icon + '"/></svg></div>'
        + "<h3>" + esc(it.title) + "</h3>"
        + descs
        + '<div class="value"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>' + esc(it.val) + "</div>"
        + "</div>";
    }).join("");
    return '<div class="feat-group reveal">'
      + '<div class="feat-group-head">'
      + '<span class="feat-group-tag">' + esc(g.tag) + "</span>"
      + "<div><h3>" + esc(g.title) + "</h3><p>" + esc(g.desc) + "</p></div>"
      + "</div>"
      + '<div class="feature-grid">' + cards + "</div>"
      + "</div>";
  }).join("");
}

/* 渲染常见问题手风琴（faq 页） */
function renderFaq(d) {
  var list = document.getElementById("faqList");
  if (!list || !d || !d.items) return;
  list.innerHTML = d.items.map(function(it) {
    return '<div class="faq-item reveal">'
      + '<button class="faq-q" type="button" aria-expanded="false">'
      + "<span>" + esc(it.q) + "</span>"
      + '<svg class="faq-chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>'
      + "</button>"
      + '<div class="faq-a"><p>' + esc(it.a) + "</p></div>"
      + "</div>";
  }).join("");
  list.querySelectorAll(".faq-q").forEach(function(btn) {
    btn.addEventListener("click", function() {
      var item = btn.parentElement;
      var open = item.classList.toggle("open");
      btn.setAttribute("aria-expanded", open ? "true" : "false");
    });
  });
}

/* 动态内容渲染（对比表 / 场景 / 功能分组 / FAQ） */
function renderDynamic() {
  var page = getPageName();
  var dict = (T[page] && T[page][CURRENT_LANG]) || null;
  if (!dict) return;
  if (page === "comparison") renderComparison(dict);
  else if (page === "solutions") renderSolutions(dict);
  else if (page === "features") renderFeatures(dict);
  else if (page === "faq") renderFaq(dict);
}

/* 应用所有翻译 */
function applyTranslations() {
  /* 更新 <html lang> */
  document.documentElement.lang = CURRENT_LANG;

  /* 更新 <title> */
  var titleEl = document.querySelector("title");
  if (titleEl && titleEl.hasAttribute("data-i18n")) {
    titleEl.textContent = t(titleEl.getAttribute("data-i18n"));
  }

  /* 更新所有带 data-i18n 的 meta（含 description / og:* / twitter:*） */
  document.querySelectorAll("meta[data-i18n]").forEach(function(el) {
    var val = t(el.getAttribute("data-i18n"));
    if (val) el.setAttribute("content", val);
  });

  /* 更新所有 data-i18n 元素 */
  document.querySelectorAll("[data-i18n]").forEach(function(el) {
    var key = el.getAttribute("data-i18n");
    var val = t(key);
    if (val && el.tagName.toLowerCase() !== "meta") el.textContent = val;
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

  /* 渲染动态内容（对比表 / 场景 / 功能分组 / FAQ） */
  renderDynamic();

  /* 通知交互脚本重新观察动态注入的渐入元素 */
  try { window.dispatchEvent(new Event("reveal:refresh")); } catch (e) {}

  /* 更新 hreflang 标签 */
  updateHreflang();

  /* 更新语言切换器当前选项 */
  var switcher = document.getElementById("langSelect");
  if (switcher) switcher.value = CURRENT_LANG;

  /* 通知页面：语言已变更 */
  try {
    document.dispatchEvent(new CustomEvent("lang:changed", { detail: { lang: CURRENT_LANG } }));
  } catch (e) {}

  /* 同步 JSON-LD 结构化数据的描述语言 */
  try {
    var ld = document.getElementById("ldjsonApp");
    if (ld && ld.textContent) {
      var obj = JSON.parse(ld.textContent);
      obj.description = t("page.desc");
      ld.textContent = JSON.stringify(obj);
    }
  } catch (e) {}
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

/* 对外暴露 API（供其他脚本使用） */
window.AIOpsI18n = {
  t: t,
  getLang: function () { return CURRENT_LANG; },
  setLang: setLang,
  onLangChange: function (cb) { document.addEventListener("lang:changed", cb); }
};

})();
