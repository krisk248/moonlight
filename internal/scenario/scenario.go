// Package scenario defines the YAML schema and loading/saving for test
// scenarios. Each scenario is a list of browser actions; screenshot steps
// may carry one or both of:
//
//   - ai_check  → asks the local SmolVLM2 model a yes/no question
//   - dom_check → deterministic DOM assertions (text, visibility, URL)
//
// A scenario that uses only dom_check runs entirely without Ollama.
package scenario

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Scenario is a single recorded test flow.
type Scenario struct {
	Name         string   `yaml:"name"`
	URL          string   `yaml:"url"`
	Viewport     Viewport `yaml:"viewport,omitempty"`
	Headless     *bool    `yaml:"headless,omitempty"`
	StorageState string   `yaml:"storage_state,omitempty"`
	Steps        []Step   `yaml:"steps"`
	Tags         []string `yaml:"tags,omitempty"`
}

// Viewport is the browser window size for the run.
type Viewport struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

// Step is one browser action plus optional verifications.
type Step struct {
	Action   string    `yaml:"action"`
	Name     string    `yaml:"name,omitempty"`
	URL      string    `yaml:"url,omitempty"`
	Selector string    `yaml:"selector,omitempty"`
	Role     string    `yaml:"role,omitempty"`
	RoleName string    `yaml:"role_name,omitempty"`
	Label    string    `yaml:"label,omitempty"`
	Text     string    `yaml:"text,omitempty"`
	Value    string    `yaml:"value,omitempty"`
	Key      string    `yaml:"key,omitempty"`
	MS       int       `yaml:"ms,omitempty"`
	Y        int       `yaml:"y,omitempty"`
	FullPage bool      `yaml:"full_page,omitempty"`
	AICheck  *AICheck  `yaml:"ai_check,omitempty"`
	DOMCheck *DOMCheck `yaml:"dom_check,omitempty"`
}

// AICheck is the SmolVLM2 yes/no assertion. Skipped at runtime if Ollama is
// unreachable, the model isn't loaded, or the scenario was launched in basic
// mode — failures in this layer never crash a deterministic test.
type AICheck struct {
	Prompt string `yaml:"prompt"`
	Expect string `yaml:"expect"` // "yes" / "no" / "true" / "false"
}

// DOMCheck is a set of deterministic assertions. All listed conditions must
// be true for the check to PASS. Any single failure marks the check failed.
type DOMCheck struct {
	ContainsText    []string `yaml:"contains_text,omitempty"`    // page body must contain every string
	SelectorVisible string   `yaml:"selector_visible,omitempty"` // this CSS selector must resolve to a visible element
	SelectorHidden  string   `yaml:"selector_hidden,omitempty"`  // this selector must NOT be visible (may exist hidden / not exist)
	URLContains     string   `yaml:"url_contains,omitempty"`     // page URL must contain this substring
	URLMatches      string   `yaml:"url_matches,omitempty"`      // page URL must match this regex
}

// NeedsAI returns true if any step in the scenario carries an AICheck.
// The runner uses this to decide whether to connect to Ollama at all.
func (s *Scenario) NeedsAI() bool {
	for _, st := range s.Steps {
		if st.AICheck != nil {
			return true
		}
	}
	return false
}

// Load reads a YAML file into a Scenario.
func Load(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Scenario
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes a Scenario as YAML.
func Save(s *Scenario, path string) error {
	b, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// ListDir returns all *.yaml files in a directory as Scenarios.
func ListDir(dir string) ([]*Scenario, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*Scenario
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		s, err := Load(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
