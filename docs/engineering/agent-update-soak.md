# Agent 热更新与终端权限浸泡清单

基线：**v0.19.64+**。用于发布前手工/半自动回归，防止 Windows Job、macOS kickstart、Linux 双 unit / ProtectSystem、nsenter 只读再次 silently regress。

## 1. 环境矩阵

| 平台 | 最低覆盖 | 备注 |
|------|----------|------|
| Windows Server 2019/2022 | 1 台服务安装 Agent | 验证 Job breakaway + ProgramData helper |
| Windows Server 2012/R2（可选） | 1 台 | 专用 `*-win2012.exe` |
| Ubuntu 22.04 / RHEL/Rocky 9 | 各 1 台 | canonical unit=`aiops-agent` |
| 遗留 unit 主机 | 1 台仍跑 `aiops-monitor-agent` | 升级应迁移/兼容重启 |
| macOS（可选） | 1 台 LaunchDaemon | `--install-service` 后 kickstart |
| 容器内 Agent（可选） | Docker privileged 或挂载 host PID/NS | 无 nsenter 时降级纯 shell |

## 2. 热更新（agent_update）验收

对每台目标主机：

1. 控制台「自动更新」推送目标版本（或 API `POST /api/v1/agents/update`）。
2. 任务进入 `running` → `pending_verify` → 版本 ACK 后 `success`（勿在 `pending_verify` 窗口内 soft-retry 风暴）。
3. **Windows**：`ProgramData\aiops-agent-update\` 有 helper 日志；服务停止时 helper 不被 Job 连带杀死；最终进程版本 = 目标。
4. **Linux**：`systemctl cat aiops-agent`（或遗留名）中 `ProtectSystem=false`、`User=root`（除非显式 `allow-nonroot`）；热更新后远程 vim `/etc/hosts` 可写。
5. **macOS**：更新后 `launchctl print` 服务为 running。
6. 失败路径：故意下发错误 SHA / 停掉 dist → 状态 `failed`，冷却释放，可再次触发。

自动化门禁（CI）：

```bash
go test ./cmd/server/ -count=1 -run 'Legacy.*AgentUpdate|AgentUpdate'
go test ./cmd/agent/ -count=1 -run 'AgentUpdate|UnitHeal|ResolveAgentConfig'
```

## 3. 终端权限验收（Linux）

| 场景 | 期望 |
|------|------|
| root 服务 + 无沙箱 unit | `touch /etc/aiops-term-write-test && rm` 成功 |
| 遗留 ProtectSystem=strict | 启动自愈或 `--install-service` 后恢复可写 |
| 容器 Agent 无 host ns | nsenter 失败 → 降级 shell；文档告知需 `--pid=host` 或宿主安装 |
| `/etc/aiops-agent/allow-nonroot` 存在 | 自愈不强制改 User=root |
| resolv.conf 为 symlink | 终端内 `ls -l /etc/resolv.conf` 正常，不误报只读 |

详见 [terminal-linux-privileges.md](./terminal-linux-privileges.md)。

## 4. systemd 双 unit

- **写入/安装**：永远写 `/etc/systemd/system/aiops-agent.service`。
- **检测/重启/自愈**：优先 `aiops-agent`，回退 `aiops-monitor-agent`。
- 升级后建议 `systemctl disable --now aiops-monitor-agent` 并删除遗留 unit（安装脚本会 purge）。

## 5. 发布检查（摘要）

- [ ] README / docs/i18n badge = 本 tag
- [ ] CHANGELOG 有对应章节
- [ ] `make audit` 或 CI 绿（含 `cmd/agent` 更新相关测试）
- [ ] 至少 1 Win + 1 Linux 完成第 2、3 节手工项
