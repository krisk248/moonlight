// Package web serves the Moonlight HTTP API + embedded SvelteKit frontend.
package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/krisk248/moonlight/internal/buildinfo"
	"github.com/krisk248/moonlight/internal/lifecycle"
	"github.com/krisk248/moonlight/internal/recorder"
	"github.com/krisk248/moonlight/internal/runner"
	"github.com/krisk248/moonlight/internal/scenario"
	"github.com/krisk248/moonlight/internal/settings"
	"github.com/krisk248/moonlight/internal/vision"
)

type Config struct {
	ProjectRoot string
	ScenarioDir string
	BaselineDir string
	RunsDir     string
	OllamaHost  string
	OllamaModel string
	Headless    bool
}

type Server struct {
	cfg      Config
	jobs     *jobRegistry
	vision   *vision.Client
	static   fs.FS
	settings *settings.Store
}

func New(cfg Config, static fs.FS) *Server {
	for _, d := range []string{cfg.ScenarioDir, cfg.BaselineDir, cfg.RunsDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	store, _ := settings.NewStore(cfg.ProjectRoot)
	if store == nil {
		// Fall back to in-memory defaults if disk write fails — better than crash.
		store, _ = settings.NewStore(os.TempDir())
	}
	cur := store.Get()
	return &Server{
		cfg:      cfg,
		jobs:     newJobRegistry(),
		vision:   vision.NewClient(cur.OllamaHost, cur.OllamaModel, 120*time.Second),
		static:   static,
		settings: store,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Get("/status", s.handleStatus)
		r.Get("/settings", s.handleGetSettings)
		r.Put("/settings", s.handlePutSettings)
		r.Get("/scenarios", s.handleListScenarios)
		r.Post("/scenarios", s.handleCreateScenario)
		r.Post("/scenarios/record", s.handleRecord)
		r.Get("/scenarios/{name}", s.handleGetScenario)
		r.Post("/scenarios/{name}/baseline", s.handleBaseline)
		r.Post("/scenarios/{name}/run", s.handleRun)
		r.Delete("/scenarios/{name}", s.handleDeleteScenario)
		r.Get("/runs", s.handleListRuns)
		r.Get("/runs/{id}", s.handleGetRun)
		r.Get("/jobs", s.handleListJobs)
		r.Get("/jobs/{id}", s.handleGetJob)
	})

	// Static-file routes for run artifacts so the UI can show screenshots/diffs.
	r.Get("/files/runs/*", s.serveFiles(s.cfg.RunsDir, "/files/runs"))
	r.Get("/files/baselines/*", s.serveFiles(s.cfg.BaselineDir, "/files/baselines"))

	// Embedded Svelte SPA — everything else hits the frontend's static files.
	r.Handle("/*", spaHandler(s.static))
	return r
}

// ---------- handlers --------------------------------------------------------

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	type out struct {
		BuildID            string `json:"build_id"`
		BuildDate          string `json:"build_date"`
		Version            string `json:"version"`
		IsSourceBuild      bool   `json:"is_source_build"`
		AIEnabled          bool   `json:"ai_enabled"`
		OllamaUp           bool   `json:"ollama_up"`
		OllamaModel        string `json:"ollama_model"`
		OllamaModelPresent bool   `json:"ollama_model_present"`
		RevocationActive   bool   `json:"revocation_active"`
	}
	cur := s.settings.Get()
	// Only probe Ollama when AI is enabled — saves an HTTP call per dashboard refresh otherwise.
	ollamaUp := false
	modelPresent := false
	if cur.AIEnabled {
		ollamaUp = s.vision.Ping()
		if ollamaUp {
			modelPresent = s.vision.IsModelLoaded(cur.OllamaModel)
		}
	}
	writeJSON(w, out{
		BuildID:            buildinfo.BuildID,
		BuildDate:          buildinfo.BuildDate,
		Version:            buildinfo.Version,
		IsSourceBuild:      buildinfo.IsSourceBuild(),
		AIEnabled:          cur.AIEnabled,
		OllamaUp:           ollamaUp,
		OllamaModel:        cur.OllamaModel,
		OllamaModelPresent: modelPresent,
		RevocationActive:   buildinfo.RevocationURL != "",
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.settings.Get())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body settings.Settings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, err)
		return
	}
	// Reasonable defaults if a field is left blank.
	defaults := settings.Default()
	if body.OllamaHost == "" {
		body.OllamaHost = defaults.OllamaHost
	}
	if body.OllamaModel == "" {
		body.OllamaModel = defaults.OllamaModel
	}
	if err := s.settings.Set(body); err != nil {
		writeError(w, 500, err)
		return
	}
	// Rebuild the vision client to honour new host/model immediately.
	s.vision = vision.NewClient(body.OllamaHost, body.OllamaModel, 120*time.Second)
	writeJSON(w, body)
}

func (s *Server) handleListScenarios(w http.ResponseWriter, _ *http.Request) {
	list, err := scenario.ListDir(s.cfg.ScenarioDir)
	if err != nil {
		writeError(w, 500, err)
		return
	}

	type scenSummary struct {
		Name        string   `json:"name"`
		URL         string   `json:"url"`
		Steps       int      `json:"steps"`
		HasBaseline bool     `json:"has_baseline"`
		HasStorage  bool     `json:"has_storage"`
		NeedsAI     bool     `json:"needs_ai"`
		Tags        []string `json:"tags"`
	}
	out := make([]scenSummary, 0, len(list))
	for _, sc := range list {
		bd := filepath.Join(s.cfg.BaselineDir, sc.Name)
		hasBaseline := false
		if entries, err := os.ReadDir(bd); err == nil {
			for _, e := range entries {
				if e.Name() != ".gitkeep" {
					hasBaseline = true
					break
				}
			}
		}
		tags := sc.Tags
		if tags == nil {
			tags = []string{}
		}
		out = append(out, scenSummary{
			Name:        sc.Name,
			URL:         sc.URL,
			Steps:       len(sc.Steps),
			HasBaseline: hasBaseline,
			HasStorage:  sc.StorageState != "",
			NeedsAI:     sc.NeedsAI(),
			Tags:        tags,
		})
	}
	writeJSON(w, out)
}

func (s *Server) handleGetScenario(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	sc, err := scenario.Load(filepath.Join(s.cfg.ScenarioDir, name+".yaml"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	writeJSON(w, sc)
}

func (s *Server) handleDeleteScenario(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	_ = os.Remove(filepath.Join(s.cfg.ScenarioDir, name+".yaml"))
	_ = os.RemoveAll(filepath.Join(s.cfg.BaselineDir, name))
	w.WriteHeader(204)
}

// handleCreateScenario writes an empty starter YAML at the requested name.
// Body: {"name": "...", "url": "..."}
func (s *Server) handleCreateScenario(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, err)
		return
	}
	body.Name = sanitizeName(body.Name)
	if body.Name == "" || body.URL == "" {
		writeError(w, 400, fmt.Errorf("name and url are required"))
		return
	}
	path := filepath.Join(s.cfg.ScenarioDir, body.Name+".yaml")
	if _, err := os.Stat(path); err == nil {
		writeError(w, 409, fmt.Errorf("scenario %q already exists", body.Name))
		return
	}
	sc := &scenario.Scenario{
		Name:     body.Name,
		URL:      body.URL,
		Viewport: scenario.Viewport{Width: 1280, Height: 720},
		Steps: []scenario.Step{
			{Action: "goto", URL: "/"},
			{Action: "screenshot", Name: "landing",
				DOMCheck: &scenario.DOMCheck{URLContains: ""}},
		},
	}
	if err := scenario.Save(sc, path); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, sc)
}

// handleRecord launches `npx playwright codegen` as a background job. The
// browser window opens on whoever's desktop the server is running on.
// Body: {"name": "...", "url": "..."}
func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, err)
		return
	}
	body.Name = sanitizeName(body.Name)
	if body.Name == "" || body.URL == "" {
		writeError(w, 400, fmt.Errorf("name and url are required"))
		return
	}
	out := filepath.Join(s.cfg.ScenarioDir, body.Name+".yaml")

	job := s.jobs.create("record", fmt.Sprintf("record %s (%s)", body.Name, body.URL))
	go func() {
		s.jobs.log(job.ID, fmt.Sprintf("[record] launching codegen for %s", body.URL))
		s.jobs.log(job.ID, "[record] a Chromium window will open on the server's desktop")
		s.jobs.log(job.ID, "[record] click through your flow, then close the window")
		_, err := recorder.Record(recorder.Options{
			URL:          body.URL,
			ScenarioName: body.Name,
			Viewport:     scenario.Viewport{Width: 1280, Height: 720},
			OutputPath:   out,
			OnLog:        func(line string) { s.jobs.log(job.ID, line) },
		})
		if err != nil {
			s.jobs.finish(job.ID, nil, err)
			return
		}
		s.jobs.log(job.ID, fmt.Sprintf("[record] wrote %s", out))
		s.jobs.finish(job.ID, nil, nil)
	}()
	writeJSON(w, job)
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) handleBaseline(w http.ResponseWriter, r *http.Request) {
	s.kickoffRun(w, r, runner.ModeBaseline)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	s.kickoffRun(w, r, runner.ModeRun)
}

func (s *Server) kickoffRun(w http.ResponseWriter, r *http.Request, mode runner.Mode) {
	name := chi.URLParam(r, "name")
	sc, err := scenario.Load(filepath.Join(s.cfg.ScenarioDir, name+".yaml"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	job := s.jobs.create(string(mode), name)
	go func() {
		cur := s.settings.Get()
		slog.Info("job.start", "id", job.ID, "kind", string(mode), "scenario", name, "ai_enabled", cur.AIEnabled, "default_timeout_ms", cur.DefaultTimeoutMS)
		opts := runner.Opts{
			ProjectRoot:      s.cfg.ProjectRoot,
			BaselineDir:      s.cfg.BaselineDir,
			RunsDir:          s.cfg.RunsDir,
			Headless:         s.cfg.Headless,
			DefaultTimeoutMS: cur.DefaultTimeoutMS,
			OnLog: func(line string) {
				s.jobs.log(job.ID, line)
				slog.Debug("job.log", "id", job.ID, "line", line)
			},
		}
		if mode == runner.ModeRun && cur.AIEnabled {
			opts.Vision = s.vision
		}
		result, runErr := runner.Run(sc, mode, opts)
		s.jobs.finish(job.ID, result, runErr)
		passed := false
		if result != nil {
			passed = result.Passed
		}
		slog.Info("job.finish", "id", job.ID, "kind", string(mode), "scenario", name, "passed", passed, "error", errOrEmpty(runErr))
	}()
	writeJSON(w, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.jobs.list())
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if j, ok := s.jobs.get(id); ok {
		writeJSON(w, j)
		return
	}
	writeError(w, 404, fmt.Errorf("no such job"))
}

func (s *Server) handleListRuns(w http.ResponseWriter, _ *http.Request) {
	entries, err := os.ReadDir(s.cfg.RunsDir)
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	type runSummary struct {
		ID         string `json:"id"`
		Scenario   string `json:"scenario"`
		Passed     bool   `json:"passed"`
		StartedAt  string `json:"started_at"`
		StepCount  int    `json:"step_count"`
		FailCount  int    `json:"fail_count"`
	}
	var out []runSummary
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		reportPath := filepath.Join(s.cfg.RunsDir, e.Name(), "report.json")
		b, err := os.ReadFile(reportPath)
		if err != nil {
			continue
		}
		var rr runner.RunResult
		if err := json.Unmarshal(b, &rr); err != nil {
			continue
		}
		failed := 0
		for _, st := range rr.Steps {
			if !st.Passed {
				failed++
			}
		}
		out = append(out, runSummary{
			ID:        e.Name(),
			Scenario:  rr.Scenario,
			Passed:    rr.Passed,
			StartedAt: rr.StartedAt,
			StepCount: len(rr.Steps),
			FailCount: failed,
		})
		if len(out) >= 50 {
			break
		}
	}
	writeJSON(w, out)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, err := os.ReadFile(filepath.Join(s.cfg.RunsDir, id, "report.json"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// serveFiles maps /prefix/<path> → <base>/<path>, with traversal guards.
func (s *Server) serveFiles(base, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rel, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, prefix+"/"))
		if err != nil || strings.Contains(rel, "..") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(base, rel))
	}
}

// ---------- helpers ---------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	slog.Warn("http.error", "code", code, "err", err.Error())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func errOrEmpty(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

// spaHandler serves the embedded Svelte build. Unknown paths fall back to
// index.html so SvelteKit client-side routing works.
func spaHandler(static fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(static, path); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// ---------- in-memory job registry -----------------------------------------

type Job struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Label      string             `json:"label"`
	Status     string             `json:"status"`
	Log        []string           `json:"log"`
	Result     *runner.RunResult  `json:"result,omitempty"`
	Error      string             `json:"error,omitempty"`
	StartedAt  string             `json:"started_at"`
	FinishedAt string             `json:"finished_at,omitempty"`
}

type jobRegistry struct {
	mu   sync.Mutex
	jobs map[string]*Job
	seq  int
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{jobs: map[string]*Job{}}
}

func (r *jobRegistry) create(kind, label string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	j := &Job{
		ID:        fmt.Sprintf("job-%d-%d", time.Now().Unix(), r.seq),
		Kind:      kind,
		Label:     label,
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	r.jobs[j.ID] = j
	return j
}

func (r *jobRegistry) log(id, line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		j.Log = append(j.Log, line)
	}
}

func (r *jobRegistry) finish(id string, result *runner.RunResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return
	}
	j.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		j.Status = "failed"
		j.Error = err.Error()
	} else {
		j.Status = "completed"
		j.Result = result
		// Persist the report so /api/runs picks it up.
		if result != nil {
			_ = os.MkdirAll(result.RunDir, 0o755)
			if b, err := json.MarshalIndent(result, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(result.RunDir, "report.json"), b, 0o644)
			}
		}
	}
}

func (r *jobRegistry) get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	return j, ok
}

func (r *jobRegistry) list() []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, j)
	}
	return out
}

// (unused but exposed for future) Lifecycle returns the sentinel + state
// paths so callers can introspect.
func (s *Server) Lifecycle() (sentinel, state string) {
	return lifecycle.SentinelFile, lifecycle.StateFile
}
