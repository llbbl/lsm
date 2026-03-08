// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/config"
)

func newLinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link APP",
		Short: "Register the current directory as an app in the central config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := args[0]

			dir, err := resolveDir()
			if err != nil {
				return err
			}

			globalCfg, err := config.LoadGlobalConfig(dir)
			if err != nil {
				return fmt.Errorf("loading global config: %w", err)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			// Resolve symlinks for consistent path matching
			if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
				cwd = resolved
			}

			if globalCfg.Apps == nil {
				globalCfg.Apps = make(map[string]string)
			}

			// Remove any existing app that points to this path
			for name, path := range globalCfg.Apps {
				if path == cwd && name != app {
					delete(globalCfg.Apps, name)
				}
			}
			globalCfg.Apps[app] = cwd

			if err := config.SaveGlobalConfig(dir, globalCfg); err != nil {
				return fmt.Errorf("saving global config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Linked %s -> %s\n", app, cwd)
			return nil
		},
	}
}
