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
	"time"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/audit"
)

func newAuditShowCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "show <seq>",
		Short: "Show a single audit event by sequence number",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			seq, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("audit show: invalid seq %q: %w", args[0], err)
			}
			resolved, err := resolveAuditPath(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(resolved); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("audit show: %s not found", resolved)
				}
				return fmt.Errorf("audit show: stat %s: %w", resolved, err)
			}

			warn := func(msg string) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", msg)
			}
			e, err := audit.Show(resolved, seq, warn)
			if err != nil {
				if errors.Is(err, audit.ErrNotFound) {
					return fmt.Errorf("audit show: seq %d not found", seq)
				}
				return fmt.Errorf("audit show: %w", err)
			}

			format, err := formatFromCmd(cmd, cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("audit show: %w", err)
			}
			if format == "json" {
				body, err := json.MarshalIndent(e, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			return writeEventShowText(cmd.OutOrStdout(), e)
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "audit.jsonl path (default: ~/.lsm/audit.jsonl)")
	return cmd
}

// writeEventShowText renders one event in multi-line pretty form.
func writeEventShowText(out io.Writer, e audit.Event) error {
	_, _ = fmt.Fprintf(out, "seq:             %d\n", e.Seq)
	_, _ = fmt.Fprintf(out, "ts:              %s\n", e.Timestamp.UTC().Format(time.RFC3339Nano))
	_, _ = fmt.Fprintf(out, "event:           %s\n", e.Event)
	_, _ = fmt.Fprintf(out, "app:             %s\n", e.App)
	_, _ = fmt.Fprintf(out, "env:             %s\n", e.Env)
	_, _ = fmt.Fprintf(out, "schema_version:  %d\n", e.SchemaVersion)
	_, _ = fmt.Fprintf(out, "local_only:      %v\n", e.LocalOnly)
	_, _ = fmt.Fprintf(out, "actor.ppid:         %d\n", e.Actor.PPID)
	_, _ = fmt.Fprintf(out, "actor.parent_comm:  %s\n", e.Actor.ParentComm)
	_, _ = fmt.Fprintf(out, "actor.tty:          %s\n", e.Actor.TTY)
	_, _ = fmt.Fprintf(out, "actor.cwd:          %s\n", e.Actor.CWD)
	_, _ = fmt.Fprintf(out, "actor.agent_marker: %s\n", e.Actor.AgentMarker)
	_, _ = fmt.Fprintf(out, "actor.uid:          %d\n", e.Actor.UID)
	if len(e.Fields) > 0 {
		body, err := json.MarshalIndent(e.Fields, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "fields:          %s\n", string(body))
	}
	_, _ = fmt.Fprintf(out, "prev:            %s\n", e.Prev)
	_, _ = fmt.Fprintf(out, "hash:            %s\n", e.Hash)
	return nil
}
