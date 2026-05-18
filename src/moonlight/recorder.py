"""Wraps `playwright codegen` and converts the recorded session into YAML.

Uses ast.parse for the Python source so locators like
`page.get_by_role("button", name="Yes")` survive intact — earlier regex-based
parsing collapsed them into broken CSS selectors.
"""
from __future__ import annotations

import ast
import subprocess
import sys
import tempfile
from pathlib import Path

import yaml


# Locator-chain actions emitted by codegen.
_ACTIONS = {"click", "fill", "press", "select_option", "check", "uncheck", "set_input_files"}


def _literal(node: ast.AST):
    if isinstance(node, ast.Constant):
        return node.value
    return None


def _parse_locator_call(call: ast.Call) -> dict | None:
    """Turn `page.get_by_role("button", name="Yes")` into {role: button, name: Yes}.

    Also handles `page.locator("css")` → {selector: css}.
    """
    if not isinstance(call.func, ast.Attribute):
        return None
    if not (isinstance(call.func.value, ast.Name) and call.func.value.id == "page"):
        return None

    method = call.func.attr
    pos = [_literal(a) for a in call.args]
    kw = {k.arg: _literal(k.value) for k in call.keywords if k.arg}

    if method == "locator":
        return {"selector": pos[0] if pos else ""}

    if method.startswith("get_by_"):
        kind = method[len("get_by_"):]   # role, label, text, placeholder, test_id, alt_text, title
        out: dict = {kind: pos[0] if pos else ""}
        for k, v in kw.items():
            if v is not None:
                out[k] = v
        return out

    return None


def _parse_line(line: str) -> dict | None:
    """Convert one codegen statement into a YAML step (or None)."""
    line = line.strip()
    if not line.startswith("await "):
        return None
    expr_src = line[len("await "):].rstrip()
    try:
        tree = ast.parse(expr_src, mode="eval").body
    except SyntaxError:
        return None
    if not isinstance(tree, ast.Call):
        return None

    func = tree.func

    # Case 1: bare `page.goto("url")` (one call, no chain)
    if isinstance(func, ast.Attribute) and isinstance(func.value, ast.Name) and func.value.id == "page":
        if func.attr == "goto" and tree.args:
            return {"action": "goto", "url": _literal(tree.args[0])}
        return None

    # Case 2: chain like LOCATOR.<ACTION>(...)
    if not isinstance(func, ast.Attribute):
        return None
    action = func.attr
    if action not in _ACTIONS:
        return None
    action_args = [_literal(a) for a in tree.args]

    locator_node = func.value
    if not isinstance(locator_node, ast.Call):
        return None
    loc = _parse_locator_call(locator_node)
    if loc is None:
        return None

    step: dict = {"action": "click" if action == "click" else action, **loc}

    if action == "fill":
        step["value"] = action_args[0] if action_args else ""
    elif action == "press":
        step["key"] = action_args[0] if action_args else ""
    elif action == "select_option":
        step["action"] = "select"
        step["value"] = action_args[0] if action_args else ""

    return step


def _parse_codegen(py_source: str) -> tuple[str | None, list[dict]]:
    base_url: str | None = None
    steps: list[dict] = []
    placeholder_idx = 0

    def add_screenshot(hint: str) -> None:
        nonlocal placeholder_idx
        placeholder_idx += 1
        steps.append({
            "action": "screenshot",
            "name": f"{hint}-{placeholder_idx:02d}",
            "ai_check": {
                "prompt": "TODO: describe what should be visible here",
                "expect": "yes",
            },
        })

    for raw in py_source.splitlines():
        step = _parse_line(raw)
        if not step:
            continue
        if step["action"] == "goto":
            url = step["url"]
            if base_url is None:
                base_url = url
                steps.append({"action": "goto", "url": "/"})
            else:
                steps.append({"action": "goto", "url": url})
            add_screenshot("after-goto")
            continue
        steps.append(step)
        if step["action"] in {"click", "press"}:
            add_screenshot(f"after-{step['action']}")

    return base_url, steps


def record(
    url: str,
    scenario_name: str,
    scenarios_dir: Path,
    viewport: dict[str, int],
    auth_state_path: Path | None = None,
) -> Path:
    """Launch playwright codegen, then write scenarios/<name>.yaml. Returns the path."""
    scenarios_dir.mkdir(parents=True, exist_ok=True)
    out_yaml = scenarios_dir / f"{scenario_name}.yaml"

    with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False) as tmp:
        tmp_path = Path(tmp.name)

    cmd = [
        sys.executable, "-m", "playwright", "codegen",
        "--target", "python-async",
        "--output", str(tmp_path),
        f"--viewport-size={viewport['width']},{viewport['height']}",
    ]
    if auth_state_path is not None:
        auth_state_path.parent.mkdir(parents=True, exist_ok=True)
        cmd += ["--save-storage", str(auth_state_path)]
    cmd.append(url)

    print(f"[recorder] launching: {' '.join(cmd)}")
    print("[recorder] click through your app (incl. login), then close the Chromium window.")
    subprocess.run(cmd, check=True)

    source = tmp_path.read_text()
    base, steps = _parse_codegen(source)
    if not base:
        raise RuntimeError("no goto() detected — recording produced no usable steps")

    scenario: dict = {
        "name": scenario_name,
        "url": base,
        "viewport": viewport,
        "steps": steps,
    }
    if auth_state_path is not None and auth_state_path.exists():
        try:
            rel = auth_state_path.relative_to(scenarios_dir.parent)
            scenario["storage_state"] = str(rel)
        except ValueError:
            scenario["storage_state"] = str(auth_state_path)

    out_yaml.write_text(yaml.safe_dump(scenario, sort_keys=False, default_flow_style=False))
    print(f"[recorder] wrote {out_yaml} — open it and fill in the TODO ai_check prompts.")
    return out_yaml
