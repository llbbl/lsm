// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"

	"github.com/llbbl/lsm/internal/config"
	"github.com/llbbl/lsm/internal/crypto"
	"github.com/llbbl/lsm/internal/store"
)

// resolveDir returns the lsm directory path.
func resolveDir() (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".lsm"), nil
}

// loadIdentity loads the age identity from the lsm directory.
func loadIdentity(dir string) (*age.X25519Identity, error) {
	keyPath := filepath.Join(dir, "key.txt")
	return crypto.LoadIdentity(keyPath)
}

// openStore creates, loads, and returns a Store for the given config.
func openStore(cfg *config.Config) (*store.Store, error) {
	id, err := loadIdentity(cfg.Dir)
	if err != nil {
		return nil, err
	}
	s := store.New(cfg.Dir, cfg.App, cfg.Env, id)
	if err := s.Load(); err != nil {
		return nil, err
	}
	return s, nil
}

// resolveWithPositional resolves config, consuming optional positional app and env args.
// It returns the resolved config and remaining args after consuming app/env.
// This handles the pattern: command [app] [env] <required-args...>
func resolveWithPositional(args []string, requiredCount int) (*config.Config, []string, error) {
	extra := len(args) - requiredCount

	posApp := flagApp
	posEnv := flagEnv

	var remaining []string

	switch {
	case extra >= 2 && flagApp == "" && flagEnv == "":
		posApp = args[0]
		posEnv = args[1]
		remaining = args[2:]
	case extra >= 1 && flagApp == "" && flagEnv == "":
		// Could be app only, try to resolve with it
		posApp = args[0]
		remaining = args[1:]
	case extra >= 1 && flagApp != "" && flagEnv == "":
		// App set by flag, extra could be env
		posEnv = args[0]
		remaining = args[1:]
	case extra >= 1 && flagApp == "" && flagEnv != "":
		// Env set by flag, extra could be app
		posApp = args[0]
		remaining = args[1:]
	default:
		remaining = args
	}

	cfg, err := config.Resolve(flagDir, posApp, posEnv)
	if err != nil {
		return nil, nil, err
	}
	return cfg, remaining, nil
}

// readInput reads from stdin if path is "-", otherwise reads the file at path.
func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// determineEditor returns the editor command from EDITOR, VISUAL, or "vi".
func determineEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	return "vi"
}

// maskValue masks a secret value for safe terminal display.
func maskValue(value string) string {
	n := len(value)
	switch {
	case n == 0:
		return ""
	case n <= 3:
		return strings.Repeat("*", n)
	case n <= 8:
		return value[:1] + strings.Repeat("*", n-1)
	case n <= 20:
		return value[:2] + strings.Repeat("*", n-2)
	default:
		return value[:2] + "********"
	}
}

// isTerminal checks if stdin is a terminal (character device).
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// isInGitRepo checks if the current directory is inside a git repository
// by walking up the directory tree looking for a .git directory.
func isInGitRepo() bool {
	dir, err := os.Getwd()
	if err != nil {
		return false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// ensureGitignored checks if filename is in the cwd's .gitignore.
// If not, it appends the filename. Returns true if .gitignore was modified.
// Only checks/modifies .gitignore in the current directory.
func ensureGitignored(filename string) (bool, error) {
	gitignorePath := filepath.Join(".", ".gitignore")

	// Read existing .gitignore
	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	content := string(data)

	// Check if filename is already listed (exact line match)
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == filename {
			return false, nil
		}
	}

	// Append to .gitignore
	var newContent string
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		newContent = content + "\n" + filename + "\n"
	} else {
		newContent = content + filename + "\n"
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// findEnvFiles looks for common .env files in the current directory.
func findEnvFiles() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(cwd)
	if err != nil {
		return nil, err
	}

	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ".env" || strings.HasPrefix(name, ".env.") {
			// Skip .env.example files — they typically don't contain real secrets
			if strings.HasSuffix(name, ".example") || strings.HasSuffix(name, ".sample") || strings.HasSuffix(name, ".template") {
				continue
			}
			found = append(found, filepath.Join(cwd, name))
		}
	}
	sort.Strings(found)
	return found, nil
}

// secureRemove overwrites a file with zeros before deleting it.
func secureRemove(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		// File already gone, nothing to do
		return nil
	}
	zeros := make([]byte, info.Size())
	if err := os.WriteFile(path, zeros, 0600); err != nil {
		return fmt.Errorf("overwriting temp file: %w", err)
	}
	return os.Remove(path)
}
