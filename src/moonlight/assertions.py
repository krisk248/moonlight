"""Step-level assertion helpers — wrap vision client calls with normalized verdicts."""
from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from .vision import SmolVLM2Client, VisionVerdict


@dataclass
class CheckResult:
    kind: str
    passed: bool
    detail: str


def ai_check(client: SmolVLM2Client, image: Path, prompt: str, expect: str | bool = "yes") -> CheckResult:
    if isinstance(expect, bool):
        expect_yes = expect
    else:
        expect_yes = str(expect).strip().lower() in {"yes", "true", "y", "1"}
    verdict: VisionVerdict = client.functional_check(image, prompt, expect_yes=expect_yes)
    return CheckResult(
        kind="ai_check",
        passed=verdict.passed,
        detail=f"expected={'YES' if expect_yes else 'NO'} | model: {verdict.raw}",
    )


def ocr_check(client: SmolVLM2Client, image: Path, contains: list[str]) -> CheckResult:
    text = client.ocr(image)
    lower = text.lower()
    missing = [s for s in contains if s.lower() not in lower]
    return CheckResult(
        kind="ocr_check",
        passed=not missing,
        detail=(
            f"all required strings found: {contains}"
            if not missing
            else f"missing strings: {missing} | OCR text: {text[:300]}"
        ),
    )
