// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromFile(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a .env file to import
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("IMPORTED_KEY=imported_value\nOTHER=test\n"), 0644); err != nil {
		t.Fatalf("writing env file: %v", err)
	}

	_, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev", envFile)
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	// Verify imported
	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "IMPORTED_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "imported_value" {
		t.Errorf("imported value = %q, want %q", out, "imported_value")
	}
}

func TestImportFromStdin(t *testing.T) {
	dir := setupTestEnv(t)

	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("PIPED_KEY=piped_value\nOTHER_KEY=other\n")
		_ = w.Close()
	}()

	_, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev", "-")
	if err != nil {
		t.Fatalf("import from stdin error: %v", err)
	}

	os.Stdin = origStdin

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "PIPED_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "piped_value" {
		t.Errorf("got %q, want %q", out, "piped_value")
	}
}

func TestSetFromStdin(t *testing.T) {
	dir := setupTestEnv(t)

	// Replace stdin with a pipe
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("stdin_value")
		_ = w.Close()
	}()

	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "STDIN_KEY", "-")
	if err != nil {
		t.Fatalf("set from stdin error: %v", err)
	}

	// Restore stdin for get
	os.Stdin = origStdin

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "STDIN_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "stdin_value" {
		t.Errorf("got %q, want %q", out, "stdin_value")
	}
}

// TestImportMerge verifies that import merges into existing secrets.
func TestImportMerge(t *testing.T) {
	dir := setupTestEnv(t)

	// Set an existing key
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "EXISTING", "keep_me")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Import a file with a new key
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("NEW_KEY=new_value\n"), 0644); err != nil {
		t.Fatalf("writing env file: %v", err)
	}

	_, err = runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev", envFile)
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	// Both keys should exist
	out, _ := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "EXISTING")
	if !strings.Contains(out, "keep_me") {
		t.Errorf("existing key lost after import: %q", out)
	}
	out, _ = runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "NEW_KEY")
	if !strings.Contains(out, "new_value") {
		t.Errorf("imported key missing: %q", out)
	}
}
