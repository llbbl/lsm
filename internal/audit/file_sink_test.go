package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func readEvents(t *testing.T, path string) []Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var out []Event
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		out = append(out, e)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func TestFileSink_FreshFile_FirstWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	defer s.Close()

	if err := s.Write(context.Background(), Event{Event: "get", App: "a", Env: "prod"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	events := readEvents(t, path)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Seq != 1 {
		t.Errorf("seq: want 1, got %d", got.Seq)
	}
	if got.Prev != initialPrev() {
		t.Errorf("prev: want zero hash, got %q", got.Prev)
	}
	if len(got.Hash) != 64 {
		t.Errorf("hash: want 64 chars, got %d", len(got.Hash))
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version: want %d, got %d", SchemaVersion, got.SchemaVersion)
	}
	if got.Timestamp.IsZero() {
		t.Errorf("timestamp not filled in")
	}
}

func TestFileSink_TwoWrites_ChainLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.Write(ctx, Event{Event: "get"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, Event{Event: "set"}); err != nil {
		t.Fatal(err)
	}

	events := readEvents(t, path)
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	if events[0].Seq != 1 || events[1].Seq != 2 {
		t.Errorf("seq mismatch: %d, %d", events[0].Seq, events[1].Seq)
	}
	if events[1].Prev != events[0].Hash {
		t.Errorf("prev should equal previous hash")
	}
	if events[0].Hash == events[1].Hash {
		t.Errorf("hashes should differ")
	}
}

func TestFileSink_ReopenContinuesChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	s1, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Write(context.Background(), Event{Event: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Write(context.Background(), Event{Event: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	first := readEvents(t, path)
	lastHash := first[len(first)-1].Hash
	lastSeq := first[len(first)-1].Seq

	s2, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if err := s2.Write(context.Background(), Event{Event: "c"}); err != nil {
		t.Fatal(err)
	}

	all := readEvents(t, path)
	if len(all) != 3 {
		t.Fatalf("want 3 events, got %d", len(all))
	}
	third := all[2]
	if third.Seq != lastSeq+1 {
		t.Errorf("seq: want %d, got %d", lastSeq+1, third.Seq)
	}
	if third.Prev != lastHash {
		t.Errorf("prev: want %q, got %q", lastHash, third.Prev)
	}
}

func TestFileSink_RespectsCallerTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := s.Write(context.Background(), Event{Event: "get", Timestamp: ts}); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(context.Background(), Event{Event: "set"}); err != nil {
		t.Fatal(err)
	}

	events := readEvents(t, path)
	if !events[0].Timestamp.Equal(ts) {
		t.Errorf("caller timestamp not preserved: got %v", events[0].Timestamp)
	}
	if events[1].Timestamp.IsZero() {
		t.Errorf("sink should fill zero timestamp")
	}
}

func TestFileSink_IgnoresCallerSeqHashPrev(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	in := Event{
		Event: "get",
		Seq:   999,
		Prev:  "deadbeef",
		Hash:  "cafebabe",
	}
	if err := s.Write(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	events := readEvents(t, path)
	got := events[0]
	if got.Seq != 1 {
		t.Errorf("seq not reassigned: got %d", got.Seq)
	}
	if got.Prev != initialPrev() {
		t.Errorf("prev not reassigned: got %q", got.Prev)
	}
	if got.Hash == "cafebabe" {
		t.Errorf("hash not reassigned")
	}
	if len(got.Hash) != 64 {
		t.Errorf("hash should be 64 hex chars, got %d", len(got.Hash))
	}
}

func TestFileSink_Concurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const (
		goroutines = 100
		perG       = 10
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range perG {
				if err := s.Write(context.Background(), Event{
					Event:  "get",
					Fields: map[string]any{"g": id, "i": j},
				}); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	events := readEvents(t, path)
	want := goroutines * perG
	if len(events) != want {
		t.Fatalf("want %d events, got %d", want, len(events))
	}
	for i, e := range events {
		if e.Seq != uint64(i+1) {
			t.Fatalf("seq[%d]: want %d, got %d", i, i+1, e.Seq)
		}
		if i == 0 {
			if e.Prev != initialPrev() {
				t.Fatalf("first prev should be zero hash")
			}
		} else if e.Prev != events[i-1].Hash {
			t.Fatalf("chain break at %d: prev=%q want=%q", i, e.Prev, events[i-1].Hash)
		}
	}
}

func TestFileSink_MalformedTail_Errors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(context.Background(), Event{Event: "get"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("this is not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if _, err := NewFileSink(path); err == nil {
		t.Fatal("expected error on malformed tail")
	}
}

func TestFileSink_EmptyFile_Opens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("empty file should open: %v", err)
	}
	defer s.Close()
	if err := s.Write(context.Background(), Event{Event: "get"}); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, path)
	if events[0].Seq != 1 || events[0].Prev != initialPrev() {
		t.Errorf("empty file should start fresh chain")
	}
}

func TestFileSink_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Write(context.Background(), Event{Event: "get"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("perms: want 0600, got %o", mode)
	}
}

func TestFileSink_Close_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second close should be nil, got %v", err)
	}
	if err := s.Write(context.Background(), Event{Event: "get"}); err == nil {
		t.Errorf("write after close should error")
	}
}

func TestFileSink_CanceledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Write(ctx, Event{Event: "get"}); err == nil {
		t.Errorf("write with canceled context should error")
	}
}
