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
	Env  string            `yaml:"env"`
	Apps map[string]string `yaml:"apps,omitempty"` // app name -> absolute path
}

// ProjectConfig represents .lsm.yaml in a project directory.
type ProjectConfig struct {
	App string `yaml:"app"`
	Env string `yaml:"env"`
}

// Resolve determines the final Config using the priority chain:
// 1. CLI flags (flagDir, flagApp, flagEnv)
// 2. .lsm.yaml in current directory
// 3. Directory name -> app, ~/.lsm/config.yaml -> default env
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

	// Resolve app: flag > project config > registry lookup > directory name
	if flagApp != "" {
		cfg.App = flagApp
	} else if projCfgLoaded && projCfg.App != "" {
		cfg.App = projCfg.App
	} else if regApp := ResolveAppFromRegistry(&globalCfg, cwd); regApp != "" {
		cfg.App = regApp
	} else if cwd != "" {
		cfg.App = filepath.Base(cwd)
	} else {
		return nil, fmt.Errorf("cannot determine app name: use --app flag or create .lsm.yaml")
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
