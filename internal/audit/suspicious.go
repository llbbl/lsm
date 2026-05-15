// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import "time"

// Reason strings emitted by the suspicious detector. These are stable
// identifiers; downstream consumers may match against them.
const (
	ReasonOutsideHours          = "outside_hours"
	ReasonBurst                 = "burst"
	ReasonNewParentComm         = "new_parent_comm"
	ReasonNonInteractiveNoAgent = "non_interactive_no_agent"
)

// Flagged is one suspicious event with the reasons it was flagged. Multiple
// detectors can match the same event, so Reasons is a slice.
type Flagged struct {
	Event   Event
	Reasons []string
}

// SuspiciousOptions configures the detector pipeline.
type SuspiciousOptions struct {
	// HoursStart and HoursEnd define the working-hours window in UTC,
	// expressed as 24-hour clock hours [HoursStart, HoursEnd). Events whose
	// hour falls outside this range are flagged with ReasonOutsideHours.
	// When both fields are zero, the defaults (7, 23) are applied.
	HoursStart, HoursEnd int

	// BurstThreshold and BurstWindow configure burst detection: more than
	// BurstThreshold events from one parent_comm within BurstWindow are
	// flagged with ReasonBurst. Zero values fall back to 50 and 1 minute.
	BurstThreshold int
	BurstWindow    time.Duration

	// Lookback bounds the new-parent-comm detector. parent_comm values that
	// do not appear in records older than (Now - Lookback) are considered
	// "new" and flagged. Zero falls back to 30 days.
	Lookback time.Duration

	// Now overrides the reference time for tests. If zero, time.Now().UTC().
	Now time.Time
}

// applyDefaults fills any zero fields with their documented defaults.
func (o *SuspiciousOptions) applyDefaults() {
	if o.Now.IsZero() {
		o.Now = time.Now().UTC()
	}
	if o.BurstThreshold == 0 {
		o.BurstThreshold = 50
	}
	if o.BurstWindow == 0 {
		o.BurstWindow = time.Minute
	}
	if o.Lookback == 0 {
		o.Lookback = 30 * 24 * time.Hour
	}
	if o.HoursStart == 0 && o.HoursEnd == 0 {
		o.HoursStart = 7
		o.HoursEnd = 23
	}
}

// Suspicious scans the audit log at path and invokes fn for each flagged
// event, in file order. Iteration stops if fn returns a non-nil error.
//
// The function uses a two-pass strategy: a first pass collects the set of
// parent_comm values seen in events older than (Now - Lookback), and a
// second pass streams every event through the four detectors. Both passes
// delegate file I/O and JSONL parsing to Query.
//
// The returned skippedNewParentComm flag reports whether the new-parent-comm
// detector was skipped because the audit log contains no events older than
// the lookback window. Callers can surface this as a side-band note.
func Suspicious(path string, opts SuspiciousOptions, fn func(Flagged) error) (skippedNewParentComm bool, err error) {
	opts.applyDefaults()
	lookbackCutoff := opts.Now.Add(-opts.Lookback)

	// First pass: build the "established" parent_comm set from events older
	// than the lookback cutoff, and track the oldest timestamp seen so we
	// can decide whether the new-parent-comm detector is meaningful.
	established := make(map[string]bool)
	var oldestSeen time.Time
	var sawAny bool
	scanErr := Query(path, QueryFilter{}, func(e Event) error {
		if e.Timestamp.Before(lookbackCutoff) {
			established[e.Actor.ParentComm] = true
		}
		if !sawAny || e.Timestamp.Before(oldestSeen) {
			oldestSeen = e.Timestamp
			sawAny = true
		}
		return nil
	})
	if scanErr != nil {
		return false, scanErr
	}

	// If no event predates the lookback cutoff, every parent_comm would be
	// "new" — noisy and unhelpful. Skip that detector.
	skipNew := !sawAny || !oldestSeen.Before(lookbackCutoff)

	// Second pass: streaming detection over every event in file order.
	bursts := make(map[string][]time.Time)
	scanErr = Query(path, QueryFilter{}, func(e Event) error {
		var reasons []string

		// 1) Outside working hours.
		h := e.Timestamp.UTC().Hour()
		if h < opts.HoursStart || h >= opts.HoursEnd {
			reasons = append(reasons, ReasonOutsideHours)
		}

		// 2) Burst: maintain a per-parent_comm sliding window of timestamps.
		recent := bursts[e.Actor.ParentComm]
		cutoff := e.Timestamp.Add(-opts.BurstWindow)
		drop := 0
		for drop < len(recent) && recent[drop].Before(cutoff) {
			drop++
		}
		recent = recent[drop:]
		recent = append(recent, e.Timestamp)
		bursts[e.Actor.ParentComm] = recent
		if len(recent) > opts.BurstThreshold {
			reasons = append(reasons, ReasonBurst)
		}

		// 3) New parent_comm: only consider events inside the lookback
		// window. parent_comm == "" never counts as new (uninformative).
		if !skipNew && !e.Timestamp.Before(lookbackCutoff) && e.Actor.ParentComm != "" {
			if !established[e.Actor.ParentComm] {
				reasons = append(reasons, ReasonNewParentComm)
			}
		}

		// 4) Non-interactive AND not a known agent.
		if e.Actor.TTY == "" && e.Actor.AgentMarker == "" {
			reasons = append(reasons, ReasonNonInteractiveNoAgent)
		}

		if len(reasons) == 0 {
			return nil
		}
		return fn(Flagged{Event: e, Reasons: reasons})
	})
	if scanErr != nil {
		return skipNew, scanErr
	}
	return skipNew, nil
}
