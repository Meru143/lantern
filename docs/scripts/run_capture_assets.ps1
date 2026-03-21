# Generate README screenshots and demo GIFs from a local lantern server.

param(
    [string]$DBPath = "",
    [string]$OutDir = "",
    [int]$UIPort = 40461,
    [int]$OTLPGRPCPort = 43171,
    [int]$OTLPHTTPPort = 43181
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\\..")).Path
$serverHelper = Join-Path $repoRoot ".agents\\skills\\webapp-testing\\scripts\\with_server.py"
$captureScript = Join-Path $repoRoot "docs\\scripts\\capture_assets.py"

if ([string]::IsNullOrWhiteSpace($OutDir)) {
    $OutDir = Join-Path $repoRoot "docs\\images"
}

if ([string]::IsNullOrWhiteSpace($DBPath)) {
    $tempDir = Join-Path $env:TEMP "lantern-docs-assets"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
    $timestamp = Get-Date -Format "yyyyMMddHHmmssfff"
    $DBPath = Join-Path $tempDir "lantern-docs-$timestamp.db"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
if (Test-Path $DBPath) {
    Remove-Item $DBPath -Force
}

$serverCommand = "go run ./cmd/lantern serve --no-tui --db `"$DBPath`" --ui-port $UIPort --otlp-grpc $OTLPGRPCPort --otlp-http $OTLPHTTPPort"
$url = "http://127.0.0.1:$UIPort"

Push-Location $repoRoot
try {
    python $serverHelper --server $serverCommand --port $UIPort -- python $captureScript --db $DBPath --url $url --out $OutDir
} finally {
    Pop-Location
}
