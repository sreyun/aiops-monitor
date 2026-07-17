---
kind: dependency_management
name: 多语言依赖声明与 Vendor 策略
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
    - plugins/requirements.txt
    - docker/Dockerfile
    - android/app/build.gradle
    - android/build.gradle
---

本仓库为 Go + Python + Android 的多语言项目，各子系统的依赖管理方式如下：

**Go 后端（Server/Agent）**
- 使用 `go.mod` + `go.sum` 进行版本声明，模块名为 `aiops-monitor`，Go 版本锁定为 1.22。
- 当前仅引入少量第三方包：`github.com/lib/pq`（PostgreSQL 驱动）、`github.com/skip2/go-qrcode`、`github.com/ledongthuc/pdf`。
- 构建时通过 `GOFLAGS=-mod=vendor` 启用 vendor 模式，但根目录 `vendor/` 为空，实际构建依赖网络拉取；Dockerfile 中同时 COPY `vendor/` 作为可选项，说明团队有意在离线/私有镜像环境中预置 vendor 目录。
- Docker 构建默认使用华为云 SWR 镜像源（`swr.cn-east-3.myhuaweicloud.com/sreyun/docker.io`），未配置 `GOPROXY` 或 `GOPRIVATE`，依赖下载走公网或企业代理环境。
- 二进制产物直接输出到 `bin/` 和 `dist/`，无 go:generate 或自动化更新脚本。

**Python 插件层**
- 位于 `plugins/`，可选依赖集中在 `plugins/requirements.txt`，仅声明 `psutil>=5.9`。
- Agent 容器镜像中通过 `pip3 install --no-cache-dir --break-system-packages psutil` 在安装阶段按需安装，不强制所有平台都具备该依赖。
- 插件以纯 Python 脚本形式被 Go agent 动态加载，无独立虚拟环境或 pipenv/poetry 管理。

**Android 客户端**
- 基于 Gradle（`android/build.gradle`、`app/build.gradle`、`settings.gradle`）管理 Kotlin/Java 依赖，未包含 `gradle/libs.versions.toml` 等集中式版本目录。
- 使用系统 Gradle Wrapper（`gradle/wrapper/gradle-wrapper.properties`）。

**约定与建议**
- 新增 Go 依赖后应同步提交 `go.sum`，并在需要离线构建时将 `go mod vendor` 产物纳入仓库。
- 建议统一设置 `GOPROXY`（如 `https://goproxy.cn,direct`）并配置 `GOPRIVATE` 以支持私有模块。
- Python 插件如需更多依赖，应在 `plugins/requirements.txt` 中集中声明，避免在 Dockerfile 中硬编码 `pip install`。