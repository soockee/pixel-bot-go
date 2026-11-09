param(
    [string]$SecretName = 'anthropic_api_key',
    [string]$Model = 'claude-sonnet-4-6'
)

# Create the secret once with: podman secret create anthropic_api_key .\anthropic_api_key.txt
$ErrorActionPreference = 'Stop'
$image = 'pixel-bot-go/claude-api-test:local'
$scriptDir = $PSScriptRoot

podman build --tag $image --file (Join-Path $scriptDir 'Containerfile') $scriptDir
if ($LASTEXITCODE -ne 0) { throw 'Podman image build failed.' }

podman run --rm --env "CLAUDE_MODEL=$Model" --secret "$SecretName,type=mount,target=anthropic_api_key" $image
if ($LASTEXITCODE -ne 0) { throw 'Claude API test failed.' }
