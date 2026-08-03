<div align="center">

# AIOps

**Open-Source, selbst gehostete Host-Monitoring- & SRE-Plattform**  
Beobachten · Alarmieren · Beheben · Remote-Ops · KI-Diagnose — eine Binary unter Ihrer Kontrolle.

[![Version](https://img.shields.io/badge/Version-v0.19.59-blue)](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.59)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Stars](https://img.shields.io/github/stars/sreyun/aiops-monitor?style=social)](https://github.com/sreyun/aiops-monitor)

**[简体中文](README.md) · [繁體中文](README.zh-TW.md) · [English](README_EN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Español](README.es.md) · [Português](README.pt-BR.md) · [Русский](README.ru.md)**

[Schnellstart](#-schnellstart) · [Kernfähigkeiten](#-kernfähigkeiten) · [Dokumentation](docs/README.md) · [Änderungsprotokoll](CHANGELOG.md) · [Releases](https://github.com/sreyun/aiops-monitor/releases)

</div>

---

## Warum AIOps

Ops-Stacks wachsen: Metriken hier, Alarme dort, Bastion und Runbooks woanders. Kommerzielle Suiten rechnen nach Host ab — und behalten Ihre Daten in ihrer Cloud.

AIOps bündelt den üblichen Pfad in **eine selbst gehostete Plattform**:

| | AIOps | Typischer Klebe-Stack |
|---|---|---|
| **Teile** | 1 Go-Server + 1 agent ohne Dependencies | Zabbix / Prometheus / Grafana / Alertmanager / Bastion / Runbooks… |
| **Time-to-Value** | `docker compose up -d` (~3 Min.) | Tage an Verdrahtung |
| **Daten** | PostgreSQL + VictoriaMetrics, **Ihnen** | SaaS oder verstreute DBs |
| **Remote** | Web-Terminal / Desktop / Port-Forward; Agent nur **ausgehend** | Extra-VPN / Bastion |
| **Schleife** | Alarm → Playbook → Incident/SLO/Ticket → KI-RCA | Menschen kleben Lücken |
| **Lizenz** | **MIT**, kein Host-Cap | Pro Node / Modul |

> Für private Rechenzentren, Hybrid-Cloud und Teams, die Sichtbarkeit, Kontrolle, Änderungssicherheit und erklärbare Ops brauchen.

---

## ✨ Kernfähigkeiten

Sechs Säulen — keine Feature-Wäscheliste:

```
  Observe ──────► Govern ──────► Remediate ──────► Diagnose
  Hosts/GPU/logs   Silence/route   Playbooks/gates   AI · RAG · MCP
  Probes/OOB       Multi-channel   Incident/SLO      Evidence gate

  Remote · terminal/desktop/forward (reverse tunnel)   Security · RBAC/MFA/FIM
```

1. **Beobachten** — Plattformübergreifender Agent (Linux / Windows / macOS / Kylin), GPU, Logs, HTTP/TCP-Probes, API-SLIs, Redfish / SNMP / NetFlow / Container / K8s / Hyper-V.
2. **Steuern** — Schwellwert-Presets, Silence / Inhibit / Route; Feishu / DingTalk / E-Mail / SMS / Sprache.
3. **Beheben & SRE** — Playbooks mit Freigabe-Guardrails; Incidents, SLO, Tickets, Freeze-Fenster, auditiertes Break-Glass.
4. **KI-Diagnose** — Inspektion + RCA (OpenAI-kompatibel; sonst Heuristik); pgvector-RAG, Skills, MCP (Cursor / Claude); Sprach-Selbsttest.
5. **Remote-Ops** — Web-Terminal (Replay, Beobachten, Audit, Zweitpasswort), Remote-Desktop (JPEG/H.264), Port-Forward / HTTP-Proxy mit SSRF-Schutz.
6. **Sichere Auslieferung** — RBAC, MFA, Agent-Fingerprint, AES-256-GCM; Web-Konsole; Android / HarmonyOS separat.

Aktuelles Release **[v0.19.59](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.59)** · Spiegel: [GitHub](https://github.com/sreyun/aiops-monitor) / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## 🚀 Schnellstart

> Der Server **benötigt** PostgreSQL und VictoriaMetrics.

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

Installation → **[docs/getting-started/install.en.md](docs/getting-started/install.en.md)** · Produktion → **[docs/getting-started/deploy.en.md](docs/getting-started/deploy.en.md)**

---

## 🏗 Architektur

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

## 📚 Dokumentation

Lange Texte liegen unter [`docs/`](docs/README.md). Alte Root-Dateinamen bleiben als **Weiterleitungen**.

| Need | Doc |
|------|-----|
| Install | [docs/getting-started/install.md](docs/getting-started/install.md) · [EN](docs/getting-started/install.en.md) |
| Production deploy | [docs/getting-started/deploy.md](docs/getting-started/deploy.md) · [EN](docs/getting-started/deploy.en.md) |
| End-user guide | [docs/guides/user-guide.md](docs/guides/user-guide.md) |
| Port forward | [docs/guides/forward.md](docs/guides/forward.md) |
| Content audit / playbooks | [docs/guides/content-audit.md](docs/guides/content-audit.md) |
| CI / SQL gates | [docs/engineering/ci-gate.md](docs/engineering/ci-gate.md) |

---

## 🤝 Mitwirken

Issues, PRs und Übersetzungen willkommen. Empfohlen: `make build` · `make audit`.

Wenn AIOps einen Klebe-Stack ersetzt: **bitte einen Star** — das hält das Projekt sichtbar und wartbar.

---

## Lizenz

[MIT](LICENSE). Kein Host-Cap. Mobile Clients als separate Pakete (Quellcode nicht in diesem Repo).

---

<p align="center">
  <b>AIOps · Ops-Komplexität in eine Plattform, die Sie besitzen.</b><br/>
  <sub>Star ⭐ · Fork · Issue · Self-hosted Ops gemeinsam bauen</sub>
</p>
