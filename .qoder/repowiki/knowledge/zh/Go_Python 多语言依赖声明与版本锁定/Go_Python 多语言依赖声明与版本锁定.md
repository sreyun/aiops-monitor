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
    - docker/Dockerfile
---

本仓库采用“按组件拆分、各自管理”的依赖策略，涉及 Go 服务端/Agent、Python 插件层以及 Android 前端三个子系统：

1. **Go 模块（Server + Agent）**
   - 根目录 `go.mod` 使用 module `aiops-monitor`，Go 版本固定为 1.22。
   - 仅声明了 3 个第三方依赖：`github.com/lib/pq`（PostgreSQL 驱动）、`github.com/skip2/go-qrcode`（二维码生成）、`github.com/ledongthuc/pdf`（PDF 导出），均为极小依赖集。
   - 通过 `go.sum` 对每个依赖的 h1 哈希进行校验，确保构建可重复；未使用 vendor 目录（`vendor/` 为空）。
   - 未发现 GOPRIVATE / GONOSUMCHECK / GONOSUMDB / GONOPROXY 等私有代理配置，也未见自定义 go proxy 环境变量或 `.golangci.yml` 中的相关设置，表明当前依赖全部来自公共 Go 模块代理。

2. **Python 插件层**
   - `plugins/requirements.txt` 仅声明一个可选依赖 `psutil>=5.9`，用于非 Linux 平台的基础指标采集；Linux 下由 Go 核心原生采集，可不安装。
   - 无 `pipenv`、`poetry`、`Pipfile.lock` 等更严格的锁文件，仅以 `>=` 宽松约束为主。

3. **Android 前端**
   - 基于 Gradle/Kotlin，依赖声明位于 `android/app/build.gradle` 与 `android/build.gradle`，由 Gradle 负责解析与缓存，不在 Go/Python 体系内。

4. **Docker 构建**
   - `docker/Dockerfile` 中通过 `go mod download` 拉取依赖，未挂载本地 vendor 目录，依赖在镜像构建时从网络获取。

**开发者约定**
- Go 新增依赖需同步更新 `go.mod` 与 `go.sum`，禁止手动编辑 `go.sum`。
- Python 插件新增依赖请追加到 `plugins/requirements.txt`，并评估是否应改为严格版本号（如 `==x.y.z`）以保证可复现性。
- 若后续引入私有 Go 模块或私有 PyPI 源，需在 CI 环境中注入 `GOPROXY`/`GONOSUMDB`/`PIP_INDEX_URL` 等变量，并在仓库中补充相应文档。