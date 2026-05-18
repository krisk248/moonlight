package runner

import "testing"

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"login-flow", "login-flow"},
		{"after click 3", "after-click-3"},
		{"emoji 🎉 here", "emoji---here"},
		{"---trim---", "trim"},
	}
	for _, tt := range tests {
		if got := slug(tt.in); got != tt.want {
			t.Errorf("slug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildSummaryLog(t *testing.T) {
	steps := []StepResult{
		{Index: 1, Action: "goto", Passed: true},
		{Index: 2, Action: "screenshot", Name: "landing", Passed: true, AINarration: "A login form."},
		{Index: 3, Action: "click", Passed: false, Error: "selector not found"},
	}
	got := buildSummaryLog(steps)
	if !contains(got, "Step 1 [goto] PASS") {
		t.Errorf("missing step 1: %s", got)
	}
	if !contains(got, "A login form.") {
		t.Errorf("missing AI narration: %s", got)
	}
	if !contains(got, "selector not found") {
		t.Errorf("missing error detail: %s", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
