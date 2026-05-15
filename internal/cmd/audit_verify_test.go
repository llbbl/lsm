// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/audit"
)

func TestAudit_BareShowsHelp(t *testing.T) {
	out, err := runCmd(t, "audit")
	if err != nil {
		t.Fatalf("audit error: %v", err)
	}
	if !strings.Contains(out, "verify") {
		t.Errorf("audit help should mention 'verify' subcommand, got: %s", out)
	}
	if !strings.Contains(out, "Inspect and verify") {
		t.Errorf("audit help should include short description, got: %s", out)
	}
}

func TestAuditVerify_MissingFile(t *testing.T) {
	dir := t.TempDir()
	out, err := runCmd(t, "audit", "verify", "--dir", dir)
	if err != nil {
		t.Fatalf("audit verify error: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected 'not found' message, got: %s", out)
	}
}

func TestAuditVerify_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "audit", "verify", "--file", path)
	if err != nil {
		t.Fatalf("audit verify error: %v", err)
	}
	if !strings.Contains(out, "empty") {
		t.Errorf("expected 'empty' message, got: %s", out)
	}
}

func TestAuditVerify_ValidChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := audit.NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	for range 3 {
		if err := sink.Write(context.Background(), audit.Event{Event: "test", App: "a", Env: "dev"}); err != nil {
			t.Fatalf("sink.Write: %v", err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "audit", "verify", "--file", path)
	if err != nil {
		t.Fatalf("audit verify error: %v", err)
	}
	if !strings.Contains(out, "OK") || !strings.Contains(out, "events=3") {
		t.Errorf("expected OK with events=3, got: %s", out)
	}
}

func TestAuditVerify_TamperedChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := audit.NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	for range 3 {
		if err := sink.Write(context.Background(), audit.Event{Event: "test", App: "a", Env: "dev"}); err != nil {
			t.Fatalf("sink.Write: %v", err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	// Tamper row 2.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	var ev audit.Event
	if err := json.Unmarshal(lines[1], &ev); err != nil {
		t.Fatal(err)
	}
	ev.App = "tampered"
	tampered, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	lines[1] = tampered
	var buf bytes.Buffer
	for _, l := range lines {
		buf.Write(l)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "audit", "verify", "--file", path)
	if err == nil {
		t.Fatalf("expected error for tampered chain, output: %s", out)
	}
	if !strings.Contains(err.Error(), "FAIL") {
		t.Errorf("error should contain FAIL, got: %v", err)
	}
	if !strings.Contains(err.Error(), "seq=2") {
		t.Errorf("error should mention seq=2, got: %v", err)
	}
}
