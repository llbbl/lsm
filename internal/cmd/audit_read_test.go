// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/audit"
)

// seedAuditLog writes n events through a real FileSink and returns the path.
func seedAuditLog(t *testing.T, n int, mutate func(i int, e *audit.Event)) string {
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
			t.Fatalf("sink.Write: %v", err)
		}
	}
	return path
}

func TestAuditTail_Default(t *testing.T) {
	path := seedAuditLog(t, 5, nil)
	out, err := runCmd(t, "audit", "tail", "--file", path)
	if err != nil {
		t.Fatalf("audit tail: %v", err)
	}
	if strings.Count(out, "\n") != 5 {
		t.Errorf("expected 5 lines, got: %q", out)
	}
}

func TestAuditTail_LimitN(t *testing.T) {
	path := seedAuditLog(t, 10, nil)
	out, err := runCmd(t, "audit", "tail", "-n", "3", "--file", path)
	if err != nil {
		t.Fatalf("audit tail: %v", err)
	}
	if strings.Count(out, "\n") != 3 {
		t.Errorf("expected 3 lines, got: %q", out)
	}
	// runCmd's stdout is a buffer (non-TTY), so default format is JSON.
	if !strings.Contains(out, `"seq":8`) || !strings.Contains(out, `"seq":10`) {
		t.Errorf("expected last 3 seqs in output: %s", out)
	}
}

func TestAuditTail_MissingFile(t *testing.T) {
	_, err := runCmd(t, "audit", "tail", "--file", filepath.Join(t.TempDir(), "nope.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err=%v, want 'not found'", err)
	}
}

func TestAuditShow_Found(t *testing.T) {
	path := seedAuditLog(t, 5, nil)
	out, err := runCmd(t, "audit", "show", "3", "--file", path)
	if err != nil {
		t.Fatalf("audit show: %v", err)
	}
	// JSON output (non-TTY) — should contain seq 3 (pretty-printed for show).
	if !strings.Contains(out, `"seq": 3`) {
		t.Errorf("expected seq 3 in output: %s", out)
	}
}

func TestAuditShow_NotFound(t *testing.T) {
	path := seedAuditLog(t, 3, nil)
	_, err := runCmd(t, "audit", "show", "999", "--file", path)
	if err == nil {
		t.Fatal("expected error for non-existent seq")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err=%v, want 'not found'", err)
	}
}

func TestAuditShow_TextFormat(t *testing.T) {
	path := seedAuditLog(t, 3, nil)
	out, err := runCmd(t, "audit", "show", "2", "--file", path, "--format", "text")
	if err != nil {
		t.Fatalf("audit show: %v", err)
	}
	if !strings.Contains(out, "seq:") || !strings.Contains(out, "hash:") {
		t.Errorf("text format missing expected labels: %s", out)
	}
}

func TestAuditQuery_AppFilter(t *testing.T) {
	path := seedAuditLog(t, 4, func(i int, e *audit.Event) {
		if i%2 == 0 {
			e.App = "alpha"
		} else {
			e.App = "beta"
		}
	})
	out, err := runCmd(t, "audit", "query", "--file", path, "--app", "alpha")
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if strings.Count(out, "\n") != 2 {
		t.Errorf("expected 2 matches, got: %q", out)
	}
	if strings.Contains(out, `"app":"beta"`) {
		t.Errorf("query should not include beta: %s", out)
	}
}

func TestAuditQuery_SinceDuration(t *testing.T) {
	path := seedAuditLog(t, 2, nil)
	// All events were just written, so --since 1h should match both.
	out, err := runCmd(t, "audit", "query", "--file", path, "--since", "1h")
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if strings.Count(out, "\n") != 2 {
		t.Errorf("expected 2 matches, got: %q", out)
	}
}

func TestAuditQuery_BadTime(t *testing.T) {
	path := seedAuditLog(t, 1, nil)
	_, err := runCmd(t, "audit", "query", "--file", path, "--since", "not-a-time")
	if err == nil {
		t.Fatal("expected error for bad time")
	}
	if !strings.Contains(err.Error(), "--since") {
		t.Errorf("err=%v, want flag mention", err)
	}
}

func TestAuditQuery_TextFormat(t *testing.T) {
	path := seedAuditLog(t, 2, nil)
	out, err := runCmd(t, "audit", "query", "--file", path, "--format", "text")
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if !strings.Contains(out, "seq=1") || !strings.Contains(out, "seq=2") {
		t.Errorf("expected text format with seq columns: %s", out)
	}
}

func TestAuditQuery_DefaultIsJSON(t *testing.T) {
	path := seedAuditLog(t, 1, nil)
	out, err := runCmd(t, "audit", "query", "--file", path)
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	// runCmd buffer is non-TTY → default should be JSON.
	if !strings.Contains(out, `"seq":1`) {
		t.Errorf("expected JSON output, got: %s", out)
	}
}

func TestAuditQuery_SeqRange(t *testing.T) {
	path := seedAuditLog(t, 5, nil)
	out, err := runCmd(t, "audit", "query", "--file", path, "--seq-from", "2", "--seq-to", "4")
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if strings.Count(out, "\n") != 3 {
		t.Errorf("expected 3 events (seq 2-4), got: %s", out)
	}
}

// TestAuditFormat_NotInheritedAcrossInvocations is the regression test for
// the package-global auditFormat leak. Before the fix, the first runCmd
// with --format=text would set the package variable and the second runCmd
// (without the flag) would still see "text", because NewRootCmd() did not
// reset it. After the fix the flag lives on the cobra flag set, which is
// rebuilt per NewRootCmd().
func TestAuditFormat_NotInheritedAcrossInvocations(t *testing.T) {
	path := seedAuditLog(t, 2, nil)

	// First invocation: explicit text format.
	textOut, err := runCmd(t, "audit", "show", "1", "--file", path, "--format", "text")
	if err != nil {
		t.Fatalf("first invocation: %v", err)
	}
	if !strings.Contains(textOut, "seq:") || !strings.Contains(textOut, "hash:") {
		t.Fatalf("first invocation should be text format, got: %q", textOut)
	}
	if strings.Contains(textOut, `"seq":`) {
		t.Fatalf("first invocation looks like JSON, not text: %q", textOut)
	}

	// Second invocation against a fresh NewRootCmd, no --format flag.
	// runCmd writes to a bytes.Buffer (non-TTY), so auto-detect must
	// resolve to JSON. If auditFormat leaked across invocations this
	// would still emit text.
	jsonOut, err := runCmd(t, "audit", "show", "1", "--file", path)
	if err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	if !strings.Contains(jsonOut, `"seq": 1`) {
		t.Errorf("second invocation should auto-detect JSON, got: %q", jsonOut)
	}
	if strings.Contains(jsonOut, "hash:            ") {
		t.Errorf("second invocation leaked text format from prior run: %q", jsonOut)
	}
}

// TestAuditQuery_BadFormat verifies that an unknown --format value is
// surfaced as an error rather than silently auto-detecting.
func TestAuditQuery_BadFormat(t *testing.T) {
	path := seedAuditLog(t, 1, nil)
	_, err := runCmd(t, "audit", "query", "--file", path, "--format", "yaml")
	if err == nil {
		t.Fatal("expected error for unknown --format value")
	}
	if !strings.Contains(err.Error(), "unknown --format") {
		t.Errorf("err=%v, want 'unknown --format' mention", err)
	}
}

// TestParseDurationExt_RejectsMixed locks in the documented restriction
// that mixed-unit forms like "1d12h" are not accepted by the extended
// parser. Only a single unit per duration.
func TestParseDurationExt_RejectsMixed(t *testing.T) {
	if _, err := parseDurationExt("1d12h"); err == nil {
		t.Fatal("expected parseDurationExt to reject mixed 1d12h")
	}
}
