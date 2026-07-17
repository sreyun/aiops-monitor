---
kind: frontend_style
name: 多端前端样式体系：CSS 变量主题 + Material3 + 原生 Android 资源
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - website/css/style.css
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/themes.xml
    - android/app/src/main/res/values/colors.xml
---

## 系统概览

本仓库包含三个独立的前端风格子系统，分别服务于 Web 管理面板、营销网站与 Android 客户端，各自采用不同的技术栈但共享“深色优先、品牌蓝为主色”的视觉基调。

### 1. Web 管理面板（cmd/server/web）

- **样式方案**：纯 CSS + CSS 自定义属性（`--accent` / `--bg` / `--panel` 等），通过 `[data-theme="light"]` 切换明暗主题，无构建工具链。
- **设计语言**：参考 TDesign 暗色 / Linear，单层中性色阶、4px 间距栅格、克制状态反馈，零外链图标（内联 SVG）。
- **主题实现**：在 `style.css` 中定义两套 `:root` 变量块（默认深色 + `[data-theme="light"]` 浅色覆盖），并通过 Go embed 直接嵌入二进制，避免外部依赖。
- **响应式策略**：基于 CSS Grid（`.app` 侧栏+主内容）、`auto-fill/minmax` 卡片网格、`@media` 断点（600/900/1200px）控制侧栏收起与宽屏模式。
- **交互细节**：统一的 `transition-fast/transition-slow` 缓动曲线、focus-visible 焦点环、toast/modal/mask 组件级动画。

### 2. 营销网站（website）

- **样式方案**：独立的 `website/css/style.css`，同样使用 CSS 变量 + `[data-theme="light"]` 双主题，但与面板完全隔离（不同变量命名空间）。
- **设计风格**：更偏现代 SaaS 落地页——大字号 Hero、渐变背景、玻璃态导航栏、滚动 reveal 动画、代码块高亮配色。
- **特色组件**：FAQ 手风琴、功能分组侧边导航、对比表格、移动端吸底 CTA、返回顶部按钮。

### 3. Android 客户端（android）

- **UI 框架**：Jetpack Compose + Material Design 3（MaterialTheme）。
- **主题定义**：`ui/theme/Theme.kt` 中显式声明 `DarkColorScheme`（默认）和 `LightColorScheme`，并定义 `AppShapes` 圆角体系（6/10/14/20/28dp）。
- **原生资源**：`res/values/themes.xml` 继承 `Material.Light.NoActionBar`，`colors.xml` 仅定义应用图标双色（`#0F1118` / `#5B8CFF`）。

## 关键文件

- `cmd/server/web/style.css` — 管理面板全部样式（~2800 行，含主题变量、布局、组件、动画）
- `website/css/style.css` — 营销网站样式（~650 行，独立主题）
- `android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt` — Compose Material3 主题
- `android/app/src/main/res/values/themes.xml` — Android 原生主题基类
- `android/app/src/main/res/values/colors.xml` — 图标颜色资源

## 架构与约定

| 维度 | Web 面板 | 营销站 | Android |
|------|----------|--------|---------|
| 主题切换 | `[data-theme]` 属性选择器 | 同上 | `darkTheme` 参数 |
| 颜色来源 | CSS 变量（`--accent` 等） | 独立 CSS 变量 | Material ColorScheme |
| 字体栈 | `-apple-system, PingFang SC, Microsoft YaHei` | 同左 | 系统字体族 |
| 圆角体系 | `--r-xs/sm/r/lg/xl/2xl` (6/8/10/14/18/24px) | 同命名空间 | 6/10/14/20/28dp |
| 阴影层级 | `--shadow-sm/lg/xl` 三档 | 两档 | elevation 系统 |
| 响应式 | CSS Grid + @media | CSS Grid + @media | Compose 自适应布局 |

## 开发者应遵循的规则

1. **新增颜色必须走 CSS 变量**：面板用 `--ok/--warn/--crit/--info` 语义色，禁止硬编码十六进制值；Android 使用 `MaterialTheme.colorScheme` 对应角色。
2. **统一间距与圆角**：使用 `--sp-*` 与 `--r-*` 变量，保持 4px 基准栅格；Android 使用 `AppShapes` 中的预设。
3. **主题适配**：所有新组件需同时验证深色/浅色模式下 WCAG AA 对比度（注释中已标注 muted2 调整历史）。
4. **无外链依赖**：图标使用内联 SVG，字体使用系统字体栈，确保离线可用。
5. **可访问性**：保留 `focus-visible` 焦点环，尊重 `prefers-reduced-motion` 媒体查询。