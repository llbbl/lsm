// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// ProjectedLog is the redacted, label-tagged projection of an Event
// that remote sinks emit. It is NOT a valid audit.Event — there is no
// hash, no prev, no full timestamp formatting; it is the line shape
// for Loki/OTLP, not for the local chain.
type ProjectedLog struct {
	// Labels (low-cardinality, indexed by Loki).
	Labels map[string]string // event, app (hashed), env (hashed), host, tty_present, agent_marker

	// Body (high-cardinality or sensitive, but still allowed off-machine).
	Body map[string]any // seq, ts, event, actor.{parent_comm, ppid, uid, agent_marker}, fields.* (filtered)
}

// Transformer applies the project's redaction policy to an Event.
// Salt is required for hashing app/env; HostName is the value used for
// the host label.
type Transformer struct {
	Salt     []byte
	HostName string
}

// fieldAllowlist is the set of Event.Fields keys whose values are safe
// to ship verbatim off-machine. Everything else is replaced with
// `<name>_present: true`. The allowlist is intentionally empty at this
// stage; widening it is a future ticket.
var fieldAllowlist = map[string]struct{}{}

// Project applies the allowlist + redaction rules to e and returns the
// projection ready for a remote sink to emit. If e.LocalOnly is true,
// or e.Event starts with "audit.", Project returns (nil, false) — the
// caller MUST drop without further processing.
//
// The LocalOnly and "audit.*" checks are the FIRST two gates and run
// before any field is read or transformed. The contract (see
// docs/observability.md, "Implementation contract for remote sinks")
// is that privileged data must not be transformed at all — not
// hashed, not redacted, not logged at debug level — because each of
// those steps risks leaking via metrics, traces, or future
// instrumentation.
func (t Transformer) Project(e Event) (*ProjectedLog, bool) {
	// Gate 1: caller-flagged local-only. Return before touching any
	// other field of e.
	if e.LocalOnly {
		return nil, false
	}
	// Gate 2: audit.* events describe the audit system itself and are
	// always local by convention.
	if strings.HasPrefix(e.Event, "audit.") {
		return nil, false
	}

	labels := map[string]string{
		"event":        e.Event,
		"app":          t.hashLabel(e.App),
		"env":          t.hashLabel(e.Env),
		"host":         t.HostName,
		"tty_present":  ttyPresent(e.Actor.TTY),
		"agent_marker": agentMarkerLabel(e.Actor.AgentMarker),
	}

	body := map[string]any{
		"seq":                e.Seq,
		"ts":                 e.Timestamp.UTC().Format(time.RFC3339),
		"event":              e.Event,
		"actor.ppid":         e.Actor.PPID,
		"actor.uid":          e.Actor.UID,
		"actor.parent_comm":  e.Actor.ParentComm,
		"actor.agent_marker": e.Actor.AgentMarker,
	}

	for k, v := range e.Fields {
		if _, ok := fieldAllowlist[k]; ok {
			body["fields."+k] = v
			continue
		}
		body["fields."+k+"_present"] = true
	}

	return &ProjectedLog{Labels: labels, Body: body}, true
}

// hashLabel returns the first 12 hex chars of HMAC-SHA256(salt, value).
// Empty input maps to empty output — an absent app/env is not a
// meaningful value to hash.
func (t Transformer) hashLabel(value string) string {
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, t.Salt)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))[:12]
}

func ttyPresent(tty string) string {
	if tty != "" {
		return "true"
	}
	return "false"
}

func agentMarkerLabel(marker string) string {
	if marker == "" {
		return "none"
	}
	return marker
}
