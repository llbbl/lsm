// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/config"
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

			// Load the GitHub markers so apps wired to GitHub can show a
			// compact "→ gh:<repo>" suffix. This is best-effort: a config
			// read failure must not break the plain app listing, so an error
			// just yields no markers (nil map) and unchanged output.
			var ghLinks map[string]config.GitHubLink
			if globalCfg, cerr := config.LoadGlobalConfig(dir); cerr == nil {
				ghLinks = globalCfg.GitHub
			}

			for _, app := range apps {
				line := app
				if link, ok := ghLinks[app]; ok && link.Repo != "" {
					line = fmt.Sprintf("%s  → gh:%s", app, link.Repo)
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
