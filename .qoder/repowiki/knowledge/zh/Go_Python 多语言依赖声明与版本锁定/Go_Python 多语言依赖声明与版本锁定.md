---
kind: dependency_management
name: Go/Python 多语言依赖声明与版本锁定
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
    - plugins/requirements.txt
    - android/app/build.gradle
    - android/build.gradle
    - docker/Dockerfile
---

本仓库采用多语言、多模块的依赖管理方式，各子项目各自维护独立的依赖清单：

1. **Go 核心（Server + Agent）**
   - 使用 Go Modules，根目录 `go.mod` 声明 module 为 `aiops-monitor`，Go 版本固定为 1.22。
   - 当前仅显式 require 三个第三方包：`github.com/lib/pq`（PostgreSQL 驱动）、`github.com/skip2/go-qrcode`（二维码生成）、`github.com/ledongthuc/pdf`（PDF 导出），其余均为标准库或内嵌代码。
   - 配套的 `go.sum` 对每个依赖同时记录源码哈希与 go.mod 哈希，确保可重现构建。
   - 未启用 vendor 目录（`vendor/` 为空），也未在 go.mod 中配置 GOPRIVATE / GONOSUMDB / proxy 等私有代理，依赖拉取完全依赖全局 Go 环境变量或网络可达性。
   - 所有依赖均使用精确 commit hash 形式的 pseudo-version（如 `v0.0.0-20200617195104-da1b6568686e`），而非语义化版本号，便于锁定到具体提交。

2. **Python 插件层**
   - `plugins/requirements.txt` 声明可选依赖 `psutil>=5.9`，用于非 Linux 平台的基础指标采集；Linux 上由 Go 原生采集，可不安装。
   - 无虚拟环境或 pipenv/poetry 锁文件，部署时需手动 `pip install -r plugins/requirements.txt`。

3. **Android 前端**
   - 基于 Gradle + Kotlin，依赖通过 `android/app/build.gradle` 及顶层 `build.gradle` 声明，未使用 BOM 或集中版本目录，依赖版本散落在各 build 脚本中。

4. **Docker 镜像**
   - `docker/Dockerfile` 以官方 `golang:1.22` 为基础镜像，在镜像内执行 `go mod download` 拉取依赖，不依赖宿主机 Go 环境。

**开发者约定**
- 新增 Go 依赖后需同步更新 `go.mod` 并检查 `go.sum` 是否被正确写入。
- Python 插件依赖统一维护在 `plugins/requirements.txt`，升级时注意兼容 psutil 最低版本 5.9。
- Android 依赖建议在顶层 `gradle/libs.versions.toml`（若引入）或统一 build 脚本中收敛版本，避免分散声明。