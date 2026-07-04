// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const setLong = `Set or update a secret.

Ways to provide the value:

  lsm set KEY VALUE        value taken from the command line
  lsm set KEY -            value read from stdin
  lsm set KEY              value entered interactively (or piped stdin)

Positional app/env always precede a required KEY VALUE:

  lsm set app KEY VALUE
  lsm set app env KEY VALUE

The value is optional only when KEY is the single positional argument. To
prompt for a specific app/env, use the --app/--env flags (or run 'lsm set KEY'
in a linked directory), for example 'lsm set --app app --env env KEY'.

When only KEY is given and stdin is a terminal, lsm prompts for the value and
reads it with no echo (the typed secret is never shown or stored in shell
history). When only KEY is given and stdin is piped or redirected, lsm reads
the value from stdin exactly as if you had passed '-', so
'echo tok | lsm set KEY' works.

For stdin input (both 'set KEY -' and piped 'set KEY'), a single trailing
newline is stripped, so 'echo tok | lsm set KEY' stores 'tok', not 'tok\n'.
Only one trailing newline is removed; interior newlines and trailing spaces
are preserved. This avoids the common 'echo' footgun where a stored token
carries an invisible trailing newline.`

func newSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [app] [env] KEY [VALUE]",
		Short: "Set or update a secret",
		Long:  setLong,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// VALUE is optional only when KEY is the single positional
			// argument. With two or more positionals the last one is a
			// required VALUE and leading positionals resolve to app/env
			// exactly as they did before VALUE became optional — this keeps
			// `set app KEY VALUE` and `set app env KEY VALUE` unambiguous.
			cfg, remaining, err := resolveWithPositional(args, setTrailingCount(len(args)))
			if err != nil {
				return err
			}

			if len(remaining) < 1 {
				return fmt.Errorf("requires KEY argument")
			}
			if len(remaining) > 2 {
				// After leading app/env are peeled, the valid trailing is at
				// most KEY VALUE. Anything more is junk — reject rather than
				// silently drop it.
				return fmt.Errorf("too many arguments")
			}

			key := remaining[0]

			var value string
			switch {
			case len(remaining) == 1:
				// Only KEY given: prompt on a TTY, otherwise read stdin like '-'.
				value, err = acquireSecret(key, isTerminal(), os.Stdin, cmd.ErrOrStderr())
				if err != nil {
					return err
				}
			case remaining[1] == "-":
				// Explicit stdin.
				data, err := readInput("-")
				if err != nil {
					return fmt.Errorf("reading from stdin: %w", err)
				}
				value = trimTrailingNewline(string(data))
			default:
				// Explicit value from the command line: stored verbatim.
				value = remaining[1]
			}

			s, err := openStore(cmd, cfg)
			if err != nil {
				return err
			}

			s.Set(key, value)
			return s.Save()
		},
	}
}

// setTrailingCount reports how many trailing positionals resolveWithPositional
// should preserve for `set`. VALUE is optional only when KEY is the single
// positional argument, so one arg keeps one trailing positional (KEY, no
// VALUE) and any larger count keeps two (KEY VALUE), letting the leading
// positionals resolve to app/env exactly as they did before VALUE became
// optional. This deliberately avoids treating `set app KEY VALUE` (three bare
// positionals) as an app/env-plus-prompt form, which would silently mis-store.
func setTrailingCount(nArgs int) int {
	if nArgs == 1 {
		return 1
	}
	return 2
}

// acquireSecret obtains a secret value when no VALUE was given on the command
// line. When isTTY is true it prompts (hidden, no echo) and requires a
// non-empty entry. Otherwise it reads all of stdin and strips a single
// trailing newline, matching the explicit 'set KEY -' form.
func acquireSecret(key string, isTTY bool, stdin io.Reader, prompt io.Writer) (string, error) {
	if isTTY {
		v, err := readSecretFromTerminal(key, prompt)
		if err != nil {
			return "", err
		}
		if v == "" {
			return "", fmt.Errorf("no value entered")
		}
		return v, nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("reading from stdin: %w", err)
	}
	return trimTrailingNewline(string(data)), nil
}

// readSecretFromTerminal prompts on prompt (stderr) and reads a line from the
// controlling terminal with echo disabled. term.ReadPassword returns the value
// without the trailing newline, so no trimming is needed; we emit our own
// newline afterward because the user's Enter is swallowed. It is a package
// var so tests can substitute a fake reader.
var readSecretFromTerminal = func(key string, prompt io.Writer) (string, error) {
	_, _ = fmt.Fprintf(prompt, "Value for %s: ", key)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(prompt)
	if err != nil {
		return "", fmt.Errorf("reading value: %w", err)
	}
	return string(b), nil
}

// trimTrailingNewline removes exactly one trailing newline: a single '\n', plus
// the '\r' immediately before it when present (so both "\n" and "\r\n" line
// endings are handled). It does not touch trailing spaces or tabs, additional
// trailing newlines, or interior newlines — only the lone trailing newline that
// tools like 'echo' append is removed.
func trimTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		s = strings.TrimSuffix(s[:len(s)-1], "\r")
	}
	return s
}
