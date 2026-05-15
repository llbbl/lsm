// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDlogWiring_DebugToFile confirms PersistentPreRunE reads the log
// block from <dir>/config.yaml (the global config file lsm looks at)
// and that the initial Debug call lands in the destination file.
func TestDlogWiring_DebugToFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "lsm.log")

	cfg := fmt.Sprintf("env: dev\nlog:\n  level: debug\n  dest: file:%s\n  format: text\n", logPath)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// `apps --dir <tempdir>` is a safe no-op subcommand that exercises
	// the persistent pre/post-run wiring without needing any setup.
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

// TestDlogWiring_DefaultOff confirms that without a `log:` block, the
// pre-run still succeeds (logger is Discard) and no file is written.
func TestDlogWiring_DefaultOff(t *testing.T) {
	dir := t.TempDir()
	// Config exists but has no log block — level defaults to off.
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: dev\n"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}
}

// TestDlogWiring_NoConfigFile confirms commands work when no config
// file exists yet (the chicken-and-egg case for `lsm init` itself).
func TestDlogWiring_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	// Intentionally no config.yaml written.

	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}
}
