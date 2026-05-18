// Package recorder shells out to `npx playwright codegen` for the actual
// browser recording, then parses the emitted Python source into a YAML
// scenario. Codegen is the official Playwright tool — we don't reinvent it.
//
// Requires Node.js + npm on the tester's machine. If npx is missing, returns
// a clear error pointing at the install instructions.
package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/krisk248/moonlight/internal/scenario"
)

// Options for a recording session.
type Options struct {
	URL          string
	ScenarioName string
	Viewport     scenario.Viewport
	OutputPath   string // where to write the .yaml
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

	tmpFile, err := os.CreateTemp("", "moonlight-codegen-*.py")
	if err != nil {
		return nil, err
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	args := []string{
		"--yes",
		"playwright",
		"codegen",
		"--target", "python-async",
		"--output", tmpFile.Name(),
		fmt.Sprintf("--viewport-size=%d,%d", opts.Viewport.Width, opts.Viewport.Height),
		opts.URL,
	}
	cmd := exec.Command("npx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("codegen failed: %w", err)
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
	rGoto         = regexp.MustCompile(`await page\.goto\("([^"]+)"\)`)
	rRoleClick    = regexp.MustCompile(`await page\.get_by_role\("([^"]+)"(?:,\s*name="([^"]+)")?\)\.click\(\)`)
	rRoleFill     = regexp.MustCompile(`await page\.get_by_role\("([^"]+)"(?:,\s*name="([^"]+)")?\)\.fill\("([^"]*)"\)`)
	rRolePress    = regexp.MustCompile(`await page\.get_by_role\("([^"]+)"(?:,\s*name="([^"]+)")?\)\.press\("([^"]+)"\)`)
	rLabelFill    = regexp.MustCompile(`await page\.get_by_label\("([^"]+)"\)\.fill\("([^"]*)"\)`)
	rLabelClick   = regexp.MustCompile(`await page\.get_by_label\("([^"]+)"\)\.click\(\)`)
	rTextClick    = regexp.MustCompile(`await page\.get_by_text\("([^"]+)"\)\.click\(\)`)
	rPlaceFill    = regexp.MustCompile(`await page\.get_by_placeholder\("([^"]+)"\)\.fill\("([^"]*)"\)`)
	rLocClick     = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.click\(\)`)
	rLocFill      = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.fill\("([^"]*)"\)`)
	rLocSelect    = regexp.MustCompile(`await page\.locator\("([^"]+)"\)\.select_option\("([^"]+)"\)`)
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
			addScreenshot("after-goto")
			continue
		}
		if m := rRolePress.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "press", Role: m[1], RoleName: m[2], Key: m[3]})
			addScreenshot("after-press")
			continue
		}
		if m := rRoleFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Role: m[1], RoleName: m[2], Value: m[3]})
			continue
		}
		if m := rRoleClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Role: m[1], RoleName: m[2]})
			addScreenshot("after-click")
			continue
		}
		if m := rLabelFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Label: m[1], Value: m[2]})
			continue
		}
		if m := rLabelClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Label: m[1]})
			addScreenshot("after-click")
			continue
		}
		if m := rTextClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Text: m[1]})
			addScreenshot("after-click")
			continue
		}
		if m := rPlaceFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Selector: fmt.Sprintf(`[placeholder="%s"]`, m[1]), Value: m[2]})
			continue
		}
		if m := rLocFill.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "fill", Selector: m[1], Value: m[2]})
			continue
		}
		if m := rLocClick.FindStringSubmatch(line); m != nil {
			steps = append(steps, scenario.Step{Action: "click", Selector: m[1]})
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
