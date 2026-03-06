// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	flagDir string
	flagApp string
	flagEnv string
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "lsm",
		Short:         "Local Secrets Manager - per-app encrypted secrets with age",
		Long:          "lsm manages per-app, per-environment secrets encrypted with age.\nNo remote services, no billing, no accounts.",
		SilenceUsage:  true,
		SilenceErrors: true,
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
