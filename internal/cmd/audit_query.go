// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/audit"
)

func newAuditQueryCmd() *cobra.Command {
	var (
		path     string
		fApp     string
		fEnv     string
		fEvent   string
		fParent  string
		fAgent   string
		fTTY     string
		fSince   string
		fUntil   string
		fSeqFrom uint64
		fSeqTo   uint64
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Filter and stream audit events for forensics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := resolveAuditPath(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(resolved); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("audit query: %s not found", resolved)
				}
				return fmt.Errorf("audit query: stat %s: %w", resolved, err)
			}

			filter := audit.QueryFilter{
				App:         fApp,
				Env:         fEnv,
				Event:       fEvent,
				ParentComm:  fParent,
				AgentMarker: fAgent,
				SeqFrom:     fSeqFrom,
				SeqTo:       fSeqTo,
			}
			switch fTTY {
			case "":
				// no constraint
			case "present":
				b := true
				filter.TTYPresent = &b
			case "absent":
				b := false
				filter.TTYPresent = &b
			default:
				return fmt.Errorf("audit query: --tty must be 'present' or 'absent', got %q", fTTY)
			}
			if fSince != "" {
				t, err := parseTimeFlag(fSince)
				if err != nil {
					return fmt.Errorf("audit query: --since: %w", err)
				}
				filter.Since = &t
			}
			if fUntil != "" {
				t, err := parseTimeFlag(fUntil)
				if err != nil {
					return fmt.Errorf("audit query: --until: %w", err)
				}
				filter.Until = &t
			}

			format, err := formatFromCmd(cmd, cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("audit query: %w", err)
			}
			return audit.Query(resolved, filter, func(e audit.Event) error {
				return writeEventLine(cmd.OutOrStdout(), e, format)
			})
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "audit.jsonl path (default: ~/.lsm/audit.jsonl)")
	cmd.Flags().StringVar(&fApp, "app", "", "filter: exact match on app")
	cmd.Flags().StringVar(&fEnv, "env", "", "filter: exact match on env")
	cmd.Flags().StringVar(&fEvent, "event", "", "filter: exact match on event name")
	cmd.Flags().StringVar(&fParent, "parent-comm", "", "filter: exact match on actor.parent_comm")
	cmd.Flags().StringVar(&fAgent, "agent-marker", "", "filter: exact match on actor.agent_marker (e.g., claude)")
	cmd.Flags().StringVar(&fTTY, "tty", "", "filter: 'present' (TTY non-empty) or 'absent' (TTY empty)")
	cmd.Flags().StringVar(&fSince, "since", "", "filter: events with timestamp >= this (RFC3339, duration like 24h/7d, or 'now')")
	cmd.Flags().StringVar(&fUntil, "until", "", "filter: events with timestamp < this (RFC3339, duration, or 'now')")
	cmd.Flags().Uint64Var(&fSeqFrom, "seq-from", 0, "filter: events with seq >= this")
	cmd.Flags().Uint64Var(&fSeqTo, "seq-to", 0, "filter: events with seq <= this")
	return cmd
}
