<div align="center">

# AIOps

**Open-source, self-hosted host monitoring & SRE platform**  
Observe · Alert · Remediate · Remote ops · AI diagnosis — one binary you fully control.

[![Version](https://img.shields.io/badge/Version-v0.19.59-blue)](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.59)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Stars](https://img.shields.io/github/stars/sreyun/aiops-monitor?style=social)](https://github.com/sreyun/aiops-monitor)

**[简体中文](README.md) · [繁體中文](README.zh-TW.md) · [English](README_EN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Español](README.es.md) · [Português](README.pt-BR.md) · [Русский](README.ru.md)**

[Quick start](#-quick-start) · [Core capabilities](#-core-capabilities) · [Docs](docs/README.md) · [Changelog](CHANGELOG.md) · [Releases](https://github.com/sreyun/aiops-monitor/releases)

</div>

---

## Why AIOps

Ops stacks keep growing: metrics here, alerts there, a bastion for shells, another tool for runbooks. Commercial suites meter by host or module — and keep your data in their cloud.

AIOps collapses the common path into **one self-hosted platform**:

| | AIOps | Typical glue stack |
|---|---|---|
| **Parts** | 1 Go server + 1 zero-dep agent | Zabbix / Prometheus / Grafana / Alertmanager / bastion / runbooks… |
| **Time-to-value** | `docker compose up -d` (~3 min) | Days of wiring |
| **Data** | PostgreSQL + VictoriaMetrics, **yours** | SaaS or scattered DBs |
| **Remote** | Web terminal / desktop / port-forward; agent **outbound-only** | Extra VPN / bastion |
| **Loop** | Alert → playbook → incident/SLO/ticket → AI RCA | Humans glue the gaps |
| **License** | **MIT**, no host caps | Per-node / per-module fees |

> Built for private DC, hybrid cloud, and teams that need visibility, control, change safety, and explainable ops.

---

## ✨ Core capabilities

Six pillars — not a laundry list:

```
  Observe ──────► Govern ──────► Remediate ──────► Diagnose
  Hosts/GPU/logs   Silence/route   Playbooks/gates   AI · RAG · MCP
  Probes/OOB       Multi-channel   Incident/SLO      Evidence gate

  Remote · terminal/desktop/forward (reverse tunnel)   Security · RBAC/MFA/FIM
```

1. **Observe** — Cross-platform agent (Linux / Windows / macOS / Kylin), GPU, logs, HTTP/TCP probes, API SLIs, Redfish / SNMP / NetFlow / containers / K8s / Hyper-V.  
2. **Govern** — Threshold presets, silence / inhibit / route; Feishu / DingTalk / email / SMS / voice.  
3. **Remediate & SRE** — Playbooks with approval guardrails; incidents, SLO, tickets, freeze windows, audited break-glass.  
4. **AI diagnosis** — Inspection + RCA (OpenAI-compatible models; heuristics if unset); pgvector RAG, Skills, MCP for Cursor / Claude; speech self-test.  
5. **Remote ops** — Web terminal (replay, observe, audit, secondary password), remote desktop (JPEG/H.264), port-forward / HTTP proxy with SSRF guards.  
6. **Secure delivery** — RBAC, MFA, agent fingerprint, AES-256-GCM config crypto; Web console; Android / HarmonyOS apps distributed separately.

Current release **[v0.19.59](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.59)** · Mirrors: [GitHub](https://github.com/sreyun/aiops-monitor) / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## 🚀 Quick start

> Server **requires** both PostgreSQL and VictoriaMetrics.

```bash
docker compose up -d
# open http://localhost:8529 → finish first-time security setup
# copy the Agent install command from the UI onto each host
```

Binary / from source:

```bash
export AIOPS_POSTGRES_DSN="postgres://aiops:secret@127.0.0.1:5432/aiops?sslmode=disable"
export AIOPS_VM_URL="http://127.0.0.1:8428"
./aiops-server

go build ./cmd/server ./cmd/agent   # Go 1.26+
```

Full install → **[docs/getting-started/install.en.md](docs/getting-started/install.en.md)** · Production → **[docs/getting-started/deploy.en.md](docs/getting-started/deploy.en.md)**

---

## 🏗 Architecture

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

## 📚 Documentation

Long-form docs live under [`docs/`](docs/README.md). Old root filenames remain as **redirect stubs**.

| Need | Doc |
|------|-----|
| Install | [docs/getting-started/install.md](docs/getting-started/install.md) · [EN](docs/getting-started/install.en.md) |
| Production deploy | [docs/getting-started/deploy.md](docs/getting-started/deploy.md) · [EN](docs/getting-started/deploy.en.md) |
| End-user guide | [docs/guides/user-guide.md](docs/guides/user-guide.md) |
| Port forward | [docs/guides/forward.md](docs/guides/forward.md) |
| Content audit / playbooks | [docs/guides/content-audit.md](docs/guides/content-audit.md) |
| CI / SQL gates | [docs/engineering/ci-gate.md](docs/engineering/ci-gate.md) |

---

## 🤝 Contributing

Issues, PRs, and translations welcome. Suggested: `make build` · `make audit`.

If AIOps replaces a glue stack for you, **please Star the repo** — it keeps the project visible and maintainable.

---

## License

[MIT](LICENSE). No host caps, no “enterprise-only” traps. Mobile clients are separate packages (source not in this repo).

---

<p align="center">
  <b>AIOps · Collapse ops complexity into a platform you own.</b><br/>
  <sub>Star ⭐ · Fork · Open an issue · Build self-hosted ops together</sub>
</p>
