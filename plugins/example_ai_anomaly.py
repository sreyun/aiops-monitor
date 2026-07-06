#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""示例插件：轻量异常检测（AI / AIOPS 层）。

对本机 CPU 使用率维护一个滚动基线（均值 / 标准差），用 z-score 判定异常，
异常时产出事件，并输出异常分数指标。这是把"异常检测 / 智能分析"放在 Python
侧的示范——真实场景可无缝替换为 Prophet、statsmodels、sklearn，或调用你
已有的模型服务（含你现有的 RAGFlow + Dify + 本地 vLLM 栈）。

状态（滚动窗口）持久化到插件目录下的 .anomaly_state.json，跨次运行累积基线。
"""
import json
import os
import sys

# 取当前 CPU 使用率：优先 psutil；无 psutil 时用 loadavg 粗略替代（仅类 Unix）。
try:
    import psutil
    cpu = psutil.cpu_percent(interval=0.3)
except ImportError:
    try:
        with open("/proc/loadavg") as f:
            cpu = min(100.0, float(f.read().split()[0]) * 10.0)
    except Exception:
        print("{}")
        sys.exit(0)

STATE = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".anomaly_state.json")
WINDOW = 30      # 滚动窗口样本数
MIN_SAMPLES = 8  # 达到多少样本后才开始判定
Z_THRESHOLD = 3.0

hist = []
if os.path.exists(STATE):
    try:
        with open(STATE) as f:
            hist = json.load(f).get("hist", [])
    except Exception:
        hist = []

out = {"metrics": {}, "events": []}

if len(hist) >= MIN_SAMPLES:
    mean = sum(hist) / len(hist)
    var = sum((x - mean) ** 2 for x in hist) / len(hist)
    std = var ** 0.5
    z = (cpu - mean) / std if std > 1e-6 else 0.0
    out["metrics"]["cpu.anomaly_zscore"] = round(z, 2)
    if z >= Z_THRESHOLD:
        out["events"].append({
            "level": "warning",
            "message": f"CPU 异常升高：当前 {cpu:.1f}%，基线 {mean:.1f}% (z={z:.1f})",
        })

# 更新滚动窗口
hist.append(round(cpu, 1))
hist = hist[-WINDOW:]
try:
    with open(STATE, "w") as f:
        json.dump({"hist": hist}, f)
except Exception:
    pass

# 去掉空字段后输出
if not out["metrics"]:
    out.pop("metrics")
if not out["events"]:
    out.pop("events")
json.dump(out, sys.stdout)
