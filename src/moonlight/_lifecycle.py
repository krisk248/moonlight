"""Centralized lifecycle enforcement.

On every entry point this module:
  1. Refuses to start if a sentinel file exists (previously disabled).
  2. Fetches the revocation JSON from a public-URL gist (no auth needed —
     the URL itself is the secret).
  3. Reads the `last_day` field. If today is past it → wipes sensitive
     data and exits silently.
  4. If the URL is unreachable, falls back to the last cached response
     (good for 24 hours).
  5. If the URL has been unreachable AND no recent cache for >30 days →
     wipes and exits silently. Deadman backstop.

All user-facing messages are deliberately generic; no dates, no mechanism,
no policy details surface to the operator.
"""
from __future__ import annotations

import json
import os
import shutil
import sys
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import httpx

from . import _buildinfo


SENTINEL_FILE = ".killed"
STATE_FILE = ".moonlight-state.json"
GENERIC_MSG = "This binary is no longer authorized. Contact your administrator."

CACHE_TTL_HOURS = 24       # how long a successful response is trusted offline
NO_CONTACT_DAYS = 30       # silent kill after this many days with no fresh data


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

def _fetch_spec() -> dict | None:
    """HTTP GET the revocation URL. Returns the parsed JSON or None on failure."""
    url = _buildinfo.REVOCATION_URL
    if not url:
        return None
    try:
        r = httpx.get(url, timeout=8.0, follow_redirects=True,
                      headers={"User-Agent": "moonlight-lifecycle"})
        r.raise_for_status()
        return r.json()
    except Exception:
        return None


def _is_past(last_day_str: str) -> bool:
    """True if today is strictly past last_day. Empty/blank means 'no expiry set'."""
    if not last_day_str:
        return False
    try:
        last_day = date.fromisoformat(last_day_str.strip())
    except (ValueError, AttributeError):
        return False
    return date.today() > last_day


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
    now = _now()

    spec = _fetch_spec()
    if spec is not None:
        # Fresh data — cache it and check the kill date.
        state["cached_spec"] = spec
        state["last_success"] = now.isoformat()
        state.pop("first_unreachable", None)
        _save_state(state)
        if _is_past(spec.get("last_day", "")):
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
        if _is_past(cached.get("last_day", "")):
            _kill()
        return

    # 5. No fresh cache. How long since we last reached central?
    if last_success is None:
        # We've never reached central. Record when we first failed and start the clock.
        if "first_unreachable" not in state:
            state["first_unreachable"] = now.isoformat()
            _save_state(state)
            return
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
