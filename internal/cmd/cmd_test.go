// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/crypto"
)

// setupTestEnv creates a temp lsm directory with a generated key and returns
// the dir path and a cleanup function. It also sets up flags.
func setupTestEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Generate identity
	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("generating identity: %v", err)
	}
	if err := crypto.SaveIdentity(filepath.Join(dir, "key.txt"), id); err != nil {
		t.Fatalf("saving identity: %v", err)
	}

	// Create config.yaml
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: dev\n"), 0644); err != nil {
		t.Fatalf("writing config.yaml: %v", err)
	}

	return dir
}

// runCmd executes a command and returns stdout output.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	rootCmd := NewRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	// Reset flags before each run
	flagDir = ""
	flagApp = ""
	flagEnv = ""

	err := rootCmd.Execute()
	return buf.String(), err
}

func TestSetAndGet(t *testing.T) {
	dir := setupTestEnv(t)

	// Set a value
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "MY_KEY", "my_value")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Get the value
	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "MY_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "my_value" {
		t.Errorf("get output = %q, want %q", out, "my_value")
	}
}

func TestSetAndGet_WithPositionalArgs(t *testing.T) {
	dir := setupTestEnv(t)

	// Set with positional app and env
	_, err := runCmd(t, "set", "--dir", dir, "myapp", "dev", "DB_URL", "postgres://localhost")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Get with positional app and env
	out, err := runCmd(t, "get", "--dir", dir, "myapp", "dev", "DB_URL")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "postgres://localhost" {
		t.Errorf("get output = %q, want %q", out, "postgres://localhost")
	}
}

func TestGet_NotFound(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "NOPE")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestDelete(t *testing.T) {
	dir := setupTestEnv(t)

	// Set then delete
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DEL_KEY", "value"); err != nil {
		t.Fatalf("set error: %v", err)
	}
	_, err := runCmd(t, "delete", "--dir", dir, "--app", "testapp", "--env", "dev", "DEL_KEY")
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}

	// Verify deleted
	_, err = runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "DEL_KEY")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDelete_NotFound(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "delete", "--dir", dir, "--app", "testapp", "--env", "dev", "NOPE")
	if err == nil {
		t.Fatal("expected error deleting nonexistent key")
	}
}

func TestList(t *testing.T) {
	dir := setupTestEnv(t)

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY1", "val1"); err != nil {
		t.Fatalf("set KEY1 error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY2", "val2"); err != nil {
		t.Fatalf("set KEY2 error: %v", err)
	}

	out, err := runCmd(t, "list", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}

	if !strings.Contains(out, "KEY1") || !strings.Contains(out, "KEY2") {
		t.Errorf("list output missing keys: %s", out)
	}
}

func TestDump(t *testing.T) {
	dir := setupTestEnv(t)

	// Use a temp directory for output files so we don't pollute cwd.
	outDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(outDir); err != nil {
		t.Fatalf("chdir to outDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DB_URL", "postgres://localhost"); err != nil {
		t.Fatalf("set DB_URL error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "API_KEY", "secret"); err != nil {
		t.Fatalf("set API_KEY error: %v", err)
	}

	out, err := runCmd(t, "dump", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("dump error: %v", err)
	}

	// Terminal output should have masked values, not real ones.
	if strings.Contains(out, "postgres://localhost") {
		t.Errorf("dump terminal output should not contain real value: %s", out)
	}
	if !strings.Contains(out, "DB_URL=") {
		t.Errorf("dump missing DB_URL key: %s", out)
	}
	if !strings.Contains(out, "Wrote 2 secrets") {
		t.Errorf("dump missing write confirmation: %s", out)
	}
}

func TestApps(t *testing.T) {
	dir := setupTestEnv(t)

	// Create stores for two apps
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "app1", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set app1 error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "app2", "--env", "prod", "K", "V"); err != nil {
		t.Fatalf("set app2 error: %v", err)
	}

	out, err := runCmd(t, "apps", "--dir", dir)
	if err != nil {
		t.Fatalf("apps error: %v", err)
	}

	if !strings.Contains(out, "app1") || !strings.Contains(out, "app2") {
		t.Errorf("apps output = %q, want app1 and app2", out)
	}
}

func TestEnvs(t *testing.T) {
	dir := setupTestEnv(t)

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set dev error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "production", "K", "V"); err != nil {
		t.Fatalf("set production error: %v", err)
	}

	out, err := runCmd(t, "envs", "--dir", dir, "myapp")
	if err != nil {
		t.Fatalf("envs error: %v", err)
	}

	if !strings.Contains(out, "dev") || !strings.Contains(out, "production") {
		t.Errorf("envs output = %q, want dev and production", out)
	}
}

func TestLink(t *testing.T) {
	lsmDir := setupTestEnv(t)
	projDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatalf("chdir to projDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	out, err := runCmd(t, "link", "--dir", lsmDir, "myapp")
	if err != nil {
		t.Fatalf("link error: %v", err)
	}

	if !strings.Contains(out, "Linked myapp") {
		t.Errorf("unexpected output: %s", out)
	}
	if !strings.Contains(out, projDir) {
		t.Errorf("output missing path %q: %s", projDir, out)
	}

	// Verify config.yaml has the app entry
	data, err := os.ReadFile(filepath.Join(lsmDir, "config.yaml"))
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "myapp") {
		t.Errorf("config.yaml missing app name: %s", content)
	}
	if !strings.Contains(content, projDir) {
		t.Errorf("config.yaml missing path %q: %s", projDir, content)
	}
}

func TestLink_RemovesDuplicatePath(t *testing.T) {
	lsmDir := setupTestEnv(t)
	projDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatalf("chdir to projDir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: chdir back: %v", err)
		}
	}()

	// Link as "oldname"
	_, err := runCmd(t, "link", "--dir", lsmDir, "oldname")
	if err != nil {
		t.Fatalf("first link error: %v", err)
	}

	// Re-link same directory as "newname"
	_, err = runCmd(t, "link", "--dir", lsmDir, "newname")
	if err != nil {
		t.Fatalf("second link error: %v", err)
	}

	// Verify "oldname" is removed, "newname" exists
	data, err := os.ReadFile(filepath.Join(lsmDir, "config.yaml"))
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "oldname") {
		t.Errorf("config.yaml still contains oldname: %s", content)
	}
	if !strings.Contains(content, "newname") {
		t.Errorf("config.yaml missing newname: %s", content)
	}
}

func TestSetUpdate(t *testing.T) {
	dir := setupTestEnv(t)

	// Set initial
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "first"); err != nil {
		t.Fatalf("set initial error: %v", err)
	}
	// Update
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "second"); err != nil {
		t.Fatalf("set update error: %v", err)
	}

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "second" {
		t.Errorf("get after update = %q, want %q", out, "second")
	}
}

func TestMultipleEnvs(t *testing.T) {
	dir := setupTestEnv(t)

	// Set in dev
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DB", "dev_db"); err != nil {
		t.Fatalf("set dev error: %v", err)
	}
	// Set in prod
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "production", "DB", "prod_db"); err != nil {
		t.Fatalf("set prod error: %v", err)
	}

	// Get dev
	out, _ := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "DB")
	if out != "dev_db" {
		t.Errorf("dev DB = %q, want %q", out, "dev_db")
	}

	// Get prod
	out, _ = runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "production", "DB")
	if out != "prod_db" {
		t.Errorf("production DB = %q, want %q", out, "prod_db")
	}
}

func TestResolveWithPositional_SingleExtraArg(t *testing.T) {
	dir := setupTestEnv(t)

	// With one extra arg and no flags, it should be treated as app
	// Set with explicit flags first
	_, err := runCmd(t, "set", "--dir", dir, "--app", "customapp", "--env", "dev", "POS_KEY", "pos_val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Get using positional app (single extra arg should be treated as app)
	out, err := runCmd(t, "get", "--dir", dir, "--env", "dev", "customapp", "POS_KEY")
	if err != nil {
		t.Fatalf("get with positional app error: %v", err)
	}
	if out != "pos_val" {
		t.Errorf("got %q, want %q", out, "pos_val")
	}
}
