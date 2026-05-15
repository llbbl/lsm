// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// collect runs Suspicious and returns all flagged events.
func collect(t *testing.T, path string, opts SuspiciousOptions) ([]Flagged, bool) {
	t.Helper()
	var got []Flagged
	skipped, err := Suspicious(path, opts, func(f Flagged) error {
		got = append(got, f)
		return nil
	})
	if err != nil {
		t.Fatalf("Suspicious: %v", err)
	}
	return got, skipped
}

func TestSuspicious_OutsideHours_Default(t *testing.T) {
	// Reference "now" is well past the lookback so no new-parent-comm skip.
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// One event well inside, one before the lookback (establishes "node"),
	// then in-window events at varied hours.
	hours := []int{12, 3, 23} // 12:00 inside, 03:00 outside, 23:00 outside (boundary, end is exclusive)
	path := makeChain(t, 1+len(hours), func(i int, e *Event) {
		if i == 0 {
			// Old, establishing event.
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		h := hours[i-1]
		e.Timestamp = time.Date(2026, 5, 14, h, 0, 0, 0, time.UTC)
	})

	got, skipped := collect(t, path, SuspiciousOptions{Now: now})
	if skipped {
		t.Fatal("expected new-parent-comm not skipped, file spans more than lookback")
	}
	// Expect flagged: the 03:00 and the 23:00 events only.
	// The 12:00 event is interactive, has agent, and inside hours → not flagged.
	// The old establishing event at -60d: hour=12 (12:00), TTY+agent set → not flagged.
	var outside []Flagged
	for _, f := range got {
		if slices.Contains(f.Reasons, ReasonOutsideHours) {
			outside = append(outside, f)
		}
	}
	if len(outside) != 2 {
		t.Fatalf("expected 2 outside_hours flags, got %d (%+v)", len(outside), got)
	}
}

func TestSuspicious_OutsideHours_CustomWindow(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// One old event to satisfy the lookback, one at 18:00 (outside 09-17).
	path := makeChain(t, 2, func(i int, e *Event) {
		if i == 0 {
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		e.Timestamp = time.Date(2026, 5, 14, 18, 0, 0, 0, time.UTC)
	})

	got, _ := collect(t, path, SuspiciousOptions{Now: now, HoursStart: 9, HoursEnd: 17})
	var hit bool
	for _, f := range got {
		if f.Event.Timestamp.Hour() == 18 && slices.Contains(f.Reasons, ReasonOutsideHours) {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("expected 18:00 event flagged with outside_hours, got %+v", got)
	}
}

func TestSuspicious_Burst(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// 1 old establishing event + 5 rapid events (10s apart) from "node".
	path := makeChain(t, 6, func(i int, e *Event) {
		if i == 0 {
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		// All inside working hours so we don't double-flag.
		e.Timestamp = time.Date(2026, 5, 14, 12, 0, (i-1)*10, 0, time.UTC)
	})
	opts := SuspiciousOptions{Now: now, BurstThreshold: 3, BurstWindow: time.Minute}
	got, _ := collect(t, path, opts)
	// With threshold=3 and window=1m, events 1..3 are not flagged (count
	// after appending == 1, 2, 3 — not >3). Event 4 makes the window count
	// 4 (>3) and event 5 makes it 5 (>3).
	var bursts []Flagged
	for _, f := range got {
		if slices.Contains(f.Reasons, ReasonBurst) {
			bursts = append(bursts, f)
		}
	}
	if len(bursts) != 2 {
		t.Fatalf("expected 2 burst flags, got %d (%+v)", len(bursts), got)
	}
}

func TestSuspicious_Burst_SpreadOutNotFlagged(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// 5 events spread 10 minutes apart — never more than one within any 1m window.
	path := makeChain(t, 6, func(i int, e *Event) {
		if i == 0 {
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		e.Timestamp = time.Date(2026, 5, 14, 12, (i-1)*10, 0, 0, time.UTC)
	})
	got, _ := collect(t, path, SuspiciousOptions{Now: now, BurstThreshold: 3, BurstWindow: time.Minute})
	for _, f := range got {
		if slices.Contains(f.Reasons, ReasonBurst) {
			t.Fatalf("did not expect burst, got %+v", f)
		}
	}
}

func TestSuspicious_NewParentComm(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// Old events from node and bash establish those; a recent event from
	// python should be flagged as new.
	parents := []string{"node", "bash", "python"}
	tsOffsets := []time.Duration{-60 * 24 * time.Hour, -45 * 24 * time.Hour, -1 * time.Hour}
	path := makeChain(t, 3, func(i int, e *Event) {
		e.Actor.ParentComm = parents[i]
		e.Timestamp = now.Add(tsOffsets[i])
	})
	got, skipped := collect(t, path, SuspiciousOptions{Now: now})
	if skipped {
		t.Fatal("expected detector not skipped")
	}
	var flagged bool
	for _, f := range got {
		if f.Event.Actor.ParentComm == "python" && slices.Contains(f.Reasons, ReasonNewParentComm) {
			flagged = true
		}
		if f.Event.Actor.ParentComm == "node" && slices.Contains(f.Reasons, ReasonNewParentComm) {
			t.Fatalf("node should be established, not flagged new: %+v", f)
		}
	}
	if !flagged {
		t.Fatalf("expected python flagged with new_parent_comm, got %+v", got)
	}
}

func TestSuspicious_NewParentComm_SkippedWhenFileYoung(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// All events within the lookback window — no "old" events at all.
	path := makeChain(t, 3, func(i int, e *Event) {
		e.Timestamp = now.Add(-time.Duration(i) * time.Hour)
	})
	got, skipped := collect(t, path, SuspiciousOptions{Now: now})
	if !skipped {
		t.Fatal("expected new-parent-comm detector to be skipped")
	}
	for _, f := range got {
		if slices.Contains(f.Reasons, ReasonNewParentComm) {
			t.Fatalf("should not emit new_parent_comm when skipped: %+v", f)
		}
	}
}

func TestSuspicious_NonInteractiveNoAgent(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// 4 events, all at hour 12:
	//  0: old establishing, TTY+agent set
	//  1: TTY="" agent="" → flagged
	//  2: TTY set, agent="" → not flagged
	//  3: TTY="" agent set → not flagged
	path := makeChain(t, 4, func(i int, e *Event) {
		e.Timestamp = time.Date(2026, 5, 14, 12, 0, i*5, 0, time.UTC)
		if i == 0 {
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		switch i {
		case 1:
			e.Actor.TTY = ""
			e.Actor.AgentMarker = ""
		case 2:
			e.Actor.TTY = "/dev/ttys001"
			e.Actor.AgentMarker = ""
		case 3:
			e.Actor.TTY = ""
			e.Actor.AgentMarker = "claude"
		}
	})
	got, _ := collect(t, path, SuspiciousOptions{Now: now})
	var nonInteractive int
	for _, f := range got {
		if slices.Contains(f.Reasons, ReasonNonInteractiveNoAgent) {
			nonInteractive++
			if f.Event.Actor.TTY != "" || f.Event.Actor.AgentMarker != "" {
				t.Fatalf("wrong event flagged: %+v", f)
			}
		}
	}
	if nonInteractive != 1 {
		t.Fatalf("expected 1 non_interactive_no_agent flag, got %d (%+v)", nonInteractive, got)
	}
}

func TestSuspicious_MultiReason(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// Old establishing event, then a 03:00 event with no TTY and no agent.
	path := makeChain(t, 2, func(i int, e *Event) {
		if i == 0 {
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		e.Timestamp = time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
		e.Actor.TTY = ""
		e.Actor.AgentMarker = ""
	})
	got, _ := collect(t, path, SuspiciousOptions{Now: now})
	var found *Flagged
	for i := range got {
		if got[i].Event.Timestamp.Hour() == 3 {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("did not find the 03:00 event: %+v", got)
	}
	if !slices.Contains(found.Reasons, ReasonOutsideHours) || !slices.Contains(found.Reasons, ReasonNonInteractiveNoAgent) {
		t.Fatalf("expected both outside_hours and non_interactive_no_agent, got %v", found.Reasons)
	}
}

func TestSuspicious_CallbackErrorStops(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// Two flaggable events.
	path := makeChain(t, 3, func(i int, e *Event) {
		if i == 0 {
			e.Timestamp = now.Add(-60 * 24 * time.Hour)
			return
		}
		// 03:00 events both get flagged for outside_hours.
		e.Timestamp = time.Date(2026, 5, 14, 3, 0, i*5, 0, time.UTC)
	})
	stop := errors.New("stop")
	var seen int
	_, err := Suspicious(path, SuspiciousOptions{Now: now}, func(_ Flagged) error {
		seen++
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if seen != 1 {
		t.Fatalf("expected callback to be invoked once, got %d", seen)
	}
}
