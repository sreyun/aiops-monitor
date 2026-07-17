---
kind: build_system
name: Go + Docker 多阶段构建与 CI/CD 发布流水线
category: build_system
scope:
    - '**'
source_files:
    - go.mod
    - build.ps1
    - docker/Dockerfile
    - docker/Dockerfile.dev
    - docker-compose.yml
    - .github/workflows/release.yml
    - deploy.sh
    - android/build.gradle
---

## 1. 构建系统与工具链

- **语言与依赖**：Go 1.22，使用 `go.mod` + `vendor/` 模式（`GOFLAGS=-mod=vendor`），所有第三方依赖预提交到 vendor 目录，保证离线可复现构建。
- **Android 客户端**：Gradle (AGP 8.5.2 + Kotlin 1.9.24)，独立在 `android/` 子项目，通过 `gradlew` 构建 APK。
- **容器化**：Docker 多阶段镜像，提供生产 `docker/Dockerfile` 与开发加速版 `docker/Dockerfile.dev`；Compose 编排 Server + VictoriaMetrics + PostgreSQL。
- **CI/CD**：GitHub Actions `.github/workflows/release.yml`，推送 `v*` tag 时自动触发多架构镜像构建并推送到华为云 SWR。

## 2. 核心构建入口

| 场景 | 入口文件 | 说明 |
|------|----------|------|
| Windows 本地开发 | `build.ps1` | 读取 Git tag 注入 `main.appVersion`，默认构建 server+agent，支持 `-CrossCompile` 生成 Linux/macOS 二进制 |
| 生产镜像构建 | `docker/Dockerfile` | 多阶段：builder 用 `--platform=$BUILDPLATFORM` 原生交叉编译，最终产物按 target 拆分 server/agent 镜像 |
| 开发镜像构建 | `docker/Dockerfile.dev` | 跳过 dist 全平台打包，仅构建当前目标平台，加速本地 `docker compose up --build` |
| 一键部署脚本 | `deploy.sh` | SSH 上传二进制、替换、systemctl 重启（面向 Windows 宿主机部署） |
| 本地服务编排 | `docker-compose.yml` | 定义 aiops-server / victoriametrics / postgres / aiops-agent 四服务，含健康检查与数据卷 |
| CI 发布流水线 | `.github/workflows/release.yml` | QEMU + buildx 构建 linux/amd64 + arm64，HMAC-SHA256 计算 SWR 登录密码，输出 GHA Step Summary |

## 3. 版本注入与产物命名约定

- **版本号来源**：优先取 `git describe --tags`，无 tag 则回退为 `dev-<short-commit>`。
- **注入方式**：`-ldflags="-s -w -X main.appVersion=<version>"`，server 与 agent 共享同一变量名。
- **二进制命名**：
  - 本机：`bin/aiops-server.exe`、`bin/aiops-agent.exe`
  - 交叉编译：`aiops-server-linux`、`aiops-agent-mac` 等
  - 镜像内分发包：`dist/aiops-agent-{os}-{arch}`（由 Dockerfile 一次性产出全部平台，供服务端 `/dl/` 下载）
- **镜像标签策略**：`swr.cn-east-3.myhuaweicloud.com/sreyun/aiops-server:<tag>`，可选同时打 `:latest`。

## 4. 多架构与交叉编译策略

- **Docker Buildx + QEMU**：CI 中先 `setup-qemu-action` 再 `setup-buildx-action`，以 `--platform=linux/amd64,linux/arm64` 并行构建。
- **CGO 关闭**：`CGO_ENABLED=0` 确保静态链接，避免依赖宿主 glibc 版本差异。
- **SWR 镜像源**：默认走华为云 SWR 代理的 docker.io 镜像（`BASE_REGISTRY=swr.cn-east-3.myhuaweicloud.com/sreyun/docker.io`），CI 强制切回官方源 `BASE_REGISTRY=docker.io` 以获得最新 Go 基础镜像。

## 5. Android 构建

- 顶层 `android/build.gradle` 声明 AGP/Kotlin 插件版本，具体模块构建由 `android/app/build.gradle` 管理。
- 未集成到主仓库的 CI 流程，需开发者本地使用 Android Studio 或 `./gradlew assembleRelease` 构建。

## 6. 开发者应遵循的规则

1. **版本号**：发布前打 `vX.Y.Z` tag，CI 会自动解析并注入到二进制；本地开发可用 `git tag v0.0.0-dev` 模拟。
2. **依赖更新**：新增/升级 Go 依赖后必须执行 `go mod tidy && go mod vendor`，保持 vendor 同步。
3. **跨平台构建**：新增目标平台需在 `docker/Dockerfile` 的 dist 构建段追加对应 `GOOS/GOARCH` 组合。
4. **敏感信息**：`AIOPS_SECRET_KEY`、PG 密码等通过环境变量注入，禁止硬编码进镜像或配置文件。
5. **本地调试**：优先使用 `docker compose up --build`，开发镜像已内置热重载友好的 Alpine + zip 环境。
6. **Windows 部署**：使用 `build.ps1 -CrossCompile` 生成 Linux 二进制后，再用 `deploy.sh` 远程部署。

## 7. 关键文件清单

- `go.mod` / `vendor/` — Go 依赖与供应商快照
- `build.ps1` — Windows 本地构建脚本
- `docker/Dockerfile` — 生产多阶段镜像（含全平台 agent 分发包）
- `docker/Dockerfile.dev` — 开发用快速镜像
- `docker-compose.yml` — 本地三服务编排
- `.github/workflows/release.yml` — 多架构镜像 CI 流水线
- `deploy.sh` — SSH 远程一键部署脚本
- `android/build.gradle` — Android Gradle 根构建配置
