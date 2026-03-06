// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [app] [env] KEY",
		Short: "Remove a secret",
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

			if !s.Delete(key) {
				return fmt.Errorf("key %q not found", key)
			}

			return s.Save()
		},
	}
}
