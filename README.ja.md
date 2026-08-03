<div align="center">

# AIOps

**オープンソースのセルフホスト型ホスト監視 & SRE プラットフォーム**  
観測 · アラート · 自動修復 · リモート運用 · AI 診断 — 完全に自分で制御できる 1 バイナリへ。

[![Version](https://img.shields.io/badge/Version-v0.19.59-blue)](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.59)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Stars](https://img.shields.io/github/stars/sreyun/aiops-monitor?style=social)](https://github.com/sreyun/aiops-monitor)

**[简体中文](README.md) · [繁體中文](README.zh-TW.md) · [English](README_EN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Español](README.es.md) · [Português](README.pt-BR.md) · [Русский](README.ru.md)**

[クイックスタート](#-クイックスタート) · [コア機能](#-コア機能) · [ドキュメント](docs/README.md) · [変更履歴](CHANGELOG.md) · [Releases](https://github.com/sreyun/aiops-monitor/releases)

</div>

---

## なぜ AIOps か

監視・アラート・Bastion・Runbook が別々に増え、商用製品はホスト課金でデータはクラウド側に残りがちです。

AIOps はよく使う経路を **1 つのセルフホスト基盤** にまとめます：

| | AIOps | 典型的な寄せ集め |
|---|---|---|
| **構成** | Go サーバー 1 + 依存ゼロ Agent 1 | Zabbix / Prometheus / Grafana / Alertmanager / Bastion / Runbook… |
| **導入** | `docker compose up -d`（約 3 分） | 連携に数日 |
| **データ** | PostgreSQL + VictoriaMetrics（**自社保持**） | SaaS や分散 DB |
| **リモート** | Web 端末／デスクトップ／ポート転送、Agent **外向きのみ** | 別途 VPN／Bastion |
| **ループ** | アラート → Playbook → インシデント／SLO／チケット → AI RCA | 人手でつなぐ |
| **ライセンス** | **MIT**、ホスト数制限なし | ノード／モジュール課金 |

> プライベート DC・ハイブリッドクラウド、可視化・制御・変更安全・説明可能な運用を求めるチーム向け。

---

## ✨ コア機能

機能の羅列ではなく、6 本の柱：

```
  Observe ──────► Govern ──────► Remediate ──────► Diagnose
  Hosts/GPU/logs   Silence/route   Playbooks/gates   AI · RAG · MCP
  Probes/OOB       Multi-channel   Incident/SLO      Evidence gate

  Remote · terminal/desktop/forward (reverse tunnel)   Security · RBAC/MFA/FIM
```

1. **観測** — クロスプラットフォーム Agent（Linux／Windows／macOS／Kylin）、GPU、ログ、HTTP／TCP プローブ、API SLI、Redfish／SNMP／NetFlow／コンテナ／K8s／Hyper-V。
2. **ガバナンス** — 閾値プリセット、silence／inhibit／route；Feishu／DingTalk／メール／SMS／音声。
3. **修復 & SRE** — 承認ガード付き Playbook；インシデント、SLO、チケット、凍結ウィンドウ、監査付き break-glass。
4. **AI 診断** — 点検＋RCA（OpenAI 互換、未設定時はヒューリスティック）；pgvector RAG、Skills、MCP（Cursor／Claude）；音声セルフテスト。
5. **リモート運用** — Web 端末（再生／観戦／監査／二次パスワード）、リモートデスクトップ（JPEG／H.264）、ポート転送／HTTP プロキシと SSRF 防御。
6. **セキュアな提供** — RBAC、MFA、Agent 指紋、AES-256-GCM；Android／HarmonyOS は別配布。

現行リリース **[v0.19.59](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.59)** · [GitHub](https://github.com/sreyun/aiops-monitor)／[Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## 🚀 クイックスタート

> サーバーは PostgreSQL と VictoriaMetrics の**両方が必須**です。

```bash
docker compose up -d
# open http://localhost:8529 → finish first-time security setup
# copy the Agent install command from the UI onto each host
```

```bash
export AIOPS_POSTGRES_DSN="postgres://aiops:secret@127.0.0.1:5432/aiops?sslmode=disable"
export AIOPS_VM_URL="http://127.0.0.1:8428"
./aiops-server

go build ./cmd/server ./cmd/agent   # Go 1.26+
```

詳細インストール → **[docs/getting-started/install.en.md](docs/getting-started/install.en.md)** · 本番 → **[docs/getting-started/deploy.en.md](docs/getting-started/deploy.en.md)**

---

## 🏗 アーキテクチャ

```mermaid
flowchart LR
  subgraph Clients
    Web[Web Console]
    Mob[Android / HarmonyOS]
  end
  subgraph Server
    API[HTTP · WS · MCP]
    Core[Alerts · Playbooks · SRE · AI]
    PG[(PostgreSQL)]
    VM[(VictoriaMetrics)]
  end
  subgraph Fleet
    Ag[Agents]
    Ext[BMC · switches · storage]
  end
  Web --> API
  Mob --> API
  API --> Core
  Core --> PG
  Core --> VM
  Ag -->|outbound report / terminal| API
  Ag --> Ext
```

---

## 📚 ドキュメント

長文は [`docs/`](docs/README.md) に集約。ルートの旧ファイル名は**リダイレクト**として残しています。

| Need | Doc |
|------|-----|
| Install | [docs/getting-started/install.md](docs/getting-started/install.md) · [EN](docs/getting-started/install.en.md) |
| Production deploy | [docs/getting-started/deploy.md](docs/getting-started/deploy.md) · [EN](docs/getting-started/deploy.en.md) |
| End-user guide | [docs/guides/user-guide.md](docs/guides/user-guide.md) |
| Port forward | [docs/guides/forward.md](docs/guides/forward.md) |
| Content audit / playbooks | [docs/guides/content-audit.md](docs/guides/content-audit.md) |
| CI / SQL gates | [docs/engineering/ci-gate.md](docs/engineering/ci-gate.md) |

---

## 🤝 貢献

Issue／PR／翻訳を歓迎します。目安：`make build` · `make audit`。

AIOps が寄せ集めスタックを置き換えたら、**ぜひ Star** をお願いします。

---

## ライセンス

[MIT](LICENSE)。ホスト数制限なし。モバイルは別パッケージ（本リポジトリにソースなし）。

---

<p align="center">
  <b>AIOps · 運用の複雑さを、自分で所有する基盤へ。</b><br/>
  <sub>Star ⭐ · Fork · Issue · セルフホスト運用を一緒に</sub>
</p>
