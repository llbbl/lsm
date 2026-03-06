// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [app] [env] KEY VALUE",
		Short: "Set or update a secret",
		Long:  "Set or update a secret. Use '-' as value to read from stdin.",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, remaining, err := resolveWithPositional(args, 2)
			if err != nil {
				return err
			}

			if len(remaining) < 2 {
				return fmt.Errorf("requires KEY and VALUE arguments")
			}

			key := remaining[0]
			value := remaining[1]

			// Read from stdin if value is "-"
			if value == "-" {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading from stdin: %w", err)
				}
				value = string(data)
			}

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			s.Set(key, value)
			return s.Save()
		},
	}
}
