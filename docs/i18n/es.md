<div align="center">

# AIOps

**Plataforma open-source autoalojada de monitorización de hosts y SRE**  
Observar · Alertar · Remediar · Ops remotas · Diagnóstico IA — un binario bajo tu control.

[![Version](https://img.shields.io/badge/Version-v0.19.64-blue)](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.64)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](../../LICENSE)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20Windows%20%7C%20macOS%20%7C%20Android%20%7C%20HarmonyOS-lightgrey)]()
[![Stars](https://img.shields.io/github/stars/sreyun/aiops-monitor?style=social)](https://github.com/sreyun/aiops-monitor)

**[简体中文](../../README.md) · [繁體中文](zh-TW.md) · [English](en.md) · [日本語](ja.md) · [한국어](ko.md) · [Français](fr.md) · [Deutsch](de.md) · [Español](es.md) · [Português](pt-BR.md) · [Русский](ru.md)**

[Inicio rápido](#-inicio-rápido) · [Capacidades clave](#-capacidades-clave) · [Documentación](../README.md) · [Registro de cambios](../../CHANGELOG.md) · [Releases](https://github.com/sreyun/aiops-monitor/releases)

</div>

---

## Por qué AIOps

Las pilas de ops crecen: métricas, alertas, bastión y runbooks por separado. Los productos comerciales cobran por host y dejan tus datos en su nube.

AIOps concentra el camino habitual en **una plataforma autoalojada**:

| | AIOps | Stack típico “pegado” |
|---|---|---|
| **Piezas** | 1 servidor Go + 1 agente sin dependencias | Zabbix / Prometheus / Grafana / Alertmanager / bastión / runbooks… |
| **Puesta en marcha** | `docker compose up -d` (~3 min) | Días de integración |
| **Datos** | PostgreSQL + VictoriaMetrics, **tuyos** | SaaS o BD dispersas |
| **Remoto** | Terminal / escritorio / port-forward web; agente **solo saliente** | VPN / bastión extra |
| **Bucle** | Alerta → playbook → incidente/SLO/ticket → RCA IA | Personas unen huecos |
| **Licencia** | **MIT**, sin tope de hosts | Por nodo / módulo |

> Para DC privados, nube híbrida y equipos que necesitan visibilidad, control, seguridad del cambio y ops explicables.

---

## ✨ Capacidades clave

Seis pilares — no una lista interminable:

```
  Observe ──────► Govern ──────► Remediate ──────► Diagnose
  Hosts/GPU/logs   Silence/route   Playbooks/gates   AI · RAG · MCP
  Probes/OOB       Multi-channel   Incident/SLO      Evidence gate

  Remote · terminal/desktop/forward (reverse tunnel)   Security · RBAC/MFA/FIM
```

1. **Observar** — Agente multiplataforma (Linux / Windows / macOS / Kylin), GPU, logs, sondas HTTP/TCP, SLI de API, Redfish / SNMP / NetFlow / contenedores / K8s / Hyper-V.
2. **Gobernar** — Umbrales, silence / inhibit / route; Feishu / DingTalk / correo / SMS / voz.
3. **Remediar y SRE** — Playbooks con aprobaciones; incidentes, SLO, tickets, ventanas de congelación, break-glass auditado.
4. **Diagnóstico IA** — Inspección + RCA (modelos compatibles OpenAI; heurística si no hay modelo); RAG pgvector, Skills, MCP (Cursor / Claude); autotest de voz.
5. **Ops remotas** — Terminal web (replay, observar, auditoría, contraseña secundaria), escritorio remoto (JPEG/H.264), port-forward / proxy HTTP con protección SSRF.
6. **Entrega segura** — RBAC, MFA, huella del agente, cifrado AES-256-GCM; consola Web; Android / HarmonyOS por separado.

Versión actual **[v0.19.64](https://github.com/sreyun/aiops-monitor/releases/tag/v0.19.64)** · Espejos: [GitHub](https://github.com/sreyun/aiops-monitor) / [Gitee](https://gitee.com/bigdatasafe/aiops-monitor)

---

## 🚀 Inicio rápido

> El servidor **requiere** PostgreSQL y VictoriaMetrics.

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

Instalación → **[../getting-started/install.en.md](../getting-started/install.en.md)** · Producción → **[../getting-started/deploy.en.md](../getting-started/deploy.en.md)**

---

## 🏗 Arquitectura

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

## 📚 Documentación

La documentación larga y los README localizados están en [`docs/`](../README.md). En la raíz solo quedan el README chino y el changelog.

| Need | Doc |
|------|-----|
| Install | [../getting-started/install.md](../getting-started/install.md) · [EN](../getting-started/install.en.md) |
| Production deploy | [../getting-started/deploy.md](../getting-started/deploy.md) · [EN](../getting-started/deploy.en.md) |
| End-user guide | [../guides/user-guide.md](../guides/user-guide.md) |
| Port forward | [../guides/forward.md](../guides/forward.md) |
| Content audit / playbooks | [../guides/content-audit.md](../guides/content-audit.md) |
| CI / SQL gates | [../engineering/ci-gate.md](../engineering/ci-gate.md) |

---

## 🤝 Contribuir

Issues, PRs y traducciones bienvenidas. Sugerido: `make build` · `make audit`.

Si AIOps reemplaza un stack pegado, **deja una Star** — mantiene el proyecto visible y mantenible.

---

## Licencia

[MIT](../../LICENSE). Sin tope de hosts. Clientes móviles en paquetes separados (código fuente fuera de este repo).

---

<p align="center">
  <b>AIOps · Reduce la complejidad ops a una plataforma que posees.</b><br/>
  <sub>Star ⭐ · Fork · Issue · Construyamos ops autoalojadas juntos</sub>
</p>
