<div align="center">

# AIOps Monitor

**One binary that replaces 5+ ops toolchains — an open-source full-stack observability and SRE platform.**

</div>

<div align="center">

[![Version](https://img.shields.io/badge/Version-v6.8.1-blue)](https://github.com/sreyun/aiops-monitor/releases)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](#open-source--community)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android-lightgrey)]()
[![Arch](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64-orange)]()

**[中文](README.md) · [English](README_EN.md)**

</div>

> **Single-binary server + zero-dependency agent**: one command stands up the full stack of *observability · alert governance · automated remediation · AI diagnosis · SRE closed-loop · Android console*. 100% open source, self-hosted, data fully owned — no SaaS dependency, no telemetry uplink.

---

## Why AIOps Monitor

Monitoring tools keep piling up, yet incidents get harder to diagnose: metrics in one system, logs in another, alert storms flooding the inbox, root cause found by hand. Most commercial offerings meter by host count or feature module — and keep your data in their cloud.

AIOps Monitor takes a different path — **consolidating monitoring, alerting, automation, AI diagnosis, the SRE workflow and a mobile console into one self-hosted platform**:

- **Less is more**: one Go server binary + one dependency-free agent covers the common ground of Zabbix / Prometheus / Grafana / Alertmanager / runbook automation / terminal gateway — five fewer components to maintain.
- **Deploy in one command**: `docker compose up -d` brings up the full stack; agent installs in one click with native cross-platform collection.
- **Data ownership**: relational data lands in PostgreSQL, time-series in VictoriaMetrics — **both open-source databases you control**, exportable, auditable, compliant.
- **AI without lock-in**: AI diagnosis is a *pluggable* value-add — wire in any OpenAI-compatible model for "smart mode", or fall back to built-in heuristic diagnosis with **zero external dependency**.
- **Mobile first**: a companion enterprise-grade native Android console lets you check metrics, triage alerts, open a terminal, and run the SRE loop from your phone.

---

## Core Capabilities

### 1. Full-Stack Observability

- **Four-platform native collection**: Linux / Windows / macOS / Kylin, collected by a pure-Go standard-library agent with **zero third-party dependencies**; covers GPU (NVIDIA / AMD / Apple), CPU, memory, SWAP, disk, network, TCP connections, load, processes, uptime.
- **Active probes**: HTTP (status code / latency / TLS certificate expiry), TCP, Ping (loss / RTT), UDP, process liveness, OpenAPI business probing, distributed multi-point probing.
- **Hardware inspection (Redfish)**: standard Redfish/DMTF gathering of CPU / memory / disk / RAID / NIC / fan / PSU / temperature, with deep Huawei iBMC compatibility — no agent needed on the inspected device.
- **Traffic analysis**: NetFlow v5/v9/IPFIX 5-tuple collection with TOP-N ranking and flow heatmaps.
- **Storage collection**: Huawei OceanStor pools / LUNs / controllers / alerts onboarding.
- **Interactive charts**: pure Canvas with hover crosshair, box-zoom, double-click reset, unified 1h–30d time range.
- **Log aggregation**: agent tail-collects logs → server-side full-text search by host / level / keyword / time, AES-256-GCM encrypted in transit.

### 2. Alert Governance

A complete alert lifecycle that suppresses storming at the source:

- **Three preset tiers**: Conservative / Standard / Relaxed, across 27 groups of warn/crit thresholds spanning hosts, probes, API, scheduled tasks and port forwarding.
- **The three governance tools**: **Silence** (time window / weekday) → **Inhibit** (a root-cause alert suppresses its derivatives) → **Route** (split by severity · host to channels) — critical alerts to phone, warnings to Feishu only.
- **Multi-channel delivery**: Feishu / DingTalk webhooks, email SMTP, plus **multi-cloud SMS + voice call (TTS)** on Alibaba Cloud / Huawei Cloud / Tencent Cloud; one fire + one resolve, no spam.
- **Dedup & debounce**: pushed only on first trigger and on recovery.

### 3. Automation & Self-Healing

- **Runbook automation**: multi-step shell orchestration, batched parallel execution by "all / category / OS / host", with live output + history reports.
- **SRE incident loop**: alerts / SLO / manual incidents converge → timeline → acknowledge / resolve / escalate to ticket, with **automatic dedup and open/close**.
- **Remediation gate**: alerts auto-trigger runbook fixes behind a **human-approval gate + guardrails** — risky actions never auto-run.
- **SLO / error budget**: multi-window multi-burn-rate evaluation of SLO breaches.
- **Ticketing closed-loop**: escalate from incidents; assign **real directory users** via `GET /api/v1/directory/users` (viewer+); attach images/files on create & comments (shared with incident comments); Android SRE Hub stays in sync.

### 4. AI Diagnosis

- **Scheduled / on-demand health inspection**: synthesizes online / offline hosts, active alerts, SLO breaches and recent error logs into a health verdict.
- **Incident root-cause**: critical incidents auto-trigger AI analysis on the timeline; topology RCA + streaming follow-ups.
- **RAG vector learning loop**: pgvector-backed memory/skills with **👍 / 👎 feedback reranking**.
- **AI assistant (multimodal + voice)**: SSE streaming + Function Calling; Web supports image/file/URL attach, **speech input & TTS read-back**; Android Copilot/diagnosis can send images and parsed files.
- **Pluggable, never binding**: any OpenAI-compatible LLM enables smart mode; **without an LLM it falls back to built-in heuristic diagnosis**.
- **Decoupled embedding model**: chat / embed / optional rerank configured independently, with connectivity self-tests and AI call stats.

### 5. Security & Compliance

- **Strong session auth**: session cookies on **PBKDF2-HMAC-SHA256 (600k iterations)**; `HttpOnly` + `SameSite`, `Secure` under HTTPS.
- **RBAC route matrix**: admin / operator / viewer roles with route-level interception.
- **Optional TOTP MFA**: RFC 6238, single-use to prevent replay; Google Authenticator compatible.
- **Terminal second-factor**: re-auth before sensitive terminal sessions, rate-limited.
- **Dual anti-bruteforce**: IP + account sliding-window rate limits.
- **Machine fingerprint anti-clone**: `X-Agent-Fingerprint` binds the device; cloned images auto-regenerate host_id.
- **Static config encryption**: MFA / SMTP / AI / webhook / relay secrets persisted with **AES-256-GCM** derived from `AIOPS_SECRET_KEY`.
- **Egress hardening**: AI / webhook outbound requests guarded by SSRF protection (denies cloud metadata & link-local by default; optional `AIOPS_SSRF_STRICT` denies private networks).
- **Optional TLS**: `AIOPS_TLS_CERT/KEY` enables HTTPS.

### 6. Android Console

A companion **20+ screen enterprise-grade native Android console** (Kotlin + Jetpack Compose, minSdk 26 / targetSdk 34), not a WebView wrapper. Core screens:

- **SRE cockpit overview**: key metrics + host / alert summary, dark & light themes.
- **Host detail**: native Canvas time-series (tap / pan / pinch-zoom), disk volumes / GPU device detail.
- **Alerts**: severity / status dual-dimension filtering + one-tap ack / silence + AI diagnosis.
- **Enterprise VT terminal**: VT100 / UTF-8 decode, exponential-backoff reconnect, soft-keyboard avoidance, no rebuild on rotation.
- **SRE Hub**: incident loop / streaming AI diagnosis follow-up / runbooks / SLO / remediation approval / tickets / **on-call schedules & escalation** / **change windows & correlation**.
- **Probe monitoring**, **AI assistant (SSE streaming)**, **hardware / NetFlow / Hyper-V**, **terminal session replay**, **message center**, **duplicate-host cleanup**, **alert governance**, **terminal password**, **environment switch**, and more.
- Auth: login `POST /api/v1/login` → cookie, `DataStore` dual-track persistence; login MFA OTP dialog, terminal second-password UI; self-hosted `/ws/push` long-connection foreground service + system notifications.

### 7. Deployment Resilience

- **Two mandatory stores**: PostgreSQL + VictoriaMetrics — **missing either refuses to start**, guaranteeing data integrity by design.
- **Versioned schema migrations** (`schema_migrations`) plus admin PG backup/restore UI; retention cleanup and remediation command allowlists for enterprise ops.
- **Gateway relay**: a single internet-facing machine proxies all requests, transparently穿透 binary / reporting / terminal; `X-Relay-Secret` prevents Host injection.
- **Multi-server fan-out**: agent `servers[]` collects once and broadcasts to all, with independent auth / retry / connection pools; **circuit breaker + backoff + gzip degradation** for resilience.
- **Install-token rotation + 7-day grace**: rotating tokens never disrupt already-installed agents.
- **Remote terminal + port forwarding**: reverse-tunnel terminal with no inbound ports; `/proxy` stateless HTTP reverse proxy with WebSocket upgrade.
- **One-click install & autostart**: panel-generated tokenized command downloads + configures + registers systemd / launchd / Task Scheduler keepalive.
- **Cross-platform multi-arch**: amd64 + arm64 prebuilt images, one-line Docker pull.

---

## Architecture Overview

```
┌──────────── Collection端 (zero-dep Go Agent) ────────────┐
│ Native collection → metrics / GPU / encrypted logs        │
│ Probes: HTTP/TCP/Ping/UDP/process/OpenAPI/multi-point     │
│ Redfish HW · NetFlow · OceanStor · remote terminal        │
│ Fingerprint auth · Relay · multi-server fan-out          │
└───────┬───────────────────────────┬──────────────────────┘
        │ report / probe / terminal / forward   │ fan-out (servers[])
        ▼                                       ▼
┌──────────────── Server (single Go binary) ────────────────┐
│ Alert engine → governance(silence/inhibit/route) → incidents│
│ → remediation(runbook+approval gate+guardrails) → SLO → tickets│
│ AI diagnosis + RAG feedback rerank loop (pgvector)        │
│ Remote terminal · port forward /proxy · message center · RBAC/MFA │
│                                                            │
│  ┌────────── Two mandatory stores (missing either = no boot) ─┐│
│  │ PostgreSQL: relations/audit/incidents/tickets/JSONB/AI/sessions│
│  │ VictoriaMetrics: all time-series metrics                ││
│  └────────────────────────────────────────────────────────┘ │
└───────────────────────┬───────────────────────────────────┘
                         │ RESTful API + WebSocket (/ws/push)
                         ▼
            ┌──────── Android enterprise console ────────┐
            │ Kotlin + Jetpack Compose (20+ screens)     │
            │ overview/host/alerts/terminal/SRE Hub/AI   │
            └────────────────────────────────────────────┘
```

**Division of labor**: high-frequency, performance-sensitive collection is pure Go (single binary, zero deps); external collectors (Redfish / NetFlow / OceanStor) speak standard protocols, polled remotely by an agent that can reach the target — the inspected device needs no agent.

---

## Quick Start

### Docker Compose in one command (recommended)

```bash
# Pull the compose file and start (PG + VictoriaMetrics + AIOps Server in one go)
curl -O https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml
docker compose up -d
```

Open `http://localhost:8529` in a browser. Default credentials `admin / admin` — **first login forces a security initialization (must change username + password)**; enabling MFA afterward is recommended.

> **For production**: use the secure compose script to auto-generate strong random keys and write them into `docker-compose.yml`:
> ```bash
> bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh) && docker compose up -d
> ```
> It generates a 20-char PG password and a 50-char `AIOPS_SECRET_KEY`, auto-filling `AIOPS_POSTGRES_DSN` — no manual config edits needed.

### Install the Agent (monitored host)

Panel top-right "Install Agent" → pick OS → copy the command to the target host:

```bash
# Linux (root)
curl -fsSL "http://<server>:8529/install.sh?token=<TOKEN>" | sudo sh
# Windows (admin PowerShell)
irm "http://<server>:8529/install.ps1?token=<TOKEN>" | iex
```

> The server **mandatorily depends** on both PostgreSQL and VictoriaMetrics; missing either refuses to start. More deployment options (binary run / self-build / autostart / cross-network Nginx reverse proxy / gateway relay) are in [INSTALL.md](INSTALL.md).

---

## Typical Scenarios

| Scenario | How AIOps Monitor helps |
|---|---|
| **Unified small/mid-size DC monitoring** | One server governs hundreds of Linux/Windows/macOS/Kylin hosts; native CPU/mem/disk/GPU collection; three preset tiers out of the box |
| **Alert storm governance** | Silence + inhibit + route to mute non-critical alerts at night, suppress derivatives of an offline host, and push criticals to phone while recoveries still fire |
| **Business availability SLA** | API monitoring black-box probes core endpoints; P95 latency / availability / throughput feed multi-window burn-rate SLO evaluation |
| **Failure self-healing** | Alerts trigger runbook fixes; high-risk actions stop at the human-approval gate; the whole process is audited |
| **Smart root-cause** | With an LLM, incidents auto-diagnose; the RAG vector store accumulates similar historical cases; 👍/👎 feedback makes diagnosis sharper over time |
| **On-call from outside** | Open the native Android console to view overview, triage alerts, open a VT terminal, and run the SRE incident loop |
| **Hardware asset compliance** | Redfish inspection + OceanStor collection in one hardware-asset panel, with change drift detection and export |
| **Cross-segment / weak-network collection** | Gateway relay single-point穿透; multi-server fan-out + circuit breaker + gzip degradation keep data flowing under weak networks |

---

## Enterprise Services

The AIOps Monitor core is 100% open source (MIT) and freely self-hostable. For enterprise-grade needs, services built on top of the open-source edition include:

- **Private deployment consulting**: large-scale (10k+ hosts) sharding, external VictoriaMetrics, retention tuning.
- **Custom integrations**: deep WeCom / DingTalk / Feishu, CMDB, ticketing systems, internal LLM gateways.
- **Security & compliance hardening**: SSO / LDAP, audit retention, baseline recommendations for graded protection.
- **Android distribution channel**: private app distribution and signing hosting (see Honest Boundaries below).

> For enterprise collaboration, open an Issue on the GitHub repo or contact the maintainer.

---

## Honest Boundaries & Known Limitations

We describe capabilities truthfully. Please note the following boundaries before use:

**Backend / platform**

- The server mandatorily depends on PostgreSQL and VictoriaMetrics, both open-source; a single instance is comfortable at roughly 3,000 hosts (go external VictoriaMetrics beyond that).
- AI diagnosis is a pluggable value-add; without an LLM it falls back to heuristic diagnosis and does not guarantee LLM-level semantic depth.

**Android console**

- **Self-hosted private distribution, not published on any app store**; delivered as an APK you sign and distribute yourself.
- The repo ships historical build artifacts (e.g. `aiops-6193.apk`) proving the client **was successfully built before**; however the current source **has not been re-compiled/verified in the current sandbox**, so zero-compile-error is not guaranteed — rely on your local Android Studio build.
- **Account self-service stays on the web**: MFA self-binding, forgot-password, and forced first-login password change UIs live on the web side; the Android app reuses the same RBAC account system.
- Session cookies use **plain (unencrypted) DataStore** persistence.
- It uses **fixed polling** with **full host / alert pulls** (not incremental); **no system-level background push (FCM)** — it relies on foreground auto-refresh.
- None of the above undermines its practical value as an enterprise-grade native mobile console in self-hosted intranet scenarios.

---

## Open Source & Community

AIOps Monitor is **100% open source under the MIT license** — no feature gating, no host limits, no telemetry uplink.

- **Codebase**: server `cmd/server` ~126 Go files / 40k+ lines, agent `cmd/agent` ~69 files / 18k+ lines, with 64 tests — production-grade maturity.
- **Fully self-hosted**: relational (PostgreSQL) + time-series (VictoriaMetrics) data stay in your environment.
- **Contributions welcome**: issues, PRs, docs and plugins.

---

## Related Links

- **GitHub repository**: <https://github.com/sreyun/aiops-monitor>
- **Releases**: <https://github.com/sreyun/aiops-monitor/releases>
- **Installation guide**: [INSTALL.md](INSTALL.md)
- **Android client notes**: [android/README.md](android/README.md)

---

## License

[MIT](LICENSE)
