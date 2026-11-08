# Builds the frontend and the app, then starts it. Arguments go to the app:
#   .\run.ps1                 normal run
#   .\run.ps1 -dry-run        log input instead of sending it
# Developer views appear with "debug": true in pixle_bot_config.json.
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true # a failed npm or go build stops the script

Push-Location $PSScriptRoot
try {
    if (Get-Process pixel-bot-go -ErrorAction SilentlyContinue) {
        throw 'pixel-bot-go.exe is running; close it first so it can be rebuilt'
    }
    Push-Location frontend
    try {
        if (-not (Test-Path node_modules)) { npm ci }
        npm run build
    } finally { Pop-Location }
    go build -ldflags '-H=windowsgui' -o pixel-bot-go.exe . # as released: no terminal window; runs with arguments use this one
    & .\pixel-bot-go.exe @args
} finally { Pop-Location }
