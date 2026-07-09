#!/bin/bash
set -e

SERVER="192.168.30.15"
USER="root"
REMOTE_PATH="/opt/aiops-monitor"

echo "========================================"
echo "部署 AIOps 服务端到 $SERVER"
echo "========================================"
echo

# 1. 上传新编译的服务端程序
echo "[1/4] 上传 aiops-server.exe..."
scp aiops-server.exe ${USER}@${SERVER}:${REMOTE_PATH}/aiops-server.exe.new
echo "✓ 上传完成"
echo

# 2. 停止旧服务
echo "[2/4] 停止旧服务..."
ssh ${USER}@${SERVER} "systemctl stop aiops-monitor 2>/dev/null || pkill -f aiops-server.exe || true"
sleep 2
echo "✓ 已停止"
echo

# 3. 替换二进制文件
echo "[3/4] 替换二进制文件..."
ssh ${USER}@${SERVER} "cd ${REMOTE_PATH} && mv aiops-server.exe.new aiops-server.exe && chmod +x aiops-server.exe"
echo "✓ 替换完成"
echo

# 4. 启动新服务
echo "[4/4] 启动新服务..."
ssh ${USER}@${SERVER} "systemctl start aiops-monitor 2>/dev/null || cd ${REMOTE_PATH} && nohup ./aiops-server.exe > server.log 2>&1 &"
sleep 3
echo "✓ 启动完成"
echo

# 验证服务状态
echo "验证服务状态..."
ssh ${USER}@${SERVER} "ps aux | grep aiops-server.exe | grep -v grep" || echo "服务可能未启动，请检查日志"
echo
echo "========================================"
echo "部署完成！"
echo "访问地址: http://${SERVER}:8529"
echo "========================================"
