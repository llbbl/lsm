// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/audit"
)

func newAuditSuspiciousCmd() *cobra.Command {
	var (
		path        string
		hoursWindow string
		burstN      int
		burstWindow string
		lookback    string
	)
	cmd := &cobra.Command{
		Use:   "suspicious",
		Short: "Flag suspicious patterns in the audit log",
		Long: `Scan the audit log and surface events matching one or more
heuristic detectors. The command is purely informational: it always exits 0
even when events are flagged. Detectors:

  outside_hours              event timestamp falls outside the working-hours
                             window (default 07:00-23:00 UTC).
  burst                      more than --burst-threshold events from one
                             parent_comm landed within --burst-window.
  new_parent_comm            parent_comm was not observed in records older
                             than --lookback. Skipped (with a stderr note)
                             when the audit log is younger than the window.
  non_interactive_no_agent   actor has no TTY and no agent marker.

Each event may match multiple detectors; all matching reasons are reported.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := resolveAuditPath(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(resolved); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("audit suspicious: %s not found", resolved)
				}
				return fmt.Errorf("audit suspicious: stat %s: %w", resolved, err)
			}

			hStart, hEnd, err := parseHoursWindow(hoursWindow)
			if err != nil {
				return fmt.Errorf("audit suspicious: --hours: %w", err)
			}
			bw, err := parseDurationExt(burstWindow)
			if err != nil {
				return fmt.Errorf("audit suspicious: --burst-window: %w", err)
			}
			lb, err := parseDurationExt(lookback)
			if err != nil {
				return fmt.Errorf("audit suspicious: --lookback: %w", err)
			}

			format, err := formatFromCmd(cmd, cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("audit suspicious: %w", err)
			}

			opts := audit.SuspiciousOptions{
				HoursStart:     hStart,
				HoursEnd:       hEnd,
				BurstThreshold: burstN,
				BurstWindow:    bw,
				Lookback:       lb,
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			var count int
			skipped, scanErr := audit.Suspicious(resolved, opts, func(f audit.Flagged) error {
				count++
				return writeFlaggedLine(out, f, format)
			})
			if scanErr != nil {
				return fmt.Errorf("audit suspicious: %w", scanErr)
			}
			if skipped {
				fmt.Fprintln(errOut, "note: new-parent-comm detector skipped — audit log spans less than the lookback window")
			}
			if count == 0 {
				fmt.Fprintln(out, "no suspicious events found")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "audit.jsonl path (default: ~/.lsm/audit.jsonl)")
	cmd.Flags().StringVar(&hoursWindow, "hours", "07:00-23:00", "working-hours window in UTC; events outside are flagged")
	cmd.Flags().IntVar(&burstN, "burst-threshold", 50, "flag if more than N events from one parent_comm fall within --burst-window")
	cmd.Flags().StringVar(&burstWindow, "burst-window", "1m", "duration window for burst detection (e.g. 30s, 5m)")
	cmd.Flags().StringVar(&lookback, "lookback", "30d", "treat events whose parent_comm is unseen in records older than this as 'new'")
	return cmd
}

// parseHoursWindow parses a "HH:MM-HH:MM" window into integer start/end
// hours. Minutes are accepted in the syntax but discarded — detector
// resolution is hourly. The end hour is exclusive.
func parseHoursWindow(s string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM-HH:MM, got %q", s)
	}
	startH, err := parseHourClock(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("start: %w", err)
	}
	endH, err := parseHourClock(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("end: %w", err)
	}
	return startH, endH, nil
}

// parseHourClock accepts "HH" or "HH:MM" and returns the hour component.
// Hours must be in [0, 24]; minutes (if present) must be in [0, 59] but are
// otherwise ignored.
func parseHourClock(s string) (int, error) {
	s = strings.TrimSpace(s)
	hPart := s
	mPart := ""
	if i := strings.IndexByte(s, ':'); i >= 0 {
		hPart, mPart = s[:i], s[i+1:]
	}
	h, err := strconv.Atoi(hPart)
	if err != nil {
		return 0, fmt.Errorf("invalid hour %q", s)
	}
	if h < 0 || h > 24 {
		return 0, fmt.Errorf("hour %d out of range [0,24]", h)
	}
	if mPart != "" {
		m, err := strconv.Atoi(mPart)
		if err != nil {
			return 0, fmt.Errorf("invalid minute in %q", s)
		}
		if m < 0 || m > 59 {
			return 0, fmt.Errorf("minute %d out of range [0,59]", m)
		}
	}
	return h, nil
}

// writeFlaggedLine emits one flagged event in the resolved format.
//
// Text: "[reasons,...] <columnar event row>"
// JSON: {"reasons":[...], "event":{...}}
func writeFlaggedLine(out io.Writer, f audit.Flagged, format string) error {
	if format == "json" {
		body, err := json.Marshal(struct {
			Reasons []string    `json:"reasons"`
			Event   audit.Event `json:"event"`
		}{Reasons: f.Reasons, Event: f.Event})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	}
	if _, err := fmt.Fprintf(out, "[%s] ", strings.Join(f.Reasons, ",")); err != nil {
		return err
	}
	return writeEventText(out, f.Event)
}
