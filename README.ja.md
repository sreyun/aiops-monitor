# AIOps

> **エンタープライズ級ホスト監視・SRE 運用プラットフォーム · 100% オープンソース · プライベート自ホスティング · データ永久自社保有**
>
> 1 つの Go バイナリ ＋ 依存ゼロの Agent で、メトリクス収集、インテリジェントアラート、リモートターミナル、自動自己修復から、SRE クローズドループ、AI 巡検診断、Android / HarmonyOS モバイルコンソールまでの運用全行程をカバー。PostgreSQL ＋ VictoriaMetrics の二重ストレージ、コマンド1つで導入、3 分で本稼働。
>
> **English:** AIOps is an open-source, self-hosted enterprise host-monitoring & SRE platform. One Go binary plus a zero-dependency agent covers the full ops loop — metrics, alerting, remote terminal, auto-remediation, SRE closure, AI diagnosis, and a native mobile console — on unified PostgreSQL + VictoriaMetrics storage. MIT licensed.

---

## 目次

- [プロジェクト概要](#プロジェクト概要)
- [機能特性](#機能特性)
- [インストール手順](#インストール手順)
- [使い方](#使い方)
- [設定パラメータ説明](#設定パラメータ説明)
- [技術アーキテクチャ概要](#技術アーキテクチャ概要)
- [コントリビューションガイドライン](#コントリビューションガイドライン)
- [ライセンス](#ライセンス)

---

## プロジェクト概要

AIOps は**エンタープライズ級ホスト監視・SRE 運用プラットフォーム**です。「Go ネイティブ収集 ＋ Python プラグイン層 ＋ リアルタイムパネル」のハイブリッドアーキテクチャを採用し、クロスプラットフォーム（Linux / Windows / macOS / 麒麟などの国産 OS）のメトリクス収集、GPU 監視、カスタム死活監視、リモートターミナル、自動化 Playbook、SRE ハブ（インシデント / 自動修復 / SLO / チケット）、ログ収集・検索、AI 巡検診断などの能力を提供します。

v5.5.0 以降、統一ストレージは **PostgreSQL（すべてのリレーショナルデータ）＋ VictoriaMetrics（すべての時系列データ）** となり、内蔵の `aiops.db` は廃止されました。新たに設定キーの AES-256-GCM 静止暗号化、オプションの TLS 転送暗号化、初回ログイン時の強制セキュリティ初期化、クロスプラットフォームの起動自動化と生存維持（保活）を搭載しています。

**対象ユーザー**

- **運用エンジニア・SRE**：ホストと業務の可用性監視、アラート治理、SLO とインシデントのクローズドループ、自動修復とチケットフロー。
- **プラットフォーム・開発**：API 監視、ポート転送と HTTP プロキシ、リモートターミナル、ログ検索、AI 補助診断。
- **セキュリティ・コンプライアンス**：RBAC、MFA、監査、SSRF 防御、TLS と静止暗号化。

**従来の監視手法との差別化優位性**

- 単一バイナリのサーバー ＋ 依存ゼロの Agent。3 プラットフォームのネイティブ収集と GPU 対応。
- Server-Agent 分離型デプロイ。Agent は**リバース接続**で入站ポートを開く必要なし。
- PG ＋ VM の統一ストレージで、リレーショナルと時系列のデータが各々の役割を果たす。
- 内蔵の AI 巡検と RAG 記憶庫により、pgvector で類似事例を検索。
- アラート治理（サイレンス / 抑制 / ルーティング）と統一メッセージセンターでノイズを低減し、操作性を向上。

> 位置付けとして、AIOps はあなたが完全に掌握する1つのプラットフォームで、可観測性、アラート、自動化、AI 診断、SRE クローズドループ、モバイルを統合し —— 分散した複数のツールの保守を減らすことを目指しています。

---

## 機能特性

### ホストとリソース監視

- **クロスプラットフォームのネイティブ収集**：Linux / Windows / macOS / 麒麟など。第三者依存ゼロ。
- **メトリクス項目**：CPU、メモリ、スワップ、ディスク、プロセス、ポート、ネットワーク、DiskIO、IOPS、GPU、負荷、連続稼働時間。
- **GPU 監視**：best-effort で VRAM、利用率、温度などを収集。
- **外部収集器（対象デバイスに Agent の導入不要）**：
  - **Redfish**：BMC / iDRAC / iLO をポーリングし、CPU / メモリ / ストレージ / 温度 / ファン / 電源 / ファームウェアを収集。
  - **NetFlow**：UDP でスイッチ / ファイアウォールの Flow（v5 / v9 テンプレート化）を受信。
  - **Huawei OceanStor**：DeviceManager REST でストレージ / ディスク筐体の健康状態を収集。
  - **SNMP**：独自実装のプロトコルスタック（v2c / v3 USM ＋ Trap）でスイッチ / ルーターを管理。
  - **5 タプルパケット**、**SNI / DNS と平文 HTTP の内容監査**（コンプライアンス制御可能、デフォルト無効）。
  - **コンテナインベントリ**（Docker / Podman を自動検出）、**Hyper-V** 仮想マシン収集。

### ログと可観測性

- **ログ収集**：増分 tail。オプションで gzip ＋ AES-256-GCM 暗号化して上報。
- **全文検索**：ログを集中保存し、検索可能。
- **メトリクストレンド**：VictoriaMetrics が担い、生精度を長期保持。必要に応じて集計クエリ可能で、長期履歴の遡及も容易。

### 死活監視と API 監視

- **カスタム死活監視**：Ping / TCP / HTTP / プロセス生存。
- **API 業務監視**：可用率 / P95 / スループット。分散した複数拠点からのプローブ（WebSocket プローブ）。
- 結果は統一して時系列 DB に書き込まれ、アラートと SLO 計算をトリガー可能。

### アラートと治理

- **しきい値エンジン**：CPU / メモリ / ディスク / IO / IOPS / GPU / 負荷 / プロセス / 接続数 / オフライン判定。
- **アラート治理**：重複排除、サイレンス（時間帯 / 曜日）、抑制（主原因が派生を抑制）、ルーティング（チャネル振り分け）。
- **マルチチャネル通知**：Feishu、DingTalk、メール、SMS、音声通話。
- **復旧通知**とアラートライフサイクル管理。

### リモートターミナルとポート転送

- **Web ターミナル**：ブラウザ ↔ サーバー ↔ Agent のリバースチャネル（wait / rx / tx）。マルチタブ、セッション録画再生、読み取り専用傍観、コマンド監査、二次パスワード認証。
- **ポート転送**：TCP / UDP マッピング、HTTP リバースプロキシ（`/proxy/{hostID}/{port}/{path}`、WebSocket 対応）。SSRF 送信防御付き、グローバル無効化可能。
- **マシン指紋認証**：Token のローテーションしても既存 Agent に影響なし。

### 自動化と SRE ハブ

- **自動化 Playbook**：複数ステップのコマンド。ターゲット選択（全部 / 分類 / システム / ホスト）、並行実行、履歴レポート。
- **自動修復**：ガードレール ＋ 承認フローで危険な操作を防止。
- **インシデント管理**：集約、クローズドループ、事例エクスポート。
- **SLO**：エラーバジェット管理。
- **チケットフロー**：インシデント / 自動修復と連動。
- **統一メッセージセンター**：インシデント / アラート / SLO / 自動修復 / AI / チケットを統合受信箱に。

### AI 運用能力

- **AI 巡検**：定期的に自動巡検し、レポートを生成。
- **根本原因研判**：コンテキストを組み合わせて診断の提案を提示。
- **RAG 記憶庫**：pgvector ベースの類似事例検索（diagnosis_embeddings / ai_memory_embeddings）。
- **自律 Agent**：Function Calling で運用アクションを実行。
- **AI Copilot / 音声**：対話型アシスタントと音声対話（TTS / STT）。

### セキュリティとコンプライアンス

- **認証**：セッション Cookie ＋ RBAC（admin / operator / viewer）。
- **MFA**：TOTP ワンタイムパスワード（単回使用）。
- **ターミナル二次パスワード**：レート制限 ＋ ロック期間。
- **Agent マシン指紋**：`X-Agent-Fingerprint`（machine-id ＋ MAC）。
- **インストールトークン**：ローテーション ＋ 7 日の猶予期間。
- **リレーキー**：`AIOPS_RELAY_SECRET` で送信元を検証。
- **静止暗号化**：設定キーを AES-256-GCM で暗号化。
- **転送暗号化**：オプションの TLS。初回ログイン時の強制セキュリティ初期化。

### モバイル端末

- **ネイティブ Android App**（Kotlin ＋ Jetpack Compose）：29 のナビゲーションルート / 20 以上の画面。SRE コックピット、ホスト詳細（ネイティブ Canvas チャート）、アラート（レベル / 状態の 2 軸フィルタ ＋ AI 診断）、エンタープライズ級 VT ターミナル、運用センター（インシデント / 承認 / SLO / チケット）、監視死活、AI アシスタント（SSE ストリーミング）、ハードウェア / NetFlow / Hyper-V、ターミナル再生、メッセージセンターなど。DataStore でセッションを永続化、MFA ポップアップ、自前の `/ws/push` 長接続でプッシュ通知。
- **HarmonyOS NEXT App**（ArkTS）：Android 側と「機能 / 情報アーキテクチャ / インタラクションのクローズドループ」を揃えたネイティブ HarmonyOS コンソール。

### デプロイと体験

- **Web パネル**：サーバー内蔵ダッシュボード。2 テーマ（ダーク / ライト）。3 言語切替（簡体中文 / 繁体中文 / English）。※ マーケティングサイト（website/）は 簡中 / 繁中 / English / 日本語 の 4 言語に対応。
- **マルチサーバーブロードキャスト**：1 回の収集を複数のサーバーへ並行上報（マルチDC 災害復旧）。
- **ゲートウェイ中継**：社内でインターネットに接続した 1 台のマシンがすべての上報をクラウドへ代理。

---

## インストール手順

> サーバーは PostgreSQL と VictoriaMetrics に**強依存**しており、いずれかが欠けても起動を拒否します。

### 方式一：Docker Compose（推奨）

```bash
# 1. 設定の準備（必要に応じて compose 内の環境変数 / パスワードを変更）
cp docker-compose.yml docker-compose.override.yml   # 任意

# 2. server + victoriametrics + postgres(pgvector) をワンクリック起動
docker compose up -d

# 3. http://localhost:8529 にアクセスし、案内に従って初回セキュリティ初期化を完了
```

イメージはデフォルトで Huawei Cloud SWR（`swr.cn-east-3.myhuaweicloud.com/sreyun/...`）を使用します。自构建イメージに置き換え可能です。

### 方式二：バイナリデプロイ

```bash
# サーバー：必須環境変数を設定して直接実行
export AIOPS_POSTGRES_DSN="postgres://aiops:パスワード@localhost:5432/aiops?sslmode=disable"
export AIOPS_VM_URL="http://localhost:8428"
export AIOPS_LISTEN=":8529"            # 任意、デフォルト :8529
./aiops-server

# Agent：設定をコピーして起動
cp config.example.yaml config.yaml
./aiops-agent --config config.yaml
```

### 方式三：ソースからのビルド

```bash
# Go 1.26+ が必要（go.mod を参照）
go build ./cmd/server ./cmd/agent

# またはリポジトリのスクリプトを使用（Windows は build.ps1、クロスコンパイル込み。Makefile はセキュリティゲート付き：vet/test/govulncheck/gosec/staticcheck/sbom）
make build          # Linux/macOS
./build.ps1         # Windows
```

インストール完了後、Web の「インストールコマンド」ページで Agent インストール指令（Token を自動注入）を生成し、対象ホストに貼り付けるだけで管理対象に。

より詳しいインストールと設定は [INSTALL.md](INSTALL.md) と [DEPLOY_GUIDE.md](DEPLOY_GUIDE.md) を参照（英語版は [INSTALL_EN.md](INSTALL_EN.md) / [DEPLOY_GUIDE_EN.md](DEPLOY_GUIDE_EN.md)）。

---

## 使い方

1. **初回ログイン**：`http://<サーバー>:8529` にアクセス。初回ログインでユーザー名 ＋ パスワードの変更を必須化。管理者アカウントへの **MFA** 有効化を推奨。
2. **ホストの管理**：「インストールコマンド」ページで指令を生成 → 対象ホストで実行 → Agent がリバース接続して自動登録。`category`（本番 / テスト / DB / オフィス）でグループ化可能。
3. **監視の設定**：
   - 「アラート」ページでしきい値と治理ルール（サイレンス / 抑制 / ルーティング）を設定。
   - 「死活監視」ページで Ping / TCP / HTTP / プロセス生存タスクを追加。「API 監視」ページで業務インタフェースを接続。
   - 「Playbook」ページで自動修復ステップを編成し、必要に応じて承認ガードレールを有効化。
4. **リモート運用**：ホストカードで「ターミナル」を開きリアルタイム排障（録画再生と二次パスワードに対応）。「転送」ページでポート転送 / HTTP プロキシルールを作成。
5. **SRE クローズドループ**：インシデント、SLO、チケットは「運用センター」で連動。AI 巡検と診断は「AI アシスタント」で確認。
6. **モバイル端末**：Android / HarmonyOS App をインストールし、自構築サーバーのアドレスとアカウントを入力。DingTalk / Feishu のようなプッシュは自前の `/ws/push` 長接続で届きます。
7. **外部デバイス**：Agent の `config.yaml` に `redfish_targets` / `oceanstor_targets` / `netflow` / `snmp` などを記入。対象デバイスに Agent を導入せずに管理対象に。

---

## 設定パラメータ説明

### サーバー（環境変数で上書き）

| 変数 | 説明 | 必須 |
|---|---|---|
| `AIOPS_POSTGRES_DSN` | PostgreSQL 接続文字列（リレーショナルデータ ＋ 監査 ＋ イベント ＋ チケット ＋ pgvector RAG） | はい |
| `AIOPS_VM_URL` | VictoriaMetrics アドレス（時系列データ） | はい |
| `AIOPS_LISTEN` | サーバー待受アドレス、デフォルト `:8529` | いいえ |
| `AIOPS_SECRET_KEY` | 設定キーの静止暗号化キー（AES-256-GCM） | いいえ（本番設定を推奨） |
| `AIOPS_TLS_CERT` / `AIOPS_TLS_KEY` | TLS 証明書 / 秘密鍵（HTTPS 有効化） | いいえ（本番設定を推奨） |
| `AIOPS_RELAY_SECRET` | リレー送信元検証キー | いいえ |
| `AIOPS_TERMINAL_DISABLED` | Web ターミナルをグローバル無効化 | いいえ |
| `AIOPS_ALLOW_ANONYMOUS_AGENTS` | 未認証 Agent の接入を許可（デバッグのみ） | いいえ |
| `AIOPS_TRUST_PROXY` | 前置リバースプロキシを信頼（X-Forwarded-*） | いいえ |
| `AIOPS_REQUIRE_TOKEN` | Agent にインストール Token の携帯を要求 | いいえ |
| `AIOPS_FORWARD_*` | ポート転送関連（待受アドレス / ポート範囲など） | いいえ |

サーバーにはさらに `cmd/server/config_example.yaml`（通知 webhook、しきい値の段階、分類、install_token、転送、アカウント、checks など）があります。

### Agent（`config.yaml` / `config.json`）

| グループ | 主要パラメータ | 説明 |
|---|---|---|
| 基本 | `server` / `token` / `category` | サーバーアドレス、インストール Token、ホスト分類タグ |
| 上報 | `report_interval` / `plugin_interval` | メトリクス上報間隔（デフォルト 30s）/ プラグイン周期（デフォルト 60s） |
| マルチサーバー | `servers[]` | 空でない場合、単一 server を**上書き**：1 回の収集をすべてのサーバーへ並行上報 |
| ログ | `log_paths` / `log_encrypt` | 収集パス（ワイルドカード対応）/ gzip+AES 暗号化上報の有無（デフォルト true） |
| TLS | `tls_skip_verify` / `ca_cert` | 証明書検証をスキップ（非安全）/ 自署名を検証する CA を指定 |
| リレー | `relay` / `listen` / `relay_secret` | ゲートウェイモード：社内マシンが本ゲートウェイ経由で上報 |
| ハードウェア | `redfish_targets` / `oceanstor_targets` | BMC / Huawei ストレージの帯外収集（対象に Agent 不要） |
| ネットワーク | `netflow` / `snmp` / `packet_capture` / `sni_dns_capture` | NetFlow / SNMP(v2c/v3+Trap) / 5 タプル / SNI-DNS と内容監査 |
| 仮想化 | `hyperv_*` / `container_*` | Hyper-V とコンテナインベントリ（通常は自動検出） |

パスワード系フィールドは一律、`*_env` 環境変数（例：`REDFISH_DELL_PASSWORD`）を優先して使用し、平文のディスク書き込みを避けてください。完全な例は [config.example.yaml](config.example.yaml) と [cmd/agent/config_example.yaml](cmd/agent/config_example.yaml) を参照。

---

## 技術アーキテクチャ概要

AIOps は **Server-Agent 分離**アーキテクチャを採用し、**Go ＋ Python のハイブリッド設計原則**を組み合わせます。高頻度で性能に敏感な基礎メトリクス収集は Go（単一バイナリ、依存ゼロ）で、可変または AI に依存するカスタムロジックは Python プラグインで実装します。

```mermaid
graph TB
  subgraph Browser["ブラウザ / モバイル端末"]
    UI["Web パネル · Android · HarmonyOS"]
  end
  subgraph Server["サーバー (Go)"]
    MW["ミドルウェア: 認証/CORS/gzip/セキュリティヘッダ"]
    API["HTTP API ルーティング"]
    WS["WebSocket プッシュ / ターミナル"]
    SRE["アラート·死活·Playbook·SRE·AI·メッセージ"]
    PG[(PostgreSQL)]
    VM[(VictoriaMetrics)]
  end
  subgraph Agent["Agent (Go)"]
    COL["ネイティブ収集器 (高頻度)"]
    EXT["外部収集器 (Redfish/NetFlow/OceanStor/SNMP)"]
    PLG["Python プラグインランナー"]
    LOG["ログ収集"]
    CH["リバースチャネル (ターミナル/転送)"]
  end
  subgraph Ext["外部デバイス"]
    BMC["BMC/iDRAC/iLO"]
    SW["スイッチ/ファイアウォール"]
    STOR["OceanStor"]
  end
  UI --> MW --> API
  API --> SRE --> PG & VM
  API --> WS
  COL --> API
  EXT --> BMC & SW & STOR
  PLG --> API
  LOG --> API
  CH <--> API
```

**主要な設計**

- **通信プロトコル**：HTTP REST（Agent 登録 / 上報 / ログ / 管理 API）；WebSocket（ブラウザのリアルタイムプッシュとリモートターミナル）；Agent が能動的にロングポーリングしてターミナル / 転送のリバースチャネルを確立。
- **データフロー**：Agent が収集 → サーバーへ上報 → リレーショナルデータは PG へ、時系列データは VM へ一括書き込み；ブラウザは REST / WebSocket で取得・下发。
- **二重ストレージ強制**：PG（リレーショナル ＋ 監査 ＋ イベント ＋ チケット ＋ セッション ＋ ベクトル記憶）と VM（メトリクス / トレンド）。いずれかが欠けても起動を拒否。
- **外部収集器**：標準プロトコルでリモートポーリング。対象デバイスへの侵入ゼロ、ネットワーク接続性のみ必要。
- **マルチサーバーブロードキャスト**：単一 Agent が1回の収集を複数サーバーへ並行上報。弱いネットワーク下でリトライ / サーキットブレーカー / gzip 低下。
- **AI / RAG**：プラグイン可能な LLM ＋ pgvector による類似事例検索。

より完全なアーキテクチャ、データフロー、性能、トラブルシューティングは [.qoder/repowiki/zh](.qoder/repowiki/zh/content) の「架構設計」「核心機能」「故障排除」などのドキュメントを参照。

---

## コントリビューションガイドライン

コード、ドキュメント、翻訳の貢献を歓迎します！

1. **Issue の提出**：バグ、機能提案、ドキュメント修正を歓迎します。再現手順と環境情報をできるだけ提供してください。
2. **開発環境**：Go 1.26+、Python 3（プラグイン SDK）。`make build` でビルド、`make audit` でセキュリティゲート（vet / test / govulncheck / gosec / staticcheck / sbom）を実行。
3. **コード規約**：
   - Go コードは `gofmt` / `go vet` を使用。新規ロジックにはテストを添付。
   - サーバーはゼロフレームワーク、ゼロ CGO。単一バイナリ、第三者依存ゼロの Agent 原則を維持。
   - コミットメッセージは「なぜ」を明確に表現。
4. **国際化**：マーケティングサイトは 簡中 / 繁中 / English / 日本語 の 4 言語、管理パネルは 簡中 / 繁中 / English の 3 言語に対応。新規文案は各言語辞書に同期してください（マーケティングサイトは `website/js/i18n.js` と `website/js/i18n-extra.js`；管理パネルは `cmd/server/web/` 配下の `i18n-dashboard*.js`。フロー：権威辞書を変更 → 英語を補完 → `build_en` / `build_tw` を実行 → parity を検証 → `go build` で再埋め込み）。
5. **PR の提出**：Fork → ブランチ開発 → 変更とテストを記述 → CI とレビューを待機。
6. **セキュリティ脆弱性**：公開 Issue は行わず、プライベートメッセージ / セキュリティチャネルから報告してください。優先的に対応します。
7. **開発者向け詳細規約**は [.qoder/repowiki/zh/content/开发者指南](.qoder/repowiki/zh/content/开发者指南) を参照。

---

## ライセンス

本プロジェクトは **MIT ライセンス**でオープンソースとして公開されています。詳細は [LICENSE](LICENSE) を参照。

- コードは GitHub でホスティングされ、透明で信頼できる。ホスト台数制限なし、機能の削減なし、「企業版」のような手口なし。
- 第三者依存（例：`vendor/` 配下の `lib/pq`、`go-qrcode`、`ledongthuc/pdf`、および `harmony/` の ohpm 依存）はそれぞれのライセンスに従い、原作者に帰属します。

---

<p align="center">
  <b>AIOps · 運用の複雑さを、あなたが完全に掌握する1つのプラットフォームに収束させる。</b>
</p>
