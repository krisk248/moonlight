// Package runner executes a scenario start-to-finish: drives the browser
// step by step, runs DOM and (optionally) AI assertions on screenshots, and
// returns a RunResult.
//
// Modes:
//   - ModeBaseline → screenshots are copied into baselineDir; no assertions run.
//   - ModeRun      → screenshots are compared against baseline (pixel diff),
//                    DOM assertions evaluated, AI assertions evaluated IF the
//                    scenario carries any ai_check and the vision client is
//                    available.
package runner

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/krisk248/moonlight/internal/browser"
	"github.com/krisk248/moonlight/internal/diff"
	"github.com/krisk248/moonlight/internal/dom"
	"github.com/krisk248/moonlight/internal/scenario"
	"github.com/krisk248/moonlight/internal/vision"
)

type Mode string

const (
	ModeBaseline Mode = "baseline"
	ModeRun      Mode = "run"
)

// StepResult is the outcome of one step.
type StepResult struct {
	Index          int                    `json:"index"`
	Action         string                 `json:"action"`
	Name           string                 `json:"name"`
	Passed         bool                   `json:"passed"`
	Error          string                 `json:"error,omitempty"`
	ScreenshotPath string                 `json:"screenshot,omitempty"`
	BaselinePath   string                 `json:"baseline,omitempty"`
	DiffOverlay    string                 `json:"diff_overlay,omitempty"`
	DiffRatio      float64                `json:"diff_ratio,omitempty"`
	AINarration    string                 `json:"ai_narration,omitempty"`
	Checks         []CheckResult          `json:"checks,omitempty"`
	Console        []browser.ConsoleEntry `json:"console_messages,omitempty"`
	Network        []browser.NetworkEntry `json:"failed_requests,omitempty"`
	PageErrors     []string               `json:"page_errors,omitempty"`
}

type CheckResult struct {
	Kind   string `json:"kind"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// RunResult is the outcome of a whole scenario run.
type RunResult struct {
	Scenario   string       `json:"scenario"`
	URL        string       `json:"url"`
	StartedAt  string       `json:"started_at"`
	FinishedAt string       `json:"finished_at"`
	Passed     bool         `json:"passed"`
	Mode       string       `json:"mode"`
	AIUsed     bool         `json:"ai_used"`
	Steps      []StepResult `json:"steps"`
	RunDir     string       `json:"run_dir"`
	AISummary  string       `json:"ai_summary,omitempty"`
}

// Opts groups dependencies passed into Run.
type Opts struct {
	ProjectRoot       string
	BaselineDir       string
	RunsDir           string
	Vision            *vision.Client // may be nil; consulted only if scenario needs AI
	Headless          bool
	DiffTolerance     uint8 // 0..255 channel-delta threshold per pixel; 12 is a reasonable default
	DefaultTimeoutMS  int   // global Playwright timeout per action; 0 → 15000
	OnLog             func(string)
}

// VisionClient narrows the vision.Client API to what the runner needs so
// tests can pass a mock. (Currently unused in tests; exported for future use.)
type VisionClient interface {
	FunctionalCheck(imagePath, question string, expectYes bool) (vision.Verdict, error)
	Narrate(imagePath string) (string, error)
	SummarizeRun(runLog string) (string, error)
	Ping() bool
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// Run executes a scenario.
func Run(s *scenario.Scenario, mode Mode, opts Opts) (*RunResult, error) {
	if opts.DiffTolerance == 0 {
		opts.DiffTolerance = 12
	}

	started := time.Now().UTC()
	stamp := started.Format("20060102-150405")
	runDir := filepath.Join(opts.RunsDir, fmt.Sprintf("%s-%s", stamp, s.Name))
	baselineDir := filepath.Join(opts.BaselineDir, s.Name)

	log := opts.OnLog
	if log == nil {
		log = func(string) {}
	}

	// Per-scenario decision: do we even talk to the AI?
	useAI := mode == ModeRun && s.NeedsAI() && opts.Vision != nil && opts.Vision.Ping()
	if mode == ModeRun && s.NeedsAI() && !useAI {
		log("[ai] scenario requests ai_check but vision is unavailable — skipping AI assertions")
	}

	log(fmt.Sprintf("[start] mode=%s url=%s steps=%d ai=%v", mode, s.URL, len(s.Steps), useAI))

	headless := opts.Headless
	if s.Headless != nil {
		headless = *s.Headless
	}

	storageStatePath := ""
	if s.StorageState != "" {
		storageStatePath = filepath.Join(opts.ProjectRoot, s.StorageState)
	}

	timeoutMS := opts.DefaultTimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 15000
	}
	br, err := browser.New(browser.Opts{
		BaseURL:          s.URL,
		Viewport:         s.Viewport,
		Headless:         headless,
		DefaultTimeoutMS: timeoutMS,
		StorageStatePath: storageStatePath,
	})
	if err != nil {
		return nil, err
	}
	defer br.Close()

	var steps []StepResult
	for i, step := range s.Steps {
		idx := i + 1
		name := step.Name
		if step.Action != "screenshot" || name == "" {
			name = fmt.Sprintf("%s-%02d", step.Action, idx)
		}
		log(fmt.Sprintf("[step %02d] %s", idx, step.Action))

		res := StepResult{Index: idx, Action: step.Action, Name: name, Passed: true}

		// Per-step timeout override — restored after the action.
		if step.TimeoutMS > 0 {
			br.SetStepTimeout(step.TimeoutMS)
		}

		var stepErr error
		switch step.Action {
		case "goto":
			stepErr = br.Goto(step.URL)
		case "click":
			stepErr = br.Click(step)
		case "fill":
			stepErr = br.Fill(step)
		case "select":
			stepErr = br.SelectOption(step)
		case "press":
			stepErr = br.Press(step)
		case "wait_for":
			stepErr = br.WaitFor(step)
		case "wait_ms":
			br.WaitMS(step.MS)
		case "wait_for_networkidle":
			stepErr = br.WaitForNetworkidle()
		case "upload_file":
			stepErr = br.Upload(step)
		case "download_file":
			stepErr = br.Download(step)
		case "scroll":
			stepErr = br.Scroll(step.Y)
		case "screenshot":
			shot := filepath.Join(runDir, "screenshots", fmt.Sprintf("%02d-%s.png", idx, slug(name)))
			stepErr = br.Screenshot(shot, step.FullPage)
			if stepErr == nil {
				res.ScreenshotPath = shot
				if mode == ModeBaseline {
					base := filepath.Join(baselineDir, fmt.Sprintf("%02d-%s.png", idx, slug(name)))
					if cperr := copyFile(shot, base); cperr == nil {
						res.BaselinePath = base
					}
				} else {
					// pixel diff against baseline
					base := filepath.Join(baselineDir, fmt.Sprintf("%02d-%s.png", idx, slug(name)))
					if fileExists(base) {
						overlay := filepath.Join(runDir, "diffs", fmt.Sprintf("%02d-%s.png", idx, slug(name)))
						if d, derr := diff.Diff(base, shot, overlay, opts.DiffTolerance); derr == nil {
							res.BaselinePath = base
							res.DiffOverlay = overlay
							res.DiffRatio = d.Ratio
						}
					}

					// dom_check (deterministic, always runs)
					if step.DOMCheck != nil {
						domRes := dom.Evaluate(br.Page(), step.DOMCheck)
						cr := CheckResult{
							Kind:   "dom_check",
							Passed: domRes.Passed,
							Detail: strings.Join(domRes.Details, " | "),
						}
						res.Checks = append(res.Checks, cr)
						if !cr.Passed {
							res.Passed = false
						}
					}

					// ai_check (optional)
					if useAI && step.AICheck != nil {
						q := step.AICheck.Prompt
						if !strings.Contains(strings.ToUpper(q), "TODO") {
							expectYes := strings.EqualFold(step.AICheck.Expect, "yes") ||
								strings.EqualFold(step.AICheck.Expect, "true")
							v, verr := opts.Vision.FunctionalCheck(shot, q, expectYes)
							cr := CheckResult{
								Kind:   "ai_check",
								Passed: v.Passed,
								Detail: fmt.Sprintf("expected=%s | model: %s", yesOrNo(expectYes), v.Raw),
							}
							if verr != nil {
								cr.Detail = "AI error: " + verr.Error()
							}
							res.Checks = append(res.Checks, cr)
							if !cr.Passed {
								res.Passed = false
							}
						}
						if narr, nerr := opts.Vision.Narrate(shot); nerr == nil {
							res.AINarration = narr
						}
					}
				}
			}
		default:
			stepErr = fmt.Errorf("unknown action: %s", step.Action)
		}
		if stepErr != nil {
			res.Passed = false
			res.Error = stepErr.Error()
		}

		// Restore the global timeout for the next step.
		if step.TimeoutMS > 0 {
			br.ResetTimeout(timeoutMS)
		}

		cons, net, errs := br.SnapshotEvents()
		res.Console = cons
		res.Network = net
		res.PageErrors = errs
		steps = append(steps, res)
	}

	overall := true
	for _, st := range steps {
		if !st.Passed {
			overall = false
			break
		}
	}

	rr := &RunResult{
		Scenario:   s.Name,
		URL:        s.URL,
		StartedAt:  started.Format(time.RFC3339),
		FinishedAt: time.Now().UTC().Format(time.RFC3339),
		Passed:     overall,
		Mode:       string(mode),
		AIUsed:     useAI,
		Steps:      steps,
		RunDir:     runDir,
	}

	if useAI {
		summary, _ := opts.Vision.SummarizeRun(buildSummaryLog(steps))
		rr.AISummary = summary
	}
	return rr, nil
}

func yesOrNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

func buildSummaryLog(steps []StepResult) string {
	var b strings.Builder
	for _, s := range steps {
		v := "PASS"
		if !s.Passed {
			v = "FAIL"
		}
		fmt.Fprintf(&b, "Step %d [%s] %s", s.Index, s.Action, v)
		if s.AINarration != "" {
			fmt.Fprintf(&b, " — %s", s.AINarration)
		}
		if s.Error != "" {
			fmt.Fprintf(&b, " — error: %s", s.Error)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func fileExists(path string) bool {
	_, err := osStat(path)
	return err == nil
}
