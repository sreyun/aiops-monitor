<div align="center">

# AIOps Monitor

**One binary that replaces 5+ ops toolchains — an open-source full-stack observability and SRE platform.**

</div>

<div align="center">

[![Version](https://img.shields.io/badge/Version-v0.18.9-blue)](https://github.com/sreyun/aiops-monitor/releases)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#open-source--community)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

**[中文](README.md) · [English](README_EN.md)**

</div>

> **Single-binary server + zero-dependency agent**: one command stands up *observability · alert governance · automated remediation · AI diagnosis · SRE closed-loop · remote desktop · SQL toolkit · security center · Android console*. 100% open source, self-hosted, data fully owned — no SaaS dependency, no telemetry uplink.

**Current release [v0.18.9](https://github.com/sreyun/aiops-monitor/releases/tag/v0.18.9)** · Mirrors: GitHub / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## Why AIOps Monitor

Monitoring tools keep piling up, yet incidents get harder to diagnose: metrics in one system, logs in another, alert storms flooding the inbox, root cause found by hand. Most commercial offerings meter by host count or feature module — and keep your data in their cloud.

AIOps Monitor takes a different path — **consolidating monitoring, alerting, automation, AI diagnosis, the SRE workflow, remote control, and a mobile console into one self-hosted platform**:

- **Less is more**: one Go server binary + one dependency-free agent covers the common ground of Zabbix / Prometheus / Grafana / Alertmanager / runbook automation / terminal gateway / remote desktop — five fewer components to maintain.
- **Deploy in one command**: `docker compose up -d` brings up the full stack; agent installs in one click with native cross-platform collection.
- **Data ownership**: relational data lands in PostgreSQL, time-series in VictoriaMetrics — **both open-source databases you control**.
- **AI without lock-in**: AI diagnosis is a *pluggable* value-add — wire in any OpenAI-compatible model for "smart mode", or fall back to built-in heuristic diagnosis with **zero external dependency**.
- **Mobile**: Android / HarmonyOS consoles are **externally distributed packages** (mobile source is not in this repo). The Web UI ships as a PWA for phone browsers covering metrics, alerts, terminal, and SRE loops.

---

## What's New (v0.18 → Year-1 MVP)

| Release | Highlights |
|---|---|
| **v0.18.9+ Year-1** | Incident closed-loop; ops effect KPIs (MTTR/MTTA, alert noise, change failure rate, AI adoption/verify, closed-loop rate); Hermes multi-turn tools + fallback + skill distill; business-service tree; see [docs/year1-acceptance.md](docs/year1-acceptance.md) |
| **v0.18.9** | Lightweight ITSM tickets/changes, OpsLink, SQL↔ChangeRecord linking |
| **v0.18.2** | Windows lock-screen **Ctrl+Alt+Del**: Session-0 service injects SAS via named pipe; auto-enables `SoftwareSASGeneration` |
| **v0.18.0** | Controllable security loop (playbook approval / SQL change gate / security overview); resource search; host inspect; lock-screen desktop toolbar |
| **v0.17.x** | Hyper-V / containers / Kubernetes resource layer and cross-layer AI localization |

See [Releases](https://github.com/sreyun/aiops-monitor/releases) for full notes.

---

## Core Capabilities

### 1. Full-Stack Observability

- **Four-platform native collection**: Linux / Windows / macOS / Kylin; pure-Go standard-library agent with **zero third-party dependencies**; GPU (NVIDIA / AMD / Apple), CPU, memory, SWAP, disk, network, TCP, load, processes, uptime.
- **Active probes**: HTTP / TCP / Ping / UDP / process / OpenAPI / multi-point probing.
- **Hardware inspection (Redfish)**: DMTF Redfish with deep Huawei iBMC support — no agent on the BMC-managed host.
- **Traffic analysis**: NetFlow v5/v9/IPFIX TOP-N and heatmaps.
- **Storage**: Huawei OceanStor pools / LUNs / controllers / alerts.
- **Interactive charts**: Canvas hover / box-zoom / 1h–30d ranges.
- **Log aggregation**: agent tail → encrypted (AES-256-GCM) full-text search.
- **Deep host inspect (`host_inspect`)**: OS / kernel / NICs / disks / services; playbook steps can persist results; Windows Chinese locale encoding fixes.

### 2. Resource Closed Loop

- **Hyper-V**: auto-detect on Windows hosts; VM inventory & resource summary.
- **Containers**: auto-enable when Docker / Podman CLI is present.
- **Kubernetes**: nodes / pods / deployments / events with cross-layer search.
- **Global resource search**: hosts / VMs / containers / K8s objects in one place.
- **Topology**: edge CRUD + **auto-discover** (`POST /api/v1/topology/auto-discover`) for blast-radius / RCA.

### 3. Alert Governance

- **Three preset tiers**: Conservative / Standard / Relaxed across host, probe, API, playbook, and forwarding dimensions.
- **Silence → Inhibit → Route**: criticals to phone, warnings to IM only.
- **Multi-channel**: Feishu / DingTalk / SMTP / multi-cloud SMS + TTS voice; one fire + one resolve.
- **Security finding lifecycle**: host / web scan findings are trackable; scans support watchdog + cancel.

### 4. Automation & Self-Healing

- **Runbooks**: shell + built-in modules, preflight, concurrency, when/vars, retry, reverse rollback, live output + audit.
- **High-risk gate**: scheduled high-risk runs enter `pending_approval`; results support `partial` + failure reasons.
- **SRE incident loop**: alerts / SLO / manual → timeline → ack / resolve / escalate; **on-call** + escalation.
- **Change management**: freeze windows; RCA correlates recent changes; unapproved remediation blocked in freeze.
- **Remediation gate**: human approval + command allowlists / dangerous-command blocks + guardrails.
- **SLO / error budget**: multi-window multi-burn-rate.
- **Ticketing**: escalate from incidents; assign **real directory users**; image/file attachments on create & comments.

### 5. AI Diagnosis

- Scheduled / on-demand health inspection; critical incidents auto-diagnose on the timeline.
- Topology RCA + streaming follow-ups.
- **RAG learning loop** (pgvector) with 👍 / 👎 feedback rerank; verified resolutions promote to Skills.
- **WeKnora** external document RAG with automatic local fallback.
- Multimodal assistant (SSE + tools); Web speech I/O; Android Copilot attachments.
- Pluggable LLM; heuristic fallback when none configured.

### 6. Remote Desktop & Terminal

- **Remote terminal** via agent reverse tunnel (no inbound ports) + session replay.
- **Web remote desktop**:
  - JPEG / H.264 (when available); multi-monitor; quality presets (Fast / Balanced / **Clear @ 15fps**).
  - File transfer (~100MB via agent channel) and clipboard sync.
  - **Windows lock / logoff**: with **Windows service + desktop worker**, follows Winlogon; toolbar: Ctrl+Alt+Del / Wake / Unlock credentials / Esc / Win+L / Task Manager.
  - Ctrl+Alt+Del is injected by the **Session-0 service over a named pipe** (Agent ≥ v0.18.2 + `--install-service`).
- **Port forward / `/proxy`**: HTTP reverse proxy with WebSocket upgrade; jump-host to other LAN services.

### 7. SQL Toolkit

- Multi-datasource connections, schema / history, **EXPLAIN diff**.
- **High-risk SQL change ticket gate** aligned with the SRE change system.

### 8. Security & Compliance

- Strong session cookies (**PBKDF2-HMAC-SHA256, 600k**); RBAC (admin / operator / viewer); optional TOTP MFA.
- Terminal / desktop second-factor; IP + account anti-bruteforce; machine fingerprint anti-clone.
- **Security center**: overview + host / web security tabs; finding lifecycle.
- Cross-platform LLM content audit (AF_PACKET / optional TShark); SSRF egress guards; `AIOPS_SECRET_KEY` AES-GCM at rest; optional TLS.

### 9. Android Console

20+ native Compose screens: SRE cockpit, host charts, alerts, VT terminal, SRE Hub (incidents / AI / runbooks / SLO / tickets / on-call / changes), probes, AI assistant, hardware / NetFlow / Hyper-V, replay, message center, and more.

### 10. Web Console UX

Unified design tokens; global AI entry; Web Speech where supported; admin **Data & Backup** (retention, allowlists, PG backup/restore).

### 11. Deployment Resilience

Mandatory PostgreSQL + VictoriaMetrics; versioned migrations; gateway relay; multi-server fan-out with circuit breaker; install-token rotation + 7-day grace; one-click install & autostart (Windows service recommended for lock-screen desktop); amd64 + arm64 images.

---

## Architecture Overview

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
│ Alerts → governance → incidents → playbook approval → SLO → tickets │
│ AI + RAG (pgvector) · SQL toolkit · security center            │
│ Desktop relay · topology/RCA · resource search · RBAC / MFA    │
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

**Division of labor**: high-frequency collection is pure Go; protocol collectors are polled remotely; Windows lock-screen desktop requires **LocalSystem service + in-session desktop worker**.

---

## Quick Start

### Docker Compose (production / development)

| File | Use case | Notes |
|---|---|---|
| `docker-compose.yml` | **Production (default)** | Prebuilt Huawei SWR images; Agent optional via `--profile agent` |
| `docker-compose.dev.yml` | **Development overlay** | Local `docker/Dockerfile.dev`; PG/VM from Docker Hub; Agent on by default |

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

> If SWR pulls fail, point `POSTGRES_IMAGE` / `VM_IMAGE` at Docker Hub in `.env`, or use the development overlay.

### Install the Agent

```bash
# Linux (root)
curl -fsSL "http://<server>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows (admin PowerShell) — install as a service for lock-screen desktop
irm "http://<server>:8529/install.ps1?token=<TOKEN>" | iex
# After upgrades:
# aiops-agent --install-service
```

> Server **requires** both PostgreSQL and VictoriaMetrics. See [INSTALL.md](INSTALL.md) and [`config.example.yaml`](config.example.yaml).

### Windows remote desktop checklist

1. Agent must be installed as a **Windows service** (LocalSystem).  
2. Upgrade to **≥ v0.18.2** and run `aiops-agent --install-service` for SAS pipe + `SoftwareSASGeneration`.  
3. In the Web desktop: **Ctrl+Alt+Del** → **Unlock** (credentials stay in memory; audits omit plaintext).  
4. If Application Control blocks install, allowlist the binary first.

---

## Typical Scenarios

| Scenario | How AIOps Monitor helps |
|---|---|
| Unified DC monitoring | Hundreds of hosts, native metrics, three threshold presets |
| Alert storm governance | Silence + inhibit + route |
| SLA / availability | API probes + multi-window burn-rate SLO |
| Self-healing | Runbooks behind approval gates |
| Lock-screen rescue | Windows service agent + Web desktop CAD / unlock |
| Smart RCA | LLM + topology + RAG feedback learning |
| Ticket evidence loop | Escalate → assign real users → attach proof |
| Resource + change | Hyper-V / containers / K8s + SQL change gate |
| Security inspection | Security center findings lifecycle |
| On-call from outside | Native Android console |
| Cross-segment / weak net | Relay + multi-server fan-out + circuit breaker |

---

## Enterprise Services

The core is 100% MIT open source. Optional services on top of the OSS edition:

- Private deployment consulting (10k+ hosts, external VM, retention).
- Custom integrations (WeCom / DingTalk / Feishu, CMDB, internal LLM gateways).
- Security hardening (SSO / LDAP, audit retention, compliance baselines).
- Private Android distribution / signing.

> Open an Issue on GitHub / Gitee or contact the maintainer.

---

## Honest Boundaries & Known Limitations

**Backend / platform**

- Mandatory PostgreSQL + VictoriaMetrics; ~3,000 hosts per single instance before externalizing VM.
- AI is pluggable; without an LLM, heuristic fallback is shallower.
- Web speech depends on browser Web Speech API.
- Ticket attachments are JSON snapshots — fine for evidence screenshots, not object-store scale.

**Remote desktop**

- Windows lock / logoff / UAC need the agent **service install**; a user-session process cannot reliably inject Ctrl+Alt+Del.
- Secure-desktop worker uses GDI/JPEG (avoids ffmpeg black frames); FPS/bandwidth depend on the network.
- macOS may hit Secure Input / Screen Recording limits; Linux needs a graphical session / greeter.
- Unlock sends in-memory credentials only — use only in trusted ops contexts.

**Android console**

- Private APK distribution (not on app stores); build/sign yourself.
- Account self-service (MFA bind, password reset, forced first-login change) remains on the Web.
- Plain DataStore cookie persistence; fixed polling; no FCM.

---

## Open Source & Community

MIT licensed — no feature gating, no host limits, no telemetry.

- **Codebase (approx.)**: `cmd/server` 250+ Go files, `cmd/agent` 120+ files, **130+** automated tests.
- Fully self-hosted data plane (PostgreSQL + VictoriaMetrics).
- Dual remotes: GitHub and Gitee stay in sync for branches and tags.
- Contributions welcome: issues, PRs, docs, plugins.

---

## Related Links

| Resource | Link |
|---|---|
| GitHub | <https://github.com/sreyun/aiops-monitor> |
| Gitee | <https://gitee.com/bigdatasafe/aiops-monitor> |
| Releases | <https://github.com/sreyun/aiops-monitor/releases> |
| Install guide | [INSTALL.md](INSTALL.md) |
| Agent config sample | [config.example.yaml](config.example.yaml) |
| Android | [android/README.md](android/README.md) |
| HarmonyOS | [harmony/README.md](harmony/README.md) |

---

## License

[MIT](LICENSE)
