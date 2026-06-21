package audit

import (
	"os"
)

// agentMarkers lists environment variables set by known AI coding tools. The
// first matching marker wins. These vars are read-only signals set by external
// tools (not lsm configuration), so consulting them does not violate lsm's
// "config-only" rule for its own settings.
var agentMarkers = []struct {
	env, name string
}{
	{"CLAUDE_CODE", "claude"},
	{"CLAUDECODE", "claude"},
	{"CURSOR_TRACE_ID", "cursor"},
	{"AIDER_PROCESS", "aider"},
	{"CONTINUE_SESSION_ID", "continue"},
	{"OPENHANDS_RUNTIME", "openhands"},
}

// CaptureActor returns process metadata for the running lsm invocation.
// Fields that cannot be determined are left at their zero value.
//
// CaptureActor never returns an error: it represents a best-effort snapshot,
// and downstream consumers can detect missing fields by their zero values.
func CaptureActor() Actor {
	a := Actor{
		PPID: os.Getppid(),
		UID:  os.Getuid(),
	}
	if cwd, err := os.Getwd(); err == nil {
		a.CWD = cwd
	}
	a.ParentComm = lookupParentComm(a.PPID)
	a.TTY = lookupTTY()
	a.AgentMarker = detectAgentMarker()
	return a
}

// detectAgentMarker returns a short identifier for the AI coding tool whose
// environment marker is present, or "" if none of the known markers are set.
func detectAgentMarker() string {
	for _, m := range agentMarkers {
		if os.Getenv(m.env) != "" {
			return m.name
		}
	}
	return ""
}

// lookupTTY returns a best-effort identifier for the controlling terminal of
// stdin. It returns "" when stdin is not a character device (pipe/redirect,
// typical for `go test` or scripted invocations). When stdin is a terminal, it
// attempts to resolve the actual device path; if that fails it returns the
// sentinel "tty" so callers can still answer the "is this interactive?"
// question via tty != "".
func lookupTTY() string {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	if stat.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	if dev := resolveTTYDevice(); dev != "" {
		return dev
	}
	return "tty"
}
