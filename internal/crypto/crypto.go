// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package crypto

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// GenerateIdentity creates a new age X25519 identity (keypair).
func GenerateIdentity() (*age.X25519Identity, error) {
	return age.GenerateX25519Identity()
}

// LoadIdentity reads an age identity from the given key file path.
// The file should contain a Bech32-encoded age secret key line.
//
// The key file must not grant any permission bits to group or other —
// concretely, the check is `mode & 0o077 == 0`. Modes 0400, 0500, 0600
// (which SaveIdentity itself writes), and 0700 are accepted; stricter
// modes just remove access lsm doesn't need. Group/other-readable modes
// (0640, 0644, 0660, etc.) are rejected rather than loaded, because a
// private key readable by other users is a key disclosure waiting to
// happen and that must be corrected explicitly, not used silently.
//
// If the file holds more than one identity, the first is used and the
// extra count is returned via warnCount so callers can surface a warning
// (the crypto package is a library and does not write to stderr itself).
// warnCount is the number of additional identities beyond the first; it
// is 0 in the normal single-identity case.
func LoadIdentity(keyPath string) (id *age.X25519Identity, warnCount int, err error) {
	info, err := os.Stat(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, fmt.Errorf("key file not found: %s (run 'lsm init' to create one)", keyPath)
		}
		return nil, 0, fmt.Errorf("stat key file: %w", err)
	}
	// Reject any mode that grants group or other a permission bit. The
	// private key must stay owner-only; SaveIdentity writes 0600, so this
	// just enforces what we create.
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return nil, 0, fmt.Errorf("key file %s has mode %#o (grants group/other access); refusing to load private key — run 'chmod 600 %s'", keyPath, mode, keyPath)
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, 0, fmt.Errorf("reading key file: %w", err)
	}

	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("parsing key file: %w", err)
	}

	if len(identities) == 0 {
		return nil, 0, fmt.Errorf("no identities found in %s", keyPath)
	}

	x, ok := identities[0].(*age.X25519Identity)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected identity type in %s", keyPath)
	}

	return x, len(identities) - 1, nil
}

// SaveIdentity writes an age identity to the given path with a comment header.
func SaveIdentity(keyPath string, identity *age.X25519Identity) error {
	var buf strings.Builder
	buf.WriteString("# created by lsm\n")
	buf.WriteString("# public key: ")
	buf.WriteString(identity.Recipient().String())
	buf.WriteString("\n")
	buf.WriteString(identity.String())
	buf.WriteString("\n")

	return os.WriteFile(keyPath, []byte(buf.String()), 0600)
}

// Encrypt encrypts data for the given recipient.
func Encrypt(data []byte, recipient age.Recipient) ([]byte, error) {
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("creating encryptor: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("writing encrypted data: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing encryptor: %w", err)
	}
	return buf.Bytes(), nil
}

// Decrypt decrypts data using the given identity.
func Decrypt(data []byte, identity age.Identity) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		return nil, fmt.Errorf("decrypting data: %w", err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading decrypted data: %w", err)
	}
	return plain, nil
}
