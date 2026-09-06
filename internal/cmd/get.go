// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [app] [env] KEY",
		Short: "Get a single secret value",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, remaining, err := resolveWithPositional(args, 1)
			if err != nil {
				return err
			}

			if len(remaining) < 1 {
				return fmt.Errorf("requires KEY argument")
			}

			key := remaining[0]

			s, err := openStore(cmd, cfg)
			if err != nil {
				return err
			}

			val, ok := s.Get(key)
			if !ok {
				return fmt.Errorf("key %q not found", key)
			}

			out := cmd.OutOrStdout()
			writeSecretValue(out, val, isStdoutTerminal(out))
			return nil
		},
	}
}

// writeSecretValue writes a secret to w. Piped output stays byte-exact so
// `$(lsm get KEY)` and `lsm get KEY | pbcopy` see the value and nothing else.
// On a terminal a newline is added: without one the shell prints its
// partial-line marker (a trailing % in zsh) directly against the value, which
// then gets caught by a double-click selection.
func writeSecretValue(w io.Writer, val string, tty bool) {
	if tty {
		_, _ = fmt.Fprintln(w, val)
		return
	}
	_, _ = fmt.Fprint(w, val)
}
