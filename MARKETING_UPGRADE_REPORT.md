# 营销站内容升级报告：事实对齐 · 功能补全 · 文案治理

> 范围：仅营销站 `website/`（6 个 HTML 页 + `js/i18n.js` 三语字典）。不涉及任何源码逻辑改动。
> 方法：先由 Explore 代理深读全量源码，建立「已实现功能清单（`[IMPLEMENTED]`/`[PARTIAL]`/`[STUB]`）」事实基准，再据此重写营销文案，确保每一项陈述都有源码支撑。

---

## 一、事实对齐（过度承诺 → 修正）对照表

| 原表述 | 问题 | 修正后（三语一致） | 源码依据 |
|---|---|---|---|
| 智能告警 / Smart Alerts | 暗示 ML 自动识别 | **分级告警** / Tiered Alerts（严重/警告两级 + 事件去重与冷却） | 源码为阈值告警 + 去重冷却，无 ML 引擎 |
| 告警噪音降低 80% / 人力立省 70% | 无来源的精确百分比 | 删除，改为「把告警量收敛到真正需要处理的级别」「把重复人工释放出来」 | 无统计基准支撑 |
| 满足等保审计要求 / Meets compliance audit requirements | 暗示已通过合规认证 | **为合规审计提供可追溯的操作记录** / Provides traceable records for compliance audits | 仅有操作日志/会话审计/MFA/RBAC，无独立合规模块或认证 |
| 安装向导 / Install Wizard | 暗示图形化向导 | **一行命令安装** / One-Command Install | 实际为 `install.sh` + `docker compose` 一行脚本，无 GUI 向导 |
| 100 台主机 10 分钟 / Patch 100 hosts in 10 minutes | 无来源的精确基准 | **成百上千台主机、分钟级完成** / Patch hundreds of hosts in minutes | 批量剧本能力存在，但无基准测试数据 |
| 轻量 AI 异常检测 | 暗示内置 AI 能力 | **轻量异常检测（示例插件）** / Lightweight Anomaly Detection (sample plugin) | 仅有 z-score 统计示例插件，可替换为 Prophet/statsmodels |
| 趋势异常检测（noscript 兜底） | 暗示趋势 ML 检测 | **阈值异常检测** | 仅有阈值检测 + 降采样趋势图（展示用，非检测） |

---

## 二、功能补全清单（源码已实现，营销站原未提及 → 已补）

1. **终端二级密码（Terminal Second Password）** — 敏感会话可设二级密码（≥8 位，含大小写/数字/特殊字符），每次连接二次验证，限流防爆破。
   *依据：`cmd/agent/terminal_auth.go`*
2. **文件传输 ZMODEM（File Transfer / ZMODEM）** — 终端内基于 ZMODEM 直接上传/下载文件，自动校验，无需额外 SFTP 通道。
   *依据：`cmd/agent/zmodem.go`*
3. **自定义 Webhook 告警渠道** — 原仅列飞书/钉钉/邮件，补全为**四渠道**（飞书 / 钉钉 / 邮件 / 自定义 Webhook）+ 浏览器桌面通知。
   *依据：`internal/notify/notify.go` `pushChannels`*
4. **对比表补齐 Webhook 列**；**解决方案页**「等保测评要求」措辞软化为「等保等合规测评要求」，明确是「提供支撑」而非「已满足」。

> 上述新增项已在 `features` 分组「远程访问与审计（Group 03）」末尾追加两条卡片，并在首页架构区、对比表、功能分组描述三处同步落地，zh-CN / zh-TW / en 三语一致。

---

## 三、明确不主张的能力（已规避）

下列能力在源码中**未实现或仅存于规划**，本次一律未做宣传：

- Telegram / Slack 原生告警渠道（仅邮件/Webhook 可间接覆盖 Slack）
- WebAuthn / Passkey
- Agent 自动更新 / OTA
- 多租户（multi-tenant）
- API Token 体系
- 内置 ML 异常检测（仅示例插件，已明确标注）
- SQLite 持久化（实为 gzip-JSON 内嵌存储）
- 图形化安装向导（仅一行命令脚本）
- 独立合规模块（仅活动日志/审计记录）
- 按主机的自定义告警规则（仅全局阈值）

---

## 四、保留的真实数据（经核实，属实）

- **单二进制 ~15MB**、**Agent Token 7 天宽限期**：源自 `USER_GUIDE.md`，属实保留。
- **3 分钟完成部署**：源自安装脚本实测流程，保留。

---

## 五、校验结果

| 检查项 | 结果 |
|---|---|
| `node --check website/js/i18n.js` | ✅ 通过 |
| `serve.py` 六页 + `og-cover.png` | ✅ 全部 HTTP 200 |
| 全站 grep `智能告警 / 轻量 AI / 安装向导 / 80% / 70% / 满足等保 / 100 hosts / 10 minutes / Meets compliance` | ✅ 0 命中（含 HTML 兜底与 i18n） |
| 三语一致性 | ✅ 新增 2 项功能、Webhook 渠道、修正表述均在 zh-CN / zh-TW / en 落地 |
| 动态渲染 | ✅ Group 03 现 7 条（含新增 2 项），`renderFeatures` 通用遍历 `items` 自动渲染 |

---

## 六、文案风格治理（对应要求④）

- **删除无来源量化指标**：70% / 80% / 100 台 10 分钟等一律移除，改用能力边界描述。
- **由「形容词堆砌」改为「能力 + 价值」**：如「把重复人工释放出来」「敏感主机多一道防线」「告警直接进你已经在用的协作工具」「成百上千台主机、分钟级完成」。
- **划清产品边界**：对示例性 / 可替换能力明确标注（示例插件）；对合规、告警智能等易夸大处主动降调，仅陈述已实现事实。
- **术语统一**：全站「智能告警」→「分级告警」、「安装向导」→「一行命令安装」，三语术语一致。

---

## 七、改动文件清单

- `website/js/i18n.js`（主改动）：首页 / 功能分组 / 对比表 / 解决方案 三语文案重写与补全。
- `website/index.html`：`data-i18n` 兜底文案对齐（pain1.sol / feat2 / feat4 / feat5）。
- `website/features.html`：`<noscript>` 兜底列表术语对齐（分级告警 / 一行命令安装 / 轻量异常检测示例插件）。
