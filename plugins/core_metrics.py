#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""核心指标采集插件（基于 psutil）。

用途：在【非 Linux】平台（Windows / macOS）上为 Go Agent 核心提供基础指标，
作为原生采集器就绪前的兜底。这体现了混合架构的一个优点——Python 插件层
甚至能补齐 Go 核心暂未原生支持的平台。

说明：
- 在 Linux 上，Go 核心已用 /proc + syscall 原生采集，本插件产出会被忽略，可删除。
- 自采两次网络计数(间隔 0.3s)计算速率，无需跨进程状态。
"""
import json
import os
import sys
import time

try:
    import psutil
except ImportError:
    print("{}")  # 没装 psutil 就不产出，交给原生采集器/其它来源
    sys.exit(0)

IS_WINDOWS = os.name == "nt"
DISK = (os.environ.get("SystemDrive", "C:") + "\\") if IS_WINDOWS else "/"

cpu = psutil.cpu_percent(interval=0.3)
vm = psutil.virtual_memory()

try:
    du = psutil.disk_usage(DISK)
    d_total, d_used, d_pct = du.total, du.used, du.percent
except Exception:
    d_total = d_used = 0
    d_pct = 0.0

n1 = psutil.net_io_counters()
time.sleep(0.3)
n2 = psutil.net_io_counters()
sent = max(0.0, (n2.bytes_sent - n1.bytes_sent) / 0.3)
recv = max(0.0, (n2.bytes_recv - n1.bytes_recv) / 0.3)

try:
    load1 = os.getloadavg()[0]
except (AttributeError, OSError):
    load1 = 0.0

base = {
    "cpu_percent": round(cpu, 1),
    "cpu_cores": psutil.cpu_count(logical=True) or 0,
    "mem_total": int(vm.total),
    "mem_used": int(vm.total - vm.available),
    "mem_percent": round(vm.percent, 1),
    "disk_total": int(d_total),
    "disk_used": int(d_used),
    "disk_percent": round(d_pct, 1),
    "net_sent_rate": round(sent, 1),
    "net_recv_rate": round(recv, 1),
    "load1": round(load1, 2),
    "proc_count": len(psutil.pids()),
    "uptime": int(time.time() - psutil.boot_time()),
}

json.dump({"base": base}, sys.stdout)
