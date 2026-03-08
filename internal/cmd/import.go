// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import [app] [env] [FILE]",
		Short: "Bulk import from a .env file",
		Long: `Import KEY=VALUE pairs from a .env file. Use '-' to read from stdin.

If no file is specified, lsm looks for .env files in the current directory.
If exactly one is found, it is used automatically. If multiple are found,
you'll be asked to specify which one.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, remaining, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			var file string
			if len(remaining) >= 1 {
				file = remaining[0]
			} else {
				// Auto-detect .env files in cwd
				envFiles, err := findEnvFiles()
				if err != nil {
					return fmt.Errorf("scanning for .env files: %w", err)
				}
				switch len(envFiles) {
				case 0:
					return fmt.Errorf("no .env files found in current directory; specify a file path or use '-' for stdin")
				case 1:
					file = envFiles[0]
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %s\n", file)
				default:
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Multiple .env files found:")
					for _, f := range envFiles {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", filepath.Base(f))
					}
					return fmt.Errorf("specify which file to import: lsm import <file>")
				}
			}

			content, err := readInput(file)
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			if err := s.Import(string(content)); err != nil {
				return fmt.Errorf("importing: %w", err)
			}

			if err := s.Save(); err != nil {
				return err
			}

			count := len(s.List())
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d secrets from %s\n", count, filepath.Base(file))
			if file != "-" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reminder: delete %s if it contains sensitive values\n", filepath.Base(file))
			}

			return nil
		},
	}
}
