# HTTP 代理 "unexpected EOF" 问题修复部署指南

## 问题描述
HTTP 代理访问时出现"无法解析上游响应: unexpected EOF"错误，偶尔可以正常访问。

## 根本原因
Agent 端连接目标服务时缺乏超时控制，导致：
1. 连接可能挂起过久后被意外关闭
2. 上游服务响应慢时连接超时
3. 错误信息不够详细，难以诊断

## 修复内容

### Agent 端修复 (`cmd/agent/forward.go`)
- ✅ 添加 5 秒连接超时
- ✅ 添加读写超时（HTTP: 60秒, TCP: 30秒）
- ✅ 改进错误信息和诊断日志
- ✅ 修复 Content-Length 计算错误

### Server 端修复 (`cmd/server/forward.go`)
- ✅ 改进超时错误信息
- ✅ 添加友好的错误提示
- ✅ 增强日志记录

## 部署步骤

### 1. 重新编译服务端
```bash
# 在开发机上执行
cd D:\个人专用\工具开发\aiops-monitor
go build -o aiops-server.exe cmd/server/*.go
```

### 2. 重新编译 Agent（所有被监控机器都需要更新）
```bash
# Windows Agent
GOOS=windows GOARCH=amd64 go build -o aiops-agent.exe cmd/agent/*.go

# Linux Agent
GOOS=linux GOARCH=amd64 go build -o aiops-agent cmd/agent/*.go
```

### 3. 部署服务端
```bash
# 上传到远程服务器
scp aiops-server.exe root@192.168.30.15:/opt/aiops-monitor/

# SSH 到服务器
ssh root@192.168.30.15

# 停止旧服务
systemctl stop aiops-monitor
# 或者
pkill -f aiops-server.exe

# 替换二进制文件
cd /opt/aiops-monitor
mv aiops-server.exe aiops-server.exe.bak
mv aiops-server.exe.new aiops-server.exe
chmod +x aiops-server.exe

# 启动新服务
systemctl start aiops-monitor
```

### 4. 部署 Agent（每台被监控机器）
```bash
# 在被监控机器上
cd /path/to/agent
mv aiops-agent aiops-agent.bak
# 上传新的 agent 并替换
chmod +x aiops-agent
systemctl restart aiops-agent
```

### 5. 验证修复
访问之前失败的 HTTP 代理链接，应该：
- 要么正常显示页面
- 要么显示友好的错误信息（如"上游服务未返回响应"、"读取上游响应超时"等）
- 不再出现 "unexpected EOF"

## 临时解决方案（不重启服务）

如果暂时无法部署，可以通过以下方式缓解：
1. **增加重试**：浏览器遇到错误后手动刷新页面
2. **检查目标服务**：确保被监控的 HTTP 服务响应正常、没有超时
3. **调整网络配置**：检查网络延迟和丢包率

## 监控和日志

修复后，服务端会记录详细的日志：
```
log: 2026-07-09 HTTP代理超时 host=xxx port=8080 path=/api/health err="上游服务响应超时（60秒）"
```

Agent 端也会记录：
```
log: 2026-07-09 HTTP转发读取上游响应失败 session=xxx target=localhost:8080 err="context deadline exceeded" detail="读取上游响应超时（服务响应过慢）"
```

## 预期效果

✅ 不再出现 "unexpected EOF" 错误
✅ 连接失败时有明确的错误提示
✅ 慢速服务会超时而不是挂起
✅ 日志中包含详细的诊断信息
