---
kind: dependency_management
name: 多语言依赖管理：Go vendor + Python requirements + Docker 镜像内嵌
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
    - plugins/requirements.txt
    - docker/Dockerfile
    - .github/workflows/release.yml
---

## 1. 使用的系统与工具
- **Go 模块**：使用 `go.mod`/`go.sum` 声明依赖，构建时通过 `-mod=vendor` 启用本地 vendoring。
- **Python 插件**：使用 `plugins/requirements.txt` 声明可选依赖（psutil），在 Agent 镜像中通过 pip3 安装。
- **Docker 多阶段构建**：`docker/Dockerfile` 将 Go 二进制与 Python 插件打包进镜像，实现离线可部署。
- **CI/CD**：`.github/workflows/release.yml` 基于 GitHub Actions 触发多架构镜像构建并推送至华为云 SWR。

## 2. 关键文件与位置
- `go.mod` / `go.sum` — Go 依赖清单与校验和
- `plugins/requirements.txt` — Python 插件依赖（psutil）
- `docker/Dockerfile` — 多阶段构建、vendor 模式、插件打包
- `.github/workflows/release.yml` — 版本化发布流水线
- `vendor/` — Go vendor 目录（当前为空，需执行 `go mod vendor` 生成）

## 3. 架构与约定
- **Go 依赖锁定**：仅 3 个第三方包（lib/pq、ledongthuc/pdf、skip2/go-qrcode），版本固定到 commit SHA，避免漂移。
- **Vendor 优先**：Docker 构建强制 `GOFLAGS=-mod=vendor`，确保离线构建与网络不可用环境下的稳定性；README 明确标注“Docker 构建离线化（go mod vendor）”。
- **Python 依赖最小化**：仅在 Agent 镜像中安装 psutil，Server 镜像不装 Python；Linux 平台下核心指标由 Go 原生采集，psutil 为可选增强。
- **镜像分层**：builder 阶段交叉编译所有目标平台二进制，最终镜像只包含运行时所需内容，减小体积。
- **私有镜像源**：默认使用华为云 SWR 镜像加速（`swr.cn-east-3.myhuaweicloud.com/sreyun/docker.io`），CI 中覆盖为官方源以获取最新 golang:alpine。

## 4. 开发者应遵循的规则
- **新增 Go 依赖后必须执行**：`go mod tidy && go mod vendor`，并将生成的 `vendor/` 提交到仓库，否则 CI 构建会失败。
- **禁止直接修改 `go.sum`**：始终通过 `go get` 或编辑 `go.mod` 后 tidy 来更新依赖，保证校验和一致。
- **Python 插件依赖变更**：同步更新 `plugins/requirements.txt`，并在本地验证 Agent 镜像能正常安装。
- **不要引入 CGO 依赖**：Docker 构建使用 `CGO_ENABLED=0`，新增依赖应避免需要系统库的 C 扩展。
- **版本升级注意**：Dockerfile 注释指出 Go 1.22 已 EOL 且含 CVE，升级时需先在 SWR 同步新基础镜像再改默认值。
- **私有依赖**：如需引入内部 Go 包，需在 GOPROXY/GOPRIVATE 配置中声明（当前未配置，按需添加）。