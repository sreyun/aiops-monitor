# Linux 远程终端权限边缘说明

针对「看起来是 root，但 vim `/etc` 报 E45 只读」类问题。代码基线：`unit_heal_linux.go`、`term_nsenter_linux.go`、`service_linux.go`（v0.19.61+）。

## 根因摘要

| 原因 | 表现 | 处理 |
|------|------|------|
| systemd `ProtectSystem`/`ProtectHome` | 即使用户为 root，挂载命名空间内 `/etc`/`/usr` 只读 | unit 显式 `Protect*=false`；启动自愈重写 |
| 历史安装用了 `SUDO_USER` | 进程非 root，无法写系统文件 | 默认 `User=root`；安装/升级 `--install-service` |
| 热更新在沙箱 NS 内跑 | 写 `/etc/systemd` 失败，旧沙箱 unit 残留 | 升级助手经 `nsenter -t 1 -m` 或 `host_run` 再装 |
| Agent 跑在容器内 | 无宿主机 mount NS，nsenter 失败 | 宿主安装 Agent，或容器 `--pid=host` + 合适能力 |
| 主动选择非 root | 需要受限账号 | 创建 `/etc/aiops-agent/allow-nonroot` 后自愈不再强制 root |

## allow-nonroot

```bash
sudo mkdir -p /etc/aiops-agent
sudo touch /etc/aiops-agent/allow-nonroot
# 然后按需编辑 unit User= 并 daemon-reload + restart
```

存在该文件时，Agent **不会**把 User 强制改回 root，但仍会尝试关闭 Protect* 沙箱（除非你自行加硬 drop-in）。

## nsenter 行为

交互终端优先：

```text
nsenter -t 1 -m -u -i -n -p --wd=/root -- /bin/bash -l
```

- `--wd` 使用宿主机路径（通常 `/root`），避免容器内 cwd。
- nsenter 不可用时降级为进程内 PTY/shell，并在日志中记录；此时若 Agent 本身在只读 NS，权限问题无法从终端侧单独修复。

## 容器内 Agent

不推荐把生产 Agent 放在无 host PID/mount 的普通容器里做「等同宿主」运维。若必须：

1. `--pid=host --privileged`（或精细 cap + 挂载）以便 nsenter；或
2. 仅做指标采集，远程变更改走宿主 Agent / 剧本模块。

## resolv.conf symlink

部分发行版 `/etc/resolv.conf` → `../run/systemd/resolve/...`。这与 ProtectSystem 无关；若编辑失败，检查目标路径是否在只读挂载上，或用 `resolvectl` / NetworkManager 改 DNS。

## Android / 移动推送说明

控制台与 Android App 告警推送走自建 **`/ws/push` WebSocket**，不依赖 FCM/APNs。外网断开时需 VPN 或反向代理保活；系统杀后台时依赖前台服务保活策略（见移动端仓库说明）。**FCM 为后续可选项，本仓库不内置。**

## OTel（增量）

指标栈仍以 Victoria/VictoriaMetrics 为主。外部 OTel Collector 可通过 Prometheus remote_write 打到本平台 `/api/v1/prom/write`。完整 OTLP APM（traces 存储）不在近期范围，见 [year1-acceptance.md](./year1-acceptance.md)。
