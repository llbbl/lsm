// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "exec [app] [env] -- command [args...]",
		Short:              "Inject secrets as env vars and run a command",
		DisableFlagParsing: false,
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

			s, err := openStore(cfg)
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
			overridden := make(map[string]bool, len(secrets))
			var env []string
			for _, e := range os.Environ() {
				k, _, _ := strings.Cut(e, "=")
				if _, ok := secrets[k]; ok {
					overridden[k] = true
					env = append(env, fmt.Sprintf("%s=%s", k, secrets[k]))
				} else {
					env = append(env, e)
				}
			}
			for k, v := range secrets {
				if !overridden[k] {
					env = append(env, fmt.Sprintf("%s=%s", k, v))
				}
			}

			// Replace this process with the command
			return syscall.Exec(binary, commandArgs, env)
		},
	}
}
