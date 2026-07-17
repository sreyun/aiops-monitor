---
kind: external_dependency
name: PostgreSQL + pgvector - 关系数据存储与向量检索
slug: postgresql-pgvector
category: external_dependency
category_hints:
    - vendor_identity
scope:
    - '**'
---

### PostgreSQL + pgvector
- **角色**：全部关系数据（配置/用户/审计/事件/工单/会话/密钥）及 RAG 诊断向量的持久化存储
- **集成点**：`cmd/server/pgstore.go` 实现所有表操作；`docker-compose.yml` 使用 `pgvector/pgvector:pg18` 镜像
- **关键特性**：启用 pgvector 扩展用于 AI 诊断相似案例检索，向量维度由 `ai.embed_dimensions` 配置决定（默认 1536）
- **部署约束**：Windows/Mac 下建议使用 Docker 具名卷而非绑定挂载，避免 fsync 不可靠导致 WAL 损坏