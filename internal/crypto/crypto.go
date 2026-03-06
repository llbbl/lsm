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
func LoadIdentity(keyPath string) (*age.X25519Identity, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("key file not found: %s (run 'lsm init' to create one)", keyPath)
		}
		return nil, fmt.Errorf("reading key file: %w", err)
	}

	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parsing key file: %w", err)
	}

	if len(identities) == 0 {
		return nil, fmt.Errorf("no identities found in %s", keyPath)
	}

	id, ok := identities[0].(*age.X25519Identity)
	if !ok {
		return nil, fmt.Errorf("unexpected identity type in %s", keyPath)
	}

	return id, nil
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
