---
kind: dependency_management
name: Go 模块与 Python 插件依赖管理
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
    - docker/Dockerfile
    - plugins/requirements.txt
---

## 1. 使用的系统与策略
- Go 侧：采用 **Go Modules**（`go.mod` + `go.sum`）声明依赖，并通过 **vendor 目录** 进行本地缓存。Docker 构建阶段强制使用 `-mod=vendor`，确保镜像内不联网拉取第三方包。
- Python 侧：插件层通过 `plugins/requirements.txt` 声明可选依赖（psutil），在 Agent 容器镜像中通过 `pip3 install` 安装。
- 基础镜像源：默认使用华为云 SWR 镜像仓库作为 Go/Alpine 基础镜像的代理，CI 可通过 `BASE_REGISTRY=docker.io` 切换回官方源。

## 2. 关键文件与位置
- `go.mod` / `go.sum`：Go 单模块根依赖清单，当前仅引入 `github.com/lib/pq`、`github.com/ledongthuc/pdf`、`github.com/skip2/go-qrcode` 三个第三方库。
- `vendor/`：Go 依赖的 vendored 副本（当前为空，说明尚未执行 `go mod vendor`）。
- `docker/Dockerfile`：多阶段构建脚本，统一注入 `GOFLAGS=-mod=vendor`，并负责打包 plugins.zip 供 Server 分发。
- `plugins/requirements.txt`：Python 插件可选依赖声明，核心指标兜底逻辑会用到 psutil。

## 3. 架构与约定
- **单一 Go 模块**：整个仓库只有一个 module `aiops-monitor`，Server 与 Agent 同属一个模块，避免跨模块版本不一致问题。
- **Vendor 优先构建**：Dockerfile 所有 `go build` 均带 `GOFLAGS=-mod=vendor`，保证离线可复现；若 vendor 缺失则构建失败，从而强制开发者先提交 vendor 快照。
- **CGO 关闭**：所有二进制均以 `CGO_ENABLED=0` 静态编译，消除 C 依赖带来的平台差异。
- **Python 依赖最小化**：仅在 Agent 镜像中安装 psutil，且使用 `--break-system-packages` 绕过 pip 保护，便于在多阶段镜像中直接写入系统环境。
- **基础镜像私有化**：默认 `BASE_REGISTRY=swr.cn-east-3.myhuicloud.com/sreyun/docker.io`，国内网络下加速拉取 golang:alpine 与 alpine:3.20。

## 4. 开发者应遵循的规则
1. 新增 Go 依赖后必须执行 `go mod tidy && go mod vendor`，并将生成的 `vendor/` 与 `go.sum` 一起提交到 Git，否则 Docker 构建会失败。
2. 不要修改 `docker/Dockerfile` 中的 `GOFLAGS=-mod=vendor` 标志，这是保证构建可复现的关键约束。
3. 升级 Go 版本时同步更新 `go.mod` 的 `go 1.22` 行以及 Dockerfile 中 `GO_VERSION` 默认值，并注意注释中关于 Go 1.22 EOL 的安全提醒。
4. 为 Python 插件添加新依赖时，请在 `plugins/requirements.txt` 中声明版本号范围（如 `psutil>=5.9`），并在 Agent 镜像的 `pip3 install` 命令中保持一致。
5. 如需切换基础镜像源，通过 `--build-arg BASE_REGISTRY=docker.io` 传入，而非硬编码修改 Dockerfile 默认值。