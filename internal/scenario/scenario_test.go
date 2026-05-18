package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "demo.yaml")

	want := &Scenario{
		Name: "demo",
		URL:  "https://example.com",
		Viewport: Viewport{Width: 1280, Height: 720},
		Steps: []Step{
			{Action: "goto", URL: "/"},
			{Action: "screenshot", Name: "landing",
				DOMCheck: &DOMCheck{
					ContainsText: []string{"Example Domain"},
					URLContains:  "example.com",
				}},
			{Action: "screenshot", Name: "verified",
				AICheck: &AICheck{Prompt: "Is this the landing page?", Expect: "yes"}},
		},
	}

	if err := Save(want, p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.URL != want.URL {
		t.Errorf("metadata mismatch: got %+v want %+v", got, want)
	}
	if len(got.Steps) != 3 {
		t.Errorf("got %d steps, want 3", len(got.Steps))
	}
	if got.Steps[1].DOMCheck == nil || got.Steps[1].DOMCheck.URLContains != "example.com" {
		t.Errorf("dom_check not round-tripped")
	}
	if got.Steps[2].AICheck == nil || got.Steps[2].AICheck.Expect != "yes" {
		t.Errorf("ai_check not round-tripped")
	}
}

func TestNeedsAI(t *testing.T) {
	tests := []struct {
		name  string
		steps []Step
		want  bool
	}{
		{"no checks", []Step{{Action: "goto", URL: "/"}}, false},
		{"only dom_check", []Step{{Action: "screenshot", DOMCheck: &DOMCheck{ContainsText: []string{"x"}}}}, false},
		{"has ai_check", []Step{{Action: "screenshot", AICheck: &AICheck{Prompt: "ok?"}}}, true},
		{"mixed", []Step{
			{Action: "screenshot", DOMCheck: &DOMCheck{URLContains: "/a"}},
			{Action: "screenshot", AICheck: &AICheck{Prompt: "ok?"}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scenario{Steps: tt.steps}
			if got := s.NeedsAI(); got != tt.want {
				t.Errorf("NeedsAI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListDir(t *testing.T) {
	tmp := t.TempDir()
	// good scenario
	_ = os.WriteFile(filepath.Join(tmp, "a.yaml"), []byte(`name: a
url: https://example.com
steps: []
`), 0o644)
	// not a yaml — should be ignored
	_ = os.WriteFile(filepath.Join(tmp, "readme.txt"), []byte("ignore me"), 0o644)
	// malformed yaml — should be silently skipped
	_ = os.WriteFile(filepath.Join(tmp, "b.yaml"), []byte(":\n  - bad"), 0o644)

	list, err := ListDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "a" {
		names := make([]string, len(list))
		for i, s := range list {
			names[i] = s.Name
		}
		t.Errorf("expected [a], got %v", names)
	}
}
