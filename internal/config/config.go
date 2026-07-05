// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the resolved configuration for an lsm operation.
type Config struct {
	Dir string // Path to the lsm directory (e.g., ~/.lsm)
	App string // App name
	Env string // Environment name
}

// GlobalConfig represents ~/.lsm/config.yaml
type GlobalConfig struct {
	Env    string                `yaml:"env"`
	Apps   map[string]string     `yaml:"apps,omitempty"` // app name -> absolute path
	Log    LogConfig             `yaml:"log,omitempty"`
	OTLP   OTLPConfig            `yaml:"otlp,omitempty"`
	GitHub map[string]GitHubLink `yaml:"github,omitempty"` // app name -> last successful gh push marker
}

// GitHubLink is a durable, per-app marker recording the last successful
// `lsm gh push`. It is a LOCAL HINT only: the live GitHub secrets list is
// always authoritative (GitHub's secrets API is write-only, so values can
// never be read back to verify the marker). It deliberately stores no secret
// NAMES or VALUES — names already live in the audit log; only repo/target/
// timestamp/count are recorded here.
type GitHubLink struct {
	Repo       string `yaml:"repo"`                  // OWNER/REPO the push targeted
	Target     string `yaml:"target"`                // "actions" (repo level) or "env:<name>"
	LastPushed string `yaml:"last_pushed,omitempty"` // RFC3339 timestamp of the push
	LastCount  int    `yaml:"last_count,omitempty"`  // number of secrets set in that push
}

// LogConfig holds the dlog (developer/flow-tracing log) configuration.
// All fields default sensibly when unset; see internal/dlog for the
// accepted values and defaults.
type LogConfig struct {
	Level  string `yaml:"level,omitempty"`
	Dest   string `yaml:"dest,omitempty"`
	Format string `yaml:"format,omitempty"`
}

// OTLPConfig holds configuration for the audit OTLP/HTTP remote sink.
// Duration-shaped fields (BatchWindow, RetryBaseDelay) are stored as
// strings here and parsed by the sink-construction layer; this keeps
// the YAML surface friendly ("5s") without coupling the config package
// to time.Duration.
type OTLPConfig struct {
	Enabled        bool              `yaml:"enabled,omitempty"`
	Endpoint       string            `yaml:"endpoint,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	BatchSize      int               `yaml:"batch_size,omitempty"`
	BatchWindow    string            `yaml:"batch_window,omitempty"`
	QueueCap       int               `yaml:"queue_cap,omitempty"`
	MaxRetries     int               `yaml:"max_retries,omitempty"`
	RetryBaseDelay string            `yaml:"retry_base_delay,omitempty"`
}

// ProjectConfig represents .lsm.yaml in a project directory.
type ProjectConfig struct {
	App string `yaml:"app"`
	Env string `yaml:"env"`
}

// Resolve determines the final Config using the priority chain:
// 1. CLI flags (flagDir, flagApp, flagEnv)
// 2. .lsm.yaml in current directory
// 3. Registry lookup by current directory path, ~/.lsm/config.yaml -> default env
func Resolve(flagDir, flagApp, flagEnv string) (*Config, error) {
	cfg := &Config{}

	// Resolve dir
	if flagDir != "" {
		cfg.Dir = flagDir
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		cfg.Dir = filepath.Join(home, ".lsm")
	}

	// Load project config (.lsm.yaml in cwd)
	var projCfg ProjectConfig
	projCfgLoaded := false
	cwd, err := os.Getwd()
	if err == nil {
		// Resolve symlinks so registry paths match (e.g., /tmp -> /private/tmp on macOS)
		if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
			cwd = resolved
		}
		projPath := filepath.Join(cwd, ".lsm.yaml")
		if data, err := os.ReadFile(projPath); err == nil {
			if err := yaml.Unmarshal(data, &projCfg); err == nil {
				projCfgLoaded = true
			}
		}
	}

	// Load global config
	var globalCfg GlobalConfig
	globalPath := filepath.Join(cfg.Dir, "config.yaml")
	if data, err := os.ReadFile(globalPath); err == nil {
		if err := yaml.Unmarshal(data, &globalCfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", globalPath, err)
		}
	}

	// Resolve app: flag > project config > registry lookup.
	if flagApp != "" {
		cfg.App = flagApp
	} else if projCfgLoaded && projCfg.App != "" {
		cfg.App = projCfg.App
	} else if regApp := ResolveAppFromRegistry(&globalCfg, cwd); regApp != "" {
		cfg.App = regApp
	} else {
		return nil, fmt.Errorf("cannot determine app name: run 'lsm link <app>' in this project, pass --app, or create .lsm.yaml")
	}

	// Resolve env: flag > project config > global config
	if flagEnv != "" {
		cfg.Env = flagEnv
	} else if projCfgLoaded && projCfg.Env != "" {
		cfg.Env = projCfg.Env
	} else if globalCfg.Env != "" {
		cfg.Env = globalCfg.Env
	} else {
		return nil, fmt.Errorf("cannot determine environment: use --env flag, create .lsm.yaml, or set env in %s", globalPath)
	}

	return cfg, nil
}

// LoadGlobalConfig reads the global config from the lsm directory.
func LoadGlobalConfig(dir string) (*GlobalConfig, error) {
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, err
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// SaveGlobalConfig writes the global config.yaml in the lsm directory.
func SaveGlobalConfig(dir string, cfg *GlobalConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0600)
}

// SetGitHubLink records (or replaces) the per-app GitHub push marker in the
// global config and persists it. It performs a load-modify-save against the
// config.yaml in dir so callers don't have to manage the round-trip or worry
// about clobbering a nil GitHub map. Any pre-existing config (env, apps, log,
// otlp, other github entries) is preserved.
func SetGitHubLink(dir, app string, link GitHubLink) error {
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		return err
	}
	if cfg.GitHub == nil {
		cfg.GitHub = make(map[string]GitHubLink)
	}
	cfg.GitHub[app] = link
	return SaveGlobalConfig(dir, cfg)
}

// ResolveAppFromRegistry performs a reverse lookup on the Apps map,
// returning the app name whose registered path matches cwd exactly.
// Returns empty string if no match is found.
func ResolveAppFromRegistry(cfg *GlobalConfig, cwd string) string {
	for app, path := range cfg.Apps {
		if path == cwd {
			return app
		}
	}
	return ""
}

// SaveProjectConfig writes a .lsm.yaml file in the given directory.
func SaveProjectConfig(dir string, cfg *ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".lsm.yaml"), data, 0644)
}
