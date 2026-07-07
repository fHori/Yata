<#
.SYNOPSIS
  Build (and optionally run) Yata on Windows.

.EXAMPLE
  .\build.ps1            # build frontend + backend -> yata.exe
  .\build.ps1 -Run       # build then start on the configured port (default 8420)
  .\build.ps1 -SkipWeb   # rebuild only the Go binary
#>
param(
    [switch]$Run,
    [switch]$SkipWeb
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot

if (-not $SkipWeb) {
    Write-Host "==> Building frontend (web/ -> static/dashboard.js)" -ForegroundColor Cyan
    Push-Location (Join-Path $root 'web')
    try {
        if (-not (Test-Path 'node_modules')) { npm install }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "frontend build failed" }
    } finally { Pop-Location }
}

Write-Host "==> Building backend (cmd/yata -> yata.exe)" -ForegroundColor Cyan
Push-Location $root
try {
    go build -o yata.exe ./cmd/yata
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
} finally { Pop-Location }

Write-Host "==> Build complete: yata.exe" -ForegroundColor Green

if ($Run) {
    Write-Host "==> Starting Yata (Ctrl+C to stop)" -ForegroundColor Cyan
    & (Join-Path $root 'yata.exe')
}
