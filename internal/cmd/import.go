// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import [app] [env] FILE",
		Short: "Bulk import from a .env file",
		Long:  "Import KEY=VALUE pairs from a .env file. Use '-' to read from stdin.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, remaining, err := resolveWithPositional(args, 1)
			if err != nil {
				return err
			}

			if len(remaining) < 1 {
				return fmt.Errorf("requires FILE argument (use '-' for stdin)")
			}

			content, err := readInput(remaining[0])
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

			return s.Save()
		},
	}
}
