---
kind: dependency_management
name: Go/Python 多语言依赖声明与最小化策略
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

本仓库采用多语言、多模块的依赖管理方式，核心 Go 服务与 Python 插件各自维护独立的依赖清单，未使用 vendor 目录或私有代理。

## 1. 使用的系统与工具
- Go 模块：go.mod + go.sum（Go 1.22），通过 require 显式声明第三方包及精确版本（含 commit hash）。
- Python 插件：plugins/requirements.txt 声明可选依赖，由运行环境自行安装。
- Android (Kotlin)：通过 Gradle (android/build.gradle, android/app/build.gradle) 管理 Android 库依赖，不在本仓库中集中声明。
- 无 vendor 目录：vendor/ 为空，构建时直接从网络拉取依赖。
- 无 GOPROXY/GOPRIVATE 配置：仓库内未发现任何 Go 代理或私有仓库环境变量设置。

## 2. 关键文件
- go.mod — Go 模块根定义，仅引入 3 个第三方包：lib/pq（PostgreSQL 驱动）、skip2/go-qrcode（二维码生成）、ledongthuc/pdf（PDF 导出）。
- go.sum — 对应三个包的校验和，版本均为带 commit hash 的 pseudo-version，保证可重现构建。
- plugins/requirements.txt — 插件层可选依赖 psutil>=5.9，注释说明 Linux 下可由 Go 核心原生采集替代。
- android/build.gradle / android/app/build.gradle — Android/Kotlin 依赖声明入口。

## 3. 架构与约定
- 极小依赖面：Go 服务端仅依赖数据库驱动、二维码与 PDF 两个辅助库，业务逻辑全部自实现，避免引入重型框架。
- 伪版本号锁定：所有依赖均使用 v0.0.0-YYYYMMDDHHMMSS-<commit> 形式的 pseudo-version，而非语义化标签，确保每次构建固定到具体提交。
- 插件解耦：Python 插件作为独立子项目，通过 requirements.txt 声明可选依赖，不侵入主 Go 模块。
- 无 vendoring：依赖在 CI/本地直接下载，未将第三方源码纳入版本控制。

## 4. 开发者应遵循的规则
- 新增 Go 依赖必须通过 go mod tidy 更新 go.mod 与 go.sum，禁止手动编辑。
- 优先选择轻量级、功能单一的库，保持 Go 二进制体积可控。
- 如需引入私有 Go 模块，应在构建环境设置 GOPRIVATE 与 GONOSUMDB，而非写入仓库。
- Python 插件新增依赖需同步更新 plugins/requirements.txt 并确认对 Linux 原生采集路径的影响。
- Android 依赖变更在 android/ 下的 Gradle 文件中完成，注意与服务器端依赖解耦。