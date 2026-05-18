"""HTML + JSON report generation."""
from __future__ import annotations

import os
from dataclasses import asdict
from pathlib import Path

from jinja2 import Environment, FileSystemLoader, select_autoescape

from .runner import RunResult


def _env() -> Environment:
    tpl_dir = Path(__file__).parent / "templates"
    return Environment(
        loader=FileSystemLoader(str(tpl_dir)),
        autoescape=select_autoescape(["html"]),
    )


def write_html(result: RunResult, out_path: Path) -> Path:
    out_path.parent.mkdir(parents=True, exist_ok=True)
    env = _env()
    tpl = env.get_template("report.html.j2")

    def rel(abs_or_rel: str) -> str:
        p = Path(abs_or_rel)
        try:
            return os.path.relpath(p, out_path.parent)
        except ValueError:
            return str(p)

    html = tpl.render(result=result, rel=rel)
    out_path.write_text(html)
    return out_path


def write_json(result: RunResult, out_path: Path) -> Path:
    import json
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(asdict(result), indent=2))
    return out_path
