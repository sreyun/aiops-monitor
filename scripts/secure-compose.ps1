# secure-compose.ps1 - 在本地生成强随机密钥并写入 .env（Windows 版）
# 用法：
#   powershell -ExecutionPolicy Bypass -File scripts/secure-compose.ps1
#   # 或从仓库根目录直接：
#   powershell -File scripts/secure-compose.ps1
#
# 作用：
#   1. 若 .env 不存在，从 .env.example 复制（存在时）并生成密钥；
#   2. 若 .env 存在但 POSTGRES_PASSWORD / AIOPS_SECRET_KEY 为空，
#      自动补写强随机值（幂等，不覆盖已有值）；
#   3. docker-compose.yml 已改为 `${VAR:?}` 强制校验：缺密钥时直接启动失败，
#      必须先运行本脚本（或 Linux 的 scripts/secure-compose.sh）。

$ErrorActionPreference = "Stop"

function New-RandomString([int]$length, [string]$alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789") {
    $bytes = New-Object byte[] $length
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $sb = New-Object System.Text.StringBuilder
    foreach ($b in $bytes) {
        [void]$sb.Append($alphabet[$b % $alphabet.Length])
    }
    return $sb.ToString()
}

function New-SecretKey {
    return "aiops-" + (New-RandomString 44)
}

$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"
$envExample = Join-Path $root ".env.example"

if (-not (Test-Path -LiteralPath $envFile)) {
    if (Test-Path -LiteralPath $envExample) {
        Copy-Item -LiteralPath $envExample -Destination $envFile
        Write-Host "==> 已从 .env.example 复制生成 .env"
    } else {
        Set-Content -LiteralPath $envFile -Value @(
            "POSTGRES_PASSWORD=",
            "AIOPS_SECRET_KEY=",
            "AIOPS_CONTENT_AUDIT_INGEST_TOKEN=",
            "AIOPS_INSTALL_TOKEN=",
            "AIOPS_HTTP_PORT=8529",
            "AIOPS_FORWARD_PORT_RANGE=10100-10300",
            "TZ=Asia/Shanghai",
            "AIOPS_VM_URL=http://victoriametrics:8428"
        ) -Encoding UTF8
        Write-Host "==> 已创建最小 .env"
    }
}

$pgPassword = New-RandomString 20
$secretKey = New-SecretKey
$installToken = (New-RandomString 32 "abcdef0123456789")

$lines = [System.Collections.Generic.List[string]]::new([System.IO.File]::ReadAllLines($envFile))
$seenPg = $false
$seenKey = $false
$seenToken = $false
for ($i = 0; $i -lt $lines.Count; $i++) {
    $line = $lines[$i]
    if ($line -match '^POSTGRES_PASSWORD=') {
        $seenPg = $true
        if ([string]::IsNullOrWhiteSpace($line.Substring("POSTGRES_PASSWORD=".Length))) {
            $lines[$i] = "POSTGRES_PASSWORD=" + $pgPassword
        }
    } elseif ($line -match '^AIOPS_SECRET_KEY=') {
        $seenKey = $true
        if ([string]::IsNullOrWhiteSpace($line.Substring("AIOPS_SECRET_KEY=".Length))) {
            $lines[$i] = "AIOPS_SECRET_KEY=" + $secretKey
        }
    } elseif ($line -match '^AIOPS_INSTALL_TOKEN=') {
        $seenToken = $true
        if ([string]::IsNullOrWhiteSpace($line.Substring("AIOPS_INSTALL_TOKEN=".Length))) {
            $lines[$i] = "AIOPS_INSTALL_TOKEN=" + $installToken
        }
    }
}
if (-not $seenPg) { $lines.Add("POSTGRES_PASSWORD=" + $pgPassword) }
if (-not $seenKey) { $lines.Add("AIOPS_SECRET_KEY=" + $secretKey) }
if (-not $seenToken) { $lines.Add("AIOPS_INSTALL_TOKEN=" + $installToken) }

[System.IO.File]::WriteAllLines($envFile, $lines, (New-Object System.Text.UTF8Encoding($false)))
Write-Host ""
Write-Host "==> 完成！密钥已写入 $envFile （勿提交到 Git）"
Write-Host "    POSTGRES_PASSWORD / AIOPS_SECRET_KEY / AIOPS_INSTALL_TOKEN 已确保为强随机值"
Write-Host "    下一步：docker compose up -d"
