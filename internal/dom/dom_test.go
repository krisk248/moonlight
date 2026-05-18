package dom

import (
	"strings"
	"testing"

	"github.com/krisk248/moonlight/internal/scenario"
)

func TestEvaluateStrings_NilCheckIsPass(t *testing.T) {
	got := EvaluateStrings("body", "https://x.com/", nil)
	if !got.Passed {
		t.Errorf("nil check should pass, got %+v", got)
	}
}

func TestEvaluateStrings_ContainsText(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
		pass bool
	}{
		{"all present", "Welcome back, dashboard ready", []string{"Welcome", "dashboard"}, true},
		{"one missing", "Welcome back", []string{"Welcome", "dashboard"}, false},
		{"empty list passes", "anything", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &scenario.DOMCheck{ContainsText: tt.want}
			got := EvaluateStrings(tt.body, "", c)
			if got.Passed != tt.pass {
				t.Errorf("Passed = %v, want %v (details: %v)", got.Passed, tt.pass, got.Details)
			}
		})
	}
}

func TestEvaluateStrings_URLContains(t *testing.T) {
	pass := EvaluateStrings("", "https://app.example.com/dashboard?ref=x",
		&scenario.DOMCheck{URLContains: "/dashboard"})
	if !pass.Passed {
		t.Errorf("url_contains should match: %v", pass.Details)
	}

	fail := EvaluateStrings("", "https://app.example.com/login",
		&scenario.DOMCheck{URLContains: "/dashboard"})
	if fail.Passed {
		t.Errorf("url_contains should NOT match for /login")
	}
}

func TestEvaluateStrings_URLMatches(t *testing.T) {
	pass := EvaluateStrings("", "https://app.example.com/users/42",
		&scenario.DOMCheck{URLMatches: `/users/\d+$`})
	if !pass.Passed {
		t.Errorf("url_matches regex should match: %v", pass.Details)
	}

	fail := EvaluateStrings("", "https://app.example.com/users/abc",
		&scenario.DOMCheck{URLMatches: `/users/\d+$`})
	if fail.Passed {
		t.Errorf("url_matches should fail for /users/abc")
	}

	bad := EvaluateStrings("", "x", &scenario.DOMCheck{URLMatches: "(["})
	if bad.Passed {
		t.Errorf("invalid regex should fail closed")
	}
	if !strings.Contains(strings.ToLower(bad.Details[0]), "invalid") {
		t.Errorf("expected invalid-regex detail, got %v", bad.Details)
	}
}

func TestEvaluateStrings_MultipleConditions_AllMustPass(t *testing.T) {
	c := &scenario.DOMCheck{
		ContainsText: []string{"OK"},
		URLContains:  "/done",
	}
	got := EvaluateStrings("OK then", "https://x.com/done", c)
	if !got.Passed {
		t.Errorf("both should pass, got details: %v", got.Details)
	}
	got2 := EvaluateStrings("OK then", "https://x.com/elsewhere", c)
	if got2.Passed {
		t.Errorf("one failure should fail the whole check")
	}
}
