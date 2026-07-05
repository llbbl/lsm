// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/llbbl/lsm/internal/config"
	"github.com/llbbl/lsm/internal/dlog"
	"github.com/spf13/cobra"
)

var (
	flagDir string
	flagApp string
	flagEnv string
)

// dlogState holds the per-invocation debug logger backend closer so
// PersistentPostRunE can release the file handle (if any) when the
// command tree finishes executing.
type dlogState struct {
	logger *slog.Logger
	closer io.Closer
}

var activeDlog dlogState

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "lsm",
		Version:       resolveVersion(),
		Short:         "Local Secrets Manager - per-app encrypted secrets with age",
		Long:          "lsm manages per-app, per-environment secrets encrypted with age.\nNo remote services, no billing, no accounts.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			level, dest, format, err := readLogConfig()
			if err != nil {
				return err
			}
			l, closer, err := dlog.New(level, dest, format)
			if err != nil {
				return err
			}
			activeDlog = dlogState{logger: l, closer: closer}
			c.SetContext(dlog.Into(c.Context(), l))
			l.Debug("command starting", "command", c.Name())
			return nil
		},
		PersistentPostRunE: func(c *cobra.Command, _ []string) error {
			if activeDlog.closer != nil {
				if err := activeDlog.closer.Close(); err != nil {
					dlog.From(c.Context()).Warn("dlog closer", "err", err)
				}
			}
			activeDlog = dlogState{}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(&flagDir, "dir", "d", "", "path to lsm directory (default: ~/.lsm)")
	rootCmd.PersistentFlags().StringVarP(&flagApp, "app", "a", "", "app name (overrides .lsm.yaml or linked project)")
	rootCmd.PersistentFlags().StringVarP(&flagEnv, "env", "e", "", "environment name (default: from config)")

	// Make `lsm --version` print the same rich, multi-line build details as
	// `lsm version` rather than the default single-line "lsm version X".
	// The template runs against rootCmd, but the detailed block needs runtime
	// facts cobra doesn't carry, so render it eagerly here.
	rootCmd.SetVersionTemplate(resolveVersionInfo().String())

	rootCmd.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newSetCmd(),
		newGetCmd(),
		newDeleteCmd(),
		newListCmd(),
		newDumpCmd(),
		newExecCmd(),
		newEditCmd(),
		newImportCmd(),
		newAppsCmd(),
		newEnvsCmd(),
		newLinkCmd(),
		newCleanCmd(),
		newAuditCmd(),
		newGhCmd(),
	)

	return rootCmd
}

// readLogConfig pulls the `log:` block out of the global config file
// (~/.lsm/config.yaml, or --dir/<dir>/config.yaml). A missing config
// file is non-fatal: dlog is off by default, and `lsm init` must work
// before any config exists. A malformed config IS fatal — the user
// asked lsm to read it and we can't honor that silently.
func readLogConfig() (level, dest, format string, err error) {
	dir, derr := resolveDir()
	if derr != nil {
		// Home-dir lookup failed; fall back to defaults rather than
		// blocking every command behind an environment-detection error.
		return "", "", "", nil
	}
	path := filepath.Join(dir, "config.yaml")
	if _, statErr := os.Stat(path); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return "", "", "", nil
		}
		// Unreadable for some other reason (permissions, etc.) —
		// treat as defaults so `lsm init` and friends still run.
		return "", "", "", nil
	}
	cfg, err := config.LoadGlobalConfig(dir)
	if err != nil {
		return "", "", "", err
	}
	return cfg.Log.Level, cfg.Log.Dest, cfg.Log.Format, nil
}

func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
