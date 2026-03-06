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
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: dev\n"), 0644)

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

func TestInitCmd(t *testing.T) {
	dir := t.TempDir()

	out, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	if !strings.Contains(out, "Created key at") {
		t.Errorf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Public key:") {
		t.Errorf("missing public key in output: %s", out)
	}

	// Verify key file exists
	if _, err := os.Stat(filepath.Join(dir, "key.txt")); err != nil {
		t.Errorf("key.txt not created: %v", err)
	}

	// Verify config.yaml created
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Errorf("config.yaml not created: %v", err)
	}
}

func TestInitCmd_AlreadyExists(t *testing.T) {
	dir := t.TempDir()

	// First init
	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("first init error: %v", err)
	}

	// Second init should fail
	_, err = runCmd(t, "init", "--dir", dir)
	if err == nil {
		t.Fatal("expected error on second init without --force")
	}
}

func TestInitCmd_Force(t *testing.T) {
	dir := t.TempDir()

	_, _ = runCmd(t, "init", "--dir", dir)

	// Force should work
	_, err := runCmd(t, "init", "--dir", dir, "--force")
	if err != nil {
		t.Fatalf("init --force error: %v", err)
	}
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
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DEL_KEY", "value")
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

	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY1", "val1")
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY2", "val2")

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

	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DB_URL", "postgres://localhost")
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "API_KEY", "secret")

	out, err := runCmd(t, "dump", "--dir", dir, "--app", "testapp", "--env", "dev")
	if err != nil {
		t.Fatalf("dump error: %v", err)
	}

	if !strings.Contains(out, "DB_URL=postgres://localhost") {
		t.Errorf("dump missing DB_URL: %s", out)
	}
	if !strings.Contains(out, "API_KEY=secret") {
		t.Errorf("dump missing API_KEY: %s", out)
	}
}

func TestImportFromFile(t *testing.T) {
	dir := setupTestEnv(t)

	// Create a .env file to import
	envFile := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(envFile, []byte("IMPORTED_KEY=imported_value\nOTHER=test\n"), 0644)

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

func TestApps(t *testing.T) {
	dir := setupTestEnv(t)

	// Create stores for two apps
	runCmd(t, "set", "--dir", dir, "--app", "app1", "--env", "dev", "K", "V")
	runCmd(t, "set", "--dir", dir, "--app", "app2", "--env", "prod", "K", "V")

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

	runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "dev", "K", "V")
	runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "production", "K", "V")

	out, err := runCmd(t, "envs", "--dir", dir, "myapp")
	if err != nil {
		t.Fatalf("envs error: %v", err)
	}

	if !strings.Contains(out, "dev") || !strings.Contains(out, "production") {
		t.Errorf("envs output = %q, want dev and production", out)
	}
}

func TestLink(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	out, err := runCmd(t, "link", "myapp", "staging")
	if err != nil {
		t.Fatalf("link error: %v", err)
	}

	if !strings.Contains(out, "Created .lsm.yaml") {
		t.Errorf("unexpected output: %s", out)
	}

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(dir, ".lsm.yaml"))
	if err != nil {
		t.Fatalf("reading .lsm.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "app: myapp") {
		t.Errorf(".lsm.yaml missing app: %s", content)
	}
	if !strings.Contains(content, "env: staging") {
		t.Errorf(".lsm.yaml missing env: %s", content)
	}
}

func TestSetUpdate(t *testing.T) {
	dir := setupTestEnv(t)

	// Set initial
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "first")
	// Update
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "second")

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
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DB", "dev_db")
	// Set in prod
	runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "production", "DB", "prod_db")

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

func TestSetFromStdin(t *testing.T) {
	dir := setupTestEnv(t)

	// Replace stdin with a pipe
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		w.WriteString("stdin_value")
		w.Close()
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

func TestImportFromStdin(t *testing.T) {
	dir := setupTestEnv(t)

	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		w.WriteString("PIPED_KEY=piped_value\nOTHER_KEY=other\n")
		w.Close()
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
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf '\\nNEW_KEY=new_val\\n' >> \"$1\"\n"), 0755)

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

func TestExecCmd_MissingSeparator(t *testing.T) {
	dir := setupTestEnv(t)

	// Save original os.Args and restore after
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Simulate: lsm exec --dir <dir> --app testapp --env dev echo hello
	// (no -- separator)
	os.Args = []string{"lsm", "exec", "--dir", dir, "--app", "testapp", "--env", "dev", "echo", "hello"}

	_, err := runCmd(t, "exec", "--dir", dir, "--app", "testapp", "--env", "dev", "echo", "hello")
	if err == nil {
		t.Fatal("expected error when -- separator is missing")
	}
}

func TestExecCmd_CommandNotFound(t *testing.T) {
	dir := setupTestEnv(t)

	// Set a secret so the store loads
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Simulate: lsm exec --dir <dir> --app testapp --env dev -- nonexistent_command_xyz
	os.Args = []string{"lsm", "exec", "--dir", dir, "--app", "testapp", "--env", "dev", "--", "nonexistent_command_xyz_12345"}

	_, err = runCmd(t, "exec", "--dir", dir, "--app", "testapp", "--env", "dev", "nonexistent_command_xyz_12345")
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("error = %q, want it to contain 'command not found'", err.Error())
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
