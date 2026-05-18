package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/krisk248/moonlight/internal/buildinfo"
)

func TestIsPast(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty means no expiry", "", false},
		{"future date is not past", "2099-01-01", false},
		{"past date is past", "1999-01-01", true},
		{"today is not past (inclusive of today)", time.Now().UTC().Format("2006-01-02"), false},
		{"malformed → not past (fail-safe)", "not-a-date", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPast(tt.in); got != tt.want {
				t.Errorf("isPast(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFetchSpec_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RevocationSpec{LastDay: "2030-12-31"})
	}))
	defer srv.Close()

	old := buildinfo.RevocationURL
	buildinfo.RevocationURL = srv.URL
	defer func() { buildinfo.RevocationURL = old }()

	spec := fetchSpec()
	if spec == nil || spec.LastDay != "2030-12-31" {
		t.Fatalf("fetchSpec() = %+v", spec)
	}
}

func TestFetchSpec_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	old := buildinfo.RevocationURL
	buildinfo.RevocationURL = srv.URL
	defer func() { buildinfo.RevocationURL = old }()

	if spec := fetchSpec(); spec != nil {
		t.Errorf("expected nil on 500, got %+v", spec)
	}
}

func TestFetchSpec_EmptyURL(t *testing.T) {
	old := buildinfo.RevocationURL
	buildinfo.RevocationURL = ""
	defer func() { buildinfo.RevocationURL = old }()
	if spec := fetchSpec(); spec != nil {
		t.Errorf("expected nil for empty URL, got %+v", spec)
	}
}

func TestStateRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MOONLIGHT_HOME", tmp)

	in := state{
		CachedSpec:       &RevocationSpec{LastDay: "2030-01-01"},
		LastSuccess:      time.Now().UTC().Format(time.RFC3339),
		FirstUnreachable: "",
	}
	saveState(in)
	out := loadState()
	if out.LastSuccess != in.LastSuccess {
		t.Errorf("LastSuccess mismatch: got %q want %q", out.LastSuccess, in.LastSuccess)
	}
	if out.CachedSpec == nil || out.CachedSpec.LastDay != "2030-01-01" {
		t.Errorf("CachedSpec mismatch: got %+v", out.CachedSpec)
	}
}

func TestSentinelDetection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MOONLIGHT_HOME", tmp)

	if _, err := os.Stat(sentinelPath()); err == nil {
		t.Fatal("sentinel should not exist yet")
	}
	if err := os.WriteFile(filepath.Join(tmp, SentinelFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinelPath()); err != nil {
		t.Errorf("sentinel should exist now: %v", err)
	}
}

func TestWipeSensitive_LeavesGitkeep(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MOONLIGHT_HOME", tmp)
	scenDir := filepath.Join(tmp, "scenarios")
	authDir := filepath.Join(tmp, "auth-state")
	if err := os.MkdirAll(scenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scenDir, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scenDir, "real-scenario.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "session.json"), []byte("cookie"), 0o644); err != nil {
		t.Fatal(err)
	}

	wipeSensitive()

	if _, err := os.Stat(filepath.Join(scenDir, ".gitkeep")); err != nil {
		t.Errorf(".gitkeep was deleted (should be kept): %v", err)
	}
	if _, err := os.Stat(filepath.Join(scenDir, "real-scenario.yaml")); err == nil {
		t.Errorf("real-scenario.yaml should have been wiped")
	}
	if _, err := os.Stat(filepath.Join(authDir, "session.json")); err == nil {
		t.Errorf("auth-state/session.json should have been wiped")
	}
}
