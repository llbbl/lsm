//go:build !windows

package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureActor_BasicFields(t *testing.T) {
	a := CaptureActor()

	if a.PPID <= 0 {
		t.Errorf("PPID should be > 0, got %d", a.PPID)
	}
	if a.UID < 0 {
		t.Errorf("UID should be >= 0, got %d", a.UID)
	}
	if a.CWD == "" {
		t.Error("CWD should be non-empty")
	}
	if !filepath.IsAbs(a.CWD) {
		t.Errorf("CWD should be absolute, got %q", a.CWD)
	}
	if a.ParentComm == "" {
		t.Error("ParentComm should be non-empty (test runner has a parent)")
	}
	// AgentMarker may be "" or any of the known markers depending on whoever
	// runs the test. Don't assert a specific value.
	// TTY is expected to be "" under `go test` (stdin is a pipe, not a char
	// device). Verified separately in TestLookupTTY_NotInteractive.
}

func TestLookupParentComm_Self(t *testing.T) {
	// Looking up the current process via its own PID exercises the same code
	// path as looking up a parent, but uses an ID we know is valid.
	got := lookupParentComm(os.Getpid())
	if got == "" {
		t.Errorf("lookupParentComm(self) returned empty; expected non-empty comm")
	}
}

func TestLookupParentComm_InvalidPID(t *testing.T) {
	cases := []int{0, -1, -42}
	for _, pid := range cases {
		if got := lookupParentComm(pid); got != "" {
			t.Errorf("lookupParentComm(%d) = %q, want empty", pid, got)
		}
	}
}

func TestDetectAgentMarker(t *testing.T) {
	// Clear every known marker so a real CLAUDE_CODE env in the test runner
	// doesn't bleed into the "no marker" assertion.
	for _, m := range agentMarkers {
		t.Setenv(m.env, "")
	}

	if got := detectAgentMarker(); got != "" {
		t.Errorf("with no markers set, detectAgentMarker() = %q, want \"\"", got)
	}

	t.Setenv("CLAUDE_CODE", "1")
	if got := detectAgentMarker(); got != "claude" {
		t.Errorf("with CLAUDE_CODE set, detectAgentMarker() = %q, want \"claude\"", got)
	}
}

func TestDetectAgentMarker_AllKnown(t *testing.T) {
	// Each marker resolves to its expected short name in isolation.
	for _, m := range agentMarkers {
		t.Run(m.env, func(t *testing.T) {
			for _, other := range agentMarkers {
				t.Setenv(other.env, "")
			}
			t.Setenv(m.env, "1")
			if got := detectAgentMarker(); got != m.name {
				t.Errorf("with %s set, detectAgentMarker() = %q, want %q", m.env, got, m.name)
			}
		})
	}
}

func TestLookupTTY_NotInteractive(t *testing.T) {
	// Swap stdin for a pipe so we exercise the "not a character device" branch
	// deterministically. Under `go test` invoked from a terminal, the real
	// stdin can still be a char device (the controlling tty), which would not
	// represent the automation scenario downstream cares about.
	//
	// A pipe is a non-char device — the same shape stdin takes when lsm is
	// invoked from a script or CI runner.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	if got := lookupTTY(); got != "" {
		t.Errorf("lookupTTY() with piped stdin = %q, want \"\"", got)
	}
}
