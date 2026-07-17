---
kind: frontend_style
name: AIOps Monitor 前端样式体系：原生 CSS + CSS 变量主题系统
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - cmd/server/web/theme-init.js
    - cmd/server/web/index.html
---

## 1. 使用的系统/方法
- 纯原生 CSS + CSS 自定义属性（CSS Variables），未引入任何 UI 框架（无 Tailwind、Ant Design、Element、Bootstrap 等），也未使用 Sass/Less 预处理器。
- 通过 data-theme="dark|light" 切换明/暗主题，默认深色；主题切换由 theme-init.js 在 <head> 同步执行以避免首屏闪烁。
- 样式文件经 Go embed 打包进二进制，静态路由精确匹配 /style.css，因此注释中明确避免 @import 外部 token 文件以防 404。

## 2. 核心文件与包
- cmd/server/web/style.css（约 2860 行）—— 全部样式集中在此单文件中，包含设计 Token、全局重置、布局、组件、响应式规则。
- cmd/server/web/theme-init.js —— 页面加载前从 localStorage("aiops_theme") 读取并设置 document.documentElement.data-theme，防止闪白。
- cmd/server/web/index.html —— 入口 HTML，内联大量视图结构（概览/主机/告警/日志/转发/硬件/NetFlow/数据源/SRE 等），并通过 class="view" 配合 CSS .view.active 做 SPA 级视图切换。
- website/css/style.css —— 营销站独立样式，与面板样式解耦。
- Android 端 android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt 及 res/values/themes.xml/colors.xml 为移动端 Material3 主题，与 Web 风格相互独立。

## 3. 架构与约定
- Token 驱动：所有颜色、圆角、阴影、过渡、间距、字体均定义在 :root 的 CSS 变量下，如 --accent/--ok/--warn/--crit/--info、--panel/--panel2/--line、--r-*、--shadow-*、--transition-*、--sp-* 等；浅色主题通过 [data-theme="light"] 覆盖同一组变量，保证双主题一致性。
- 语义化别名：新增 --surface/--text-primary/--border-subtle/--elevation-* 等别名，统一命名空间，便于未来扩展。
- 零外链、自给自足：品牌色与面板色在 style.css 内联声明，不依赖外部 CDN 或 @import，确保离线可运行。
- 组件类命名：采用 BEM 风格的扁平 class（.btn、.card、.modal、.field、.badge、.toast、.mask、.grid、.stack、.chip 等），按功能域组织，而非按页面拆分文件。
- 布局策略：主应用使用 CSS Grid（.app { grid-template-columns: var(--sidew) 1fr }），侧栏宽度由 --sidew 控制，支持 .app.collapsed 收起模式；内容区默认居中限宽 max-width:1440px，可通过 .app.wide 扩展到 1720px。
- 响应式：基于 @media (max-width:...) 断点，结合 auto-fill/minmax 网格实现卡片自适应；移动端隐藏语言切换按钮、调整侧栏等。
- 动画与反馈：统一的 --transition-fast/transition/transition-slow 缓动曲线，配合 pulseBreath、rowEnter、cardPulse 等关键帧提供克制微交互。
- 无障碍：focus-visible 统一描边环、aria-label/title/i18n 属性齐全，checkbox/radio 保留原生 accent-color 渲染以保证键盘可达性。
- iOS/Android 表单兼容：全局重置 -webkit-appearance:none，再对 checkbox/radio 恢复 accent-color，select 选项背景/高亮在深浅主题分别覆盖。

## 4. 开发者应遵循的规则
1. 只使用 CSS 变量：新增颜色/尺寸/阴影一律引用 var(--xxx)，禁止硬编码十六进制值；需要新 Token 时先在 :root 或 [data-theme] 块中声明。
2. 复用已有组件类：优先组合 .btn、.card、.modal、.field、.badge、.toast、.grid、.stack、.chip 等现有类，不要重复造轮子。
3. 主题安全：所有视觉差异必须通过 data-theme 选择器覆盖变量，不得写死 #fff / #000 等绝对色。
4. 保持单文件：样式继续集中在 style.css，按区块用注释分隔，避免拆成多文件导致 embed 路由复杂化。
5. 响应式优先：使用 auto-fill/minmax 网格和 @media 断点，不在 JS 里计算布局。
6. 动画克制：使用已定义的 --transition-* 缓动，新增动画需参考 pulseBreath/rowEnter 风格，避免过度动效影响监控场景可读性。
7. 无障碍基线：交互元素添加 aria-label/role，focus-visible 状态可见，文本对比度满足 WCAG AA。