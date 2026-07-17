---
kind: frontend_style
name: AIOps 监控平台前端样式体系（Web + Android）
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - cmd/server/web/theme-init.js
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/colors.xml
    - android/app/src/main/res/values/themes.xml
---

## 1. 系统概览

本项目包含两个独立的前端，各自拥有完整的样式体系：
- Web 仪表盘：Go 内嵌的纯 CSS/JS SPA，通过 cmd/server/web/style.css 提供全部样式，采用 CSS 变量驱动的双主题（深色默认 / 浅色可选）。
- Android 仪表盘：基于 Jetpack Compose + Material Design 3 的 Kotlin 应用，通过 Theme.kt 定义 ColorScheme、Shapes 等设计令牌。

两者均遵循“克制、扁平、专业”的深色后台风格，参考 TDesign 暗色与 Linear 的设计语言。

## 2. 关键文件与包

- Web 仪表盘核心样式：cmd/server/web/style.css（约 3000 行，含全部样式与主题变量）
- Web 主题切换：cmd/server/web/theme-init.js（运行时切换 data-theme）
- Android 主题定义：android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt（Material3 主题）
- Android 资源：android/app/src/main/res/values/colors.xml、themes.xml（图标与基础主题）

## 3. 架构与设计约定

### Web 仪表盘（CSS 变量 + 双主题）
- 设计令牌集中管理：所有颜色、圆角、阴影、间距、字体、过渡时间均以 CSS 自定义属性在 :root 中声明，形成单一事实源。
- 双主题实现：通过 [data-theme="light"] 选择器覆盖同一组变量名，实现深色/浅色无缝切换；深色为默认主题。
- 语义化命名：变量按功能域分组——品牌主色（--accent/--accent2）、状态色（--ok/--warn/--crit/--info）、中性色阶（--bg/--panel/--line/--txt）、布局（--sidew/--touch-min）、组件专用（--ov-card-shadow 等）。v5.5.0 新增语义别名（--surface、--text-primary、--elevation-*）以对齐现代设计系统。
- WCAG 对比度保障：注释明确记录 muted2 值调整以满足 AA 4.5:1 标准，避免浅底上可读性不足。
- 零外链、无构建依赖：纯 CSS，无需 Sass/Less/Tailwind，直接由 Go embed 嵌入二进制。
- 响应式策略：使用 CSS Grid（.app 两栏布局、.cards 6 列 KPI、.grid auto-fill 卡片网格）+ @media 断点（600px/640px/900px），支持侧栏收起（.app.collapsed）与宽屏模式（.app.wide）。
- 交互一致性：统一的按钮族（.btn.primary/.danger/.ghost）、弹窗（.mask/.modal）、Toast、分页（.pager）、标签（.badge.*）、进度条（.bar/.fill）等原子类。

### Android 仪表盘（Material Design 3）
- ColorScheme 驱动：DarkColorScheme 与 LightColorScheme 分别定义 primary/secondary/tertiary/background/surface/error 等完整调色板，与 Web 端的 --accent/--ok/--warn 等语义色一一对应。
- Shapes 体系：AppShapes 提供 extraSmall(6dp) → extraLarge(28dp) 五档圆角，与 Web 端 --r-xs…--r-2xl 数值一致。
- 默认深色：darkTheme = true 作为监控工具的行业惯例，用户可在设置中切换。
- 最小 Android 资源：colors.xml 仅保留应用图标背景/前景色，其余全部走 Compose 主题。

## 4. 开发者应遵守的规则

1. 优先使用 CSS 变量而非硬编码颜色：新增 UI 元素时引用 var(--xxx)，不要直接写十六进制色值，确保跟随主题切换。
2. 保持语义色映射一致：Web 端 --ok/--warn/--crit/--info 与 Android 端 secondary/tertiary/error 对应，跨端状态色需保持一致。
3. 遵循统一间距与圆角：使用 --sp-* 与 --r-* 变量，避免随意设定 px 值。
4. 对比度达标：新增文本/边框色时，确保在深/浅两种背景下均满足 WCAG AA 4.5:1（可参考 muted2 的调整思路）。
5. 响应式优先：使用 CSS Grid/Flexbox 组合，配合 @media (max-width: ...) 处理窄屏，不要固定宽度布局。
6. Android 侧使用 Material3 组件：通过 MaterialTheme 提供的 color/scale/typography，不要手写原生 View 样式。
7. 零外链原则：Web 端不得引入外部 CSS/字体/图标库，所有资源必须内联或随二进制分发。