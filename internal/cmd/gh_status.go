// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/llbbl/lsm/internal/config"
	"github.com/llbbl/lsm/internal/dlog"
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

	log := dlog.From(ctx)
	log.Debug("gh status resolved target",
		"repo", repo, "target", ghMarkerTarget(ghEnv), "local_count", len(s.List()))

	remote, err := listRemoteSecrets(ctx, repo, ghEnv)
	if err != nil {
		return err
	}
	log.Debug("gh status remote listed", "repo", repo, "remote_count", len(remote))

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

	reconcileGhMarker(cmd, cfg, repo, ghEnv, len(remote))

	return nil
}

// reconcileGhMarker prints the durable per-app push marker (if one exists) and
// reconciles it against the LIVE GitHub state, which is authoritative. The
// marker is only a local hint: GitHub's secrets API is write-only, so we can
// never verify it by reading values back. Drift is flagged explicitly when:
//   - the marker's repo/target differs from what is being queried now, or
//   - the live remote is empty while the marker claims a push happened.
//
// remoteCount is the number of secrets the live `gh secret list` returned for
// the queried repo/target.
func reconcileGhMarker(cmd *cobra.Command, cfg *config.Config, repo, ghEnv string, remoteCount int) {
	out := cmd.OutOrStdout()
	log := dlog.From(cmd.Context())

	globalCfg, err := config.LoadGlobalConfig(cfg.Dir)
	if err != nil {
		// Non-fatal: the live buckets above are the real answer. Surface a
		// hint but don't fail status over a local marker read.
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: github marker: %v\n", err)
		return
	}
	link, ok := globalCfg.GitHub[cfg.App]
	if !ok {
		log.Debug("gh status no marker", "app", cfg.App)
		return
	}

	queriedTarget := ghMarkerTarget(ghEnv)
	log.Debug("gh status marker found",
		"app", cfg.App, "marker_repo", link.Repo, "marker_target", link.Target,
		"queried_repo", repo, "queried_target", queriedTarget, "remote_count", remoteCount)

	_, _ = fmt.Fprintf(out, "\nLocal marker (a hint only; live GitHub state above is authoritative):\n")
	when := link.LastPushed
	if when == "" {
		when = "unknown time"
	}
	_, _ = fmt.Fprintf(out, "  lsm last pushed %d secret(s) to %s %s at %s\n",
		link.LastCount, link.Repo, link.Target, when)

	// Reconcile against the authoritative live state.
	var drift []string
	if link.Repo != repo {
		drift = append(drift, fmt.Sprintf("marker repo %q differs from queried repo %q", link.Repo, repo))
	}
	if link.Target != queriedTarget {
		drift = append(drift, fmt.Sprintf("marker target %q differs from queried target %q", link.Target, queriedTarget))
	}
	if remoteCount == 0 && len(drift) == 0 {
		// Same repo/target as the marker, yet GitHub reports no secrets:
		// something deleted them remotely since the recorded push.
		drift = append(drift, "marker claims a push but the live remote has no secrets for this target")
	}

	if len(drift) > 0 {
		_, _ = fmt.Fprintf(out, "  DRIFT: the live GitHub state disagrees with this local marker:\n")
		for _, d := range drift {
			_, _ = fmt.Fprintf(out, "    - %s\n", d)
		}
		_, _ = fmt.Fprintln(out, "  Trust the live state above; the marker is only a local record of the last lsm push.")
	}
}
