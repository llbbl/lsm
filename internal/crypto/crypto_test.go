// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}
	if id == nil {
		t.Fatal("GenerateIdentity() returned nil")
	}
	if id.Recipient() == nil {
		t.Fatal("identity has no recipient")
	}
	// Verify key string format
	if len(id.String()) == 0 {
		t.Fatal("identity string is empty")
	}
	if len(id.Recipient().String()) == 0 {
		t.Fatal("recipient string is empty")
	}
}

func TestSaveAndLoadIdentity(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.txt")

	// Generate and save
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}
	if err := SaveIdentity(keyPath, id); err != nil {
		t.Fatalf("SaveIdentity() error: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("key file permissions = %o, want 0600", perm)
	}

	// Load and verify
	loaded, err := LoadIdentity(keyPath)
	if err != nil {
		t.Fatalf("LoadIdentity() error: %v", err)
	}
	if loaded.String() != id.String() {
		t.Errorf("loaded identity doesn't match: got %s, want %s", loaded.String(), id.String())
	}
	if loaded.Recipient().String() != id.Recipient().String() {
		t.Errorf("loaded recipient doesn't match")
	}
}

func TestLoadIdentity_NotFound(t *testing.T) {
	_, err := LoadIdentity("/nonexistent/key.txt")
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}

func TestLoadIdentity_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.txt")
	os.WriteFile(keyPath, []byte("not a valid key"), 0600)

	_, err := LoadIdentity(keyPath)
	if err == nil {
		t.Fatal("expected error for invalid key file")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"simple text", []byte("hello world")},
		{"empty", []byte("")},
		{"env format", []byte("KEY=value\nOTHER=test\n")},
		{"binary-ish", []byte{0, 1, 2, 255, 254, 253}},
		{"multiline value", []byte("KEY=\"line1\nline2\nline3\"")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.data, id.Recipient())
			if err != nil {
				t.Fatalf("Encrypt() error: %v", err)
			}
			if len(encrypted) == 0 && len(tt.data) > 0 {
				t.Fatal("encrypted data is empty")
			}

			decrypted, err := Decrypt(encrypted, id)
			if err != nil {
				t.Fatalf("Decrypt() error: %v", err)
			}
			if string(decrypted) != string(tt.data) {
				t.Errorf("roundtrip failed: got %q, want %q", decrypted, tt.data)
			}
		})
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	id1, _ := GenerateIdentity()
	id2, _ := GenerateIdentity()

	encrypted, err := Encrypt([]byte("secret"), id1.Recipient())
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	_, err = Decrypt(encrypted, id2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestDecrypt_CorruptData(t *testing.T) {
	id, _ := GenerateIdentity()
	_, err := Decrypt([]byte("not encrypted data"), id)
	if err == nil {
		t.Fatal("expected error for corrupt data")
	}
}

func TestLoadIdentity_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.txt")
	os.WriteFile(keyPath, []byte(""), 0600)

	_, err := LoadIdentity(keyPath)
	if err == nil {
		t.Fatal("expected error for empty key file")
	}
}

func TestLoadIdentity_OnlyComments(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.txt")
	os.WriteFile(keyPath, []byte("# just a comment\n# another comment\n"), 0600)

	_, err := LoadIdentity(keyPath)
	if err == nil {
		t.Fatal("expected error for key file with only comments")
	}
}

func TestLoadIdentity_MalformedKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.txt")
	// Write something that looks like a key line but is not valid bech32
	os.WriteFile(keyPath, []byte("AGE-SECRET-KEY-NOTVALID\n"), 0600)

	_, err := LoadIdentity(keyPath)
	if err == nil {
		t.Fatal("expected error for malformed key content")
	}
}

func TestEncryptDecrypt_LargeData(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}

	// Generate 1MB of data
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	encrypted, err := Encrypt(data, id.Recipient())
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	decrypted, err := Decrypt(encrypted, id)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if len(decrypted) != len(data) {
		t.Fatalf("decrypted length = %d, want %d", len(decrypted), len(data))
	}
	for i := range data {
		if decrypted[i] != data[i] {
			t.Fatalf("mismatch at byte %d: got %d, want %d", i, decrypted[i], data[i])
		}
	}
}
