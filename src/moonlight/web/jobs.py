"""In-memory async job tracker for record/baseline/run/pull operations."""
from __future__ import annotations

import asyncio
import threading
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Awaitable, Callable


@dataclass
class Job:
    id: str
    kind: str            # "record" | "baseline" | "run" | "pull"
    label: str
    status: str = "pending"   # pending | running | completed | failed
    log_lines: list[str] = field(default_factory=list)
    started_at: float = field(default_factory=time.time)
    finished_at: float | None = None
    result: dict[str, Any] = field(default_factory=dict)
    error: str | None = None

    def append(self, line: str) -> None:
        self.log_lines.append(line.rstrip())

    @property
    def log(self) -> str:
        return "\n".join(self.log_lines)


class JobRunner:
    def __init__(self) -> None:
        self._jobs: dict[str, Job] = {}
        self._lock = threading.Lock()

    # ---------- registry -----------------------------------------------------

    def list(self) -> list[Job]:
        with self._lock:
            return sorted(self._jobs.values(), key=lambda j: j.started_at, reverse=True)

    def get(self, job_id: str) -> Job | None:
        return self._jobs.get(job_id)

    def _new(self, kind: str, label: str) -> Job:
        j = Job(id=uuid.uuid4().hex[:10], kind=kind, label=label)
        with self._lock:
            self._jobs[j.id] = j
        return j

    # ---------- runners ------------------------------------------------------

    def submit_thread(
        self,
        kind: str,
        label: str,
        target: Callable[[Job], dict[str, Any] | None],
    ) -> Job:
        """Run a sync function on a daemon thread, capturing logs and result."""
        job = self._new(kind, label)

        def _run() -> None:
            job.status = "running"
            try:
                out = target(job) or {}
                job.result = out
                job.status = "completed"
            except Exception as e:
                job.error = f"{type(e).__name__}: {e}"
                job.status = "failed"
                job.append(f"[error] {job.error}")
            finally:
                job.finished_at = time.time()

        threading.Thread(target=_run, daemon=True).start()
        return job

    def submit_async(
        self,
        kind: str,
        label: str,
        coro_factory: Callable[[Job], Awaitable[dict[str, Any] | None]],
    ) -> Job:
        """Run an async coroutine to completion in its own event loop."""
        def _wrap(j: Job) -> dict[str, Any]:
            return asyncio.run(coro_factory(j)) or {}
        return self.submit_thread(kind, label, _wrap)

    def stream_subprocess(self, job: Job, popen) -> int:
        """Tee a subprocess.Popen's stdout into the job log; return exit code."""
        if popen.stdout:
            for line in popen.stdout:
                job.append(line)
        return popen.wait()


# Single shared instance — imported by app.py
JOBS = JobRunner()
