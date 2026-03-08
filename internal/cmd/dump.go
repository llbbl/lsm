// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newDumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump [app] [env]",
		Short: "Dump secrets to a .env file (masked output to terminal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			keys := s.List()
			if len(keys) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No secrets to dump.")
				return nil
			}

			// Determine output file path.
			output, _ := cmd.Flags().GetString("output")
			if output == "" {
				output = ".env"
			}

			// Check if output file already exists.
			if _, err := os.Stat(output); err == nil {
				// File exists — prompt for confirmation.
				if isTerminal() {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s already exists. Overwrite? [y/N] ", output)
					scanner := bufio.NewScanner(cmd.InOrStdin())
					if scanner.Scan() {
						response := strings.TrimSpace(strings.ToLower(scanner.Text()))
						if response != "y" && response != "yes" {
							_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
							return nil
						}
					}
				}
			}

			// Write real .env content to file.
			content := s.RawContent()
			if err := os.WriteFile(output, []byte(content), 0600); err != nil {
				return fmt.Errorf("writing output file: %w", err)
			}

			// Print masked values to terminal.
			dump := s.Dump()
			for _, key := range keys {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, maskValue(dump[key]))
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d secrets to %s\n", len(keys), output)

			// Auto-update .gitignore if in a git repo.
			if isInGitRepo() {
				added, err := ensureGitignored(filepath.Base(output))
				if err != nil {
					slog.Debug("failed to update .gitignore", "error", err)
				} else if added {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Added %s to .gitignore\n", filepath.Base(output))
				}
			}

			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file path (default: .env)")

	return cmd
}
