// Package recorder shells out to `npx playwright codegen` for the actual
// browser recording, then parses the emitted Python source into a YAML
// scenario. Codegen is the official Playwright tool — we don't reinvent it.
//
// Requires Node.js + npm on the tester's machine. If npx is missing, returns
// a clear error pointing at the install instructions.
package recorder

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/krisk248/moonlight/internal/scenario"
)

// PlaywrightVersion pins npx's Playwright to the same release Playwright-Go
// v0.5700.1 tracks. This avoids a third Chromium being downloaded.
const PlaywrightVersion = "1.57.0"

// Options for a recording session.
type Options struct {
	URL          string
	ScenarioName string
	Viewport     scenario.Viewport
	OutputPath   string // where to write the .yaml
	OnLog        func(string) // optional — receives lines of npx stdout/stderr live
}

// Record launches `npx playwright codegen` (which opens a Chromium window on
// the operator's desktop), waits for them to close it, and persists the
// recorded session as a YAML scenario at opts.OutputPath.
func Record(opts Options) (*scenario.Scenario, error) {
	if _, err := exec.LookPath("npx"); err != nil {
		return nil, fmt.Errorf("npx not found. Install Node.js + npm, then retry. (https://nodejs.org/)")
	}
	if opts.Viewport.Width == 0 {
		opts.Viewport = scenario.Viewport{Width: 1280, Height: 720}
	}
	log := opts.OnLog
	if log == nil {
		log = func(string) {}
	}

	tmpFile, err := os.CreateTemp("", "moonlight-codegen-*.py")
	if err != nil {
		return nil, err
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// `playwright@VER` pins the JS package to the same release Playwright-Go
	// uses, so npx doesn't yank a different Chromium version into the cache.
	args := []string{
		"--yes",
		"playwright@" + PlaywrightVersion,
		"codegen",
		"--target", "python-async",
		"--output", tmpFile.Name(),
		fmt.Sprintf("--viewport-size=%d,%d", opts.Viewport.Width, opts.Viewport.Height),
		opts.URL,
	}

	// Capture stderr so a useful message surfaces when codegen fails.
	var stderrBuf bytes.Buffer
	cmd := exec.Command("npx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	log(fmt.Sprintf("[record] $ npx %s", strings.Join(args, " ")))
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		// Trim very long stderr to the first ~600 chars so the UI stays readable.
		if len(msg) > 600 {
			msg = msg[:600] + "…"
		}
		return nil, fmt.Errorf("codegen failed:\n%s", msg)
	}

	source, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}

	base, steps := parseCodegen(string(source))
	if base == "" {
		return nil, fmt.Errorf("no goto() detected in recording")
	}

	sc := &scenario.Scenario{
		Name:     opts.ScenarioName,
		URL:      base,
		Viewport: opts.Viewport,
		Steps:    steps,
	}
	if opts.OutputPath != "" {
		if err := scenario.Save(sc, opts.OutputPath); err != nil {
			return nil, fmt.Errorf("save yaml: %w", err)
		}
	}
	return sc, nil
}

// ---------- parser ---------------------------------------------------------

var (
	rGoto       = regexp.MustCompile(`await page\.goto\("([^"]+)"\)`)
	rRoleClick  = regexp.MustCompile(`await page\.get_by_role\("([^"]+)"(?:,\s*name="([^"]+)")?\)\.click\(\)`)
	rRoleFill   = regexp.MustCompile(`await page\.get_by_role\("([^"]+)"(?:,\s*name="([^"]+)")?\)\.fill\("([^"]*)"\)`)
	rRolePress  = regexp.MustCompile(`await page\.get_by_role\("([^"]+)"(?:,\s*name="([^"]+)")?\)\.press\("([^"]+)"\)`)
	rLabelFill  = regexp.MustCompile(`await page\.get_by_label\("([^"]+)"\)\.fill\("([^"]*)"\)`)
	rLabelClick = regexp.MustCompile(`await page\.get_by_label\("([^"]+)"\)\.click\(\)`)
	rTextClick  = regexp.MustCompile(`await page\.get_by_text\("([^"]+)"\)\.click\(\)`)
	rPlaceFill  = regexp.MustCompile(`await page\.get_by_placeholder\("([^"]+)"\)\.fill\("([^"]*)"\)`)
	rLocClick   = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.click\(\)`)
	rLocFill    = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.fill\("([^"]*)"\)`)
	rLocSelect  = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.select_option\("([^"]+)"\)`)
	// File upload: page.locator("input[type=file]").set_input_files("/path/file")
	rLocUpload = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.set_input_files\("([^"]+)"\)`)
)

func parseCodegen(source string) (baseURL string, steps []scenario.Step) {
	placeholderIdx := 0
	addScreenshot := func(hint string) {
		placeholderIdx++
		steps = append(steps, scenario.Step{
			Action: "screenshot",
			Name:   fmt.Sprintf("%s-%02d", hint, placeholderIdx),
			DOMCheck: &scenario.DOMCheck{
				URLContains: "", // tester can fill in
			},
			AICheck: &scenario.AICheck{
				Prompt: "TODO: describe what should be visible here",
				Expect: "yes",
			},
		})
	}
	// addWait drops an explicit "wait for the page to settle" step in
	// between navigation-y actions. This is the main remedy for the
	// "timeout 30000ms exceeded" failures users hit when an SPA needs a
	// moment to render the next element they clicked.
	addWait := func() {
		steps = append(steps, scenario.Step{Action: "wait_for_networkidle"})
	}

	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "await ") {
			continue
		}

		if m := rGoto.FindStringSubmatch(line); m != nil {
			url := m[1]
			if baseURL == "" {
				baseURL = url
				steps = append(steps, scenario.Step{Action: "goto", URL: "/"})
			} else {
				steps = append(steps, scenario.Step{Action: "goto", URL: url})
			}
			addWait()
			addScreenshot("after-goto")
			continue
		}
		if m := rRolePress.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "press", Role: m[1], RoleName: m[2], Key: m[3]})
			addWait()
			addScreenshot("after-press")
			continue
		}
		if m := rRoleFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Role: m[1], RoleName: m[2], Value: m[3]})
			continue
		}
		if m := rRoleClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Role: m[1], RoleName: m[2]})
			addWait()
			addScreenshot("after-click")
			continue
		}
		if m := rLabelFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Label: m[1], Value: m[2]})
			continue
		}
		if m := rLabelClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Label: m[1]})
			addWait()
			addScreenshot("after-click")
			continue
		}
		if m := rTextClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Text: m[1]})
			addWait()
			addScreenshot("after-click")
			continue
		}
		if m := rPlaceFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Selector: fmt.Sprintf(`[placeholder="%s"]`, m[1]), Value: m[2]})
			continue
		}
		if m := rLocUpload.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "upload_file", Selector: m[1], Path: m[2]})
			continue
		}
		if m := rLocFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Selector: m[1], Value: m[2]})
			continue
		}
		if m := rLocClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Selector: m[1]})
			addWait()
			addScreenshot("after-click")
			continue
		}
		if m := rLocSelect.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "select", Selector: m[1], Value: m[2]})
			continue
		}
	}
	return baseURL, steps
}
