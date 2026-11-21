// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/llbbl/lsm/internal/audit"
	"github.com/llbbl/lsm/internal/config"
	"github.com/spf13/cobra"
)

var openAuditSink = func(path string) (audit.Sink, error) {
	return audit.NewFileSink(path)
}

// emitAudit records command metadata and warns on stderr if the append fails.
func emitAudit(cmd *cobra.Command, cfg *config.Config, event string, fields map[string]any) {
	sink, err := openAuditSink(filepath.Join(cfg.Dir, "audit.jsonl"))
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: audit: %v\n", err)
		return
	}
	defer func() { _ = sink.Close() }()

	entry := audit.Event{
		Event:  event,
		App:    cfg.App,
		Env:    cfg.Env,
		Actor:  audit.CaptureActor(),
		Fields: fields,
	}
	// Record effects even if the command's context was canceled after they occurred.
	if err := sink.Write(context.WithoutCancel(cmd.Context()), entry); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: audit: %v\n", err)
	}
}
