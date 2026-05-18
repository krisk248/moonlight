#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PID_FILE="$REPO_ROOT/logs/moonlight.pid"

if [[ -f "$PID_FILE" ]]; then
    PID=$(cat "$PID_FILE")
    if kill "$PID" 2>/dev/null; then
        echo "[ok] stopped PID $PID"
    else
        echo "[warn] PID $PID not running"
    fi
    rm -f "$PID_FILE"
else
    pkill -f "uv run python app.py" 2>/dev/null && echo "[ok] killed app.py processes" || echo "[warn] nothing to stop"
fi
