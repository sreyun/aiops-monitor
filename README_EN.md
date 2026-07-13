<div align="center">

# AIOps Monitor

**Enterprise Host Monitoring & SRE Ops Platform** 鈥?Go-native collection + Python plugin layer + real-time dashboard + threshold alerts + remote terminal + automation playbooks + SRE hub (incidents / auto-remediation / SLO / tickets) + log collection & search + AI inspection & diagnosis

[![Version](https://img.shields.io/badge/Version-v5.5.5-blue)](https://github.com/sreyun/aiops-monitor/releases)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#license)
[![Docker](https://img.shields.io/badge/Docker-multi--arch-blue?logo=docker&logoColor=white)](docker-compose.yml)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

[涓�枃](README.md) 路 [English](README_EN.md)

</div>

> Single-binary server, zero-dependency agent, tri-platform native collection (incl. GPU), one-command install. Built-in interactive trend charts, custom probes, remote terminal (no port opening + terminal password), automation playbooks, SRE hub (incidents / auto-remediation / SLO / tickets), log collection & full-text search, AI inspection & incident diagnosis, multi-user RBAC, MFA two-factor, PWA installable, port forwarding & HTTP proxy, i18n (zh / en / zh-TW).
>
> **v5.5.0 architecture upgrade**: storage unified on **PostgreSQL (all relational data) + VictoriaMetrics (all time-series)** 鈥?the embedded `aiops.db` single-file store is fully retired. Adds config-secret **AES-256-GCM encryption at rest**, optional **TLS in transit**, forced **security initialization** on first login, and cross-platform **boot autostart + keep-alive** (systemd / launchd / Scheduled Task).

## Table of Contents

- [Platform & Architecture Support](#platform--architecture-support)
- [Quick Start](#quick-start)
- [Core Features](#core-features)
- [Installation & Deployment](#installation--deployment)
- [Configuration Reference](#configuration-reference)
- [Monitoring Metrics](#monitoring-metrics)
- [Custom Monitoring (Probes)](#custom-monitoring-probes)
- [Automation Playbook](#automation-playbook)
- [Port Forwarding and HTTP Proxy](#port-forwarding-and-http-proxy)
- [Remote Terminal](#remote-terminal)
- [Plugin Development](#plugin-development)
- [Alert Configuration](#alert-configuration)
- [Alert Governance](#alert-governance)
- [API Monitoring](#api-monitoring)
- [AI Ops Assistant](#ai-ops-assistant)
- [Unified Message Center](#unified-message-center)
- [Advanced Features](#advanced-features)
- [Security Mechanisms](#security-mechanisms)
- [Cross-Network Deployment](#cross-network-deployment)
- [FAQ / Troubleshooting](#faq--troubleshooting)
- [Tech Stack & Architecture](#tech-stack--architecture)
- [Performance & Scale](#performance--scale)
- [API Reference](#api-reference)
- [Roadmap](#roadmap)
- [License](#license)

## Platform & Architecture Support

| Architecture | Linux | Windows | macOS |
|---|:---:|:---:|:---:|
| **AMD64 / x86_64** | 鉁?| 鉁?| 鉁?Intel Mac |
| **ARM64 / aarch64** | 鉁?| 鈥?| 鉁?Apple Silicon (M1/M2/M3/M4) |

> **Apple Silicon native**: `GOARCH=arm64` + `GOOS=darwin`, no Rosetta needed.  
> **Intel Mac native**: `GOARCH=amd64` + `GOOS=darwin`.  
> Docker images configured for `amd64` + `arm64` multi-arch cross-compilation; `docker pull` auto-selects matching architecture.

### Agent Cross-Compile Artifacts

| Filename | Platform | Architecture |
|---|---|---|
| `aiops-agent-linux-amd64` | Linux | AMD64 |
| `aiops-agent-linux-arm64` | Linux | ARM64 |
| `aiops-agent-darwin-amd64` | macOS | Intel |
| `aiops-agent-darwin-arm64` | macOS | Apple Silicon |
| `aiops-agent.exe` | Windows | AMD64 |

Install scripts auto-detect CPU architecture and download the matching binary 鈥?no manual selection needed.

---

## Quick Start

### Docker One-Click (Recommended)

```bash
# Choose the download URL based on your network environment:
#
# Option A (minimal, local/test): use the repo's built-in default secrets, start directly
# -- Via GitHub --
curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml
# -- Via Gitee mirror (recommended if GitHub is slow) --
curl -O https://gitee.com/bigdatasafe/aiops-monitor/raw/master/docker-compose.yml
docker compose up -d

# Option B (recommended, production): download and auto-generate strong random secrets
# -- Via GitHub --
bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh) && docker compose up -d
# -- Via Gitee mirror (recommended if GitHub is slow) --
bash <(curl -fsSL https://gitee.com/bigdatasafe/aiops-monitor/raw/master/scripts/secure-compose.sh) && docker compose up -d
```

> Three-container stack: `aiops-server` (Go single binary with `//go:embed` front-end) + `postgres` + `victoriametrics`, all brought up by one compose command. The server **requires** PG + VM and refuses to start without them.
>
> Images are hosted on Huawei Cloud SWR (`swr.cn-east-3.myhuaweicloud.com/sreyun/`). Every tag push triggers GitHub Actions to build `linux/amd64` + `linux/arm64` multi-arch images and push them to SWR; `docker pull` auto-selects the matching architecture.

> **Default credentials**: `admin / admin`. **On first login a forced "Security Initialization" dialog requires changing the username + password before you can enter**; enabling MFA afterwards is recommended. The command above auto-generates random DB password and encryption key 鈥?**make sure to save the printed `PG password` and `SECRET_KEY`**.

### Binary Direct Run

```bash
# Start server (default listen :8529)
./bin/aiops-server

# Start agent (run from repo root to find plugins/)
./bin/aiops-agent --server http://<server-IP>:8529 --category Production
```

Open `http://localhost:8529` 鈥?host card and metrics appear within seconds.

---

## Core Features

| Capability | Description |
|---|---|
| **Tri-platform native collection** | Linux (`/proc` + `syscall`), Windows (Win32 API), macOS (`sysctl`), all zero third-party deps |
| **Comprehensive metrics** | CPU / Memory / SWAP / Multi-disk / Network / TCP connections / Load / Processes / Uptime / **GPU** |
| **GPU monitoring** | NVIDIA (`nvidia-smi`), AMD (Linux sysfs), Apple (macOS `ioreg`), best-effort + cache |
| **Interactive trend charts** | Pure Canvas, hover crosshair + tooltip, drag-zoom, double-click reset, enlarge preview |
| **Custom probes** | HTTP (status/latency/TLS cert days) / TCP / Ping (loss%/RTT) / process; history curves |
| **Remote terminal** | Browser full TTY via Agent reverse connection (no inbound port); multi-tab, recording playback, read-only observe, command audit |
| **Port forwarding (TCP / UDP / HTTP)** | Map a remote host's TCP / UDP ports to the server's local ports via the Agent tunnel (HTTP uses a stateless proxy tunnel straight to web services); port-range batch forwarding (≤100 ports per batch) + persistent rules with enable/disable/edit/copy |
| **Automation playbooks** | Multi-step orchestration + target selection (all/category/system/host) 鈫?batch parallel execution 鈫?real-time output + history |
| **Alert push** | Feishu / DingTalk Webhook + Email SMTP + **multi-cloud SMS + multi-cloud Voice call** (Aliyun / Huawei / Tencent, TTS), trigger/recover transitions only, no spam |
| **Custom alert thresholds** | 27 fine-grained warn/crit pairs (host / probe / API / task / forward), host dimension also offers conservative/standard/relaxed presets, zero-value auto-backfill |
| **Embedding model config** | RAG embedding model decoupled from chat model, any OpenAI-compatible `/embeddings` (OpenAI / BaiLian / bge / m3e), configurable dimension + one-click self-check |
| **Multi-user RBAC** | admin / operator / viewer, route-level permission, user management UI |
| **MFA two-factor** | TOTP (RFC 6238), Google Authenticator compatible, QR enrollment |
| **Account recovery** | Forgot username / forgot password (email code) / MFA unbind via email, anti-enumeration |
| **Multi-server push** | Single agent pushes to multiple servers; collect once, broadcast all; independent auth/retry |
| **Gateway relay mode** | One internet-connected machine proxies all requests to cloud; binary/report/terminal auto-tunnel |
| **Machine fingerprint auth** | machine-id + MAC hash fingerprint binding; token rotation doesn't affect installed agents |
| **SRE hub** | Incidents (alert / SLO / manual with timeline) 路 alert鈫抪laybook closed-loop auto-remediation (guardrails + approval) 路 SLO / error budget (long-window queried from VM) 路 tickets |
| **Log collection & search** | Agent `--log-paths` incremental tailing 鈫?server search by host / level / keyword / time; auto level classification error/warn/info |
| **AI inspection & diagnosis** | Scheduled health inspection + incident root-cause analysis; agent-level analysis when an AI provider is configured, heuristic fallback otherwise; **error/warn logs are fed into the analysis context** |
| **Unified storage (PG + VM)** | Relational data (config / users / audit / incidents / tickets / sessions) in PostgreSQL, time-series (metrics / trends) in VictoriaMetrics; embedded aiops.db fully retired, refuses to start without both |
| **Encryption at rest & TLS** | Config secrets (MFA / SMTP / AI / webhook / relay) sealed with AES-256-GCM (`AIOPS_SECRET_KEY`); optional HTTPS/TLS in transit |
| **Message center** | In-app notification bell aggregating incidents / AI diagnosis / auto-remediation / tickets, each deep-linked into the SRE hub |
| **Persistence** | All state in PostgreSQL + VictoriaMetrics (the in-memory tiered window is a hot cache only) |
| **PWA installable** | Install to desktop, Service Worker offline cache, standalone window |
| **gzip compression** | API/static auto-gzip, ~8-10x bandwidth reduction for multi-host polling |
| **One-click install** | Dashboard-generated Token command, auto-download + config + boot autostart |

---

## Installation & Deployment

### Option 1: Docker (Pre-built Images 路 Recommended)

**One-click deploy (auto-generates random passwords):**

```bash
# Via GitHub:
curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml && \
PG_PWD=$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c20) && \
SECRET_KEY="aiops-$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c44)" && \
sed -i "s|h3Y7Vmb1CZBOApZM86D|${PG_PWD}|g" docker-compose.yml && \
sed -i "s|aiops-K7p2mQ9vR4xN8wZ3bY6dF1hJ5sL0tGc-CHANGE-ME-2026|${SECRET_KEY}|" docker-compose.yml && \
echo "PG password: ${PG_PWD}" && echo "SECRET_KEY: ${SECRET_KEY}" && \
docker compose up -d

# Via Gitee mirror (recommended if GitHub is slow):
curl -O https://gitee.com/bigdatasafe/aiops-monitor/raw/master/docker-compose.yml && \
PG_PWD=$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c20) && \
SECRET_KEY="aiops-$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c44)" && \
sed -i "s|h3Y7Vmb1CZBOApZM86D|${PG_PWD}|g" docker-compose.yml && \
sed -i "s|aiops-K7p2mQ9vR4xN8wZ3bY6dF1hJ5sL0tGc-CHANGE-ME-2026|${SECRET_KEY}|" docker-compose.yml && \
echo "PG password: ${PG_PWD}" && echo "SECRET_KEY: ${SECRET_KEY}" && \
docker compose up -d
```

> The command above: downloads compose file 鈫?generates random passwords/keys 鈫?writes them into config 鈫?pulls images and starts. **Make sure to save the printed passwords and keys!**

**Pin to a specific version (recommended for production):**

```bash
# Replace :latest with a specific version tag in docker-compose.yml
sed -i 's|aiops-server:latest|aiops-server:v5.5.5|' docker-compose.yml
sed -i 's|aiops-agent:latest|aiops-agent:v5.5.5|' docker-compose.yml
docker compose up -d
```

- Images hosted on Huawei Cloud SWR: `swr.cn-east-3.myhuaweicloud.com/sreyun/aiops-server:latest`
- Every tag push triggers GitHub Actions to build `linux/amd64` + `linux/arm64` multi-arch images
- Server data persists via volume (`/app/data`), config at `./data/server_config.json`
- Default port `8529`, modifiable in `docker-compose.yml`
- Agent container not started by default 鈥?uncomment `aiops-agent` section to enable
- To build locally, replace `image:` with the commented `build:` config in `docker-compose.yml` and run `docker compose up -d --build`

### CI/CD Auto-Build

Every version tag push (`v*`) to GitHub triggers the following pipeline:

1. **Checkout** 鈫?Extract Git tag as version number
2. **Multi-arch cross-compile** 鈫?`linux/amd64` + `linux/arm64` Go binaries
3. **Build Docker images** 鈫?Multi-arch images via `docker/build-push-action`
4. **HMAC-SHA256 auth** 鈫?Auto-generate SWR login credentials from `HW_ACCESS_KEY` / `HW_SECRET_KEY`
5. **Push to Huawei Cloud SWR** 鈫?`swr.cn-east-3.myhuaweicloud.com/sreyun/aiops-server:{tag}` and `aiops-agent:{tag}`

**Image tags:**

| Tag | Description |
|---|---|
| `:latest` | Always points to the latest Release |
| `:v5.5.5` etc. | Pin to a specific version (recommended for production) |

**Required GitHub Secrets** (configure in repo Settings 鈫?Secrets and variables 鈫?Actions):

| Secret | Description |
|---|---|
| `HW_ACCESS_KEY` | Huawei Cloud Access Key (AK) |
| `HW_SECRET_KEY` | Huawei Cloud Secret Key (SK) |

> Workflow definition: [`.github/workflows/release.yml`](.github/workflows/release.yml).

<details>
<summary>Manual Docker build</summary>

```bash
# Build server image
docker build --target server -t aiops-server .

# Build agent image
docker build --target agent -t aiops-agent .

# Run
docker run -d -p 8529:8529 -v aiops-data:/app/data --name aiops-server aiops-server
```
</details>

### Option 2: One-Click Install Script (Recommended for Production)

Click **銆孖nstall Agent銆?* in the dashboard top-right 鈫?select target OS 鈫?copy command to monitored host:

```bash
# Linux (root/sudo) 鈥?auto-detects amd64/arm64
curl -fsSL "http://<server>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows (admin PowerShell)
irm "http://<server>:8529/install.ps1?token=<TOKEN>" | iex

# macOS 鈥?auto-detects Intel/Apple Silicon
curl -fsSL "http://<server>:8529/install.sh?token=<TOKEN>" | sh
```

Command includes server URL + Token, auto-downloads matching-arch binary + plugins, writes config, registers boot autostart.

### Option 3: Binary Direct Run

**Start server**:

```bash
./bin/aiops-server                          # Default :8529
./bin/aiops-server -addr 0.0.0.0:9000       # Custom address/port
./bin/aiops-server -config /path/to/config  # Custom config path
```

**Start agent** (from repo root to find `plugins/`):

```bash
# Linux AMD64
./bin/aiops-agent-linux-amd64 --server http://<IP>:8529 --category Production

# Linux ARM64
./bin/aiops-agent-linux-arm64 --server http://<IP>:8529 --category Production

# macOS Apple Silicon
./bin/aiops-agent-darwin-arm64 --server http://<IP>:8529 --category Production

# macOS Intel
./bin/aiops-agent-darwin-amd64 --server http://<IP>:8529 --category Production

# Windows AMD64
.\bin\aiops-agent.exe --server http://<IP>:8529 --category Production
```

### Option 4: Build from Source

```bash
# Requires Go 1.22+
go build -o bin/aiops-server ./cmd/server
go build -o bin/aiops-agent  ./cmd/agent

# Cross-compile all architectures
GOOS=linux   GOARCH=amd64 go build -o bin/aiops-agent-linux-amd64   ./cmd/agent
GOOS=linux   GOARCH=arm64 go build -o bin/aiops-agent-linux-arm64   ./cmd/agent
GOOS=darwin  GOARCH=amd64 go build -o bin/aiops-agent-darwin-amd64  ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o bin/aiops-agent-darwin-arm64  ./cmd/agent
GOOS=windows GOARCH=amd64 go build -o bin/aiops-agent.exe           ./cmd/agent
```

### Boot Autostart

<details>
<summary>Linux (systemd)</summary>

```bash
cp deploy/aiops-agent.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now aiops-agent
```
</details>

<details>
<summary>Windows (NSSM)</summary>

```powershell
nssm install AIOps-Agent C:\aiops-agent\aiops-agent.exe "--server http://<IP>:8529 --category Production"
nssm set AIOps-Agent AppDirectory C:\aiops-agent
nssm start AIOps-Agent
```
</details>

<details>
<summary>Windows (Task Scheduler)</summary>

```powershell
schtasks /Create /TN "AIOps-Agent" /TR "C:\aiops-agent\start-agent.bat" /SC ONSTART /RU SYSTEM /RL HIGHEST /F
```
</details>

<details>
<summary>macOS (launchd)</summary>

```bash
cp deploy/com.aiops.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.aiops.agent.plist
```
</details>

See [INSTALL.md](INSTALL.md) for detailed deployment guide.

---

## Configuration Reference

### Agent Config (`config.example.json`)

```json
{
  "server": "http://localhost:8529",
  "servers": [
    {"server": "https://monitor-a:8529", "token": "token-a"},
    {"server": "https://monitor-b:8529", "token": "token-b"}
  ],
  "report_interval": 10,
  "plugin_interval": 15,
  "disk_path": "/",
  "plugins_dir": "plugins",
  "python": "python3",
  "state_file": "agent_state.json",
  "category": "",
  "token": ""
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `server` | string | `http://localhost:8529` | Single server URL (fallback when `servers` empty) |
| `servers` | array | `[]` | Multi-server list, each with `server` + `token`; takes precedence |
| `report_interval` | int | `10` | Base metric report interval (seconds) |
| `plugin_interval` | int | `15` | Plugin execution cycle (seconds) |
| `disk_path` | string | `/` | Primary disk path (all local disks auto-detected) |
| `plugins_dir` | string | `plugins` | Plugin directory (absolute path OK) |
| `python` | string | `python3` | Python interpreter (`python` on Windows) |
| `state_file` | string | `agent_state.json` | Agent state file (contains host_id) |
| `category` | string | `""` | Host category (dashboard groups by this) |
| `token` | string | `""` | Install Token (optional) |

### Agent CLI Parameters

| Parameter | Description | Default |
|---|---|---|
| `--server` | Server address | `http://localhost:8529` |
| `--category` | Host category | empty |
| `--interval` | Base metric report interval (s) | `10` |
| `--plugin-interval` | Plugin execution cycle (s) | `15` |
| `--plugins-dir` | Plugin directory | `plugins` |
| `--python` | Python interpreter | `python3` (`python` on Windows) |
| `--disk-path` | Primary disk path | `/` (system drive on Windows) |
| `--token` | Install Token | empty |
| `--relay` | Gateway relay mode | `false` |
| `--listen` | Relay listen address | `:8529` |
| `--config` | Config file path | `config.json` |

> Flags override config file; config file overrides defaults. `servers` array takes precedence over `server` + `token`.

### Server Config (`server_config.example.json`)

| Field | Type | Default | Description |
|---|---|---|---|
| `alerts_enabled` | bool | `true` | Enable alert push |
| `feishu.enabled` | bool | `false` | Feishu push toggle |
| `feishu.webhook` | string | `""` | Feishu bot Webhook URL |
| `dingtalk.enabled` | bool | `false` | DingTalk push toggle |
| `dingtalk.webhook` | string | `""` | DingTalk bot Webhook URL |
| `dingtalk.secret` | string | `""` | DingTalk signing Secret |
| `thresholds.cpu_warn` | float | `80` | CPU warning threshold (%) |
| `thresholds.cpu_crit` | float | `90` | CPU critical threshold (%) |
| `thresholds.mem_warn` | float | `80` | Memory warning threshold (%) |
| `thresholds.mem_crit` | float | `90` | Memory critical threshold (%) |
| `thresholds.disk_warn` | float | `85` | Disk warning threshold (%) |
| `thresholds.disk_crit` | float | `95` | Disk critical threshold (%) |
| `thresholds.offline_after_sec` | int | `60` | Host offline threshold (seconds) |
| `thresholds.diskio_warn` / `_crit` | float | `80` / `95` | Disk IO utilization warn / crit (%) |
| `thresholds.iops_warn` / `_crit` | float | `50000` / `100000` | IOPS warn / crit (total read+write) |
| `thresholds.gpu_warn` / `_crit` | float | `80` / `95` | GPU utilization warn / crit (%) |
| `thresholds.load_warn` / `_crit` | float | `4.0` / `8.0` | System load multiplier warn / crit (× CPU cores) |
| `thresholds.proc_warn` | float | `0.5` | Process-count change ratio (0.5 = ±50%) |
| `thresholds.check_ping_loss_warn` / `_crit` | float | `10` / `30` | Probe Ping packet loss warn / crit (%) |
| `thresholds.check_ping_latency_warn` / `_crit` | float | `100` / `500` | Probe Ping latency warn / crit (ms) |
| `thresholds.check_tcp_timeout_warn` / `_crit` | float | `1000` / `5000` | Probe TCP connect timeout warn / crit (ms) |
| `thresholds.check_http_resp_warn` / `_crit` | float | `1000` / `5000` | Probe HTTP response time warn / crit (ms) |
| `thresholds.check_http_status_warn` / `_crit` | int | `1` / `5` | Probe HTTP non-2xx count warn / crit |
| `thresholds.check_proc_fail_warn` / `_crit` | int | `1` / `3` | Process-alive failure count warn / crit |
| `thresholds.api_avail_warn` / `_crit` | float | `99` / `95` | API availability warn / crit (alerts below, %) |
| `thresholds.api_avg_resp_warn` / `_crit` | float | `500` / `2000` | API avg response warn / crit (ms) |
| `thresholds.api_p95_resp_warn` / `_crit` | float | `1000` / `5000` | API P95 response warn / crit (ms) |
| `thresholds.api_throughput_warn` / `_crit` | float | `100` / `10` | API throughput warn / crit (alerts below, req/s) |
| `thresholds.task_fail_warn` / `_crit` | int | `1` / `5` | Scheduled-task failure count warn / crit |
| `thresholds.task_timeout_warn` / `_crit` | float | `60` / `300` | Scheduled-task timeout warn / crit (s) |
| `thresholds.forward_conn_warn` / `_crit` | int | `200` / `280` | Port-forward active connections warn / crit |
| `thresholds.forward_bw_warn` / `_crit` | float | `80` / `95` | Port-forward bandwidth usage warn / crit (%) |
| `thresholds.forward_err_warn` / `_crit` | float | `5` / `15` | Port-forward error rate warn / crit (%) |
| `thresholds.forward_lat_warn` / `_crit` | float | `1000` / `5000` | Port-forward avg latency warn / crit (ms) |
| `require_token` | bool | `false` | Require Agent Token |
| `allow_anonymous_agents` | bool | `false` | Allow Token-less Agents |
| `terminal_disabled` | bool | `false` | Globally disable remote terminal |
| `install_token` | string | auto-gen | Agent install Token |
| `trust_proxy` | bool | `false` | Behind reverse proxy: set `true` to honor `X-Real-IP` for rate-limiting |
| `smtp.smtp_enabled` | bool | `false` | Email push toggle |
| `smtp.smtp_host` | string | `""` | SMTP server address |
| `smtp.smtp_port` | int | `0` | SMTP port (465 implicit TLS / 587 STARTTLS) |
| `smtp.smtp_username` | string | `""` | Sender email account |
| `smtp.smtp_password` | string | `""` | SMTP auth code/password (masked) |
| `smtp.smtp_from_name` | string | `"AIOps Monitor"` | Sender display name |
| `smtp.smtp_use_tls` | bool | `false` | Enable implicit TLS (465 = `true`, 587 = `false`) |
| `sms.enabled` | bool | `false` | SMS push toggle |
| `sms.provider` | string | `"aliyun"` | SMS provider: `aliyun` / `huawei` / `tencent` — all three supported |
| `sms.access_key` | string | `""` | Cloud account AccessKey (Aliyun AccessKeyId / Huawei AppKey / Tencent SecretId) |
| `sms.secret_key` | string | `""` | Cloud account SecretKey (Aliyun AccessKeySecret / Huawei AppSecret / Tencent SecretKey; masked) |
| `sms.app_id` | string | `""` | App/project id: **Huawei** = SMS app project_id; **Tencent** = SmsSdkAppId; leave empty for Aliyun |
| `sms.sign_name` | string | `""` | SMS signature (SignName) |
| `sms.template_code` | string | `""` | SMS template CODE (Aliyun TemplateCode / Huawei templateId / Tencent TemplateId) |
| `sms.template_param` | string | `""` | Custom template params (JSON, e.g. `{"code":"${code}"}`; defaults to `{"message":"<alert>"}` when empty) |
| `sms.phones` | []string | `[]` | Recipient phone numbers (Huawei/Tencent auto-prefix `+86`) |
| `voice_call.enabled` | bool | `false` | Voice call push toggle |
| `voice_call.provider` | string | `"aliyun"` | Voice provider: `aliyun` / `huawei` / `tencent` — all three supported |
| `voice_call.access_key` | string | `""` | Cloud account AccessKey (same rule as SMS) |
| `voice_call.secret_key` | string | `""` | Cloud account SecretKey (same rule as SMS; masked) |
| `voice_call.app_id` | string | `""` | App/project id: **Huawei** = project_id; **Tencent** = VoiceSdkAppid; leave empty for Aliyun |
| `voice_call.called_numbers` | []string | `[]` | Called numbers (Huawei/Tencent auto-prefix `+86`) |
| `voice_call.tts_code` | string | `""` | Voice template TTS CODE (Aliyun TtsCode / Huawei template / Tencent TemplateId) |
| `voice_call.tts_param` | string | `""` | Voice template params (JSON, default `{"message":"..."}`) |

> **Multi-cloud SMS / voice auth**: Aliyun uses ACS3-HMAC-SHA256 signature V3 (`dysmsapi` / `dyvmsapi`); Huawei uses X-WSSE (`smsapi.cn-north-4` / `rtc-api`, requires `app_id` = project_id); Tencent uses TC3-HMAC-SHA256 (`sms.tencentcloudapi.com` / `vms.tencentcloudapi.com`, requires `app_id`). Switching provider only needs a `provider` change plus the matching fields — no redeploy.

#### Custom Alert Thresholds

The platform ships **27 fine-grained (warn / crit) threshold pairs** across five monitoring dimensions, all individually editable via the `thresholds` field in `server_config.json` or the dashboard "Alert Settings" — effective on save:

| Dimension | Metrics covered |
|---|---|
| **Host resources** | CPU / Memory / Disk / Disk IO / IOPS / GPU / System load / Process-count change / Offline detection |
| **Probe monitoring** | Ping loss & latency / TCP connect timeout / HTTP response & status code / Process-alive failures |
| **API business monitoring** | Availability / Avg response / P95 response / Throughput |
| **Scheduled tasks** | Failure count / Timeout duration |
| **Port forwarding** | Active connections / Bandwidth usage / Error rate / Avg latency |

> **Zero-value backfill**: the alert engine fires on `metric ≥ threshold`, so a `0` would alert constantly. Any `0` value (unconfigured / blank form / legacy config missing a field) is automatically healed to its standard default — **fill what you need, the rest fall back to recommended defaults**. Host-resource metrics additionally offer conservative / standard / relaxed presets (default: standard) as a starting point.

### Server CLI Parameters

| Parameter | Description | Default |
|---|---|---|
| `-addr` | Listen address | `:8529` |
| `-config` | Config file path | `server_config.json` |
| `-dist` | Agent download directory | auto-detect `./dist` or executable dir |

### Environment Variables

| Variable | Required | Description |
|---|:---:|---|
| `AIOPS_POSTGRES_DSN` | **Yes** | PostgreSQL DSN, e.g. `postgres://user:pwd@host:5432/db?sslmode=disable`. All relational data lives in PG; **the server refuses to start without it** |
| `AIOPS_VM_URL` | **Yes** | VictoriaMetrics URL, e.g. `http://victoriametrics:8428`. All time-series lives in VM; **refuses to start without it** |
| `AIOPS_SECRET_KEY` | Strongly recommended | Master key for at-rest encryption of config secrets (AES-256-GCM). **Back it up 鈥?losing it makes already-encrypted secrets unrecoverable** |
| `AIOPS_TLS_CERT` / `AIOPS_TLS_KEY` | Optional | TLS cert / key paths; serves HTTPS when set, otherwise plain HTTP (put behind a TLS-terminating proxy) |
| `AIOPS_FORWARD_LISTEN` | Optional | TCP forward listen address (must be `0.0.0.0` for Docker) |
| `AIOPS_TRUST_PROXY` | Optional | Set `true` behind a trusted reverse proxy to honor `X-Real-IP` for rate limiting |
| `AIOPS_TERMINAL_DISABLED` / `AIOPS_FORWARD_DISABLED` / `AIOPS_REQUIRE_TOKEN` / `AIOPS_ALLOW_ANONYMOUS_AGENTS` | Optional | Feature/security toggles (`true`/`false` or `1`/`0`) |

> Priority: environment variables > `server_config.json`.

---

## Monitoring Metrics

| Metric | Linux | Windows | macOS |
|---|---|---|---|
| CPU usage / cores | `/proc/stat` | `GetSystemTimes` | `top -l 2` |
| Memory / SWAP | `/proc/meminfo` | `GlobalMemoryStatusEx` | `sysctl` + `vm_stat` |
| Disk (all local) | `/proc/mounts` + `statfs` | `GetDiskFreeSpaceExW` | `syscall.Statfs` + `df` |
| Network RX/TX | `/proc/net/dev` | `GetIfTable` | `netstat -ibn` |
| TCP connections | `/proc/net/tcp` | `GetTcpTable` | `netstat -an` |
| Load 1/5/15 | `/proc/loadavg` | EWMA approximation | `sysctl vm.loadavg` |
| Process count | `/proc` enumerate | `EnumProcesses` | `ps -A` |
| Uptime | `/proc/uptime` | `GetTickCount64` | `sysctl kern.boottime` |
| **GPU util/VRAM/temp** | `nvidia-smi` / amdgpu sysfs | `nvidia-smi` | `ioreg` (IOAccelerator) |

**All three platforms are zero third-party dependency**. GPU is best-effort: reports when vendor tools or OS interfaces are available, cached ~12s; no GPU/no tool = no display, doesn't affect other metrics.

---

## Custom Monitoring (Probes)

The dashboard銆孧onitoring銆峱age lets you add active probes 鈥?periodic checks on websites, ports, host connectivity, and process alive:

| Type | What to fill | Failure condition |
|---|---|---|
| **HTTP website** | URL (e.g. `https://example.com`) | Status 鈮?400, or timeout/failure |
| **TCP port** | host:port (e.g. `10.0.0.5:3306`) | Cannot connect |
| **Ping host** | host/IP (e.g. `8.8.8.8`) | 100% loss (unreachable) |
| **Process alive** | 鈶?Target host + 鈶?Process name | Process not reported by target host (or offline) |

> Process monitoring requires selecting target host first, then process name 鈥?the server checks the host's Agent-reported process list. Case-insensitive substring match. Each item supports list/pill dual view + history curve.

---

## Automation Playbook

The dashboard銆孉utomation銆峱age lets you orchestrate playbooks 鈥?ordered shell commands executed in batch on target hosts:

**Create playbook**: name + steps, each with:
- **Command**: one-line shell command (Linux `sh -c`, Windows `cmd /c`)
- **Target**: `all` / `category:xxx` / `system:linux|windows|macos` / `host:<ID>`
- **Timeout** (seconds) and **continue on failure**

**Execution**: commands sent via Agent reverse channel, executed as one-shot subprocesses, returning output + exit code. All matching online hosts execute in parallel; each host runs steps sequentially. History retains last 100 runs.

> Commands are non-interactive 鈥?don't use `vim`/`top`/`ssh`. Each step is an independent process; `cd`/`export` don't carry over 鈥?chain with `&&` in the same step.

---

## Port Forwarding and HTTP Proxy

Access a target host's internal services through the Agent's reverse tunnel without opening any inbound port on that host. Three protocols are supported — TCP / UDP / HTTP — and TCP / UDP additionally support port-range batch forwarding:

### TCP / UDP Port Mapping

Persistently map a remote host's TCP or UDP port to the server's local port. TCP suits long-lived protocols like databases and SSH; UDP suits datagram services like DNS, gaming, and media:

```bash
# Example: map Agent host's MySQL 3306 to server 13306 (TCP)
# Create a rule on the "Forwarding" page, or via API:
curl -X POST http://<server>:8529/api/v1/forward \
  -d '{"host_id":"abc123","target_port":3306,"local_port":13306}'

# UDP example: map DNS 53 to local 1353
curl -X POST http://<server>:8529/api/v1/forward \
  -d '{"host_id":"abc123","target_port":53,"local_port":1353,"protocol":"udp"}'

# Then connect from your local client
mysql -h 127.0.0.1 -P 13306 -u root -p
```

- Set `protocol` to `tcp` (default) or `udp`
- Auto-allocate (`local_port: 0`) or specify a port
- Rules can be enabled/disabled/edited/copied/deleted
- Listen address configurable (`forward_listen`), default `127.0.0.1` (local only); Docker deployments must set `0.0.0.0` or override via `AIOPS_FORWARD_LISTEN`
- Port range configurable (`forward_port_range`); Docker deployments must match the `ports` mapping

### TCP / UDP Port-Range Batch Forwarding

TCP and UDP can map an entire contiguous port range in one shot: set a start port `target_port` and an end port `target_port_end` (**inclusive**, with `target_port_end > target_port`). The system creates one independent rule per port in the range; the whole batch shares a group ID so it can be enabled / disabled / deleted together instead of one rule at a time. **Up to 100 ports per batch.**

```bash
# Example: map Agent host abc123's UDP 5000–5010 (11 ports, grouped as one)
curl -X POST http://<server>:8529/api/v1/forward \
  -d '{"host_id":"abc123","target_port":5000,"target_port_end":5010,"protocol":"udp"}'
```

> Note: port-range batch forwarding applies to TCP / UDP only. HTTP forwarding uses the stateless proxy tunnel below and is accessed per-URL on demand, so there is no "whole port range" concept.

### HTTP Reverse Proxy

A stateless proxy that needs no rule — access the target host's web service directly by URL:

```bash
# Access /api/health on port 8080 of Agent host abc123
curl http://<server>:8529/proxy/abc123/8080/api/health

# All HTTP methods + WebSocket upgrade supported
ws://<server>:8529/proxy/abc123/8080/ws
```

- Supports GET/POST/PUT/DELETE/PATCH
- Supports WebSocket upgrade (Nginx must pass the Upgrade headers)
- The panel can save frequently used proxies as quick entries
- `window.open()` scenarios use a one-time `proxy_token` for auth

> Port forwarding is enabled by default; set `forward_disabled: true` in alert settings to disable it globally.

---

## Remote Terminal

- **Multi-tab**: one-click from host card, multiple hosts/sessions simultaneously
- **Recording & playback**: auto-recorded (timestamped frames), progress bar drag, speed control
- **Read-only observe**: multiple admins can observe an active session simultaneously
- **Command audit**: executed commands auto-extracted to activity log
- **Cross-platform TTY**: Windows ConPTY (chcp 65001 + GBK鈫扷TF-8), Linux/macOS openpty
- **No port opening**: via Agent reverse connection, no inbound port on target

> Terminal/playbook share the Agent reverse channel 鈥?one session per host at a time. Cross-network requires [Nginx WebSocket config](#cross-network-deployment).

---

## Plugin Development

A plugin = an executable script that prints a JSON object to stdout. With the SDK:

```python
# plugins/my_check.py
from plugin_sdk import Plugin

p = Plugin()
p.metric("mysql.connections", 42)          # Custom metric (gauge)
p.metric("mysql.qps", 1350.5)
p.event("warning", "Replication lag 8s")   # Event (info | warning | critical)
p.emit()                                   # Output JSON
```

Drop in `plugins/` directory for auto-discovery, executed every `--plugin-interval`. Crashes/timeouts/bad JSON are logged and skipped 鈥?no impact on core. Non-`.py` executables also work as plugins 鈥?any language.

---

## Alert Configuration

Alerts are configured visually in the dashboard 鈥?no file editing:

1. Click **Alert Settings** in the top-right
2. Fill Feishu or DingTalk Webhook URL (DingTalk: fill Secret if using signing), check enable
3. **Email push**: expand SMTP section, fill server/port/account/auth code, port 465 = implicit TLS, 587 = not
4. **SMS push**: expand the SMS section, choose provider (**Aliyun / Huawei Cloud / Tencent Cloud**), fill AccessKey / SecretKey / SignName / TemplateCode / recipient phones; Huawei/Tencent also need `app_id` (Huawei = project_id, Tencent = SmsSdkAppId) and an optional custom template-param JSON, check enable
5. **Voice call push**: expand the Voice call section, choose provider (Aliyun / Huawei / Tencent), fill AccessKey / SecretKey / called numbers / TTS Code (optional TTS param); Huawei/Tencent need `app_id` (Huawei = project_id, Tencent = VoiceSdkAppid), check enable; reads the alert aloud (TTS) to on-call
6. Click **Send Test** to verify connectivity (SMS / voice can be tested separately)
7. Click **Save** 鈥?outstanding alerts re-pushed after save

| Alert type | Trigger condition | Level |
|---|---|---|
| CPU / Memory / Disk | Exceeds threshold | Warning / Critical |
| Host offline | No report within threshold | Critical |
| GPU usage | 鈮?80% warning, 鈮?90% critical | Warning / Critical |
| System load | 5-min load 鈮?cores脳2 | Warning / Critical |
| HTTP / TCP / Ping / Process | Probe failure | Custom |

> Feishu custom bot keyword: `AIOps` or `鍛婅�`. DingTalk: use "signing" security.

---

## Alert Governance

Inserts a decision layer before notifications are actually sent, applying **silence / inhibition / route** to firing alerts to suppress alert storms, reduce night-time noise, and split by business:

- **Silence rules**: match by `host / type / level`, support **time window** (start/end `HH:MM`, can cross midnight) + **weekday** temporary silence. e.g. "silence non-critical alerts every 23:00–08:00".
- **Inhibit rules**: a root-cause alert suppresses derived alerts. e.g. when "host down" fires, automatically suppress that host's CPU/memory/disk alerts, avoiding dozens of alerts from one failure.
- **Notification routes**: split matched alerts to a channel (Feishu / DingTalk / Email / Custom Webhook); `Continue` can chain to the next rule. e.g. "critical → phone/DingTalk, warning → Feishu only".
- **Recovery notifications are always sent**, unaffected by silence.

> Config: dashboard "Alerts" → "Alert Governance"; submitted as a whole, server strips unnamed rules.

## API Monitoring

Batch health / performance black-box probes for **a business system's set of APIs**, complementing the "business availability" dimension beyond host metrics:

- Dashboard "Monitoring" → "API Monitoring" to add a business system with multiple endpoints (URL / method / Header / Body / expected status / keyword / JSONPath / JSON assertion / cert warning days).
- Reuses the advanced HTTP probe engine (DNS / TCP / TLS / TTFB segmented timing); results written to VictoriaMetrics (`aiops_api_*` metric family).
- Aggregated on the fly: avg latency / **P95 latency** / 1h·24h **availability** / throughput.
- Endpoint anomalies fire unified alerts by business-system level (same source as custom probes).
- Use cases: website / OpenAPI / microservice SLA monitoring, core-link availability dashboards.

## AI Ops Assistant

A built-in **autonomous ops Agent framework** on pluggable LLMs (OpenAI-compatible / Anthropic / BaiLian) + AI inspection diagnosis — the intelligent value-add layer on top of monitoring data:

- **AI inspection (aiops)**: scheduled / manual health inspection combining online / offline hosts, active alerts, SLO breaches, and recent error logs into a health assessment; **falls back to a built-in heuristic when no LLM is configured — runs with zero external dependency**.
- **Incident diagnosis + RAG**: a critical incident auto-triggers AI root-cause analysis written into the incident timeline; optional **pgvector diagnosis embeddings** retrieve historically similar cases (requires an embedding endpoint) — gets smarter over time.
- **Autonomous Agent**: multi-turn chat in dashboard "AI Assistant" (SSE streaming + session persistence) with **Function Calling tool use** — query metrics / search logs / list alerts / retrieve similar cases / read-only terminal inspection; configurable agent rules (rules / templates) plus auto-approve and read-only terminal toggles.
- Config: intelligent analysis is enabled after filling the LLM endpoint, model and secret (AES-encrypted at rest via `AIOPS_SECRET_KEY`) in "AI Config"; the autonomous Agent, RAG diagnosis and other capabilities can then be turned on.

#### Embedding Model (RAG) Configuration

The **embedding model used for RAG is decoupled from the chat model** and can point to any OpenAI-compatible `/embeddings` service (OpenAI text-embedding-3, Aliyun BaiLian text-embedding-v2, or self-hosted bge / m3e / gte / text2vec) — no longer tied to a single vendor:

| Field (`ai.*`) | Type | Default | Description |
|---|---|---|---|
| `ai.embed_endpoint` | string | `""` | Embedding endpoint; **falls back to the main chat endpoint** when empty |
| `ai.embed_api_key` | string | `""` | Embedding API Key; falls back to the main API Key when empty (masked) |
| `ai.embed_model` | string | `""` | Embedding model name, e.g. `text-embedding-3-small` / `text-embedding-v2` / `bge-large-zh` |
| `ai.embed_dimensions` | int | `1536` | Target vector dimension; **must match the PostgreSQL `pgvector` column dimension** |

- **Decoupling benefit**: use a large model for chat and a lightweight embedding model for vectorization — independently billed and rate-limited, each optimized for cost and performance.
- **Dimension consistency**: `embed_dimensions` determines the vector length written to `diagnosis_embeddings` and must align with the table's `vector(N)` column (default 1536); changing it requires migrating the vector column.
- **Connectivity self-check**: click **Test Embedding Config** (`POST /api/v1/ai/test-embed`) in "AI Config" to validate endpoint / key / model instantly and echo back the actual returned dimension, avoiding mismatches that break RAG writes.

## Unified Message Center

Aggregates notifications from the SRE workflow and AI into a **single inbox**: incidents / alerts / SLO breaches / auto-remediation / AI inspections / tickets, with unread counts, deep links, and one-click read/all-read; persisted in PostgreSQL `kv_state`, survives refresh. Entry: the bell icon at top-right of the dashboard.

---

## Advanced Features

### Multi-Server Push

A single agent instance pushes to multiple monitoring servers simultaneously. **Collection executes once, results broadcast to all servers.**

**Configuration**: Use `servers` array in `config.json` (see Configuration Reference above), or check銆孧ulti-Server Push銆峣n the dashboard install dialog.

| Dimension | Description |
|---|---|
| Collection | Base + plugin metrics execute once, broadcast to all |
| Reporting | Concurrent push to each server, 8s timeout isolation |
| Auth | Each server independently validates fingerprint |
| Terminal | Each server has independent long-poll channel |
| Event retry | Re-queued only when all servers fail; one success = delivered |
| Connection pool | Each server has independent `http.Client` + pool |

> When `servers` is non-empty, it takes precedence; empty falls back to `server` + `token` (fully backward compatible).

### Gateway Relay Mode

When only one internal machine has internet access, install Agent in Relay mode on that machine: the relay service listens on a local port and reverse-proxies all internal Agent requests to the cloud server.

```bash
# 鈶?Gateway machine (internet-connected)
curl -fsSL "https://cloud-server/install-relay.sh?token=TOKEN" | sudo sh

# 鈶?Internal machine (via gateway)
curl -fsSL "http://<gateway-IP>:8529/install.sh?token=TOKEN" | sudo sh
```

> Relay and multi-server push are mutually exclusive: Relay = "one machine proxies all to one upstream"; multi-server = "one machine pushes to multiple upstreams".

### Machine Fingerprint Auth

Agent sends machine fingerprint (machine-id + primary MAC SHA-256 first 12 hex) to server at registration. All subsequent reports and terminal channel requests authenticate via fingerprint, **not install Token** 鈥?token rotation doesn't affect installed agents. Each server validates fingerprints independently in multi-server scenarios.

---

## Security Mechanisms

### Login & Authentication

- **Login auth**: Username + password (salted SHA-256) + session Cookie; login form doesn't pre-fill default admin
- **MFA two-factor**: Google Authenticator compatible TOTP; after enabling, login requires password + 6-digit code
- **Login rate-limiting**: Per-IP sliding window (default 8 failures per 5 min)
- **Session security**: `HttpOnly` + `SameSite=Lax`; `Secure` on HTTPS; password change clears all sessions

### Multi-User RBAC

- **admin**: Full access, including user management (create/edit/delete/reset password/unbind MFA)
- **operator**: All operations except user management (terminal/playbook/config/host deletion)
- **viewer**: View only; can manage own profile/password/MFA
- Route-level interception: every API request checked by `authMiddleware` 鈫?`routeAllowed`

### Account Recovery

- **Forgot username**: Enter bound email 鈫?receive username notification (anti-enumeration)
- **Forgot password**: Enter username 鈫?receive 6-digit code (10-min TTL) 鈫?reset after verification
- **MFA unbind via email**: Lost phone? Unbind MFA via bound email verification code
- Code security: 6-digit random, 10-min TTL, single-use, 60s send interval limit

### Agent & Data Security

- **Mandatory Agent Token** (default on): `register`/`report` must carry valid Token (constant-time compare)
- **Request body limit**: 100 MiB (covers port-forward file transfer), prevents oversized JSON memory exhaustion
- **Encryption at rest**: config MFA/SMTP/AI/webhook/relay/**SMS & voice call (AccessKey/SecretKey)** secrets sealed with AES-256-GCM derived from `AIOPS_SECRET_KEY`
- **Encryption in transit**: optional TLS (`AIOPS_TLS_CERT/KEY`); the agent supports self-signed CA trust (`--ca-cert` / `tls_skip_verify`)
- **Forced security initialization**: default admin/admin must go through a mandatory "change username + password" dialog on first login 鈥?not skippable
- **Security headers**: `nosniff`, `DENY` (anti-clickjacking), `no-referrer`
- **Secret masking**: Webhook/SMTP/AI-key/PostgreSQL-DSN masked on display, blank preserves original
- **Host identity anti-clone**: Cloned images with copied `agent_state.json` detected, `host_id` regenerated
- **Remote terminal dual auth**: Browser needs login session + terminal secondary password; open/close audited
- **For public exposure: place behind reverse proxy with HTTPS**

---

## Cross-Network Deployment

### Reverse Proxy / Domain Access (Nginx)

When exposing via domain + HTTPS through Nginx, regular monitoring works with default HTTP proxying; but **remote terminal** uses WebSocket upgrade + long-connection streaming, which Nginx doesn't forward by default, causing "metrics fine, terminal won't connect."

```nginx
# http {} block, once globally
map $http_upgrade $connection_upgrade { default upgrade; '' close; }

location / {
    proxy_pass http://127.0.0.1:8529;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host  $host;         # Auto-use domain in install command
    proxy_set_header X-Real-IP         $remote_addr;  # Real client IP (with trust_proxy)

    # 鈥斺€?Remote terminal essentials (all required) 鈥斺€?
    proxy_set_header Upgrade    $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_buffering         off;
    proxy_request_buffering off;
    proxy_read_timeout  3600s;
    proxy_send_timeout  3600s;
}
```

> Full example: [deploy/nginx-aiops.conf](deploy/nginx-aiops.conf). After editing: `nginx -t && nginx -s reload`.  
> Behind reverse proxy, set `"trust_proxy": true` in `server_config.json` to honor `X-Real-IP` for rate-limiting.  
> Cloud load balancers (ALB/CLB/K8s Ingress) similarly need WebSocket support, disabled buffering, idle timeout 鈮?h.

### Terminal Tunnel

Agent uses **active reverse connection**: server address is鍥哄寲 to `--server` at install time. Cross-network requires a **public-reachable domain or IP**. The dashboard install dialog auto-derives server address from current access URL 鈥?access via domain and the install command auto-uses that domain.

---

## FAQ / Troubleshooting

<details>
<summary><b>Agent report failure</b></summary>

- Check `--server` address is correct and server is running
- Check firewall/security group allows server port (default 8529)
- Check Agent logs for errors (`report failed: ...`)
</details>

<details>
<summary><b>Remote terminal won't connect</b></summary>

- **Behind Nginx**: must configure WebSocket upgrade headers and disable buffering (see Nginx config above)
- **Cross-network**: ensure Agent uses a public-reachable server address
- Confirm server doesn't have `terminal_disabled: true`
</details>

<details>
<summary><b>Terminal Chinese character garbled</b></summary>

- Windows ConPTY auto-applies `chcp 65001` + GBK鈫扷TF-8 conversion
- Playbook execution has 3-layer encoding: chcp 65001 + locale env vars + GBK鈫扷TF-8 API fallback
- Linux/macOS terminals default to UTF-8, no extra handling needed
</details>

<details>
<summary><b>Dashboard shows connection failed</b></summary>

- Check server is running: `curl http://localhost:8529/healthz`
- Check browser console for CORS or auth errors
- Try hard refresh (Ctrl+Shift+R)
</details>

<details>
<summary><b>Host shows offline</b></summary>

- Default 30s no report = offline; adjust `offline_after_sec` in alert settings
- Check Agent process: `ps aux | grep aiops-agent` (Linux) or Task Manager (Windows)
- Check Agent-to-server network connectivity
</details>

<details>
<summary><b>GPU info not showing</b></summary>

- NVIDIA GPU requires `nvidia-smi` installed
- AMD GPU (Linux) requires sysfs permissions
- macOS only supports Apple Silicon GPU monitoring
- GPU is best-effort 鈥?no tool = no display, doesn't affect other metrics
</details>

---

## Tech Stack & Architecture

### Tech Stack

| Component | Technology |
|---|---|
| **Relational storage** | PostgreSQL (config / users / audit / incidents / tickets / sessions / secrets) |
| **Time-series storage** | VictoriaMetrics (metrics / trends / SLO) |
| Agent core | Go 1.22+, pure stdlib, zero third-party deps |
| Server | Go 1.22+, `net/http` (Go 1.22 routing), `embed` for dashboard |
| Dashboard | Vanilla HTML/CSS/JS, no framework deps |
| Plugin layer | Python 3 + psutil (optional) |
| Alert push | Feishu/DingTalk Webhook + Email SMTP + multi-cloud SMS + multi-cloud Voice call (`net/smtp` + Aliyun / Huawei / Tencent SMS & TTS voice) |
| PWA | manifest.json + Service Worker + icon.svg |

### Architecture Diagram

```
                鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€ Go Agent Core 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
                鈹? Collector (tri-platform native) 鈫?base       鈹?
                鈹? PluginRunner 鈫?concurrent Python plugins     鈹?
                鈹? Reporter 鈫?broadcast to all servers           鈹?
  Report 鈹€HTTP鈹€鈻衡攤  Terminal 鈫?per-server reverse channel        鈹?
                鈹? Shares types with server via shared/          鈹?
                鈹斺攢鈹€鈹�攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
                   鈹?                         鈹?
              鈹屸攢鈹€鈹€鈹€鈹粹攢鈹€鈹€鈹€鈹?              鈹屸攢鈹€鈹€鈹€鈹€鈹粹攢鈹€鈹€鈹€鈹€鈹?
              鈹?Server A 鈹?              鈹? Server B  鈹? (multi-server push)
              鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?              鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
                                               鈹?subprocess + JSON
                    鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹尖攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
              鈹屸攢鈹€鈹€鈹€鈹€鈹粹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹?         鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹粹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹?      鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹粹攢鈹€鈹€鈹€鈹€鈹€鈹€鈹?
              鈹?Custom       鈹?         鈹?AI / Anomaly   鈹?      鈹?Process      鈹?
              鈹?collection   鈹?         鈹?detection      鈹?      鈹?Monitor      鈹?
              鈹?(.py)        鈹?         鈹?(.py)          鈹?      鈹?(.py)        鈹?
              鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?         鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?      鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹?
```

**Design principle**: High-frequency, performance-sensitive base collection uses Go (single binary, zero deps); variable, ecosystem-dependent custom/AI logic uses Python. Process boundary isolates each.

### Directory Structure

```
aiops-monitor/
鈹溾攢鈹€ go.mod                          # Go module
鈹溾攢鈹€ shared/
鈹?  鈹斺攢鈹€ wire.go                     # 鈽?Shared types (Agent 鈫?Server contract)
鈹溾攢鈹€ cmd/
鈹?  鈹溾攢鈹€ server/                     # Go server
鈹?  鈹?  鈹溾攢鈹€ main.go                 # Entry, routing, middleware
鈹?  鈹?  鈹溾攢鈹€ handlers.go             # API handlers
鈹?  鈹?  鈹溾攢鈹€ store.go                # In-memory store + multi-level downsampling
鈹?  鈹?  鈹溾攢鈹€ pgstore.go              # PostgreSQL store (all relational data)
鈹?  鈹?  鈹溾攢鈹€ vm.go                   # VictoriaMetrics writer/reader (all time-series)
鈹?  鈹?  鈹溾攢鈹€ crypto.go               # AES-256-GCM secret encryption at rest
鈹?  鈹?  鈹溾攢鈹€ logstore.go             # Log aggregation + search
鈹?  鈹?  鈹溾攢鈹€ aiops.go                # AI inspection + heuristic diagnosis
鈹?  鈹?  鈹溾攢鈹€ incident.go/slo.go/ticket.go/remediation.go  # SRE hub
鈹?  鈹?  鈹溾攢鈹€ message.go              # Notification message center
鈹?  鈹?  鈹溾攢鈹€ alerts.go               # Threshold alert engine
鈹?  鈹?  鈹溾攢鈹€ auth.go                 # Login auth + MFA + RBAC
鈹?  鈹?  鈹溾攢鈹€ users.go                # Multi-user management
鈹?  鈹?  鈹溾攢鈹€ check.go                # Custom monitoring (HTTP/TCP/Ping/process)
鈹?  鈹?  鈹溾攢鈹€ ws.go                   # Hand-written WebSocket (terminal)
鈹?  鈹?  鈹溾攢鈹€ terminal.go             # Remote terminal relay
鈹?  鈹?  鈹溾攢鈹€ notify.go               # Feishu/DingTalk/Email/SMS/Voice push
鈹?  鈹?  鈹溾攢鈹€ email.go                # SMTP + verification code manager
鈹?  鈹?  鈹溾攢鈹€ playbook.go             # Automation playbook engine
鈹?  鈹?  鈹溾攢鈹€ totp.go                 # TOTP two-factor auth
鈹?  鈹?  鈹溾攢鈹€ config.go               # Config persistence
鈹?  鈹?  鈹溾攢鈹€ install.go              # One-click install script generation
鈹?  鈹?  鈹斺攢鈹€ web/                    # Dashboard frontend (embedded at compile time)
鈹?  鈹?      鈹溾攢鈹€ index.html / app.js / style.css
鈹?  鈹?      鈹溾攢鈹€ manifest.json / sw.js / icon.svg
鈹?  鈹斺攢鈹€ agent/                      # 鈽?Go Agent core
鈹?      鈹溾攢鈹€ main.go                 # Config / flags / signals
鈹?      鈹溾攢鈹€ collector.go            # Collector interface
鈹?      鈹溾攢鈹€ collector_linux.go      # Linux native collection
鈹?      鈹溾攢鈹€ collector_windows.go    # Windows native collection
鈹?      鈹溾攢鈹€ collector_darwin.go     # macOS native collection
鈹?      鈹溾攢鈹€ collector_other.go      # Other platform stub
鈹?      鈹溾攢鈹€ gpu.go                  # GPU collection (tri-platform)
鈹?      鈹溾攢鈹€ terminal.go             # Remote terminal Agent-side
鈹?      鈹溾攢鈹€ pty_windows.go          # Windows ConPTY
鈹?      鈹溾攢鈹€ pty_unix.go             # Linux/macOS openpty
鈹?      鈹溾攢鈹€ pty_linux.go / pty_darwin.go
鈹?      鈹溾攢鈹€ relay.go                # Gateway relay mode
鈹?      鈹溾攢鈹€ plugins.go              # Plugin runner
鈹?      鈹溾攢鈹€ identity.go             # Stable host_id / fingerprint
鈹?      鈹斺攢鈹€ reporter.go             # Dual-heartbeat reporting
鈹溾攢鈹€ plugins/                        # 鈽?Python plugin layer
鈹?  鈹溾攢鈹€ plugin_sdk.py               # Plugin SDK
鈹?  鈹溾攢鈹€ core_metrics.py             # psutil fallback
鈹?  鈹溾攢鈹€ example_service_check.py    # Example: service probe
鈹?  鈹溾攢鈹€ example_ai_anomaly.py       # Example: anomaly detection
鈹?  鈹溾攢鈹€ process_monitor.py          # Process monitoring
鈹?  鈹斺攢鈹€ requirements.txt
鈹溾攢鈹€ deploy/
鈹?  鈹斺攢鈹€ nginx-aiops.conf            # Nginx reverse proxy example
鈹溾攢鈹€ dist/                           # Agent distribution (platform binaries)
鈹溾攢鈹€ bin/                            # Pre-compiled binaries
鈹溾攢鈹€ config.example.json             # Agent config example
鈹溾攢鈹€ server_config.example.json      # Server config example
鈹溾攢鈹€ Dockerfile                      # Multi-stage build
鈹溾攢鈹€ docker-compose.yml              # Docker Compose
鈹斺攢鈹€ INSTALL.md                      # Detailed installation guide
```

### Key Design

- **Shared code**: `shared/wire.go` imported by both server and agent 鈥?contract never drifts
- **Dual-heartbeat**: Base metrics high-frequency; plugins low-frequency, results sent alongside
- **Process isolation**: Plugins run as subprocesses, timeout killable, one bad plugin doesn't crash core
- **Alert dedup**: Only pushes on "new trigger" and "recover" transitions, persistent alerts don't spam
- **Multi-level downsampling**: Raw (~1.5h) / 1-min aggregate (48h) / 5-min aggregate (7 days)
- **Unified storage**: relational data (config / users / audit / incidents / tickets / sessions) in PostgreSQL, time-series (metrics / trends / SLO) in VictoriaMetrics; embedded aiops.db fully retired (the in-memory tiered window is a hot cache only)
- **gzip compression**: Multi-host polling JSON compresses ~8-10x; WebSocket upgrades auto-skipped

---

## Performance & Scale

- **Bandwidth**: gzip ~8-10x compression, 3000 hosts polling `/hosts` every 3s drops from MB/s to ~100KB/s
- **Report throughput**: 3000 hosts 脳 every 10s 鈮?300 writes/s, `Upsert` briefly holds write lock
- **Memory**: ~1-2 MB per host for 3-layer history, 3000 hosts 鈮?4-7 GB (tunable via retention constants)
- **Rendering**: Host list paginated (9/page), DOM only renders current page
- **Tuning**: Increase `--interval` (e.g. 10-15s) for large fleets to reduce bandwidth

> **Conclusion**: gzip + pagination + multi-level downsampling + persistence enables single-instance support for ~3000 hosts. For 10k+, consider external time-series DB (VictoriaMetrics).

---

## API Reference

<details>
<summary>Expand full API list</summary>

| Method | Path | Description |
|---|---|---|
| **Agent Communication** | | |
| POST | `/api/v1/agent/register` | Agent registration |
| POST | `/api/v1/agent/report` | Report (base + custom + events) |
| **Host Management** | | |
| GET | `/api/v1/hosts` | Host list (with metrics, online status) |
| GET | `/api/v1/hosts/meta` | Host metadata (id + hostname) |
| GET | `/api/v1/hosts/{id}/metrics` | Single host base metric history |
| GET | `/api/v1/hosts/{id}/history` | Single host time-series (auto-select layer) |
| POST | `/api/v1/hosts/{id}/category` | Set host category |
| DELETE | `/api/v1/hosts/{id}` | Delete host |
| **Alerts & Events** | | |
| GET | `/api/v1/alerts` | Threshold + custom monitor alerts |
| GET | `/api/v1/events` | Plugin events |
| GET | `/api/v1/activity` | Activity log |
| GET | `/api/v1/summary` | Summary statistics |
| **Custom Monitoring** | | |
| GET | `/api/v1/checks` | Custom monitor list |
| POST | `/api/v1/checks` | Add/update monitor |
| POST | `/api/v1/checks/{id}/run` | Trigger immediate probe |
| GET | `/api/v1/checks/{id}/history` | Check history time-series |
| DELETE | `/api/v1/checks/{id}` | Delete monitor |
| **Automation** | | |
| GET | `/api/v1/playbooks` | Playbook list |
| POST | `/api/v1/playbooks` | Create/update playbook |
| DELETE | `/api/v1/playbooks/{id}` | Delete playbook |
| POST | `/api/v1/playbooks/{id}/execute` | Execute playbook |
| GET | `/api/v1/playbooks/executions` | Execution history |
| GET | `/api/v1/playbooks/executions/{id}` | Execution details |
| **Terminal** | | |
| GET | `/api/v1/terminal/sessions` | Active session list |
| GET | `/api/v1/terminal/sessions/{id}/replay` | Session recording playback |
| GET | `/api/v1/terminal/sessions/{id}/observe` | Read-only observe (WebSocket) |
| GET | `/api/v1/hosts/{id}/terminal` | Browser WebSocket terminal |
| GET | `/api/v1/agent/terminal/wait` | Agent long-poll |
| GET | `/api/v1/agent/terminal/rx` | Server 鈫?Agent frame stream |
| POST | `/api/v1/agent/terminal/tx` | Agent 鈫?Server output stream |
| **Config Management** | | |
| GET | `/api/v1/config` | Get alert config (masked) |
| POST | `/api/v1/config` | Update alert config |
| POST | `/api/v1/config/test` | Send test message |
| **Auth & Account** | | |
| POST | `/api/v1/login` | Login |
| POST | `/api/v1/logout` | Logout |
| GET | `/api/v1/me` | Current user info |
| POST | `/api/v1/profile` | Update profile |
| POST | `/api/v1/password` | Change password |
| POST | `/api/v1/mfa/setup` | Generate MFA secret + QR URI |
| POST | `/api/v1/mfa/enable` | Enable MFA |
| POST | `/api/v1/mfa/disable` | Disable MFA |
| POST | `/api/v1/mfa/unbind-via-email` | Unbind MFA via email |
| **Account Recovery** | | |
| POST | `/api/v1/account/recover-username` | Recover username |
| POST | `/api/v1/account/send-reset-code` | Send reset code |
| POST | `/api/v1/account/reset-password` | Reset password |
| **User Management (admin)** | | |
| GET | `/api/v1/users` | User list |
| POST | `/api/v1/users` | Create user |
| POST | `/api/v1/users/{username}` | Update user |
| DELETE | `/api/v1/users/{username}` | Delete user |
| POST | `/api/v1/users/{username}/reset-password` | Reset password |
| POST | `/api/v1/users/{username}/reset-mfa` | Unbind MFA |
| **Install Distribution** | | |
| GET | `/api/v1/install/info` | Install info |
| POST | `/api/v1/install/reset-token` | Reset Token |
| GET | `/install.sh` / `/install.ps1` | Install scripts |
| GET | `/uninstall.sh` / `/uninstall.ps1` | Uninstall scripts |
| **Other** | | |
| **SRE 路 Incidents** | | |
| GET / POST | `/api/v1/incidents` | List / create incident |
| GET | `/api/v1/incidents/{id}` | Incident detail (with timeline) |
| POST | `/api/v1/incidents/{id}/ack` 路 `/resolve` 路 `/comment` 路 `/ticket` 路 `/diagnose` | Ack / resolve / comment / escalate to ticket / AI diagnosis |
| **SRE 路 Auto-remediation** | | |
| GET / POST | `/api/v1/remediation/rules` | List / upsert rules |
| DELETE | `/api/v1/remediation/rules/{id}` | Delete rule |
| GET | `/api/v1/remediation/runs` | Run history |
| POST | `/api/v1/remediation/runs/{id}/approve` 路 `/reject` | Approve & run / reject pending remediation |
| **SRE 路 SLO** | | |
| GET / POST | `/api/v1/slos` | List (with SLI / error budget) / upsert |
| DELETE | `/api/v1/slos/{id}` | Delete SLO |
| **SRE 路 Tickets** | | |
| GET / POST | `/api/v1/tickets` | List / create ticket |
| GET / POST / DELETE | `/api/v1/tickets/{id}` | Detail / update / delete |
| POST | `/api/v1/tickets/{id}/comment` | Add comment |
| **Log Aggregation** | | |
| POST | `/api/v1/agent/logs` | Agent log ingest (fingerprint-authed) |
| GET | `/api/v1/logs` | Log search (`host` / `level` / `q` / `since_min` / `limit`) |
| **AI** | | |
| GET / POST | `/api/v1/ai/config` | Get / save AI provider config |
| GET | `/api/v1/ai/inspections` | Inspection reports |
| POST | `/api/v1/ai/inspect` | Run an inspection now |
| **API Monitoring** | | |
| GET | `/api/v1/apimon/systems` | Business systems (live status + VM aggregates) |
| POST | `/api/v1/apimon/systems` | Add / update business system |
| POST | `/api/v1/apimon/systems/{id}/run` | Probe now |
| DELETE | `/api/v1/apimon/systems/{id}` | Delete business system |
| GET | `/api/v1/apimon/endpoints/{id}/history` | Endpoint history |
| **Alert Governance** | | |
| GET | `/api/v1/alerts/governance` | Governance rules (silence/inhibit/route) |
| POST | `/api/v1/alerts/governance` | Save governance rules |
| **AI Ops Assistant** | | |
| POST | `/api/v1/ai/chat` | AI chat (SSE streaming) |
| POST | `/api/v1/ai/diagnose` | Incident AI root-cause diagnosis |
| GET | `/api/v1/ai/inspections` | Inspection reports |
| **Message Center** | | |
| GET | `/api/v1/messages` | Messages + unread count (incidents / AI / remediation / tickets) |
| POST | `/api/v1/messages/read` 路 `/read-all` | Mark read / mark all read |
| **Other** | | |
| GET | `/` | Web dashboard |
| GET | `/healthz` | Health check |
| GET | `/dl/*` | Agent binary downloads |

</details>

---

## Roadmap

### Implemented

- [x] Go Agent core: tri-platform native collection + stable identity + dual-heartbeat + disconnect re-queue
- [x] GPU monitoring: NVIDIA / AMD / Apple, best-effort + cache
- [x] Python plugin layer + SDK + examples (service probe / anomaly detection / process monitor / psutil fallback)
- [x] Go server: in-memory store + multi-level downsampling + embedded persistence (survives restart)
- [x] Custom monitoring: HTTP / TCP / Ping / process; list路pill dual view + history curves
- [x] Interactive trend charts: hover crosshair + drag-zoom + enlarge preview
- [x] Auth & security: salted password + rate-limiting + mandatory Token + security headers + secret masking + anti-clone
- [x] MFA two-factor (TOTP) + account recovery (email code) + MFA unbind via email
- [x] Email alert push (SMTP)
- [x] Real-time dashboard: overview + TOP10 + category grouping/search/pagination + card路list dual view + wide toggle
- [x] Alert push: Feishu / DingTalk + Email, dedup + state transition
- [x] gzip compression + PWA installable + mobile responsive
- [x] Multi-select category filter + collapse + keyboard shortcuts
- [x] Remote terminal: reverse connection + full TTY + multi-tab + recording playback + read-only observe + command audit
- [x] Automation playbooks: multi-step orchestration + batch parallel + dedicated execution channel + 3-layer encoding fix
- [x] Multi-user RBAC: three roles + user management UI + route-level interception
- [x] Multi-server push: collect once broadcast all + independent auth/retry/connection pool
- [x] Gateway relay mode: auto-tunnel binary/report/terminal
- [x] Machine fingerprint auth: token rotation doesn't affect installed agents
- [x] One-click install: auto-detect architecture + download + config + boot autostart
- [x] Alert governance: silence (time/weekday) / inhibit (root-cause suppresses derived) / route (by level/host to channels)
- [x] Alert notification channels expanded: **multi-cloud SMS + multi-cloud Voice call** (Aliyun / Huawei / Tencent, SMS & TTS voice), working alongside Feishu / DingTalk / Email
- [x] Custom alert thresholds: 27 fine-grained warn/crit pairs (host / probe / API / task / forward), zero-value auto-backfill to defaults
- [x] Decoupled embedding model: standalone RAG embedding config (endpoint / key / model / dimension), any OpenAI-compatible `/embeddings` + one-click self-check
- [x] API monitoring: batch black-box probes for business-system APIs (availability / latency / P95 / throughput)
- [x] AI Ops Assistant: pluggable-LLM inspection diagnosis + RAG similar cases + autonomous Agent (Function Calling)
- [x] Unified Message Center: single inbox for incidents / alerts / SLO / auto-remediation / AI / tickets
- [x] Security hardening: SSRF outbound guard (safedial), log AES-256-GCM encrypted upload, pgvector RAG diagnosis embeddings
- [x] Agent enhancements: log collection (encrypted upload), Agent-Server TLS CA trust, ZMODEM file transfer, machine-fingerprint anti-clone

### In Progress / Planned

- [ ] Ultra-large scale (10k+): external time-series DB (VictoriaMetrics), server-side pagination/incremental, configurable retention
- [ ] Plugin enhancements: per-plugin interval, plugin-level config, metric types (counter/histogram)
- [ ] AIOps evolution: time-series anomaly detection (Prophet / statsmodels), alert noise reduction, root cause analysis, capacity forecasting
- [ ] Intelligent ops assistant: integrate RAGFlow + Dify + local vLLM

---

## License

MIT
