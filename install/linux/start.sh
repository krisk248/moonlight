#!/usr/bin/env bash
# Moonlight — start the dashboard on Linux.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOG_DIR="$REPO_ROOT/logs"
mkdir -p "$LOG_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
LOG_FILE="$LOG_DIR/start-$STAMP.log"
RUNTIME_LOG="$LOG_DIR/runtime-$STAMP.log"
PID_FILE="$LOG_DIR/moonlight.pid"
PORT="${MOONLIGHT_PORT:-8765}"

exec > >(tee -a "$LOG_FILE") 2>&1

info() { echo "[info]  $*"; }
ok()   { echo "[ok]    $*"; }
warn() { echo "[warn]  $*"; }
fail() { echo "[fail]  $*"; }

info "Repo:        $REPO_ROOT"
info "Script log:  $LOG_FILE"
info "Runtime log: $RUNTIME_LOG"

# 1. Ensure Ollama
if ! curl -sf -m 2 http://127.0.0.1:11434/api/version >/dev/null; then
    info "Starting Ollama..."
    if ! command -v ollama >/dev/null; then
        fail "ollama not found. Re-run ./install/linux/install.sh first."
        exit 1
    fi
    nohup ollama serve > "$LOG_DIR/ollama-serve.log" 2>&1 &
    disown || true
    for i in $(seq 1 20); do
        curl -sf -m 2 http://127.0.0.1:11434/api/version >/dev/null && break
        sleep 1
    done
fi
curl -sf -m 2 http://127.0.0.1:11434/api/version >/dev/null && ok "Ollama up." || { fail "Ollama still down. See $LOG_DIR/ollama-serve.log"; exit 1; }

# 2. Launch dashboard
cd "$REPO_ROOT"
nohup uv run python app.py > "$RUNTIME_LOG" 2>&1 &
APP_PID=$!
disown || true
echo "$APP_PID" > "$PID_FILE"

# 3. Wait for it to bind
info "Waiting for dashboard..."
for i in $(seq 1 30); do
    curl -sf -m 2 "http://127.0.0.1:$PORT/api/status" >/dev/null && break
    sleep 1
done
if ! curl -sf -m 2 "http://127.0.0.1:$PORT/api/status" >/dev/null; then
    fail "Dashboard didn't start. See $RUNTIME_LOG"
    exit 1
fi
ok "Dashboard up at http://127.0.0.1:$PORT (PID $APP_PID)"

# 4. Open browser (best effort)
if command -v xdg-open >/dev/null; then
    xdg-open "http://127.0.0.1:$PORT" >/dev/null 2>&1 || true
fi

cat <<EOF

Dashboard running. To stop it:
  ./install/linux/stop.sh

Logs:
  Script:  $LOG_FILE
  Runtime: $RUNTIME_LOG
EOF
