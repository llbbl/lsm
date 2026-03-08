// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/store"
)

func newAppsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apps",
		Short: "List all app namespaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveDir()
			if err != nil {
				return err
			}
			apps, err := store.ListApps(dir)
			if err != nil {
				return err
			}
			for _, app := range apps {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), app); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
