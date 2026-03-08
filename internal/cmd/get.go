// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [app] [env] KEY",
		Short: "Get a single secret value",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, remaining, err := resolveWithPositional(args, 1)
			if err != nil {
				return err
			}

			if len(remaining) < 1 {
				return fmt.Errorf("requires KEY argument")
			}

			key := remaining[0]

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			val, ok := s.Get(key)
			if !ok {
				return fmt.Errorf("key %q not found", key)
			}

			// Raw output, no newline
			_, _ = fmt.Fprint(cmd.OutOrStdout(), val)
			return nil
		},
	}
}
