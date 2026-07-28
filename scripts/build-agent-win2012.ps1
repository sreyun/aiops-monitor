# build-agent-win2012.ps1 — Windows-friendly wrapper around the bash builder.
# Prefer Docker Desktop / WSL. Falls back to regenerating sources locally with Go 1.20.
param(
  [string]$Version = "",
  [string]$Out = ""
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
if (-not $Version) {
  $Version = (& git -C $Root describe --tags 2>$null)
  if (-not $Version) { $Version = "dev" }
}
if (-not $Out) {
  $Out = Join-Path $Root "dist\aiops-agent-windows-amd64-win2012.exe"
}

$bash = Get-Command bash -ErrorAction SilentlyContinue
$wsl = Get-Command wsl -ErrorAction SilentlyContinue
$env:VERSION = $Version
$env:OUT = $Out

if ($bash) {
  & bash (Join-Path $Root "scripts\build-agent-win2012.sh")
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  exit 0
}
if ($wsl) {
  $unixRoot = (& wsl wslpath -a $Root).Trim()
  $unixOut = (& wsl wslpath -a $Out).Trim()
  & wsl bash -lc "VERSION='$Version' OUT='$unixOut' '$unixRoot/scripts/build-agent-win2012.sh'"
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  exit 0
}

Write-Host "[win2012] bash/wsl not found — using Docker golang:1.20 directly" -ForegroundColor Yellow
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  Write-Error "Need bash, wsl, or docker to build the win2012 Agent"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("aiops-win2012-" + [guid]::NewGuid().ToString("n"))
New-Item -ItemType Directory -Force -Path (Join-Path $tmp "src\cmd\agent"), (Join-Path $tmp "src\shared"), (Split-Path $Out) | Out-Null
Copy-Item -Recurse -Force (Join-Path $Root "cmd\agent\*") (Join-Path $tmp "src\cmd\agent")
Copy-Item -Recurse -Force (Join-Path $Root "shared\*") (Join-Path $tmp "src\shared")
if (Test-Path (Join-Path $Root "config.example.yaml")) {
  Copy-Item (Join-Path $Root "config.example.yaml") (Join-Path $tmp "src\cmd\agent\config_example.yaml") -Force
}
Get-ChildItem -Recurse (Join-Path $tmp "src\cmd\agent") -Filter *.go | ForEach-Object {
  (Get-Content -Raw $_.FullName) -replace '"log/slog"', '"golang.org/x/exp/slog"' | Set-Content -NoNewline $_.FullName -Encoding UTF8
}
@"
module aiops-monitor

go 1.20

require (
	golang.org/x/exp v0.0.0-20231108232855-2478ac86d95e
	gopkg.in/yaml.v3 v3.0.1
)
"@ | Set-Content (Join-Path $tmp "src\go.mod") -Encoding UTF8

$outDir = Split-Path $Out
$outName = Split-Path $Out -Leaf
$ld = "-s -w -X main.appVersion=$Version"
docker run --rm `
  -v "${tmp}\src:/src" `
  -v "${outDir}:/out" `
  -w /src `
  golang:1.20-bullseye `
  bash -c "set -euo pipefail; go mod tidy; CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='$ld' -o /out/$outName ./cmd/agent"

Remove-Item -Recurse -Force $tmp
if (-not (Test-Path $Out)) { Write-Error "build failed: $Out missing" }
$hash = (Get-FileHash -Algorithm SHA256 $Out).Hash.ToLowerInvariant()
"$hash  $outName" | Set-Content ($Out + ".sha256") -Encoding ASCII
Write-Host "[win2012] ok: $Out" -ForegroundColor Green
Write-Host "[win2012] sha256: $hash"
