---
kind: frontend_style
name: 前端样式体系：CSS 变量 + 深色主题 + Material3（Android）
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/colors.xml
    - android/app/src/main/res/values/themes.xml
    - website/css/style.css
---

## 1. 系统与方法论

- **Web 管理端**：纯 CSS，采用 **CSS 自定义属性（:root 变量）** 作为设计令牌中心，通过 `[data-theme="light"]` 切换浅色/深色主题。无 Sass/Less/Tailwind 等预处理或原子框架，所有样式集中在 `cmd/server/web/style.css`（约 2800 行），由 Go `embed` 内嵌到二进制中直接静态路由 `/style.css` 输出。
- **Android 移动端**：基于 **Jetpack Compose + Material Design 3**，在 `android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt` 中定义 `DarkColorScheme` / `LightColorScheme`、圆角 `AppShapes`，并通过 `AIOpsMonitorTheme` 组合根组件注入。原生资源仅保留极简图标配色 `colors.xml` 与空主题 `themes.xml`。
- **营销网站**：独立站点 `website/css/style.css`，同样使用 CSS 变量 + 深浅主题，但视觉风格更偏“SaaS 官网”（更大字号、渐变 Hero、网格背景），与管理端保持品牌色一致。

## 2. 核心文件与包

| 层 | 关键文件 | 职责 |
|---|---|---|
| Web 管理端 | `cmd/server/web/style.css` | 全局 CSS 变量、主题、布局、组件样式 |
| Android 主题 | `android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt` | Material3 颜色方案、形状、主题组合器 |
| Android 原生资源 | `android/app/src/main/res/values/colors.xml`、`themes.xml` | 图标底色、空壳主题 |
| 营销站 | `website/css/style.css` | 宣传页样式（与管理端分离） |
| 国际化文案 | `cmd/server/i18n/*.json` | 多语言文本（非样式，但与 UI 强耦合） |

## 3. 架构与设计约定

### 设计令牌（Design Tokens）
- **品牌主色**：`--accent`（蓝）、`--accent2`（青/紫变体），配合 `--ok/--warn/--crit/--info` 语义状态色及对应的 `-soft` 半透明变体。
- **中性色阶**：`--bg/--bg2/--bg3`（背景）、`--panel/--panel2/--panel3`（卡片层级）、`--line/--line2`（边框）、`--txt/--txt2/--muted/--muted2`（文本层级）。
- **间距/圆角/阴影/过渡**：统一以 `--sp-*`、`--r-*`、`--shadow*`、`--transition*` 暴露，避免硬编码。
- **语义别名**：新增 `--surface/--text-primary/--elevation-*` 等别名，便于未来迁移至 Design System。

### 主题策略
- 默认 **深色主题**（监控后台行业标准），通过 `<html data-theme="light">` 切换浅色。
- 每个 token 在 `:root` 和 `[data-theme="light"]` 下分别声明，确保对比度满足 WCAG AA。
- 营销站与管理端共享同一套品牌色家族，但具体明暗值可微调。

### 布局与响应式
- 管理端采用 **Grid 双栏**（侧边栏 + 主内容），侧边栏支持 `.app.collapsed` 收起为 64px 图标模式。
- 大量使用 `grid-template-columns: repeat(auto-fill, minmax(320px, 1fr))` 实现自适应卡片网格。
- 断点集中在 600/640/900/960/1024/1440px，覆盖手机到宽屏。

### Android 主题
- 默认启用 `darkTheme = true`，颜色方案遵循 Material3 的 primary/secondary/tertiary/surface/error 语义角色。
- 圆角体系 `AppShapes` 提供 extraSmall→extraLarge 五级阶梯，与 Web 端的 `--r-*` 对应。

## 4. 开发者应遵守的规则

1. **禁止硬编码颜色/间距/圆角**——一律使用 `var(--xxx)` 变量；新增视觉常量时同步补充浅色主题下的对应值。
2. **语义化命名优先**——用 `--ok/--warn/--crit` 表达状态，而非直接写 `#ef4444`。
3. **组件类名扁平化**——沿用现有 BEM 风格（如 `.card`、`.btn.primary`、`.field input`），不要引入新前缀。
4. **主题切换只改变量**——新增 UI 元素时必须在 `:root` 和 `[data-theme="light"]` 两处都覆盖相关变量。
5. **Android 侧统一走 Material3**——不要在 Compose 里手写 Color/Hardcode，全部通过 `MaterialTheme.colorScheme` 访问。
6. **营销站与管理端样式隔离**——`website/css/style.css` 不得引用管理端变量，两者通过品牌色家族保持一致即可。
7. **无障碍**——焦点可见性使用 `outline: 2px solid var(--accent)`，并确保对比度 ≥ 4.5:1（代码注释中已有 WCAG AA 校验说明）。