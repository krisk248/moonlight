package vision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mockOllama(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"0.24.0"}`))
		case "/api/chat":
			resp := map[string]any{
				"message": map[string]any{"content": content},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
}

func tmpPNG(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "img.png")
	// minimal PNG header — content doesn't matter, we just need a readable file
	_ = os.WriteFile(p, []byte("\x89PNG\r\n\x1a\n"), 0o644)
	return p
}

func TestPing(t *testing.T) {
	srv := mockOllama(t, "ignored")
	defer srv.Close()
	c := NewClient(srv.URL, "test-model", 5*time.Second)
	if !c.Ping() {
		t.Error("Ping() returned false against live mock server")
	}
}

func TestFunctionalCheck_YesMatchesExpectYes(t *testing.T) {
	srv := mockOllama(t, "YES, the page shows the expected content.")
	defer srv.Close()
	c := NewClient(srv.URL, "m", 5*time.Second)

	v, err := c.FunctionalCheck(tmpPNG(t), "is the page correct?", true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Passed {
		t.Errorf("expected passed=true, got %+v", v)
	}
}

func TestFunctionalCheck_NoMismatchesExpectYes(t *testing.T) {
	srv := mockOllama(t, "NO, the page shows an error.")
	defer srv.Close()
	c := NewClient(srv.URL, "m", 5*time.Second)

	v, err := c.FunctionalCheck(tmpPNG(t), "is the page correct?", true)
	if err != nil {
		t.Fatal(err)
	}
	if v.Passed {
		t.Errorf("expected passed=false, got %+v", v)
	}
}

func TestFunctionalCheck_NoMatchesExpectNo(t *testing.T) {
	srv := mockOllama(t, "NO clear errors visible.")
	defer srv.Close()
	c := NewClient(srv.URL, "m", 5*time.Second)

	v, err := c.FunctionalCheck(tmpPNG(t), "any errors?", false)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Passed {
		t.Errorf("expected passed=true, got %+v", v)
	}
}

func TestFunctionalCheck_FirstTokenWithPunctuation(t *testing.T) {
	srv := mockOllama(t, "Yes. The page is correct.")
	defer srv.Close()
	c := NewClient(srv.URL, "m", 5*time.Second)

	v, err := c.FunctionalCheck(tmpPNG(t), "?", true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Passed {
		t.Errorf("expected first-token parser to handle punctuation; got %+v", v)
	}
}

func TestNarrate(t *testing.T) {
	srv := mockOllama(t, "A login form with username and password inputs.\nIgnored second line.")
	defer srv.Close()
	c := NewClient(srv.URL, "m", 5*time.Second)

	out, err := c.Narrate(tmpPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	want := "A login form with username and password inputs."
	if out != want {
		t.Errorf("Narrate trimming wrong: got %q want %q", out, want)
	}
}
