# Moonlight — start the dashboard on Windows 11.
# Run from the repo root:
#   .\install\windows\start.ps1
[CmdletBinding()]
param(
    [int]$Port = 8765,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"
$RepoRoot = (Resolve-Path "$PSScriptRoot\..\..").Path
$LogDir   = Join-Path $RepoRoot "logs"
$Stamp    = Get-Date -Format "yyyyMMdd-HHmmss"
$LogFile  = Join-Path $LogDir "start-$Stamp.log"
$RuntimeLog = Join-Path $LogDir "runtime-$Stamp.log"

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null

function Info($m) { Write-Host "[info]  $m" -ForegroundColor White }
function Ok($m)   { Write-Host "[ok]    $m" -ForegroundColor Green }
function Warn($m) { Write-Host "[warn]  $m" -ForegroundColor Yellow }
function Fail($m) { Write-Host "[fail]  $m" -ForegroundColor Red }

try {
    Info "Repo: $RepoRoot"
    Info "Logs: $LogFile (this script) + $RuntimeLog (app stdout/stderr)"

    # 1. Ensure Ollama is running
    Info "Checking Ollama..."
    $ollamaUp = $false
    try {
        $null = Invoke-WebRequest -Uri "http://127.0.0.1:11434/api/version" -UseBasicParsing -TimeoutSec 2
        $ollamaUp = $true
    } catch { }
    if (-not $ollamaUp) {
        Info "Ollama not responding — starting it in the background..."
        if (-not (Get-Command "ollama" -ErrorAction SilentlyContinue)) {
            Fail "ollama not found in PATH. Re-run install.ps1 first."
            throw "ollama missing"
        }
        Start-Process "ollama" -ArgumentList "serve" -WindowStyle Hidden
        for ($i = 1; $i -le 20; $i++) {
            try {
                $null = Invoke-WebRequest -Uri "http://127.0.0.1:11434/api/version" -UseBasicParsing -TimeoutSec 2
                $ollamaUp = $true; break
            } catch { Start-Sleep -Seconds 1 }
        }
    }
    if (-not $ollamaUp) {
        Fail "Ollama wouldn't start. Try running 'ollama serve' manually in another window."
        throw "ollama not reachable"
    }
    Ok "Ollama is up at http://127.0.0.1:11434"

    # 2. Launch the dashboard
    Info "Launching Moonlight dashboard..."
    Push-Location $RepoRoot
    try {
        # uv run keeps the right venv; redirect stdout+stderr to the runtime log.
        $appProcess = Start-Process -FilePath "uv" -ArgumentList "run","python","app.py" `
            -RedirectStandardOutput $RuntimeLog `
            -RedirectStandardError  "$RuntimeLog.err" `
            -PassThru -WindowStyle Hidden

        # 3. Wait for the dashboard to bind
        $dashUp = $false
        for ($i = 1; $i -le 30; $i++) {
            try {
                $null = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/api/status" -UseBasicParsing -TimeoutSec 2
                $dashUp = $true; break
            } catch { Start-Sleep -Seconds 1 }
        }
        if (-not $dashUp) {
            Fail "Dashboard didn't come up within 30s. See: $RuntimeLog"
            throw "dashboard not reachable"
        }
        Ok "Dashboard is up at http://127.0.0.1:$Port (PID $($appProcess.Id))"

        # Persist PID so stop.ps1 can find it
        $appProcess.Id | Out-File -FilePath (Join-Path $LogDir "moonlight.pid") -Encoding ASCII

        # 4. Open browser
        if (-not $NoBrowser) {
            Start-Process "http://127.0.0.1:$Port"
            Ok "Opened browser."
        }
    } finally {
        Pop-Location
    }

    Write-Host ""
    Write-Host "Dashboard running. To stop it:" -ForegroundColor White
    Write-Host "  .\install\windows\stop.ps1" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Logs:" -ForegroundColor White
    Write-Host "  Script log:  $LogFile" -ForegroundColor Gray
    Write-Host "  Runtime log: $RuntimeLog" -ForegroundColor Gray
} catch {
    Fail "Start aborted: $($_.Exception.Message)"
    Write-Host "Send this log to the team if you need help: $LogFile" -ForegroundColor Yellow
    exit 1
} finally {
    Stop-Transcript | Out-Null
}
