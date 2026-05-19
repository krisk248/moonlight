// Package settings persists user-toggleable runtime preferences to a JSON
// file inside MOONLIGHT_HOME. Currently:
//
//	{ "ai_enabled": false,
//	  "ollama_host": "http://127.0.0.1:11434",
//	  "ollama_model": "ahmadwaqar/smolvlm2-2.2b-instruct" }
//
// The default is AI OFF. Testers who only need deterministic regression
// testing never need to know AI exists.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const Filename = "settings.json"

// Settings is the persisted shape. New fields must have non-nil defaults
// in Default() so older files keep working.
//
// Anything user-tunable should live here. Future devs: when you're tempted
// to hardcode a constant in code, add it here first.
type Settings struct {
	// AI verification (default off)
	AIEnabled   bool   `json:"ai_enabled"`
	OllamaHost  string `json:"ollama_host"`
	OllamaModel string `json:"ollama_model"`

	// Browser defaults applied when a scenario YAML doesn't specify its own.
	// Most laptops/desktops are 1080p — 1920x1080 is the sane default.
	DefaultViewportWidth  int  `json:"default_viewport_width"`
	DefaultViewportHeight int  `json:"default_viewport_height"`
	DefaultHeadless       bool `json:"default_headless"`

	// Runner defaults.
	DefaultTimeoutMS    int `json:"default_timeout_ms"`    // Playwright per-action timeout in ms
	DefaultDiffToleranceU8 int `json:"default_diff_tolerance"` // 0..255 per-channel delta before a pixel counts as "changed"
}

// Default returns the out-of-box configuration (AI disabled, 1080p viewport,
// headless, 15s action timeout, mild pixel-diff tolerance).
func Default() Settings {
	return Settings{
		AIEnabled:              false,
		OllamaHost:             "http://127.0.0.1:11434",
		OllamaModel:            "ahmadwaqar/smolvlm2-2.2b-instruct",
		DefaultViewportWidth:   1920,
		DefaultViewportHeight:  1080,
		DefaultHeadless:        true,
		DefaultTimeoutMS:       15000,
		DefaultDiffToleranceU8: 12,
	}
}

// Store is a thread-safe accessor backed by a JSON file on disk.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  Settings
}

func NewStore(home string) (*Store, error) {
	s := &Store{
		path: filepath.Join(home, Filename),
		cur:  Default(),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var v Settings
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	// Backfill any zero values from defaults so old files don't break.
	d := Default()
	if v.OllamaHost == "" {
		v.OllamaHost = d.OllamaHost
	}
	if v.OllamaModel == "" {
		v.OllamaModel = d.OllamaModel
	}
	if v.DefaultTimeoutMS == 0 {
		v.DefaultTimeoutMS = d.DefaultTimeoutMS
	}
	if v.DefaultViewportWidth == 0 {
		v.DefaultViewportWidth = d.DefaultViewportWidth
	}
	if v.DefaultViewportHeight == 0 {
		v.DefaultViewportHeight = d.DefaultViewportHeight
	}
	if v.DefaultDiffToleranceU8 == 0 {
		v.DefaultDiffToleranceU8 = d.DefaultDiffToleranceU8
	}
	s.cur = v
	return nil
}

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Set replaces the settings and persists them.
func (s *Store) Set(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = v
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
