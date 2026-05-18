# Moonlight — Windows 11 installer
# Run from the repo root in PowerShell 7+ (right-click → Run with PowerShell):
#   .\install\windows\install.ps1
# Every command and its output is captured to logs\install-<timestamp>.log
# so if anything breaks the tester can send you the log file.

[CmdletBinding()]
param(
    [switch]$SkipOllama,
    [switch]$SkipModel
)

$ErrorActionPreference = "Stop"
$ProgressPreference    = "SilentlyContinue"   # speeds up Invoke-WebRequest

# ---------- paths ----------------------------------------------------------
$RepoRoot = (Resolve-Path "$PSScriptRoot\..\..").Path
$LogDir   = Join-Path $RepoRoot "logs"
$Stamp    = Get-Date -Format "yyyyMMdd-HHmmss"
$LogFile  = Join-Path $LogDir "install-$Stamp.log"
$ModelName = "ahmadwaqar/smolvlm2-2.2b-instruct"

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null

function Section($msg) {
    Write-Host ""
    Write-Host "=============================================================" -ForegroundColor Cyan
    Write-Host "  $msg" -ForegroundColor Cyan
    Write-Host "=============================================================" -ForegroundColor Cyan
}
function Info($msg)  { Write-Host "[info]  $msg" -ForegroundColor White }
function Ok($msg)    { Write-Host "[ok]    $msg" -ForegroundColor Green }
function Warn($msg)  { Write-Host "[warn]  $msg" -ForegroundColor Yellow }
function Fail($msg)  { Write-Host "[fail]  $msg" -ForegroundColor Red }

function Need-Command($name) {
    return [bool](Get-Command $name -ErrorAction SilentlyContinue)
}

function Run-WithLog($label, [scriptblock]$action) {
    Info "$label ..."
    try {
        & $action
        Ok "$label — done"
    } catch {
        Fail "$label — FAILED: $($_.Exception.Message)"
        throw
    }
}

# ---------- main -----------------------------------------------------------
try {
    Section "Moonlight installer"
    Info "Repo root: $RepoRoot"
    Info "Log file:  $LogFile"
    Info "PowerShell: $($PSVersionTable.PSVersion)"

    # 1. Windows 11 check (build number 22000+)
    Section "1/8  Verifying Windows 11"
    $build = [int](Get-CimInstance Win32_OperatingSystem).BuildNumber
    Info "OS build: $build"
    if ($build -lt 22000) {
        Fail "This installer requires Windows 11 (build 22000+). Detected: $build"
        Fail "Use Windows 11 or run the Linux installer in WSL2."
        throw "Unsupported Windows version."
    }
    Ok "Windows 11 detected."

    # 2. winget — required to install everything else
    Section "2/8  Checking winget"
    if (-not (Need-Command "winget")) {
        Fail "winget not found. Install 'App Installer' from the Microsoft Store first, then re-run."
        throw "winget missing"
    }
    Ok "winget available."

    # 3. Git
    Section "3/8  Installing Git"
    if (Need-Command "git") {
        Ok "git already installed ($(git --version))"
    } else {
        Run-WithLog "winget install Git" { winget install --id Git.Git -e --silent --accept-package-agreements --accept-source-agreements }
    }

    # 4. Python 3.12
    Section "4/8  Installing Python 3.12"
    $hasPython312 = $false
    if (Need-Command "python") {
        $pyVer = (python --version 2>&1).Trim()
        Info "Found: $pyVer"
        if ($pyVer -match "3\.(1[2-9]|[2-9]\d)") { $hasPython312 = $true }
    }
    if (-not $hasPython312) {
        Run-WithLog "winget install Python 3.12" { winget install --id Python.Python.3.12 -e --silent --accept-package-agreements --accept-source-agreements }
        # Refresh PATH for current session so subsequent steps see python
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    } else {
        Ok "Python 3.12+ already present."
    }

    # 5. uv (Python project manager)
    Section "5/8  Installing uv"
    if (Need-Command "uv") {
        Ok "uv already installed ($(uv --version))"
    } else {
        Run-WithLog "winget install uv" { winget install --id astral-sh.uv -e --silent --accept-package-agreements --accept-source-agreements }
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    }

    # 6. Ollama
    Section "6/8  Installing Ollama"
    if ($SkipOllama) {
        Warn "Skipping Ollama install (-SkipOllama)."
    } elseif (Need-Command "ollama") {
        Ok "ollama already installed ($(ollama --version 2>&1 | Select-Object -First 1))"
    } else {
        Run-WithLog "winget install Ollama" { winget install --id Ollama.Ollama -e --silent --accept-package-agreements --accept-source-agreements }
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    }

    # 6b. Pull the model (Ollama service must be running first; the Windows installer auto-starts it)
    if (-not $SkipModel) {
        Section "6.5/8  Pulling SmolVLM2 model (~2.5 GB)"
        # Wait for Ollama API to come up (the installer runs it as a service)
        $apiUp = $false
        for ($i = 1; $i -le 30; $i++) {
            try {
                $null = Invoke-WebRequest -Uri "http://127.0.0.1:11434/api/version" -UseBasicParsing -TimeoutSec 2
                $apiUp = $true; break
            } catch { Start-Sleep -Seconds 2 }
        }
        if (-not $apiUp) {
            Warn "Ollama API didn't respond within 60s. Starting it manually..."
            Start-Process "ollama" -ArgumentList "serve" -WindowStyle Hidden
            Start-Sleep -Seconds 5
        }
        Run-WithLog "ollama pull $ModelName" { & ollama pull $ModelName }
    } else {
        Warn "Skipping model pull (-SkipModel)."
    }

    # 7. Python dependencies via uv
    Section "7/8  Installing Python dependencies (uv sync)"
    Push-Location $RepoRoot
    try {
        Run-WithLog "uv sync" { & uv sync }
    } finally {
        Pop-Location
    }

    # 8. Playwright Chromium
    Section "8/8  Installing Playwright Chromium browser"
    Push-Location $RepoRoot
    try {
        Run-WithLog "uv run playwright install chromium" { & uv run playwright install chromium }
    } finally {
        Pop-Location
    }

    # ---------- smoke test ------------------------------------------------
    Section "Smoke test"
    try {
        $tags = Invoke-WebRequest -Uri "http://127.0.0.1:11434/api/tags" -UseBasicParsing -TimeoutSec 5 | Select-Object -ExpandProperty Content
        if ($tags -match [regex]::Escape("smolvlm2")) {
            Ok "Ollama is running and SmolVLM2 model is pulled."
        } else {
            Warn "Ollama is running but the model is not yet listed. It may finish indexing shortly."
        }
    } catch {
        Warn "Could not reach Ollama API on the smoke test. Run 'ollama serve' manually if needed."
    }

    Section "Done"
    Ok "Moonlight is installed."
    Write-Host ""
    Write-Host "Next steps:" -ForegroundColor White
    Write-Host "  1. Start the dashboard:" -ForegroundColor White
    Write-Host "     .\install\windows\start.ps1" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  2. The browser will open automatically at http://127.0.0.1:8765" -ForegroundColor White
    Write-Host ""
    Write-Host "If anything went wrong, send this log to the team:" -ForegroundColor White
    Write-Host "  $LogFile" -ForegroundColor Yellow
} catch {
    Fail "Installer aborted: $($_.Exception.Message)"
    Write-Host ""
    Write-Host "Send this log file to the team for help:" -ForegroundColor Red
    Write-Host "  $LogFile" -ForegroundColor Yellow
    exit 1
} finally {
    Stop-Transcript | Out-Null
}
