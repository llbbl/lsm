// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/config"
)

func newLinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link APP [ENV]",
		Short: "Create .lsm.yaml in the current directory",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := args[0]
			env := ""
			if len(args) > 1 {
				env = args[1]
			}

			projCfg := &config.ProjectConfig{
				App: app,
			}
			if env != "" {
				projCfg.Env = env
			}

			if err := config.SaveProjectConfig(".", projCfg); err != nil {
				return fmt.Errorf("creating .lsm.yaml: %w", err)
			}

			if env != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Created .lsm.yaml (app: %s, env: %s)\n", app, env)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Created .lsm.yaml (app: %s)\n", app)
			}
			return nil
		},
	}
}
