// Package applog sets up structured logging shared by every Moonlight
// subsystem. Output goes to BOTH stdout and a per-run log file under
// $MOONLIGHT_HOME/logs/, so beta operators can tail the file or read the
// console.
package applog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

var (
	// File is the path of the current log file. Empty until Init() has been called.
	File string
)

// Init creates the log directory, opens the per-process log file, and installs
// a slog default logger that writes JSON to the file and a human text format
// to stdout. Returns the underlying io.Writer for the file (for tests).
func Init(home string) (io.Writer, error) {
	dir := filepath.Join(home, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	File = filepath.Join(dir, fmt.Sprintf("moonlight-server-%s.log", stamp))
	f, err := os.OpenFile(File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	// Human-readable lines to stdout, JSON to file. Both honour the same handler
	// level so changing one changes both.
	stdoutH := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	fileH := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})

	// A trivial "fan-out" handler — calls each child for every record.
	slog.SetDefault(slog.New(&fanout{[]slog.Handler{stdoutH, fileH}}))

	slog.Info("logger initialized", "logfile", File)
	return f, nil
}

type fanout struct {
	handlers []slog.Handler
}

func (f *fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}
func (f *fanout) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range f.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}
func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		out[i] = h.WithAttrs(attrs)
	}
	return &fanout{out}
}
func (f *fanout) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		out[i] = h.WithGroup(name)
	}
	return &fanout{out}
}
