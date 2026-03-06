// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolve_FlagsOverrideAll(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Resolve(dir, "myapp", "production")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg.Dir != dir {
		t.Errorf("Dir = %q, want %q", cfg.Dir, dir)
	}
	if cfg.App != "myapp" {
		t.Errorf("App = %q, want %q", cfg.App, "myapp")
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want %q", cfg.Env, "production")
	}
}

func TestResolve_ProjectConfigOverridesDefaults(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	// Write global config
	os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("env: dev"), 0644)

	// Write project config
	os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("app: customapp\nenv: staging"), 0644)

	// Change to project dir
	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	cfg, err := Resolve(lsmDir, "", "")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg.App != "customapp" {
		t.Errorf("App = %q, want %q", cfg.App, "customapp")
	}
	if cfg.Env != "staging" {
		t.Errorf("Env = %q, want %q", cfg.Env, "staging")
	}
}

func TestResolve_GlobalConfigForEnv(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	// Write global config only
	os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("env: dev"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	cfg, err := Resolve(lsmDir, "", "")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	// App should be directory name
	if cfg.App != filepath.Base(projDir) {
		t.Errorf("App = %q, want %q", cfg.App, filepath.Base(projDir))
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
}

func TestResolve_FlagOverridesProjectConfig(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("app: projapp\nenv: staging"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	cfg, err := Resolve(lsmDir, "flagapp", "production")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg.App != "flagapp" {
		t.Errorf("App = %q, want %q", cfg.App, "flagapp")
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want %q", cfg.Env, "production")
	}
}

func TestResolve_NoEnvAvailable(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	_, err := Resolve(lsmDir, "app", "")
	if err == nil {
		t.Fatal("expected error when no env is available")
	}
}

func TestResolve_DefaultDir(t *testing.T) {
	// With no flags, dir defaults to ~/.lsm
	// We just test that it doesn't error with explicit app/env
	projDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	cfg, err := Resolve("", "testapp", "dev")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".lsm")
	if cfg.Dir != expected {
		t.Errorf("Dir = %q, want %q", cfg.Dir, expected)
	}
}

func TestSaveProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &ProjectConfig{App: "myapp", Env: "staging"}
	if err := SaveProjectConfig(dir, cfg); err != nil {
		t.Fatalf("SaveProjectConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".lsm.yaml"))
	if err != nil {
		t.Fatalf("reading .lsm.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "app: myapp") {
		t.Errorf("missing app in config: %s", content)
	}
	if !strings.Contains(content, "env: staging") {
		t.Errorf("missing env in config: %s", content)
	}
}

func TestLoadGlobalConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if cfg.Env != "" {
		t.Errorf("Env = %q, want empty", cfg.Env)
	}
}

func TestResolve_MalformedProjectConfig(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	// Write valid global config so env resolves
	os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("env: dev"), 0644)

	// Write malformed project .lsm.yaml (invalid YAML)
	os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("{{invalid yaml:::"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Should not crash; malformed project config is silently ignored
	cfg, err := Resolve(lsmDir, "", "")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	// App should fall back to directory name since project config failed to parse
	if cfg.App != filepath.Base(projDir) {
		t.Errorf("App = %q, want %q (directory name fallback)", cfg.App, filepath.Base(projDir))
	}
}

func TestResolve_MalformedGlobalConfig(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	// Write malformed global config.yaml
	os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("{{invalid yaml:::"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Should return error for malformed global config
	_, err := Resolve(lsmDir, "app", "")
	if err == nil {
		t.Fatal("expected error for malformed global config.yaml")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %q, want it to contain 'parsing'", err.Error())
	}
}

func TestSaveProjectConfig_VerifyContent(t *testing.T) {
	dir := t.TempDir()
	cfg := &ProjectConfig{App: "webapp", Env: "production"}
	if err := SaveProjectConfig(dir, cfg); err != nil {
		t.Fatalf("SaveProjectConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".lsm.yaml"))
	if err != nil {
		t.Fatalf("reading .lsm.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "app: webapp") {
		t.Errorf("missing 'app: webapp' in: %s", content)
	}
	if !strings.Contains(content, "env: production") {
		t.Errorf("missing 'env: production' in: %s", content)
	}

	// Verify it can be loaded back
	var loaded ProjectConfig
	loadedData, _ := os.ReadFile(filepath.Join(dir, ".lsm.yaml"))
	if err := yaml.Unmarshal(loadedData, &loaded); err != nil {
		t.Fatalf("failed to parse saved config: %v", err)
	}
	if loaded.App != "webapp" || loaded.Env != "production" {
		t.Errorf("loaded config = {%q, %q}, want {webapp, production}", loaded.App, loaded.Env)
	}
}

func TestLoadGlobalConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("{{bad yaml"), 0644)

	_, err := LoadGlobalConfig(dir)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoadGlobalConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: production"), 0644)

	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want %q", cfg.Env, "production")
	}
}
