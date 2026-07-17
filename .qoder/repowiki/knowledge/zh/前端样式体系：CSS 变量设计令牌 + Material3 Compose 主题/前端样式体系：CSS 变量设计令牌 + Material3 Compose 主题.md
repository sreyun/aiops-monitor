---
kind: frontend_style
name: 前端样式体系：CSS 变量设计令牌 + Material3 Compose 主题
category: frontend_style
scope:
    - '**'
source_files:
    - cmd/server/web/style.css
    - android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt
    - android/app/src/main/res/values/themes.xml
    - website/css/style.css
---

## 系统概览

本仓库包含三个独立的前端界面，各自采用不同的样式方案，但共享统一的品牌色与深色基调。

### Web 管理面板（cmd/server/web）
- 样式方案：原生 CSS + CSS 自定义属性（CSS Variables），无构建器、无框架依赖
- 核心文件：style.css（约 2986 行），通过 Go embed 内嵌到二进制中直接由服务端提供
- 设计语言：参考 TDesign 暗色 / Linear 风格，克制扁平的深色后台界面
- 主题机制：基于 :root 和 [data-theme="light"] 选择器的双主题切换，所有视觉值均通过 CSS 变量暴露
- 响应式策略：使用 CSS Grid + auto-fill/minmax() 自适应网格，配合 @media 断点处理移动端

### Android App（android/app）
- UI 框架：Jetpack Compose + Material Design 3
- 主题定义：ui/theme/Theme.kt 中定义 DarkColorScheme 与 LightColorScheme，默认深色模式
- 圆角体系：AppShapes 提供 5 级圆角（6dp → 28dp）
- 原生资源：res/values/colors.xml 仅定义应用图标颜色，其余全部走 Compose 主题

### 营销网站（website）
- 样式方案：独立 css/style.css，与 Web 面板完全隔离
- 设计语言：简约大气现代风，同样以深色为基调，品牌色克制点缀
- 特性：支持深浅主题切换、滚动动画、响应式导航栏

## 关键文件与包

- cmd/server/web/style.css — Web 面板全局样式与设计令牌
- android/app/src/main/java/com/aiops/monitor/ui/theme/Theme.kt — Android Compose 主题定义
- android/app/src/main/res/values/themes.xml — Android 原生主题（Material.Light.NoActionBar）
- website/css/style.css — 营销站样式
- cmd/server/web/index.html — Web 入口，引用 style.css 与 theme-init.js

## 架构与约定

1. 设计令牌集中化：Web 面板在 :root 中统一定义品牌色（--accent）、语义色（--ok/--warn/--crit/--info）、中性色阶、间距、圆角、阴影、过渡等，并通过语义别名（--surface、--text-primary、--elevation-*）二次封装
2. 主题切换：通过 <html data-theme="..."> 或 JS 切换 data-theme 属性，CSS 变量自动覆盖实现深浅主题
3. 零外链原则：Web 面板不依赖任何外部 CSS 库，所有样式自给自足，便于 Go embed 打包
4. 组件类命名：使用 BEM 风格的短类名（.card、.btn、.modal、.toast、.badge 等），按功能域组织
5. Android 遵循 Material3：颜色、形状、字体全部通过 MaterialTheme 注入，避免硬编码颜色值

## 开发者应遵循的规则

- 禁止硬编码颜色/尺寸：Web 侧必须使用 CSS 变量（如 var(--accent)、var(--r-sm)），Android 侧必须通过 MaterialTheme.colorScheme 获取
- 保持主题一致性：新增 UI 元素需同时适配深色与浅色主题，确保 WCAG AA 对比度达标
- 复用已有组件类：优先使用 .btn、.card、.badge 等通用类，而非新建样式
- 响应式优先：使用 CSS Grid 的 auto-fill/minmax() 布局，避免固定宽度
- Android 默认深色：Compose 主题默认启用 darkTheme = true，符合监控工具行业惯例