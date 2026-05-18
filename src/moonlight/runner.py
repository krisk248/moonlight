"""Scenario runner — loads YAML, drives Playwright, calls vision checks, writes report."""
from __future__ import annotations

import asyncio
import json
import re
import shutil
from dataclasses import dataclass, field, asdict
from datetime import datetime
from pathlib import Path
from typing import Any

import yaml


# Patterns produced by the OLD regex-based recorder. The selectors are mangled
# in two ways depending on what the user recorded:
#   1. Plain role+name:        textbox", name="Username
#   2. YAML-escaped role+name: button", name="Login 
#   3. Quoted (rare):          "button", name="Yes"
# All collapse to <role>", name="<name>  after stripping any stray quote chars.
_BROKEN_SELECTOR_RE = re.compile(
    r'^"?(?P<role>[A-Za-z_][A-Za-z0-9_-]*)"?\s*,\s*name="?(?P<name>.*?)"?\s*$'
)


def _migrate_step(step: dict) -> dict:
    """Repair locators recorded by the pre-AST parser. No-op on clean YAMLs."""
    sel = step.get("selector")
    if isinstance(sel, str) and "name=" in sel:
        m = _BROKEN_SELECTOR_RE.match(sel.strip())
        if m:
            step.pop("selector")
            step["role"] = m.group("role")
            name = m.group("name")
            if name:
                step["name"] = name
    return step

from .assertions import CheckResult, ai_check, ocr_check
from .browser import Browser
from .diff import pixel_diff
from .vision import SmolVLM2Client


# ---------- data classes ----------------------------------------------------

@dataclass
class StepResult:
    index: int
    action: str
    name: str
    passed: bool
    checks: list[dict[str, Any]] = field(default_factory=list)
    screenshot: str | None = None
    baseline: str | None = None
    diff_overlay: str | None = None
    diff_ratio: float | None = None
    error: str | None = None
    ai_narration: str | None = None
    console_messages: list[dict] = field(default_factory=list)
    failed_requests: list[dict] = field(default_factory=list)
    page_errors: list[str] = field(default_factory=list)


@dataclass
class RunResult:
    scenario: str
    url: str
    started_at: str
    finished_at: str
    passed: bool
    steps: list[StepResult]
    run_dir: str
    ai_summary: str | None = None


# ---------- helpers ---------------------------------------------------------

def _load_config(project_root: Path) -> dict:
    return yaml.safe_load((project_root / "config.yaml").read_text())


def _load_scenario(scenario_path: Path) -> dict:
    return yaml.safe_load(scenario_path.read_text())


def _slug(name: str) -> str:
    return "".join(c if c.isalnum() or c in "-_" else "-" for c in name).strip("-")


# ---------- single-step dispatcher -----------------------------------------

async def _do_step(
    idx: int,
    step: dict,
    browser: Browser,
    vision: SmolVLM2Client | None,
    run_dir: Path,
    baseline_dir: Path,
    diff_threshold: float,
    mode: str,  # "baseline" or "run"
) -> StepResult:
    step = _migrate_step(step)
    action = step["action"]
    # `name` is the screenshot label, but `name=` can also be a locator kwarg
    # for get_by_role. For step naming, prefer an explicit "label" field then
    # fall back to action+idx so role-name doesn't leak into the screenshot id.
    if action == "screenshot":
        name = step.get("name") or f"{action}-{idx:02d}"
    else:
        name = f"{action}-{idx:02d}"
    result = StepResult(index=idx, action=action, name=name, passed=True)

    try:
        if action == "goto":
            await browser.goto(step["url"])
        elif action == "click":
            await browser.do_click(step)
        elif action == "fill":
            await browser.do_fill(step)
        elif action == "select":
            await browser.do_select(step)
        elif action == "press":
            await browser.do_press(step)
        elif action == "wait_for":
            await browser.do_wait_for(step)
        elif action == "wait_ms":
            await browser.wait_ms(int(step["ms"]))
        elif action == "scroll":
            await browser.scroll(int(step.get("y", 400)))
        elif action == "screenshot":
            shot = run_dir / "screenshots" / f"{idx:02d}-{_slug(name)}.png"
            await browser.screenshot(shot, full_page=bool(step.get("full_page")))
            result.screenshot = str(shot)

            # baseline mode → save and skip checks
            if mode == "baseline":
                base_target = baseline_dir / f"{idx:02d}-{_slug(name)}.png"
                base_target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(shot, base_target)
                result.baseline = str(base_target)
                return result

            # run mode → regression diff + custom checks
            baseline_path = baseline_dir / f"{idx:02d}-{_slug(name)}.png"
            if baseline_path.exists():
                overlay_path = run_dir / "diffs" / f"{idx:02d}-{_slug(name)}.png"
                diff = pixel_diff(baseline_path, shot, overlay_path)
                result.baseline = str(baseline_path)
                result.diff_overlay = str(diff.overlay_path)
                result.diff_ratio = diff.ratio

                if diff.ratio > diff_threshold and vision is not None:
                    verdict = vision.regression_judge(baseline_path, shot)
                    result.checks.append({
                        "kind": "regression_judge",
                        "passed": verdict.passed,
                        "detail": f"diff_ratio={diff.ratio:.4f} | model: {verdict.raw}",
                    })
                    if not verdict.passed:
                        result.passed = False

            if vision is not None:
                # AI narration runs on EVERY screenshot — single sentence for the report.
                try:
                    result.ai_narration = vision.narrate(shot)
                except Exception as e:
                    result.ai_narration = f"(narration unavailable: {e})"

                if (ac := step.get("ai_check")):
                    if "TODO" not in str(ac.get("prompt", "")).upper():
                        cr = ai_check(vision, shot, ac["prompt"], ac.get("expect", "yes"))
                        result.checks.append(asdict(cr))
                        if not cr.passed:
                            result.passed = False
                if (oc := step.get("ocr_check")):
                    cr = ocr_check(vision, shot, list(oc.get("contains", [])))
                    result.checks.append(asdict(cr))
                    if not cr.passed:
                        result.passed = False
        else:
            raise ValueError(f"unknown action: {action}")
    except Exception as e:
        result.passed = False
        result.error = f"{type(e).__name__}: {e}"

    # Capture browser events that fired during this step.
    events = browser.snapshot_events()
    result.console_messages = events["console"]
    result.failed_requests = events["failed_requests"]
    result.page_errors = events["page_errors"]
    return result


# ---------- public API ------------------------------------------------------

async def _execute(
    scenario: dict,
    project_root: Path,
    mode: str,
    on_log=None,
) -> RunResult:
    cfg = _load_config(project_root)
    scenario_name = scenario["name"]
    started = datetime.now()
    stamp = started.strftime("%Y%m%d-%H%M%S")
    run_dir = project_root / cfg["paths"]["runs"] / f"{stamp}-{scenario_name}"
    (run_dir / "screenshots").mkdir(parents=True, exist_ok=True)
    (run_dir / "diffs").mkdir(parents=True, exist_ok=True)
    baseline_dir = project_root / cfg["paths"]["baselines"] / scenario_name

    vision = None
    if mode == "run":
        vision = SmolVLM2Client(
            host=cfg["ollama"]["host"],
            model=cfg["ollama"]["model"],
            timeout_s=cfg["ollama"]["timeout_s"],
            options=cfg["ollama"].get("options"),
        )

    viewport = scenario.get("viewport") or cfg["browser"]["viewport"]
    headless = scenario.get("headless", cfg["browser"]["headless"])
    threshold = float(cfg["diff"]["threshold"])
    storage_state = scenario.get("storage_state")
    if storage_state:
        storage_state = project_root / storage_state

    def _log(line: str) -> None:
        if on_log:
            on_log(line)

    _log(f"[start] mode={mode} url={scenario['url']} steps={len(scenario['steps'])}")
    steps: list[StepResult] = []
    async with Browser(
        base_url=scenario["url"],
        viewport=viewport,
        headless=headless,
        default_timeout_ms=cfg["browser"]["default_timeout_ms"],
        storage_state=storage_state,
    ) as browser:
        for idx, step in enumerate(scenario["steps"], start=1):
            preview = step.get("url") or step.get("selector") or step.get("name") or ""
            _log(f"[step {idx:02d}] {step['action']} {preview}")
            res = await _do_step(idx, step, browser, vision, run_dir, baseline_dir, threshold, mode)
            steps.append(res)
            verdict = "PASS" if res.passed else "FAIL"
            if res.ai_narration:
                _log(f"   ↳ AI sees: {res.ai_narration}")
            for c in res.checks:
                _log(f"   ↳ AI {c['kind']}: {'PASS' if c['passed'] else 'FAIL'}")
            if res.failed_requests:
                _log(f"   ↳ network: {len(res.failed_requests)} failed request(s)")
            if res.page_errors:
                _log(f"   ↳ JS errors: {len(res.page_errors)}")
            if res.error:
                _log(f"   ↳ error: {res.error}")
            _log(f"   ↳ verdict: {verdict}")

    finished = datetime.now()
    overall = all(s.passed for s in steps)

    # AI run summary — only in 'run' mode (baseline has no verdicts to summarize).
    summary: str | None = None
    if mode == "run" and vision is not None:
        log_lines = []
        for s in steps:
            line = f"Step {s.index} [{s.action}] {'PASS' if s.passed else 'FAIL'}"
            if s.ai_narration:
                line += f" — {s.ai_narration}"
            if s.error:
                line += f" — error: {s.error[:120]}"
            for c in s.checks:
                if not c.get("passed"):
                    line += f" — AI {c['kind']}: {c['detail'][:120]}"
            log_lines.append(line)
        try:
            summary = vision.summarize_run("\n".join(log_lines))
            _log(f"[summary] {summary}")
        except Exception as e:
            summary = f"(summary unavailable: {e})"

    return RunResult(
        scenario=scenario_name,
        url=scenario["url"],
        started_at=started.isoformat(timespec="seconds"),
        finished_at=finished.isoformat(timespec="seconds"),
        passed=overall,
        steps=steps,
        run_dir=str(run_dir),
        ai_summary=summary,
    )


def run_scenario(scenario_path: Path, project_root: Path, mode: str = "run", on_log=None) -> RunResult:
    scenario = _load_scenario(scenario_path)
    return asyncio.run(_execute(scenario, project_root, mode, on_log=on_log))


def write_json(result: RunResult, out_path: Path) -> None:
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(asdict(result), indent=2))
