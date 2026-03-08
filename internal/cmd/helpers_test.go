// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/config"
)

func TestResolveDir_Default(t *testing.T) {
	// Reset global flag
	origFlagDir := flagDir
	flagDir = ""
	defer func() { flagDir = origFlagDir }()

	dir, err := resolveDir()
	if err != nil {
		t.Fatalf("resolveDir() error: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".lsm")
	if dir != expected {
		t.Errorf("resolveDir() = %q, want %q", dir, expected)
	}
}

func TestResolveDir_WithFlag(t *testing.T) {
	origFlagDir := flagDir
	flagDir = "/tmp/custom-lsm"
	defer func() { flagDir = origFlagDir }()

	dir, err := resolveDir()
	if err != nil {
		t.Fatalf("resolveDir() error: %v", err)
	}
	if dir != "/tmp/custom-lsm" {
		t.Errorf("resolveDir() = %q, want %q", dir, "/tmp/custom-lsm")
	}
}

func TestOpenStore_MissingKeyFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Dir: dir,
		App: "testapp",
		Env: "dev",
	}

	_, err := openStore(cfg)
	if err == nil {
		t.Fatal("expected error when key file is missing")
	}
}

func TestResolveWithPositional_TwoExtraArgs(t *testing.T) {
	// Save and reset flags
	origApp := flagApp
	origEnv := flagEnv
	origDir := flagDir
	defer func() {
		flagApp = origApp
		flagEnv = origEnv
		flagDir = origDir
	}()

	// Set up a temp dir with valid config
	dir := setupTestEnv(t)
	flagDir = dir
	flagApp = ""
	flagEnv = ""

	// args = [app, env, key] with requiredCount=1 => extra=2, consumes app+env
	args := []string{"myapp", "dev", "MY_KEY"}
	cfg, remaining, err := resolveWithPositional(args, 1)
	if err != nil {
		t.Fatalf("resolveWithPositional() error: %v", err)
	}

	if cfg.App != "myapp" {
		t.Errorf("App = %q, want %q", cfg.App, "myapp")
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
	if len(remaining) != 1 || remaining[0] != "MY_KEY" {
		t.Errorf("remaining = %v, want [MY_KEY]", remaining)
	}
}

func TestResolveWithPositional_FlagsAlreadySet(t *testing.T) {
	origApp := flagApp
	origEnv := flagEnv
	origDir := flagDir
	defer func() {
		flagApp = origApp
		flagEnv = origEnv
		flagDir = origDir
	}()

	dir := setupTestEnv(t)
	flagDir = dir
	flagApp = "flagapp"
	flagEnv = "dev"

	// With both flags set and requiredCount=1, extra=0 => default case, no consumption
	args := []string{"MY_KEY"}
	cfg, remaining, err := resolveWithPositional(args, 1)
	if err != nil {
		t.Fatalf("resolveWithPositional() error: %v", err)
	}

	if cfg.App != "flagapp" {
		t.Errorf("App = %q, want %q", cfg.App, "flagapp")
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
	if len(remaining) != 1 || remaining[0] != "MY_KEY" {
		t.Errorf("remaining = %v, want [MY_KEY]", remaining)
	}
}

func TestResolveWithPositional_AppFlagSet_EnvPositional(t *testing.T) {
	origApp := flagApp
	origEnv := flagEnv
	origDir := flagDir
	defer func() {
		flagApp = origApp
		flagEnv = origEnv
		flagDir = origDir
	}()

	dir := setupTestEnv(t)
	flagDir = dir
	flagApp = "flagapp"
	flagEnv = ""

	// App set by flag, one extra arg should be treated as env
	// args = [env, key] with requiredCount=1 => extra=1, app flag set, env not set
	args := []string{"staging", "MY_KEY"}
	cfg, remaining, err := resolveWithPositional(args, 1)
	if err != nil {
		t.Fatalf("resolveWithPositional() error: %v", err)
	}

	if cfg.App != "flagapp" {
		t.Errorf("App = %q, want %q", cfg.App, "flagapp")
	}
	if cfg.Env != "staging" {
		t.Errorf("Env = %q, want %q", cfg.Env, "staging")
	}
	if len(remaining) != 1 || remaining[0] != "MY_KEY" {
		t.Errorf("remaining = %v, want [MY_KEY]", remaining)
	}
}

func TestResolveWithPositional_EnvFlagSet_AppPositional(t *testing.T) {
	origApp := flagApp
	origEnv := flagEnv
	origDir := flagDir
	defer func() {
		flagApp = origApp
		flagEnv = origEnv
		flagDir = origDir
	}()

	dir := setupTestEnv(t)
	flagDir = dir
	flagApp = ""
	flagEnv = "dev"

	// Env set by flag, one extra arg should be treated as app
	args := []string{"posapp", "MY_KEY"}
	cfg, remaining, err := resolveWithPositional(args, 1)
	if err != nil {
		t.Fatalf("resolveWithPositional() error: %v", err)
	}

	if cfg.App != "posapp" {
		t.Errorf("App = %q, want %q", cfg.App, "posapp")
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
	if len(remaining) != 1 || remaining[0] != "MY_KEY" {
		t.Errorf("remaining = %v, want [MY_KEY]", remaining)
	}
}

func TestReadInput_FromFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(f, []byte("file content"), 0644); err != nil {
		t.Fatalf("writing input file: %v", err)
	}

	data, err := readInput(f)
	if err != nil {
		t.Fatalf("readInput() error: %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("got %q, want %q", string(data), "file content")
	}
}

func TestReadInput_FromStdin(t *testing.T) {
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("piped data")
		_ = w.Close()
	}()

	data, err := readInput("-")
	if err != nil {
		t.Fatalf("readInput('-') error: %v", err)
	}
	if string(data) != "piped data" {
		t.Errorf("got %q, want %q", string(data), "piped data")
	}
}

func TestReadInput_FileNotFound(t *testing.T) {
	_, err := readInput("/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDetermineEditor(t *testing.T) {
	t.Run("uses EDITOR", func(t *testing.T) {
		t.Setenv("EDITOR", "nano")
		t.Setenv("VISUAL", "code")

		if got := determineEditor(); got != "nano" {
			t.Errorf("got %q, want %q", got, "nano")
		}
	})

	t.Run("falls back to VISUAL", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		t.Setenv("VISUAL", "code")

		if got := determineEditor(); got != "code" {
			t.Errorf("got %q, want %q", got, "code")
		}
	})

	t.Run("defaults to vi", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		t.Setenv("VISUAL", "")

		if got := determineEditor(); got != "vi" {
			t.Errorf("got %q, want %q", got, "vi")
		}
	})
}

func TestSecureRemove(t *testing.T) {
	t.Run("overwrites and deletes file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "secret.txt")
		if err := os.WriteFile(f, []byte("sensitive data"), 0600); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		if err := secureRemove(f); err != nil {
			t.Fatalf("secureRemove() error: %v", err)
		}

		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Error("file still exists after secureRemove")
		}
	})

	t.Run("no error if file already gone", func(t *testing.T) {
		if err := secureRemove("/nonexistent/path/gone.txt"); err != nil {
			t.Errorf("secureRemove() on missing file: %v", err)
		}
	})
}

func TestEnsureGitignored_NewFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	added, err := ensureGitignored(".env")
	if err != nil {
		t.Fatalf("ensureGitignored: %v", err)
	}
	if !added {
		t.Error("expected .gitignore to be created")
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(data), ".env") {
		t.Errorf(".gitignore missing .env: %s", data)
	}
}

func TestEnsureGitignored_AlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(".gitignore", []byte(".env\nnode_modules\n"), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	added, err := ensureGitignored(".env")
	if err != nil {
		t.Fatalf("ensureGitignored: %v", err)
	}
	if added {
		t.Error("should not modify .gitignore when entry exists")
	}
}

func TestEnsureGitignored_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(".gitignore", []byte("node_modules\n"), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	added, err := ensureGitignored(".env")
	if err != nil {
		t.Fatalf("ensureGitignored: %v", err)
	}
	if !added {
		t.Error("expected .env to be added")
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "node_modules") {
		t.Error("lost existing content")
	}
	if !strings.Contains(content, ".env") {
		t.Error("missing .env entry")
	}
}

func TestIsInGitRepo(t *testing.T) {
	// Current directory (the lsm project) should be a git repo
	if !isInGitRepo() {
		t.Error("expected to be in a git repo")
	}
}

func TestIsInGitRepo_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if isInGitRepo() {
		t.Error("temp dir should not be a git repo")
	}
}
