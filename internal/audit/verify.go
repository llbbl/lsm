// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// VerifyResult summarizes a Verify call.
type VerifyResult struct {
	OK       bool   // true iff the entire chain validates
	Events   uint64 // events processed before stopping
	FailSeq  uint64 // seq of the first broken row (0 if OK)
	FailLine uint64 // 1-indexed line number where the failure was observed
	Reason   string // human-readable explanation, empty if OK
}

// Verify reads JSONL audit events from r and validates the hash chain.
// On any failure, it stops at the first broken row and returns OK=false
// with FailSeq / FailLine / Reason set.
//
// An empty input (no events) is treated as valid (OK=true, Events=0).
//
// Scanner / I/O errors are returned via the error return; content failures
// (malformed JSON, hash mismatch, etc.) populate the VerifyResult instead.
func Verify(r io.Reader) (VerifyResult, error) {
	scanner := bufio.NewScanner(r)
	// Default 64KB buffer can be tight for events with large Fields maps.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1<<20)

	var (
		res        VerifyResult
		line       uint64
		prevSeq    uint64
		prevHash   = initialPrev()
		seenAnyRow bool
	)

	fail := func(seq, lineNum uint64, reason string) VerifyResult {
		return VerifyResult{
			OK:       false,
			Events:   res.Events,
			FailSeq:  seq,
			FailLine: lineNum,
			Reason:   reason,
		}
	}

	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		// Skip blank lines defensively; FileSink never emits them, but we
		// don't want a trailing newline to count as a failure.
		if len(raw) == 0 {
			continue
		}

		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return fail(0, line, "malformed JSON"), nil
		}

		if e.SchemaVersion != SchemaVersion {
			return fail(e.Seq, line, fmt.Sprintf("unsupported schema_version: %d", e.SchemaVersion)), nil
		}

		expectedSeq := prevSeq + 1
		if e.Seq != expectedSeq {
			return fail(e.Seq, line, fmt.Sprintf("seq gap: expected %d, got %d", expectedSeq, e.Seq)), nil
		}

		expectedPrev := prevHash
		if !seenAnyRow {
			expectedPrev = initialPrev()
		}
		if e.Prev != expectedPrev {
			return fail(e.Seq, line, "prev mismatch: chain broken"), nil
		}

		if e.Hash == "" {
			return fail(e.Seq, line, "missing hash"), nil
		}

		expectedHash := computeHash(e.Prev, e)
		if e.Hash != expectedHash {
			return fail(e.Seq, line, "hash mismatch: row tampered"), nil
		}

		prevSeq = e.Seq
		prevHash = e.Hash
		seenAnyRow = true
		res.Events++
	}

	if err := scanner.Err(); err != nil {
		return VerifyResult{}, fmt.Errorf("audit: scan: %w", err)
	}

	res.OK = true
	return res, nil
}
