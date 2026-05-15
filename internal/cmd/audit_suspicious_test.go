// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llbbl/lsm/internal/audit"
)

// seedSuspiciousLog writes n events with caller-controlled timestamps and
// actors through a real FileSink. Distinct from seedAuditLog because the
// suspicious detectors need precise timestamp + actor control.
func seedSuspiciousLog(t *testing.T, mutate func(i int, e *audit.Event), n int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := audit.NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	defer sink.Close()
	for i := range n {
		ev := audit.Event{
			Event: "test",
			App:   "myapp",
			Env:   "dev",
			Actor: audit.Actor{ParentComm: "node", AgentMarker: "claude", TTY: "/dev/tty0"},
		}
		if mutate != nil {
			mutate(i, &ev)
		}
		if err := sink.Write(context.Background(), ev); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	return path
}

func TestAuditSuspicious_EmptyChain(t *testing.T) {
	path := seedSuspiciousLog(t, nil, 0)
	out, err := runCmd(t, "audit", "suspicious", "--file", path)
	if err != nil {
		t.Fatalf("audit suspicious: %v", err)
	}
	if !strings.Contains(out, "no suspicious events found") {
		t.Errorf("expected empty-result message, got: %q", out)
	}
}

func TestAuditSuspicious_BurstJSON(t *testing.T) {
	// Old establishing event + 5 rapid events. With --burst-threshold 3,
	// events 4 and 5 land as bursts.
	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	path := seedSuspiciousLog(t, func(i int, e *audit.Event) {
		if i == 0 {
			e.Timestamp = base.Add(-60 * 24 * time.Hour)
			return
		}
		e.Timestamp = base.Add(time.Duration(i-1) * 10 * time.Second)
	}, 6)

	out, err := runCmd(t, "audit", "suspicious", "--file", path, "--burst-threshold", "3", "--burst-window", "1m")
	if err != nil {
		t.Fatalf("audit suspicious: %v", err)
	}
	if !strings.Contains(out, `"reasons":["burst"]`) {
		t.Errorf("expected JSON burst reason in output, got: %s", out)
	}
}

func TestAuditSuspicious_BurstText(t *testing.T) {
	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	path := seedSuspiciousLog(t, func(i int, e *audit.Event) {
		if i == 0 {
			e.Timestamp = base.Add(-60 * 24 * time.Hour)
			return
		}
		e.Timestamp = base.Add(time.Duration(i-1) * 10 * time.Second)
	}, 6)

	out, err := runCmd(t, "audit", "suspicious", "--file", path,
		"--burst-threshold", "3", "--burst-window", "1m", "--format", "text")
	if err != nil {
		t.Fatalf("audit suspicious: %v", err)
	}
	if !strings.Contains(out, "[burst]") {
		t.Errorf("expected text [burst] reason prefix, got: %s", out)
	}
}

func TestAuditSuspicious_BadHours(t *testing.T) {
	path := seedSuspiciousLog(t, nil, 1)
	_, err := runCmd(t, "audit", "suspicious", "--file", path, "--hours", "bad-format")
	if err == nil {
		t.Fatal("expected error for bad --hours")
	}
	if !strings.Contains(err.Error(), "--hours") {
		t.Errorf("err=%v, want '--hours' mention", err)
	}
}

func TestAuditSuspicious_BadBurstWindow(t *testing.T) {
	path := seedSuspiciousLog(t, nil, 1)
	_, err := runCmd(t, "audit", "suspicious", "--file", path, "--burst-window", "nope")
	if err == nil {
		t.Fatal("expected error for bad --burst-window")
	}
	if !strings.Contains(err.Error(), "--burst-window") {
		t.Errorf("err=%v, want '--burst-window' mention", err)
	}
}

func TestAuditSuspicious_NewParentCommSkipNote(t *testing.T) {
	// All events recent — file too young for the lookback. Command must
	// not error; the stderr note is bundled into runCmd's buffer.
	path := seedSuspiciousLog(t, nil, 2)
	out, err := runCmd(t, "audit", "suspicious", "--file", path)
	if err != nil {
		t.Fatalf("audit suspicious: %v", err)
	}
	if !strings.Contains(out, "new-parent-comm detector skipped") {
		t.Errorf("expected skip note in output, got: %s", out)
	}
}

func TestAuditSuspicious_MissingFile(t *testing.T) {
	_, err := runCmd(t, "audit", "suspicious", "--file", filepath.Join(t.TempDir(), "nope.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err=%v, want 'not found'", err)
	}
}
