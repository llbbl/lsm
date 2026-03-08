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

func TestImport_AutoDetectSingleFile(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Create a single .env file in the working directory
	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("AUTO_KEY=auto_value\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir to workDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	out, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	if !strings.Contains(out, "Found") {
		t.Errorf("output missing 'Found': %s", out)
	}
	if !strings.Contains(out, "Reminder: delete") {
		t.Errorf("output missing reminder: %s", out)
	}

	// Verify the secret was imported
	got, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "AUTO_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got != "auto_value" {
		t.Errorf("got %q, want %q", got, "auto_value")
	}
}

func TestImport_AutoDetectMultipleFiles(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("K=V\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".env.local"), []byte("K2=V2\n"), 0644); err != nil {
		t.Fatalf("writing .env.local: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir to workDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	out, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err == nil {
		t.Fatal("expected error for multiple .env files")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "specify which file") {
		t.Errorf("error missing 'specify which file': %v", err)
	}
	if !strings.Contains(out, "Multiple .env files found") {
		t.Errorf("output missing 'Multiple .env files found': %s", out)
	}
	if !strings.Contains(out, ".env") || !strings.Contains(out, ".env.local") {
		t.Errorf("output missing filenames: %s", out)
	}
}

func TestImport_AutoDetectNoFiles(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir to workDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	_, err = runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err == nil {
		t.Fatal("expected error for no .env files")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "no .env files found") {
		t.Errorf("error missing 'no .env files found': %v", err)
	}
}

func TestImport_AutoDetectSkipsExample(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Create .env and .env.example — only .env should be picked up
	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("REAL_KEY=real_value\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".env.example"), []byte("EXAMPLE_KEY=placeholder\n"), 0644); err != nil {
		t.Fatalf("writing .env.example: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir to workDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	out, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	if !strings.Contains(out, "Found") {
		t.Errorf("output missing 'Found': %s", out)
	}

	// Verify only the real key was imported
	got, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "REAL_KEY")
	if err != nil {
		t.Fatalf("get REAL_KEY error: %v", err)
	}
	if got != "real_value" {
		t.Errorf("got %q, want %q", got, "real_value")
	}

	// EXAMPLE_KEY should not exist
	_, err = runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "EXAMPLE_KEY")
	if err == nil {
		t.Error("expected error for EXAMPLE_KEY — .env.example should have been skipped")
	}
}

func TestImport_ExplicitFileShowsReminder(t *testing.T) {
	dir := setupTestEnv(t)

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("EXPLICIT_KEY=explicit_value\n"), 0644); err != nil {
		t.Fatalf("writing env file: %v", err)
	}

	out, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev", envFile)
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	if !strings.Contains(out, "Reminder: delete") {
		t.Errorf("output missing reminder: %s", out)
	}
	if !strings.Contains(out, "Imported") {
		t.Errorf("output missing 'Imported': %s", out)
	}
}

func TestImport_StdinNoReminder(t *testing.T) {
	dir := setupTestEnv(t)

	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("STDIN_KEY=stdin_value\n")
		_ = w.Close()
	}()

	out, err := runCmd(t, "import", "--dir", dir, "--app", "testapp", "--env", "dev", "-")
	if err != nil {
		t.Fatalf("import from stdin error: %v", err)
	}

	os.Stdin = origStdin

	if strings.Contains(out, "Reminder: delete") {
		t.Errorf("stdin import should NOT show reminder, got: %s", out)
	}
	if !strings.Contains(out, "Imported") {
		t.Errorf("output missing 'Imported': %s", out)
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
