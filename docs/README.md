# AIOps 文档中心 · Documentation

> 仓库根目录只保留 [README.md](../README.md)（产品简介）与 [CHANGELOG.md](../CHANGELOG.md)；完整说明按主题存放在本目录。

## 目录结构

```text
/
├── README.md                 # 产品简介（简体中文）
├── CHANGELOG.md
└── docs/
    ├── README.md             # 本索引
    ├── i18n/                 # 多语言 README
    ├── getting-started/      # 安装与生产部署
    ├── guides/               # 使用与专项能力
    └── engineering/          # 工程门禁与验收
```

## 多语言 README · Localized READMEs

| Language | File |
|----------|------|
| 简体中文 | [../README.md](../README.md) |
| 繁體中文 | [i18n/zh-TW.md](./i18n/zh-TW.md) |
| English | [i18n/en.md](./i18n/en.md) |
| 日本語 | [i18n/ja.md](./i18n/ja.md) |
| 한국어 | [i18n/ko.md](./i18n/ko.md) |
| Français | [i18n/fr.md](./i18n/fr.md) |
| Deutsch | [i18n/de.md](./i18n/de.md) |
| Español | [i18n/es.md](./i18n/es.md) |
| Português (Brasil) | [i18n/pt-BR.md](./i18n/pt-BR.md) |
| Русский | [i18n/ru.md](./i18n/ru.md) |

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
| [engineering/agent-update-soak.md](./engineering/agent-update-soak.md) | Agent 热更新 / 终端权限浸泡清单 |
| [engineering/terminal-linux-privileges.md](./engineering/terminal-linux-privileges.md) | Linux 终端只读边缘（nsenter / allow-nonroot / 容器） |
| [engineering/year1-acceptance.md](./engineering/year1-acceptance.md) | 年度验收 / POC 清单（可选）；演示脚本 `scripts/demo-year1-loop.sh` |

## 文档约定

- **根目录**仅保留：`README.md`、`CHANGELOG.md`（外加代码 / 配置 / 许可证等非文档文件）。
- **多语言简介**统一放在 `docs/i18n/`；长文放在 `getting-started/` · `guides/` · `engineering/`。
- 不再保留旧路径跳转页，避免仓库文件列表被 stub 淹没。
