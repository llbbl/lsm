// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClean_RemovesFiles(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Set secrets in the store
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DB_URL", "postgres://localhost"); err != nil {
		t.Fatalf("set error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "API_KEY", "secret123"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Create .env with the same keys
	envPath := filepath.Join(workDir, ".env")
	if err := os.WriteFile(envPath, []byte("DB_URL=postgres://localhost\nAPI_KEY=secret123\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Chdir(workDir)

	out, err := runCmd(t, "clean", "--dir", dir, "--app", "testapp", "--env", "dev", "--force")
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if !strings.Contains(out, "Removed .env") {
		t.Errorf("output missing 'Removed .env': %s", out)
	}

	// Verify file is gone
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Errorf(".env should have been removed")
	}
}

func TestClean_SkipsMissingKeys(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Set only one key in the store
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DB_URL", "postgres://localhost"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Create .env with a key NOT in the store
	envPath := filepath.Join(workDir, ".env")
	if err := os.WriteFile(envPath, []byte("DB_URL=postgres://localhost\nNOT_IN_STORE=missing\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Chdir(workDir)

	out, err := runCmd(t, "clean", "--dir", dir, "--app", "testapp", "--env", "dev", "--force")
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if !strings.Contains(out, "Skipping") {
		t.Errorf("output missing 'Skipping': %s", out)
	}
	if !strings.Contains(out, "NOT_IN_STORE") {
		t.Errorf("output missing missing key name: %s", out)
	}
	if !strings.Contains(out, "Nothing to remove") {
		t.Errorf("output missing 'Nothing to remove': %s", out)
	}

	// Verify file still exists
	if _, err := os.Stat(envPath); err != nil {
		t.Errorf(".env should still exist: %v", err)
	}
}

func TestClean_NoEnvFiles(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	t.Chdir(workDir)

	out, err := runCmd(t, "clean", "--dir", dir, "--app", "testapp", "--env", "dev", "--force")
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if !strings.Contains(out, "No .env files found") {
		t.Errorf("output missing 'No .env files found': %s", out)
	}
}

func TestClean_SkipsExampleFiles(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Set secret
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "REAL_KEY", "value"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Create .env and .env.example
	envPath := filepath.Join(workDir, ".env")
	examplePath := filepath.Join(workDir, ".env.example")
	if err := os.WriteFile(envPath, []byte("REAL_KEY=value\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if err := os.WriteFile(examplePath, []byte("EXAMPLE_KEY=placeholder\n"), 0644); err != nil {
		t.Fatalf("writing .env.example: %v", err)
	}

	t.Chdir(workDir)

	out, err := runCmd(t, "clean", "--dir", dir, "--app", "testapp", "--env", "dev", "--force")
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if !strings.Contains(out, "Removed .env") {
		t.Errorf("output missing 'Removed .env': %s", out)
	}

	// .env should be gone
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Errorf(".env should have been removed")
	}

	// .env.example should still exist
	if _, err := os.Stat(examplePath); err != nil {
		t.Errorf(".env.example should still exist: %v", err)
	}
}

func TestClean_PartialMatch(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Set two keys in the store
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "STORED_KEY", "value1"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Create .env with one stored key and one missing key
	envPath := filepath.Join(workDir, ".env")
	if err := os.WriteFile(envPath, []byte("STORED_KEY=value1\nMISSING_KEY=value2\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Chdir(workDir)

	out, err := runCmd(t, "clean", "--dir", dir, "--app", "testapp", "--env", "dev", "--force")
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	// Should be skipped
	if !strings.Contains(out, "Skipping") {
		t.Errorf("output missing 'Skipping': %s", out)
	}
	if !strings.Contains(out, "MISSING_KEY") {
		t.Errorf("output missing 'MISSING_KEY': %s", out)
	}

	// File should still exist
	if _, err := os.Stat(envPath); err != nil {
		t.Errorf(".env should still exist: %v", err)
	}
}

func TestClean_MultipleFiles(t *testing.T) {
	dir := setupTestEnv(t)
	workDir := t.TempDir()

	// Set secrets
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY1", "val1"); err != nil {
		t.Fatalf("set error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY2", "val2"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Create two .env files — both safe
	env1 := filepath.Join(workDir, ".env")
	env2 := filepath.Join(workDir, ".env.local")
	if err := os.WriteFile(env1, []byte("KEY1=val1\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if err := os.WriteFile(env2, []byte("KEY2=val2\n"), 0644); err != nil {
		t.Fatalf("writing .env.local: %v", err)
	}

	t.Chdir(workDir)

	out, err := runCmd(t, "clean", "--dir", dir, "--app", "testapp", "--env", "dev", "--force")
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if !strings.Contains(out, "Removed .env") {
		t.Errorf("output missing 'Removed .env': %s", out)
	}
	if !strings.Contains(out, "Removed .env.local") {
		t.Errorf("output missing 'Removed .env.local': %s", out)
	}

	// Both should be gone
	if _, err := os.Stat(env1); !os.IsNotExist(err) {
		t.Errorf(".env should have been removed")
	}
	if _, err := os.Stat(env2); !os.IsNotExist(err) {
		t.Errorf(".env.local should have been removed")
	}
}
