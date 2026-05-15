// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/llbbl/lsm/internal/dlog"
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

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
		Version:       Version,
		Short:         "Local Secrets Manager - per-app encrypted secrets with age",
		Long:          "lsm manages per-app, per-environment secrets encrypted with age.\nNo remote services, no billing, no accounts.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			l, closer, err := dlog.New("")
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
	rootCmd.PersistentFlags().StringVarP(&flagApp, "app", "a", "", "app name (default: current directory name)")
	rootCmd.PersistentFlags().StringVarP(&flagEnv, "env", "e", "", "environment name (default: from config)")

	rootCmd.AddCommand(
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
	)

	return rootCmd
}

func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
