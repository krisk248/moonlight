"""SmolVLM2 client — wraps the local Ollama API for QA-flavoured prompts."""
from __future__ import annotations

import base64
from dataclasses import dataclass
from pathlib import Path

import ollama


FUNCTIONAL_PROMPT = (
    "You are a QA test assistant. Look at the screenshot of a web application. "
    "Answer the following question with 'YES' or 'NO' as the FIRST word, then a "
    "one-sentence reason. Question: {question}"
)

OCR_PROMPT = (
    "Transcribe all visible text in this screenshot exactly as shown. "
    "Output the text only, no commentary, no markdown."
)

REGRESSION_PROMPT = (
    "Two screenshots of the same web page are provided — the FIRST is the baseline "
    "and the SECOND is the current run. First, answer 'MEANINGFUL' or 'COSMETIC' "
    "as the FIRST word: MEANINGFUL = a real UI/bug change, COSMETIC = noise such "
    "as timestamps, ads, random IDs, or animations. Then write ONE short sentence "
    "describing exactly what visibly changed between the two screenshots."
)

NARRATE_PROMPT = (
    "In ONE short sentence, describe what is visible on this web app screenshot. "
    "Mention specific UI elements (buttons, forms, errors, content). No preamble, "
    "no 'this image shows' — just the description."
)

SUMMARY_PROMPT = (
    "You are a senior QA engineer. Below is a per-step log from one regression "
    "run. Write 2–3 sentences summarizing: (1) what was tested, (2) what worked, "
    "(3) what failed and the likely cause. Be specific and concrete. No bullet "
    "points, no markdown. Output the summary only.\n\n"
    "Run log:\n{log}"
)


@dataclass
class VisionVerdict:
    passed: bool
    raw: str

    @property
    def reason(self) -> str:
        parts = self.raw.split(maxsplit=1)
        return parts[1].strip() if len(parts) > 1 else ""


class SmolVLM2Client:
    def __init__(
        self,
        host: str = "http://localhost:11434",
        model: str = "ahmadwaqar/smolvlm2-2.2b-instruct",
        timeout_s: int = 120,
        options: dict | None = None,
    ) -> None:
        self.model = model
        self._client = ollama.Client(host=host, timeout=timeout_s)
        self._options = {"temperature": 0.0, "num_predict": 200, **(options or {})}

    def _ask(self, prompt: str, images: list[Path]) -> str:
        encoded = [base64.b64encode(p.read_bytes()).decode("ascii") for p in images]
        resp = self._client.chat(
            model=self.model,
            options=self._options,
            messages=[{"role": "user", "content": prompt, "images": encoded}],
        )
        return (resp.get("message") or {}).get("content", "").strip()

    def functional_check(self, image: Path, question: str, expect_yes: bool = True) -> VisionVerdict:
        raw = self._ask(FUNCTIONAL_PROMPT.format(question=question), [image])
        first = raw.split(maxsplit=1)[0].strip(".,:;!?").upper() if raw else ""
        got_yes = first.startswith("YES")
        return VisionVerdict(passed=(got_yes == expect_yes), raw=raw)

    def ocr(self, image: Path) -> str:
        return self._ask(OCR_PROMPT, [image])

    def regression_judge(self, baseline: Path, current: Path) -> VisionVerdict:
        raw = self._ask(REGRESSION_PROMPT, [baseline, current])
        first = raw.split(maxsplit=1)[0].strip(".,:;!?").upper() if raw else ""
        # passed = differences are cosmetic (not real regressions)
        return VisionVerdict(passed=first.startswith("COSMETIC"), raw=raw)

    def narrate(self, image: Path) -> str:
        """Return one short sentence describing the screen — for the report."""
        raw = self._ask(NARRATE_PROMPT, [image])
        # Trim to one sentence if the model rambled
        for sep in ("\n", ". "):
            if sep in raw:
                raw = raw.split(sep, 1)[0].rstrip(".") + "."
                break
        return raw.strip()

    def summarize_run(self, run_log: str) -> str:
        """2–3 sentence verdict over an entire run. Pure text — no images."""
        # Trim to ~3000 chars so the model context isn't blown.
        log = run_log[-3000:]
        try:
            resp = self._client.chat(
                model=self.model,
                options={**self._options, "num_predict": 300},
                messages=[{"role": "user", "content": SUMMARY_PROMPT.format(log=log)}],
            )
            return (resp.get("message") or {}).get("content", "").strip()
        except Exception as e:
            return f"(AI summary unavailable: {e})"

    def ping(self) -> bool:
        try:
            self._client.list()
            return True
        except Exception:
            return False
