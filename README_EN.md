# AIOps Monitor

[中文](README.md) | [English](README_EN.md)

> **Lightweight Host Monitoring & Ops Platform** — Go-native collection + Python plugin layer + real-time dashboard + threshold alerts + remote terminal + automation playbooks

Single-binary server, zero-dependency agent, tri-platform native collection (incl. GPU), one-command install, ready out of the box. Built-in interactive trend charts, custom probes, remote terminal (no port opening), automation playbooks, multi-user RBAC, MFA two-factor, embedded persistence, PWA installable.

---

## Platform & Architecture Support

| Architecture | Linux | Windows | macOS |
|---|:---:|:---:|:---:|
| **AMD64 / x86_64** | ✅ | ✅ | ✅ Intel Mac |
| **ARM64 / aarch64** | ✅ | — | ✅ Apple Silicon (M1/M2/M3/M4) |

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

Install scripts auto-detect CPU architecture and download the matching binary — no manual selection needed.

---

## Quick Start

### Docker One-Click (Recommended)

```bash
git clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor
docker compose up -d aiops-server
# Open http://localhost:8529 in your browser
```

> **Default credentials**: `admin / admin`. Change username and password immediately after first login, and consider enabling MFA.

### Binary Direct Run

```bash
# Start server (default listen :8529)
./bin/aiops-server

# Start agent (run from repo root to find plugins/)
./bin/aiops-agent --server http://<server-IP>:8529 --category Production
```

Open `http://localhost:8529` — host card and metrics appear within seconds.

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
| **Automation playbooks** | Multi-step orchestration + target selection (all/category/system/host) → batch parallel execution → real-time output + history |
| **Alert push** | Feishu / DingTalk Webhook + Email SMTP, trigger/recover transitions only, no spam |
| **Multi-user RBAC** | admin / operator / viewer, route-level permission, user management UI |
| **MFA two-factor** | TOTP (RFC 6238), Google Authenticator compatible, QR enrollment |
| **Account recovery** | Forgot username / forgot password (email code) / MFA unbind via email, anti-enumeration |
| **Multi-server push** | Single agent pushes to multiple servers; collect once, broadcast all; independent auth/retry |
| **Gateway relay mode** | One internet-connected machine proxies all requests to cloud; binary/report/terminal auto-tunnel |
| **Machine fingerprint auth** | machine-id + MAC hash fingerprint binding; token rotation doesn't affect installed agents |
| **Persistence** | Embedded lightweight DB (gzip+JSON to `aiops.db`), survives restart |
| **PWA installable** | Install to desktop, Service Worker offline cache, standalone window |
| **gzip compression** | API/static auto-gzip, ~8-10x bandwidth reduction for multi-host polling |
| **One-click install** | Dashboard-generated Token command, auto-download + config + boot autostart |

---

## Installation & Deployment

### Option 1: Docker

```bash
git clone https://github.com/sreyun/aiops-monitor.git && cd aiops-monitor
docker compose up -d aiops-server
```

- Server data persists via volume (`/app/data`), config at `./server_config.json`
- Default port `8529`, modifiable in `docker-compose.yml`
- Agent container not started by default — uncomment `aiops-agent` section to enable
- Docker images support `amd64` and `arm64` dual-arch; `docker pull` auto-matches

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

Click **「Install Agent」** in the dashboard top-right → select target OS → copy command to monitored host:

```bash
# Linux (root/sudo) — auto-detects amd64/arm64
curl -fsSL "http://<server>:8529/install.sh?token=<TOKEN>" | sudo sh

# Windows (admin PowerShell)
irm "http://<server>:8529/install.ps1?token=<TOKEN>" | iex

# macOS — auto-detects Intel/Apple Silicon
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
| `thresholds.offline_after_sec` | int | `30` | Host offline threshold (seconds) |
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

### Server CLI Parameters

| Parameter | Description | Default |
|---|---|---|
| `-addr` | Listen address | `:8529` |
| `-config` | Config file path | `server_config.json` |
| `-dist` | Agent download directory | auto-detect `./dist` or executable dir |

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

The dashboard「Monitoring」page lets you add active probes — periodic checks on websites, ports, host connectivity, and process alive:

| Type | What to fill | Failure condition |
|---|---|---|
| **HTTP website** | URL (e.g. `https://example.com`) | Status ≥ 400, or timeout/failure |
| **TCP port** | host:port (e.g. `10.0.0.5:3306`) | Cannot connect |
| **Ping host** | host/IP (e.g. `8.8.8.8`) | 100% loss (unreachable) |
| **Process alive** | ① Target host + ② Process name | Process not reported by target host (or offline) |

> Process monitoring requires selecting target host first, then process name — the server checks the host's Agent-reported process list. Case-insensitive substring match. Each item supports list/pill dual view + history curve.

---

## Automation Playbook

The dashboard「Automation」page lets you orchestrate playbooks — ordered shell commands executed in batch on target hosts:

**Create playbook**: name + steps, each with:
- **Command**: one-line shell command (Linux `sh -c`, Windows `cmd /c`)
- **Target**: `all` / `category:xxx` / `system:linux|windows|macos` / `host:<ID>`
- **Timeout** (seconds) and **continue on failure**

**Execution**: commands sent via Agent reverse channel, executed as one-shot subprocesses, returning output + exit code. All matching online hosts execute in parallel; each host runs steps sequentially. History retains last 100 runs.

> Commands are non-interactive — don't use `vim`/`top`/`ssh`. Each step is an independent process; `cd`/`export` don't carry over — chain with `&&` in the same step.

---

## Remote Terminal

- **Multi-tab**: one-click from host card, multiple hosts/sessions simultaneously
- **Recording & playback**: auto-recorded (timestamped frames), progress bar drag, speed control
- **Read-only observe**: multiple admins can observe an active session simultaneously
- **Command audit**: executed commands auto-extracted to activity log
- **Cross-platform TTY**: Windows ConPTY (chcp 65001 + GBK→UTF-8), Linux/macOS openpty
- **No port opening**: via Agent reverse connection, no inbound port on target

> Terminal/playbook share the Agent reverse channel — one session per host at a time. Cross-network requires [Nginx WebSocket config](#cross-network-deployment).

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

Drop in `plugins/` directory for auto-discovery, executed every `--plugin-interval`. Crashes/timeouts/bad JSON are logged and skipped — no impact on core. Non-`.py` executables also work as plugins — any language.

---

## Alert Configuration

Alerts are configured visually in the dashboard — no file editing:

1. Click **Alert Settings** in the top-right
2. Fill Feishu or DingTalk Webhook URL (DingTalk: fill Secret if using signing), check enable
3. **Email push**: expand SMTP section, fill server/port/account/auth code, port 465 = implicit TLS, 587 = not
4. Click **Send Test** to verify connectivity
5. Click **Save** — outstanding alerts re-pushed after save

| Alert type | Trigger condition | Level |
|---|---|---|
| CPU / Memory / Disk | Exceeds threshold | Warning / Critical |
| Host offline | No report within threshold | Critical |
| GPU usage | ≥ 80% warning, ≥ 90% critical | Warning / Critical |
| System load | 5-min load ≥ cores×2 | Warning / Critical |
| HTTP / TCP / Ping / Process | Probe failure | Custom |

> Feishu custom bot keyword: `AIOps` or `告警`. DingTalk: use "signing" security.

---

## Advanced Features

### Multi-Server Push

A single agent instance pushes to multiple monitoring servers simultaneously. **Collection executes once, results broadcast to all servers.**

**Configuration**: Use `servers` array in `config.json` (see Configuration Reference above), or check「Multi-Server Push」in the dashboard install dialog.

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
# ① Gateway machine (internet-connected)
curl -fsSL "https://cloud-server/install-relay.sh?token=TOKEN" | sudo sh

# ② Internal machine (via gateway)
curl -fsSL "http://<gateway-IP>:8529/install.sh?token=TOKEN" | sudo sh
```

> Relay and multi-server push are mutually exclusive: Relay = "one machine proxies all to one upstream"; multi-server = "one machine pushes to multiple upstreams".

### Machine Fingerprint Auth

Agent sends machine fingerprint (machine-id + primary MAC SHA-256 first 12 hex) to server at registration. All subsequent reports and terminal channel requests authenticate via fingerprint, **not install Token** — token rotation doesn't affect installed agents. Each server validates fingerprints independently in multi-server scenarios.

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
- Route-level interception: every API request checked by `authMiddleware` → `routeAllowed`

### Account Recovery

- **Forgot username**: Enter bound email → receive username notification (anti-enumeration)
- **Forgot password**: Enter username → receive 6-digit code (10-min TTL) → reset after verification
- **MFA unbind via email**: Lost phone? Unbind MFA via bound email verification code
- Code security: 6-digit random, 10-min TTL, single-use, 60s send interval limit

### Agent & Data Security

- **Mandatory Agent Token** (default on): `register`/`report` must carry valid Token (constant-time compare)
- **Request body limit**: 2 MiB, prevents oversized JSON memory exhaustion
- **Security headers**: `nosniff`, `DENY` (anti-clickjacking), `no-referrer`
- **Secret masking**: Webhook/SMTP passwords masked on display, blank preserves original
- **Host identity anti-clone**: Cloned images with copied `agent_state.json` detected, `host_id` regenerated
- **Remote terminal dual auth**: Browser needs login session + Agent needs Token; open/close audited
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

    # —— Remote terminal essentials (all required) ——
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
> Cloud load balancers (ALB/CLB/K8s Ingress) similarly need WebSocket support, disabled buffering, idle timeout ≥1h.

### Terminal Tunnel

Agent uses **active reverse connection**: server address is固化 to `--server` at install time. Cross-network requires a **public-reachable domain or IP**. The dashboard install dialog auto-derives server address from current access URL — access via domain and the install command auto-uses that domain.

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

- Windows ConPTY auto-applies `chcp 65001` + GBK→UTF-8 conversion
- Playbook execution has 3-layer encoding: chcp 65001 + locale env vars + GBK→UTF-8 API fallback
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
- GPU is best-effort — no tool = no display, doesn't affect other metrics
</details>

---

## Tech Stack & Architecture

### Tech Stack

| Component | Technology |
|---|---|
| Agent core | Go 1.22+, pure stdlib, zero third-party deps |
| Server | Go 1.22+, `net/http` (Go 1.22 routing), `embed` for dashboard |
| Dashboard | Vanilla HTML/CSS/JS, no framework deps |
| Plugin layer | Python 3 + psutil (optional) |
| Alert push | Feishu/DingTalk Webhook + Email SMTP (`net/smtp` + `crypto/tls`) |
| PWA | manifest.json + Service Worker + icon.svg |

### Architecture Diagram

```
                ┌─────────────── Go Agent Core ────────────────┐
                │  Collector (tri-platform native) → base       │
                │  PluginRunner → concurrent Python plugins     │
                │  Reporter → broadcast to all servers           │
  Report ─HTTP─►│  Terminal → per-server reverse channel        │
                │  Shares types with server via shared/          │
                └──┬──────────────────────────┬─────────────────┘
                   │                          │
              ┌────┴────┐               ┌─────┴─────┐
              │ Server A │               │  Server B  │  (multi-server push)
              └─────────┘               └───────────┘
                                               │ subprocess + JSON
                    ┌──────────────────────────┼──────────────────────┐
              ┌─────┴───────┐          ┌───────┴───────┐       ┌──────┴───────┐
              │ Custom       │          │ AI / Anomaly   │       │ Process      │
              │ collection   │          │ detection      │       │ Monitor      │
              │ (.py)        │          │ (.py)          │       │ (.py)        │
              └──────────────┘          └───────────────┘       └──────────────┘
```

**Design principle**: High-frequency, performance-sensitive base collection uses Go (single binary, zero deps); variable, ecosystem-dependent custom/AI logic uses Python. Process boundary isolates each.

### Directory Structure

```
aiops-monitor/
├── go.mod                          # Go module
├── shared/
│   └── wire.go                     # ★ Shared types (Agent ↔ Server contract)
├── cmd/
│   ├── server/                     # Go server
│   │   ├── main.go                 # Entry, routing, middleware
│   │   ├── handlers.go             # API handlers
│   │   ├── store.go                # In-memory store + multi-level downsampling
│   │   ├── db.go                   # Embedded lightweight DB (gzip+JSON)
│   │   ├── alerts.go               # Threshold alert engine
│   │   ├── auth.go                 # Login auth + MFA + RBAC
│   │   ├── users.go                # Multi-user management
│   │   ├── check.go                # Custom monitoring (HTTP/TCP/Ping/process)
│   │   ├── ws.go                   # Hand-written WebSocket (terminal)
│   │   ├── terminal.go             # Remote terminal relay
│   │   ├── notify.go               # Feishu/DingTalk/Email push
│   │   ├── email.go                # SMTP + verification code manager
│   │   ├── playbook.go             # Automation playbook engine
│   │   ├── totp.go                 # TOTP two-factor auth
│   │   ├── config.go               # Config persistence
│   │   ├── install.go              # One-click install script generation
│   │   └── web/                    # Dashboard frontend (embedded at compile time)
│   │       ├── index.html / app.js / style.css
│   │       ├── manifest.json / sw.js / icon.svg
│   └── agent/                      # ★ Go Agent core
│       ├── main.go                 # Config / flags / signals
│       ├── collector.go            # Collector interface
│       ├── collector_linux.go      # Linux native collection
│       ├── collector_windows.go    # Windows native collection
│       ├── collector_darwin.go     # macOS native collection
│       ├── collector_other.go      # Other platform stub
│       ├── gpu.go                  # GPU collection (tri-platform)
│       ├── terminal.go             # Remote terminal Agent-side
│       ├── pty_windows.go          # Windows ConPTY
│       ├── pty_unix.go             # Linux/macOS openpty
│       ├── pty_linux.go / pty_darwin.go
│       ├── relay.go                # Gateway relay mode
│       ├── plugins.go              # Plugin runner
│       ├── identity.go             # Stable host_id / fingerprint
│       └── reporter.go             # Dual-heartbeat reporting
├── plugins/                        # ★ Python plugin layer
│   ├── plugin_sdk.py               # Plugin SDK
│   ├── core_metrics.py             # psutil fallback
│   ├── example_service_check.py    # Example: service probe
│   ├── example_ai_anomaly.py       # Example: anomaly detection
│   ├── process_monitor.py          # Process monitoring
│   └── requirements.txt
├── deploy/
│   └── nginx-aiops.conf            # Nginx reverse proxy example
├── dist/                           # Agent distribution (platform binaries)
├── bin/                            # Pre-compiled binaries
├── config.example.json             # Agent config example
├── server_config.example.json      # Server config example
├── Dockerfile                      # Multi-stage build
├── docker-compose.yml              # Docker Compose
└── INSTALL.md                      # Detailed installation guide
```

### Key Design

- **Shared code**: `shared/wire.go` imported by both server and agent — contract never drifts
- **Dual-heartbeat**: Base metrics high-frequency; plugins low-frequency, results sent alongside
- **Process isolation**: Plugins run as subprocesses, timeout killable, one bad plugin doesn't crash core
- **Alert dedup**: Only pushes on "new trigger" and "recover" transitions, persistent alerts don't spam
- **Multi-level downsampling**: Raw (~1.5h) / 1-min aggregate (48h) / 5-min aggregate (7 days)
- **Embedded persistence**: gzip+JSON atomic flush to `aiops.db`, periodic save + exit flush
- **gzip compression**: Multi-host polling JSON compresses ~8-10x; WebSocket upgrades auto-skipped

---

## Performance & Scale

- **Bandwidth**: gzip ~8-10x compression, 3000 hosts polling `/hosts` every 3s drops from MB/s to ~100KB/s
- **Report throughput**: 3000 hosts × every 10s ≈ 300 writes/s, `Upsert` briefly holds write lock
- **Memory**: ~1-2 MB per host for 3-layer history, 3000 hosts ≈ 4-7 GB (tunable via retention constants)
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
| GET | `/api/v1/agent/terminal/rx` | Server → Agent frame stream |
| POST | `/api/v1/agent/terminal/tx` | Agent → Server output stream |
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
- [x] Custom monitoring: HTTP / TCP / Ping / process; list·pill dual view + history curves
- [x] Interactive trend charts: hover crosshair + drag-zoom + enlarge preview
- [x] Auth & security: salted password + rate-limiting + mandatory Token + security headers + secret masking + anti-clone
- [x] MFA two-factor (TOTP) + account recovery (email code) + MFA unbind via email
- [x] Email alert push (SMTP)
- [x] Real-time dashboard: overview + TOP10 + category grouping/search/pagination + card·list dual view + wide toggle
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

### In Progress / Planned

- [ ] Ultra-large scale (10k+): external time-series DB (VictoriaMetrics), server-side pagination/incremental, configurable retention
- [ ] Plugin enhancements: per-plugin interval, plugin-level config, metric types (counter/histogram)
- [ ] AIOps evolution: time-series anomaly detection (Prophet / statsmodels), alert noise reduction, root cause analysis, capacity forecasting
- [ ] Intelligent ops assistant: integrate RAGFlow + Dify + local vLLM

---

## License

MIT
