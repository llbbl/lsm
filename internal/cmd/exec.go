// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var execProcess = syscall.Exec

func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "exec [app] [env] -- command [args...]",
		Short:              "Inject secrets as env vars and run a command",
		DisableFlagParsing: false,
		Annotations:        map[string]string{"stdout": "data"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// ArgsLenAtDash returns the index in args where "--" was,
			// or -1 if "--" was not used. This avoids coupling to os.Args.
			dashAt := cmd.ArgsLenAtDash()
			if dashAt < 0 || dashAt >= len(args) {
				return fmt.Errorf("usage: lsm exec [app] [env] -- command [args...]")
			}

			preArgs := args[:dashAt]
			commandArgs := args[dashAt:]

			cfg, _, err := resolveWithPositional(preArgs, 0)
			if err != nil {
				return err
			}

			s, err := openStore(cmd, cfg)
			if err != nil {
				return err
			}

			// Find the command binary
			binary, err := exec.LookPath(commandArgs[0])
			if err != nil {
				return fmt.Errorf("command not found: %s", commandArgs[0])
			}

			// Build environment: secrets override existing env vars
			secrets := s.Dump()
			env := buildEnvWithSecrets(os.Environ(), secrets)

			// Write the handoff event before replacing the current process.
			emitAudit(cmd, cfg, "exec", map[string]any{
				"command": filepath.Base(commandArgs[0]), "count": len(secrets),
			})
			return execProcess(binary, commandArgs, env)
		},
	}
}

// buildEnvWithSecrets merges secrets into an existing environment.
// Secrets override any existing variable with the same key.
func buildEnvWithSecrets(environ []string, secrets map[string]string) []string {
	overridden := make(map[string]bool, len(secrets))
	var env []string
	for _, e := range environ {
		k, _, _ := strings.Cut(e, "=")
		if v, ok := secrets[k]; ok {
			overridden[k] = true
			env = append(env, k+"="+v)
		} else {
			env = append(env, e)
		}
	}
	for k, v := range secrets {
		if !overridden[k] {
			env = append(env, k+"="+v)
		}
	}
	return env
}
