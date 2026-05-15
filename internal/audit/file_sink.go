package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// computeHash returns the SHA-256 hex digest of (prev || canonical_json(e))
// with e.Hash cleared. The struct's JSON field order is fixed by the type
// definition, and encoding/json sorts map keys for map[string]X types, so the
// only nondeterministic field (Fields) still produces deterministic bytes.
func computeHash(prev string, e Event) string {
	e.Hash = ""
	body, _ := json.Marshal(e) // marshaling this concrete struct cannot fail
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// initialPrev returns the Prev value for the first event in a chain.
func initialPrev() string {
	return strings.Repeat("0", 64)
}

// FileSink writes audit events as JSONL to a single file. A FileSink is safe
// for concurrent use by goroutines within one process; cross-process safety is
// out of scope (would require advisory locking).
type FileSink struct {
	mu       sync.Mutex
	path     string
	f        *os.File
	lastSeq  uint64
	lastHash string
	closed   bool
}

// NewFileSink opens path for append (creating it with 0600 if absent) and
// recovers the chain tail so the next Write extends the existing chain. If
// the file's last line is not parseable JSON, NewFileSink returns an error
// rather than silently starting a new chain.
func NewFileSink(path string) (*FileSink, error) {
	lastSeq, lastHash, err := recoverTail(path)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}

	return &FileSink{
		path:     path,
		f:        f,
		lastSeq:  lastSeq,
		lastHash: lastHash,
	}, nil
}

// recoverTail reads the file and parses the last non-empty line to recover
// (lastSeq, lastHash). For a missing or empty file, it returns (0, zeroHash).
func recoverTail(path string) (uint64, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, initialPrev(), nil
		}
		return 0, "", fmt.Errorf("audit: read %s: %w", path, err)
	}
	data = bytes.TrimRight(data, "\n\r\t ")
	if len(data) == 0 {
		return 0, initialPrev(), nil
	}

	idx := bytes.LastIndexByte(data, '\n')
	var lastLine []byte
	if idx < 0 {
		lastLine = data
	} else {
		lastLine = data[idx+1:]
	}

	var e Event
	if err := json.Unmarshal(lastLine, &e); err != nil {
		return 0, "", fmt.Errorf("audit: malformed tail in %s: %w", path, err)
	}
	if e.Hash == "" {
		return 0, "", fmt.Errorf("audit: tail event in %s has empty hash", path)
	}
	return e.Seq, e.Hash, nil
}

// Write appends e to the file. The sink overwrites SchemaVersion, Seq, Prev,
// and Hash on every call; Timestamp is filled in only if zero. Caller-provided
// values for any sink-managed field are ignored.
func (s *FileSink) Write(ctx context.Context, e Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("audit: write to closed sink")
	}

	e.SchemaVersion = SchemaVersion
	e.Seq = s.lastSeq + 1
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	} else {
		e.Timestamp = e.Timestamp.UTC()
	}
	e.Prev = s.lastHash
	e.Hash = computeHash(s.lastHash, e)

	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: marshal event: %w", err)
	}
	body = append(body, '\n')

	if _, err := s.f.Write(body); err != nil {
		return fmt.Errorf("audit: write event: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		return fmt.Errorf("audit: fsync: %w", err)
	}

	s.lastSeq = e.Seq
	s.lastHash = e.Hash
	return nil
}

// Close closes the underlying file. It is idempotent.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}
