// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"os/exec"

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

			s, err := openStore(cfg)
			if err != nil {
				return err
			}

			// Determine editor
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = os.Getenv("VISUAL")
			}
			if editor == "" {
				editor = "vi"
			}

			// Write decrypted content to temp file
			tmpFile, err := os.CreateTemp("", "lsm-edit-*.env")
			if err != nil {
				return fmt.Errorf("creating temp file: %w", err)
			}
			tmpPath := tmpFile.Name()
			defer func() {
				// Overwrite before removing for secure deletion
				if info, err := os.Stat(tmpPath); err == nil {
					zeros := make([]byte, info.Size())
					os.WriteFile(tmpPath, zeros, 0600)
				}
				os.Remove(tmpPath)
			}()

			content := s.RawContent()
			if _, err := tmpFile.WriteString(content); err != nil {
				tmpFile.Close()
				return fmt.Errorf("writing temp file: %w", err)
			}
			tmpFile.Close()

			// Open editor
			editorCmd := exec.Command(editor, tmpPath)
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
