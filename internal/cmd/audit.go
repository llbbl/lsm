// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import "github.com/spf13/cobra"

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Inspect and verify the audit log",
	}
	cmd.AddCommand(newAuditVerifyCmd())
	return cmd
}
