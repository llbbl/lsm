// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

// Package audit provides a hash-chained audit log for lsm. Callers supply
// event metadata, and the sink assigns the sequence, timestamp, and hashes.
package audit

import "time"

// SchemaVersion is the on-disk schema version for audit events. Any breaking
// change to field names or semantics requires a bump.
const SchemaVersion = 1

// Event is one record in the audit log. Hash and Prev are populated by the
// Sink (single writer); callers leave them zero.
//
// Note: Actor has no omitempty tag and its fields likewise omit omitempty.
// Every event therefore records a complete Actor shape, even when the caller
// leaves it zero-valued. See the Actor doc comment for rationale.
type Event struct {
	SchemaVersion int            `json:"schema_version"`
	Seq           uint64         `json:"seq"`
	Timestamp     time.Time      `json:"ts"`
	Event         string         `json:"event"`
	App           string         `json:"app,omitempty"`
	Env           string         `json:"env,omitempty"`
	Actor         Actor          `json:"actor"`
	Fields        map[string]any `json:"fields,omitempty"`
	LocalOnly     bool           `json:"local_only,omitempty"`
	Prev          string         `json:"prev"`
	Hash          string         `json:"hash"`
}

// Actor describes the process that triggered an event. Fields are intentionally
// declared without omitempty so every event records a complete, predictable
// shape. UID 0 (root) is a meaningful value that omitempty would silently drop,
// and a stable schema lets downstream consumers (query tools, dashboards) rely
// on every field being present.
//
// Use CaptureActor to populate this struct from the current process context.
type Actor struct {
	PPID        int    `json:"ppid"`
	ParentComm  string `json:"parent_comm"`
	TTY         string `json:"tty"`
	CWD         string `json:"cwd"`
	AgentMarker string `json:"agent_marker"`
	UID         int    `json:"uid"`
}
