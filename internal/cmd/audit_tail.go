// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/audit"
)

func newAuditTailCmd() *cobra.Command {
	var (
		path   string
		num    int
		follow bool
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Show the most recent audit events; optionally follow live",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := resolveAuditPath(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(resolved); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("audit tail: %s not found", resolved)
				}
				return fmt.Errorf("audit tail: stat %s: %w", resolved, err)
			}

			format, err := formatFromCmd(cmd, cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("audit tail: %w", err)
			}
			events, err := audit.Tail(resolved, num)
			if err != nil {
				return fmt.Errorf("audit tail: %w", err)
			}
			for _, e := range events {
				if err := writeEventLine(cmd.OutOrStdout(), e, format); err != nil {
					return err
				}
			}

			if !follow {
				return nil
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			err = audit.Follow(ctx, resolved, 100*time.Millisecond, func(e audit.Event) error {
				return writeEventLine(cmd.OutOrStdout(), e, format)
			})
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "audit.jsonl path (default: ~/.lsm/audit.jsonl)")
	cmd.Flags().IntVarP(&num, "num", "n", 20, "number of events to show")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "watch the file and print new events as they arrive")
	return cmd
}
