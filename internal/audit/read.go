// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// ErrNotFound is returned by Show when no event matches the requested seq.
var ErrNotFound = errors.New("audit: event not found")

// scannerBuffer mirrors the buffer ceiling used by Verify so events with
// large Fields maps don't trip bufio's default 64KB line limit.
func scannerBuffer(s *bufio.Scanner) {
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 1<<20)
}

// Tail reads the file at path and returns the last n events. Streams the file
// line-by-line into a ring buffer of size n, so memory use is bounded by n
// regardless of file size. Returns an empty slice for empty files.
func Tail(path string, n int) ([]Event, error) {
	if n <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scannerBuffer(scanner)

	ring := make([]Event, 0, n)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("audit: parse line: %w", err)
		}
		if len(ring) < n {
			ring = append(ring, e)
			continue
		}
		// Shift left by one and append. With n bounded by the user this
		// is fine; an offset cursor would micro-optimize but adds complexity.
		copy(ring, ring[1:])
		ring[n-1] = e
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("audit: scan: %w", err)
	}
	return ring, nil
}

// FollowFunc is called for each new event observed in follow mode.
type FollowFunc func(Event) error

// Follow tails the file starting from its current end-of-file and invokes fn
// for each new event observed. Polls every interval. Returns when ctx is
// canceled (returning ctx.Err()) or when fn returns an error.
//
// The file must exist when Follow is called; missing-file is a fail-fast
// pre-condition (tailing a non-existent log is almost always a misconfig).
// Follow does not handle file truncation or rotation — if the underlying
// file shrinks below the recorded offset, the next read attempt will return
// no data until the file grows back past the offset.
func Follow(ctx context.Context, path string, interval time.Duration, fn FollowFunc) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	offset := info.Size()

	// Buffered carry for partial trailing lines between polls.
	var carry []byte

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		if fi.Size() <= offset {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		offset += int64(len(data))

		buf := append(carry, data...)
		carry = nil
		for {
			idx := bytes.IndexByte(buf, '\n')
			if idx < 0 {
				carry = append(carry[:0], buf...)
				break
			}
			line := buf[:idx]
			buf = buf[idx+1:]
			if len(line) == 0 {
				continue
			}
			var e Event
			if err := json.Unmarshal(line, &e); err != nil {
				return fmt.Errorf("audit: parse line: %w", err)
			}
			if err := fn(e); err != nil {
				return err
			}
		}
	}
}

// Show reads the file and returns the first event matching seq. If multiple
// rows share the same seq (only possible in a malformed chain), Show returns
// the first match and reports a duplicate via the warn callback (if non-nil).
// Returns ErrNotFound if no event with that seq is found.
func Show(path string, seq uint64, warn func(string)) (Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return Event{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scannerBuffer(scanner)

	var (
		found  bool
		result Event
	)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return Event{}, fmt.Errorf("audit: parse line: %w", err)
		}
		if e.Seq != seq {
			continue
		}
		if !found {
			result = e
			found = true
			// Intentionally do NOT break: we keep scanning to surface any
			// duplicate seqs through the warn callback. Sequence numbers
			// must be unique, so a duplicate is a corruption signal worth
			// reporting rather than silently returning the first hit.
			continue
		}
		if warn != nil {
			warn(fmt.Sprintf("audit: duplicate seq=%d ignored", seq))
		}
	}
	if err := scanner.Err(); err != nil {
		return Event{}, fmt.Errorf("audit: scan: %w", err)
	}
	if !found {
		return Event{}, ErrNotFound
	}
	return result, nil
}

// QueryFilter describes filter predicates for Query. Zero-valued fields are
// treated as "no constraint" — all predicates are AND-ed.
type QueryFilter struct {
	App         string
	Env         string
	Event       string
	ParentComm  string
	AgentMarker string
	TTYPresent  *bool      // nil=any, &true=must be present, &false=must be absent
	Since       *time.Time // nil = no lower bound; inclusive
	Until       *time.Time // nil = no upper bound; exclusive
	SeqFrom     uint64     // 0 = no lower bound
	SeqTo       uint64     // 0 = no upper bound
}

// matches returns true if e satisfies every set predicate in f.
func (f QueryFilter) matches(e Event) bool {
	if f.App != "" && e.App != f.App {
		return false
	}
	if f.Env != "" && e.Env != f.Env {
		return false
	}
	if f.Event != "" && e.Event != f.Event {
		return false
	}
	if f.ParentComm != "" && e.Actor.ParentComm != f.ParentComm {
		return false
	}
	if f.AgentMarker != "" && e.Actor.AgentMarker != f.AgentMarker {
		return false
	}
	if f.TTYPresent != nil {
		present := e.Actor.TTY != ""
		if present != *f.TTYPresent {
			return false
		}
	}
	if f.Since != nil && e.Timestamp.Before(*f.Since) {
		return false
	}
	if f.Until != nil && !e.Timestamp.Before(*f.Until) {
		return false
	}
	if f.SeqFrom != 0 && e.Seq < f.SeqFrom {
		return false
	}
	if f.SeqTo != 0 && e.Seq > f.SeqTo {
		return false
	}
	return true
}

// Query reads the file and invokes fn for each event matching the filter.
// Streaming: the entire result set is never buffered in memory. Iteration
// stops early (and the error is propagated) if fn returns non-nil.
func Query(path string, filter QueryFilter, fn func(Event) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scannerBuffer(scanner)

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("audit: parse line: %w", err)
		}
		if !filter.matches(e) {
			continue
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("audit: scan: %w", err)
	}
	return nil
}
