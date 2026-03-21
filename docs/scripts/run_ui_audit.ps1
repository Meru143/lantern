# Audit the local lantern web UI with squirrelscan.

param(
    [string]$DBPath = "",
    [int]$UIPort = 40462,
    [int]$OTLPGRPCPort = 43172,
    [int]$OTLPHTTPPort = 43182
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\\..")).Path
$serverHelper = Join-Path $repoRoot ".agents\\skills\\webapp-testing\\scripts\\with_server.py"
$squirrel = Join-Path $env:LOCALAPPDATA "squirrel\\bin\\squirrel.exe"

if (-not (Test-Path $squirrel)) {
    throw "squirrel executable not found at $squirrel"
}

if ([string]::IsNullOrWhiteSpace($DBPath)) {
    $tempDir = Join-Path $env:TEMP "lantern-ui-audit"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
    $timestamp = Get-Date -Format "yyyyMMddHHmmssfff"
    $DBPath = Join-Path $tempDir "lantern-audit-$timestamp.db"
}

$auditDir = Join-Path $env:TEMP "lantern-ui-audit-project"
New-Item -ItemType Directory -Force -Path $auditDir | Out-Null
if (Test-Path $DBPath) {
    Remove-Item $DBPath -Force
}

$serverCommand = "go run ./cmd/lantern serve --no-tui --db `"$DBPath`" --ui-port $UIPort --otlp-grpc $OTLPGRPCPort --otlp-http $OTLPHTTPPort"
$url = "http://127.0.0.1:$UIPort"

Push-Location $repoRoot
try {
    Push-Location $auditDir
    try {
        & $squirrel init -n lantern-local --force | Out-Null
    } finally {
        Pop-Location
    }

    $auditCommand = "pwsh -NoProfile -Command `"Set-Location '$auditDir'; & '$squirrel' audit $url --format llm -m 3`""
    python $serverHelper --server $serverCommand --port $UIPort -- powershell -NoProfile -Command "Set-Location '$auditDir'; & '$squirrel' audit '$url' --format llm -m 3"
} finally {
    Pop-Location
}
