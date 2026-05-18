// Package dom evaluates DOMCheck assertions against a live Playwright page.
// All assertions are deterministic — no AI involved — so scenarios that use
// only dom_check blocks can run with no Ollama dependency.
package dom

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/playwright-community/playwright-go"

	"github.com/krisk248/moonlight/internal/scenario"
)

// Result is the outcome of evaluating a DOMCheck.
type Result struct {
	Passed  bool
	Details []string // one line per sub-assertion, indicating pass/fail
}

// Evaluate runs every populated field of the DOMCheck against the page.
// Returns Passed=false on the first failure; collects detail lines for all
// fields it touches so reports can show what was checked.
func Evaluate(page playwright.Page, check *scenario.DOMCheck) Result {
	if check == nil {
		return Result{Passed: true}
	}
	var details []string
	passed := true

	// contains_text — every required string must appear in the page body
	if len(check.ContainsText) > 0 {
		body, err := page.TextContent("body")
		if err != nil {
			passed = false
			details = append(details, fmt.Sprintf("contains_text: ERROR reading body — %v", err))
		} else {
			missing := []string{}
			for _, want := range check.ContainsText {
				if !strings.Contains(body, want) {
					missing = append(missing, want)
				}
			}
			if len(missing) == 0 {
				details = append(details, fmt.Sprintf("contains_text: all %d strings found", len(check.ContainsText)))
			} else {
				passed = false
				details = append(details, fmt.Sprintf("contains_text: MISSING %v", missing))
			}
		}
	}

	if check.SelectorVisible != "" {
		visible, err := isVisible(page, check.SelectorVisible)
		if err != nil || !visible {
			passed = false
			details = append(details, fmt.Sprintf("selector_visible: %q NOT visible", check.SelectorVisible))
		} else {
			details = append(details, fmt.Sprintf("selector_visible: %q visible", check.SelectorVisible))
		}
	}

	if check.SelectorHidden != "" {
		visible, err := isVisible(page, check.SelectorHidden)
		if err == nil && visible {
			passed = false
			details = append(details, fmt.Sprintf("selector_hidden: %q IS visible (should not be)", check.SelectorHidden))
		} else {
			details = append(details, fmt.Sprintf("selector_hidden: %q not visible", check.SelectorHidden))
		}
	}

	if check.URLContains != "" {
		got := page.URL()
		if strings.Contains(got, check.URLContains) {
			details = append(details, fmt.Sprintf("url_contains: %q present", check.URLContains))
		} else {
			passed = false
			details = append(details, fmt.Sprintf("url_contains: %q NOT in %q", check.URLContains, got))
		}
	}

	if check.URLMatches != "" {
		got := page.URL()
		re, err := regexp.Compile(check.URLMatches)
		if err != nil {
			passed = false
			details = append(details, fmt.Sprintf("url_matches: invalid regex %q — %v", check.URLMatches, err))
		} else if re.MatchString(got) {
			details = append(details, fmt.Sprintf("url_matches: %q matched", check.URLMatches))
		} else {
			passed = false
			details = append(details, fmt.Sprintf("url_matches: %q NOT match %q", check.URLMatches, got))
		}
	}

	return Result{Passed: passed, Details: details}
}

func isVisible(page playwright.Page, selector string) (bool, error) {
	loc := page.Locator(selector)
	count, err := loc.Count()
	if err != nil || count == 0 {
		return false, err
	}
	return loc.First().IsVisible()
}

// EvaluateStrings is a pure helper used by tests. It runs the same logic as
// Evaluate but against pre-supplied inputs, sidestepping the need for a live
// page. Browser-bound assertions (selector_visible/hidden) are skipped here.
func EvaluateStrings(body, currentURL string, check *scenario.DOMCheck) Result {
	if check == nil {
		return Result{Passed: true}
	}
	var details []string
	passed := true

	if len(check.ContainsText) > 0 {
		missing := []string{}
		for _, want := range check.ContainsText {
			if !strings.Contains(body, want) {
				missing = append(missing, want)
			}
		}
		if len(missing) == 0 {
			details = append(details, fmt.Sprintf("contains_text: all %d strings found", len(check.ContainsText)))
		} else {
			passed = false
			details = append(details, fmt.Sprintf("contains_text: MISSING %v", missing))
		}
	}

	if check.URLContains != "" {
		if strings.Contains(currentURL, check.URLContains) {
			details = append(details, fmt.Sprintf("url_contains: %q present", check.URLContains))
		} else {
			passed = false
			details = append(details, fmt.Sprintf("url_contains: %q NOT in %q", check.URLContains, currentURL))
		}
	}

	if check.URLMatches != "" {
		re, err := regexp.Compile(check.URLMatches)
		if err != nil {
			passed = false
			details = append(details, fmt.Sprintf("url_matches: invalid regex %v", err))
		} else if re.MatchString(currentURL) {
			details = append(details, fmt.Sprintf("url_matches: %q matched", check.URLMatches))
		} else {
			passed = false
			details = append(details, fmt.Sprintf("url_matches: %q NOT match %q", check.URLMatches, currentURL))
		}
	}

	return Result{Passed: passed, Details: details}
}
