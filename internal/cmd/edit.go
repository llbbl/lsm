// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [app] [env]",
		Short: "Open decrypted secrets in $EDITOR, re-encrypt on save",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := resolveWithPositional(args, 0)
			if err != nil {
				return err
			}

			s, err := openStore(cmd, cfg)
			if err != nil {
				return err
			}

			editor := determineEditor()

			// Write decrypted content to a temp file *inside* the lsm
			// directory (cfg.Dir, ~/.lsm, mode 0700) rather than the shared
			// system $TMPDIR. $TMPDIR is often a world-readable location like
			// /tmp; writing decrypted secrets there exposes the plaintext to
			// other local users for the duration of the editor session and
			// leaves it behind if the process is SIGKILLed before the deferred
			// secureRemove runs. Keeping it in the owner-only lsm dir confines
			// the plaintext to a directory only the owner can traverse.
			// os.CreateTemp creates the file with mode 0600 (owner read/write
			// only), and the deferred secureRemove below still cleans it up.
			tmpFile, err := os.CreateTemp(cfg.Dir, "lsm-edit-*.env")
			if err != nil {
				return fmt.Errorf("creating temp file: %w", err)
			}
			tmpPath := tmpFile.Name()
			defer func() {
				if err := secureRemove(tmpPath); err != nil {
					slog.Warn("failed to securely remove temp file", "path", tmpPath, "error", err)
				}
			}()

			content := s.RawContent()
			if _, err := tmpFile.WriteString(content); err != nil {
				_ = tmpFile.Close()
				return fmt.Errorf("writing temp file: %w", err)
			}
			if err := tmpFile.Close(); err != nil {
				return fmt.Errorf("closing temp file: %w", err)
			}

			// Open editor — tokenize $EDITOR so values like "zed --wait" work.
			fields := strings.Fields(editor)
			if len(fields) == 0 {
				return fmt.Errorf("editor is empty")
			}
			editorArgs := append(fields[1:], tmpPath)
			editorCmd := exec.Command(fields[0], editorArgs...)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr

			if err := editorCmd.Run(); err != nil {
				return fmt.Errorf("editor exited with error: %w", err)
			}

			// Read back edited content
			edited, err := os.ReadFile(tmpPath)
			if err != nil {
				return fmt.Errorf("reading edited file: %w", err)
			}

			// Replace store contents and save
			if err := s.SetRaw(string(edited)); err != nil {
				return fmt.Errorf("parsing edited content: %w", err)
			}

			return s.Save()
		},
	}
}
