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
	if err := os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("env: dev"), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	// Write project config
	if err := os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("app: customapp\nenv: staging"), 0644); err != nil {
		t.Fatalf("writing project config: %v", err)
	}

	// Change to project dir
	t.Chdir(projDir)

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
	if err := os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("env: dev"), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	t.Chdir(projDir)

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

	if err := os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("app: projapp\nenv: staging"), 0644); err != nil {
		t.Fatalf("writing project config: %v", err)
	}

	t.Chdir(projDir)

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

	t.Chdir(projDir)

	_, err := Resolve(lsmDir, "app", "")
	if err == nil {
		t.Fatal("expected error when no env is available")
	}
}

func TestResolve_DefaultDir(t *testing.T) {
	// With no flags, dir defaults to ~/.lsm
	// We just test that it doesn't error with explicit app/env
	projDir := t.TempDir()
	t.Chdir(projDir)

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
	if err := os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("env: dev"), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	// Write malformed project .lsm.yaml (invalid YAML)
	if err := os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("{{invalid yaml:::"), 0644); err != nil {
		t.Fatalf("writing project config: %v", err)
	}

	t.Chdir(projDir)

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
	if err := os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte("{{invalid yaml:::"), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	t.Chdir(projDir)

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
	loadedData, err := os.ReadFile(filepath.Join(dir, ".lsm.yaml"))
	if err != nil {
		t.Fatalf("reading .lsm.yaml for reload: %v", err)
	}
	if err := yaml.Unmarshal(loadedData, &loaded); err != nil {
		t.Fatalf("failed to parse saved config: %v", err)
	}
	if loaded.App != "webapp" || loaded.Env != "production" {
		t.Errorf("loaded config = {%q, %q}, want {webapp, production}", loaded.App, loaded.Env)
	}
}

func TestLoadGlobalConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("{{bad yaml"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	_, err := LoadGlobalConfig(dir)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoadGlobalConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: production"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want %q", cfg.Env, "production")
	}
}

func TestSaveGlobalConfig_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	original := &GlobalConfig{
		Env: "dev",
		Apps: map[string]string{
			"webapp":  "/home/user/projects/webapp",
			"api-svc": "/home/user/projects/api",
		},
	}

	if err := SaveGlobalConfig(dir, original); err != nil {
		t.Fatalf("SaveGlobalConfig() error: %v", err)
	}

	loaded, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if loaded.Env != original.Env {
		t.Errorf("Env = %q, want %q", loaded.Env, original.Env)
	}
	if len(loaded.Apps) != len(original.Apps) {
		t.Fatalf("Apps length = %d, want %d", len(loaded.Apps), len(original.Apps))
	}
	for k, v := range original.Apps {
		if loaded.Apps[k] != v {
			t.Errorf("Apps[%q] = %q, want %q", k, loaded.Apps[k], v)
		}
	}
}

func TestSaveGlobalConfig_NilApps(t *testing.T) {
	dir := t.TempDir()
	cfg := &GlobalConfig{Env: "prod"}

	if err := SaveGlobalConfig(dir, cfg); err != nil {
		t.Fatalf("SaveGlobalConfig() error: %v", err)
	}

	loaded, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig() error: %v", err)
	}
	if loaded.Env != "prod" {
		t.Errorf("Env = %q, want %q", loaded.Env, "prod")
	}
	if len(loaded.Apps) != 0 {
		t.Errorf("Apps = %v, want empty", loaded.Apps)
	}
}

func TestLoadGlobalConfig_NoGitHubBlock_NilMap(t *testing.T) {
	// Back-compat: an old config with no `github:` key must read cleanly and
	// leave the GitHub map nil (not an error).
	dir := t.TempDir()
	content := "env: dev\napps:\n  webapp: /home/user/webapp\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.GitHub != nil {
		t.Errorf("GitHub = %v, want nil for a config with no github block", cfg.GitHub)
	}
	// A lookup into the nil map must be safe and miss.
	if _, ok := cfg.GitHub["webapp"]; ok {
		t.Errorf("unexpected github marker for webapp")
	}
}

func TestLoadGlobalConfig_WithGitHubBlock(t *testing.T) {
	dir := t.TempDir()
	content := `env: dev
github:
  webapp:
    repo: llbbl/lsm
    target: actions
    last_pushed: "2026-06-20T10:00:00Z"
    last_count: 3
  api:
    repo: acme/api
    target: env:production
    last_pushed: "2026-06-19T09:00:00Z"
    last_count: 1
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	web, ok := cfg.GitHub["webapp"]
	if !ok {
		t.Fatalf("missing github marker for webapp: %v", cfg.GitHub)
	}
	if web.Repo != "llbbl/lsm" || web.Target != "actions" || web.LastCount != 3 {
		t.Errorf("webapp marker = %+v", web)
	}
	if web.LastPushed != "2026-06-20T10:00:00Z" {
		t.Errorf("webapp LastPushed = %q", web.LastPushed)
	}
	api := cfg.GitHub["api"]
	if api.Target != "env:production" || api.Repo != "acme/api" || api.LastCount != 1 {
		t.Errorf("api marker = %+v", api)
	}
}

func TestSetGitHubLink_CreatesAndPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	// Seed an existing config with env + apps but no github block.
	content := "env: dev\napps:\n  webapp: /home/user/webapp\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	link := GitHubLink{Repo: "llbbl/lsm", Target: "actions", LastPushed: "2026-06-20T10:00:00Z", LastCount: 2}
	if err := SetGitHubLink(dir, "webapp", link); err != nil {
		t.Fatalf("SetGitHubLink: %v", err)
	}

	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	// Pre-existing fields preserved.
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.Apps["webapp"] != "/home/user/webapp" {
		t.Errorf("Apps not preserved: %v", cfg.Apps)
	}
	// New marker written.
	got := cfg.GitHub["webapp"]
	if got != link {
		t.Errorf("github marker = %+v, want %+v", got, link)
	}

	// A second SetGitHubLink for another app must not clobber the first.
	link2 := GitHubLink{Repo: "acme/api", Target: "env:prod", LastPushed: "2026-06-21T11:00:00Z", LastCount: 5}
	if err := SetGitHubLink(dir, "api", link2); err != nil {
		t.Fatalf("SetGitHubLink api: %v", err)
	}
	cfg, err = LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.GitHub["webapp"] != link {
		t.Errorf("first marker clobbered: %+v", cfg.GitHub["webapp"])
	}
	if cfg.GitHub["api"] != link2 {
		t.Errorf("second marker = %+v, want %+v", cfg.GitHub["api"], link2)
	}
}

func TestResolveAppFromRegistry_MatchFound(t *testing.T) {
	cfg := &GlobalConfig{
		Apps: map[string]string{
			"webapp": "/home/user/webapp",
			"api":    "/home/user/api",
		},
	}
	got := ResolveAppFromRegistry(cfg, "/home/user/api")
	if got != "api" {
		t.Errorf("ResolveAppFromRegistry() = %q, want %q", got, "api")
	}
}

func TestResolveAppFromRegistry_NoMatch(t *testing.T) {
	cfg := &GlobalConfig{
		Apps: map[string]string{
			"webapp": "/home/user/webapp",
		},
	}
	got := ResolveAppFromRegistry(cfg, "/home/user/other")
	if got != "" {
		t.Errorf("ResolveAppFromRegistry() = %q, want empty", got)
	}
}

func TestResolveAppFromRegistry_NilMap(t *testing.T) {
	cfg := &GlobalConfig{}
	got := ResolveAppFromRegistry(cfg, "/some/path")
	if got != "" {
		t.Errorf("ResolveAppFromRegistry() = %q, want empty", got)
	}
}

func TestResolveAppFromRegistry_EmptyMap(t *testing.T) {
	cfg := &GlobalConfig{Apps: map[string]string{}}
	got := ResolveAppFromRegistry(cfg, "/some/path")
	if got != "" {
		t.Errorf("ResolveAppFromRegistry() = %q, want empty", got)
	}
}

func TestResolve_RegistryLookup(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	// Resolve symlinks to match what Resolve() will see for cwd
	resolvedProjDir, err := filepath.EvalSymlinks(projDir)
	if err != nil {
		t.Fatalf("EvalSymlinks() error: %v", err)
	}

	// Write global config with apps registry
	configContent := "env: dev\napps:\n  myregisteredapp: " + resolvedProjDir + "\n"
	if err := os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	t.Chdir(projDir)

	cfg, err := Resolve(lsmDir, "", "")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg.App != "myregisteredapp" {
		t.Errorf("App = %q, want %q", cfg.App, "myregisteredapp")
	}
}

func TestResolve_ProjectConfigOverridesRegistry(t *testing.T) {
	lsmDir := t.TempDir()
	projDir := t.TempDir()

	// Resolve symlinks to match what Resolve() will see for cwd
	resolvedProjDir, err := filepath.EvalSymlinks(projDir)
	if err != nil {
		t.Fatalf("EvalSymlinks() error: %v", err)
	}

	// Write global config with registry entry for this dir
	configContent := "env: dev\napps:\n  registryapp: " + resolvedProjDir + "\n"
	if err := os.WriteFile(filepath.Join(lsmDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	// Write project config that should take priority
	if err := os.WriteFile(filepath.Join(projDir, ".lsm.yaml"), []byte("app: projapp\nenv: staging"), 0644); err != nil {
		t.Fatalf("writing project config: %v", err)
	}

	t.Chdir(projDir)

	cfg, err := Resolve(lsmDir, "", "")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	// Project config should win over registry
	if cfg.App != "projapp" {
		t.Errorf("App = %q, want %q (project config should override registry)", cfg.App, "projapp")
	}
}

func TestLoadGlobalConfig_WithLogBlock(t *testing.T) {
	dir := t.TempDir()
	content := "env: dev\nlog:\n  level: debug\n  dest: stderr\n  format: json\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Dest != "stderr" {
		t.Errorf("Log.Dest = %q, want %q", cfg.Log.Dest, "stderr")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}

	// Round-trip: marshal it back and ensure the log block is preserved.
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "log:") {
		t.Errorf("expected marshaled config to contain log block, got %q", out)
	}
}

func TestLoadGlobalConfig_WithOTLPBlock(t *testing.T) {
	dir := t.TempDir()
	content := `env: dev
otlp:
  enabled: true
  endpoint: https://otlp.example.com/v1/logs
  headers:
    Authorization: Bearer xxx
  batch_size: 50
  batch_window: 3s
  queue_cap: 500
  max_retries: 5
  retry_base_delay: 250ms
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if !cfg.OTLP.Enabled {
		t.Errorf("OTLP.Enabled = false, want true")
	}
	if cfg.OTLP.Endpoint != "https://otlp.example.com/v1/logs" {
		t.Errorf("OTLP.Endpoint = %q", cfg.OTLP.Endpoint)
	}
	if cfg.OTLP.Headers["Authorization"] != "Bearer xxx" {
		t.Errorf("OTLP.Headers = %v", cfg.OTLP.Headers)
	}
	if cfg.OTLP.BatchSize != 50 {
		t.Errorf("OTLP.BatchSize = %d, want 50", cfg.OTLP.BatchSize)
	}
	if cfg.OTLP.BatchWindow != "3s" {
		t.Errorf("OTLP.BatchWindow = %q, want %q", cfg.OTLP.BatchWindow, "3s")
	}
	if cfg.OTLP.QueueCap != 500 {
		t.Errorf("OTLP.QueueCap = %d, want 500", cfg.OTLP.QueueCap)
	}
	if cfg.OTLP.MaxRetries != 5 {
		t.Errorf("OTLP.MaxRetries = %d, want 5", cfg.OTLP.MaxRetries)
	}
	if cfg.OTLP.RetryBaseDelay != "250ms" {
		t.Errorf("OTLP.RetryBaseDelay = %q, want %q", cfg.OTLP.RetryBaseDelay, "250ms")
	}
}

func TestLoadGlobalConfig_MissingOTLPBlock_DefaultsZero(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: dev\n"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	zero := OTLPConfig{}
	if cfg.OTLP.Enabled != zero.Enabled ||
		cfg.OTLP.Endpoint != zero.Endpoint ||
		cfg.OTLP.BatchSize != zero.BatchSize ||
		cfg.OTLP.BatchWindow != zero.BatchWindow ||
		cfg.OTLP.QueueCap != zero.QueueCap ||
		cfg.OTLP.MaxRetries != zero.MaxRetries ||
		cfg.OTLP.RetryBaseDelay != zero.RetryBaseDelay ||
		len(cfg.OTLP.Headers) != 0 {
		t.Errorf("OTLP not zero: %+v", cfg.OTLP)
	}
}

func TestLoadGlobalConfig_MissingLogBlock_DefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: dev\n"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if cfg.Log != (LogConfig{}) {
		t.Errorf("Log = %+v, want zero LogConfig", cfg.Log)
	}
}
