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
	Name         string   `yaml:"name"                    json:"name"`
	URL          string   `yaml:"url"                     json:"url"`
	Viewport     Viewport `yaml:"viewport,omitempty"      json:"viewport,omitempty"`
	Headless     *bool    `yaml:"headless,omitempty"      json:"headless,omitempty"`
	StorageState string   `yaml:"storage_state,omitempty" json:"storage_state,omitempty"`
	Steps        []Step   `yaml:"steps"                   json:"steps"`
	Tags         []string `yaml:"tags,omitempty"          json:"tags,omitempty"`
}

// Viewport is the browser window size for the run.
type Viewport struct {
	Width  int `yaml:"width"  json:"width"`
	Height int `yaml:"height" json:"height"`
}

// Step is one browser action plus optional verifications.
type Step struct {
	Action   string    `yaml:"action"              json:"action"`
	Name     string    `yaml:"name,omitempty"      json:"name,omitempty"`
	URL      string    `yaml:"url,omitempty"       json:"url,omitempty"`
	Selector string    `yaml:"selector,omitempty"  json:"selector,omitempty"`
	Role     string    `yaml:"role,omitempty"      json:"role,omitempty"`
	RoleName string    `yaml:"role_name,omitempty" json:"role_name,omitempty"`
	Label    string    `yaml:"label,omitempty"     json:"label,omitempty"`
	Text     string    `yaml:"text,omitempty"      json:"text,omitempty"`
	Value    string    `yaml:"value,omitempty"     json:"value,omitempty"`
	Key      string    `yaml:"key,omitempty"       json:"key,omitempty"`
	MS       int       `yaml:"ms,omitempty"        json:"ms,omitempty"`
	Y        int       `yaml:"y,omitempty"         json:"y,omitempty"`
	FullPage bool      `yaml:"full_page,omitempty" json:"full_page,omitempty"`

	// upload_file / download_file
	Path   string `yaml:"path,omitempty"    json:"path,omitempty"`    // file to upload (or, for download_file, where to save)
	SaveTo string `yaml:"save_to,omitempty" json:"save_to,omitempty"` // alias of Path for download_file readability

	// Per-step override of the global timeout (in ms). Use for known-slow pages.
	TimeoutMS int `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty"`

	AICheck  *AICheck  `yaml:"ai_check,omitempty"  json:"ai_check,omitempty"`
	DOMCheck *DOMCheck `yaml:"dom_check,omitempty" json:"dom_check,omitempty"`
}

// AICheck is the SmolVLM2 yes/no assertion. Skipped at runtime if Ollama is
// unreachable, the model isn't loaded, or the scenario was launched in basic
// mode — failures in this layer never crash a deterministic test.
type AICheck struct {
	Prompt string `yaml:"prompt" json:"prompt"`
	Expect string `yaml:"expect" json:"expect"`
}

// DOMCheck is a set of deterministic assertions. All listed conditions must
// be true for the check to PASS. Any single failure marks the check failed.
type DOMCheck struct {
	ContainsText    []string `yaml:"contains_text,omitempty"    json:"contains_text,omitempty"`
	SelectorVisible string   `yaml:"selector_visible,omitempty" json:"selector_visible,omitempty"`
	SelectorHidden  string   `yaml:"selector_hidden,omitempty"  json:"selector_hidden,omitempty"`
	URLContains     string   `yaml:"url_contains,omitempty"     json:"url_contains,omitempty"`
	URLMatches      string   `yaml:"url_matches,omitempty"      json:"url_matches,omitempty"`
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
