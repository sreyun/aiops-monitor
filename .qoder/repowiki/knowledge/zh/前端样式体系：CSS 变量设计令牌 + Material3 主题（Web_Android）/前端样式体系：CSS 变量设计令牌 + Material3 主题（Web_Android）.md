---
kind: frontend_style
name: 前端样式体系：CSS 变量设计令牌 + Material3 主题（Web/Android）
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

## 系统概览

本仓库包含两套独立的前端样式体系，分别服务于 Web 管理后台与 Android 客户端，二者在品牌色、语义化命名和明暗主题策略上保持视觉一致性。

### 1. Web 管理后台（Go 内嵌静态资源）

- **技术栈**：原生 CSS + CSS 自定义属性（CSS Variables），无构建器、无 Tailwind/SCSS/Less，零外链。
- **核心文件**：`cmd/server/web/style.css`（约 2800 行）、`cmd/server/web/theme-init.js`。
- **设计令牌（Design Tokens）**：通过 `:root` 与 `[data-theme="light"]` 两个根级块集中声明，覆盖：
  - 品牌主色 `--accent` / `--accent2`、状态色 `--ok` / `--warn` / `--crit` / `--info` / `--purple` 及其 soft/border/text 变体；
  - 中性色阶 `--bg` / `--panel` / `--line` / `--txt` / `--muted` 等；
  - 圆角 `--r-*`、阴影 `--shadow-*`、过渡 `--transition*`、间距 `--sp-*`、模糊 `--blur-*`；
  - 语义别名 `--surface` / `--text-primary` / `--elevation-*` 等，便于跨组件复用。
- **主题切换**：`theme-init.js` 在 `<head>` 同步执行，从 `localStorage.aiops_theme` 读取并设置 `documentElement.data-theme`，避免首屏闪烁；默认深色。
- **布局与组件约定**：
  - 应用骨架 `.app` 使用 CSS Grid（侧边栏 + 主内容），支持 `.collapsed` 图标模式与 `.wide` 宽屏模式；
  - 统一按钮族 `.btn`（primary/danger/ghost/block）、弹窗 `.mask/.modal`、Toast `.toast`、分页 `.pager`、搜索 `.search`；
  - 数据展示卡片 `.card`、主机卡片 `.host`、指标进度条 `.bar/.fill`、标签 `.badge`、空态 `.empty`；
  - 响应式以 `@media(max-width:640px)` / `900px` / `1440px` 断点为主，大量使用 `auto-fill/minmax` 网格自适应。
- **可访问性**：全局表单重置去除浏览器默认 outline，显式定义 `:focus-visible` 焦点环；注释中明确 WCAG AA 对比度校验。

### 2. Android 客户端（Jetpack Compose + Material3）

- **技术栈**：Kotlin + Jetpack Compose，基于 Material Design 3 (`MaterialTheme`)。
- **核心文件**：`android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt`、`res/values/themes.xml`、`res/values/colors.xml`。
- **主题定义**：
  - `DarkColorScheme` / `LightColorScheme` 分别定义 primary/secondary/tertiary/background/surface/error 等完整色板；
  - 默认启用深色主题（监控工具行业标准）；
  - 圆角体系 `AppShapes` 提供 extraSmall(6dp) → extraLarge(28dp) 五级阶梯，与 Web 的 `--r-*` 对应。
- **Activity 主题**：`Theme.AIOpsMonitor` 继承 `Material.Light.NoActionBar`，Compose 层再覆盖为自定义 ColorScheme。

### 3. 营销网站（独立站点）

- **位置**：`website/css/style.css`，与后台样式解耦但共享同一套品牌色与 token 命名风格（`--accent` / `--ok` / `--warn` / `--crit` / `--gradient` 等）。
- **特点**：更强调渐变、光晕、滚动动画与 CTA 转化，同样支持 `data-theme="light"` 明暗切换。

## 关键文件

- `cmd/server/web/style.css` — Web 后台全部样式与设计令牌
- `cmd/server/web/theme-init.js` — 主题预置脚本（localStorage 持久化）
- `android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt` — Compose Material3 主题
- `android/app/src/main/res/values/themes.xml` — Android Activity 主题
- `android/app/src/main/res/values/colors.xml` — 启动图标颜色
- `website/css/style.css` — 营销站样式（与后台共享设计语言）

## 架构与约定

| 维度 | 约定 |
|------|------|
| 令牌来源 | CSS Variables 集中声明于 `:root`，按“品牌色 → 语义状态 → 中性色 → 几何/动效”分组 |
| 主题开关 | Web 用 `data-theme` 属性，Android 用 `darkTheme` 布尔参数 |
| 组件类名 | 扁平 BEM 风格（如 `.nav-item.active::before`），不依赖框架选择器 |
| 响应式 | 移动端优先，大量 `auto-fill/minmax` 网格 + 少量断点覆盖 |
| 无障碍 | 显式 `:focus-visible` 焦点环、WCAG AA 对比度注释、`prefers-reduced-motion` 降级 |
| 图标 | 内联 SVG，尺寸统一 16–18px，颜色跟随当前文本色变量 |

## 开发者应遵循的规则

1. **新增颜色必须走变量**：禁止硬编码十六进制值，优先使用 `--accent` / `--ok` / `--panel` 等已有 token；若确需新语义，先在 `:root` 补充变量再使用。
2. **主题兼容**：所有新增样式需在 `[data-theme="light"]` 下验证可读性与对比度，必要时添加浅色覆盖块。
3. **组件复用**：通用交互（按钮、弹窗、输入框、标签）一律复用现有类名（`.btn.primary`、`.field input`、`.badge` 等），不要重复造轮子。
4. **圆角/阴影/过渡**：统一使用 `--r-*`、`--shadow-*`、`--transition*` 变量，保持视觉节奏一致。
5. **响应式**：优先使用 CSS Grid `repeat(auto-fill, minmax(...))` 实现自适应，仅在必要时写 `@media` 覆盖。
6. **Android 侧**：新增 UI 元素使用 `AIOpsMonitorTheme` 提供的 `MaterialTheme` 色彩与形状，不要直接写死 `Color()`。
7. **可访问性**：任何新增交互控件必须提供 `:focus-visible` 样式，确保键盘导航可见。