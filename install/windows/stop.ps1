# Moonlight — stop the dashboard.
[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$RepoRoot = (Resolve-Path "$PSScriptRoot\..\..").Path
$PidFile  = Join-Path $RepoRoot "logs\moonlight.pid"

function Info($m) { Write-Host "[info]  $m" -ForegroundColor White }
function Ok($m)   { Write-Host "[ok]    $m" -ForegroundColor Green }
function Warn($m) { Write-Host "[warn]  $m" -ForegroundColor Yellow }

if (Test-Path $PidFile) {
    $appPid = (Get-Content $PidFile | Select-Object -First 1).Trim()
    Info "Stopping Moonlight (PID $appPid)..."
    try {
        Stop-Process -Id $appPid -Force -ErrorAction Stop
        Ok "Stopped."
    } catch {
        Warn "PID $appPid not running (already stopped?)."
    }
    Remove-Item $PidFile -Force
} else {
    Warn "No PID file at $PidFile — trying to find the process by name..."
    $procs = Get-Process -Name "python*" -ErrorAction SilentlyContinue | Where-Object {
        $_.CommandLine -match "app.py"
    }
    foreach ($p in $procs) {
        Info "Stopping PID $($p.Id)..."
        Stop-Process -Id $p.Id -Force
    }
    Ok "Done."
}
