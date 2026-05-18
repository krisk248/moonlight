# Moonlight

Automated visual regression testing for web apps. Drives Chromium with
Playwright, captures screenshots, and uses a local vision model
(SmolVLM2-2.2B via Ollama) to add yes/no sanity checks on top of pixel
diffs. The AI flags obvious anomalies (CAPTCHAs, wrong pages, missing
content); pixel diff, browser-event capture, and Playwright assertions
catch the rest.

Operational lifecycle and refresh procedures: see [OPERATIONS.md](OPERATIONS.md).

Works on **Windows 11** and **Linux**. Each tester runs the whole stack on
their own machine; no central server or login needed.

### Honest capabilities (so expectations match reality)

| Capability | Status |
|---|---|
| Records browser flows reliably | yes |
| Replays them deterministically | yes |
| Detects pixel-level visual regressions | yes |
| Captures browser console / network / JS errors | yes |
| Flags obvious wrong-page failures via the AI | yes |
| Catches subtle UI bugs the way a human would | **no** — the model is small (2.2B); use for sanity checks, not as your only QA layer |
| Reads on-screen text reliably (OCR) | **no** — hallucinates HTML at Q8; switch to FP16 if OCR is critical |
| Replaces human QA | **no** |

---

## Windows 11 — Quick start

> Tested on Windows 11 with PowerShell 7+. You need `winget` (App Installer
> from the Microsoft Store) and ~6 GB of free disk space for the model and
> Chromium.

```powershell
# 1. Clone
git clone https://github.com/<your-org>/moonlight
cd moonlight

# 2. Install (one command — sets up Python, uv, Ollama, the model, Chromium)
.\install\windows\install.ps1

# 3. Start the dashboard
.\install\windows\start.ps1
```

A browser opens at <http://127.0.0.1:8765>. That's it.

If anything goes wrong, the install script writes a full transcript to
`logs\install-<timestamp>.log`. Send that to the team.

To stop: `.\install\windows\stop.ps1`

---

## Linux — Quick start

```bash
git clone https://github.com/<your-org>/moonlight
cd moonlight
./install/linux/install.sh
./install/linux/start.sh
```

Open <http://127.0.0.1:8765>. Same as Windows.

---

## What it does

Once the dashboard is up:

1. **Click `+ Record Scenario`**, type a name and your app's URL, tick
   *Save login state* if your app needs sign-in.
2. **A Chromium window opens on your desktop.** Click through your test
   flow — login, navigate, do whatever you want to verify. Close the
   window when you're done.
3. **The scenario YAML is generated** with placeholder AI prompts at every
   screenshot step. Open the scenario and replace each `TODO: …` with a
   specific yes/no question, e.g. *"Is the user logged in viewing the
   dashboard with no error messages?"*
4. **Click `Baseline`** — Moonlight replays your recording silently and
   saves the screenshots as the "correct" reference.
5. **Click `Run`** — every time you want to check for regressions. The AI
   compares each screenshot to its baseline, answers your yes/no question,
   captures browser console + network errors, and writes a full report.

The report shows for every step:
- baseline / current / diff thumbnails side-by-side
- a one-sentence AI description of what's on screen
- yes/no AI verdicts with explanation
- browser console messages, failed network requests, JS errors
- an AI summary at the top describing the whole run in plain English

---

## What's bundled

| Scenario | What it tests |
|---|---|
| `demo-example` | Smoke test against example.com. Always passes. |
| `google-tts-demo` | Searches Wikipedia for "Text-to-speech" and verifies the article loads. Uses Wikipedia instead of Google because Google blocks headless browsers — and the AI correctly caught that, which is documented in the repo's history. |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ Your machine                                                 │
│                                                              │
│   Moonlight (Python + FastAPI)        :8765                  │
│     ├── runs Playwright → Chromium                           │
│     ├── sends screenshots to Ollama for verification         │
│     └── writes runs/<timestamp>/report.html + report.json    │
│                                                              │
│   Ollama (separate process)           :11434                 │
│     └── serves the SmolVLM2 2.2B vision model                │
└──────────────────────────────────────────────────────────────┘
```

- **Chromium (Playwright)** is the "hands and eyes" — clicks, types, screenshots.
- **SmolVLM2** is the "QA brain" — looks at screenshots and judges if they're correct.
- Browser events (console, network, page errors) are also captured per step for debugging.

---

## CLI alternative (if you prefer the terminal)

```bash
uv run moonlight serve          # same as start.ps1 / start.sh
uv run moonlight record URL NAME
uv run moonlight baseline NAME
uv run moonlight run NAME
uv run moonlight run --all
uv run moonlight selftest
```

---

## Logging

Every script writes to the `logs/` folder (gitignored):

- `logs/install-<ts>.log` — full transcript of the installer
- `logs/start-<ts>.log` — what `start` did
- `logs/runtime-<ts>.log` — Moonlight's own stdout/stderr while running
- `logs/ollama-serve.log` — Ollama server output
- `runs/<ts>/report.html` — per-run AI-annotated report
- `runs/<ts>/report.json` — same data, machine-readable

When something breaks, attaching the relevant log usually pinpoints it.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| Installer says `winget not found` | Open Microsoft Store → install *App Installer*, then re-run. |
| Dashboard page shows but `Run` errors with "ollama unreachable" | Ollama isn't running. `start.ps1` should start it; try running it manually with `ollama serve` in a new terminal. |
| `Run` fails with "Unexpected token … name=" in selector | Old recording with a broken selector. The runner auto-migrates these in memory; if you still see it, check that you're on the latest version. |
| Tests fail because the AI says NO | Open the run report, read the **AI sees** line and the **AI looked at the screen** verdict. Often you just need to tighten the `ai_check.prompt` in the YAML. |
| Google or DuckDuckGo blocked with CAPTCHA | Expected — they block headless browsers. The AI correctly says NO. Use Wikipedia or your own app for tests. |

---

## Updating

```bash
git pull
# Windows
.\install\windows\install.ps1   # re-runs uv sync, refreshes deps
# Linux
./install/linux/install.sh
```

Your scenarios, baselines, and saved login state are preserved across updates.

---

## License

MIT — see [LICENSE](LICENSE).
