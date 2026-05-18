#!/usr/bin/env bash
# Removes .venv + caches + (optionally) Ollama model. Keeps scenarios/baselines/auth-state.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOG_DIR="$REPO_ROOT/logs"
mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/uninstall-$(date +%Y%m%d-%H%M%S).log"
exec > >(tee -a "$LOG_FILE") 2>&1

bash "$(dirname "${BASH_SOURCE[0]}")/stop.sh" || true

echo "[info] removing .venv..."
rm -rf "$REPO_ROOT/.venv"

echo "[info] removing __pycache__..."
find "$REPO_ROOT" -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true

echo "[info] removing Playwright browser cache..."
rm -rf "$HOME/.cache/ms-playwright" || true

if [[ "${1:-}" == "--remove-model" ]]; then
    ollama rm "ahmadwaqar/smolvlm2-2.2b-instruct" || true
fi

echo "[ok] Uninstall complete. Scenarios, baselines, and auth-state are preserved."
