"""Centralized lifecycle enforcement.

On every entry point this module:
  1. Refuses to start if a sentinel file exists (previously disabled).
  2. Fetches the revocation JSON from a private GitHub repo via the API.
  3. If this install is on the revoked list → wipes sensitive data and exits.
  4. If the URL is unreachable, falls back to a cached response (<24h).
  5. If the URL has been unreachable AND no recent cache for >28 days → wipes
     and exits silently. This is the deadman backstop.

All user-facing messages are deliberately generic; no dates, no mechanism,
no policy details surface to the operator. See OPERATIONS.md for the pointer
to internal documentation.
"""
from __future__ import annotations

import base64
import json
import os
import shutil
import sys
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import httpx

from . import _buildinfo


SENTINEL_FILE = ".killed"
STATE_FILE = ".moonlight-state.json"
GENERIC_MSG = "This binary is no longer authorized. Contact your administrator."

CACHE_TTL_HOURS = 24       # how long a successful response is trusted
NO_CONTACT_DAYS = 28       # silent kill after this many days with no fresh data


# ---------- environment ----------------------------------------------------

def _home() -> Path:
    return Path(os.environ.get("MOONLIGHT_HOME", Path.cwd()))


def _is_source_build() -> bool:
    return _buildinfo.BUILD_DATE == "SOURCE_BUILD"


def _now() -> datetime:
    return datetime.now(timezone.utc)


# ---------- state file -----------------------------------------------------

def _load_state() -> dict[str, Any]:
    p = _home() / STATE_FILE
    if not p.exists():
        return {}
    try:
        return json.loads(p.read_text())
    except Exception:
        return {}


def _save_state(state: dict[str, Any]) -> None:
    try:
        (_home() / STATE_FILE).write_text(json.dumps(state, separators=(",", ":")))
    except Exception:
        pass


def _install_id(state: dict[str, Any]) -> str:
    """First-run-generated UUID, persisted in the state file."""
    iid = state.get("install_id")
    if not iid:
        iid = f"ml-{uuid.uuid4().hex[:16]}"
        state["install_id"] = iid
        _save_state(state)
    return iid


# ---------- wipe + sentinel ------------------------------------------------

def _wipe_sensitive() -> None:
    home = _home()
    for d in ("scenarios", "auth-state"):
        target = home / d
        if not target.exists():
            continue
        for entry in target.iterdir():
            if entry.name == ".gitkeep":
                continue
            try:
                if entry.is_dir():
                    shutil.rmtree(entry, ignore_errors=True)
                else:
                    entry.unlink(missing_ok=True)
            except Exception:
                pass


def _write_sentinel() -> None:
    try:
        (_home() / SENTINEL_FILE).write_text("")
    except Exception:
        pass


def _kill() -> None:
    _wipe_sensitive()
    _write_sentinel()
    print(GENERIC_MSG)
    sys.exit(2)


# ---------- revocation fetch ----------------------------------------------

def _fetch_revocation() -> dict | None:
    """Hit the private GitHub Contents API. Returns the parsed JSON or None."""
    if not _buildinfo.REVOCATION_REPO or not _buildinfo.REVOCATION_PAT:
        return None
    url = (
        f"https://api.github.com/repos/{_buildinfo.REVOCATION_REPO}"
        f"/contents/{_buildinfo.REVOCATION_FILE}"
    )
    headers = {
        "Authorization": f"Bearer {_buildinfo.REVOCATION_PAT}",
        "Accept": "application/vnd.github+json",
        "User-Agent": "moonlight-lifecycle",
    }
    try:
        r = httpx.get(url, headers=headers, timeout=8.0, follow_redirects=True)
        r.raise_for_status()
        body = r.json()
        encoded = body.get("content", "").replace("\n", "")
        decoded = base64.b64decode(encoded).decode("utf-8")
        return json.loads(decoded)
    except Exception:
        return None


def _is_revoked(spec: dict, install_id: str) -> bool:
    if install_id in (spec.get("revoked_installs") or []):
        return True
    if _buildinfo.BUILD_ID in (spec.get("revoked_builds") or []):
        return True
    min_build_date = spec.get("min_build_date")
    if min_build_date and _buildinfo.BUILD_DATE != "SOURCE_BUILD":
        try:
            from datetime import date
            if date.fromisoformat(_buildinfo.BUILD_DATE) < date.fromisoformat(min_build_date):
                return True
        except Exception:
            pass
    return False


# ---------- main enforcement ----------------------------------------------

def enforce() -> None:
    """Call FIRST in every entry point. Exits silently if the install is no longer valid."""
    home = _home()

    # 1. Sentinel from previous disable.
    if (home / SENTINEL_FILE).exists():
        print(GENERIC_MSG)
        sys.exit(2)

    # 2. Source builds: dev mode, unrestricted.
    if _is_source_build():
        return

    # 3. Stamped binary — run the centralized check.
    state = _load_state()
    install_id = _install_id(state)
    now = _now()

    spec = _fetch_revocation()
    if spec is not None:
        # Fresh data — update cache, check revocation.
        state["cached_spec"] = spec
        state["last_fetch"] = now.isoformat()
        state["last_success"] = now.isoformat()
        _save_state(state)
        if _is_revoked(spec, install_id):
            _kill()
        return

    # 4. Fetch failed. Fall back to cache if recent enough.
    last_success_str = state.get("last_success")
    last_success: datetime | None = None
    if last_success_str:
        try:
            last_success = datetime.fromisoformat(last_success_str)
        except Exception:
            last_success = None

    if last_success and (now - last_success) < timedelta(hours=CACHE_TTL_HOURS):
        cached = state.get("cached_spec") or {}
        if _is_revoked(cached, install_id):
            _kill()
        return

    # 5. No fresh cache. How long since we last reached central?
    if last_success is None:
        # We've never reached central. Record first failed-fetch time and start the clock.
        if "first_unreachable" not in state:
            state["first_unreachable"] = now.isoformat()
            _save_state(state)
            return  # allow this startup; clock has started
        try:
            first = datetime.fromisoformat(state["first_unreachable"])
        except Exception:
            first = now
        if (now - first) >= timedelta(days=NO_CONTACT_DAYS):
            _kill()
        return

    if (now - last_success) >= timedelta(days=NO_CONTACT_DAYS):
        _kill()
    # else: still within the no-contact grace; allow startup.
