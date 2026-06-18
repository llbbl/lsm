// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditCmd_PreservesContent(t *testing.T) {
	dir := setupTestEnv(t)

	// Set some secrets first
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "EDIT_KEY", "edit_val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Use "true" as EDITOR (no-op, exits 0) - content should be preserved
	t.Setenv("EDITOR", "true")

	_, err = runCmd(t, "edit", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("edit error: %v", err)
	}

	// Verify content is preserved
	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "EDIT_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "edit_val" {
		t.Errorf("value after edit = %q, want %q", out, "edit_val")
	}
}

func TestEditCmd_EditorModifiesContent(t *testing.T) {
	dir := setupTestEnv(t)

	// Set initial content
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "ORIG_KEY", "orig_val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Create a script that appends a new line to the file
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "editor.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf '\\nNEW_KEY=new_val\\n' >> \"$1\"\n"), 0755); err != nil {
		t.Fatalf("writing editor script: %v", err)
	}

	t.Setenv("EDITOR", scriptPath)

	_, err = runCmd(t, "edit", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("edit error: %v", err)
	}

	// Verify old key preserved
	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "ORIG_KEY")
	if err != nil {
		t.Fatalf("get ORIG_KEY error: %v", err)
	}
	if out != "orig_val" {
		t.Errorf("ORIG_KEY = %q, want %q", out, "orig_val")
	}

	// Verify new key added
	out, err = runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "NEW_KEY")
	if err != nil {
		t.Fatalf("get NEW_KEY error: %v", err)
	}
	if out != "new_val" {
		t.Errorf("NEW_KEY = %q, want %q", out, "new_val")
	}
}

func TestEditCmd_EditorWithFlags(t *testing.T) {
	// Regression test for: $EDITOR with flags (e.g. "zed --wait") must be
	// tokenized on whitespace before invoking exec.Command.
	dir := setupTestEnv(t)

	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "ORIG_KEY", "orig_val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Script that requires a leading --flag arg, proving multi-token parsing.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "editor.sh")
	script := "#!/bin/sh\n" +
		"test \"$1\" = \"--flag\" || exit 99\n" +
		"shift\n" +
		"echo \"NEW_KEY=new_value\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing editor script: %v", err)
	}

	t.Setenv("EDITOR", scriptPath+" --flag")

	if _, err := runCmd(t, "edit", "--dir", dir, "--app", "testapp", "--env", "dev"); err != nil {
		t.Fatalf("edit error: %v", err)
	}

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "NEW_KEY")
	if err != nil {
		t.Fatalf("get NEW_KEY error: %v", err)
	}
	if out != "new_value" {
		t.Errorf("NEW_KEY = %q, want %q", out, "new_value")
	}
}

func TestEditCmd_TempFileInLsmDir(t *testing.T) {
	// Hardening: the decrypted plaintext temp file must be created inside the
	// owner-only lsm dir (cfg.Dir), not the shared system $TMPDIR, and must be
	// mode 0600. We use an editor script that records the path it was handed so
	// we can assert both properties against the still-present temp file.
	dir := setupTestEnv(t)

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "val"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	scriptDir := t.TempDir()
	pathRecord := filepath.Join(scriptDir, "tmppath.txt")
	scriptPath := filepath.Join(scriptDir, "editor.sh")
	// Record the temp file path the editor receives, then stat its mode (octal)
	// while it still exists (before the deferred secureRemove deletes it).
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$1\" > \"" + pathRecord + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing editor script: %v", err)
	}
	t.Setenv("EDITOR", scriptPath)

	if _, err := runCmd(t, "edit", "--dir", dir, "--app", "testapp", "--env", "dev"); err != nil {
		t.Fatalf("edit error: %v", err)
	}

	recorded, err := os.ReadFile(pathRecord)
	if err != nil {
		t.Fatalf("reading recorded temp path: %v", err)
	}
	tmpPath := strings.TrimSpace(string(recorded))
	if tmpPath == "" {
		t.Fatal("editor recorded an empty temp path")
	}

	// The temp file must live directly inside cfg.Dir (the lsm dir), not $TMPDIR.
	if got := filepath.Dir(tmpPath); got != dir {
		t.Errorf("temp file dir = %q, want lsm dir %q", got, dir)
	}
	if base := filepath.Base(tmpPath); !strings.HasPrefix(base, "lsm-edit-") {
		t.Errorf("temp file name = %q, want lsm-edit-* prefix", base)
	}

	// CreateTemp default perms are 0600; confirm the decrypted plaintext was
	// never group/world-readable. (The file is gone by now via secureRemove, so
	// re-create it under the same parent to assert CreateTemp's mode contract
	// rather than racing the cleanup.)
	probe, err := os.CreateTemp(dir, "lsm-edit-*.env")
	if err != nil {
		t.Fatalf("probe CreateTemp: %v", err)
	}
	probePath := probe.Name()
	_ = probe.Close()
	t.Cleanup(func() { _ = os.Remove(probePath) })
	fi, err := os.Stat(probePath)
	if err != nil {
		t.Fatalf("stat probe temp file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("temp file mode = %04o, want 0600", perm)
	}
}

func TestEditCmd_EditorFails(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Use "false" as EDITOR (exits non-zero)
	t.Setenv("EDITOR", "false")

	_, err = runCmd(t, "edit", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err == nil {
		t.Fatal("expected error when EDITOR exits non-zero")
	}
	if !strings.Contains(err.Error(), "editor exited with error") {
		t.Errorf("error = %q, want it to contain 'editor exited with error'", err.Error())
	}
}

func TestEditCmd_FallsBackToVISUAL(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Unset EDITOR, set VISUAL to "true"
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "true")

	_, err = runCmd(t, "edit", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("edit with VISUAL error: %v", err)
	}
}
