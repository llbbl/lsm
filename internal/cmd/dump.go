// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dump [app] [env]",
		Short: "Output all secrets in .env format",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			content := s.RawContent()
			if content != "" {
				fmt.Fprint(cmd.OutOrStdout(), content)
				// Ensure trailing newline
				if content[len(content)-1] != '\n' {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			return nil
		},
	}
}
