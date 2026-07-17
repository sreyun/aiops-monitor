---
kind: dependency_management
name: 多语言依赖管理策略（Go/Python/Android）
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
    - plugins/requirements.txt
    - android/build.gradle
    - android/app/build.gradle
---

本仓库为多语言混合项目，各子模块采用各自生态的依赖声明方式，未使用统一的依赖锁定或私有注册表机制。

**Go 后端（Agent + Server）**
- 使用 go.mod + go.sum 进行依赖声明与版本锁定，Go 版本固定为 1.22。
- 当前仅引入 3 个第三方库：github.com/lib/pq（PostgreSQL 驱动）、github.com/ledongthuc/pdf（PDF 生成）、github.com/skip2/go-qrcode（二维码）。
- 未启用 Go Modules vendor 模式（vendor/ 目录为空），也未配置 GOPROXY、GOPRIVATE、GONOSUMCHECK 等环境变量，构建时直接从官方代理或源拉取。
- 无 go.work 多模块工作区文件，整个仓库视为单一 module aiops-monitor。

**Python 插件层**
- 位于 plugins/requirements.txt，仅声明可选依赖 psutil>=5.9，用于非 Linux 平台的基础指标采集兜底。
- 核心指标采集由 Go 原生实现，psutil 仅在需要时安装，属于软依赖。
- 未使用虚拟环境配置文件（如 .venv、poetry.lock、Pipfile.lock），依赖版本通过 >= 宽松约束。

**Android 客户端**
- 使用 Gradle + Kotlin DSL，顶层 android/build.gradle 声明 Android Gradle Plugin 8.5.2 与 Kotlin 1.9.24。
- 应用级 android/app/build.gradle 中集中声明所有依赖，包括 Compose BOM 2024.09.00、Retrofit 2.11.0、OkHttp 4.12.0、Navigation Compose 2.8.0 等。
- 仓库源指向 google() 与 mavenCentral()，未配置私有 Maven 仓库或 Nexus。
- 未使用 Gradle Version Catalog 或 dependency-conventions 等高级特性，依赖版本直接硬编码在 build.gradle 中。

**通用约定**
- 各语言子项目的依赖版本均显式锁定（Go 通过 go.sum，Android 通过具体版本号），但 Python 侧使用宽松范围。
- 未发现统一的依赖更新自动化脚本或 CI 中的依赖扫描任务。