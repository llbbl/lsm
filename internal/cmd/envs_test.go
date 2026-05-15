// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvs_NoArg_UsesProjectConfig verifies that `lsm envs` with no positional
// arg resolves the app from a .lsm.yaml in the current working directory.
func TestEnvs_NoArg_UsesProjectConfig(t *testing.T) {
	dir := setupTestEnv(t)

	// Seed two envs for "myapp" in the lsm dir.
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set dev error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "production", "K", "V"); err != nil {
		t.Fatalf("set production error: %v", err)
	}

	// Create a project directory with a .lsm.yaml pointing at "myapp".
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("app: myapp\nenv: dev\n"), 0644); err != nil {
		t.Fatalf("writing .lsm.yaml: %v", err)
	}
	t.Chdir(projDir)

	out, err := runCmd(t, "envs", "--dir", dir)
	if err != nil {
		t.Fatalf("envs error: %v", err)
	}

	if !strings.Contains(out, "dev") || !strings.Contains(out, "production") {
		t.Errorf("envs output = %q, want dev and production", out)
	}
}

// TestEnvs_NoArg_UsesRegistry verifies that `lsm envs` with no positional arg
// resolves the app from the global registry (apps map) when cwd matches.
func TestEnvs_NoArg_UsesRegistry(t *testing.T) {
	lsmDir := setupTestEnv(t)
	projDir := t.TempDir()
	t.Chdir(projDir)

	// Link cwd to "registeredapp".
	if _, err := runCmd(t, "link", "--dir", lsmDir, "registeredapp"); err != nil {
		t.Fatalf("link error: %v", err)
	}

	// Seed envs for that app.
	if _, err := runCmd(t, "set", "--dir", lsmDir, "--app", "registeredapp", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set dev error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", lsmDir, "--app", "registeredapp", "--env", "staging", "K", "V"); err != nil {
		t.Fatalf("set staging error: %v", err)
	}

	out, err := runCmd(t, "envs", "--dir", lsmDir)
	if err != nil {
		t.Fatalf("envs error: %v", err)
	}

	if !strings.Contains(out, "dev") || !strings.Contains(out, "staging") {
		t.Errorf("envs output = %q, want dev and staging", out)
	}
}

// TestEnvs_ExplicitPositional verifies the original `lsm envs APP` form still works.
func TestEnvs_ExplicitPositional(t *testing.T) {
	dir := setupTestEnv(t)

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "explicitapp", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set dev error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "explicitapp", "--env", "production", "K", "V"); err != nil {
		t.Fatalf("set production error: %v", err)
	}

	// cwd must have a resolvable app/env for Resolve to succeed when the
	// positional consumes only "app" — env still comes from global config.yaml.
	t.Chdir(t.TempDir())

	out, err := runCmd(t, "envs", "--dir", dir, "explicitapp")
	if err != nil {
		t.Fatalf("envs error: %v", err)
	}

	if !strings.Contains(out, "dev") || !strings.Contains(out, "production") {
		t.Errorf("envs output = %q, want dev and production", out)
	}
}
