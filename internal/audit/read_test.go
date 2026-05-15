// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func makeChain(t *testing.T, n int, mutate func(i int, e *Event)) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	defer sink.Close()
	for i := range n {
		ev := Event{
			Event: "test",
			App:   "myapp",
			Env:   "dev",
			Actor: Actor{ParentComm: "node", AgentMarker: "claude", TTY: "/dev/tty0"},
		}
		if mutate != nil {
			mutate(i, &ev)
		}
		if err := sink.Write(context.Background(), ev); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	return path
}

func TestTail_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Tail(path, 5)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestTail_LastN(t *testing.T) {
	path := makeChain(t, 5, nil)
	got, err := Tail(path, 3)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	if got[0].Seq != 3 || got[1].Seq != 4 || got[2].Seq != 5 {
		t.Errorf("seqs = %d,%d,%d; want 3,4,5", got[0].Seq, got[1].Seq, got[2].Seq)
	}
}

func TestTail_NGreaterThanCount(t *testing.T) {
	path := makeChain(t, 5, nil)
	got, err := Tail(path, 100)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("len=%d, want 5", len(got))
	}
}

func TestTail_MissingFile(t *testing.T) {
	_, err := Tail(filepath.Join(t.TempDir(), "nope.jsonl"), 5)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestShow_Found(t *testing.T) {
	path := makeChain(t, 5, func(i int, e *Event) {
		e.App = "app-" + string(rune('a'+i))
	})
	ev, err := Show(path, 3, nil)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if ev.Seq != 3 || ev.App != "app-c" {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestShow_NotFound(t *testing.T) {
	path := makeChain(t, 3, nil)
	_, err := Show(path, 999, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err=%v, want ErrNotFound", err)
	}
}

func TestQuery_NoFilters(t *testing.T) {
	path := makeChain(t, 4, nil)
	var count int
	if err := Query(path, QueryFilter{}, func(Event) error { count++; return nil }); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if count != 4 {
		t.Errorf("count=%d, want 4", count)
	}
}

func TestQuery_AppFilter(t *testing.T) {
	path := makeChain(t, 5, func(i int, e *Event) {
		if i%2 == 0 {
			e.App = "alpha"
		} else {
			e.App = "beta"
		}
	})
	var seqs []uint64
	err := Query(path, QueryFilter{App: "alpha"}, func(e Event) error {
		seqs = append(seqs, e.Seq)
		return nil
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(seqs) != 3 {
		t.Errorf("got %v, want 3 matches", seqs)
	}
}

func TestQuery_MultipleFiltersAND(t *testing.T) {
	path := makeChain(t, 4, func(i int, e *Event) {
		e.App = "shared"
		if i == 2 {
			e.Env = "prod"
		}
	})
	var matched []uint64
	err := Query(path, QueryFilter{App: "shared", Env: "prod"}, func(e Event) error {
		matched = append(matched, e.Seq)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0] != 3 {
		t.Errorf("got %v, want [3]", matched)
	}
}

func TestQuery_TTYPresent(t *testing.T) {
	path := makeChain(t, 3, func(i int, e *Event) {
		if i == 1 {
			e.Actor.TTY = ""
		}
	})
	want := true
	var seqs []uint64
	err := Query(path, QueryFilter{TTYPresent: &want}, func(e Event) error {
		seqs = append(seqs, e.Seq)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 2 {
		t.Errorf("got %v, want 2", seqs)
	}

	absent := false
	var seqsAbsent []uint64
	if err := Query(path, QueryFilter{TTYPresent: &absent}, func(e Event) error {
		seqsAbsent = append(seqsAbsent, e.Seq)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seqsAbsent) != 1 || seqsAbsent[0] != 2 {
		t.Errorf("got %v, want [2]", seqsAbsent)
	}
}

func TestQuery_TimeWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 4 {
		ev := Event{Event: "test", Timestamp: base.Add(time.Duration(i) * time.Hour)}
		if err := sink.Write(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	since := base.Add(1 * time.Hour)
	until := base.Add(3 * time.Hour)
	var seqs []uint64
	err = Query(path, QueryFilter{Since: &since, Until: &until}, func(e Event) error {
		seqs = append(seqs, e.Seq)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Half-open: includes seq=2 (1h) and seq=3 (2h), excludes seq=4 (3h).
	if len(seqs) != 2 || seqs[0] != 2 || seqs[1] != 3 {
		t.Errorf("got %v, want [2 3]", seqs)
	}
}

func TestQuery_SeqRange(t *testing.T) {
	path := makeChain(t, 6, nil)
	var seqs []uint64
	err := Query(path, QueryFilter{SeqFrom: 2, SeqTo: 4}, func(e Event) error {
		seqs = append(seqs, e.Seq)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 3 || seqs[0] != 2 || seqs[2] != 4 {
		t.Errorf("got %v, want [2 3 4]", seqs)
	}
}

func TestQuery_CallbackErrorStops(t *testing.T) {
	path := makeChain(t, 5, nil)
	sentinel := errors.New("stop")
	var seen int
	err := Query(path, QueryFilter{}, func(Event) error {
		seen++
		if seen == 2 {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err=%v, want sentinel", err)
	}
	if seen != 2 {
		t.Errorf("seen=%d, want 2", seen)
	}
}

func TestFollow_NewEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	// Pre-populate one event so the file exists and has a baseline.
	if err := sink.Write(context.Background(), Event{Event: "seed"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu   sync.Mutex
		seen []uint64
		done = make(chan error, 1)
	)
	go func() {
		done <- Follow(ctx, path, 10*time.Millisecond, func(e Event) error {
			mu.Lock()
			seen = append(seen, e.Seq)
			n := len(seen)
			mu.Unlock()
			if n >= 2 {
				cancel()
			}
			return nil
		})
	}()

	// Give Follow a moment to record the initial offset.
	time.Sleep(30 * time.Millisecond)
	if err := sink.Write(context.Background(), Event{Event: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Write(context.Background(), Event{Event: "b"}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Follow returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Follow did not return within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 {
		t.Errorf("seen=%v, want at least 2", seen)
	}
}

func TestFollow_ContextCancelBeforeEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var called bool
	err := Follow(ctx, path, 10*time.Millisecond, func(Event) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want Canceled", err)
	}
	if called {
		t.Error("callback should not have been invoked")
	}
}

func TestFollow_MissingFileFailsFast(t *testing.T) {
	err := Follow(t.Context(), filepath.Join(t.TempDir(), "nope.jsonl"), 10*time.Millisecond, func(Event) error { return nil })
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
