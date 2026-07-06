#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""示例插件：服务健康检查（自定义采集 + 事件）。

检查若干 TCP 端口是否可达，产出连通性/时延指标；不可达时产生 critical 事件。
这类"业务/中间件探活"正是适合放在 Python 插件层的自定义采集，Go 核心无需改动。

真实场景可把 TARGETS 改为从配置或服务发现读取，或扩展为 HTTP 探活、SQL 探活等。
"""
import socket
import time

from plugin_sdk import Plugin

# 待检查的服务：(host, port, 名称)。
# 注意：不要探测本机监控 API（127.0.0.1:8080），该端点已由服务端内置自监控负责。
# 此插件应用于监控外部依赖服务（如数据库、缓存、第三方 API）。
TARGETS = [
    # ("db.example.com", 3306, "mysql"),
    # ("redis.example.com", 6379, "redis"),
    # ("api.example.com", 443, "external-api"),
]

p = Plugin()
for host, port, name in TARGETS:
    t0 = time.time()
    ok = False
    try:
        with socket.create_connection((host, port), timeout=2):
            ok = True
    except OSError:
        ok = False
    latency_ms = round((time.time() - t0) * 1000, 1)

    p.metric(f"svc.{name}.up", 1 if ok else 0)
    if ok:
        p.metric(f"svc.{name}.latency_ms", latency_ms)
    else:
        p.event("critical", f"服务 {name} ({host}:{port}) 不可达")

p.emit()
