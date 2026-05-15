// Package dlog is a thin wrapper over log/slog for internal flow
// tracing. It is distinct from the audit log (forthcoming), which
// records user-visible, durable events. dlog is for developers
// debugging the binary, off by default.
//
// Settings come from explicit parameters; lsm reads them from the
// user's `~/.lsm/config.yaml` (the `log:` block) in PersistentPreRunE
// and passes them through. No environment variables are consulted.
package dlog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Discard is the no-op logger returned when level is "off". Useful as a
// zero-value fallback in callers that haven't yet been wired through
// PersistentPreRunE (tests, init paths).
var Discard = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))

type ctxKey struct{}

// Into returns a context with the given logger attached.
func Into(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		l = Discard
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// From returns the logger stashed on ctx, or Discard if none.
func From(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return Discard
	}
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return Discard
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// New constructs a logger from explicit parameters. Empty strings fall
// back to defaults; unknown values fall back to defaults AND emit a
// one-line warning to stderr describing the fallback.
//
//	level  = debug | info | warn | error | off   (default: off)
//	dest   = stderr | stdout | file:/absolute/path (default: stderr)
//	format = text | json                          (default: text)
//
// When level resolves to "off" the returned logger is Discard.
//
// New does not read or mutate the process environment, so concurrent
// callers are safe.
func New(level, dest, format string) (*slog.Logger, io.Closer, error) {
	levelRaw := strings.ToLower(strings.TrimSpace(level))
	if levelRaw == "" {
		levelRaw = "off"
	}

	parsed, levelOK := parseLevel(levelRaw)
	if !levelOK {
		fmt.Fprintf(os.Stderr, "dlog: unknown level %q; falling back to default\n", level)
		parsed, _ = parseLevel("off")
		levelRaw = "off"
	}

	if levelRaw == "off" {
		return Discard, nopCloser{}, nil
	}

	formatRaw := strings.ToLower(strings.TrimSpace(format))
	if formatRaw == "" {
		formatRaw = "text"
	}
	if formatRaw != "text" && formatRaw != "json" {
		fmt.Fprintf(os.Stderr, "dlog: unknown format %q; falling back to default\n", format)
		formatRaw = "text"
	}

	destRaw := strings.TrimSpace(dest)
	if destRaw == "" {
		destRaw = "stderr"
	}

	var (
		w      io.Writer
		closer io.Closer = nopCloser{}
	)
	switch {
	case strings.EqualFold(destRaw, "stderr"):
		w = os.Stderr
	case strings.EqualFold(destRaw, "stdout"):
		w = os.Stdout
	case strings.HasPrefix(destRaw, "file:"):
		path := strings.TrimPrefix(destRaw, "file:")
		expanded, err := expandHome(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dlog: cannot resolve dest path %q; falling back to default\n", dest)
			w = os.Stderr
			break
		}
		if !filepath.IsAbs(expanded) {
			fmt.Fprintf(os.Stderr, "dlog: dest file path %q must be absolute; falling back to default\n", dest)
			w = os.Stderr
			break
		}
		f, err := os.OpenFile(expanded, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return Discard, nopCloser{}, fmt.Errorf("dlog: open %s: %w", expanded, err)
		}
		w = f
		closer = f
	default:
		fmt.Fprintf(os.Stderr, "dlog: unknown dest %q; falling back to default\n", dest)
		w = os.Stderr
	}

	opts := &slog.HandlerOptions{Level: parsed, AddSource: false}
	var h slog.Handler
	if formatRaw == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h), closer, nil
}

func parseLevel(s string) (slog.Level, bool) {
	switch s {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	case "off":
		return slog.LevelError + 1, true
	}
	return slog.LevelError + 1, false
}

func expandHome(p string) (string, error) {
	if p == "" {
		return p, nil
	}
	if p == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}
