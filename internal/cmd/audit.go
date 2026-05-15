// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/audit"
)

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Inspect and verify the audit log",
	}
	// Persistent flag without a backing variable: cobra stores it on the flag
	// set, which keeps audit-format state scoped to each NewRootCmd() rather
	// than living at package scope (where it would leak across test runs).
	cmd.PersistentFlags().String("format", "", "output format: text, json, or auto (default: auto)")
	cmd.AddCommand(
		newAuditVerifyCmd(),
		newAuditTailCmd(),
		newAuditShowCmd(),
		newAuditQueryCmd(),
	)
	return cmd
}

// resolveAuditPath returns the audit.jsonl path, honoring an explicit --file
// override and otherwise falling back to <resolveDir()>/audit.jsonl.
func resolveAuditPath(file string) (string, error) {
	if file != "" {
		return file, nil
	}
	dir, err := resolveDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit.jsonl"), nil
}

// resolveFormat returns "text" or "json", honoring an explicit override and
// otherwise auto-detecting via stdout's TTY status. The cobra command's
// OutOrStdout may be a buffer in tests, so we check the real os.Stdout to
// answer "is the user looking at a terminal?". Unknown explicit values
// surface as an error so users get a clear signal rather than a silent
// fallthrough to auto-detect.
func resolveFormat(explicit string, out io.Writer) (string, error) {
	switch strings.ToLower(explicit) {
	case "json":
		return "json", nil
	case "text":
		return "text", nil
	case "", "auto":
		if isStdoutTerminal(out) {
			return "text", nil
		}
		return "json", nil
	default:
		return "", fmt.Errorf("unknown --format %q; expected json, text, or auto", explicit)
	}
}

// formatFromCmd reads the inherited --format persistent flag off cmd and
// resolves it to a concrete "text" or "json". Subcommands call this at
// execution time so cobra has finished parsing flags.
func formatFromCmd(cmd *cobra.Command, out io.Writer) (string, error) {
	explicit, _ := cmd.Flags().GetString("format")
	return resolveFormat(explicit, out)
}

// isStdoutTerminal mirrors the audit/actor.go TTY detection: char-device
// check on os.Stdout. We also confirm the cobra writer IS os.Stdout — when
// tests redirect output to a bytes.Buffer we should auto-detect as JSON
// (non-TTY), which falls out naturally because the buffer is not os.Stdout.
func isStdoutTerminal(out io.Writer) bool {
	if out != os.Stdout {
		return false
	}
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

// parseTimeFlag accepts an RFC3339 timestamp, a duration with the suffixes
// supported by parseDurationExt (h/m/s/d/w), or the literal "now". A duration
// is subtracted from now so `--since 24h` means "events in the last 24 hours".
func parseTimeFlag(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if s == "now" {
		return time.Now().UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	d, err := parseDurationExt(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: not RFC3339, not duration (%w)", s, err)
	}
	return time.Now().UTC().Add(-d), nil
}

// parseDurationExt extends time.ParseDuration to accept "Nd" and "Nw"
// suffixes. Only a single unit is supported; mixed forms like "1d12h" are
// rejected to keep the surface tight.
func parseDurationExt(s string) (time.Duration, error) {
	if len(s) < 2 {
		return time.ParseDuration(s)
	}
	last := s[len(s)-1]
	switch last {
	case 'd', 'w':
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("invalid %c-duration %q: %w", last, s, err)
		}
		mult := 24 * time.Hour
		if last == 'w' {
			mult = 7 * 24 * time.Hour
		}
		return time.Duration(n) * mult, nil
	}
	return time.ParseDuration(s)
}

// writeEventLine emits one event in the resolved format.
func writeEventLine(out io.Writer, e audit.Event, format string) error {
	if format == "json" {
		body, err := json.Marshal(e)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	}
	return writeEventText(out, e)
}

// writeEventText emits a single line in the text columnar format.
func writeEventText(out io.Writer, e audit.Event) error {
	appEnv := e.App
	if e.Env != "" {
		if appEnv != "" {
			appEnv += "/" + e.Env
		} else {
			appEnv = e.Env
		}
	}
	if appEnv == "" {
		appEnv = "-"
	}
	actor := e.Actor.ParentComm
	if e.Actor.AgentMarker != "" {
		if actor != "" {
			actor += "/" + e.Actor.AgentMarker
		} else {
			actor = e.Actor.AgentMarker
		}
	}
	if actor == "" {
		actor = "-"
	}
	tty := e.Actor.TTY
	if tty == "" {
		tty = "-"
	}
	_, err := fmt.Fprintf(out, "%s  seq=%d  event=%s  %s  %s  %s\n",
		e.Timestamp.UTC().Format(time.RFC3339), e.Seq, e.Event, appEnv, actor, tty)
	return err
}
