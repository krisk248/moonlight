# Moonlight Roadmap

A living plan. Built honestly, refined against real tester feedback. If
you see a feature here it doesn't mean it'll ship — it means we've thought
about it and decided where it sits.

---

## v1.0 — Shipped (current build)

The version a tester unzips and runs today. Foundation, not ceiling.

- ✅ Cross-platform binary distribution (Linux + Windows amd64, ~10 MB)
- ✅ One-command bootstrap (`install-browsers` then `start.sh`/`start.bat`)
- ✅ Web dashboard (SvelteKit, embedded in the Go binary)
  - Dashboard tab — system status + at-a-glance + latest runs
  - Scenarios tab — list, record, baseline, run, delete
  - Suites tab — scenarios grouped by tag, run a whole suite
  - History tab — every run, grouped by scenario, with pass/fail stats
  - Settings tab — AI on/off, Ollama config, default timeout
- ✅ Recording via `npx playwright codegen` (pinned to v1.57.0 to match the Go binding)
  - Auto-inserts `wait_for_networkidle` between actions
  - Auto-inserts screenshot + AI-check placeholders
- ✅ Replay with full Playwright-Go integration
- ✅ Pixel diff with red-overlay PNG
- ✅ DOM-based assertions: `contains_text`, `selector_visible`, `selector_hidden`, `url_contains`, `url_matches`
- ✅ Optional AI verification (SmolVLM2 via Ollama) — disabled by default, behind Settings toggle
- ✅ File upload + download step actions
- ✅ Per-step and global Playwright timeouts
- ✅ Scenario tags + suite grouping
- ✅ Centralized kill switch via gist `last_day` field (24h cache, 30-day deadman backstop)
- ✅ Structured logging — `logs/moonlight-server-<timestamp>.log`
- ✅ 8 internal Go packages, all unit-tested

**Known limits documented openly:** recording brittleness (rerecord on UI change), no test-data management, no CI runner mode, single-threaded execution, AI vision good for obvious anomalies only (small model).

---

## v1.1 — Next, after beta feedback (~1 week)

Goal: cut the re-record fatigue that kills record-and-replay tools at scale.

| Priority | Feature | Effort | Note |
|----------|---------|--------|------|
| **1** | **Reusable step blocks** | 2 days | Define `login` once, reference from many scenarios. New top-level `blocks:` key, `- action: include, block: login` in steps. |
| **2** | **Per-step retries** | 1 day | `retries: 2` on flaky steps; runner backs off + retries before marking FAIL. Logs each attempt. |
| **3** | **In-browser YAML editor** | 3 days | CodeMirror inside the scenario detail page. Edit selector, save, run — no Notepad needed. |
| 4 | Smarter selector strategy (post-record) | 1 day | Prefer `data-testid` → `aria-label` → `role` → text → CSS when emitting YAML. |

Build only after beta feedback confirms these are the top three complaints. Cherry-pick from the list if real pain points differ.

---

## v1.2 — Within ~1 month if v1.1 adoption looks healthy

Goal: make Moonlight viable for teams of 2–5 testers running tests nightly.

| Feature | Effort | Note |
|---------|--------|------|
| **Environment switching** | 1 day | `--env=staging` flag swaps base URL via config map. Same scenarios on dev/staging/prod. |
| **CI runner mode** | 1 day | `moonlight test --ci --junit-xml=results.xml` returns 0/1, dumps JUnit. Plugs into GitHub Actions, Jenkins, GitLab CI. |
| **Parallel execution** | 2 days | `--parallel=N` opens N Chromium pools. 5× speedup on big suites. |
| **Storage-state UX** | 1 day | One-click "Capture login state" → all dependent scenarios skip the login flow. |
| **Run report exports** | 1 day | Download a run as a single HTML/PDF with screenshots + verdicts inline, share via email. |

---

## v2.0 — Speculative, build only if v1.x success warrants

Goal: graduate from "useful internal tool" to "we'd consider productizing this."

| Feature | Effort | Note |
|---------|--------|------|
| **AI self-healing on selector miss** | 5 days | When a click selector fails, ask SmolVLM2 to find the element by description. Logs warning if it auto-recovers. Behind a Settings toggle. |
| **Project grouping** | 3 days | Multiple "projects" (apps) in one Moonlight install. Each project has its own scenarios/baselines/env config. |
| **Analytics tab** | 4 days | Pass-rate trends, flakiest scenarios, total runs over time. Charts via uPlot or similar. |
| **Selector inspector** | 5 days | Click on an image in the report → highlight where the selector resolved. Helps debug "why did this fail?" |
| **API testing** | 5+ days | A YAML action type for HTTP probes (`action: request`), so a scenario can call `/api/health` then check the UI. |

---

## Design principles (intentional non-goals)

These are **NOT** going to be built. Worth stating openly so we don't churn:

- **No hidden destructive payloads.** The kill switch is documented in OPERATIONS.md by design. We don't ship silent timers or unannounced wipes. (See git history — this was a refused early request.)
- **No per-user accounts/login.** Moonlight runs in a trusted local environment. Adding auth would be cargo-cult.
- **No cloud-hosted SaaS.** Each tester runs the binary locally. Centralization happens via the gist for kill, and via emailing YAML scenarios for sharing.
- **No browser other than Chromium.** Playwright supports Firefox/WebKit, but adding multi-browser doubles every test's flakiness surface. Not justified for an internal tool.
- **No replacement for human QA.** Moonlight catches regressions in known flows. Exploratory testing still needs a human.

---

## How to read this roadmap

- ✅ = shipped in current build
- **Bold rows** = high-confidence, will likely happen
- Plain rows = will happen if real evidence supports it
- v2.0 = aspirational until at least one v1.x release sees adoption

If you're a tester reading this: tell us what hurts. We'll re-prioritize.

If you're a future maintainer: don't build v1.2 before v1.1 ships and v1.1 doesn't ship before v1.0 has at least one week of real-world use. The point of every layer is to learn before building the next.

---

*Maintained as a working document. Last updated when v1.0 shipped.*
