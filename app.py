"""Moonlight one-shot launcher.

Run this from the project root:

    uv run python app.py
    # or, after `source .venv/bin/activate`:
    python app.py

It bootstraps Ollama if needed and opens the dashboard at http://127.0.0.1:8765.
"""
from __future__ import annotations

from moonlight._lifecycle import enforce as _enforce_lifecycle

_enforce_lifecycle()

from moonlight.web.app import main


if __name__ == "__main__":
    main()
