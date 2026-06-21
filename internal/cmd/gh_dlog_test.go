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

// enableDlogToFile rewrites the lsm dir's config.yaml log block so dlog emits
// debug lines (JSON) to the given file path, preserving the rest of the config
// (env, apps registry, etc.). Returns nothing; callers read the file after the
// command runs.
func enableDlogToFile(t *testing.T, dir, logPath string) {
	t.Helper()
	cfg, err := config.LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	cfg.Log = config.LogConfig{Level: "debug", Dest: "file:" + logPath, Format: "json"}
	if err := config.SaveGlobalConfig(dir, cfg); err != nil {
		t.Fatalf("SaveGlobalConfig: %v", err)
	}
}

func TestGhPush_DlogTraceLines_NoValuesLeak(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"DB_URL":  "postgres://localhost",
		"API_KEY": "sk-super-secret-value",
	})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	logPath := filepath.Join(t.TempDir(), "dlog.jsonl")
	enableDlogToFile(t, dir, logPath)

	if _, err := runCmd(t, "gh", "push", "--dir", dir, "--force"); err != nil {
		t.Fatalf("gh push error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading dlog file: %v", err)
	}
	logContent := string(data)

	// Trace lines we expect along the push path.
	wantMsgs := []string{
		"gh resolved",
		"gh push resolved target",
		"gh secret set ok",
		"gh marker written",
	}
	for _, m := range wantMsgs {
		if !strings.Contains(logContent, m) {
			t.Errorf("dlog output missing trace line %q:\n%s", m, logContent)
		}
	}

	// Secret NAMES are allowed in logs; VALUES must NEVER appear.
	for _, value := range []string{"postgres://localhost", "sk-super-secret-value"} {
		if strings.Contains(logContent, value) {
			t.Errorf("secret VALUE %q leaked into dlog output:\n%s", value, logContent)
		}
	}
}

func TestGhStatus_DlogTraceLines(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"BOTH": "v1"})
	resp := map[string]string{
		"secret list": `[{"name":"BOTH","updatedAt":"2026-05-05T10:00:00Z"}]`,
	}
	fakeGh(t, resp)
	fakeOrigin(t, "https://github.com/llbbl/lsm.git")
	forceInteractive(t)

	logPath := filepath.Join(t.TempDir(), "dlog.jsonl")
	enableDlogToFile(t, dir, logPath)

	if _, err := runCmd(t, "gh", "status", "--dir", dir); err != nil {
		t.Fatalf("gh status error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading dlog file: %v", err)
	}
	logContent := string(data)
	for _, m := range []string{"gh status resolved target", "gh status remote listed"} {
		if !strings.Contains(logContent, m) {
			t.Errorf("dlog output missing status trace line %q:\n%s", m, logContent)
		}
	}
}
