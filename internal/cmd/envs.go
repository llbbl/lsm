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
		Use:   "envs APP",
		Short: "List all environments for an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveDir()
			if err != nil {
				return err
			}
			app := args[0]
			envs, err := store.ListEnvs(dir, app)
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
