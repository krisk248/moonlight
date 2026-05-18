# Moonlight

Automated regression testing for web apps, written in Go.

Drives Chromium via Playwright, captures screenshots, and verifies each step
with either:

- **Basic mode** — deterministic DOM assertions (text, visibility, URL). No
  AI involved. Runs without Ollama.
- **AI mode** — augments DOM assertions with a local SmolVLM2-2.2B vision
  model that answers yes/no questions about screenshots. Requires Ollama.

The mode is decided **per scenario** by whether the YAML carries `ai_check:`
blocks. A scenario with only `dom_check:` blocks never contacts Ollama.

---

## Quick start

```bash
# 1. Build
go build -o moonlight ./cmd/moonlight

# 2. Install Playwright browsers (one-time, ~150 MB)
go run github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1 install chromium

# 3. Start the dashboard
./moonlight serve
# → open http://127.0.0.1:8765

# 4. Or run a scenario directly from the terminal
./moonlight baseline demo-basic   # first time — captures reference screenshots
./moonlight run      demo-basic   # subsequent runs — diff against baseline
```

`demo-basic.yaml` uses only `dom_check` — runs with no Ollama installed.

`demo-ai.yaml` uses `ai_check` — requires:

```bash
# Install Ollama (Linux)
curl -fsSL https://ollama.com/install.sh | sh
ollama serve &
ollama pull ahmadwaqar/smolvlm2-2.2b-instruct

# Then:
./moonlight run demo-ai
```

---

## Scenario YAML

```yaml
name: login-smoke
url: https://app.example.com
viewport: {width: 1280, height: 720}
headless: true
steps:
  - action: goto
    url: /login
  - action: fill
    selector: input[name=email]
    value: qa@example.com
  - action: fill
    selector: input[name=password]
    value: <REDACTED>
  - action: click
    selector: button[type=submit]
  - action: wait_for
    selector: .dashboard
  - action: screenshot
    name: dashboard
    dom_check:                            # basic mode — deterministic
      contains_text: ["Welcome back"]
      selector_visible: ".user-menu"
      url_contains: "/dashboard"
    ai_check:                             # AI mode — adds a yes/no judgement
      prompt: "Is the user logged in viewing a dashboard with no error messages?"
      expect: yes
```

### Supported `action` types

`goto`, `click`, `fill`, `select`, `press`, `wait_for`, `wait_ms`, `scroll`,
`screenshot`.

Locators on `click`/`fill`/`press`/`select`/`wait_for` steps: one of
`selector`, `role` (+ optional `role_name`), `label`, `text`.

### Supported `dom_check` fields

| Field | Asserts |
|---|---|
| `contains_text: ["A", "B"]` | Page body contains EVERY string in the list |
| `selector_visible: ".x"` | This selector resolves to a visible element |
| `selector_hidden: ".x"` | This selector is NOT visible (or doesn't exist) |
| `url_contains: "/path"` | The current page URL contains the substring |
| `url_matches: "^https://.*$"` | The current page URL matches the regex |

All listed conditions must pass for the check to succeed.

### `ai_check` fields

| Field | Meaning |
|---|---|
| `prompt: "..."` | Yes/no question for SmolVLM2 |
| `expect: yes \| no` | Expected first-token answer |

---

## Architecture

```
.
├── cmd/moonlight/         # main entry — CLI + embedded HTML status page
│   ├── main.go
│   └── web/index.html     # embedded dashboard (vanilla JS for now)
├── internal/
│   ├── buildinfo/         # stamped at build time (version, build id, kill URL)
│   ├── lifecycle/         # centralized kill switch (last_day from a gist)
│   ├── scenario/          # YAML schema + Load/Save/ListDir
│   ├── browser/           # Playwright wrapper, browser event capture
│   ├── dom/               # deterministic dom_check evaluation
│   ├── vision/            # Ollama client for ai_check / narrate / summarize
│   ├── diff/              # native Go pixel diff with red overlay
│   ├── runner/            # scenario orchestrator
│   └── web/               # HTTP API server (chi router)
└── scenarios/             # YAML scenarios (private by default, gitignored)
```

Every package has a unit test suite. Run with:

```bash
go test ./...
```

Six packages, table-driven tests, HTTP mocks for the Ollama client and the
revocation URL — no live browser or live Ollama needed for the unit suite.

---

## Kill switch (lifecycle)

Each binary has a fetch URL baked in at build time
(`internal/buildinfo.RevocationURL`). On every startup the binary:

1. Refuses to start if `.killed` exists.
2. Source builds (`go run`, plain `go build`) are unrestricted.
3. Stamped builds fetch the URL — expected JSON: `{"last_day": ""}`.
4. If `last_day` is set and today is past it → silently deletes
   `scenarios/*.yaml` + `auth-state/*.json`, writes `.killed`, exits.
5. If the URL is unreachable, last successful response is cached and trusted
   for 24 hours.
6. If unreachable for 30 days straight → silent kill regardless.

To kill all installations org-wide: set `last_day` to yesterday in the
hosted JSON. Within ~5 min (CDN cache), every running binary's next start
will wipe and exit.

To build a stamped binary (replace the URL with yours):

```bash
RURL="https://gist.githubusercontent.com/krisk248/14fec43b1bb1ba9dbbde91728b4c9985/raw/revoked.json"
BUILD_DATE=$(date -u +%Y-%m-%d)
BUILD_ID="ml-$BUILD_DATE-$(head -c4 /dev/urandom | xxd -p)"

go build \
  -ldflags "-X github.com/krisk248/moonlight/internal/buildinfo.BuildDate=$BUILD_DATE \
            -X github.com/krisk248/moonlight/internal/buildinfo.BuildID=$BUILD_ID \
            -X github.com/krisk248/moonlight/internal/buildinfo.RevocationURL=$RURL" \
  -o dist/moonlight ./cmd/moonlight
```

For a Windows .exe from Linux:

```bash
GOOS=windows GOARCH=amd64 go build \
  -ldflags "-X ... " \
  -o dist/moonlight.exe ./cmd/moonlight
```

(See [OPERATIONS.md](OPERATIONS.md) for the operational lifecycle pointer.)

---

## What's intentionally NOT yet in this build

This is the Go rewrite's first cut. The following are TODO and tracked for
follow-up sessions:

- **Svelte dashboard** — current frontend is a minimal embedded HTML page
  for status verification. SvelteKit UI is planned.
- **Recorder** — `playwright codegen` integration to auto-generate YAML
  from a browser session. Currently you author YAML by hand.
- **Login state persistence** — `storage_state:` field works, but no UX yet
  for capturing the file at record time.
- **GitHub Actions release pipeline** — local builds work; CI pipeline
  TBD.

---

## Honest capabilities

| Capability | Status |
|---|---|
| Records browser flows reliably | Manual YAML for now; codegen integration TODO |
| Replays scenarios deterministically | yes |
| Detects pixel-level visual regressions | yes |
| Captures browser console / network / JS errors | yes |
| Deterministic DOM assertions (no AI) | yes |
| Flags obvious wrong-page failures via AI | yes (when `ai_check` is used) |
| Catches subtle UI bugs the way a human would | **no** — SmolVLM2 is small; use AI for sanity checks, not as primary QA |
| Replaces human QA | **no** |

---

## License

MIT — see [LICENSE](LICENSE).
