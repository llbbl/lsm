// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/store"
)

func newEnvsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "envs [app]",
		Short: "List all environments for an app",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			envs, err := store.ListEnvs(cfg.Dir, cfg.App)
			if err != nil {
				return err
			}
			for _, env := range envs {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), env); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
