---
kind: frontend_style
name: 前端样式体系：CSS 变量 + 明暗主题 + Material3（Web/Android）
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - cmd/server/web/theme-init.js
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/themes.xml
    - android/app/src/main/res/values/colors.xml
    - website/css/style.css
---

## 1. 系统概览
本项目包含两套独立的前端风格体系，分别服务于 Web 管理后台与 Android App，两者在视觉语言上保持统一但实现技术栈不同。

- **Web 管理后台**：纯 CSS + CSS 自定义属性（CSS Variables），通过 `data-theme` 属性切换明暗主题，无构建工具、无 CSS-in-JS、无第三方 UI 框架。
- **Android App**：基于 Jetpack Compose + Material Design 3，使用 `MaterialTheme` 的 ColorScheme 与 Shapes 定义品牌色板与圆角体系。
- **营销网站**（website/）：独立的 CSS 文件，采用与后台一致的 CSS 变量命名约定，但配色更偏产品官网风格。

## 2. 核心文件与包
- Web 主题入口与全局样式：
  - `cmd/server/web/style.css`（约 2900 行，全部样式集中于此）
  - `cmd/server/web/theme-init.js`（首屏同步应用 localStorage 中的主题，避免闪烁）
  - `cmd/server/web/index.html`（引入 theme-init.js 与 style.css）
- Android 主题定义：
  - `android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt`（Compose Material3 颜色方案与圆角）
  - `android/app/src/main/res/values/themes.xml`（Activity 级 Theme，继承 Material.Light.NoActionBar）
  - `android/app/src/main/res/values/colors.xml`（图标资源色）
- 营销站样式：
  - `website/css/style.css`

## 3. 架构与设计约定
### 3.1 CSS 设计令牌（Design Tokens）
Web 侧所有视觉常量均通过 `:root` 下的 CSS 变量声明，形成分层令牌体系：
- **品牌色**：`--accent` / `--accent2` 及其 soft 变体，贯穿按钮、链接、高亮等交互元素。
- **语义状态色**：`--ok` / `--warn` / `--crit` / `--info` 及对应 `-soft`、`-txt`、`-border` 变体，用于告警、健康度等状态表达。
- **中性色阶**：`--bg` / `--bg2` / `--panel` / `--line` / `--txt` / `--muted` 等，按层级组织背景、面板、边框、文本。
- **布局与动效**：统一的 `--r-*` 圆角、`--shadow-*` 阴影、`--transition-*` 过渡时长、`--sp-*` 间距。
- **语义别名**（v5.5.0 新增）：`--surface` / `--text-primary` / `--elevation-*` 等，为未来迁移到设计系统预留接口。

### 3.2 明暗主题策略
- 通过 `<html data-theme="dark|light">` 切换，`theme-init.js` 在页面渲染前从 `localStorage.aiops_theme` 读取并同步设置，避免首屏闪烁。
- 深色主题为默认（监控 SaaS 行业标准），浅色主题覆盖同一组变量名，保证组件无需感知主题即可适配。
- 部分区域（如代码块、终端）强制使用固定深色，不受主题影响。

### 3.3 响应式与布局
- 使用 CSS Grid 构建主布局（`.app` 两栏：侧边栏 + 内容区），支持 `.collapsed` 模式将侧边栏收缩为 64px 图标栏。
- 卡片网格普遍采用 `grid-template-columns: repeat(auto-fill, minmax(320px, 1fr))` 自适应列数。
- 关键断点：`600px`（用户下拉菜单宽度）、`640px`（顶栏语言切换隐藏）、`900px`（安装按钮仅留图标）。

### 3.4 Android Material3 主题
- 使用 `darkColorScheme` / `lightColorScheme` 定义深/浅两套 ColorScheme，主色 `#5B8CFF` 与 Web 侧 `--accent` 保持一致家族。
- 自定义 `AppShapes` 圆角体系（6/10/14/20/28 dp），与 Web 侧 `--r-xs` ~ `--r-2xl` 一一对应。
- Activity 级 Theme 继承 `Material.Light.NoActionBar`，Compose 层再按需覆盖为深色。

## 4. 开发者规范
- **禁止硬编码颜色值**：所有颜色必须引用 CSS 变量或 Material3 的 `colorScheme.*`，不得直接写十六进制色值。
- **新增状态色时**：需同时提供 `-soft`、`-txt`、`-border` 三个变体，并在 `:root` 与 `[data-theme="light"]` 两处同步声明。
- **主题切换**：通过 JS 修改 `document.documentElement.setAttribute('data-theme', ...)`，不要直接操作内联样式。
- **组件类名**：遵循 BEM 风格（如 `.btn.primary`、`.card.ok`、`.row-item.critical`），状态修饰符以语义化后缀区分。
- **无障碍**：focus-visible 必须保留 `outline` 或 `box-shadow` 焦点环；WCAG AA 对比度已在注释中记录，新增文本颜色时需验证。
- **移动端优先**：新增样式先考虑窄屏（≤640px），再用媒体查询扩展桌面布局。
