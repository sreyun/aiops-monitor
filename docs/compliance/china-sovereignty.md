# 信创与数据主权合规（中国区）

> AIOps 面向自建机房、混合云与信创环境。本章记录已适配能力与待办清单，
> 作为政企采购与等保评估的参考材料。

## 1. 已适配能力

| 维度 | 现状 |
|---|---|
| 操作系统 | Linux（麒麟/欧拉/统信兼容：Agent 有 kysec/SELinux/AppArmor 检测与降级）、Windows Server 2012+、macOS |
| 硬件 | Redfish（华为 iBMC/Dell iDRAC/HP iLO）、华为 OceanStor 存储、鲲鹏/飞腾（linux/arm64 官方构建） |
| 云/镜像 | 华为云 SWR 镜像、docker-compose 一键部署 |
| 移动端 | HarmonyOS 原生客户端（ArkTS） |
| 数据库 | PostgreSQL 系（pgvector）；PG 兼容的国产库（人大金仓/达梦/高斯）需按 SQL 方言逐项验证 |
| 密钥 | 自管 AES-256-GCM 主密钥（`AIOPS_SECRET_KEY`），不依赖外部 KMS |
| 审计 | 全量操作审计（`audit_log`）+ 终端录制 + AI 工具审计双写 PG |
| 网络 | 反向通道/relay 免开公网入站，符合内网合规要求 |

## 2. 等保 2.0 三级映射建议

| 等保要求 | AIOps 对应 |
|---|---|
| 身份鉴别 | 密码策略 + MFA(TOTP) + 登录防爆破 |
| 访问控制 | RBAC（admin/operator/viewer）+ 路由级鉴权 |
| 安全审计 | 审计日志 + 终端录制 + 导出（`/api/v1/audit-export`） |
| 入侵防范 | FIM 文件监控 + 主机/Web 漏洞扫描 + 内容审计 |
| 数据保密性 | 传输 TLS + 配置落盘加密 + 日志传输加密 |
| 备份恢复 | PG 备份/恢复 + 远程备份（`backup.go`） |
| 集中管控 | 统一控制台 + 多主机安全策略下发 |

## 3. 待办清单（按优先级）

1. **国产数据库适配矩阵**：对达梦/人大金仓/高斯完成 SQL 方言回归测试，
   输出官方支持声明。
2. **加密机/国密对接**：SM2/SM4 证书与加密机（HSM）支持（当前为国际算法）。
3. **等保测评材料包**：将“安全基线检测报告”`logProductionSecurityBaseline`
   固化为可导出的测评附件。
4. **数据出境说明**：明确默认全部数据本地化，无遥测上报（可选关闭的
   版本检查除外）。

## 4. 合规红线（默认值必须安全）

- 生产强制 `AIOPS_SECRET_KEY`（缺省启动仅警告，`AIOPS_STRICT_SECURITY=true`
  时直接拒绝启动）。
- 默认口令 `admin/admin` 首次登录强制改密（`MustChangePassword`）。
- docker-compose 已改为缺失密钥即启动失败（`${VAR:?}`），杜绝弱默认口令上线。
