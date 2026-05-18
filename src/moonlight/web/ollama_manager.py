"""Detects, starts, and pulls models for a local Ollama instance.

The web UI uses this to give QA testers a one-click setup: if Ollama isn't
running, click 'Start'; if the configured model isn't pulled, click 'Pull'.
"""
from __future__ import annotations

import os
import shutil
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

import httpx


@dataclass
class OllamaStatus:
    binary_path: str | None
    server_running: bool
    server_url: str
    server_pid: int | None
    model_present: bool
    model_name: str
    models_available: list[str]


class OllamaManager:
    def __init__(self, host: str, model: str, log_dir: Path) -> None:
        self.host = host.rstrip("/")
        self.model = model
        self.log_dir = log_dir
        self.log_dir.mkdir(parents=True, exist_ok=True)

    # ---------- discovery ----------------------------------------------------

    @staticmethod
    def find_binary() -> str | None:
        for candidate in (
            shutil.which("ollama"),
            os.path.expanduser("~/.local/bin/ollama"),
            "/usr/local/bin/ollama",
        ):
            if candidate and Path(candidate).exists():
                return candidate
        return None

    def _server_pid(self) -> int | None:
        try:
            out = subprocess.run(
                ["pgrep", "-f", "ollama serve"],
                capture_output=True, text=True, check=False,
            ).stdout.strip().splitlines()
            return int(out[0]) if out else None
        except Exception:
            return None

    def _server_alive(self) -> bool:
        try:
            r = httpx.get(f"{self.host}/api/version", timeout=2)
            return r.status_code == 200
        except Exception:
            return False

    def _models(self) -> list[str]:
        try:
            r = httpx.get(f"{self.host}/api/tags", timeout=5)
            r.raise_for_status()
            return [m["name"] for m in r.json().get("models", [])]
        except Exception:
            return []

    def status(self) -> OllamaStatus:
        running = self._server_alive()
        models = self._models() if running else []
        return OllamaStatus(
            binary_path=self.find_binary(),
            server_running=running,
            server_url=self.host,
            server_pid=self._server_pid() if running else None,
            model_present=any(m.startswith(self.model.split(":")[0]) for m in models),
            model_name=self.model,
            models_available=models,
        )

    # ---------- control ------------------------------------------------------

    def start_server(self) -> tuple[bool, str]:
        if self._server_alive():
            return True, "already running"
        binary = self.find_binary()
        if not binary:
            return False, "ollama binary not found — install it first"
        log_path = self.log_dir / "ollama-serve.log"
        subprocess.Popen(
            [binary, "serve"],
            stdout=open(log_path, "ab"),
            stderr=subprocess.STDOUT,
            env={**os.environ, "OLLAMA_HOST": "127.0.0.1:11434"},
            start_new_session=True,
        )
        # Wait briefly for the server to come up
        for _ in range(20):
            if self._server_alive():
                return True, f"started (log: {log_path})"
            time.sleep(0.5)
        return False, f"started but didn't respond in 10s (log: {log_path})"

    def pull_model(self) -> subprocess.Popen:
        """Start a non-blocking `ollama pull` and return the Popen handle.

        The caller (JobRunner) tee's stdout into the job log for the UI.
        """
        binary = self.find_binary()
        if not binary:
            raise RuntimeError("ollama binary not found")
        return subprocess.Popen(
            [binary, "pull", self.model],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )
