---
kind: build_system
name: 构建与制品管理（Go + Docker + GitHub Actions）
category: build_system
scope:
    - '**'
source_files:
    - go.mod
    - docker/Dockerfile
    - docker/Dockerfile.dev
    - docker-compose.yml
    - .github/workflows/release.yml
    - build.ps1
    - deploy.sh
    - plugins/plugin_sdk.py
---

## 1. 构建系统与工具链
- **语言/模块**：Go 1.22，使用 `go.mod` + `vendor/` 模式，构建时通过 `-mod=vendor` 锁定依赖。
- **二进制产物**：两个独立入口 `cmd/server`、`cmd/agent`，均通过 ldflags 注入 `main.appVersion` 版本号。
- **本地构建脚本**：`build.ps1`（PowerShell），支持本机与交叉编译（Linux/macOS），输出到 `bin/`。
- **容器镜像**：Dockerfile 采用多阶段构建，分 `server` 与 `agent` 两个 target；开发用 `docker/Dockerfile.dev` 跳过全平台 dist 构建以加速。
- **编排**：`docker-compose.yml` 一键拉起 server + VictoriaMetrics + PostgreSQL + 可选 Agent，默认使用华为云 SWR 镜像。
- **CI/CD**：`.github/workflows/release.yml` 在推送 `v*` 标签或手动触发时，用 buildx + QEMU 构建 linux/amd64 + linux/arm64 双架构镜像，推送到华为云 SWR。
- **部署脚本**：`deploy.sh` 通过 SSH + scp 将 Windows 版二进制上传并替换重启，适用于裸机部署场景。

## 2. 关键文件与位置
- `go.mod` / `go.sum` — Go 模块声明与依赖锁定
- `docker/Dockerfile` — 生产多阶段镜像（含全平台 agent dist 包）
- `docker/Dockerfile.dev` — 开发镜像（仅当前平台，更快）
- `docker-compose.yml` — 本地/测试环境一键编排
- `.github/workflows/release.yml` — Release CI（多架构 → SWR）
- `build.ps1` — Windows 本地构建 & 交叉编译脚本
- `deploy.sh` — 裸机远程部署脚本
- `plugins/` — Python 插件目录（Agent 侧动态加载）

## 3. 架构与约定
- **版本注入**：所有构建路径统一通过 `-X main.appVersion=${VERSION}` 注入，版本号来源为 Git tag（CI）或 `git describe`（本地脚本）。
- **CGO 关闭**：全部构建使用 `CGO_ENABLED=0`，确保静态链接、可移植。
- **多目标镜像**：同一 Dockerfile 通过 `target: server|agent` 产出不同镜像；同时内置全平台 agent 下载包（`/app/dist/`），供 Server `/dl/` 端点分发。
- **基础镜像源**：默认走华为云 SWR 镜像代理（`BASE_REGISTRY` 参数），CI 强制切回 `docker.io` 官方源以保证一致性。
- **依赖策略**：`go mod vendor` + `GOFLAGS=-mod=vendor`，构建完全离线可复现。
- **插件机制**：Agent 侧通过 Python 插件（`plugins/*.py` + `plugin_sdk.py`）扩展采集能力，Server 打包 `plugins.zip` 随 dist 下发。

## 4. 开发者应遵循的规则
- **新增入口**：在 `cmd/server` 或 `cmd/agent` 下新建子命令，保持单入口结构。
- **版本号**：不要硬编码版本，一律通过 ldflags 注入；本地可用 `build.ps1`，CI 由 tag 驱动。
- **依赖变更**：修改 `go.mod` 后执行 `go mod vendor`，提交 `vendor/` 快照，避免 CI 拉网失败。
- **跨平台构建**：优先使用 Dockerfile 的 `--platform` + buildx；Windows 本地可用 `build.ps1 -CrossCompile`。
- **插件扩展**：新增 Python 插件放在 `plugins/`，并在 `requirements.txt` 声明依赖；如需随 dist 分发，更新 Dockerfile 中 zip 步骤。
- **环境变量**：服务配置统一通过 `AIOPS_*` 前缀环境变量注入，参见 `docker-compose.yml` 中的示例。
- **安全注意**：Dockerfile 注释已指出 Go 1.22 EOL 及 CVE，发布前应升级至受支持的 Go 版本并同步 SWR 基础镜像。