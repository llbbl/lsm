// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSalt_GenerateAndReread(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.salt")

	first, err := LoadOrCreateSalt(path)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(first) != SaltSize {
		t.Fatalf("first salt length = %d, want %d", len(first), SaltSize)
	}

	second, err := LoadOrCreateSalt(path)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("salt changed between calls")
	}
}

func TestLoadOrCreateSalt_FileMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.salt")

	if _, err := LoadOrCreateSalt(path); err != nil {
		t.Fatalf("LoadOrCreateSalt: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadOrCreateSalt_WrongSizeRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.salt")
	if err := os.WriteFile(path, []byte("too-short"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := LoadOrCreateSalt(path)
	if err == nil {
		t.Fatal("expected error for wrong-size salt")
	}
}

func TestLoadOrCreateSalt_WeakenedModeRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.salt")
	// 32 bytes, 0644 mode.
	buf := make([]byte, SaltSize)
	for i := range buf {
		buf[i] = byte(i)
	}
	if err := os.WriteFile(path, buf, 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := LoadOrCreateSalt(path)
	if err == nil {
		t.Fatal("expected error for weakened-mode salt")
	}
}

// TestLoadOrCreateSalt_ModeAcceptance exercises the mode predicate
// (`mode & 0o077 == 0`): any user-only mode is accepted (including
// stricter-than-0600 modes such as 0400, 0500, 0700, which merely drop
// permission bits lsm doesn't need post-creation); any mode granting
// group or other a permission bit is rejected as weakened.
func TestLoadOrCreateSalt_ModeAcceptance(t *testing.T) {
	cases := []struct {
		name    string
		mode    os.FileMode
		wantErr bool
	}{
		{"0400_read_only_owner", 0400, false},
		{"0500_read_exec_owner", 0500, false},
		{"0600_default", 0600, false},
		{"0700_full_owner", 0700, false},
		{"0640_group_read", 0640, true},
		{"0644_world_read", 0644, true},
		{"0660_group_rw", 0660, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "observability.salt")
			buf := make([]byte, SaltSize)
			for i := range buf {
				buf[i] = byte(i)
			}
			if err := os.WriteFile(path, buf, c.mode); err != nil {
				t.Fatalf("seed: %v", err)
			}
			// WriteFile honors umask; force the exact mode.
			if err := os.Chmod(path, c.mode); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			got, err := LoadOrCreateSalt(path)
			if c.wantErr {
				if err == nil {
					t.Fatalf("LoadOrCreateSalt(%#o): expected error, got nil", c.mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadOrCreateSalt(%#o): unexpected error: %v", c.mode, err)
			}
			if !bytes.Equal(got, buf) {
				t.Errorf("salt mismatch after read")
			}
		})
	}
}
