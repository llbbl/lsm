// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDlogWiring_DebugToFile confirms PersistentPreRunE attaches a dlog
// logger configured from LSM_LOG_* and that its initial Debug call lands
// in the destination file.
func TestDlogWiring_DebugToFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "lsm.log")
	t.Setenv("LSM_LOG_LEVEL", "debug")
	t.Setenv("LSM_LOG_DEST", "file:"+logPath)
	t.Setenv("LSM_LOG_FORMAT", "text")

	// `apps --dir <empty-tmp>` is a safe no-op subcommand that exercises
	// the persistent pre/post-run wiring without needing any setup.
	dir := t.TempDir()
	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "command starting") {
		t.Errorf("log missing 'command starting' line; got: %q", s)
	}
	if !strings.Contains(s, "apps") {
		t.Errorf("log missing command name 'apps'; got: %q", s)
	}
}

// TestDlogWiring_DefaultOff confirms that without LSM_LOG_LEVEL set, the
// pre-run still succeeds (logger is Discard) — no destination file is
// written because the env-driven default is "off".
func TestDlogWiring_DefaultOff(t *testing.T) {
	t.Setenv("LSM_LOG_LEVEL", "")
	t.Setenv("LSM_LOG_DEST", "")
	t.Setenv("LSM_LOG_FORMAT", "")

	dir := t.TempDir()
	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}
}
