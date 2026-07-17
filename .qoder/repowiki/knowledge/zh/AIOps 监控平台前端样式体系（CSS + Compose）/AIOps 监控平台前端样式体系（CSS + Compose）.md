---
kind: frontend_style
name: AIOps 监控平台前端样式体系（CSS + Compose）
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - cmd/server/web/theme-init.js
    - cmd/server/web/index.html
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
---

## 1. 系统/方法概述

- Web 管理面板：纯原生 CSS + 少量内联 JS，**不依赖任何 UI 框架或 CSS 工具链**（无 Tailwind、Bootstrap、Ant Design、styled-components 等），通过 Go `embed` 将静态资源打包进二进制。
- Android 客户端：基于 **Jetpack Compose + Material3**，使用官方 `darkColorScheme` / `lightColorScheme` 定义明暗主题与圆角体系。
- 营销站（website/）：独立静态站点，有单独的 `css/style.css`，与主面板样式解耦。

## 2. 关键文件与包

- Web 面板
  - `cmd/server/web/style.css` — 全局样式与组件样式（约 2800+ 行）
  - `cmd/server/web/theme-init.js` — 首屏同步应用 `data-theme`，避免闪烁
  - `cmd/server/web/index.html` — 页面骨架，引用 `/style.css` 和 `/theme-init.js`
- Android 客户端
  - `android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt` — Material3 颜色方案与 Shapes 定义
  - `android/app/src/main/res/values/themes.xml`、`colors.xml`、`strings.xml` — 传统 XML 资源（与 Compose 并存）
- 营销站
  - `website/css/style.css` — 独立样式，不参与主面板

## 3. 架构与设计约定

### 3.1 设计 Token 体系（Web）
- 所有视觉变量集中在 `:root` 的 CSS Custom Properties，分为三层：
  - 品牌色：`--accent`、`--accent2` 及其 soft/hover 变体
  - 语义状态色：`--ok`、`--warn`、`--crit`、`--info` 及对应 `*-soft`、`*-txt`、`*-border`
  - 中性色阶：`--bg`、`--panel`、`--line`、`--txt`、`--muted` 等
- 统一间距/圆角/阴影/过渡：`--sp-*`、`--r-*`、`--shadow-*`、`--transition-*`
- 语义化别名（v5.5.0 新增）：`--surface`、`--text-primary`、`--elevation-*` 等，便于跨模块复用
- 概览页专属 token 以 `--ov-*` 前缀隔离，避免污染全局

### 3.2 明暗主题策略
- 默认深色主题，浅色通过 `[data-theme="light"]` 覆盖全部 CSS 变量
- 主题切换入口在用户下拉菜单，`localStorage.aiops_theme` 持久化
- `theme-init.js` 在 `<head>` 中同步执行，设置 `documentElement.data-theme`，避免首屏闪烁
- 同时配合 `<meta name="theme-color">` 适配移动端浏览器地址栏色

### 3.3 布局与响应式
- 应用骨架采用 CSS Grid：`.app { grid-template-columns: var(--sidew) 1fr }`
- 侧边栏可折叠为图标模式（`.app.collapsed`），桌面端宽度从 236px 收缩至 64px
- 内容区默认居中限宽 `max-width:1440px`，支持「宽屏」模式（`.app.wide` → 1720px）
- 大量使用 `auto-fill` + `minmax()` 实现卡片网格自适应，无需媒体查询断点

### 3.4 组件样式命名规范
- 全小写 kebab-case 类名：`.card`、`.btn`、`.modal`、`.host`、`.topbar`、`.nav-item` 等
- 状态修饰符通过附加 class：`.card.ok`、`.card.crit`、`.badge.warning`、`.st-up`、`.fwd-off`
- 视图切换通过父容器 class 控制：`.checks-list.pill`、`.fwd-list.fwd-grid`、`.view.active`
- 图标按钮统一 `.icon-btn`，危险操作加 `.danger` 修饰

### 3.5 Android 主题（Compose）
- 使用 `MaterialTheme(colorScheme = ..., shapes = AppShapes)` 包裹整个应用
- 深色/浅色两套 `colorScheme`，默认深色；圆角体系 `AppShapes` 提供 xs/small/medium/large/xl 五级
- 颜色值直接硬编码为 `Color(0xFFxxxxxx)`，未抽取到集中 token 文件

### 3.6 无障碍与交互细节
- 表单元素统一重置 `-webkit-appearance`，保留 checkbox/radio 原生渲染并通过 `accent-color` 着色
- 焦点环统一用 `outline:2px solid var(--accent)` + `outline-offset`，键盘可达
- 触摸目标最小尺寸 `--touch-min:44px`，遵循 Apple HIG

## 4. 开发者应遵循的规则

1. **只使用 CSS 变量**：新增颜色/间距/圆角必须写入 `:root` 或对应 `[data-theme]` 块，禁止硬编码十六进制值。
2. **语义优先**：优先使用 `--ok`、`--warn`、`--crit` 等语义变量而非具体色值，确保主题一致性。
3. **类名规范**：使用全小写 kebab-case，状态修饰符以空格分隔追加（如 `.card.crit`）。
4. **主题兼容**：新增样式需同时考虑 `[data-theme="light"]` 下的覆盖，保证明暗双主题可读性。
5. **Token 分层**：通用变量放 `:root`，页面专属变量加 `--ov-*` 前缀，避免冲突。
6. **Android 侧**：新增颜色/形状应在 `Theme.kt` 的 `DarkColorScheme` / `LightColorScheme` 中成对定义，保持对称。
7. **零外链原则**：Web 面板所有样式内联于 `style.css`，不得引入外部 CDN 样式库。