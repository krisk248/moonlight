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
type Settings struct {
	AIEnabled       bool   `json:"ai_enabled"`
	OllamaHost      string `json:"ollama_host"`
	OllamaModel     string `json:"ollama_model"`
	DefaultTimeoutMS int   `json:"default_timeout_ms"` // global Playwright timeout (per action). 0 → 15000.
}

// Default returns the out-of-box configuration (AI disabled).
func Default() Settings {
	return Settings{
		AIEnabled:        false,
		OllamaHost:       "http://127.0.0.1:11434",
		OllamaModel:      "ahmadwaqar/smolvlm2-2.2b-instruct",
		DefaultTimeoutMS: 15000,
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
