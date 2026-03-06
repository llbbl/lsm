// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"

	"github.com/llbbl/lsm/internal/crypto"
)

// entry represents a single line or item in the .env file,
// preserving comments and blank lines for round-trip fidelity.
type entry struct {
	// For key=value entries
	Key   string
	Value string
	// Raw line for comments/blanks (Key will be empty)
	Raw string
}

// Store manages secrets for a single app+env combination.
type Store struct {
	entries  []entry
	dir      string
	app      string
	env      string
	identity *age.X25519Identity
}

// New creates a new Store. It does not load from disk yet; call Load() for that.
func New(dir, app, env string, identity *age.X25519Identity) *Store {
	return &Store{
		dir:      dir,
		app:      app,
		env:      env,
		identity: identity,
	}
}

// FilePath returns the path to the encrypted .age file.
func (s *Store) FilePath() string {
	return filepath.Join(s.dir, fmt.Sprintf("%s.%s.age", s.app, s.env))
}

// Load reads and decrypts the .age file, parsing its contents.
// If the file doesn't exist, the store starts empty (no error).
func (s *Store) Load() error {
	path := s.FilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.entries = nil
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	plain, err := crypto.Decrypt(data, s.identity)
	if err != nil {
		return fmt.Errorf("decrypting %s: %w", path, err)
	}

	s.entries, err = ParseEnv(string(plain))
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

// Save encrypts and writes the store contents to the .age file.
func (s *Store) Save() error {
	content := Serialize(s.entries)
	encrypted, err := crypto.Encrypt([]byte(content), s.identity.Recipient())
	if err != nil {
		return fmt.Errorf("encrypting: %w", err)
	}
	path := s.FilePath()
	return os.WriteFile(path, encrypted, 0600)
}

// Get returns the value for the given key, and whether it exists.
func (s *Store) Get(key string) (string, bool) {
	for _, e := range s.entries {
		if e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

// Set adds or updates a key-value pair.
func (s *Store) Set(key, value string) {
	for i, e := range s.entries {
		if e.Key == key {
			s.entries[i].Value = value
			return
		}
	}
	s.entries = append(s.entries, entry{Key: key, Value: value})
}

// Delete removes a key. Returns true if the key existed.
func (s *Store) Delete(key string) bool {
	for i, e := range s.entries {
		if e.Key == key {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return true
		}
	}
	return false
}

// List returns all key names in order.
func (s *Store) List() []string {
	var keys []string
	for _, e := range s.entries {
		if e.Key != "" {
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// Dump returns all key-value pairs as a map.
func (s *Store) Dump() map[string]string {
	m := make(map[string]string)
	for _, e := range s.entries {
		if e.Key != "" {
			m[e.Key] = e.Value
		}
	}
	return m
}

// DumpOrdered returns keys in the order they appear.
func (s *Store) DumpOrdered() []entry {
	var result []entry
	for _, e := range s.entries {
		if e.Key != "" {
			result = append(result, e)
		}
	}
	return result
}

// Import merges key-value pairs from parsed .env content. Existing keys are overwritten.
func (s *Store) Import(content string) error {
	entries, err := ParseEnv(content)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Key != "" {
			s.Set(e.Key, e.Value)
		}
	}
	return nil
}

// SetRaw replaces the store contents with the given raw .env content.
func (s *Store) SetRaw(content string) error {
	entries, err := ParseEnv(content)
	if err != nil {
		return err
	}
	s.entries = entries
	return nil
}

// RawContent returns the serialized .env content.
func (s *Store) RawContent() string {
	return Serialize(s.entries)
}

// ParseEnv parses .env formatted content into entries.
// Supports comments (#), blank lines, unquoted values, single-quoted values,
// double-quoted values (with multiline support), and export prefix.
func ParseEnv(content string) ([]entry, error) {
	var entries []entry
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Blank line
		if trimmed == "" {
			entries = append(entries, entry{Raw: line})
			continue
		}

		// Comment
		if strings.HasPrefix(trimmed, "#") {
			entries = append(entries, entry{Raw: line})
			continue
		}

		// Strip optional "export " prefix
		parsed := trimmed
		if after, found := strings.CutPrefix(parsed, "export "); found {
			parsed = after
		}

		// Find the = separator
		eqIdx := strings.Index(parsed, "=")
		if eqIdx < 0 {
			// Line with no = sign, preserve as raw
			entries = append(entries, entry{Raw: line})
			continue
		}

		key := strings.TrimSpace(parsed[:eqIdx])
		val := parsed[eqIdx+1:]

		// Handle quoted values
		if strings.HasPrefix(val, `"`) {
			// Double-quoted: may span multiple lines
			raw := val[1:] // strip opening quote
			var fullVal strings.Builder
			for {
				// Look for closing quote
				closeIdx := strings.Index(raw, `"`)
				if closeIdx >= 0 {
					fullVal.WriteString(raw[:closeIdx])
					val = fullVal.String()
					break
				}
				// No closing quote on this line, consume and continue
				fullVal.WriteString(raw)
				fullVal.WriteString("\n")
				i++
				if i >= len(lines) {
					return nil, fmt.Errorf("unterminated double-quoted value for key %s", key)
				}
				raw = lines[i]
			}
		} else if strings.HasPrefix(val, "'") {
			// Single-quoted: value is everything between quotes
			endIdx := strings.LastIndex(val[1:], "'")
			if endIdx < 0 {
				return nil, fmt.Errorf("unterminated single-quoted value for key %s", key)
			}
			val = val[1 : endIdx+1]
		} else {
			// Unquoted: strip inline comments and trim
			if commentIdx := strings.Index(val, " #"); commentIdx >= 0 {
				val = val[:commentIdx]
			}
			val = strings.TrimSpace(val)
		}

		entries = append(entries, entry{Key: key, Value: val})
	}

	return entries, nil
}

// Serialize converts entries back to .env format.
func Serialize(entries []entry) string {
	var b strings.Builder
	for i, e := range entries {
		if e.Key == "" {
			b.WriteString(e.Raw)
		} else {
			b.WriteString(e.Key)
			b.WriteString("=")
			if strings.Contains(e.Value, "\n") || strings.Contains(e.Value, `"`) || strings.Contains(e.Value, "'") {
				b.WriteString(`"`)
				b.WriteString(e.Value)
				b.WriteString(`"`)
			} else {
				b.WriteString(e.Value)
			}
		}
		if i < len(entries)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ListApps returns sorted unique app names from .age files in the directory.
func ListApps(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	appSet := make(map[string]bool)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".age") {
			continue
		}
		name := strings.TrimSuffix(f.Name(), ".age")
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			appSet[parts[0]] = true
		}
	}

	var apps []string
	for app := range appSet {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	return apps, nil
}

// ListEnvs returns sorted environment names for the given app.
func ListEnvs(dir, app string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	prefix := app + "."
	var envs []string
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".age") {
			continue
		}
		name := strings.TrimSuffix(f.Name(), ".age")
		if after, found := strings.CutPrefix(name, prefix); found {
			env := after
			if env != "" {
				envs = append(envs, env)
			}
		}
	}
	sort.Strings(envs)
	return envs, nil
}
