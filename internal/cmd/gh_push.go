// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/dlog"
)

func newGhPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push local secrets to GitHub Actions",
		Long: "Push every secret in the current project's local store to GitHub Actions.\n\n" +
			"The target repo defaults to the cwd's 'origin' remote (override with --repo).\n" +
			"By default secrets are set at the repository level; use --gh-env to target a\n" +
			"specific GitHub Actions environment instead.\n\n" +
			"Secret values are streamed to 'gh secret set' on stdin and never appear in\n" +
			"argv or any temporary file. No backup is written.",
		Args: cobra.NoArgs,
		RunE: runGhPush,
	}
	cmd.Flags().String("repo", "", "target GitHub repository as OWNER/REPO (default: parsed from origin remote)")
	cmd.Flags().String("gh-env", "", "target a GitHub Actions environment's secrets instead of the repo")
	cmd.Flags().Bool("prune", false, "delete GitHub secrets that no longer exist locally")
	cmd.Flags().BoolP("force", "y", false, "skip confirmation prompts")
	return cmd
}

func runGhPush(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	repoFlag, _ := cmd.Flags().GetString("repo")
	ghEnv, _ := cmd.Flags().GetString("gh-env")
	prune, _ := cmd.Flags().GetBool("prune")
	force, _ := cmd.Flags().GetBool("force")

	cfg, s, err := ghResolve(cmd)
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

	names := sortedNames(s.List())
	dump := s.Dump()

	log := dlog.From(ctx)
	// Flow trace: NAMES only. dump holds VALUES and must NEVER be logged.
	log.Debug("gh push resolved target",
		"repo", repo, "target", ghMarkerTarget(ghEnv), "prune", prune, "secret_count", len(names))

	out := cmd.OutOrStdout()

	// Confirmation: show names (never values) and the target.
	target := repo
	if ghEnv != "" {
		target = fmt.Sprintf("%s (environment %q)", repo, ghEnv)
	}
	_, _ = fmt.Fprintf(out, "About to set %d secret(s) on %s:\n", len(names), target)
	for _, n := range names {
		_, _ = fmt.Fprintf(out, "  %s\n", n)
	}
	proceed, err := confirm(cmd, force, fmt.Sprintf("Set %d secret(s) on %s? [y/N] ", len(names), target))
	if err != nil {
		return err
	}
	if !proceed {
		_, _ = fmt.Fprintln(out, "Aborted.")
		return nil
	}

	// Push each secret with its value on stdin — never in argv.
	//
	// This loop is NOT atomic: `gh secret set` commits each secret to GitHub
	// as it runs, and no backup is taken. If the Kth call fails, the K-1
	// already-set secrets are live on the remote. Track exactly which names
	// were set so a failure can report the partial outcome AND so the audit
	// trail records reality (the subset actually set) rather than the full
	// intended set. Names only, never values.
	setNames := make([]string, 0, len(names))
	for _, name := range names {
		args := []string{"secret", "set", name, "--repo", repo}
		if ghEnv != "" {
			args = append(args, "--env", ghEnv)
		}
		if _, err := ghRun(ctx, dump[name], args...); err != nil {
			// Record the partial push before surfacing the failure: the
			// secrets in setNames are already live on GitHub.
			log.Debug("gh secret set failed", "name", name, "set_so_far", len(setNames))
			emitGhPushEvent(cmd, cfg, repo, ghEnv, setNames, nil)
			return fmt.Errorf("%d of %d secrets were set before failure on %s: %w",
				len(setNames), len(names), name, err)
		}
		log.Debug("gh secret set ok", "name", name)
		setNames = append(setNames, name)
	}

	var pruned []string
	if prune {
		pruned, err = runGhPushPrune(cmd, repo, ghEnv, names, force)
		if err != nil {
			// All secrets were set, but pruning failed partway. Record what
			// actually happened (every name set, plus whatever was deleted)
			// before surfacing the prune failure.
			emitGhPushEvent(cmd, cfg, repo, ghEnv, setNames, pruned)
			return err
		}
	}

	emitGhPushEvent(cmd, cfg, repo, ghEnv, setNames, pruned)

	// Record the durable per-app marker now that the push fully succeeded.
	// Best-effort: a marker-write failure must not fail a push whose secrets
	// are already live (warns to stderr inside writeGhMarker). NAMES/values
	// never reach the marker — only repo/target/timestamp/count.
	writeGhMarker(cmd, cfg, repo, ghEnv, len(setNames))

	_, _ = fmt.Fprintf(out, "Set %d secret(s) on %s\n", len(setNames), repo)
	if len(pruned) > 0 {
		_, _ = fmt.Fprintf(out, "Pruned %d secret(s) no longer present locally\n", len(pruned))
	}
	return nil
}

// runGhPushPrune computes the set of remote secret names absent from local,
// confirms separately, and deletes them. It honors the same --force /
// non-terminal rules as the main push confirmation.
func runGhPushPrune(cmd *cobra.Command, repo, ghEnv string, localNames []string, force bool) ([]string, error) {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	remote, err := listRemoteSecrets(ctx, repo, ghEnv)
	if err != nil {
		return nil, err
	}

	localSet := make(map[string]struct{}, len(localNames))
	for _, n := range localNames {
		localSet[n] = struct{}{}
	}

	var toDelete []string
	for _, r := range remote {
		if _, ok := localSet[r.Name]; !ok {
			toDelete = append(toDelete, r.Name)
		}
	}
	sort.Strings(toDelete)

	dlog.From(ctx).Debug("gh prune set computed",
		"repo", repo, "target", ghMarkerTarget(ghEnv), "remote_count", len(remote), "prune_count", len(toDelete))

	if len(toDelete) == 0 {
		return nil, nil
	}

	_, _ = fmt.Fprintf(out, "The following %d remote secret(s) are not present locally and will be DELETED:\n", len(toDelete))
	for _, n := range toDelete {
		_, _ = fmt.Fprintf(out, "  %s\n", n)
	}
	proceed, err := confirm(cmd, force, fmt.Sprintf("Delete %d remote secret(s) from %s? [y/N] ", len(toDelete), repo))
	if err != nil {
		return nil, err
	}
	if !proceed {
		_, _ = fmt.Fprintln(out, "Prune aborted; secrets were set but nothing was deleted.")
		return nil, nil
	}

	// Like the set loop, deletes are not atomic: each `gh secret delete`
	// removes one secret from the remote with no backup. Track how many were
	// deleted so a mid-loop failure reports the partial outcome and the caller
	// can record the names actually removed.
	deleted := make([]string, 0, len(toDelete))
	for _, name := range toDelete {
		args := []string{"secret", "delete", name, "--repo", repo}
		if ghEnv != "" {
			args = append(args, "--env", ghEnv)
		}
		if _, err := ghRun(ctx, "", args...); err != nil {
			dlog.From(ctx).Debug("gh secret delete failed", "name", name, "deleted_so_far", len(deleted))
			return deleted, fmt.Errorf("%d of %d remote secrets were deleted before failure on %s: %w",
				len(deleted), len(toDelete), name, err)
		}
		dlog.From(ctx).Debug("gh secret delete ok", "name", name)
		deleted = append(deleted, name)
	}
	return deleted, nil
}
