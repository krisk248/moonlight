#!/usr/bin/env bash
# Moonlight cross-compile build (Linux + Windows amd64) with the kill-switch
# URL stamped in via ldflags.
#
# Usage:
#   MOONLIGHT_REVOCATION_URL="https://gist.githubusercontent.com/<user>/<id>/raw/revoked.json" \
#     ./install/build/build.sh
#
# Outputs:
#   dist/moonlight-linux-amd64-<date>/    folder, ready to zip
#   dist/moonlight-windows-amd64-<date>/  folder, ready to zip
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

C_CYAN='\033[1;36m'; C_GREEN='\033[1;32m'; C_YELLOW='\033[1;33m'; C_RESET='\033[0m'
log()  { echo -e "${C_CYAN}[build]${C_RESET} $*"; }
warn() { echo -e "${C_YELLOW}[warn]${C_RESET} $*"; }
ok()   { echo -e "${C_GREEN}[ok]${C_RESET} $*"; }

REVOCATION_URL="${MOONLIGHT_REVOCATION_URL:-}"
if [[ -z "$REVOCATION_URL" ]]; then
  warn "MOONLIGHT_REVOCATION_URL not set — kill switch will be INACTIVE in the built binary."
fi

BUILD_DATE="$(date -u +%Y-%m-%d)"
BUILD_ID="ml-$BUILD_DATE-$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
VERSION="0.1.0"
LD="-X github.com/krisk248/moonlight/internal/buildinfo.BuildDate=$BUILD_DATE"
LD="$LD -X github.com/krisk248/moonlight/internal/buildinfo.BuildID=$BUILD_ID"
LD="$LD -X github.com/krisk248/moonlight/internal/buildinfo.Version=$VERSION"
LD="$LD -X github.com/krisk248/moonlight/internal/buildinfo.RevocationURL=$REVOCATION_URL"
LD="$LD -s -w"   # strip debug info → smaller binary

log "Build ID: $BUILD_ID"
log "Revocation URL: ${REVOCATION_URL:-(none)}"

# Make sure the frontend is fresh — the Svelte build lands in cmd/moonlight/web/
if [[ -d frontend ]]; then
  log "Building Svelte frontend..."
  (cd frontend && pnpm install --frozen-lockfile 2>/dev/null || pnpm install)
  (cd frontend && pnpm build) > /tmp/moonlight-svelte-build.log 2>&1 || {
    warn "Frontend build failed — see /tmp/moonlight-svelte-build.log"
    exit 1
  }
  ok "Frontend built into cmd/moonlight/web/"
fi

# Tests must pass before we cut a release binary.
log "Running unit tests..."
go test ./... > /tmp/moonlight-test.log 2>&1 || {
  warn "Tests failed — see /tmp/moonlight-test.log"
  tail -20 /tmp/moonlight-test.log
  exit 1
}
ok "All tests pass"

build_one() {
  local goos="$1" goarch="$2" outbin="$3" launcher="$4" launcher_name="$5"
  local outdir="dist/moonlight-$goos-$goarch-$BUILD_DATE"
  rm -rf "$outdir"
  mkdir -p "$outdir"/{scenarios,baselines,auth-state,logs,runs}

  log "Compiling $goos/$goarch → $outdir/$outbin"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build \
    -ldflags "$LD" \
    -o "$outdir/$outbin" ./cmd/moonlight

  # Launcher that sets MOONLIGHT_HOME so the binary picks up the bundled dirs.
  printf '%s' "$launcher" > "$outdir/$launcher_name"
  chmod +x "$outdir/$launcher_name"

  cp config.yaml LICENSE OPERATIONS.md README.md "$outdir/" 2>/dev/null || true

  # Per-platform install note.
  cat > "$outdir/INSTALL.txt" <<EOF
Moonlight $VERSION
build:   $BUILD_ID
built:   $BUILD_DATE

First-time setup (one-time, ~170 MB Chromium download):
  $outbin install-browsers

Then start the dashboard:
  $launcher_name           (opens http://127.0.0.1:8765)

Recording new scenarios requires Node.js + npm (https://nodejs.org/).

Kill switch:
  This build $( [[ -n "$REVOCATION_URL" ]] && echo "polls $REVOCATION_URL" || echo "is a dev build with no kill switch.")
  on every startup. See OPERATIONS.md.
EOF

  ok "$outdir built ($(du -sh "$outdir" | cut -f1))"
}

# ---------- Linux build ----------
# Launcher defaults to `serve` if no subcommand is given.
LINUX_LAUNCHER='#!/usr/bin/env bash
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export MOONLIGHT_HOME="$HERE"
if [[ $# -eq 0 ]]; then
    exec "$HERE/moonlight" serve
else
    exec "$HERE/moonlight" "$@"
fi
'
build_one linux amd64 moonlight "$LINUX_LAUNCHER" "start.sh"

# ---------- Windows build (cross-compile from Linux) ----------
WINDOWS_LAUNCHER='@echo off
set MOONLIGHT_HOME=%~dp0
if "%~1"=="" (
    "%~dp0moonlight.exe" serve
) else (
    "%~dp0moonlight.exe" %*
)
'
build_one windows amd64 moonlight.exe "$WINDOWS_LAUNCHER" "start.bat"

log "All builds done."
log "Contents of dist/:"
ls -la dist/

log "To distribute, zip the folders:"
echo "  cd dist && zip -rq moonlight-linux-amd64-$BUILD_DATE.zip moonlight-linux-amd64-$BUILD_DATE"
echo "  cd dist && zip -rq moonlight-windows-amd64-$BUILD_DATE.zip moonlight-windows-amd64-$BUILD_DATE"
