# HTTP 代理 "无法解析上游响应" 问题修复

## 问题根因

通过代码分析发现，问题的根本原因是 **session 生命周期管理不当**：

1. **Agent 端**：tx goroutine 读取完上游响应后会立即关闭连接，导致 rx goroutine 写入失败
2. **Server 端**：`handleAgentForwardTx` 在 Agent 读取完成后会立即调用 `defer sess.close()`，导致 pipe reader goroutine 退出，但此时 `http.ReadResponse` 可能还在读取数据，造成 "unexpected EOF"

## 修复内容

### Agent 端修复 (`cmd/agent/forward.go`)
- 移除了不适当的 `SetDeadline` 全局超时设置
- 修改了连接生命周期管理，让 rx goroutine 单独控制连接关闭
- tx goroutine 不再在读取完成后立即关闭连接，避免截断 rx 的写入

### Server 端修复 (`cmd/server/forward.go`)
- 移除了 `handleAgentForwardTx` 中的 `defer sess.close()`
- 让 HTTP proxy handler 控制 session 的生命周期
- 改进了错误信息，提供更友好的中文提示

## 编译完成

已编译的文件：
- `aiops-server.exe` - 服务端（Windows）
- `aiops-agent.exe` - Agent 端（Windows）

## 部署步骤

### 1. 备份现有文件
```bash
# 在远程服务器上执行
cd /opt/aiops-monitor
cp aiops-server.exe aiops-server.exe.backup-$(date +%Y%m%d)
cp aiops-agent.exe aiops-agent.exe.backup-$(date +%Y%m%d)
```

### 2. 上传新文件
```bash
# 在本地执行
scp aiops-server.exe root@192.168.30.15:/opt/aiops-monitor/
scp aiops-agent.exe root@192.168.30.15:/opt/aiops-monitor/
```

### 3. 重启服务
```bash
# 在远程服务器上执行
cd /opt/aiops-monitor
./restart-server.sh
```

或者手动重启：
```bash
# 停止旧进程
pkill -f aiops-server.exe

# 启动新进程（前台运行）
./aiops-server.exe

# 或后台运行
nohup ./aiops-server.exe > server.log 2>&1 &
```

### 4. 更新 Agent（所有被监控的机器）
```bash
# 在每台被监控的机器上执行
cd /opt/aiops-agent
cp aiops-agent.exe aiops-agent.exe.backup-$(date +%Y%m%d)
# 上传新的 aiops-agent.exe 到 /opt/aiops-agent/
./restart-agent.sh
```

### 5. 验证修复
```bash
# 查看日志
tail -f /opt/aiops-monitor/server.log

# 测试 HTTP 代理
curl http://localhost:8529/proxy/{hostID}/{port}/test
```

## 预期效果

修复后：
- ✅ 不再出现 "unexpected EOF" 错误
- ✅ HTTP 代理稳定工作，不会"偶尔才能刷出来"
- ✅ 错误信息更友好，便于诊断

## 技术细节

### 竞态条件分析

**问题场景**：
```
1. 用户请求 → Server → Agent → 目标服务
2. 目标服务返回响应 → Agent 读取完成
3. Agent 的 tx goroutine 读取到 EOF
4. Agent 立即关闭连接 ← 问题发生点
5. Server 的 handleAgentForwardTx 调用 sess.close()
6. Server 的 pipe reader goroutine 退出
7. http.ReadResponse 还在读取数据
8. 管道关闭 → "unexpected EOF"
```

**修复方案**：
- Agent 端：tx goroutine 不关闭连接，由 rx goroutine 统一管理
- Server 端：`handleAgentForwardTx` 不调用 `sess.close()`，由 HTTP proxy handler 控制

这样确保了数据流的完整性，避免了竞态条件。
