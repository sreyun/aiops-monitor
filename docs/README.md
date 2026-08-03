# AIOps 文档中心 · Documentation

> 根目录 [README.md](../README.md) 面向快速了解与试用；完整说明按主题分目录存放。

## 目录结构

```text
docs/
├── README.md                 # 本索引
├── getting-started/          # 安装与生产部署
│   ├── install.md / install.en.md
│   └── deploy.md  / deploy.en.md
├── guides/                   # 使用与专项能力
│   ├── user-guide.md
│   ├── forward.md
│   └── content-audit.md
└── engineering/              # 工程门禁与验收
    ├── ci-gate.md
    └── year1-acceptance.md
```

## 多语言 README · Localized READMEs

| Language | File |
|----------|------|
| 简体中文 | [../README.md](../README.md) |
| 繁體中文 | [../README.zh-TW.md](../README.zh-TW.md) |
| English | [../README_EN.md](../README_EN.md) |
| 日本語 | [../README.ja.md](../README.ja.md) |
| 한국어 | [../README.ko.md](../README.ko.md) |
| Français | [../README.fr.md](../README.fr.md) |
| Deutsch | [../README.de.md](../README.de.md) |
| Español | [../README.es.md](../README.es.md) |
| Português (Brasil) | [../README.pt-BR.md](../README.pt-BR.md) |
| Русский | [../README.ru.md](../README.ru.md) |

## 快速入口

| 文档 | 说明 |
|------|------|
| [../README.md](../README.md) | 产品简介、核心能力、3 分钟上手（中文） |
| [../README_EN.md](../README_EN.md) | Product overview & quick start (English) |
| [../CHANGELOG.md](../CHANGELOG.md) | 版本变更记录 |

## 入门 · Getting started

| 文档 | 语言 |
|------|------|
| [getting-started/install.md](./getting-started/install.md) | 中文 · 安装速查 |
| [getting-started/install.en.md](./getting-started/install.en.md) | English · Install |
| [getting-started/deploy.md](./getting-started/deploy.md) | 中文 · 生产部署 / 容灾 / 备份 |
| [getting-started/deploy.en.md](./getting-started/deploy.en.md) | English · Production deploy |

## 使用指南 · Guides

| 文档 | 说明 |
|------|------|
| [guides/user-guide.md](./guides/user-guide.md) | 完整安装使用说明书 |
| [guides/forward.md](./guides/forward.md) | 端口转发 / HTTP 反向代理 |
| [guides/content-audit.md](./guides/content-audit.md) | 内容审计、Agent 安装与剧本专家指南 |

## 工程 · Engineering

| 文档 | 说明 |
|------|------|
| [engineering/ci-gate.md](./engineering/ci-gate.md) | CI / SQL 变更 / 安全扫描门禁 |
| [engineering/year1-acceptance.md](./engineering/year1-acceptance.md) | 年度验收 / POC 清单（可选） |

## 文档约定

- **根目录**只保留：`README*.md`、`CHANGELOG.md`，以及旧路径**兼容跳转页**（`INSTALL.md` 等）。
- **正文**按 `getting-started/` · `guides/` · `engineering/` 分类存放；旧扁平路径（如 `docs/install.md`）保留跳转页。
- 链接失效时，优先查本索引；欢迎 PR 修正死链。
