// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/llbbl/lsm/internal/crypto"
)

func TestParseEnv_Simple(t *testing.T) {
	input := "KEY=value\nOTHER=test"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "value")
	assertEqual(t, kv["OTHER"], "test")
}

func TestParseEnv_Comments(t *testing.T) {
	input := "# This is a comment\nKEY=value\n# Another comment"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	if len(kv) != 1 {
		t.Errorf("expected 1 key, got %d", len(kv))
	}
	assertEqual(t, kv["KEY"], "value")
}

func TestParseEnv_BlankLines(t *testing.T) {
	input := "KEY=value\n\nOTHER=test\n"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "value")
	assertEqual(t, kv["OTHER"], "test")
}

func TestParseEnv_DoubleQuoted(t *testing.T) {
	input := `KEY="hello world"`
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "hello world")
}

func TestParseEnv_SingleQuoted(t *testing.T) {
	input := `KEY='hello world'`
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "hello world")
}

func TestParseEnv_MultilineDoubleQuoted(t *testing.T) {
	input := "KEY=\"line1\nline2\nline3\""
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "line1\nline2\nline3")
}

func TestParseEnv_ExportPrefix(t *testing.T) {
	input := "export KEY=value"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "value")
}

func TestParseEnv_InlineComment(t *testing.T) {
	input := "KEY=value # this is a comment"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "value")
}

func TestParseEnv_UnterminatedDoubleQuote(t *testing.T) {
	input := `KEY="unterminated`
	_, err := ParseEnv(input)
	if err == nil {
		t.Fatal("expected error for unterminated double quote")
	}
}

func TestParseEnv_UnterminatedSingleQuote(t *testing.T) {
	input := `KEY='unterminated`
	_, err := ParseEnv(input)
	if err == nil {
		t.Fatal("expected error for unterminated single quote")
	}
}

func TestParseEnv_EmptyValue(t *testing.T) {
	input := "KEY="
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["KEY"], "")
}

func TestParseEnv_URLValue(t *testing.T) {
	input := "DATABASE_URL=postgres://user:pass@host:5432/db"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	kv := entriesToMap(entries)
	assertEqual(t, kv["DATABASE_URL"], "postgres://user:pass@host:5432/db")
}

func TestSerialize_Roundtrip(t *testing.T) {
	input := "# comment\nKEY=value\n\nOTHER=test"
	entries, err := ParseEnv(input)
	if err != nil {
		t.Fatalf("ParseEnv() error: %v", err)
	}
	output := Serialize(entries)
	assertEqual(t, output, input)
}

func TestSerialize_MultilineValue(t *testing.T) {
	entries := []entry{
		{Key: "KEY", Value: "line1\nline2"},
	}
	output := Serialize(entries)
	assertEqual(t, output, "KEY=\"line1\nline2\"")
}

func TestStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}

	s := New(dir, "myapp", "dev", id)

	// Set
	s.Set("KEY1", "value1")
	s.Set("KEY2", "value2")

	// Get
	val, ok := s.Get("KEY1")
	if !ok || val != "value1" {
		t.Errorf("Get(KEY1) = %q, %v; want %q, true", val, ok, "value1")
	}

	// Update
	s.Set("KEY1", "updated")
	val, ok = s.Get("KEY1")
	if !ok || val != "updated" {
		t.Errorf("after update, Get(KEY1) = %q, %v; want %q, true", val, ok, "updated")
	}

	// List
	keys := s.List()
	if len(keys) != 2 {
		t.Errorf("List() = %v, want 2 keys", keys)
	}

	// Delete
	if !s.Delete("KEY1") {
		t.Error("Delete(KEY1) returned false")
	}
	_, ok = s.Get("KEY1")
	if ok {
		t.Error("KEY1 still exists after delete")
	}

	// Delete nonexistent
	if s.Delete("NOPE") {
		t.Error("Delete(NOPE) returned true for nonexistent key")
	}

	// Dump
	m := s.Dump()
	if len(m) != 1 {
		t.Errorf("Dump() has %d entries, want 1", len(m))
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	id, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity() error: %v", err)
	}

	// Create and save
	s1 := New(dir, "testapp", "dev", id)
	s1.Set("DB_URL", "postgres://localhost/test")
	s1.Set("API_KEY", "secret123")
	if err := s1.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(s1.FilePath()); err != nil {
		t.Fatalf("encrypted file not created: %v", err)
	}

	// Load into new store
	s2 := New(dir, "testapp", "dev", id)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	val, ok := s2.Get("DB_URL")
	if !ok || val != "postgres://localhost/test" {
		t.Errorf("after load, Get(DB_URL) = %q, %v", val, ok)
	}
	val, ok = s2.Get("API_KEY")
	if !ok || val != "secret123" {
		t.Errorf("after load, Get(API_KEY) = %q, %v", val, ok)
	}
}

func TestStore_LoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "noapp", "dev", id)
	if err := s.Load(); err != nil {
		t.Fatalf("Load() on nonexistent file should not error: %v", err)
	}
	if len(s.List()) != 0 {
		t.Error("expected empty store for nonexistent file")
	}
}

func TestStore_Import(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	s.Set("EXISTING", "keep")
	err := s.Import("NEW_KEY=new_value\nEXISTING=overwritten")
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}

	val, _ := s.Get("NEW_KEY")
	assertEqual(t, val, "new_value")
	val, _ = s.Get("EXISTING")
	assertEqual(t, val, "overwritten")
}

func TestStore_FilePath(t *testing.T) {
	s := New("/home/user/.lsm", "myapp", "production", nil)
	expected := "/home/user/.lsm/myapp.production.age"
	if s.FilePath() != expected {
		t.Errorf("FilePath() = %q, want %q", s.FilePath(), expected)
	}
}

func TestListApps(t *testing.T) {
	dir := t.TempDir()
	// Create some fake .age files
	for _, name := range []string{"app1.dev.age", "app1.prod.age", "app2.dev.age", "config.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	apps, err := ListApps(dir)
	if err != nil {
		t.Fatalf("ListApps() error: %v", err)
	}
	if len(apps) != 2 || apps[0] != "app1" || apps[1] != "app2" {
		t.Errorf("ListApps() = %v, want [app1 app2]", apps)
	}
}

func TestListEnvs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"myapp.dev.age", "myapp.production.age", "other.dev.age"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	envs, err := ListEnvs(dir, "myapp")
	if err != nil {
		t.Fatalf("ListEnvs() error: %v", err)
	}
	if len(envs) != 2 || envs[0] != "dev" || envs[1] != "production" {
		t.Errorf("ListEnvs() = %v, want [dev production]", envs)
	}
}

func TestListApps_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	apps, err := ListApps(dir)
	if err != nil {
		t.Fatalf("ListApps() error: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("ListApps() = %v, want empty", apps)
	}
}

func TestListApps_NonexistentDir(t *testing.T) {
	apps, err := ListApps("/nonexistent/dir")
	if err != nil {
		t.Fatalf("ListApps() error: %v", err)
	}
	if apps != nil {
		t.Errorf("ListApps() = %v, want nil", apps)
	}
}

func TestStore_DumpOrdered(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	s.Set("ALPHA", "a_val")
	s.Set("BETA", "b_val")
	s.Set("GAMMA", "g_val")

	ordered := s.DumpOrdered()
	if len(ordered) != 3 {
		t.Fatalf("DumpOrdered() returned %d entries, want 3", len(ordered))
	}

	// Verify order matches insertion order
	expected := []struct{ key, val string }{
		{"ALPHA", "a_val"},
		{"BETA", "b_val"},
		{"GAMMA", "g_val"},
	}
	for i, e := range expected {
		if ordered[i].Key != e.key || ordered[i].Value != e.val {
			t.Errorf("DumpOrdered()[%d] = {%q, %q}, want {%q, %q}",
				i, ordered[i].Key, ordered[i].Value, e.key, e.val)
		}
	}
}

func TestStore_SetRaw(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	s.Set("OLD_KEY", "old_val")

	err := s.SetRaw("NEW_KEY=new_val\nOTHER=other_val\n")
	if err != nil {
		t.Fatalf("SetRaw() error: %v", err)
	}

	// Old key should be gone
	if _, ok := s.Get("OLD_KEY"); ok {
		t.Error("OLD_KEY still exists after SetRaw")
	}

	// New keys should exist
	val, ok := s.Get("NEW_KEY")
	if !ok || val != "new_val" {
		t.Errorf("Get(NEW_KEY) = %q, %v; want %q, true", val, ok, "new_val")
	}
	val, ok = s.Get("OTHER")
	if !ok || val != "other_val" {
		t.Errorf("Get(OTHER) = %q, %v; want %q, true", val, ok, "other_val")
	}
}

func TestStore_RawContent(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	s.Set("KEY1", "val1")
	s.Set("KEY2", "val2")

	raw := s.RawContent()
	if raw != "KEY1=val1\nKEY2=val2" {
		t.Errorf("RawContent() = %q, want %q", raw, "KEY1=val1\nKEY2=val2")
	}
}

func TestStore_RawContent_Empty(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	raw := s.RawContent()
	if raw != "" {
		t.Errorf("RawContent() on empty store = %q, want empty", raw)
	}
}

func TestListEnvs_NoMatchingApp(t *testing.T) {
	dir := t.TempDir()
	// Create files for a different app
	for _, name := range []string{"otherapp.dev.age", "otherapp.prod.age"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	envs, err := ListEnvs(dir, "nonexistent")
	if err != nil {
		t.Fatalf("ListEnvs() error: %v", err)
	}
	if len(envs) != 0 {
		t.Errorf("ListEnvs() = %v, want empty", envs)
	}
}

func TestListEnvs_NonexistentDir(t *testing.T) {
	envs, err := ListEnvs("/nonexistent/dir", "app")
	if err != nil {
		t.Fatalf("ListEnvs() error: %v", err)
	}
	if envs != nil {
		t.Errorf("ListEnvs() = %v, want nil", envs)
	}
}

func TestStore_Load_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	// Write garbage to the .age file
	if err := os.WriteFile(s.FilePath(), []byte("this is not valid age encrypted data"), 0600); err != nil {
		t.Fatalf("writing corrupt file: %v", err)
	}

	err := s.Load()
	if err == nil {
		t.Fatal("expected error loading corrupt file")
	}
}

func TestStore_Load_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	// Create a file with no read permissions
	path := s.FilePath()
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod 0000: %v", err)
	}
	defer func() {
		if err := os.Chmod(path, 0600); err != nil {
			t.Logf("warning: restoring permissions: %v", err)
		}
	}()

	err := s.Load()
	if err == nil {
		t.Fatal("expected error loading unreadable file")
	}
}

func TestStore_SetRaw_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	id, _ := crypto.GenerateIdentity()
	s := New(dir, "app", "dev", id)

	// Unterminated double quote should produce an error
	err := s.SetRaw(`KEY="unterminated`)
	if err == nil {
		t.Fatal("expected error for invalid .env content in SetRaw")
	}
}

// helpers

func entriesToMap(entries []entry) map[string]string {
	m := make(map[string]string)
	for _, e := range entries {
		if e.Key != "" {
			m[e.Key] = e.Value
		}
	}
	return m
}

func assertEqual(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
