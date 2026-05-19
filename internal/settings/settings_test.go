package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_AIDisabled(t *testing.T) {
	d := Default()
	if d.AIEnabled {
		t.Errorf("AI should be off by default; got enabled")
	}
	if d.OllamaHost == "" || d.OllamaModel == "" {
		t.Errorf("defaults missing host/model: %+v", d)
	}
}

func TestDefault_Viewport1080p(t *testing.T) {
	d := Default()
	if d.DefaultViewportWidth != 1920 || d.DefaultViewportHeight != 1080 {
		t.Errorf("expected 1920x1080 default; got %dx%d", d.DefaultViewportWidth, d.DefaultViewportHeight)
	}
	if !d.DefaultHeadless {
		t.Errorf("default should be headless")
	}
	if d.DefaultTimeoutMS != 15000 {
		t.Errorf("default timeout should be 15000 ms; got %d", d.DefaultTimeoutMS)
	}
	if d.DefaultDiffToleranceU8 != 12 {
		t.Errorf("default diff tolerance should be 12; got %d", d.DefaultDiffToleranceU8)
	}
}

func TestStoreRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	s1, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Get().AIEnabled {
		t.Errorf("freshly-created store should have AI disabled")
	}

	if err := s1.Set(Settings{AIEnabled: true, OllamaHost: "http://x:9999", OllamaModel: "m"}); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify persistence
	s2, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Get()
	if !got.AIEnabled || got.OllamaHost != "http://x:9999" || got.OllamaModel != "m" {
		t.Errorf("not persisted: %+v", got)
	}
}

func TestLoad_BackfillMissingFields(t *testing.T) {
	tmp := t.TempDir()
	// Write an older settings file missing OllamaHost/Model
	old := map[string]any{"ai_enabled": true}
	b, _ := json.Marshal(old)
	if err := os.WriteFile(filepath.Join(tmp, Filename), b, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if !got.AIEnabled {
		t.Errorf("ai_enabled should still be true: %+v", got)
	}
	if got.OllamaHost == "" || got.OllamaModel == "" {
		t.Errorf("zero values should backfill from defaults: %+v", got)
	}
}
