// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/llbbl/lsm/internal/store"
	"github.com/spf13/cobra"
)

type envFileStatus struct {
	path        string
	keys        []string
	missingKeys []string
}

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clean [app] [env]",
		Aliases: []string{"c"},
		Short:   "Remove .env files after verifying secrets are encrypted",
		Long: `Find .env files in the current directory and remove them after verifying
that all their secrets exist in the encrypted store.

Files are only removed if every key they contain is present in the store.
Files with missing keys are skipped with a warning. Deletion uses secure
overwrite (zero-fill before remove).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			envFiles, err := findEnvFiles()
			if err != nil {
				return fmt.Errorf("scanning for .env files: %w", err)
			}
			if len(envFiles) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No .env files found in current directory")
				return nil
			}

			force, _ := cmd.Flags().GetBool("force")
			out := cmd.OutOrStdout()

			// Analyze each file
			var safe, unsafe []envFileStatus
			for _, path := range envFiles {
				content, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("reading %s: %w", filepath.Base(path), err)
				}

				entries, err := store.ParseEnv(string(content))
				if err != nil {
					return fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
				}

				var keys, missing []string
				for _, e := range entries {
					if e.Key == "" {
						continue
					}
					keys = append(keys, e.Key)
					if _, ok := s.Get(e.Key); !ok {
						missing = append(missing, e.Key)
					}
				}

				status := envFileStatus{path: path, keys: keys, missingKeys: missing}
				if len(missing) == 0 {
					safe = append(safe, status)
				} else {
					unsafe = append(unsafe, status)
				}
			}

			// Print summary
			if len(safe) > 0 {
				_, _ = fmt.Fprintf(out, "Found %d .env file(s) safe to remove:\n", len(safe))
				for _, f := range safe {
					_, _ = fmt.Fprintf(out, "  %s (%d secrets — all in encrypted store)\n", filepath.Base(f.path), len(f.keys))
				}
			}

			if len(unsafe) > 0 {
				_, _ = fmt.Fprintf(out, "\nSkipping %d file(s) with keys not in store:\n", len(unsafe))
				for _, f := range unsafe {
					_, _ = fmt.Fprintf(out, "  %s — missing: %s\n", filepath.Base(f.path), strings.Join(f.missingKeys, ", "))
				}
			}

			if len(safe) == 0 {
				_, _ = fmt.Fprintln(out, "\nNothing to remove")
				return nil
			}

			// Confirm unless --force
			if !force {
				if !isTerminal() {
					return fmt.Errorf("cannot prompt for confirmation (not a terminal); use --force")
				}
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "\nRemove these files? [y/N] ")
				scanner := bufio.NewScanner(cmd.InOrStdin())
				scanner.Scan()
				answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
				if answer != "y" && answer != "yes" {
					_, _ = fmt.Fprintln(out, "Aborted")
					return nil
				}
			}

			// Remove safe files
			for _, f := range safe {
				if err := secureRemove(f.path); err != nil {
					return fmt.Errorf("removing %s: %w", filepath.Base(f.path), err)
				}
				_, _ = fmt.Fprintf(out, "Removed %s\n", filepath.Base(f.path))
			}

			return nil
		},
	}
	cmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")
	return cmd
}
