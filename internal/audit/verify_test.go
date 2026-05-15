// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeChain writes n events through a FileSink and returns the path.
func writeChain(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	for i := range n {
		ev := Event{
			Event: "test",
			App:   "myapp",
			Env:   "dev",
			Fields: map[string]any{
				"i": i,
			},
		}
		if err := sink.Write(context.Background(), ev); err != nil {
			t.Fatalf("sink.Write[%d]: %v", i, err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("sink.Close: %v", err)
	}
	return path
}

func readLines(t *testing.T, path string) [][]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	data = bytes.TrimRight(data, "\n")
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte("\n"))
	out := make([][]byte, len(parts))
	for i, p := range parts {
		out[i] = append([]byte(nil), p...)
	}
	return out
}

func writeLines(t *testing.T, path string, lines [][]byte) {
	t.Helper()
	var buf bytes.Buffer
	for _, l := range lines {
		buf.Write(l)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestVerify_EmptyInput(t *testing.T) {
	res, err := Verify(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK || res.Events != 0 {
		t.Errorf("got %+v, want OK with Events=0", res)
	}
}

func TestVerify_SingleEvent(t *testing.T) {
	path := writeChain(t, 1)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK || res.Events != 1 {
		t.Errorf("got %+v, want OK with Events=1", res)
	}
}

func TestVerify_ManyEvents(t *testing.T) {
	path := writeChain(t, 100)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK || res.Events != 100 {
		t.Errorf("got %+v, want OK with Events=100", res)
	}
}

func TestVerify_TamperedField(t *testing.T) {
	path := writeChain(t, 100)
	lines := readLines(t, path)

	// Tamper with row 50 (index 49)'s App field after the fact.
	var ev Event
	if err := json.Unmarshal(lines[49], &ev); err != nil {
		t.Fatal(err)
	}
	ev.App = "tampered"
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	lines[49] = out
	writeLines(t, path, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if res.FailSeq != 50 {
		t.Errorf("FailSeq = %d, want 50", res.FailSeq)
	}
	if !strings.Contains(res.Reason, "hash mismatch") {
		t.Errorf("Reason = %q, want hash mismatch", res.Reason)
	}
}

func TestVerify_DeletedRow(t *testing.T) {
	path := writeChain(t, 100)
	lines := readLines(t, path)
	// Delete row 50 (index 49). Now row 51 (index 49 after splice) has a prev
	// that doesn't match what comes before it.
	lines = append(lines[:49], lines[50:]...)
	writeLines(t, path, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	// The next row read after the deletion has seq=51 while expected=50.
	if res.FailSeq != 51 {
		t.Errorf("FailSeq = %d, want 51", res.FailSeq)
	}
	if !strings.Contains(res.Reason, "seq gap") {
		t.Errorf("Reason = %q, want seq gap", res.Reason)
	}
}

func TestVerify_InsertedRow(t *testing.T) {
	path := writeChain(t, 100)
	lines := readLines(t, path)

	// Construct a fake event with the next-up seq value (50) but a body that
	// won't have a valid chain hash. Reuse row 50's JSON but tweak hash so
	// the seq stays 50 (matching expected) — prev or hash will mismatch.
	var ev Event
	if err := json.Unmarshal(lines[49], &ev); err != nil {
		t.Fatal(err)
	}
	ev.App = "injected"
	fake, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	// Insert before original row 50 so the inserted row claims seq=50.
	newLines := make([][]byte, 0, len(lines)+1)
	newLines = append(newLines, lines[:49]...)
	newLines = append(newLines, fake)
	newLines = append(newLines, lines[49:]...)
	writeLines(t, path, newLines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	// Inserted row has correct prev (copied from row 50) and seq=50, so the
	// hash recomputation will fail because App was changed.
	if res.FailSeq != 50 {
		t.Errorf("FailSeq = %d, want 50", res.FailSeq)
	}
}

func TestVerify_MalformedJSON(t *testing.T) {
	path := writeChain(t, 3)
	lines := readLines(t, path)
	// Truncate the second line mid-JSON.
	lines[1] = lines[1][:10]
	writeLines(t, path, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if !strings.Contains(res.Reason, "malformed JSON") {
		t.Errorf("Reason = %q, want malformed JSON", res.Reason)
	}
	if res.FailLine != 2 {
		t.Errorf("FailLine = %d, want 2", res.FailLine)
	}
}

func TestVerify_UnsupportedSchemaVersion(t *testing.T) {
	path := writeChain(t, 2)
	lines := readLines(t, path)

	var ev Event
	if err := json.Unmarshal(lines[0], &ev); err != nil {
		t.Fatal(err)
	}
	ev.SchemaVersion = 99
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	lines[0] = out
	writeLines(t, path, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if !strings.Contains(res.Reason, "unsupported schema_version") {
		t.Errorf("Reason = %q, want unsupported schema_version", res.Reason)
	}
}

func TestVerify_SeqGap(t *testing.T) {
	path := writeChain(t, 3)
	lines := readLines(t, path)

	// Tamper line 1 (seq=1) to claim seq=42. Expected was 1.
	var ev Event
	if err := json.Unmarshal(lines[0], &ev); err != nil {
		t.Fatal(err)
	}
	ev.Seq = 42
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	lines[0] = out
	writeLines(t, path, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := Verify(f)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if !strings.Contains(res.Reason, "seq gap") {
		t.Errorf("Reason = %q, want seq gap", res.Reason)
	}
}
