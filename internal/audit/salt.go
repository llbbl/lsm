// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"crypto/rand"
	"fmt"
	"os"
)

// SaltSize is the length, in bytes, of the per-host observability salt
// stored at ~/.lsm/observability.salt.
const SaltSize = 32

// LoadOrCreateSalt reads or generates the per-host observability salt
// at path. The file is written with mode 0600. If the file does not
// exist, 32 random bytes are generated and persisted atomically (write
// to <path>.tmp then rename).
//
// Existing files must be exactly SaltSize bytes long and must have a
// mode that grants NO permission bits to group or other. Concretely,
// the check is `mode & 0o077 == 0`. Accepts 0400, 0500, 0600 (which
// lsm itself writes), and 0700 — stricter modes are not weakened, they
// just remove write access lsm doesn't need after creation. Rejects
// 0640, 0644, 0660, 0666, etc.
//
// Wrong size or weakened modes (any group/other bit set) are surfaced
// as errors rather than silently regenerating: regenerating the salt
// invalidates dashboard hash continuity (existing app/env label hashes
// stop matching), and that decision must be explicit.
func LoadOrCreateSalt(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err == nil {
		// File exists — validate size and mode before trusting it.
		if info.Size() != int64(SaltSize) {
			return nil, fmt.Errorf("audit: salt at %s has wrong size %d (want %d); refusing to use", path, info.Size(), SaltSize)
		}
		mode := info.Mode().Perm()
		// Reject any mode that grants group or other any permission bit.
		// Stricter-than-0600 modes (0400, 0500, 0700) are fine — they
		// just remove write/exec we don't need after creation.
		if mode&0o077 != 0 {
			return nil, fmt.Errorf("audit: salt at %s has mode %#o (grants group/other access); refusing to use weakened salt", path, mode)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("audit: reading salt %s: %w", path, err)
		}
		if len(data) != SaltSize {
			return nil, fmt.Errorf("audit: salt at %s read %d bytes (want %d)", path, len(data), SaltSize)
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("audit: stat salt %s: %w", path, err)
	}

	// Generate fresh salt.
	buf := make([]byte, SaltSize)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("audit: generating salt: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0600); err != nil {
		return nil, fmt.Errorf("audit: writing salt tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Best-effort cleanup; ignore error.
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("audit: renaming salt into place: %w", err)
	}
	return buf, nil
}
