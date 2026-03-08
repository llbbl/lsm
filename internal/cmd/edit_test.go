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
