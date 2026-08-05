# 千台主机部署手册（Scale to 1000+ hosts）

> 目标：单实例（server + PostgreSQL + VictoriaMetrics）稳定承载 1000+ 主机，
> 覆盖采集、告警、远程运维、AI 诊断四条主链路。本文档给出可执行的配置基线
> 与已落地/待落地能力清单。

## 1. 架构基线

```
1000+ hosts (Agent)  ──►  aiops-server (Go 单体, :8529)
                            ├── PostgreSQL (pgvector)  配置/用户/审计/事件/工单/会话
                            └── VictoriaMetrics       所有时序指标（remote_write）
```

- Agent 直连或经 relay 网关聚合；relay 仅需一台能出网的机器。
- 前端（经典版 Web 控制台）由 server 内嵌提供，无需独立部署。
- 生产必须 TLS（`AIOPS_TLS_CERT/AIOPS_TLS_KEY`）或置于 TLS 终止反向代理之后，
  并设置 `AIOPS_TRUST_PROXY=true` 使审计 IP 正确。

## 2. Agent 采集参数（agent config.yaml）

| 参数 | 100 台 | 1000 台 | 说明 |
|---|---|---|---|
| `report_interval` | 30s | 60s | 采样粒度与带宽的平衡点；Prometheus 同量级 |
| `plugin_interval` | 60s | 120s | Python 插件 spawn 开销大，大集群降频 |
| `log_encrypt` | true | true | 日志传输 AES-256-GCM |
| `tls_skip_verify` | false | false | 生产必须校验证书 |

带宽估算（60s 全量 JSON + gzip）：单台约 2–6 KB/轮，1000 台约 100–300 KB/10s，
server 出网带宽 1–3 Mbps，常规千兆内网无压力。

> 路线图：当前为上轮全量上报。增量上报（仅发送变更字段 + 断点续传）已在评估中，
> 落地后同规模带宽可再降 60–80%。

## 3. PostgreSQL 配置与数据治理

### 3.1 参数（docker-compose 已内置，生产按机型上调）

| 参数 | 建议 | 说明 |
|---|---|---|
| `max_connections` | 250–500 | 与 server `SetMaxOpenConns(200)` 对齐 |
| `shared_buffers` | 内存 25%（如 32G 机器给 8G） | 热数据命中 |
| `effective_cache_size` | 内存 50–75% | 查询计划器 |
| `work_mem` | 16–64MB | 排序/哈希；过高会撑爆内存 |
| `checkpoint_completion_target` | 0.9 | 平滑写盘 |

### 3.2 表治理

- `flow_records` 已按月分区（`migrateFlowRecordsToPartitioned`），大集群务必保持。
- `audit_log` / `events` / `ai_call_events` 为增长型表：建议按季度分区 +
  `Retention` 归档到冷表后 DELETE（保留策略见 `server_config` 的 `retention` 字段）。
- 定期 `VACUUM (ANALYZE)`：可用 cron/systemd timer 每周执行一次。
- 备份：`pg_dump` + `pg_restore` 全量 + WAL 归档（企业版建议 PITR）。

## 4. VictoriaMetrics 配置

| 参数 | 建议 | 说明 |
|---|---|---|
| `-retentionPeriod` | 36–100 | 100 为“永久”，磁盘吃紧用 36 个月 |
| `-storageDataPath` | 独立卷 | 与系统盘分离 |
| `-memory.allowedPercent` | 30–50 | 防 OOM |
| `-search.maxQueryDuration` | 30s | 防慢查询拖垮实例 |

server 侧写入已做：非阻塞 channel（8192 缓冲）+ 熔断器（`vm.go`）+ 批量
`/api/v1/import/prometheus`。大集群建议将 VM 与 PG 分机部署。

## 5. Server 侧容量要点

- 告警评估 `notify.go` 每 10s 全量 `Evaluate(ListHosts())`：1000 台主机单轮评估
  预计 <100ms，可接受；若上万台再引入增量/分片评估。
- 前端轮询已按视图 TTL 降载（overview/hosts/alerts 高频，其余低频），
  1000 台列表渲染建议启用虚拟滚动（前端构建链落地时一并做）。
- 远程终端/桌面为长连接：默认单进程可支撑数百并发会话；若并发达量，
  建议 server 多副本 + 会话亲和（后续版本支持）。

## 6. 自监控

- `GET /healthz` 接入负载均衡/容器健康检查。
- 观察 PG `pg_stat_activity`（慢 SQL）、VM 磁盘水位、server RSS。
- 日志轮转：server 输出到 stdout（容器）/ 文件（裸机），agent 已内置
  7×10MiB 滚动日志。

## 7. 压测方法（发布前必做）

1. 用脚本伪造 1000 台 Agent 并发上报（构造 `shared.Report` 结构 POST
   `/api/v1/agent/report`），观察 server CPU/P95 延迟与 PG 连接数。
2. 并发开启 100 个终端会话 + 10 路桌面，确认 PTY 无泄漏（`/proc` fd 数稳定）。
3. 查询压力：并发 dashboard 查询 + AI 诊断，确认 VM/PG 无锁等待尖峰。

## 8. 已知瓶颈与路线图

| 瓶颈 | 现状 | 规划 |
|---|---|---|
| Agent 全量上报 | 每轮全量 JSON | 增量上报 + 断点续传 |
| PG 单 JSONB 配置写放大 | 每次保存全量写 | 配置分域存储 |
| 前端首屏体积 | ~3MB 未压缩 JS | esbuild 最小构建链 |
| 大表增长 | 仅 flow_records 分区 | audit/events/ai_call 分区归档 |
