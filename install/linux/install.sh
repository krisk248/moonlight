#!/usr/bin/env bash
# Moonlight — Linux installer.
# Run from the repo root:
#   ./install/linux/install.sh
# Every command is mirrored to logs/install-<timestamp>.log via `tee`.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOG_DIR="$REPO_ROOT/logs"
mkdir -p "$LOG_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
LOG_FILE="$LOG_DIR/install-$STAMP.log"
MODEL="ahmadwaqar/smolvlm2-2.2b-instruct"

# Redirect stdout+stderr to both terminal and the log file
exec > >(tee -a "$LOG_FILE") 2>&1

C_RESET='\033[0m'; C_CYAN='\033[1;36m'; C_GREEN='\033[1;32m'
C_YELLOW='\033[1;33m'; C_RED='\033[1;31m'

section() { echo -e "\n${C_CYAN}=== $* ===${C_RESET}"; }
info()    { echo -e "[info]  $*"; }
ok()      { echo -e "${C_GREEN}[ok]    $*${C_RESET}"; }
warn()    { echo -e "${C_YELLOW}[warn]  $*${C_RESET}"; }
fail()    { echo -e "${C_RED}[fail]  $*${C_RESET}"; }

need() { command -v "$1" >/dev/null 2>&1; }

trap 'fail "Installer aborted. Log: $LOG_FILE"' ERR

section "Moonlight installer"
info "Repo: $REPO_ROOT"
info "Log:  $LOG_FILE"
info "Shell: $SHELL"

# 1. Detect package manager
section "1/7  Detecting package manager"
if   need dnf;     then PKG="dnf"
elif need apt-get; then PKG="apt"
elif need pacman;  then PKG="pacman"
elif need zypper;  then PKG="zypper"
else
    fail "No supported package manager (dnf/apt/pacman/zypper). Install Python 3.12 manually then re-run."
    exit 1
fi
ok "Using $PKG"

install_pkg() {
    case "$PKG" in
        dnf)    sudo dnf install -y "$@" ;;
        apt)    sudo apt-get update -y && sudo apt-get install -y "$@" ;;
        pacman) sudo pacman -S --noconfirm "$@" ;;
        zypper) sudo zypper install -y "$@" ;;
    esac
}

# 2. Python 3.12+
section "2/7  Python 3.12"
if need python3 && python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3,12) else 1)'; then
    ok "$(python3 --version) — OK"
else
    info "Installing python3.12..."
    case "$PKG" in
        dnf)    install_pkg python3.12 python3.12-pip ;;
        apt)    install_pkg python3.12 python3.12-venv python3-pip || install_pkg python3 python3-venv python3-pip ;;
        pacman) install_pkg python ;;
        zypper) install_pkg python312 python312-pip ;;
    esac
fi

# 3. uv
section "3/7  uv"
if need uv; then
    ok "uv already installed ($(uv --version))"
else
    info "Installing uv via official script..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    # Source the new env so uv is on PATH for the rest of this script
    export PATH="$HOME/.local/bin:$PATH"
fi

# 4. Git (for completeness)
section "4/7  Git"
if need git; then
    ok "$(git --version)"
else
    install_pkg git
fi

# 5. Ollama
section "5/7  Ollama"
if need ollama; then
    ok "ollama already installed ($(ollama --version 2>&1 | head -1))"
else
    info "Installing Ollama (official script)..."
    if ! curl -fsSL https://ollama.com/install.sh | sh; then
        warn "Official script failed (sudo issue?). Falling back to user-space binary..."
        TMPDIR="$(mktemp -d)"
        ARCH="$(uname -m)"
        case "$ARCH" in
            x86_64) ASSET="ollama-linux-amd64.tar.zst" ;;
            aarch64|arm64) ASSET="ollama-linux-arm64.tar.zst" ;;
            *) fail "Unsupported arch: $ARCH"; exit 1 ;;
        esac
        REL="$(curl -sL https://api.github.com/repos/ollama/ollama/releases/latest | grep -oP '"tag_name":\s*"\K[^"]+' | head -1)"
        info "Downloading ollama $REL ($ASSET) ..."
        curl -fL --progress-bar -o "$TMPDIR/$ASSET" "https://github.com/ollama/ollama/releases/download/$REL/$ASSET"
        mkdir -p "$HOME/.local"
        zstd -d -c "$TMPDIR/$ASSET" | tar -xf - -C "$HOME/.local/"
        export PATH="$HOME/.local/bin:$PATH"
        ok "ollama placed at $HOME/.local/bin/ollama"
    fi
fi

# Start ollama service (best effort) and pull the model
info "Ensuring Ollama server is up..."
if ! curl -sf -m 2 http://127.0.0.1:11434/api/version >/dev/null; then
    nohup ollama serve > "$LOG_DIR/ollama-serve.log" 2>&1 &
    disown || true
    for i in $(seq 1 20); do
        if curl -sf -m 2 http://127.0.0.1:11434/api/version >/dev/null; then break; fi
        sleep 1
    done
fi
curl -sf -m 2 http://127.0.0.1:11434/api/version >/dev/null && ok "Ollama API reachable" || warn "Ollama API still not reachable — proceeding anyway"

section "5.5/7  Pulling SmolVLM2 model (~2.5 GB)"
ollama pull "$MODEL"

# 6. Python deps
section "6/7  Python dependencies (uv sync)"
cd "$REPO_ROOT"
uv sync

# 7. Playwright Chromium
section "7/7  Playwright Chromium"
uv run playwright install chromium

# Smoke
section "Smoke test"
if curl -sf -m 3 http://127.0.0.1:11434/api/tags | grep -q "smolvlm2"; then
    ok "Ollama + SmolVLM2 model verified."
else
    warn "Could not verify model via API. Try: ollama list"
fi

section "Done"
ok "Moonlight installed."
echo
echo "Next: start the dashboard with:"
echo -e "  ${C_YELLOW}./install/linux/start.sh${C_RESET}"
echo
echo "If anything went wrong, share this log with the team:"
echo -e "  ${C_YELLOW}$LOG_FILE${C_RESET}"
