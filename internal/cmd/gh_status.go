// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newGhStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Compare local secrets with GitHub Actions secrets",
		Long: "Show how the current project's local secrets compare to what is on GitHub.\n\n" +
			"Three buckets are printed: in sync (both sides), local-only (would be\n" +
			"pushed), and remote-only (on GitHub, not local). GitHub's secrets API is\n" +
			"write-only, so only NAMES and update timestamps are shown — values can\n" +
			"never be read back.",
		Args: cobra.NoArgs,
		RunE: runGhStatus,
	}
	cmd.Flags().String("repo", "", "target GitHub repository as OWNER/REPO (default: parsed from origin remote)")
	cmd.Flags().String("gh-env", "", "inspect a GitHub Actions environment's secrets instead of the repo")
	return cmd
}

func runGhStatus(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	repoFlag, _ := cmd.Flags().GetString("repo")
	ghEnv, _ := cmd.Flags().GetString("gh-env")

	_, s, err := ghResolve(cmd)
	if err != nil {
		return err
	}

	if err := ghPreflight(ctx); err != nil {
		return err
	}

	repo, err := resolveGhRepo(repoFlag)
	if err != nil {
		return err
	}

	remote, err := listRemoteSecrets(ctx, repo, ghEnv)
	if err != nil {
		return err
	}

	localSet := make(map[string]struct{})
	for _, n := range s.List() {
		localSet[n] = struct{}{}
	}
	remoteSet := make(map[string]string, len(remote)) // name -> updatedAt
	for _, r := range remote {
		remoteSet[r.Name] = r.UpdatedAt
	}

	var inSync, localOnly, remoteOnly []string
	for name := range localSet {
		if _, ok := remoteSet[name]; ok {
			inSync = append(inSync, name)
		} else {
			localOnly = append(localOnly, name)
		}
	}
	for name := range remoteSet {
		if _, ok := localSet[name]; !ok {
			remoteOnly = append(remoteOnly, name)
		}
	}
	sort.Strings(inSync)
	sort.Strings(localOnly)
	sort.Strings(remoteOnly)

	out := cmd.OutOrStdout()
	target := repo
	if ghEnv != "" {
		target = fmt.Sprintf("%s (environment %q)", repo, ghEnv)
	}
	_, _ = fmt.Fprintf(out, "GitHub secrets status for %s\n\n", target)

	_, _ = fmt.Fprintf(out, "In sync (%d):\n", len(inSync))
	for _, n := range inSync {
		_, _ = fmt.Fprintf(out, "  %s  (github updated %s)\n", n, remoteSet[n])
	}

	_, _ = fmt.Fprintf(out, "\nLocal only — would be pushed (%d):\n", len(localOnly))
	for _, n := range localOnly {
		_, _ = fmt.Fprintf(out, "  %s\n", n)
	}

	_, _ = fmt.Fprintf(out, "\nRemote only — on GitHub, not local (%d):\n", len(remoteOnly))
	for _, n := range remoteOnly {
		_, _ = fmt.Fprintf(out, "  %s  (github updated %s)\n", n, remoteSet[n])
	}

	return nil
}
