# setup-tunnel.ps1 — Expose your laptop PostgreSQL + goDB to the cloud
# Requires: ngrok (https://ngrok.com/download)
#
# Usage:
#   1. Set your CLOUD_ENDPOINT to wherever your cloud VPS runs
#   2. Run: powershell -File setup-tunnel.ps1
#
# This script starts ngrok tunnels for both PostgreSQL and the goDB API,
# then launches the laptop agent which registers with the cloud.

param(
    [string]$CloudEndpoint = $(Read-Host "Cloud endpoint URL (e.g. https://api.yoursite.com)"),
    [string]$NgrokToken = $(Read-Host "ngrok auth token (get from https://dashboard.ngrok.com)")
)

$ErrorActionPreference = "Stop"

Write-Host "=== goDB Laptop Agent Setup ===" -ForegroundColor Cyan
Write-Host "Cloud endpoint: $CloudEndpoint"
Write-Host ""

# 1. Authenticate ngrok
if ($NgrokToken) {
    ngrok config add-authtoken $NgrokToken
}

# 2. Start ngrok tunnels
Write-Host "Starting ngrok tunnels..." -ForegroundColor Yellow
$postgresPort = if ($env:PGPORT) { $env:PGPORT } else { "5432" }
$apiPort = if ($env:PORT) { $env:PORT } else { "8080" }

# Tunnel for PostgreSQL (so cloud's provisioned databases can connect)
Start-Process -NoNewWindow -FilePath "ngrok" -ArgumentList "tcp $postgresPort --log=stdout > ngrok-postgres.log"

# Tunnel for goDB API (so cloud can forward requests)
Start-Process -NoNewWindow -FilePath "ngrok" -ArgumentList "http $apiPort --log=stdout > ngrok-api.log"

Write-Host "Waiting for tunnels..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# Get the public URLs from ngrok
try {
    $ngrokApi = Invoke-RestMethod -Uri "http://127.0.0.1:4040/api/tunnels" -ErrorAction Stop
    $apiUrl = ($ngrokApi.tunnels | Where-Object { $_.proto -eq "https" -and $_.config.addr -match ":$apiPort" }).public_url
    $pgUrl = ($ngrokApi.tunnels | Where-Object { $_.proto -eq "tcp" -and $_.config.addr -match ":$postgresPort" }).public_url
    
    if (-not $apiUrl) {
        Write-Host "WARNING: Could not get API tunnel URL" -ForegroundColor Red
        $apiUrl = "(check ngrok dashboard at http://127.0.0.1:4040)"
    }
    if (-not $pgUrl) {
        Write-Host "WARNING: Could not get PostgreSQL tunnel URL" -ForegroundColor Red
        $pgUrl = "(check ngrok dashboard)"
    }
} catch {
    Write-Host "WARNING: ngrok API not available. Check tunnels manually at http://127.0.0.1:4040" -ForegroundColor Red
    $apiUrl = "(see ngrok dashboard)"
    $pgUrl = "(see ngrok dashboard)"
}

Write-Host ""
Write-Host "=== Tunnel URLs ===" -ForegroundColor Green
Write-Host "API endpoint:      $apiUrl"
Write-Host "PostgreSQL:        $pgUrl"
Write-Host ""

# 3. Set env vars and launch the laptop agent
$env:CLOUD_ENDPOINT = $CloudEndpoint
$env:NODE_NAME = $env:COMPUTERNAME
$env:NODE_TOKEN = "node-secret-$(Get-Random -Minimum 100000 -Maximum 999999)"
$env:NODE_ENDPOINT = $apiUrl

Write-Host "Starting goDB laptop agent..." -ForegroundColor Yellow
Write-Host "  NODE_NAME:      $env:NODE_NAME"
Write-Host "  NODE_ENDPOINT:  $env:NODE_ENDPOINT"
Write-Host "  CLOUD_ENDPOINT: $env:CLOUD_ENDPOINT"
Write-Host "  NODE_TOKEN:     $env:NODE_TOKEN (SHARE THIS WITH YOUR CLOUD SERVER)"
Write-Host ""

# 4. Display the registration token for the cloud server
Write-Host "=== IMPORTANT ===" -ForegroundColor Magenta
Write-Host "On your cloud server, set these environment variables:" -ForegroundColor White
Write-Host "  NODE_ENDPOINT=$apiUrl"
Write-Host "  NODE_TOKEN=$env:NODE_TOKEN"
Write-Host ""

# 5. Run the goDB binary
Write-Host "Launching goDB..." -ForegroundColor Green
go run .
