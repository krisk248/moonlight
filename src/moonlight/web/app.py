"""FastAPI web dashboard — start with `python app.py` from the project root.

This is the single entry point QA testers see. It:
  - auto-detects/starts Ollama and the SmolVLM2 model
  - lists scenarios and their baselines
  - records new scenarios via Playwright codegen (opens a Chromium window)
  - runs regressions, tails their logs, and shows side-by-side reports
"""
from __future__ import annotations

import shutil
import yaml
from pathlib import Path

from fastapi import FastAPI, Form, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates

from ..recorder import record as record_session
from ..report import write_html, write_json
from ..runner import run_scenario
from .jobs import JOBS
from .ollama_manager import OllamaManager


# ---------- bootstrap ------------------------------------------------------

PROJECT_ROOT = Path(__file__).resolve().parents[3]   # …/moonlight/
WEB_DIR = Path(__file__).parent
TEMPLATES = Jinja2Templates(directory=str(WEB_DIR / "templates"))


def _config() -> dict:
    return yaml.safe_load((PROJECT_ROOT / "config.yaml").read_text())


def _ollama() -> OllamaManager:
    cfg = _config()
    return OllamaManager(
        host=cfg["ollama"]["host"],
        model=cfg["ollama"]["model"],
        log_dir=PROJECT_ROOT / "runs" / "_ollama-logs",
    )


def _scenarios_dir() -> Path:
    return PROJECT_ROOT / _config()["paths"]["scenarios"]


def _runs_dir() -> Path:
    return PROJECT_ROOT / _config()["paths"]["runs"]


def _baselines_dir() -> Path:
    return PROJECT_ROOT / _config()["paths"]["baselines"]


def _list_scenarios() -> list[dict]:
    out = []
    for p in sorted(_scenarios_dir().glob("*.yaml")):
        try:
            data = yaml.safe_load(p.read_text()) or {}
        except Exception as e:
            data = {"error": str(e)}
        baseline_dir = _baselines_dir() / p.stem
        out.append({
            "name": p.stem,
            "url": data.get("url", ""),
            "steps": len(data.get("steps") or []),
            "has_baseline": baseline_dir.exists() and any(baseline_dir.iterdir()),
            "has_storage": bool(data.get("storage_state")),
            "yaml_path": str(p),
        })
    return out


def _list_runs(limit: int = 25) -> list[dict]:
    runs: list[dict] = []
    if not _runs_dir().exists():
        return runs
    for p in sorted(_runs_dir().iterdir(), reverse=True):
        if not p.is_dir() or p.name.startswith("_"):
            continue
        report = p / "report.json"
        if not report.exists():
            continue
        try:
            import json
            data = json.loads(report.read_text())
            runs.append({
                "id": p.name,
                "scenario": data.get("scenario"),
                "passed": data.get("passed"),
                "url": data.get("url"),
                "started_at": data.get("started_at"),
                "step_count": len(data.get("steps", [])),
                "fail_count": sum(1 for s in data.get("steps", []) if not s.get("passed")),
            })
        except Exception:
            continue
        if len(runs) >= limit:
            break
    return runs


# ---------- FastAPI --------------------------------------------------------

# Ensure mount directories exist before FastAPI binds StaticFiles to them.
_runs_dir().mkdir(parents=True, exist_ok=True)
_baselines_dir().mkdir(parents=True, exist_ok=True)

app = FastAPI(title="Moonlight")
app.mount("/static", StaticFiles(directory=str(WEB_DIR / "static")), name="static")
# Static-file mounts are placed under /files/ to avoid swallowing the
# /runs/{id}/view HTML route below.
app.mount("/files/runs", StaticFiles(directory=str(_runs_dir().resolve())), name="run_files")
app.mount("/files/baselines", StaticFiles(directory=str(_baselines_dir().resolve())), name="baseline_files")


@app.get("/", response_class=HTMLResponse)
def dashboard(request: Request):
    return TEMPLATES.TemplateResponse(
        request, "dashboard.html",
        {
            "ollama": _ollama().status(),
            "scenarios": _list_scenarios(),
            "runs": _list_runs(),
            "jobs": JOBS.list()[:10],
        },
    )


# ---------- Ollama controls ------------------------------------------------

@app.post("/ollama/start")
def ollama_start():
    ok, msg = _ollama().start_server()
    if not ok:
        raise HTTPException(500, msg)
    return RedirectResponse("/", status_code=303)


@app.post("/ollama/pull")
def ollama_pull():
    mgr = _ollama()
    if not mgr.find_binary():
        raise HTTPException(500, "ollama binary not found")

    def _do(job):
        job.append(f"$ ollama pull {mgr.model}")
        popen = mgr.pull_model()
        code = JOBS.stream_subprocess(job, popen)
        if code != 0:
            raise RuntimeError(f"pull exited {code}")
        job.append("[done] model pulled")
        return {"model": mgr.model}

    job = JOBS.submit_thread("pull", f"ollama pull {mgr.model}", _do)
    return RedirectResponse(f"/jobs/{job.id}", status_code=303)


# ---------- scenarios ------------------------------------------------------

@app.get("/scenarios/new", response_class=HTMLResponse)
def scenario_new(request: Request):
    return TEMPLATES.TemplateResponse(request, "record.html", {})


@app.post("/scenarios/record")
def scenario_record(name: str = Form(...), url: str = Form(...), save_login: str = Form("")):
    name = "".join(c for c in name if c.isalnum() or c in "-_").lower() or "unnamed"
    auth_path = None
    if save_login:
        auth_path = PROJECT_ROOT / "auth-state" / f"{name}.json"

    def _do(job):
        job.append(f"[record] launching Playwright codegen → {url}")
        job.append("[record] interact with the Chromium window, then close it to finish.")
        path = record_session(
            url=url,
            scenario_name=name,
            scenarios_dir=_scenarios_dir(),
            viewport=_config()["browser"]["viewport"],
            auth_state_path=auth_path,
        )
        job.append(f"[record] wrote {path}")
        return {"scenario": name, "yaml_path": str(path)}

    job = JOBS.submit_thread("record", f"record {name} ({url})", _do)
    return RedirectResponse(f"/jobs/{job.id}", status_code=303)


@app.get("/scenarios/{name}", response_class=HTMLResponse)
def scenario_detail(request: Request, name: str):
    path = _scenarios_dir() / f"{name}.yaml"
    if not path.exists():
        raise HTTPException(404, f"no scenario {name}")
    return TEMPLATES.TemplateResponse(
        request, "scenario.html",
        {
            "name": name,
            "raw_yaml": path.read_text(),
            "data": yaml.safe_load(path.read_text()),
        },
    )


@app.post("/scenarios/{name}/save")
def scenario_save(name: str, yaml_text: str = Form(...)):
    path = _scenarios_dir() / f"{name}.yaml"
    if not path.exists():
        raise HTTPException(404)
    # Validate YAML before writing
    try:
        yaml.safe_load(yaml_text)
    except yaml.YAMLError as e:
        raise HTTPException(400, f"invalid YAML: {e}")
    path.write_text(yaml_text)
    return RedirectResponse(f"/scenarios/{name}", status_code=303)


@app.post("/scenarios/{name}/baseline")
def scenario_baseline(name: str):
    path = _scenarios_dir() / f"{name}.yaml"
    if not path.exists():
        raise HTTPException(404)

    def _do(job):
        job.append(f"[baseline] capturing for {name}")
        result = run_scenario(path, PROJECT_ROOT, mode="baseline", on_log=job.append)
        job.append(f"[baseline] {len(result.steps)} screenshots saved")
        return {"steps": len(result.steps)}

    job = JOBS.submit_thread("baseline", f"baseline {name}", _do)
    return RedirectResponse(f"/jobs/{job.id}", status_code=303)


@app.post("/scenarios/{name}/run")
def scenario_run(name: str):
    path = _scenarios_dir() / f"{name}.yaml"
    if not path.exists():
        raise HTTPException(404)

    def _do(job):
        job.append(f"[run] starting {name}")
        result = run_scenario(path, PROJECT_ROOT, mode="run", on_log=job.append)
        run_dir = Path(result.run_dir)
        write_html(result, run_dir / "report.html")
        write_json(result, run_dir / "report.json")
        verdict = "PASS" if result.passed else "FAIL"
        job.append(f"[run] {verdict} — {len([s for s in result.steps if not s.passed])} step(s) failed")
        return {"run_id": run_dir.name, "passed": result.passed}

    job = JOBS.submit_thread("run", f"run {name}", _do)
    return RedirectResponse(f"/jobs/{job.id}", status_code=303)


@app.post("/scenarios/{name}/delete")
def scenario_delete(name: str):
    path = _scenarios_dir() / f"{name}.yaml"
    if path.exists():
        path.unlink()
    bdir = _baselines_dir() / name
    if bdir.exists():
        shutil.rmtree(bdir)
    auth = PROJECT_ROOT / "auth-state" / f"{name}.json"
    if auth.exists():
        auth.unlink()
    return RedirectResponse("/", status_code=303)


# ---------- jobs / runs ----------------------------------------------------

@app.get("/jobs/{job_id}", response_class=HTMLResponse)
def job_detail(request: Request, job_id: str):
    job = JOBS.get(job_id)
    if not job:
        raise HTTPException(404)
    return TEMPLATES.TemplateResponse(request, "job.html", {"job": job})


@app.get("/jobs/{job_id}/log", response_class=HTMLResponse)
def job_log(request: Request, job_id: str):
    job = JOBS.get(job_id)
    if not job:
        raise HTTPException(404)
    return TEMPLATES.TemplateResponse(request, "_job_log.html", {"job": job})


@app.get("/runs/{run_id}/view", response_class=HTMLResponse)
def run_view(request: Request, run_id: str):
    import json
    run_dir = _runs_dir() / run_id
    report = run_dir / "report.json"
    if not report.exists():
        raise HTTPException(404, f"no run {run_id}")
    data = json.loads(report.read_text())

    def _file_url(abs_path: str | None) -> str | None:
        if not abs_path:
            return None
        try:
            rel = Path(abs_path).resolve().relative_to(_runs_dir().resolve())
            return f"/files/runs/{rel}"
        except ValueError:
            try:
                rel = Path(abs_path).resolve().relative_to(_baselines_dir().resolve())
                return f"/files/baselines/{rel}"
            except ValueError:
                return None

    # Rewrite absolute paths into URLs the browser can fetch.
    for s in data.get("steps", []):
        s["screenshot_url"] = _file_url(s.get("screenshot"))
        s["baseline_url"] = _file_url(s.get("baseline"))
        s["diff_url"] = _file_url(s.get("diff_overlay"))

    return TEMPLATES.TemplateResponse(
        request, "run.html",
        {"run_id": run_id, "run": data, "report_path": f"/files/runs/{run_id}/report.html"},
    )


@app.get("/api/status")
def api_status():
    return JSONResponse({
        "ollama": _ollama().status().__dict__,
        "scenarios": _list_scenarios(),
        "runs": _list_runs(),
    })


# ---------- entry point ----------------------------------------------------

def main(host: str = "127.0.0.1", port: int = 8765) -> None:
    import uvicorn
    # Best-effort: start Ollama if it isn't already up.
    mgr = _ollama()
    if mgr.find_binary() and not mgr._server_alive():
        ok, msg = mgr.start_server()
        print(f"[moonlight] ollama: {'ok' if ok else 'warn'} — {msg}")
    print(f"[moonlight] open http://{host}:{port}")
    uvicorn.run(app, host=host, port=port, log_level="info")
