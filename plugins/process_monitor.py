#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""示例插件：自定义进程监控（跨平台，基于 psutil）。

从同目录的 process_monitor.json 读取要监控的进程名列表（子串匹配、不区分
大小写），对每个目标产出：
    proc.<name>.count    匹配到的进程数
    proc.<name>.cpu      这些进程的 CPU 占用之和（%，可能 >100，跨多核）
    proc.<name>.mem_mb   这些进程的常驻内存之和（MB）
进程数为 0 时产生一条 critical 事件——这正是运维最关心的"关键进程是否存活"。

三平台一致：把跨平台、需要生态的采集放在 Python 插件层，正是混合架构的用意。
Linux 上基础指标由 Go 核心原生采集，本插件只做进程维度的自定义采集。
"""
import json
import os
import sys
import time

try:
    import psutil
except ImportError:
    print("{}")  # 没装 psutil 就静默跳过
    sys.exit(0)

from plugin_sdk import Plugin

CONF = os.path.join(os.path.dirname(os.path.abspath(__file__)), "process_monitor.json")

targets = []
if os.path.exists(CONF):
    try:
        with open(CONF, encoding="utf-8") as f:
            targets = [str(t) for t in json.load(f).get("processes", []) if str(t).strip()]
    except Exception:
        targets = []

if not targets:
    print("{}")  # 未配置任何进程，不产出
    sys.exit(0)

# 枚举一次进程，按名做子串匹配
matched = {t: [] for t in targets}
low_targets = [(t, t.lower()) for t in targets]
for p in psutil.process_iter(["name"]):
    try:
        nm = (p.info.get("name") or "").lower()
    except Exception:
        continue
    for t, tl in low_targets:
        if tl in nm:
            matched[t].append(p)

# 为 CPU 采样建立基线，再间隔一次读差值
for plist in matched.values():
    for pr in plist:
        try:
            pr.cpu_percent(None)
        except Exception:
            pass
time.sleep(0.3)

plugin = Plugin()
for t in targets:
    plist = matched[t]
    key = t.replace(" ", "_")
    plugin.metric(f"proc.{key}.count", len(plist))
    if plist:
        cpu = 0.0
        mem = 0
        for pr in plist:
            try:
                cpu += pr.cpu_percent(None)
            except Exception:
                pass
            try:
                mem += pr.memory_info().rss
            except Exception:
                pass
        plugin.metric(f"proc.{key}.cpu", round(cpu, 1))
        plugin.metric(f"proc.{key}.mem_mb", round(mem / 1048576, 1))
    else:
        plugin.event("critical", f"进程 {t} 未运行（监控目标缺失）")

plugin.emit()
