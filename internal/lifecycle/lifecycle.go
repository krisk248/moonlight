// Package lifecycle enforces the centralized kill switch.
//
// On every entry point Enforce() is called; it:
//  1. Refuses to start if a sentinel file exists from a previous kill.
//  2. Fetches the revocation JSON (one field: last_day) from the configured URL.
//  3. If today is past last_day → wipes sensitive data + writes sentinel + exits.
//  4. If unreachable, falls back to a cached response (<24h old).
//  5. If unreachable AND no successful fetch for 30 days → silent kill.
//
// All operator-visible messages are deliberately generic.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/krisk248/moonlight/internal/buildinfo"
)

const (
	SentinelFile     = ".killed"
	StateFile        = ".moonlight-state.json"
	GenericMessage   = "This binary is no longer authorized. Contact your administrator."
	CacheTTL         = 24 * time.Hour
	NoContactTimeout = 30 * 24 * time.Hour
)

type RevocationSpec struct {
	LastDay string `json:"last_day"`
}

type state struct {
	CachedSpec       *RevocationSpec `json:"cached_spec,omitempty"`
	LastSuccess      string          `json:"last_success,omitempty"`
	FirstUnreachable string          `json:"first_unreachable,omitempty"`
}

// Home returns the directory the binary considers "its install."
// Defaults to cwd; can be overridden with MOONLIGHT_HOME.
func Home() string {
	if h := os.Getenv("MOONLIGHT_HOME"); h != "" {
		return h
	}
	wd, _ := os.Getwd()
	return wd
}

func statePath() string    { return filepath.Join(Home(), StateFile) }
func sentinelPath() string { return filepath.Join(Home(), SentinelFile) }

func loadState() state {
	var s state
	b, err := os.ReadFile(statePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}

func saveState(s state) {
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(statePath(), b, 0o644)
}

// fetchSpec issues an HTTPS GET against the revocation URL and returns the
// parsed JSON. Any error → nil (treated as "unreachable").
func fetchSpec() *RevocationSpec {
	if buildinfo.RevocationURL == "" {
		return nil
	}
	req, err := http.NewRequest("GET", buildinfo.RevocationURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "moonlight-lifecycle")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return nil
	}
	var spec RevocationSpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return nil
	}
	return &spec
}

func isPast(lastDayStr string) bool {
	if lastDayStr == "" {
		return false
	}
	last, err := time.Parse("2006-01-02", lastDayStr)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(last.Add(24 * time.Hour))
}

func wipeSensitive() {
	home := Home()
	for _, d := range []string{"scenarios", "auth-state"} {
		dir := filepath.Join(home, d)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() == ".gitkeep" {
				continue
			}
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

func writeSentinel() {
	_ = os.WriteFile(sentinelPath(), nil, 0o644)
}

func kill() {
	wipeSensitive()
	writeSentinel()
	fmt.Println(GenericMessage)
	os.Exit(2)
}

// Enforce is the function every entry point must call first.
func Enforce() {
	// 1. Sentinel from prior kill.
	if _, err := os.Stat(sentinelPath()); err == nil {
		fmt.Println(GenericMessage)
		os.Exit(2)
	}

	// 2. Source builds: dev mode, no restriction.
	if buildinfo.IsSourceBuild() {
		return
	}

	// 3. Stamped binary — run the central check.
	s := loadState()
	now := time.Now().UTC()

	spec := fetchSpec()
	if spec != nil {
		s.CachedSpec = spec
		s.LastSuccess = now.Format(time.RFC3339)
		s.FirstUnreachable = ""
		saveState(s)
		if isPast(spec.LastDay) {
			kill()
		}
		return
	}

	// 4. Fetch failed → use cache if recent enough.
	var lastSuccess time.Time
	if s.LastSuccess != "" {
		lastSuccess, _ = time.Parse(time.RFC3339, s.LastSuccess)
	}
	if !lastSuccess.IsZero() && now.Sub(lastSuccess) < CacheTTL {
		if s.CachedSpec != nil && isPast(s.CachedSpec.LastDay) {
			kill()
		}
		return
	}

	// 5. No fresh cache. Check the no-contact deadman timer.
	if lastSuccess.IsZero() {
		if s.FirstUnreachable == "" {
			s.FirstUnreachable = now.Format(time.RFC3339)
			saveState(s)
			return
		}
		first, _ := time.Parse(time.RFC3339, s.FirstUnreachable)
		if !first.IsZero() && now.Sub(first) >= NoContactTimeout {
			kill()
		}
		return
	}
	if now.Sub(lastSuccess) >= NoContactTimeout {
		kill()
	}
}
