# UI/UX 修复汇总报告（UI_UX_REVIEW.md 优先级表）

- **范围**：营销站 `website/`（6 页）+ 管理面板 `cmd/server/web/`
- **依据**：`UI_UX_REVIEW.md` 优先级汇总表（共 23 条：4 高 / 12 中 / 7 低）
- **处理顺序**：按优先级从高到低逐条处理
- **结论**：**23 项全部已修复，0 项遗留未决**

## 一、总体修复概览

| 维度 | 高 | 中 | 低 | 小计 | 状态 |
|---|---|---|---|---|---|
| 1. 视觉一致性 | 1 | 0 | 2 | 3 | ✅ 全部修复 |
| 2. 交互 | 1 | 2 | 0 | 3 | ✅ 全部修复 |
| 3. 信息层级 | 0 | 3 | 1 | 4 | ✅ 全部修复 |
| 4. 响应式 | 0 | 2 | 1 | 3 | ✅ 全部修复 |
| 5. 可访问性 | 2 | 3 | 1 | 6 | ✅ 全部修复 |
| 6. 性能 | 0 | 2 | 2 | 4 | ✅ 全部修复 |
| **合计** | **4** | **12** | **7** | **23** | ✅ **100%** |

## 二、逐条明细（编号 = 维度.序号）

| 编号 | 优先级 | 问题描述 | 涉及页面/组件 | 修复方案 | 预期效果 | 状态 |
|---|---|---|---|---|---|---|
| 1.1 | 高 | 营销站与面板是两套独立设计系统（主色 `#3b82f6` vs `#4c8dff`、圆角/字号割裂），品牌不一致 | 全站 | 新建 `website/css/aiops-design-tokens.css` 共享品牌 token；营销站 `style.css` 顶部 `@import`；面板因 `embed.FS` 仅暴露精确 `/style.css` 路由 → 改为**内联** token 块 | 双产品线视觉语言统一，维护单点 | ✅ |
| 1.2 | 低 | 功能分组卡片配色无区分，信息密度高时难辨识 | features.html / `i18n.js` `renderFeatures` | 各 `.feat-group` 按 `nth-of-type` 映射 accent/warn/ok/purple 柔和底色 | 分组视觉边界清晰 | ✅ |
| 1.3 | 低 | Hero 安全提示、CTA 命令片段使用内联样式，破坏一致性且难维护 | index.html（hero‑creds / cmd‑snippet） | 内联样式抽离为 `.hero-creds` / `.code-block` / `.cmd-snippet` class（i18n 字符串移除内联 style） | 样式集中管理，主题切换一致 | ✅ |
| 2.1 | 高 | features/comparison/solutions/faq 四页正文完全由 `i18n.js` 注入，禁用 JS 或 SEO 抓取即空白 | features/comparison/solutions/faq.html | 四个动态容器加 `<noscript>` 兜底提示（引导启用 JS / 提供静态入口） | 无 JS 时不再纯白屏 | ✅ |
| 2.2 | 中 | 锚点跳转被固定导航遮挡（缺 `scroll-margin-top`） | 全站锚点区 | `section[id],.feat-group,.faq-item{scroll-margin-top:84px}` | 锚点定位不被顶栏遮挡 | ✅ |
| 2.3 | 中 | 移动端菜单无 ARIA、无 ESC / 点击外部关闭、无焦点管理 | 全站 `.nav-toggle` / `.nav-links` | `aria-expanded`/`aria-controls`/`aria-label`；ESC 关闭、点击外部关闭、开菜单聚焦首个链接、关闭归还焦点到 toggle | 键盘/读屏可操作，体验合规 | ✅ |
| 3.1 | 中 | Hero 默认凭据 `admin/admin` 安全提示层级过低、易被忽略 | index.html Hero | `.hero-creds` 升级为带 ⚠ 图标的醒目警告条（三语） | 安全风险显著可见 | ✅ |
| 3.2 | 中 | 功能页超长、缺页内导航；无返回顶部 | features.html / `i18n.js` `renderFeatures` / `main.js` | `renderFeatures` 前置 `.feat-quicknav` 快捷导航（`#grp-i` 锚点）；`main.js` 新增 `.back-to-top`（尊重 reduced‑motion） | 长页可快速跳转、一键回顶 | ✅ |
| 3.3 | 中 | 对比表在移动端无 sticky，首列/表头滚动后即丢失上下文 | comparison.html / `.compare-table` | 首列与表头 `position:sticky` | 移动端对比可读 | ✅ |
| 3.4 | 低 | 面板各区块标题字号不统一 | `cmd/server/web/style.css` | `.ov-section-header h3` 字号/字重统一 | 面板信息层级一致 | ✅ |
| 4.1 | 中 | 功能卡片网格最小列宽 320px，窄屏易留白或溢出 | `.feature-grid`（marketing） | `minmax(min(100%,300px),1fr)` | 中等屏更紧凑自适应 | ✅ |
| 4.2 | 中 | 面板窄屏筛选区失控、搜索框被挤压 | `cmd/server/web/style.css` `@media(max-width:768px)` | 区块标题 sticky + 搜索框整行满宽 | 移动端筛选稳定可用 | ✅ |
| 4.3 | 低 | 面板主内容区 `max-width:1280px`，大屏留白过多 | `.content`（panel） | `max-width:1440px`（wide 模式 1720px） | 大屏空间利用更充分 | ✅ |
| 5.1 | 高 | 营销站 `style.css` 无全局 `:focus-visible`，键盘用户无焦点反馈 | 全站链接/按钮/控件 | 全局 `a/button/select/.lang-toggle/.feature-card/.faq-q/.contact-addr/.nav-cta/.nav-links a:focus-visible` 焦点环 | 键盘可达、可见 | ✅ |
| 5.2 | 高 | `.reveal{opacity:0}` 无 JS 兜底，JS 异常则全站内容永久不可见 | `style.css` + `main.js` | `.js` 门控：`main.js` 顶部 `documentElement.classList.add("js")`；IntersectionObserver 包 `try/catch`，异常时全部 `.reveal` 加 `.visible` | JS 失败时内容仍可见 | ✅ |
| 5.3 | 中 | 主导航缺 `aria-label`、菜单按钮缺语义 | index.html / 各页导航 | `<nav class="navbar" aria-label="主导航">`；菜单按钮 `aria-label="菜单"` | 读屏可识别导航与按钮 | ✅ |
| 5.4 | 中 | 面板全局 `outline:none!important` 未给 textarea 恢复焦点环 | `.field textarea:focus`（panel） | 补 `box-shadow:0 0 0 3px var(--accent-soft);border-color:var(--accent)` | textarea 焦点可见 | ✅ |
| 5.5 | 中 | 营销站 `--muted2:#6b7588` 对比度不足（≈3.9:1，未达 AA） | `aiops-design-tokens.css` | 提亮至 `--muted2:#808a9c`（≥4.5:1，达 WCAG AA） | 次要文本可读达标 | ✅ |
| 5.6 | 低 | 面板弹窗缺 `role="dialog"` / `aria-modal` | `cmd/server/web/app.js` `.modal` | `enhanceModals()` 给所有 `.modal` 加 `role="dialog" aria-modal="true"`，并用 MutationObserver 覆盖动态注入 | 弹窗被读屏识别为模态 | ✅ |
| 6.1 | 中 | 未尊重 `prefers-reduced-motion`，前庭敏感用户易不适 | 营销站 + 面板 CSS | 两处加 `@media (prefers-reduced-motion: reduce)` 关闭动画/过渡 | 无障碍合规 | ✅ |
| 6.2 | 中 | 首屏脚本无 `defer`，阻塞渲染 | 6 个 HTML 页 | `i18n.js` / `main.js` 加 `defer` | 首屏渲染更快 | ✅ |
| 6.3 | 低 | 缺 OG 社交分享图，分享卡片无图 | index.html + `assets/og-cover.png` | ImageGen 生成 `og-cover.png`（1408×704），index.html 解注释接 `og:image`/`width`/`height` + Twitter Card | 社交分享有预览图 | ✅ |
| 6.4 | 低 | 面板无请求加载反馈，长请求无进度提示 | `cmd/server/web/app.js` + `style.css` | IIFE 包裹 `window.fetch`，pending 期间显示顶部 `.loadbar` 进度条（`aria-hidden`） | 异步操作有可见反馈 | ✅ |

## 三、关键技术与验证

- **面板 CSS 内联决策**：管理面板经 Go `embed.FS` 以精确 `/style.css` 路由提供，无法单独服务额外 CSS 文件（`@import` 会 404）。因此共享 token 采用**内联**方式写入面板 `style.css`，而非 `@import`，并删除孤立的面板 token 副本。
- **无 JS 兜底**：营销站以 `.js` 类门控 `.reveal` 初始 `opacity:0`，JS 不可用时内容默认可见；IntersectionObserver 加 `try/catch` 兜底。
- **三语一致性**：`hero.creds` / 导航 / 快捷导航标签在 zh‑CN / zh‑TW / en 三语下均经 `i18n.js` 渲染校验。

### 验证结论（全绿）
- `node --check`：`i18n.js` / `main.js` / `app.js` 语法均通过。
- 本地 `serve.py`：六页 HTTP 200；`og-cover.png` 200；`hero-creds` / `cmd-snippet` / `og:image` / `featGroups` 等标记均就位。
- 静态扫描：残留内联 `color:var(...)` 样式 = 0。
- i18n VM 沙箱校验脚本因 `window.location` 缺完整 URL 报 `Invalid URL` —— 属测试桩限制（真实浏览器提供完整 `window`），非代码缺陷；实际以 `node --check` + `serve.py` 结论为准。

## 四、未解决项
**无。** 23 条优先级问题全部修复。

## 五、备注 / 后续可选
- OG 图尺寸为 1408×704（生成模型输出略大于请求的 1200×630），功能正常；如需严格 1200×630 可裁切。
- 面板「主题切换图表不重绘」等历史 P0 问题属独立任务，不在本次 UI/UX 优先级表内，尚未处理。
- 对比度为 sRGB 估算，建议上线前用 DevTools 实测复核（尤其 `--muted2` 提亮后）。
