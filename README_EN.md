# AIOps Monitor

[中文](README.md) | [English](README_EN.md)

> **Lightweight Host Monitoring & Ops Platform** — Go-native collection core + Python plugin layer + real-time dashboard + threshold alerts + Feishu/DingTalk/Email push
>
> Single-binary server, zero-dependency agent, tri-platform native collection (incl. **GPU**), one-command install, ready out of the box.
>
> Built-in: **interactive trend charts** (hover crosshair / drag-zoom / full-screen preview), **custom probes** (HTTP / TCP / **Ping** / process, with history curves), **remote terminal** (browser full TTY via Agent reverse connection, no port opening), **embedded lightweight DB persistence** (history/logs/sessions survive restarts), **gzip response compression**, **PWA installable**, auth & security hardening (**MFA two-factor** + **account recovery**).

---

## Overview

AIOps Monitor is a **host monitoring and operations platform** for small-to-medium scale deployments, using a **Go + Python hybrid architecture**:

- **Go Agent core** handles high-frequency, system-level base metric collection — Linux reads `/proc` + `syscall`, Windows calls Win32 API, macOS uses `sysctl` + system commands — **all three platforms natively zero-dependency**, plus host registration, dual-heartbeat reporting, and plugin scheduling.
- **Python plugin layer** handles custom collection, business/middleware probing, anomaly detection, and other AI/automation logic. Plugins run as **subprocesses + JSON** contracts, naturally decoupled: plugin crashes/timeouts are logged and skipped, never affecting the core.
- **Go server** runs as a single binary with built-in real-time web dashboard, threshold alert engine, Feishu/DingTalk/Email push, one-click install script distribution, and host category management.

Agent and server share type definitions in `shared/`, so the collection-side and server-side data contract never drifts.

---

## Core Features

| Capability | Description |
|---|---|
| **Tri-platform native collection** | Linux (`/proc` + `syscall`), Windows (Win32 API), macOS (`sysctl` + system commands), all zero third-party deps |
| **Comprehensive metrics** | CPU / Memory / SWAP / Multi-disk / Network RX/TX rate / TCP connections / Load 1·5·15 / Process count / Uptime |
| **GPU monitoring** | Utilization / VRAM / Temperature; NVIDIA (`nvidia-smi`, Linux/Windows), AMD (Linux sysfs), Apple/others (macOS `ioreg`), best-effort with caching |
| **Interactive trend charts** | Pure Canvas rendering, hover crosshair + value tooltip, drag-to-zoom, double-click reset, click-to-enlarge; CPU/Memory/Disk/GPU/Network multi-chart |
| **Custom probes** | HTTP website (status code / latency / **TLS cert days remaining**), TCP port, **Ping host alive (loss% / RTT)**, process alive; **list/pill dual view**, each with **history curve playback** |
| **Python plugin layer** | Subprocess + JSON contract, concurrent execution, timeout isolation, crash-skip; custom collection / service probing / AI anomaly detection |
| **Real-time web dashboard** | Overview cards + resource TOP10 (CPU/Mem/Disk/GPU) + host list (category grouping/search/filter/pagination) + threshold alerts + activity log (paginated 10/30/50/100) + standard/wide layout toggle |
| **Threshold alerts** | CPU / Memory / Disk threshold + host offline + **GPU overload** + **high system load** detection, custom thresholds, visual config in dashboard |
| **Alert push** | Feishu / DingTalk bot Webhook + **Email SMTP push**, only on trigger/recover transitions (no spam); push content includes **hostname / IP / detail / timestamp** |
| **Persistence** | Embedded lightweight DB (gzip+JSON to `aiops.db`) — history / logs / sessions survive restarts, no external database needed |
| **Remote terminal** | One-click browser terminal from host card, via Agent **reverse connection** (no inbound port on target); full interactive TTY (Windows ConPTY, Linux/macOS openpty), supports color / vim·top / window maximize·restore·close; login + Token dual auth + audit |
| **Multi-select category filter** | Top-right category dropdown supports **multi-select**; overview KPI cards, resource TOP10, alerts **auto-link** to filter |
| **Category collapse** | Host list grouped by category, each group supports **click to collapse/expand** |
| **PWA installable** | Dashboard supports **PWA** — install to desktop/home screen, standalone window, Service Worker offline cache; long-press icon for quick access to hosts/alerts/monitoring |
| **Keyboard shortcuts** | Number keys **1–5** to switch views (overview/hosts/monitoring/alerts/logs) |
| **One-click install** | Dashboard generates Token-embedded install command, auto-downloads Agent + plugins, registers boot autostart |
| **Security & performance** | **Mandatory Agent Token** (default, constant-time compare) + login rate-limiting + session Cookie (HttpOnly/SameSite/Secure on HTTPS) + security headers + request body limit + secret masking + host identity anti-clone + **MFA two-factor (TOTP)** + **account recovery (email code)** + **MFA unbind via email**; **gzip response compression** reduces multi-host polling bandwidth |
| **Shared types** | `shared/wire.go` imported by both server and agent, contract unified |

---

## Architecture

```
                    ┌─────────────── Go Agent Core (high-perf / high-freq) ──────────────┐
                    │  Collector (tri-platform native) → base metrics                    │
   Report           │  PluginRunner → concurrent Python plugin scheduling, JSON merge    │
  (base+custom      │  Reporter (dual-heartbeat) → high-freq base + low-freq plugins     │
   +events) ─HTTP─► │  Shares types with server via shared/                              │
                    └───────────────────────────────┬────────────────────────────────────┘
                                                     │ subprocess + JSON (low-freq)
                          ┌──────────────────────────┼──────────────────────────┐
                    ┌─────┴───────┐          ┌────────┴────────┐         ┌────────┴────────┐
                    │ Custom       │          │  AI / Anomaly    │         │ Process Monitor │
                    │ collection   │          │  detection       │         │                 │
                    │ (.py)        │          │  (.py)           │         │  (.py)          │
                    └──────────────┘          └──────────────────┘         └─────────────────┘
```

**Design principle**: High-frequency, general-purpose, performance-sensitive base collection uses Go (single binary, zero deps, dense polling); variable, ecosystem-dependent custom/AI logic uses Python. Process boundary isolates each, letting them evolve independently.

---

## Directory Structure

```
aiops-monitor/
├── go.mod                          # Go module: aiops-monitor
├── shared/
│   └── wire.go                     # ★ Shared types (Metrics/Sample/Event/Report)
├── cmd/
│   ├── server/                     # Go server (pure stdlib, single binary, embedded dashboard)
│   │   ├── main.go                 # Entry, routing, CORS, gzip / body limit middleware
│   │   ├── handlers.go             # API handlers
│   │   ├── store.go                # In-memory store + multi-level downsampled history
│   │   ├── db.go                   # Embedded lightweight DB (gzip+JSON, auto-save/exit-flush)
│   │   ├── alerts.go               # Threshold alert engine
│   │   ├── auth.go                 # Login auth + session + rate-limiting + MFA(TOTP)
│   │   ├── check.go                # Custom monitoring (HTTP / TCP / Ping / process + history)
│   │   ├── ws.go                   # Hand-written WebSocket (terminal browser-side, zero-dep)
│   │   ├── terminal.go             # Remote terminal relay (Agent reverse channel + sessions)
│   │   ├── notify.go               # Feishu/DingTalk/Email push (dedup + state transition)
│   │   ├── email.go                # SMTP email sending + verification code/reset token manager
│   │   ├── totp.go                 # TOTP (RFC 6238) two-factor auth
│   │   ├── config.go               # Config persistence
│   │   ├── install.go              # One-click install script generation
│   │   └── web/                    # Dashboard frontend (embedded at compile time)
│   │       ├── index.html
│   │       ├── app.js
│   │       ├── style.css
│   │       ├── manifest.json        # PWA manifest
│   │       ├── sw.js                # Service Worker (offline cache)
│   │       └── icon.svg             # App icon
│   └── agent/                      # ★ Go Agent core
│       ├── main.go                 # Config / flags / signals
│       ├── collector.go            # Collector interface
│       ├── collector_linux.go      # Linux native collection (/proc + syscall)
│       ├── collector_windows.go    # Windows native collection (Win32 API)
│       ├── collector_darwin.go     # macOS native collection (sysctl + system commands)
│       ├── collector_other.go      # Other platform stub
│       ├── gpu.go                  # GPU collection (nvidia-smi parsing + cache, tri-platform)
│       ├── terminal.go             # Remote terminal Agent-side (reverse channel + framed rx + shell)
│       ├── pty_windows.go          # Windows pseudo-terminal (ConPTY)
│       ├── pty_unix.go             # Linux/macOS pseudo-terminal (openpty, common parts)
│       ├── pty_linux.go / pty_darwin.go  # Per-OS ioctl pts open
│       ├── plugins.go              # Plugin runner (subprocess + JSON, concurrent + timeout)
│       ├── identity.go             # Stable host_id / host identity
│       └── reporter.go             # Dual-heartbeat loop + register + report
├── plugins/                        # ★ Python plugin layer
│   ├── plugin_sdk.py               # Minimal plugin SDK
│   ├── core_metrics.py             # Base metric fallback (psutil)
│   ├── example_service_check.py    # Example: service probe
│   ├── example_ai_anomaly.py       # Example: CPU anomaly detection (z-score)
│   ├── process_monitor.py          # Process alive monitoring
│   ├── process_monitor.json        # Process monitor config
│   └── requirements.txt            # psutil (optional)
├── dist/                           # Agent distribution dir (platform binaries + plugins.zip)
├── bin/                            # Pre-compiled binaries
├── config.example.json             # Agent config example
├── server_config.example.json      # Server config example
├── INSTALL.md                      # Detailed installation guide
├── Dockerfile                      # Multi-stage build (server + agent)
└── docker-compose.yml              # Docker Compose one-click deploy
```

---

## Docker Deployment (Recommended)

```bash
# Clone the repo
git clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor

# Start the server
docker compose up -d aiops-server

# Open http://localhost:8080 in your browser
```

> **Default login credentials**: Username `admin` / Password `admin`. After first login, immediately change your username and password in「Profile」, and consider enabling two-factor auth (MFA).

Server data persists via volume (`/app/data`), config file at `./server_config.json`. Agent container is not started by default — uncomment the `aiops-agent` section in `docker-compose.yml` to enable the local Agent.

---

## Quick Start

### 1. Start the Server

```bash
# Use pre-compiled binary
./bin/aiops-server                     # Default listen :8080

# Or build from source (requires Go 1.22+)
go build -o bin/aiops-server ./cmd/server
./bin/aiops-server

# Specify address/port
./bin/aiops-server -addr 0.0.0.0:9000
```

Open `http://localhost:8080` in your browser to see the monitoring dashboard.

### 2. Start the Agent

**Run from the repo root** (so the `plugins/` directory is found):

```bash
# Plugins use psutil (optional, base metrics don't need it)
pip install -r plugins/requirements.txt

./bin/aiops-agent --server http://<server-IP>:8080 --category Production
```

Refresh the dashboard after a few seconds to see the host card and metrics.

### 3. One-Click Install (Recommended for Production)

Click **「Install Agent」** in the top-right of the dashboard → select target OS → copy the command to the monitored host. The command includes the server URL and Token, auto-downloads Agent + plugins, writes config, and registers boot autostart:

```bash
# Linux (root/sudo)
curl -fsSL "http://<server>:8080/install.sh?token=<TOKEN>" | sudo sh

# Windows (admin PowerShell)
irm "http://<server>:8080/install.ps1?token=<TOKEN>" | iex

# macOS
curl -fsSL "http://<server>:8080/install.sh?token=<TOKEN>" | sh
```

### Common Parameters

| Parameter | Description | Default |
|---|---|---|
| `--server` | Server address | `http://localhost:8080` |
| `--category` | Host category (dashboard groups by this) | empty |
| `--interval` | Base metric report interval (seconds) | `10` |
| `--plugin-interval` | Plugin execution cycle (seconds) | `15` |
| `--plugins-dir` | Plugin directory (absolute path OK) | `plugins` |
| `--python` | Python interpreter for `.py` plugins | `python3` (`python` on Windows) |
| `--disk-path` | Primary disk path (overview; all local disks auto-detected) | `/` (system drive on Windows) |
| `--token` | Install Token (optional) | empty |
| `--config` | Config file path | `config.json` |

You can also use a config file: `cp config.example.json config.json`, edit, and run.

### Build from Source

```bash
go build -o bin/aiops-server ./cmd/server
go build -o bin/aiops-agent  ./cmd/agent

# Cross-compile Agent
GOOS=windows GOARCH=amd64 go build -o bin/aiops-agent.exe ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o bin/aiops-agent-mac ./cmd/agent
```

---

## Monitoring Metrics

| Metric | Linux | Windows | macOS |
|---|---|---|---|
| CPU usage / cores | `/proc/stat` | `GetSystemTimes` | `top -l 2` |
| Memory / SWAP | `/proc/meminfo` | `GlobalMemoryStatusEx` | `sysctl` + `vm_stat` |
| Disk (all local) | `/proc/mounts` + `statfs` | `GetDiskFreeSpaceExW` | `syscall.Statfs` + `df` |
| Network RX/TX rate | `/proc/net/dev` | `GetIfTable` | `netstat -ibn` |
| TCP connections | `/proc/net/tcp` | `GetTcpTable` | `netstat -an` |
| Load 1/5/15 | `/proc/loadavg` | EWMA approximation | `sysctl vm.loadavg` |
| Process count | `/proc` enumerate | `EnumProcesses` | `ps -A` |
| Uptime | `/proc/uptime` | `GetTickCount64` | `sysctl kern.boottime` |
| **GPU util/VRAM/temp** | `nvidia-smi` / amdgpu sysfs | `nvidia-smi` | `ioreg` (IOAccelerator) |

**All three platforms are zero third-party dependency** — the Go core collects directly via syscall / system commands, no Python or agent framework needed.

> GPU is best-effort: reports when vendor tools (NVIDIA `nvidia-smi`) or OS interfaces are available, cached ~12s to avoid spawning a process every report cycle; no GPU/no tool = no GPU section on that host, doesn't affect other metrics.

---

## Custom Monitoring (Probes) Usage

In addition to Agent auto-reported base metrics, the dashboard「Monitoring」page lets you add **active probes**: periodic checks on websites, ports, host connectivity, and process alive, auto-generating alerts on failure. Four types:

| Type | What to fill | Description | Failure condition |
|---|---|---|---|
| **HTTP website** | URL (e.g. `https://example.com`) | Server sends HTTP(S) request, shows status code / latency / HTTPS cert days remaining | Status ≥ 400, or timeout/failure |
| **TCP port** | host:port (e.g. `10.0.0.5:3306`) | Server attempts TCP connection, shows connectivity + latency | Cannot connect |
| **Ping host** | host/IP (e.g. `8.8.8.8`) | Server ICMP ping, shows loss% and avg RTT | 100% loss (unreachable) |
| **Process alive** | **① Target host + ② Process name** | See below | Process not reported by target host (or host offline) |

**Steps**: Dashboard →「Monitoring」→「+ Add Check」→ select type → fill target → set interval & alert level → save. Each supports「Run now ▶ / History curve (1h/6h/24h/all filter) / Edit / Delete」, and toggle between **list / pill** views.

### Why does process monitoring need more than just a process name?

Process monitoring requires **① select target host + ② fill process name** (e.g. `nginx`, `mysql`, `aiops-agent`), because:

- **HTTP / TCP / Ping** are all **server-side active probes** to a target address — independent of the monitored host, so only the address is needed.
- **Process alive** checks **whether the target host's Agent reported this process** in its process list — the server doesn't run on the monitored machine, so it must know "which host to check."

So a process check's full semantics is "**is process X running on host A**". Matching: **case-insensitive substring match** (typing `nginx` matches `nginx.exe` / `nginx: master`).

> Prerequisite: target host must have Agent installed and online (Agent periodically reports process names); host offline or no process data = check shows as abnormal.

---

## Writing a Plugin

A plugin = an executable script that **prints a JSON object to stdout**. With the SDK, just a few lines:

```python
# plugins/my_check.py
from plugin_sdk import Plugin

p = Plugin()
p.metric("mysql.connections", 42)          # Custom metric (gauge)
p.metric("mysql.qps", 1350.5)
p.event("warning", "Replication lag 8s")   # Event (info | warning | critical)
p.emit()                                   # Output JSON
```

Drop it in the `plugins/` directory and it's auto-discovered, executed every `--plugin-interval`. JSON contract:

```json
{
  "metrics": { "custom_metric_name": value, ... },
  "events":  [ {"level": "warning", "message": "..."} ],
  "base":    { "cpu_percent": ..., ... }
}
```

- `metrics` keys should use namespaces (`mysql.`, `nginx.`) to avoid conflicts
- `events` `source` auto-fills to plugin name if omitted
- Plugin crashes/timeouts/bad JSON are logged and skipped — no impact on core
- Non-`.py` executables also work as plugins — any language

**AI / automation logic goes in this layer**: `example_ai_anomaly.py` uses z-score for CPU anomaly detection; real scenarios can swap in Prophet / sklearn, or integrate RAGFlow + Dify + local vLLM intelligent analysis platforms.

---

## Alert Configuration

Alerts are configured visually in the **dashboard** — no file editing:

1. Click **Alert Settings** in the top-right
2. Fill in Feishu or DingTalk bot Webhook URL (DingTalk: also fill Secret if using "signing"), check enable
3. **Email push**: expand the「Email Service (SMTP)」section, fill SMTP server address, port, sender email account, auth code/password, sender display name, check「Enable TLS/SSL」(port 465 = implicit TLS, 587 = not), check「Enable email push」
4. Click **Send Test** to verify channel connectivity (tests Feishu/DingTalk/Email simultaneously)
5. Click **Save** — outstanding alerts are immediately re-pushed after save

> SMTP auth code/password uses the same masking strategy as Webhook Secrets: stored in plaintext, masked on display, blank submission preserves the original value. Email alerts are sent to the email address bound in「Profile」.

Default thresholds: CPU/Memory 80% warning, 90% critical; Disk 85%/95%; offline 30s; GPU 80%/90%; system load (5-min avg ≥ cores×2) warning. All thresholds are adjustable in the dashboard.

Alert type coverage:
| Alert type | Trigger condition | Level |
|---|---|---|
| CPU usage | Exceeds threshold | Warning / Critical |
| Memory usage | Exceeds threshold | Warning / Critical |
| Disk usage | Exceeds threshold (multi-partition) | Warning / Critical |
| Host offline | No report within offline threshold | Critical |
| GPU usage | ≥ 80% warning, ≥ 90% critical | Warning / Critical |
| System load | 5-min load ≥ cores×2 | Warning / Critical |
| HTTP probe | Status ≥ 400, timeout, request failure | Custom |
| TCP probe | Cannot connect | Custom |
| Ping probe | 100% loss (unreachable) | Custom |
| Process alive | Target process not running on host | Custom |

> - Feishu custom bot keyword should be set to `AIOps` or `告警`
> - DingTalk: use "signing" security, put the Secret in the dashboard for auto-signing

---

## API Reference

| Method | Path | Description |
|---|---|---|
| **Agent Communication** | | |
| POST | `/api/v1/agent/register` | Agent registration |
| POST | `/api/v1/agent/report` | Report (base + custom + events) |
| **Host Management** | | |
| GET | `/api/v1/hosts` | Host list (with latest metrics, online status) |
| GET | `/api/v1/hosts/meta` | Host metadata (id + hostname, for process monitor selection) |
| GET | `/api/v1/hosts/{id}/metrics` | Single host base metric history (recent raw) |
| GET | `/api/v1/hosts/{id}/history?from=&to=` | Single host time-series history (auto-selects raw/1-min/5-min aggregation by span) |
| POST | `/api/v1/hosts/{id}/category` | Set host category override |
| DELETE | `/api/v1/hosts/{id}` | Delete host |
| **Alerts & Events** | | |
| GET | `/api/v1/alerts` | Threshold alerts + custom monitor alerts |
| GET | `/api/v1/events` | Plugin events |
| GET | `/api/v1/activity` | Activity log (operations + system + plugin) |
| GET | `/api/v1/summary` | Summary statistics |
| **Custom Monitoring** | | |
| GET | `/api/v1/checks` | Custom monitor list (with status/latency/cert days/loss) |
| POST | `/api/v1/checks` | Add/update custom monitor (type: http / tcp / ping / process) |
| POST | `/api/v1/checks/{id}/run` | Trigger one immediate probe |
| GET | `/api/v1/checks/{id}/history` | Check history time-series (latency/status/loss, for curve playback) |
| DELETE | `/api/v1/checks/{id}` | Delete custom monitor |
| **Remote Terminal** | | |
| GET | `/api/v1/hosts/{id}/terminal` | Browser WebSocket terminal (requires login session) |
| GET | `/api/v1/agent/terminal/wait` | Agent long-poll for session (Token auth) |
| GET | `/api/v1/agent/terminal/rx` | Server → Agent keystroke/resize frames (Token) |
| POST | `/api/v1/agent/terminal/tx` | Agent → Server shell output stream (Token) |
| **Config Management** | | |
| GET | `/api/v1/config` | Get alert config (masked) |
| POST | `/api/v1/config` | Update alert config |
| POST | `/api/v1/config/test` | Send alert test message |
| **Auth & Account** | | |
| POST | `/api/v1/login` | Login (get session cookie) |
| POST | `/api/v1/logout` | Logout |
| GET | `/api/v1/me` | Current user info |
| POST | `/api/v1/profile` | Update profile (incl. email binding) |
| POST | `/api/v1/password` | Change password |
| POST | `/api/v1/mfa/setup` | Generate MFA secret + QR URI |
| POST | `/api/v1/mfa/enable` | Enable two-factor (verified by TOTP code) |
| POST | `/api/v1/mfa/disable` | Disable two-factor (requires password) |
| POST | `/api/v1/mfa/unbind-via-email` | Unbind MFA via email verification code (recovery for lost phone) |
| **Account Recovery** | | |
| POST | `/api/v1/account/recover-username` | Recover username via bound email (public endpoint) |
| POST | `/api/v1/account/send-reset-code` | Send password reset code to bound email (public endpoint) |
| POST | `/api/v1/account/reset-password` | Reset password after email code verification (public endpoint) |
| **Install Distribution** | | |
| GET | `/api/v1/install/info` | Install info (URL + Token) |
| POST | `/api/v1/install/reset-token` | Reset install Token |
| GET | `/install.sh` / `/install.ps1` | One-click install scripts |
| GET | `/uninstall.sh` / `/uninstall.ps1` | One-click uninstall scripts |
| **Dashboard & Resources** | | |
| GET | `/` | Web dashboard |
| GET | `/healthz` | Health check (built-in self-monitor uses this) |
| GET | `/dl/*` | Agent binary downloads |

---

## Server Configuration Parameters

Server config file `server_config.json` (auto-generated in the server's directory) supports:

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
| `thresholds.offline_after_sec` | int | `30` | Host offline threshold (seconds) |
| `require_token` | bool | `false` | Require Agent Token |
| `allow_anonymous_agents` | bool | `false` | Allow Token-less Agents |
| `terminal_disabled` | bool | `false` | Globally disable remote terminal |
| `install_token` | string | auto-gen | Agent install Token |
| `trust_proxy` | bool | `false` | Behind trusted reverse proxy (Nginx): set `true` to honor `X-Real-IP`/`X-Forwarded-For` for client IP and login rate-limiting; keep `false` when directly exposed (headers are forgeable) |
| `smtp.smtp_enabled` | bool | `false` | Email push toggle |
| `smtp.smtp_host` | string | `""` | SMTP server address (e.g. `smtp.gmail.com`) |
| `smtp.smtp_port` | int | `0` | SMTP port (465 implicit TLS / 587 STARTTLS) |
| `smtp.smtp_username` | string | `""` | Sender email account |
| `smtp.smtp_password` | string | `""` | SMTP auth code/password (masked on display, blank preserves original) |
| `smtp.smtp_from_name` | string | `"AIOps Monitor"` | Sender display name |
| `smtp.smtp_use_tls` | bool | `false` | Enable implicit TLS (port 465 = `true`, 587 = `false`) |

---

## FAQ

### Agent report failure
- Check `--server` address is correct and server is running
- Check firewall/security group allows the server port
- Check Agent logs for errors (`report failed: ...`)

### Remote terminal won't connect
- **Behind Nginx**: must configure WebSocket upgrade headers and disable buffering (see "Reverse Proxy" section above)
- **Cross-network**: ensure Agent uses a public-reachable server address
- Confirm server doesn't have `terminal_disabled: true`

### Dashboard shows connection failed
- Check server is running: `curl http://localhost:8080/healthz`
- Check browser console for CORS or auth errors
- Try clearing browser cache or hard refresh (Ctrl+Shift+R)

### Host shows offline
- Default 30s no report = offline; adjust `offline_after_sec` in alert settings
- Check Agent process: `ps aux | grep aiops-agent`
- Check Agent-to-server network connectivity

### GPU info not showing
- NVIDIA GPU requires `nvidia-smi` installed
- AMD GPU (Linux) requires sysfs permissions
- macOS only supports Apple Silicon GPU monitoring
- GPU is best-effort — no tool = no display, doesn't affect other metrics

---

## Deployment & Operations

### Boot Autostart

**Linux (systemd)**:
```bash
cp deploy/aiops-agent.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now aiops-agent
```

**Windows (NSSM)**:
```powershell
nssm install AIOps-Agent C:\aiops-agent\aiops-agent.exe "--server http://<IP>:8080 --category Production"
nssm set AIOps-Agent AppDirectory C:\aiops-agent
nssm start AIOps-Agent
```

**Windows (Task Scheduler)**: use `deploy/start-agent.bat` wrapper, `schtasks /Create /TN "AIOps-Agent" /TR "C:\aiops-agent\start-agent.bat" /SC ONSTART /RU SYSTEM /RL HIGHEST /F`

**macOS (launchd)**:
```bash
cp deploy/com.aiops.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.aiops.agent.plist
```

See [INSTALL.md](INSTALL.md) for detailed deployment guide.

---

## Reverse Proxy / Domain Access (Nginx)

When exposing via domain + HTTPS through Nginx, **regular monitoring (metric reporting, dashboard) works with default HTTP proxying**; but **remote terminal** uses **WebSocket upgrade + long-connection real-time streaming**, which Nginx **does not forward by default** (no `Upgrade` header, buffers output), causing "metrics fine, terminal won't connect."

This is not unique to this project — all WebSocket apps (Grafana / Jupyter / code-server) need these lines behind Nginx. The server auto-sends `X-Accel-Buffering: no` on downstream streams, so you need very little:

```nginx
# http {} block, once globally
map $http_upgrade $connection_upgrade { default upgrade; '' close; }

location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host  $host;         # Auto-use domain in install command
    proxy_set_header X-Real-IP         $remote_addr;  # Real client IP (with trust_proxy)

    # —— Remote terminal essentials (all required) ——
    proxy_set_header Upgrade    $http_upgrade;         # Forward WebSocket upgrade
    proxy_set_header Connection $connection_upgrade;
    proxy_buffering         off;                       # Disable buffering
    proxy_request_buffering off;
    proxy_read_timeout  3600s;                          # Long-connection keepalive
    proxy_send_timeout  3600s;
}
```

> Full example: **[deploy/nginx-aiops.conf](deploy/nginx-aiops.conf)**. After editing: `nginx -t && nginx -s reload`.
>
> **Real client IP**: behind a reverse proxy, set `"trust_proxy": true` in `server_config.json` so the server honors `X-Real-IP`/`X-Forwarded-For` for audit logs and login rate-limiting; keep default `false` when directly exposed (headers are forgeable).

---

## Security

### Login & Authentication

- **Login auth**: Username + password (salted SHA-256) + session Cookie; login form **does not pre-fill default admin** (prevents brute-force probing); use the admin account set during deployment for first login, then change username and password.
- **Username changeable**: Login username can be changed in the「Profile」dialog (2–32 chars, letters/digits/-_., constant-time compare).
- **Two-factor auth (MFA / TOTP)**: Supports **Google Authenticator** TOTP as a second factor. After enabling, login requires password + 6-digit TOTP code; MFA-enabled status is only revealed after password verification, preventing probing.
- **Login rate-limiting**: Per-client-IP sliding window (default 8 failures per 5 min), failures logged to system log, brute-force protection.
- **Session Cookie security**: `HttpOnly` + `SameSite=Lax`; `Secure` added on HTTPS (incl. reverse proxy `X-Forwarded-Proto`); password change clears all sessions.

### Account Recovery

- **Forgot username**: Click「Forgot Username」on login page → enter bound email → system sends username notification email (anti-enumeration: always returns same success response regardless of email match).
- **Forgot password**: Click「Forgot Password」on login page → enter username → system sends 6-digit code to bound email (10-min TTL, single-use) → enter code + new password to reset. All sessions cleared after reset, old cookies invalidated.
- **Email code security**: 6-digit random code, 10-min TTL, deleted immediately after verification; max 1 send per 60s per email (anti-abuse); username comparison uses constant-time compare.

### MFA Unbind via Email

- When a user loses their phone and can't generate TOTP codes, they can unbind MFA via **bound email**:
  1. Click「Unbind via Email」in the「Disable Two-Factor」dialog
  2. System sends 6-digit code to bound email (10-min TTL, single-use)
  3. Enter correct code to disable MFA; operation logged
- If no email is bound, prompts to bind an email first.

### Agent & Data Security

- **Request body limit**: Global `MaxBytesReader` (2 MiB), prevents oversized JSON memory exhaustion.
- **Security headers**: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` (anti-clickjacking), `Referrer-Policy: no-referrer`.
- **Secret masking**: Config read API masks Webhook / signing Secret / SMTP password; blank or masked submission preserves original.
- **Mandatory Agent Token (default on)**: `register` and `report` **must carry a valid install Token** (**constant-time compare**); no/wrong Token = `403`. Only `allow_anonymous_agents: true` permits anonymous (not recommended).
- **Token non-leakage**: `/install.sh`, `/install.ps1` are public endpoints but **do not backfill the real Token when the query param is absent** — dashboard-generated commands include the Token (from the authenticated `/install/info`), so legitimate installs work, while direct `curl /install.sh` can't read it.
- **Host identity anti-clone**: Agent identity binds to machine fingerprint (machine-id + MAC); cloned images with copied `agent_state.json` are detected and `host_id` is regenerated, preventing different machines colliding on the same ID.
- **Remote terminal**: Inherently remote command execution — **dual auth**: browser WebSocket requires valid login session, Agent reverse channel requires install Token (constant-time compare); open/close logged for audit; `terminal_disabled: true` globally disables. **Strongly recommended: only enable on trusted networks, behind HTTPS reverse proxy.**
- **For public exposure: place behind a reverse proxy with HTTPS.**

### PWA Security

- Service Worker caches only static assets (HTML/CSS/JS); API requests always go to network (real-time data not cached); offline shows dashboard shell with last-known data snapshot.

---

## Cross-Network Deployment & Remote Terminal

Agent uses **active reverse connection**: the server address is固化 to `--server` at install time. If the monitored host and server are **not on the same LAN** (via WAN/domain), the Agent must use a **public-reachable domain or IP** — otherwise the internal IP only works internally, and so does the remote terminal.

**Specifying external address at install**: The dashboard「Install Agent」dialog auto-derives the server address from the current access URL. For cross-network/domain access, simply access the dashboard via your domain — the install command's server address auto-derives to the current domain (server validates the parameter with strict whitelist, preventing script injection).

**Reverse proxy (nginx / Caddy etc.)**: Remote terminal uses WebSocket + long-connection streaming relay; the proxy must pass WebSocket upgrade and **disable buffering**, otherwise the terminal won't connect or has no output. nginx example:

```nginx
location /api/v1/hosts/ {            # Browser WebSocket
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 3600s;
}
location /api/v1/agent/terminal/ {   # Agent reverse stream (must disable buffering)
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
location / {                         # Other API / dashboard
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
}
```

> If directly exposing the server via public `IP:port` (no reverse proxy), the Agent uses that address — no proxy config needed. To upgrade an existing Agent: re-run the install command with the new address on the target host.

---

## Implemented vs. Roadmap

**Implemented (all tested)**
- [x] Single Go module + `shared/` shared types
- [x] Go Agent core: tri-platform native collection (Linux/Windows/macOS), stable identity, registration, dual-heartbeat reporting, disconnect event re-queue
- [x] **GPU monitoring**: Utilization / VRAM / Temperature (NVIDIA / AMD / Apple, best-effort + cache)
- [x] Plugin runner: subprocess + JSON contract, concurrent execution, timeout isolation, crash-skip
- [x] Python plugin layer + SDK + examples (service probe / CPU anomaly / process monitor / psutil fallback)
- [x] Go server: in-memory store + **multi-level downsampled history** + **embedded lightweight DB persistence** (survives restart)
- [x] **Custom monitoring**: HTTP (status/latency/cert days) / TCP / **Ping (loss%/RTT)** / process alive; list·pill dual view; **per-item history curve**
- [x] **Interactive trend charts**: hover crosshair + value tooltip, drag-zoom, double-click reset, enlarge preview (CPU/Mem/Disk/GPU/Network)
- [x] Auth & security: salted password + session Cookie (HttpOnly/SameSite/Secure), **login rate-limiting**, **mandatory Agent Token (default, constant-time)**, security headers, body limit, secret masking, **host identity anti-clone**, **no default admin in login form**, **username changeable**
- [x] **Two-factor auth (MFA / TOTP)**: Google Authenticator compatible, enable/disable, QR code enrollment
- [x] **Account recovery**: forgot username (email), forgot password (email code reset), anti-enumeration
- [x] **MFA unbind via email**: prevents lost-phone account lockout
- [x] **Email alert push (SMTP)**: HTML email template, implicit TLS / STARTTLS, password masking
- [x] Real-time dashboard: overview cards + resource TOP10 (CPU/Mem/Disk/GPU + HTTP/TCP/Ping/process) + host category grouping/search/pagination + **card·list dual view** + threshold alerts + paginated activity log + standard/wide toggle
- [x] Alert push: Feishu / DingTalk Webhook + **Email SMTP**, dedup + state transition
- [x] **gzip response compression**: ~8–10x bandwidth reduction for multi-host polling
- [x] **PWA installable**: manifest + Service Worker + offline cache + icon + shortcuts
- [x] **Mobile responsive**: phone portrait/landscape adaptation, sidebar drawer, touch optimization, safe-area inset
- [x] **Multi-select category filter + collapse**: multi-select dropdown, overview linkage, category collapse/expand
- [x] **Keyboard shortcuts**: number keys 1–5 to switch views
- [x] **Remote terminal**: one-click browser terminal from host card, via Agent reverse connection (**no inbound port on target**) + server relay; **full interactive TTY** (Windows ConPTY, Linux/macOS openpty), supports color/line-editing/vim·top full-screen programs, **window maximize·restore·close** + size auto-adapt; login session + install Token dual auth + open/close audit
- [x] One-click install: Token mode, dashboard-generated command, auto-download, boot autostart
- [x] Host management: category tags, dashboard manual override, host deletion

**In Progress / Next**
- [ ] **Ultra-large scale (10k+)**: external time-series DB (VictoriaMetrics), server-side `/hosts` pagination/incremental, configurable history retention
- [ ] **Terminal enhancements**: session recording/playback, multi-tab, command-level audit, read-only旁观
- [ ] **Multi-tenant auth**: backend RBAC, multi-user
- [ ] **Automation ops**: playbook orchestration + batch execution
- [ ] **Plugin enhancements**: per-plugin interval, plugin-level config, metric types (counter/histogram)

**AIOps Evolution Layer (as Python plugins)**
- [ ] Time-series anomaly detection (Prophet / statsmodels), alert noise reduction/correlation, root cause analysis, capacity forecasting
- [ ] Intelligent ops assistant — integrate RAGFlow + Dify + local vLLM knowledge base stack

---

## Tech Stack

| Component | Technology |
|---|---|
| Agent core | Go 1.22+, pure stdlib, zero third-party deps |
| Server | Go 1.22+, `net/http` (Go 1.22 routing), `embed` for dashboard |
| Dashboard | Vanilla HTML/CSS/JS, no framework deps |
| Plugin layer | Python 3 + psutil (optional) |
| Alert push | Feishu / DingTalk Webhook + Email SMTP (`net/smtp` + `crypto/tls`, zero-dep) |
| PWA | manifest.json + Service Worker + icon.svg |

---

## License

MIT
