# build-agent-win2012.ps1 — Windows-friendly wrapper around the bash builder.
# Prefer a real bash. Falls back to Docker golang:1.20 (rewrite runs inside the container).
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

$env:VERSION = $Version
$env:OUT = $Out

# Avoid broken Windows "bash" stubs that just relay to missing WSL distros.
function Test-UsableBash {
  param([string]$Path)
  if (-not $Path) { return $false }
  try {
    $p = Start-Process -FilePath $Path -ArgumentList "-c","echo ok" -Wait -PassThru -WindowStyle Hidden -RedirectStandardOutput "$env:TEMP\aiops-bash-ok.txt" -RedirectStandardError "$env:TEMP\aiops-bash-err.txt"
    return ($p.ExitCode -eq 0)
  } catch {
    return $false
  }
}

$bashCmd = Get-Command bash -ErrorAction SilentlyContinue
if ($bashCmd -and (Test-UsableBash $bashCmd.Source)) {
  & $bashCmd.Source (Join-Path $Root "scripts\build-agent-win2012.sh")
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  exit 0
}

$wsl = Get-Command wsl -ErrorAction SilentlyContinue
if ($wsl) {
  $prevEap = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  $unixRoot = & wsl wslpath -a $Root 2>$null
  $wslOk = ($LASTEXITCODE -eq 0 -and $unixRoot)
  $ErrorActionPreference = $prevEap
  if ($wslOk) {
    $unixRoot = ($unixRoot | Out-String).Trim()
    $unixOut = (& wsl wslpath -a $Out 2>$null | Out-String).Trim()
    & wsl bash -lc "VERSION='$Version' OUT='$unixOut' '$unixRoot/scripts/build-agent-win2012.sh'"
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    exit 0
  }
}

Write-Host "[win2012] usable bash/wsl not found — using Docker golang:1.20" -ForegroundColor Yellow
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
Copy-Item (Join-Path $Root "scripts\win2012\go.mod") (Join-Path $tmp "src\go.mod") -Force
Copy-Item (Join-Path $Root "scripts\win2012\go.sum") (Join-Path $tmp "src\go.sum") -Force
Copy-Item (Join-Path $Root "scripts\win2012\rewrite_slog.sh") (Join-Path $tmp "src\rewrite_slog.sh") -Force

$outDir = Split-Path $Out
$outName = Split-Path $Out -Leaf
$ld = "-s -w -X main.appVersion=$Version"
$proxy = if ($env:GOPROXY) { $env:GOPROXY } else { "https://proxy.golang.org,https://goproxy.cn,direct" }
$srcDocker = (Resolve-Path (Join-Path $tmp "src")).Path
$outDocker = (Resolve-Path $outDir).Path

docker run --rm `
  -e "GOPROXY=$proxy" `
  -v "${srcDocker}:/src" `
  -v "${outDocker}:/out" `
  -w /src `
  golang:1.20-bullseye `
  bash -c "set -euo pipefail; bash /src/rewrite_slog.sh /src/cmd/agent; go mod download; CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='$ld' -o /out/$outName ./cmd/agent"

Remove-Item -Recurse -Force $tmp
if (-not (Test-Path $Out)) { Write-Error "build failed: $Out missing" }
$hash = (Get-FileHash -Algorithm SHA256 $Out).Hash.ToLowerInvariant()
"$hash  $outName" | Set-Content ($Out + ".sha256") -Encoding ASCII
Write-Host "[win2012] ok: $Out" -ForegroundColor Green
Write-Host "[win2012] sha256: $hash"
