---
kind: frontend_style
name: 前端样式体系：CSS 变量 + 双主题 + Compose Material3
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/themes.xml
    - android/app/src/main/res/values/colors.xml
    - website/css/style.css
---

## 1. 系统/方法概述
- Web 仪表盘（Go embed 静态资源）：纯 CSS，通过 :root / [data-theme] 的 CSS 自定义属性实现深色/浅色双主题；无 Sass/Less/Tailwind 等预处理或原子框架。
- Android 移动端：基于 Jetpack Compose + Material Design 3，使用 darkColorScheme / lightColorScheme 定义色板与圆角体系，默认深色主题。
- 营销网站（website/）：独立站点，同样采用 CSS 变量 + 深浅主题，但视觉风格更品牌化，与仪表盘保持统一品牌色家族。

## 2. 关键文件与包
- 仪表盘样式入口：cmd/server/web/style.css（约 3000 行，集中管理所有面板样式、组件、动画、响应式）
- 仪表盘 HTML/JS：cmd/server/web/index.html、cmd/server/web/js/*.js、cmd/server/web/i18n-dashboard*.js
- Android 主题与颜色：
  - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt（Compose ColorScheme + Shapes）
  - android/app/src/main/res/values/themes.xml（Android 原生 Theme 基类）
  - android/app/src/main/res/values/colors.xml（图标/应用级基础色）
- 营销站样式：website/css/style.css（独立设计语言，但与仪表盘共享品牌蓝/青主色）

## 3. 架构与设计约定
- CSS 变量 Token 体系
  - 全局品牌色：--accent、--accent2、--ok、--warn、--crit、--info、--purple 及对应 -soft 半透明变体。
  - 中性色阶：--bg / --bg2 / --bg3、--panel / --panel2 / --panel3、--line / --line2、--txt / --txt2 / --muted / --muted2。
  - 功能语义别名：--surface、--border-subtle、--text-primary、--elevation-* 等，用于跨组件复用。
  - 布局/动效常量：--r-* 圆角、--shadow-* 阴影、--transition* 过渡时长、--sp-* 间距、--blur-* 模糊、--sidew 侧栏宽度等。
  - 概览页专属 token 以 --ov-* 前缀隔离，避免污染全局。
- 主题切换机制
  - 深色为默认；通过 <html data-theme="light"> 切换至浅色，同一份 CSS 中用 [data-theme="light"] 覆盖变量值。
  - 顶部工具栏提供语言/主题切换 UI，由 JS 写入 data-theme 并持久到 localStorage。
- 组件样式组织
  - 按区域划分：.app 骨架 → .sidebar 侧栏 → .topbar 顶栏 → .content 内容区 → 各页面区块（KPI 卡片、主机网格、告警列表、端口转发、数据源接入等）。
  - 通用按钮族：.btn、.btn.primary、.btn.danger、.btn.ghost；状态脉冲 .pulse；弹窗 .mask/.modal；Toast .toast；分页 .pager。
  - 表格/列表：.row-item + 左侧 3px 强调条（critical/warning/info），配合 badge 标签区分状态。
  - 图表/指标：.card KPI 卡片、.grid auto-fill 自适应网格、.sparkline 迷你折线、.top-grid TOP10 排行。
- 响应式策略
  - 桌面端固定侧栏 + 主内容区；支持 .app.collapsed 收起为 64px 图标模式。
  - 移动端通过 @media(max-width:...) 隐藏/折叠导航、调整字号与间距；搜索框在窄屏下百分比宽度。
- 可访问性细节
  - 表单元素统一重置浏览器默认外观，保留 focus-visible 高亮环；checkbox/radio 使用 accent-color 跟随主题。
  - 浅色模式下对 --muted2 做了 WCAG AA 对比度修正注释，确保可读性。
- Android Compose 主题
  - 深色/浅色两套 ColorScheme，默认启用深色；AppShapes 定义 5 级圆角（6/10/14/20/28dp）。
  - 主色 primary=0xFF5B8CFF 与 Web 仪表盘 --accent 家族一致，保证多端视觉统一。
- 营销站差异化
  - website/css/style.css 拥有独立的变量集与更强烈的渐变/光晕效果，面向对外宣传；但仍沿用相同的品牌蓝/青主色与暗色基调。

## 4. 开发者应遵循的规则
- 优先使用 CSS 变量而非硬编码颜色/尺寸：新增组件时从 --accent、--panel、--r-sm 等 token 取值，不要直接写十六进制色值。
- 主题扩展方式：如需新增主题态，仅在 [data-theme="xxx"] 块内覆盖变量，不要在具体选择器里写死颜色。
- 命名约定：业务区块 token 加前缀（如 --ov-*）避免冲突；组件 class 使用短横线分隔（.nav-item、.check-card）。
- 响应式断点：复用现有 600px / 900px / 1024px 等断点，避免引入过多新断点。
- Android 端：所有颜色/圆角通过 MaterialTheme.colorScheme 与 AppShapes 引用，禁止在 Composable 中硬编码 Color(...)
- 营销站与仪表盘分离：website/css/style.css 与 cmd/server/web/style.css 是两套独立样式，修改品牌色需同步更新两处以保持统一。