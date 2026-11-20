// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/llbbl/lsm/internal/config"
	"github.com/llbbl/lsm/internal/crypto"
	"github.com/llbbl/lsm/internal/store"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "get [app] [env] KEY",
		Short:       "Get a single secret value",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"stdout": "data"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, remaining, err := resolveWithPositional(args, 1)
			if err != nil {
				if errors.Is(err, config.ErrUnresolved) {
					return &exitCodeError{code: 5, err: err}
				}
				return err
			}

			if len(remaining) < 1 {
				return fmt.Errorf("requires KEY argument")
			}

			key := remaining[0]

			id, err := loadIdentity(cmd, cfg.Dir)
			if err != nil {
				if errors.Is(err, crypto.ErrNoIdentity) {
					return &exitCodeError{code: 4, err: err}
				}
				return err
			}
			s := store.New(cfg.Dir, cfg.App, cfg.Env, id)
			if _, err := os.Stat(s.FilePath()); os.IsNotExist(err) {
				return &exitCodeError{code: 3, err: fmt.Errorf("key %q not found", key)}
			}
			if err := s.Load(); err != nil {
				return err
			}

			val, ok := s.Get(key)
			if !ok {
				return &exitCodeError{code: 2, err: fmt.Errorf("key %q not found", key)}
			}

			emitAudit(cmd, cfg, "get", map[string]any{"key": key})
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
