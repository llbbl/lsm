// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package dlog

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// captureStderr swaps os.Stderr for a pipe and returns a function that
// closes the writer and reads everything that was emitted while the
// swap was active. The previous suite did this; we preserve the
// pattern.
func captureStderr(t *testing.T) (read func() string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	return func() string {
		_ = w.Close()
		var sb strings.Builder
		br := bufio.NewReader(r)
		for {
			line, err := br.ReadString('\n')
			sb.WriteString(line)
			if err != nil {
				break
			}
		}
		return sb.String()
	}
}

func TestNew_EmptyParams_ReturnsDiscard(t *testing.T) {
	l, c, err := New("", "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	if l != Discard {
		t.Fatalf("expected Discard logger when all params empty")
	}
	if l.Enabled(context.Background(), slog.LevelError) {
		t.Fatalf("Discard should not be enabled at Error level")
	}
}

func TestNew_LevelOff_ReturnsDiscard(t *testing.T) {
	l, c, err := New("off", "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	if l != Discard {
		t.Fatalf("expected Discard logger when level=off")
	}
}

func TestNew_Levels(t *testing.T) {
	cases := []struct {
		level string
		slog  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			l, c, err := New(tc.level, "stderr", "text")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer func() { _ = c.Close() }()
			if !l.Enabled(context.Background(), tc.slog) {
				t.Errorf("level %q should enable %v", tc.level, tc.slog)
			}
			// One level below should be disabled (except for debug).
			if tc.slog > slog.LevelDebug && l.Enabled(context.Background(), tc.slog-1) {
				t.Errorf("level %q should NOT enable %v", tc.level, tc.slog-1)
			}
		})
	}
}

func TestNew_UnknownLevel_FallsBackToOff(t *testing.T) {
	readStderr := captureStderr(t)
	l, c, err := New("verbose", "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	out := readStderr()
	if !strings.Contains(out, "unknown level") || !strings.Contains(out, "verbose") {
		t.Fatalf("expected fallback warning mentioning level and value, got %q", out)
	}
	if l != Discard {
		t.Fatalf("expected Discard logger after fallback")
	}
}

func TestNew_DestStderr(t *testing.T) {
	// Redirect stderr so the slog output doesn't pollute test runs.
	_ = captureStderr(t)
	l, c, err := New("debug", "stderr", "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	l.Debug("hello")
}

func TestNew_DestStdout(t *testing.T) {
	l, c, err := New("debug", "stdout", "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = l
}

func TestNew_DestFile_Absolute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dlog.txt")
	l, c, err := New("debug", "file:"+path, "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debug("hello", "k", "v")
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("expected log file to contain message, got %q", string(data))
	}
}

func TestNew_DestFile_HomeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}
	// Create a temp dir inside HOME so the ~ expansion lands somewhere
	// real and writable. We use os.MkdirTemp on $HOME directly because
	// t.TempDir() lives outside HOME.
	tmp, err := os.MkdirTemp(home, "dlog-test-")
	if err != nil {
		t.Skipf("cannot create temp dir in HOME: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	rel, err := filepath.Rel(home, tmp)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	path := filepath.Join("~", rel, "dlog.txt")

	l, c, err := New("debug", "file:"+path, "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debug("homeward")
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	resolved := filepath.Join(tmp, "dlog.txt")
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "homeward") {
		t.Fatalf("expected log file to contain message, got %q", string(data))
	}
}

func TestNew_DestFile_RelativePath_FallsBack(t *testing.T) {
	readStderr := captureStderr(t)
	l, c, err := New("debug", "file:relative/path.log", "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = l
	out := readStderr()
	if !strings.Contains(out, "must be absolute") {
		t.Fatalf("expected absolute-path warning, got %q", out)
	}
}

func TestNew_UnknownDest_FallsBack(t *testing.T) {
	readStderr := captureStderr(t)
	l, c, err := New("debug", "syslog", "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = l
	out := readStderr()
	if !strings.Contains(out, "unknown dest") {
		t.Fatalf("expected unknown-dest warning, got %q", out)
	}
}

func TestNew_Format_Text(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dlog.txt")
	l, c, err := New("debug", "file:"+path, "text")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debug("hello", "k", "v")
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "msg=hello") {
		t.Fatalf("expected text format, got %q", s)
	}
	// Text format should not be valid JSON.
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err == nil {
		t.Fatalf("expected text not JSON, but parsed cleanly")
	}
}

func TestNew_Format_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dlog.jsonl")
	l, c, err := New("debug", "file:"+path, "json")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debug("hello", "k", "v")
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %q", len(lines), string(data))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("json: %v -- %q", err, lines[0])
	}
	if rec["level"] != "DEBUG" {
		t.Fatalf("expected level=DEBUG, got %v", rec["level"])
	}
	if rec["msg"] != "hello" {
		t.Fatalf("expected msg=hello, got %v", rec["msg"])
	}
	if rec["k"] != "v" {
		t.Fatalf("expected k=v, got %v", rec["k"])
	}
}

func TestNew_UnknownFormat_FallsBack(t *testing.T) {
	readStderr := captureStderr(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dlog.txt")
	l, c, err := New("debug", "file:"+path, "xml")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debug("hi")
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	out := readStderr()
	if !strings.Contains(out, "unknown format") {
		t.Fatalf("expected unknown-format warning, got %q", out)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "msg=hi") {
		t.Fatalf("expected text fallback output, got %q", string(data))
	}
}

func TestNew_ConcurrentCalls_NoRace(t *testing.T) {
	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			l, c, err := New("debug", "stderr", "text")
			if err != nil {
				t.Errorf("New: %v", err)
				return
			}
			defer func() { _ = c.Close() }()
			if !l.Enabled(context.Background(), slog.LevelDebug) {
				t.Errorf("expected debug level enabled")
			}
		}()
	}
	wg.Wait()
}

func TestIntoFromRoundTrip(t *testing.T) {
	l := slog.Default()
	ctx := Into(context.Background(), l)
	if got := From(ctx); got != l {
		t.Fatalf("From did not return the logger we put in")
	}
}

func TestFrom_NoLogger_ReturnsDiscard(t *testing.T) {
	got := From(context.Background())
	if got != Discard {
		t.Fatalf("expected Discard from empty context")
	}
	// Must not panic.
	got.Debug("x")
}

func TestFrom_NilContext_ReturnsDiscard(t *testing.T) {
	// Intentionally testing nil-context handling. Use a typed nil so
	// staticcheck (SA1012) doesn't flag a literal nil at the call site.
	var ctx context.Context
	got := From(ctx)
	if got != Discard {
		t.Fatalf("expected Discard from nil context")
	}
}
