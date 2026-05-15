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
