"""Playwright wrapper — async context-managed Chromium session for scenarios."""
from __future__ import annotations

from pathlib import Path
from urllib.parse import urljoin

from playwright.async_api import Browser as PWBrowser, Page, async_playwright


class Browser:
    def __init__(
        self,
        base_url: str,
        viewport: dict[str, int] | None = None,
        headless: bool = True,
        default_timeout_ms: int = 15000,
        storage_state: Path | str | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/") + "/"
        self.viewport = viewport or {"width": 1280, "height": 720}
        self.headless = headless
        self.default_timeout_ms = default_timeout_ms
        self.storage_state = Path(storage_state) if storage_state else None
        self._pw = None
        self._browser: PWBrowser | None = None
        self._context = None
        self.page: Page | None = None

    async def __aenter__(self) -> "Browser":
        self._pw = await async_playwright().start()
        self._browser = await self._pw.chromium.launch(headless=self.headless)
        ctx_kwargs = {"viewport": self.viewport}
        if self.storage_state and self.storage_state.exists():
            ctx_kwargs["storage_state"] = str(self.storage_state)
        self._context = await self._browser.new_context(**ctx_kwargs)
        self._context.set_default_timeout(self.default_timeout_ms)
        self.page = await self._context.new_page()

        # Per-step rolling buffers — runner snapshots + clears these around each step.
        self.console_messages: list[dict] = []
        self.failed_requests: list[dict] = []
        self.page_errors: list[str] = []

        self.page.on("console", lambda msg: self.console_messages.append({
            "type": msg.type,
            "text": msg.text[:500],
            "location": f"{msg.location.get('url', '')}:{msg.location.get('lineNumber', '')}",
        }))
        self.page.on("pageerror", lambda err: self.page_errors.append(str(err)[:500]))
        self.page.on("requestfailed", lambda req: self.failed_requests.append({
            "url": req.url[:300],
            "method": req.method,
            "failure": (req.failure or "")[:200],
        }))
        self.page.on("response", self._on_response)
        return self

    def _on_response(self, response):
        # Record 4xx/5xx so the report can flag failed API calls.
        try:
            if response.status >= 400:
                self.failed_requests.append({
                    "url": response.url[:300],
                    "method": response.request.method,
                    "status": response.status,
                })
        except Exception:
            pass

    def snapshot_events(self) -> dict:
        """Return current event buffers and clear them for the next step."""
        snap = {
            "console": list(self.console_messages),
            "failed_requests": list(self.failed_requests),
            "page_errors": list(self.page_errors),
        }
        self.console_messages.clear()
        self.failed_requests.clear()
        self.page_errors.clear()
        return snap

    async def __aexit__(self, exc_type, exc, tb) -> None:
        if self._browser:
            await self._browser.close()
        if self._pw:
            await self._pw.stop()

    def _resolve(self, url: str) -> str:
        if url.startswith(("http://", "https://")):
            return url
        return urljoin(self.base_url, url.lstrip("/"))

    async def goto(self, url: str) -> None:
        try:
            await self.page.goto(self._resolve(url), wait_until="networkidle", timeout=self.default_timeout_ms)
        except Exception:
            # Fall back to load — some pages (Google) keep open connections forever.
            await self.page.goto(self._resolve(url), wait_until="load", timeout=self.default_timeout_ms)

    def _locator(self, step: dict):
        """Resolve a YAML step into a Playwright Locator.

        Step may carry exactly one of: selector, role (+ optional name),
        label, text, placeholder, test_id, alt_text, title.
        """
        exact = bool(step.get("exact", False))
        if step.get("selector"):
            return self.page.locator(step["selector"])
        if step.get("role"):
            kwargs: dict = {}
            if step.get("name") is not None:
                kwargs["name"] = step["name"]
            if exact:
                kwargs["exact"] = True
            return self.page.get_by_role(step["role"], **kwargs)
        if step.get("label") is not None:
            return self.page.get_by_label(step["label"], exact=exact)
        if step.get("text") is not None:
            return self.page.get_by_text(step["text"], exact=exact)
        if step.get("placeholder") is not None:
            return self.page.get_by_placeholder(step["placeholder"], exact=exact)
        if step.get("test_id") is not None:
            return self.page.get_by_test_id(step["test_id"])
        if step.get("alt_text") is not None:
            return self.page.get_by_alt_text(step["alt_text"], exact=exact)
        if step.get("title") is not None:
            return self.page.get_by_title(step["title"], exact=exact)
        raise ValueError(
            f"step has no locator (need one of: selector, role, label, text, placeholder, test_id, alt_text, title): {step!r}"
        )

    async def do_click(self, step: dict) -> None:
        await self._locator(step).click()

    async def do_fill(self, step: dict) -> None:
        await self._locator(step).fill(step.get("value", ""))

    async def do_select(self, step: dict) -> None:
        await self._locator(step).select_option(step.get("value"))

    async def do_press(self, step: dict) -> None:
        await self._locator(step).press(step["key"])

    async def do_wait_for(self, step: dict) -> None:
        await self._locator(step).wait_for()

    async def wait_ms(self, ms: int) -> None:
        await self.page.wait_for_timeout(ms)

    async def scroll(self, y: int) -> None:
        await self.page.evaluate(f"window.scrollBy(0, {int(y)})")

    async def screenshot(self, path: Path, full_page: bool = False) -> Path:
        path.parent.mkdir(parents=True, exist_ok=True)
        await self.page.screenshot(path=str(path), full_page=full_page)
        return path

    async def save_storage_state(self, path: Path) -> Path:
        path.parent.mkdir(parents=True, exist_ok=True)
        await self._context.storage_state(path=str(path))
        return path
