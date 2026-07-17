---
kind: frontend_style
name: AIOps 监控平台前端样式体系（Web + Android）
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - cmd/server/web/theme-init.js
    - cmd/server/web/index.html
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/themes.xml
    - android/app/src/main/res/values/colors.xml
    - website/css/style.css
---

## 1. 系统概览
本项目包含两套独立的前端风格体系：
- **Web 管理后台**（Go 内嵌静态资源，纯 CSS + JS，无构建工具链）
- **Android 客户端**（Kotlin/Compose Material3 + 原生 XML Theme）
两者均采用「深色优先」的 SaaS 监控风格，通过 CSS 变量 / Material ColorScheme 实现明暗主题切换。

## 2. Web 后台样式（cmd/server/web）
### 2.1 核心文件
- `style.css` — 全局样式与组件库（约 2800+ 行），定义全部设计 Token、布局、组件、动画
- `theme-init.js` — 页面加载前同步读取 localStorage 的 aiops_theme 并设置 data-theme，避免首屏闪烁
- `index.html` — 入口，按顺序引入 theme-init.js 与 style.css
- `i18n-dashboard.*.js` — 多语言文案（与样式无关但同属前端资源）

### 2.2 设计 Token 体系
采用 CSS Custom Properties 集中声明，分三层：
- **品牌色**：--accent（蓝）、--accent2（青/紫变体）、渐变 --gradient / --gradient-soft
- **语义状态色**：--ok / --warn / --crit / --info 及其 -soft 半透明变体、对应文本色 *-txt、边框色 *-border
- **中性色阶**：背景 --bg/--bg2/--bg3、面板 --panel/--panel2/--panel3、线条 --line/--line2、文本 --txt/--txt2/--muted/--muted2、悬停 --hover-border、离线灰 --gray-off/--gray-txt
- **几何与动效**：圆角 --r-xs ~ --r-2xl、阴影 --shadow-sm ~ --shadow-xl、过渡 --transition-fast/~slow、间距 --sp-1 ~ --sp-16、模糊 --blur-*、触摸目标 --touch-min:44px
- **布局**：侧栏宽度 --sidew、内容最大宽 --max-w、顶部栏背景 --topbar-bg
- **语义别名**（v5.5.0 新增）：--surface、--text-primary/secondary/tertiary、--elevation-1~4 等，便于迁移到 Design System

### 2.3 主题策略
- 默认深色主题，通过 [data-theme="light"] 覆盖根变量实现浅色模式
- 主题切换由 JS 写入 localStorage.aiops_theme，theme-init.js 在 <head> 中同步执行，确保 CSP 安全且无闪烁
- 部分区域（如代码/终端）使用专用变量 --cmd-bg/--term-bg 保持跨主题一致性

### 2.4 组件约定
- 按钮：.btn（基础）、.btn.primary、.btn.danger、.btn.ghost、.btn.block、.btn:disabled
- 卡片：.card + 状态修饰 .ok/.warn/.crit/.info，带 hover 提升与脉冲动画
- 列表项：.row-item + 左强调条 .critical/.warning/.info，支持入场/离场动画
- 标签：.badge + 状态类 .ok/.warn/.crit/.warning/.status-ack/.status-silence
- 弹窗：.mask + .modal（头部/主体/底部三段式）
- Toast：.toast.ok/.err
- 搜索/分页：.search、.pager
- 网格：.grid（auto-fill + minmax(320px,1fr)）、.cards（6 列 KPI）、.top-grid（3 列排行榜）
- 表单：.field + 统一 input/select/textarea 重置，checkbox/radio 保留 accent-color

### 2.5 响应式策略
- 基于 CSS Grid + Flexbox，无媒体查询框架
- 关键断点：@media(max-width:900px) 隐藏安装按钮文字；@media(max-width:640px) 隐藏语言切换；@media(max-width:600px) 缩小用户下拉菜单
- 移动端侧栏可折叠为 64px 图标模式（.app.collapsed）

## 3. Android 客户端样式（android/app）
### 3.1 核心文件
- `ui/theme/Theme.kt` — Compose Material3 主题定义（ColorScheme + Shapes）
- `res/values/themes.xml` — 原生 Activity 主题（Material.Light.NoActionBar）
- `res/values/colors.xml` — 启动图标颜色（ic_bg/ic_fg）

### 3.2 设计约定
- 深色方案默认启用，主色 #5B8CFF，次色 #3ECF8E，警告 #E0A93B，错误 #E5564F
- 圆角体系：extraSmall=6dp → extraLarge=28dp，与 Web 端 --r-xs ~ --r-2xl 对齐
- 浅色方案高饱和配色，避免灰白底，保证对比度

## 4. 营销网站样式（website/css/style.css）
独立的宣传站点样式，同样采用 CSS 变量 + 明暗主题，但与后台共享品牌色家族（--accent/#3b82f6、--accent2/#22d3ee），保持跨产品视觉一致。

## 5. 开发者规范
1. 所有颜色必须走 CSS 变量，禁止硬编码十六进制值（除 logo SVG 等极少数场景）
2. 新增语义色时，同时提供 -soft 半透明变体及对应文本/边框别名
3. 组件复用：优先使用已有 class（.btn、.card、.badge、.row-item），而非新建样式
4. 主题兼容：新增 UI 元素需同时适配 [data-theme="light"] 覆盖块
5. 无障碍：focus-visible 统一使用 outline:2px solid var(--accent)，触摸目标 ≥ 44px
6. 动画：统一使用 --transition-fast/slow 时长与 cubic-bezier(.4,0,.2,1) 缓动
7. Android 侧：新增颜色/圆角需同步更新 Theme.kt 的 ColorScheme 与 AppShapes