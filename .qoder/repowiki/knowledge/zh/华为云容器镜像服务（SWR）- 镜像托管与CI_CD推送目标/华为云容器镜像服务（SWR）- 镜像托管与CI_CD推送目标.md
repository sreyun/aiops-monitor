---
kind: external_dependency
name: 华为云容器镜像服务（SWR）- 镜像托管与CI/CD推送目标
slug: huawei-swr
category: external_dependency
category_hints:
    - vendor_identity
    - client_constraint
scope:
    - '**'
---

### 华为云 SWR
- **角色**：项目 Docker 镜像的官方托管仓库，所有预构建产物（aiops-server、aiops-agent、victoria-metrics、postgres）均推送至此
- **集成点**：`docker/Dockerfile` 默认 `BASE_REGISTRY=swr.cn-east-3.myhuaweicloud.com/sreyun/docker.io`；`.github/workflows/release.yml` 在 push v* 标签时自动构建多架构镜像并推送
- **认证方式**：通过 HMAC-SHA256 从 AK/SK 动态生成登录密码，用户名格式为 `cn-east-3@<AK>`
- **约束**：SWR 不支持 OCI manifest 格式，需强制使用 `outputs: type=registry,oci-mediatypes=false` 以兼容 Docker manifest v2 schema2
- **镜像策略**：aiops-server/agent 支持 linux/amd64 + linux/arm64 双架构；postgres/victoria-metrics 仅同步了 amd64，ARM64 用户需配置 Docker 镜像加速器或使用 docker.io 官方镜像