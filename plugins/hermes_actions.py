"""Hermes Agent 动作插件 —— 自定义运维动作库。

每个函数 = 一个可被 Hermes 调用的动作。
从命令行调用：python hermes_actions.py <action_name> [host_id] [args_json]

约定：
- 动作函数接受 (host_id: str, args: dict) 参数
- 返回字符串结果（打印到 stdout，供 Hermes 引擎读取）
- 动作应快速返回，不要长期阻塞
- 异常会被引擎捕获并返回给 LLM

用法示例：
  python hermes_actions.py restart_service worker-01 '{"service":"nginx"}'
  python hermes_actions.py clear_cache worker-01 '{"path":"/tmp/cache"}'
  python hermes_actions.py scale_pods worker-01 '{"replicas":3,"deployment":"api-gateway"}'
"""

import json
import sys
import os
import subprocess


def restart_service(host_id: str, args: dict) -> str:
    """重启指定服务"""
    service = args.get("service", "")
    if not service:
        return "错误：请指定服务名称 (service)"
    # 通过 systemctl 重启（需要在 Agent 主机上执行）
    try:
        result = subprocess.run(
            ["systemctl", "restart", service],
            capture_output=True, text=True, timeout=30
        )
        if result.returncode == 0:
            return f"服务 {service} 重启成功"
        return f"服务 {service} 重启失败：{result.stderr.strip()}"
    except subprocess.TimeoutExpired:
        return f"服务 {service} 重启超时"
    except FileNotFoundError:
        return "systemctl 不可用，请确认在 Linux 主机上执行"


def clear_cache(host_id: str, args: dict) -> str:
    """清理指定路径的缓存"""
    path = args.get("path", "/tmp/cache")
    try:
        if not os.path.exists(path):
            return f"路径 {path} 不存在，无需清理"
        # 安全检查：只允许清理 /tmp 和 /var/cache 下的路径
        abs_path = os.path.abspath(path)
        if not (abs_path.startswith("/tmp") or abs_path.startswith("/var/cache")):
            return f"安全限制：不允许清理 {path}，仅允许 /tmp 和 /var/cache 下的路径"
        # 统计清理前的大小
        total_size = 0
        for root, dirs, files in os.walk(abs_path):
            for f in files:
                fp = os.path.join(root, f)
                try:
                    total_size += os.path.getsize(fp)
                except OSError:
                    pass
        # 清理
        for root, dirs, files in os.walk(abs_path, topdown=False):
            for f in files:
                try:
                    os.remove(os.path.join(root, f))
                except OSError:
                    pass
            for d in dirs:
                try:
                    os.rmdir(os.path.join(root, d))
                except OSError:
                    pass
        size_mb = total_size / (1024 * 1024)
        return f"缓存清理完成：{path} 释放了 {size_mb:.1f} MB"
    except Exception as e:
        return f"清理缓存失败：{e}"


def scale_pods(host_id: str, args: dict) -> str:
    """扩缩容 Kubernetes Pods"""
    replicas = args.get("replicas", 1)
    deployment = args.get("deployment", "")
    namespace = args.get("namespace", "default")
    if not deployment:
        return "错误：请指定 deployment 名称"
    try:
        result = subprocess.run(
            ["kubectl", "scale", f"deployment/{deployment}",
             f"--replicas={replicas}", f"-n", namespace],
            capture_output=True, text=True, timeout=30
        )
        if result.returncode == 0:
            return f"Deployment {deployment} 已扩容到 {replicas} 个副本"
        return f"扩容失败：{result.stderr.strip()}"
    except subprocess.TimeoutExpired:
        return f"扩容 {deployment} 超时"
    except FileNotFoundError:
        return "kubectl 不可用，请确认已安装并配置"


def check_service_status(host_id: str, args: dict) -> str:
    """检查服务状态（只读）"""
    service = args.get("service", "")
    if not service:
        return "错误：请指定服务名称"
    try:
        result = subprocess.run(
            ["systemctl", "status", service, "--no-pager"],
            capture_output=True, text=True, timeout=10
        )
        # 提取关键行
        lines = result.stdout.strip().split("\n")
        key_lines = [l.strip() for l in lines if l.strip() and (
            "Active:" in l or "Loaded:" in l or "Main PID:" in l or "Memory:" in l or "CGroup:" in l
        )]
        if key_lines:
            return f"服务 {service} 状态：\n" + "\n".join(key_lines[:5])
        return f"服务 {service} 状态：\n" + "\n".join(lines[:5])
    except FileNotFoundError:
        return "systemctl 不可用"


def list_actions() -> str:
    """列出所有可用动作"""
    return json.dumps({
        "actions": [
            {"name": "restart_service", "description": "重启指定服务", "params": ["service"]},
            {"name": "clear_cache", "description": "清理指定路径缓存", "params": ["path"]},
            {"name": "scale_pods", "description": "Kubernetes Pod 扩缩容", "params": ["replicas", "deployment", "namespace"]},
            {"name": "check_service_status", "description": "检查服务状态", "params": ["service"]},
        ]
    }, ensure_ascii=False)


# --- 注册表 ---
ACTIONS = {
    "restart_service": restart_service,
    "clear_cache": clear_cache,
    "scale_pods": scale_pods,
    "check_service_status": check_service_status,
    "list_actions": lambda h, a: list_actions(),
}


def main():
    if len(sys.argv) < 2:
        print(list_actions())
        return

    action_name = sys.argv[1]
    host_id = sys.argv[2] if len(sys.argv) > 2 else ""
    args_str = sys.argv[3] if len(sys.argv) > 3 else "{}"

    try:
        args = json.loads(args_str)
    except json.JSONDecodeError:
        args = {}

    action = ACTIONS.get(action_name)
    if not action:
        print(f"未知动作：{action_name}。可用动作：{', '.join(ACTIONS.keys())}")
        return

    result = action(host_id, args)
    print(result)


if __name__ == "__main__":
    main()