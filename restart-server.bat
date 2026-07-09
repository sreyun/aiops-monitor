@echo off
chcp 65001 >nul
echo ==========================================
echo 重启 AIOps 服务端
echo ==========================================
echo.

:: 查找并终止旧的服务进程
echo [1/3] 停止旧服务...
taskkill /f /im aiops-server.exe >nul 2>&1
if %errorlevel% == 0 (
    echo ✓ 已停止旧服务
    timeout /t 2 /nobreak >nul
) else (
    echo ! 未找到运行中的服务
)
echo.

:: 启动新服务
echo [2/3] 启动新服务...
start /b "" "%~dp0aiops-server.exe"
timeout /t 3 /nobreak >nul
echo ✓ 服务已启动
echo.

:: 检查服务状态
echo [3/3] 检查服务状态...
tasklist /fi "imagename eq aiops-server.exe" | findstr "aiops-server.exe" >nul
if %errorlevel% == 0 (
    echo ✓ 服务运行正常 (PID: %errorlevel%)
    echo.
    echo ==========================================
    echo 访问地址: http://localhost:8529
    echo ==========================================
) else (
    echo ✗ 服务启动失败，请检查日志
)
echo.
pause
