// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/audit"
)

func newAuditVerifyCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Walk audit.jsonl and confirm the hash chain is intact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved := path
			if resolved == "" {
				dir, err := resolveDir()
				if err != nil {
					return err
				}
				resolved = filepath.Join(dir, "audit.jsonl")
			}

			info, err := os.Stat(resolved)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					fmt.Fprintf(cmd.OutOrStdout(), "audit log not found at %s; nothing to verify\n", resolved)
					return nil
				}
				return fmt.Errorf("audit verify: stat %s: %w", resolved, err)
			}
			if info.Size() == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "audit log is empty; nothing to verify")
				return nil
			}

			f, err := os.Open(resolved)
			if err != nil {
				return fmt.Errorf("audit verify: open %s: %w", resolved, err)
			}
			defer f.Close()

			result, err := audit.Verify(f)
			if err != nil {
				return fmt.Errorf("audit verify: %w", err)
			}

			if !result.OK {
				return fmt.Errorf("FAIL at seq=%d line=%d: %s", result.FailSeq, result.FailLine, result.Reason)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "OK: chain valid (events=%d)\n", result.Events)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "audit.jsonl path (default: ~/.lsm/audit.jsonl)")
	return cmd
}
