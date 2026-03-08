// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"length 1", "a", "*"},
		{"length 2", "ab", "**"},
		{"length 3", "abc", "***"},
		{"length 4", "abcd", "a***"},
		{"length 6", "abcdef", "a*****"},
		{"length 8", "abcdefgh", "a*******"},
		{"length 9", "abcdefghi", "ab*******"},
		{"length 15", "abcdefghijklmno", "ab*************"},
		{"length 20", "abcdefghijklmnopqrst", "ab******************"},
		{"length 21", "abcdefghijklmnopqrstu", "ab********"},
		{"length 50", strings.Repeat("x", 50), "xx********"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskValue(tt.input)
			if got != tt.want {
				t.Errorf("maskValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDump_WritesFileAndMaskedOutput(t *testing.T) {
	dir := setupTestEnv(t)
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

	// Set some secrets.
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "dev", "DB_HOST", "localhost"); err != nil {
		t.Fatalf("set DB_HOST error: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "dev", "API_TOKEN", "sk-1234567890abcdef"); err != nil {
		t.Fatalf("set API_TOKEN error: %v", err)
	}

	out, err := runCmd(t, "dump", "--dir", dir, "--app", "myapp", "--env", "dev")
	if err != nil {
		t.Fatalf("dump error: %v", err)
	}

	// Terminal output should contain masked values.
	if strings.Contains(out, "localhost") {
		t.Errorf("terminal output should not contain real value 'localhost': %s", out)
	}
	if strings.Contains(out, "sk-1234567890abcdef") {
		t.Errorf("terminal output should not contain real value 'API_TOKEN': %s", out)
	}
	if !strings.Contains(out, "DB_HOST=") {
		t.Errorf("terminal output missing DB_HOST key: %s", out)
	}
	if !strings.Contains(out, "API_TOKEN=") {
		t.Errorf("terminal output missing API_TOKEN key: %s", out)
	}
	if !strings.Contains(out, "Wrote 2 secrets to myapp.dev.env") {
		t.Errorf("missing write confirmation: %s", out)
	}

	// File should contain real unmasked values.
	filePath := filepath.Join(outDir, "myapp.dev.env")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "DB_HOST=localhost") {
		t.Errorf("file missing real DB_HOST value: %s", content)
	}
	if !strings.Contains(content, "API_TOKEN=sk-1234567890abcdef") {
		t.Errorf("file missing real API_TOKEN value: %s", content)
	}
}

func TestDump_CustomOutput(t *testing.T) {
	dir := setupTestEnv(t)
	outDir := t.TempDir()
	customPath := filepath.Join(outDir, "custom-output.env")

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "dev", "SECRET", "hunter2"); err != nil {
		t.Fatalf("set SECRET error: %v", err)
	}

	out, err := runCmd(t, "dump", "--dir", dir, "--app", "myapp", "--env", "dev", "--output", customPath)
	if err != nil {
		t.Fatalf("dump error: %v", err)
	}

	if !strings.Contains(out, customPath) {
		t.Errorf("output should reference custom path %q: %s", customPath, out)
	}

	data, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("reading custom output file: %v", err)
	}
	if !strings.Contains(string(data), "SECRET=hunter2") {
		t.Errorf("custom output file missing real value: %s", string(data))
	}
}

func TestDump_EmptyStore(t *testing.T) {
	dir := setupTestEnv(t)
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

	out, err := runCmd(t, "dump", "--dir", dir, "--app", "emptyapp", "--env", "dev")
	if err != nil {
		t.Fatalf("dump error: %v", err)
	}

	if !strings.Contains(out, "No secrets to dump") {
		t.Errorf("expected empty store message, got: %s", out)
	}

	// No file should be created.
	filePath := filepath.Join(outDir, "emptyapp.dev.env")
	if _, err := os.Stat(filePath); err == nil {
		t.Error("file should not be created for empty store")
	}
}
