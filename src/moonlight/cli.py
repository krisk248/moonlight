"""Moonlight CLI — record, baseline, run, selftest."""
from __future__ import annotations

from . import _lifecycle as _lc
_lc.enforce()

import asyncio
import sys
from pathlib import Path

import click
import yaml
from rich.console import Console
from rich.table import Table

from . import _buildinfo, export
from .browser import Browser
from .recorder import record as record_session
from .report import write_html, write_json
from .runner import run_scenario
from .vision import SmolVLM2Client


console = Console()


def _project_root() -> Path:
    return Path.cwd()


def _config() -> dict:
    return yaml.safe_load((_project_root() / "config.yaml").read_text())


def _scenarios_dir() -> Path:
    return _project_root() / _config()["paths"]["scenarios"]


def _print_summary(result, run_dir: Path) -> None:
    table = Table(title=f"Moonlight — {result.scenario}", show_lines=False)
    table.add_column("#", justify="right")
    table.add_column("Action")
    table.add_column("Name")
    table.add_column("Verdict")
    for s in result.steps:
        table.add_row(
            str(s.index),
            s.action,
            s.name,
            "[green]PASS[/]" if s.passed else "[red]FAIL[/]",
        )
    console.print(table)
    overall = "[green]PASS[/]" if result.passed else "[red]FAIL[/]"
    console.print(f"Overall: {overall}   Report: {run_dir / 'report.html'}")


@click.group()
@click.version_option(version=_buildinfo.VERSION, prog_name="moonlight",
                      message=f"%(prog)s %(version)s\nbuild: {_buildinfo.BUILD_ID}")
def cli() -> None:
    """Automated visual regression testing for web apps."""


@cli.command(name="export-scenarios", hidden=True)
@click.argument("output", required=False)
def export_scenarios_cmd(output: str | None) -> None:
    """Internal: export scenarios + baselines to a portable zip."""
    sys.exit(export.run(output))


@cli.command()
@click.argument("url")
@click.argument("scenario_name")
def record(url: str, scenario_name: str) -> None:
    """Record a flow using Playwright codegen and save a YAML scenario."""
    cfg = _config()
    path = record_session(
        url=url,
        scenario_name=scenario_name,
        scenarios_dir=_scenarios_dir(),
        viewport=cfg["browser"]["viewport"],
    )
    console.print(f"[green]Recorded:[/] {path}")
    console.print("Next: edit the TODO ai_check prompts, then run [bold]moonlight baseline " + scenario_name + "[/].")


@cli.command()
@click.argument("scenario_name")
def baseline(scenario_name: str) -> None:
    """Run a scenario once and save its screenshots as the reference baseline."""
    scenario = _scenarios_dir() / f"{scenario_name}.yaml"
    if not scenario.exists():
        console.print(f"[red]No scenario at {scenario}[/]")
        sys.exit(1)
    result = run_scenario(scenario, _project_root(), mode="baseline")
    console.print(f"[green]Baseline captured for {scenario_name}[/] — {len(result.steps)} steps")


@cli.command()
@click.argument("scenario_name", required=False)
@click.option("--all", "run_all", is_flag=True, help="Run every scenario in scenarios/.")
def run(scenario_name: str | None, run_all: bool) -> None:
    """Run a scenario, diff against baseline, ask SmolVLM2 to verify each step."""
    if run_all:
        targets = sorted(_scenarios_dir().glob("*.yaml"))
    elif scenario_name:
        targets = [_scenarios_dir() / f"{scenario_name}.yaml"]
    else:
        console.print("[red]Provide a scenario name or --all[/]")
        sys.exit(1)

    any_failed = False
    for scenario in targets:
        if not scenario.exists():
            console.print(f"[red]Missing scenario:[/] {scenario}")
            any_failed = True
            continue
        result = run_scenario(scenario, _project_root(), mode="run")
        run_dir = Path(result.run_dir)
        write_html(result, run_dir / "report.html")
        write_json(result, run_dir / "report.json")
        _print_summary(result, run_dir)
        if not result.passed:
            any_failed = True

    sys.exit(1 if any_failed else 0)


@cli.command()
@click.option("--host", default="127.0.0.1", show_default=True)
@click.option("--port", default=8765, show_default=True, type=int)
def serve(host: str, port: int) -> None:
    """Start the Moonlight web dashboard."""
    from .web.app import main as web_main
    web_main(host=host, port=port)


@cli.command()
def selftest() -> None:
    """End-to-end smoke test: load example.com, ask SmolVLM2 what it sees."""
    cfg = _config()
    out_dir = _project_root() / "runs" / "selftest"
    out_dir.mkdir(parents=True, exist_ok=True)
    shot = out_dir / "example.png"

    async def _run() -> None:
        async with Browser(
            base_url="https://example.com",
            viewport=cfg["browser"]["viewport"],
            headless=True,
            default_timeout_ms=cfg["browser"]["default_timeout_ms"],
        ) as b:
            await b.goto("/")
            await b.screenshot(shot)

    asyncio.run(_run())

    client = SmolVLM2Client(
        host=cfg["ollama"]["host"],
        model=cfg["ollama"]["model"],
        timeout_s=cfg["ollama"]["timeout_s"],
        options=cfg["ollama"].get("options"),
    )
    if not client.ping():
        console.print(f"[red]Ollama unreachable at {cfg['ollama']['host']}[/] — start it with `ollama serve`.")
        sys.exit(1)

    console.print(f"[cyan]Screenshot:[/] {shot}")
    text = client.ocr(shot)
    console.print(f"[cyan]OCR:[/] {text[:300]}")
    verdict = client.functional_check(shot, "Is this the IANA example.com placeholder page?")
    console.print(f"[cyan]Functional check ({'PASS' if verdict.passed else 'FAIL'}):[/] {verdict.raw}")


def main() -> None:
    cli()


if __name__ == "__main__":
    main()
