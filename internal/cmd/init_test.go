// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/crypto"
)

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

func TestInitCmd_CreatesNestedDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep", "path")

	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init with nested path error: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("nested directory not created: %v", err)
	}

	// Verify key was created
	if _, err := os.Stat(filepath.Join(dir, "key.txt")); err != nil {
		t.Errorf("key.txt not created in nested directory: %v", err)
	}
}

func TestInitCmd_KeyFilePermissions(t *testing.T) {
	dir := t.TempDir()

	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	keyPath := filepath.Join(dir, "key.txt")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key.txt: %v", err)
	}

	// Key should be readable only by owner (0600)
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("key.txt permissions = %o, want 0600", mode)
	}
}

func TestInitCmd_DirectoryPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lsm-test")

	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}

	// Directory should be 0700
	mode := info.Mode().Perm()
	if mode != 0700 {
		t.Errorf("directory permissions = %o, want 0700", mode)
	}
}

func TestInitCmd_ConfigYamlContent(t *testing.T) {
	dir := t.TempDir()

	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}

	if string(data) != "env: dev\n" {
		t.Errorf("config.yaml content = %q, want %q", string(data), "env: dev\n")
	}
}

func TestInitCmd_PreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()

	// Create directory first
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("creating directory: %v", err)
	}

	// Create existing config with custom content
	configPath := filepath.Join(dir, "config.yaml")
	existingConfig := "env: production\n"
	if err := os.WriteFile(configPath, []byte(existingConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	// Config should be unchanged
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	if string(data) != existingConfig {
		t.Errorf("config.yaml was overwritten, got %q, want %q", string(data), existingConfig)
	}
}

func TestInitCmd_GeneratesValidKey(t *testing.T) {
	dir := t.TempDir()

	_, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	// Try to load the key
	keyPath := filepath.Join(dir, "key.txt")
	identity, err := crypto.LoadIdentity(keyPath)
	if err != nil {
		t.Fatalf("loading generated key: %v", err)
	}

	// Verify we can get the recipient (public key)
	recipient := identity.Recipient()
	if recipient.String() == "" {
		t.Error("generated key has empty public key")
	}

	// Verify public key starts with age1
	if !strings.HasPrefix(recipient.String(), "age1") {
		t.Errorf("public key should start with age1, got: %s", recipient.String())
	}
}

func TestInitCmd_OutputFormat(t *testing.T) {
	dir := t.TempDir()

	out, err := runCmd(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init error: %v", err)
	}

	expectedKeyPath := filepath.Join(dir, "key.txt")
	if !strings.Contains(out, expectedKeyPath) {
		t.Errorf("output should contain full key path %q, got: %s", expectedKeyPath, out)
	}

	// Public key should start with age1
	if !strings.Contains(out, "age1") {
		t.Errorf("output should contain age1 public key, got: %s", out)
	}
}

func TestInitCmd_ForceGeneratesNewKey(t *testing.T) {
	dir := t.TempDir()

	out1, _ := runCmd(t, "init", "--dir", dir)
	out2, err := runCmd(t, "init", "--dir", dir, "--force")
	if err != nil {
		t.Fatalf("init --force error: %v", err)
	}

	// Extract public keys from output
	key1 := extractPublicKey(out1)
	key2 := extractPublicKey(out2)

	if key1 == "" || key2 == "" {
		t.Fatalf("could not extract public keys from output")
	}

	if key1 == key2 {
		t.Error("--force should generate a new key, but got the same public key")
	}
}

// extractPublicKey extracts the age1... public key from init command output
func extractPublicKey(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		if after, ok := strings.CutPrefix(line, "Public key:"); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}
