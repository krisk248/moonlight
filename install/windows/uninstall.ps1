# Moonlight — uninstall (keeps scenarios, baselines, auth-state).
# Removes: .venv, __pycache__, Playwright browser cache, optionally Ollama model.
[CmdletBinding()]
param(
    [switch]$RemoveOllamaModel,
    [switch]$RemoveOllama
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Resolve-Path "$PSScriptRoot\..\..").Path
$LogDir   = Join-Path $RepoRoot "logs"
$Stamp    = Get-Date -Format "yyyyMMdd-HHmmss"
$LogFile  = Join-Path $LogDir "uninstall-$Stamp.log"

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null

function Info($m) { Write-Host "[info]  $m" -ForegroundColor White }
function Ok($m)   { Write-Host "[ok]    $m" -ForegroundColor Green }
function Warn($m) { Write-Host "[warn]  $m" -ForegroundColor Yellow }

try {
    & "$PSScriptRoot\stop.ps1" 2>$null

    Info "Removing .venv..."
    $venv = Join-Path $RepoRoot ".venv"
    if (Test-Path $venv) { Remove-Item -Recurse -Force $venv; Ok ".venv removed." }

    Info "Removing __pycache__..."
    Get-ChildItem -Recurse -Force -Directory -Filter "__pycache__" -Path $RepoRoot |
        ForEach-Object { Remove-Item -Recurse -Force $_.FullName }

    Info "Removing Playwright browser cache..."
    $pw = Join-Path $env:USERPROFILE "AppData\Local\ms-playwright"
    if (Test-Path $pw) { Remove-Item -Recurse -Force $pw; Ok "Playwright cache removed." }

    if ($RemoveOllamaModel) {
        Info "Removing SmolVLM2 model..."
        & ollama rm "ahmadwaqar/smolvlm2-2.2b-instruct" 2>$null
        Ok "Model removed."
    }
    if ($RemoveOllama) {
        Info "Uninstalling Ollama via winget..."
        winget uninstall --id Ollama.Ollama -e --silent 2>$null
        Ok "Ollama removed."
    }

    Ok "Uninstall complete. Your scenarios/baselines/auth-state are kept."
    Write-Host "To remove them too, delete:"
    Write-Host "  $RepoRoot\scenarios"
    Write-Host "  $RepoRoot\baselines"
    Write-Host "  $RepoRoot\auth-state"
} finally {
    Stop-Transcript | Out-Null
}
