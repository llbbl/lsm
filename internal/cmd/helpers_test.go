// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
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
