"""HC-AIOps Monitor 插件 SDK —— 让写一个插件变成几行代码。

一个插件 = 一个可执行脚本，向 stdout 打印一个 JSON 对象：

    {
      "metrics": {"自定义指标名": 数值, ...},          # 可选：自定义采集(gauge)
      "events":  [{"level": "warning", "message": "..."}],  # 可选：AI/异常/服务检查等发现
      "base":    {"cpu_percent": ..., ...}            # 可选：基础指标(仅非 Linux 兜底时用)
    }

用法：
    from plugin_sdk import Plugin
    p = Plugin()
    p.metric("mysql.connections", 42)
    p.event("warning", "主从延迟 8s")
    p.emit()

约定：
- metrics 里的 key 建议自带命名空间(如 mysql.、nginx.)避免与其它插件冲突。
- events 的 level 取 info | warning | critical；source 可不填，Go 核心会自动补成插件名。
- 插件应快速返回、不要长期阻塞；崩溃/超时不会影响 Agent 核心，只会被记录并跳过。
"""
import json
import sys


class Plugin:
    def __init__(self):
        self._metrics = {}
        self._events = []
        self._base = None

    def metric(self, name, value):
        """记录一个自定义指标(数值型)。"""
        self._metrics[str(name)] = float(value)
        return self

    def event(self, level, message):
        """产生一条事件(level: info|warning|critical)。"""
        self._events.append({"level": str(level), "message": str(message)})
        return self

    def base(self, **fields):
        """(高级)提供基础指标，仅用于非 Linux 平台兜底。"""
        self._base = fields
        return self

    def emit(self):
        """将结果以 JSON 写到 stdout，供 Go 核心读取。"""
        out = {}
        if self._metrics:
            out["metrics"] = self._metrics
        if self._events:
            out["events"] = self._events
        if self._base is not None:
            out["base"] = self._base
        json.dump(out, sys.stdout)
