---
kind: external_dependency
name: VictoriaMetrics - 时序指标存储
slug: victoriametrics
category: external_dependency
category_hints:
    - vendor_identity
scope:
    - '**'
---

### VictoriaMetrics
- **角色**：全部时序数据（指标/趋势/SLO）的存储后端，替代内置内存存储作为长期历史
- **集成点**：`cmd/server/vm.go` 实现 remote-write 写入；`docker-compose.yml` 默认使用 `swr.cn-east-3.myhuaweicloud.com/sreyun/victoria-metrics:latest`
- **配置**：通过 `AIOPS_VM_URL` 环境变量配置地址，服务端启动时强制要求 PG + VM 二者齐全，缺一拒绝启动
- **保留策略**：默认 retentionPeriod=36（36个月），可按需调整