// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/crypto"
)

// defaultConfigYAML is the initial contents of ~/.lsm/config.yaml. The
// log block is commented out so the configuration surface is
// discoverable without changing the default (silent) behavior.
const defaultConfigYAML = `env: dev

# Logging - defaults are "off" / stderr / text when unset.
# log:
#   level: debug         # off | error | warn | info | debug
#   dest: stderr         # stderr | stdout | file:/absolute/path
#   format: text         # text | json
`

func newInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a new age identity (key pair)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveDir()
			if err != nil {
				return err
			}

			// Create directory
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("creating directory %s: %w", dir, err)
			}

			keyPath := filepath.Join(dir, "key.txt")

			// Check if key already exists
			if _, err := os.Stat(keyPath); err == nil && !force {
				return fmt.Errorf("key already exists at %s (use --force to overwrite)", keyPath)
			}

			// Generate identity
			identity, err := crypto.GenerateIdentity()
			if err != nil {
				return fmt.Errorf("generating identity: %w", err)
			}

			// Save identity
			if err := crypto.SaveIdentity(keyPath, identity); err != nil {
				return fmt.Errorf("saving identity: %w", err)
			}

			// Create default config.yaml if it doesn't exist. The log
			// block is commented out so users discover the surface
			// without changing default behavior (dlog stays off).
			configPath := filepath.Join(dir, "config.yaml")
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				if err := os.WriteFile(configPath, []byte(defaultConfigYAML), 0644); err != nil {
					return fmt.Errorf("writing default config: %w", err)
				}
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created key at %s\n", keyPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Public key: %s\n", identity.Recipient().String())
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing key")
	return cmd
}
