---
kind: build_system
name: 多语言 Go/Android 混合构建与 CI/CD 流水线
category: build_system
scope:
    - '**'
source_files:
    - go.mod
    - docker/Dockerfile
    - .github/workflows/release.yml
    - docker-compose.yml
    - build.ps1
    - deploy.sh
    - android/build.gradle
    - plugins/requirements.txt
---

## 1. 构建系统与工具链

- Go 后端（Server + Agent）：Go 1.22，模块 aiops-monitor，使用 -mod=vendor 依赖管理；通过 ldflags -X main.appVersion=${VERSION} 注入版本号。
- Docker 多阶段镜像：docker/Dockerfile 定义 builder → server / agent 两个目标，基于 Alpine 3.20，CGO 关闭以纯静态二进制分发。
- CI/CD：GitHub Actions .github/workflows/release.yml 在推送 v* 标签时触发，使用 buildx + QEMU 构建 linux/amd64,linux/arm64 双架构镜像，推送到华为云 SWR（swr.cn-east-3.myhuaweicloud.com/sreyun/aiops-server、aiops-agent）。
- 本地开发：docker-compose.yml 编排 Server + VictoriaMetrics + PostgreSQL + 可选 Agent，默认拉取 SWR 预构建镜像，也可 --build 本地编译。
- Windows 本地构建：build.ps1 自动读取 Git tag 作为版本，输出 bin/aiops-server.exe、bin/aiops-agent.exe，支持 -CrossCompile 生成 Linux/macOS 二进制。
- Android 移动端：Gradle (AGP 8.5.2 + Kotlin 1.9.24)，独立于 Go 工程，位于 android/ 子目录。
- Python 插件生态：plugins/requirements.txt 声明 psutil 等可选依赖，随镜像打包为 plugins.zip 由 Server 分发。

## 2. 关键文件与包

- go.mod, go.sum：模块名、Go 版本、依赖清单
- docker/Dockerfile, docker/Dockerfile.dev：多阶段构建、多平台交叉编译、插件打包
- .github/workflows/release.yml：打 tag 触发多架构镜像构建并推送 SWR
- docker-compose.yml：一键拉起 Server + VM + PG + Agent
- build.ps1：本地/交叉编译，注入版本号
- deploy.sh：SCP + systemctl 替换二进制并重启服务
- android/build.gradle, android/app/build.gradle：AGP/Kotlin 插件声明
- plugins/requirements.txt：Python 插件运行时依赖

## 3. 架构与约定

- 版本注入统一入口：所有产物（Docker 镜像、Windows 二进制）均通过 -X main.appVersion=${VERSION} 注入同一版本号，来源为 Git tag 或 CI 传入的 VERSION 参数。
- 多架构策略：Docker 中 TARGETOS/TARGETARCH 由 buildx 自动提供，在 builder 阶段对当前目标平台执行原生交叉编译，避免 QEMU 模拟 Go 编译。同时一次性产出所有平台的 agent 二进制（linux-amd64/arm64、darwin-amd64/arm64、windows-amd64），打包进 /app/dist/plugins.zip 供 Server 的 /dl/ 端点下发安装。
- 依赖隔离：Go 使用 vendor 目录，Dockerfile 显式 COPY vendor/ 并以 GOFLAGS=-mod=vendor 构建，保证离线可复现。
- 镜像分层：server 目标仅含二进制与 dist 包；agent 目标额外安装 python3 + pip + psutil，满足插件运行环境。
- Compose 数据持久化：PostgreSQL 使用具名卷 aiops-pgdata 避免 Windows/Mac 绑定挂载 fsync 问题；VM 数据映射到 ./vm-data；终端录制与 TLS 证书映射到 ./data。
- 安全基线：Alpine 3.20 + Go 1.22（注释中已标注 EOL 风险，建议升级至 1.26+）；生产需修改默认密码与 AIOPS_SECRET_KEY。

## 4. 开发者应遵循的规则

1. 版本管理：发布前打 vX.Y.Z 标签，CI 会自动构建并推送对应镜像；本地开发可用 git describe --tags 生成的 dev 版本。
2. 新增 Go 依赖：更新 go.mod 后执行 go mod vendor，确保 vendor/ 同步，否则 Docker 构建会失败。
3. 变更 Dockerfile：若修改了构建命令或基础镜像，务必验证 linux/amd64 与 linux/arm64 均可正常构建。
4. Agent 跨平台产物：如需新增目标平台，需在 docker/Dockerfile 的 all-platform agent binaries 段追加对应 GOOS/GOARCH 构建行。
5. 环境变量配置：通过 docker-compose.yml 的 environment 字段覆盖 AIOPS_* 变量（如 DSN、TLS、SECRET_KEY），不要硬编码进镜像。
6. Android 构建：在 android/ 目录下使用 Gradle Wrapper 构建，不依赖根级构建脚本。
7. 插件扩展：新增 Python 插件时在 plugins/ 下添加文件，并在 requirements.txt 声明依赖；重新构建镜像会自动打包进 plugins.zip。
8. 远程部署：使用 deploy.sh 前需配置 SSH 免密登录到目标主机，脚本会按上传→停止旧进程→替换→启动新进程四步完成零停机更新。