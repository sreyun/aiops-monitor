<div align="center">

# AIOps

**One binary for observability · alerting · self-healing · AI diagnosis · SRE closed-loop · remote control.**

[![Version](https://img.shields.io/badge/Version-v0.19.39-blue)](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.39)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#open-source--community)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

**[中文](README.md) · [English](README_EN.md) · [日本語](README.ja.md) · [한국어](README.ko.md)**

</div>

> **Single-binary server + zero-dependency agent**: one command stands up observability, alert governance, automated remediation, AI inspection/diagnosis, SRE closed-loop, remote desktop/terminal, SQL toolkit, and security center. 100% open source, self-hosted, data fully owned — no SaaS dependency, no telemetry uplink.

**Current release [v0.19.39](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.39)** · Mirrors: [GitHub](https://github.com/sreyun/aiops-monitor) / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## Contents

- [Why AIOps](#why-aiops-monitor)
- [Highlights in v0.19.0](#highlights-in-v0190)
- [Capability Map](#capability-map)
- [Core Capabilities](#core-capabilities)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Typical Scenarios](#typical-scenarios)
- [Documentation](#documentation)
- [Honest Boundaries](#honest-boundaries)
- [Enterprise Services](#enterprise-services)
- [Open Source & Community](#open-source--community)
- [License](#license)

---

## Why AIOps

Ops stacks keep growing while incidents get harder: metrics, logs, alerts, and changes live in different systems. Commercial products often meter by host or module — and keep your data in their cloud.

AIOps consolidates the common path into **one self-hosted platform**:

| Principle | How |
|---|---|
| **Fewer components** | One Go server + one dependency-free agent covers the usual Zabbix / Prometheus / Grafana / Alertmanager / runbook / terminal / remote-desktop path |
| **Fast deploy** | `docker compose up -d` for the full stack; one-click agent install commands from the UI |
| **Data ownership** | PostgreSQL (relational / audit / vector memory) + VictoriaMetrics (time series) — exportable and auditable |
| **Pluggable AI** | Any OpenAI-compatible model for smart mode; heuristic fallback when none is configured |
| **Measurable closed loop** | Not just “a diagnosis” — dry-run → propose → approve → verify → promote, with ops effect KPIs |

---

## Highlights in v0.19.0

Building on v0.18.9, this release pushes from “can diagnose” to “can verify, gate, and learn”:

| Area | What landed |
|---|---|
| **Incident loop** | One-click `Dry-run → Propose → Approve → Verify → Promote Skill`; evidence gate on propose; verify checks host/alert/remediation/service; case-pack export |
| **Remote gate** | Terminal/desktop blocked under freeze or high-risk open incidents unless authorized; unified `remote-preflight`; admin break-glass; sessions audit `change_id` / `incident_id` |
| **Change & services** | Recurring freeze windows (daily/weekly); business-service tree & impact; emergency change SoD (author cannot self-approve) |
| **Memory depth** | Memories scoped by service / host category with `verified`; retrieval boosts verified knowledge; resolution/verify reinforce; UI filters by verification |
| **Agent learning** | Hermes multi-tool turns: only high-quality turns mint **draft** Skills; activate after verify/human accept; write tools require approval by default |
| **Ops effect** | `GET /api/v1/sre/effect`: MTTR/MTTA, alert noise, change failure rate, closed-loop rate, AI adoption/verify, skill·memory hits and draft/active |

Milestones: `v0.18.9` lightweight ITSM · `v0.18.2` Windows lock-screen Ctrl+Alt+Del · `v0.18.0` controllable security loop · `v0.17` resource layer (Hyper-V/containers/K8s). Full notes: [Releases](https://github.com/sreyun/aiops-monitor/releases).

Optional POC checklist: [docs/year1-acceptance.md](docs/year1-acceptance.md)

---

## Capability Map

```
Observe          Govern           Remediate         Diagnose
─────────        ──────           ─────────         ──────────
Hosts/GPU/logs   Silence/inhibit  Playbooks/gates   Streaming RCA
Probes/Redfish   Multi-channel    Auto-remediation  Scoped RAG/Skills
NetFlow/storage  Security findings SLO/tickets      WeKnora docs

Control          Data             Secure            Operate
────────         ───────          ──────            ────────
Terminal/desktop Multi-DS/EXPLAIN RBAC/MFA/fingerprint Effect KPIs
Port forward     SQL change gate  Security center   Compose one-shot
```

---

## Core Capabilities

### 1. Full-stack observability

- **Four-platform native collection**: Linux / Windows / macOS / Kylin; pure-Go standard-library agent with **zero third-party deps**; CPU / mem / disk / net / load / processes / GPU (NVIDIA·AMD·Apple).
- **Active probes**: HTTP (incl. TLS days left) / TCP / Ping / UDP / process / OpenAPI / multi-point.
- **Out-of-band & traffic**: Redfish (deep Huawei iBMC), NetFlow v5/v9/IPFIX, Huawei OceanStor.
- **Logs**: incremental agent tail, AES-256-GCM uplink, full-text search.
- **Deep host inspect** (`host_inspect`): OS / kernel / NICs / disks / services; playbook-persistable.
- **Interactive charts**: Canvas crosshair, box-zoom, 1h–30d ranges.

### 2. Resource closed loop

- Hyper-V / Docker·Podman / Kubernetes inventory (nodes, pods, deployments, events).
- **Global resource search** across hosts / VMs / containers / K8s.
- **Topology** with auto-discover for blast radius / RCA.
- **Business services**: bind hosts; query open incidents and recent change impact.

### 3. Alert governance

- Three threshold presets (Conservative / Standard / Relaxed).
- **Silence → Inhibit → Route**: criticals to phone/SMS, warnings to IM.
- Channels: Feishu / DingTalk / SMTP / multi-cloud SMS + TTS; one fire + one resolve.
- Security scan findings are trackable, cancellable, and closable.

### 4. Automation & SRE closed loop

- **Runbooks**: shell + built-in modules, preflight, concurrency, when/vars, retry, reverse rollback, live output + audit.
- **High-risk gate**: scheduled high-risk runs enter `pending_approval`; allowlists / dangerous-command blocks / guardrails.
- **Incident loop**: alerts / SLO / manual → timeline → ack / resolve / escalate; **one-click loop** (dry-run / propose / approve / verify / promote).
- **Change management**: freeze windows (incl. recurring), emergency SoD, correlate recent changes; freeze blocks unauthorized remediation and remote access.
- **SLO** multi-window multi-burn-rate; **on-call** + escalation.
- **Tickets**: escalate from incidents; assign real directory users; image/file attachments.
- **Lightweight ITSM**: service requests / change state machine, OpsLink, SQL↔change linking.

### 5. AI inspection & diagnosis

- Scheduled / on-demand health inspection; critical incidents can auto-diagnose on the timeline.
- **Live evidence refresh** + strong evidence gate (heartbeat-only is not enough to propose).
- **RAG memory** (pgvector): service/category scope; `verified` boost; 👍/👎 reinforce/penalize.
- **Skills**: high-quality multi-tool turns mint drafts; activate after verify/human accept; versioning, scope, customer pack import/export.
- **WeKnora** external docs with local fallback.
- Multimodal assistant: SSE, function calling, images/files/URLs; Web speech I/O when the browser supports it.
- Pluggable models; decoupled embed/chat/rerank; AI Runs / fallback / tool turns observable.

### 6. Remote desktop & terminal

- **Terminal** via agent reverse tunnel (no inbound ports) + session replay.
- **Web remote desktop**: JPEG / H.264; multi-monitor; quality presets; file transfer & clipboard.
- **Windows lock screen**: service + desktop worker follow Winlogon; Ctrl+Alt+Del via Session-0 SAS pipe (agent ≥ v0.18.2); unlock credentials stay in memory.
- **Remote gate**: freeze or high-risk hosts require preflight; admin break-glass is audited.
- **Port forward / `/proxy`**: jump-host to other LAN services.

### 7. SQL toolkit

- Multi-datasource, schema / history, EXPLAIN diff.
- High-risk SQL goes through the change gate; PostgreSQL read-only probes and controlled ops actions (see [docs/ci-gate.md](docs/ci-gate.md)).

### 8. Security & compliance

- Session cookies: PBKDF2-HMAC-SHA256 (600k); `HttpOnly` / `SameSite` / `Secure` on HTTPS.
- RBAC: admin / operator / viewer; optional TOTP MFA.
- Terminal / desktop second password; IP + account anti-bruteforce; machine fingerprint anti-clone.
- Security center (host / web scans); SSRF egress guards; `AIOPS_SECRET_KEY` AES-GCM at rest; optional TLS.

### 9. Mobile & Web

- **Android / HarmonyOS**: enterprise native consoles are **externally distributed** (mobile source is not in this repo).
- **Web / PWA**: dark professional UI; global AI entry; effect dashboard; memory / skill browsers; admin Data & Backup.

### 10. Deployment resilience

- **Two mandatory stores**: missing PostgreSQL or VictoriaMetrics refuses to boot.
- Versioned schema migrations; gateway relay; agent multi-server fan-out (circuit breaker + gzip degrade).
- Install-token rotation + 7-day grace; amd64 / arm64 images.

---

## Architecture

```
┌──────────────── Collection (zero-dep Go Agent) ────────────────┐
│ Native metrics / GPU / encrypted logs · probes · Redfish       │
│ NetFlow · OceanStor · Hyper-V / containers                     │
│ Terminal · desktop worker (Windows service follows Winlogon)   │
│ Fingerprint · Relay · multi-server fan-out                     │
└───────┬────────────────────────────────────┬───────────────────┘
        │ report / probe / terminal / desktop │ servers[] fan-out
        ▼                                    ▼
┌──────────────────── Server (single Go binary) ─────────────────┐
│ Alerts → governance → incident loop → playbooks → SLO → tickets│
│ AI + scoped RAG / Skills · SQL · security center               │
│ Remote gate · topology RCA · resource search · effect KPIs     │
│  ┌──── Two mandatory stores (missing either = no boot) ────┐   │
│  │ PostgreSQL · VictoriaMetrics                            │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬────────────────────────────────────┘
                           │ REST + WebSocket
                           ▼
              ┌──── Web / Android / HarmonyOS ────┐
              │ overview · hosts · alerts · desk   │
              │ SRE · AI · SQL · security · K8s    │
              └────────────────────────────────────┘
```

**Division of labor**: high-frequency collection is pure Go; protocol collectors are polled remotely; Windows lock-screen desktop needs **LocalSystem service + in-session worker**.

---

## Quick Start

### Docker Compose

| File | Use case |
|---|---|
| `docker-compose.yml` | **Production**: prebuilt images; agent optional (`--profile agent`) |
| `docker-compose.dev.yml` | **Dev overlay**: local build; agent on by default |

**Production (recommended)**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh)
docker compose up -d
```

**Development (from a source checkout)**

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

Open `http://localhost:8529`. Default `admin / admin` — **first login forces security initialization**; enable MFA afterward.

> `secure-compose.sh` writes strong random `POSTGRES_PASSWORD` and `AIOPS_SECRET_KEY` into `.env` (do not commit). If default image pulls fail, override `POSTGRES_IMAGE` / `VM_IMAGE` or use the development overlay.

### Install the Agent

```bash
# Linux (root)
curl -fsSL "http://<server>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows (admin PowerShell) — service install for lock-screen desktop
irm "http://<server>:8529/install.ps1?token=<TOKEN>" | iex
# After upgrades: aiops-agent --install-service
```

The server **requires** both PostgreSQL and VictoriaMetrics. See [INSTALL.md](INSTALL.md) and [`config.example.yaml`](config.example.yaml).

### Windows remote desktop checklist

1. Agent must be installed as a **Windows service** (LocalSystem).  
2. Agent ≥ **v0.18.2** with `--install-service` for the SAS pipe.  
3. In the Web desktop: **Ctrl+Alt+Del** → **Unlock** (credentials stay in memory; audits omit plaintext).  
4. If Application Control blocks install, allowlist the binary first.

---

## Typical Scenarios

| Scenario | How AIOps helps |
|---|---|
| Unified DC monitoring | Hundreds of cross-platform hosts, three threshold presets |
| Alert storm governance | Silence + inhibit + route |
| SLA / availability | API probes + multi-window burn-rate SLO |
| Self-healing | Runbooks behind approval gates |
| One-click incident loop | Evidence → dry-run → propose → approve → verify → Skill |
| Change-window emergency | Recurring freeze + emergency SoD + remote gate |
| Lock-screen rescue | Windows service agent + Web desktop CAD / unlock |
| Smart RCA + learning | LLM + topology + scoped / verified memory |
| Ticket evidence loop | Escalate → assign real users → attach proof |
| On-call from outside | Native Android / HarmonyOS consoles (external packages) |
| Cross-segment / weak net | Relay + multi-server fan-out + circuit breaker |

---

## Documentation

| Doc | Description |
|---|---|
| [INSTALL.md](INSTALL.md) / [INSTALL_EN.md](INSTALL_EN.md) | Install, binary run, reverse proxy, relay, upgrade/uninstall |
| [USER_GUIDE.md](USER_GUIDE.md) | End-user guide (features & scenarios) |
| [DEPLOY_GUIDE.md](DEPLOY_GUIDE.md) / [DEPLOY_GUIDE_EN.md](DEPLOY_GUIDE_EN.md) | Advanced deployment |
| [FORWARD_GUIDE.md](FORWARD_GUIDE.md) | Port forward / jump-host |
| [config.example.yaml](config.example.yaml) | Full agent configuration sample |
| [docs/ci-gate.md](docs/ci-gate.md) | CI / SQL / AI / closed-loop gate notes |
| [docs/year1-acceptance.md](docs/year1-acceptance.md) | POC acceptance checklist & KPI definitions |
| [Releases](https://github.com/sreyun/aiops-monitor/releases) | Release notes |

Android / HarmonyOS clients are externally distributed packages (mobile source is not in this repo).

---

## Honest Boundaries

**Platform**

- Mandatory PostgreSQL + VictoriaMetrics; ~3,000 hosts per single instance before externalizing VM.
- Without an LLM, heuristic fallback is shallower than model-backed diagnosis.
- Web speech depends on the browser Web Speech API (best on Chrome / Edge).
- Ticket attachments are JSON snapshots — fine for evidence screenshots, not an object store.

**Remote desktop**

- Windows lock / logoff / UAC need the agent **service install**.
- Secure-desktop worker uses GDI/JPEG; FPS depends on the network.
- macOS / Linux are limited by OS permissions and graphical sessions.
- Unlock sends in-memory credentials only — use in trusted ops contexts.

**Mobile**

- Private APK / app distribution (not on app stores); account self-service stays on the Web.
- Plain DataStore cookie persistence; fixed polling; no FCM.

---

## Enterprise Services

The core is 100% MIT open source. Optional services on top of the OSS edition:

- Large-scale private deployment consulting (sharding, external TSDB, retention)
- Custom integrations (WeCom / DingTalk / Feishu, CMDB, internal LLM gateways)
- Security hardening (SSO / LDAP, audit retention, compliance baselines)
- Private Android signing and distribution

Open an Issue on GitHub / Gitee or contact the maintainer.

---

## Open Source & Community

- **License**: MIT — no feature gating, no host limits, no telemetry.
- **Scale (approx.)**: `cmd/server` 290+ Go files, `cmd/agent` 120+ files, **150+** automated tests.
- **Dual remotes**: GitHub and Gitee stay in sync for branches and tags.
- Contributions welcome: issues, PRs, docs, plugins.

| Resource | Link |
|---|---|
| GitHub | <https://github.com/sreyun/aiops-monitor> |
| Gitee | <https://gitee.com/bigdatasafe/aiops-monitor> |
| Releases | <https://github.com/sreyun/aiops-monitor/releases> |

---

## License

MIT — free to use, modify, and redistribute; see repository notices and release notes.
