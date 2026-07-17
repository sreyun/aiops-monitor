---
kind: dependency_management
name: 多语言依赖管理（Go vendor + Android Gradle + Python requirements）
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - docker/Dockerfile
    - plugins/requirements.txt
    - android/build.gradle
    - android/app/build.gradle
---

本仓库为多语言 AIOps 监控平台，涉及 Go、Android(Kotlin)、Python 三类依赖，各自采用不同的声明与解析策略：

1. **Go 模块（核心后端）**
   - 使用 `go.mod` + `go.sum` 声明依赖，当前仅引入 lib/pq、ledongthuc/pdf、skip2/go-qrcode 三个第三方包。
   - 构建时强制使用 `-mod=vendor`，要求 `vendor/` 目录存在；但当前 `vendor/` 为空，说明本地未执行 `go mod vendor`，Docker 构建阶段会因缺少 vendor 而失败，需先运行 `go mod vendor` 生成快照。
   - 未发现 GOPROXY / GOPRIVATE 等代理或私有源配置，默认走 golang.org 官方网络。
   - Dockerfile 中通过 `BASE_REGISTRY` 参数将基础镜像源切换至华为 SWR 加速，但 Go 模块下载仍走默认代理。

2. **Android（Kotlin/Compose）**
   - 使用 Gradle + Kotlin DSL，顶层 `android/build.gradle` 声明 google()、mavenCentral() 两个仓库。
   - 应用级 `android/app/build.gradle` 集中声明所有实现依赖，包括 Compose BOM、Retrofit、OkHttp、DataStore 等，版本集中在文件内，未见统一的 version catalog。
   - 无私有 Maven 仓库或 Nexus/JFrog 配置。

3. **Python 插件层**
   - `plugins/requirements.txt` 声明可选依赖 psutil>=5.9，用于非 Linux 平台的进程指标采集。
   - Agent 的 Docker 镜像在启动时通过 `pip3 install --no-cache-dir --break-system-packages psutil` 动态安装，而非构建期锁定，属于“运行时按需安装”模式。

4. **构建与发布产物**
   - `bin/` 下预编译了各平台二进制（aiops-server、aiops-agent 及其多架构变体），作为离线分发渠道，不依赖外部依赖管理器。
   - `deploy.sh`、`docker-compose.yml`、`docker/Dockerfile` 负责打包与部署，其中 Dockerfile 同时产出 server 与 agent 镜像，并将 plugins 目录 zip 后嵌入 server 镜像。

开发者约定与建议：
- 新增 Go 依赖后必须执行 `go mod tidy && go mod vendor`，确保 vendor 目录与 go.mod 同步，否则 Docker 构建会失败。
- 建议统一设置 GOPROXY（如 `https://goproxy.cn`）并在 CI 中缓存 vendor，避免网络波动影响构建。
- Android 依赖可考虑迁移到 version catalog（gradle/libs.versions.toml）以统一管理版本。
- Python 插件依赖建议在构建期用 pip freeze 生成锁定文件，或在 Dockerfile 中固定版本号，避免运行时安装差异。