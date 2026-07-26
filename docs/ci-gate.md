# CI Gate（安全与 SQL 变更门禁）

本平台将「检测 → 研判 → 审批 → 执行 → 回验」拆开：自动化发现与 AI 摘要不直接改生产；高风险变更必须走变更单。

## 安全扫描门禁

| 阶段 | 行为 |
|------|------|
| 检测 | Agent 加固/CIS-lite + OSV；Web Nuclei；内容审计 DLP |
| 告警 | 危急/高危 Finding、慢 SQL、策略 `deny`/`block` → 通知；危急可建 Incident |
| AI | 可选扫后摘要（`auto_ai_summary`）；不替代引擎检出 |
| 基线 | 与上次完成扫描对比 added/removed/worsened/improved |

建议在发布流水线中：

1. 对预发目标跑 Web 扫描（或复用最近报告），阻断 **critical** 未处置项。
2. 主机镜像构建后跑一次 `host_security_scan`，阻断 **critical hardening / malware**。
3. 内容审计 Gateway 摄入使用独立 Bearer（`AIOPS_CONTENT_AUDIT_INGEST_TOKEN`），策略拦截事件必须告警。

## SQL 变更门禁

| 操作 | 要求 |
|------|------|
| EXPLAIN / Schema / 进程锁 | 只读 |
| 索引 DDL（非 prod） | `allow_exec=true` 或变更单 |
| 索引 DDL（prod） | **必须** propose → approve → 一次性 execute |
| `KILL <pid>` | **必须** 变更单（kind=`kill`），审批后执行 |
| 冻结窗 | Change Window 勾选 category=`sql` 且 `freeze=true` 时，禁止 DDL（KILL 仍可用于解堵） |

PostgreSQL 连接（`driver=postgres`）仅支持测试连通、EXPLAIN、进程/锁与 Schema 健康抽检，**不提供** DDL/KILL。

## 示例流水线钩子

```bash
# 伪代码：发布前检查最近 Web 扫描是否仍有 open critical
curl -fsS -H "Cookie: ..." \
  "$AIOPS/api/v1/security/overview" | jq -e '.web.open_critical == 0'
```

将上述检查接入 GitHub Actions / GitLab CI；失败则阻断合并或部署。
