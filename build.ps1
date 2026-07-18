# build.ps1 — 构建 AIOps Monitor 服务端和 Agent，自动注入 Git tag 版本号
# 用法:  powershell -File build.ps1
#       powershell -File build.ps1 -CrossCompile  (交叉编译 Linux/macOS)

param(
  [switch]$CrossCompile
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path

# 1. 获取最新 Git tag
$tag = & git -C $root describe --tags 2>$null
if (-not $tag) {
  $commit = & git -C $root rev-parse --short HEAD 2>$null
  $tag = if ($commit) { "dev-$commit" } else { "dev" }
}
Write-Host "构建版本: $tag" -ForegroundColor Cyan

# 2. 用 ldflags 注入版本号到 main.appVersion
#    -s -w 去符号表/调试信息，-trimpath 去构建路径：与 docker/Dockerfile 产线保持一致，
#    产出瘦身 ~30%(约 11MB→7.5MB)，杜绝本地胖包被误当 /dl 下发物。
$ldflags = "-s -w -X main.appVersion=$tag"

# 3. 构建服务端
Write-Host "构建 aiops-server ..." -ForegroundColor Yellow
& go build -trimpath -ldflags $ldflags -o "$root/bin/aiops-server.exe" "$root/cmd/server"
if ($LASTEXITCODE -ne 0) { Write-Host "服务端构建失败" -ForegroundColor Red; exit 1 }

# 4. 构建 Agent
Write-Host "构建 aiops-agent ..." -ForegroundColor Yellow
& go build -trimpath -ldflags $ldflags -o "$root/bin/aiops-agent.exe" "$root/cmd/agent"
if ($LASTEXITCODE -ne 0) { Write-Host "Agent 构建失败" -ForegroundColor Red; exit 1 }

Write-Host "构建完成: v$tag" -ForegroundColor Green
Write-Host "  bin/aiops-server.exe"
Write-Host "  bin/aiops-agent.exe"

# 5. 可选：交叉编译 Linux/macOS
if ($CrossCompile) {
  Write-Host "交叉编译 Linux/macOS ..." -ForegroundColor Yellow

  $env:GOOS = "linux"; $env:GOARCH = "amd64"
  & go build -trimpath -ldflags $ldflags -o "$root/bin/aiops-server-linux" "$root/cmd/server"
  & go build -trimpath -ldflags $ldflags -o "$root/bin/aiops-agent-linux" "$root/cmd/agent"

  $env:GOOS = "darwin"; $env:GOARCH = "amd64"
  & go build -trimpath -ldflags $ldflags -o "$root/bin/aiops-server-mac" "$root/cmd/server"
  & go build -trimpath -ldflags $ldflags -o "$root/bin/aiops-agent-mac" "$root/cmd/agent"

  $env:GOOS = ""; $env:GOARCH = ""
  Write-Host "交叉构建完成" -ForegroundColor Green
  Write-Host "  bin/aiops-server-linux"
  Write-Host "  bin/aiops-agent-linux"
  Write-Host "  bin/aiops-server-mac"
  Write-Host "  bin/aiops-agent-mac"
}
