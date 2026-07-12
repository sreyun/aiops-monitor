# AIOps Monitor · UI/UX 深度审查报告

> 审查视角：资深 UI/UX 设计师 / 前端体验工程师
> 审查范围：当前项目全部前端界面，含两条独立产品线
> - **营销站** `website/`（6 页 HTML + `css/style.css` + `js/i18n.js` + `js/main.js`）
> - **管理面板** `cmd/server/web/`（`index.html` + `style.css` + `app.js`，即用户日常使用的真实产品）
> 审查方法：逐文件实地阅读 HTML/CSS/JS，结合交互模式、可访问性规范（WCAG 2.1 AA）、响应式断点核验。
> 说明：本次**仅产出审查结论，未改动任何源码**。部分对比度数值为按 sRGB 估算（环境 Bash 不可用，未能跑自动对比度工具），建议落地前用浏览器 DevTools 实测复核。

---

## 0. 总体结论

| 产品线 | 成熟度 | 主要问题域 |
|---|---|---|
| 管理面板 `cmd/server/web/` | **高**（已具备焦点陷阱、ESC、focus 管理、明暗双主题、对比度专项优化） | 跨产品视觉语言割裂、窄屏筛选区、textarea 焦点环、超宽屏留白 |
| 营销站 `website/` | **中**（视觉风格统一但较"模板化"） | 无 JS 即空白、缺失焦点环、reveal 无兜底、响应式临界、对比度、信息层级 |

**最关键的三个系统性问题**：① 营销站与面板是**两套完全独立的设计系统**（主色、圆角、字号、阴影语言全不同），品牌触点割裂；② 营销站正文**强依赖 JS 渲染**，无 JS/被拦截/SEO 抓取时四页空白；③ 营销站**缺少全局焦点环与 reduced-motion 降级**，可访问性与韧性不足。

---

## 1. 视觉一致性

### 1.1 [高] 两套独立设计系统，跨触点品牌割裂
- **证据**：
  - 营销站 `website/css/style.css:10` `--accent:#3b82f6`；面板 `cmd/server/web/style.css:21` `--accent:#4c8dff`。
  - 圆角：营销站 `--r:12px / --r-lg:16px / --r-xl:24px`；面板 `--r:10px / --r-lg:14px / --r-xl:18px`。
  - 字号：营销站 `body{font-size:15px}`；面板 `body{font-size:13px}`。
  - Logo 渐变、阴影语言（`shadow-lg` 营销 48px 模糊 vs 面板 32px）、间距栅格均不一致。
- **影响**：用户从官网跳转到产品后，产生明显的"换了个产品"的割裂感，削弱专业度与信任。
- **修复**：抽取共享 `aiops-design-tokens.css`（统一主色阶梯、中性色阶、圆角阶梯、阴影、间距 4px 栅格、字体栈），两端 `@import` 引用；差异仅保留在"营销氛围"与"工具密度"两个层级变量上。

### 1.2 [中] 营销站功能图标/分组视觉单调
- **证据**：`features.html` 7 大分组、29 项能力，图标统一用 `var(--accent2)` 实色；`--warn`/`--purple` 等语义色在功能卡中几乎未使用（`style.css` 已定义但未消费）。
- **影响**：全站视觉几乎只有"蓝 + 绿"，模块间难以通过色彩区分，长页阅读疲劳。
- **修复**：按分组赋予差异化的强调色（如监控=蓝、告警=琥珀、安全=紫、自动化=青），呼应面板已有的丰富语义色体系。

### 1.3 [低] 营销站内联样式绕过 token 体系
- **证据**：`index.html:81-83` hero 凭据提示、`:305-310` CTA 命令块均使用内联 `style="...color:var(--muted)"` 硬编码。
- **影响**：未来换肤/主题化时这些片段易被遗漏，产生色彩漂移。
- **修复**：抽为语义类（如 `.hero-creds`、`.code-block`），统一由 token 驱动。

---

## 2. 交互体验

### 2.1 [高] 营销站"无 JS 即空白"（SEO + 韧性）
- **证据**：`features.html:49`、`:` `comparison.html:50`（`<table id="cmpTable">` 空）、`solutions.html:49`（`<div id="scnList">` 空）、`faq.html:49`（`<div id="faqList">` 空）——四页正文完全由 `i18n.js` 动态注入。
- **影响**：
  - 搜索引擎抓取到的是空容器，SEO 几乎为零（对开源项目的曝光是硬伤）。
  - 用户网络拦截 JS、或 `i18n.js` 出错时，整页无内容。
- **修复（按推荐度）**：
  1. **静态回退**：HTML 内直接写死中文文案作为默认内容，`i18n.js` 仅做覆盖；这样无 JS 也有完整可读内容。
  2. **`<noscript>` 提示**：至少加一块"请启用 JavaScript"提示，避免纯白屏。
  3. **预渲染/SSG**：若后续引入构建，对营销站做静态预渲染。

### 2.2 [中] 锚点跳转被固定导航遮挡
- **证据**：`index.html:79` "了解痛点" → `#pain-points`；但营销站 `style.css` 中**无任何 `scroll-margin-top`**（grep 确认），而固定 navbar 高约 68px。
- **影响**：点击后目标标题被导航栏盖住上半部分，首次观感突兀。
- **修复**：`section[id]{scroll-margin-top:84px}`（营销站目前完全没有该规则，面板侧已有）。

### 2.3 [中] 移动端菜单无完整可访问交互
- **证据**：`index.html:56` `nav-toggle` 仅 toggle `.open` 类；无 `aria-expanded`/`aria-controls`、无 ESC 关闭、无点击外部关闭、无焦点管理。
- **影响**：键盘/读屏用户无法感知菜单开关状态；手机上展开后只能再点一次汉堡收起。
- **修复**：补充 `aria-expanded`、`aria-controls`；点击导航项/遮罩/按 ESC 自动收起；关闭时焦点回流到触发按钮。

### 2.4 [低] 面板自动刷新缺加载态
- **证据**：`app.js` 存在 `pauseBtn`（暂停自动刷新），说明列表/图表靠轮询；但未见"刷新中"骨架/节流指示，`topbar` 仅有 pulse 心跳。
- **影响**：数据量大或网络抖动时，列表闪动、用户不确定是否在更新。
- **修复**：轮询间隙显示低频骨架/淡入；或引入乐观更新。

---

## 3. 信息层级

### 3.1 [中] Hero 中安全提示被弱化处理
- **证据**：`index.html:81-83` 默认凭据 `admin/admin` 提示用 `color:var(--muted)` 灰字小号，混在 Hero 统计上方。
- **影响**：安全关键信息层级过低，用户易忽略"首次登录必须改密"，埋下弱口令隐患（与代码审计中"默认 admin/admin 无强改密"问题叠加）。
- **修复**：用更醒目的警示样式（如 `warn-soft` 底 + 图标），或弱化为页脚说明、首屏仅保留"3 分钟部署"。

### 3.2 [中] 功能详情页超长，缺乏导航辅助
- **证据**：`features.html` 7 分组 `gap:80px`，整页约 2000px+，无目录/侧栏锚点/返回顶部。
- **影响**：用户难快速定位"告警"或"安全"模块，长滚动易迷失。
- **修复**：加 sticky 分组快速导航（右侧锚点浮标）或"返回顶部"按钮；分组标题可点击锚定。

### 3.3 [中] 对比表移动端可读性差
- **证据**：`style.css:205` `.compare-table{min-width:700px}` 横向滚动，但无 sticky 首列/表头，无"本产品"高亮文字说明。
- **影响**：手机上左右滑动时丢失行列对应关系，难以判断哪列是 AIOps。
- **修复**：`position:sticky` 首列 + 表头；或在移动端改为"逐产品卡片对比"布局。

### 3.4 [低] 面板概览页区块标题风格不一致
- **证据**：同页并存 `.ov-section-header`（统计与健康）与 `.section-title`（最近告警）两种标题样式。
- **影响**：轻微视觉噪音。
- **修复**：统一为一种区块标题组件。

---

## 4. 响应式适配

### 4.1 [中] 营销站功能卡最小宽度临界溢出
- **证据**：`style.css:152` `.feature-grid{grid-template-columns:repeat(auto-fit,minmax(320px,1fr))}`；`@media(max-width:600px)` 下 `section` 左右 padding 仅 16px。360px 视口可用宽 = 360−32 = 328px，单卡 320px 勉强，但若存在 24px padding 场景即溢出；320px 宽老安卓机直接溢出。
- **影响**：窄屏出现横向滚动条。
- **修复**：`minmax(min(100%,300px),1fr)` 或降到 `280px`。

### 4.2 [中] 面板窄屏筛选区失控
- **证据**：`index.html` 多个视图（告警/日志/检查）的 `section-title` 内嵌多个 `<select>` + 搜索框 + 按钮，`topbar` 已 `flex-wrap`，但筛选区本身无抽屉/折叠；`@media(max-width:900px)` 仅隐藏安装按钮文字。
- **影响**：<768px 时筛选控件换行堆叠，顶栏高度暴涨，挤压内容。
- **修复**：窄屏将筛选条件收进"筛选"抽屉/折叠面板。

### 4.3 [低] 面板超宽屏留白偏多
- **证据**：`style.css:331` `.content{max-width:1280px}`（可切 wide 1720）。
- **影响**：4K 下默认居中留白大（可接受，已提供 wide 切换）。
- **修复**：默认上限提到 1440，或按视口动态。

### 4.4 [低] 营销站 arch/contact 断点良好
- **证据**：`style.css:305-309`（arch 900 单列）、`:384-387`（contact 700 单列）处理得当，未见问题。

---

## 5. 可访问性（WCAG 2.1 AA）

### 5.1 [高] 营销站缺失全局焦点环
- **证据**：grep `website/css/style.css` 中**无 `:focus-visible` 规则**；`.btn-primary`/`.nav-cta`/`.feature-card`/`.lang-toggle`/`.faq-q` 均无可见焦点样式。
- **影响**：纯键盘用户（及读屏+键盘组合）无法分辨当前焦点位置，完全不可操作导航与卡片。
- **修复**：统一加
  ```css
  a:focus-visible, button:focus-visible, .feature-card:focus-visible, .lang-toggle:focus-visible {
    outline:2px solid var(--accent); outline-offset:2px;
  }
  ```
  面板侧 `.btn:focus-visible` 已有良好范式（`:130`），可直接借鉴。

### 5.2 [高] 营销站 `.reveal{opacity:0}` 无 JS 兜底
- **证据**：`style.css:249-250` `.reveal{opacity:0;transform:translateY(30px)}`，仅 `.visible` 恢复；无 `.js` 门控。
- **影响**：一旦 `main.js` 任何异常，`IntersectionObserver` 不执行，**全站所有 `.reveal` 内容永久不可见**（含 section 标题、功能卡、CTA）。
- **修复**：
  ```css
  .js .reveal{opacity:0;transform:translateY(30px)}
  .reveal.visible{opacity:1;transform:none}
  ```
  `main.js` 顶部 `document.documentElement.classList.add('js')`；并 `try/catch` 包裹观察器，失败则直接给所有 `.reveal` 加 `.visible`。

### 5.3 [中] 营销站 nav 缺 aria 属性
- **证据**：`index.html:41` `<nav class="navbar">` 无 `aria-label`；`:56` `nav-toggle` 无 `aria-expanded`/`aria-controls`。（FAQ 按钮 `aria-expanded` ✓ 已处理，见 `i18n.js:921`。）
- **修复**：补 `aria-label="主导航"` 与按钮状态属性（见 2.3）。

### 5.4 [中] 面板 textarea 焦点环缺失
- **证据**：`style.css:120-128` 全局 `input,button,select,textarea{outline:none!important}`，但恢复焦点环的规则只覆盖 `.field input:focus` 与 `select:focus`，**不含 `textarea`**（安装命令多服务端列表、Webhook 模板、剧本步骤等）。
- **影响**：键盘用户在 textarea 中无焦点指示。
- **修复**：`.field textarea:focus, textarea:focus{box-shadow:0 0 0 3px var(--accent-soft)}`。

### 5.5 [中] 营销站对比度未达标（估算）
- **证据**：`--muted2:#5a6588`（footer-bottom、hero-stat label）置于 `--bg:#0a0e1a`（约 3.6:1，低于 AA 4.5:1）。面板侧已专门把 `muted2` 提亮到 `#6b7588` 并注释"原 3.9:1 不达标"（`:18`）。
- **修复**：营销站同步提亮 `--muted`/`--muted2` 至面板同级亮度。

### 5.6 [低] 面板模态可补 role/aria-modal
- **证据**：模态依赖 `trapFocus`+`data-close`（`app.js:267-295`），但未显式 `role="dialog" aria-modal="true"`。
- **修复**：给 `.mask > .modal` 加语义属性，强化读屏支持。

---

## 6. 性能体验

### 6.1 [中] 营销站未尊重 `prefers-reduced-motion`
- **证据**：grep 营销站 `style.css` **无 `prefers-reduced-motion`**；`float`/`pulse`/数字滚动/`reveal` 动画常驻。
- **影响**：前庭功能敏感/ prefers-reduced-motion 用户会被持续动画困扰（合规性与包容性扣分）。
- **修复**：
  ```css
  @media (prefers-reduced-motion: reduce){
    *,*::before,*::after{animation:none!important;transition:none!important}
    .reveal{opacity:1!important;transform:none!important}
  }
  ```

### 6.2 [中] 营销站首屏可见时间偏晚
- **证据**：`index.html:348-349` 两个 `<script>` 无 `defer`，且正文（含 Hero 文案经 `data-i18n` 由 JS 填充）依赖 JS 执行后才出现；无骨架/loading 态。
- **影响**：TTI 偏晚，弱网白屏感明显。
- **修复**：脚本加 `defer`；关键首屏文案静态化（与 2.1 同方案）；考虑内联首屏关键 CSS。

### 6.3 [低] OG 社交分享图缺失
- **证据**：`index.html:22-24` OG 图被注释占位，无 `og:image`。
- **影响**：分享到社交平台无预览图，传播体验缺失（非性能，属体验）。
- **修复**：生成 1200×630 `assets/og-cover.png` 并取消注释。

### 6.4 [低] 面板终端交互扎实
- **证据**：`app.js` 终端已有 `beforeinput` 兜底（`:1988`）、输入焦点重定向（`:2011`）、`autocomplete` 规范——性能与兼容性处理较完善，无显著问题。

---

## 7. 优先级汇总

| 优先级 | 编号 | 问题 | 产品线 |
|---|---|---|---|
| 🔴 高 | 1.1 | 两套独立设计系统，品牌割裂 | 全局 |
| 🔴 高 | 2.1 | 营销站四页无 JS 即空白（SEO/韧性） | 营销站 |
| 🔴 高 | 5.1 | 营销站缺失全局焦点环 | 营销站 |
| 🔴 高 | 5.2 | `.reveal` 无 JS 兜底，内容可能永久不可见 | 营销站 |
| 🟠 中 | 2.2 | 锚点被固定导航遮挡（缺 scroll-margin） | 营销站 |
| 🟠 中 | 2.3 | 移动端菜单无 aria/ESC/遮罩关闭 | 营销站 |
| 🟠 中 | 3.1 | Hero 安全提示层级过低 | 营销站 |
| 🟠 中 | 3.2 | 功能页超长缺导航辅助 | 营销站 |
| 🟠 中 | 3.3 | 对比表移动端无 sticky | 营销站 |
| 🟠 中 | 4.1 | 功能卡 minmax(320px) 临界溢出 | 营销站 |
| 🟠 中 | 4.2 | 面板窄屏筛选区失控 | 面板 |
| 🟠 中 | 5.3 | nav 缺 aria-label / 按钮状态属性 | 营销站 |
| 🟠 中 | 5.4 | 面板 textarea 焦点环缺失 | 面板 |
| 🟠 中 | 5.5 | 营销站 muted2 对比度不达标（估算） | 营销站 |
| 🟠 中 | 6.1 | 未尊重 prefers-reduced-motion | 营销站 |
| 🟠 中 | 6.2 | 首屏脚本无 defer、无静态回退 | 营销站 |
| 🟢 低 | 1.2 | 功能图标色彩单调 | 营销站 |
| 🟢 低 | 1.3 | 内联样式绕过 token | 营销站 |
| 🟢 低 | 3.4 | 面板区块标题样式不一致 | 面板 |
| 🟢 低 | 4.3 | 面板超宽屏留白 | 面板 |
| 🟢 低 | 5.6 | 面板模态缺 role/aria-modal | 面板 |
| 🟢 低 | 6.3 | OG 分享图缺失 | 营销站 |
| 🟢 低 | 6.4 | 面板自动刷新缺骨架态 | 面板 |

---

## 8. 功能拓展与体验增强建议

1. **统一设计系统**：抽 `aiops-design-tokens.css` 供两端引用；建立"营销氛围 / 工具密度"双层级变量，从根上消除割裂（解决 1.1，并顺带修复 1.2/1.3）。
2. **营销站内容静态化 + `<noscript>`**：根治无 JS 空白与 SEO 问题（2.1），同时改善首屏性能（6.2）。
3. **全局可访问性基线**：把面板已有的 `:focus-visible`、对比度提亮、焦点陷阱范式抽成共享片段，营销站直接复用（5.1/5.4/5.5）。
4. **动效降级与滚动锚点**：新增 `prefers-reduced-motion` 全局块 + `scroll-margin-top` 基础样式（5.2/6.1/2.2）。
5. **面板体验增强**：窄屏筛选抽屉（4.2）、列表刷新骨架/乐观更新（2.4）、区块标题组件统一（3.4）。
6. **信息架构**：功能详情页加分组快速导航（3.2）、对比表 sticky 首列（3.3）。
7. **传播**：补 OG 分享图（6.3）。

> 落地顺序建议：先啃 4 个🔴高（设计 token 抽取 → 营销站静态回退 → 焦点环 → reveal 兜底），再按中优先级逐批推进。如需，我可直接就任意一项产出可合并的代码补丁。
