// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [app] [env]",
		Short: "List secret key names",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			for _, key := range s.List() {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), key)
			}
			return nil
		},
	}
}
