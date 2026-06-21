// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/llbbl/lsm/internal/audit"
	"github.com/llbbl/lsm/internal/config"
	"github.com/llbbl/lsm/internal/store"
)

// newGhCmd returns the parent `gh` command that owns the GitHub Actions
// secrets subcommands. It follows the same parent-with-subcommands shape as
// newAuditCmd: the parent carries no RunE of its own and simply groups the
// children.
func newGhCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gh",
		Short: "Push and inspect GitHub Actions secrets for the current project",
		Long: "Manage GitHub Actions secrets from your locally-encrypted lsm store.\n\n" +
			"gh commands are directory-bound: they operate on the app registered\n" +
			"for the current directory (via `lsm link`) and do NOT accept --app.\n" +
			"They require the GitHub CLI (`gh`) to be installed and authenticated.\n\n" +
			"Note: GitHub's secrets API is write-only. Secret VALUES can never be\n" +
			"read back; `lsm gh status` shows names and timestamps only.",
	}
	cmd.AddCommand(
		newGhPushCmd(),
		newGhStatusCmd(),
	)
	return cmd
}

// ghRunner is the seam through which all `gh` CLI invocations flow. It mirrors
// the isTerminal package-level-var pattern so tests can stub every gh call
// without a real binary or network. The default implementation shells out to
// the real `gh` on PATH.
//
// stdin, when non-nil, is wired to the child process's standard input. This is
// how secret values reach `gh secret set` without ever appearing in argv.
// stdout is returned to the caller; stderr is folded into the error on failure.
type ghRunFunc func(ctx context.Context, stdin string, args ...string) (stdout string, err error)

var ghRun ghRunFunc = defaultGhRun

func defaultGhRun(ctx context.Context, stdin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg != "" {
			return out.String(), fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), msg, err)
		}
		return out.String(), fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

// ghLookPath is a seam over exec.LookPath so tests can simulate a missing gh
// binary deterministically.
var ghLookPath = exec.LookPath

// ghResolve performs the strict, directory-bound resolution used by every gh
// command. Unlike config.Resolve it refuses any app source other than the
// registry entry for the current directory: no --app flag, no directory-name
// fallback, no .lsm.yaml app. Env is resolved independently (flag, then
// .lsm.yaml, then global config).
//
// On success it returns a fully-populated *config.Config plus the opened,
// non-empty store. Local setup is verified BEFORE any network call so users
// get a fast, clear error when the project simply isn't set up for this env.
func ghResolve(cmd *cobra.Command) (*config.Config, *store.Store, error) {
	if flagApp != "" {
		return nil, nil, fmt.Errorf("gh commands do not accept --app; they operate on the project registered for the current directory (run them from that directory, or re-link with 'lsm link <app>')")
	}

	dir, err := resolveDir()
	if err != nil {
		return nil, nil, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("getting current directory: %w", err)
	}
	// Mirror config.go: resolve symlinks so registry paths match
	// (e.g. /tmp -> /private/tmp on macOS).
	if resolved, rerr := filepath.EvalSymlinks(cwd); rerr == nil {
		cwd = resolved
	}

	globalCfg, err := config.LoadGlobalConfig(dir)
	if err != nil {
		return nil, nil, err
	}

	app := config.ResolveAppFromRegistry(globalCfg, cwd)
	if app == "" {
		return nil, nil, fmt.Errorf("current directory is not a registered lsm project: %s\nrun 'lsm link <app>' here first", cwd)
	}

	env, err := ghResolveEnv(cwd, globalCfg, dir)
	if err != nil {
		return nil, nil, err
	}

	cfg := &config.Config{Dir: dir, App: app, Env: env}

	s, err := openStore(cmd, cfg)
	if err != nil {
		return nil, nil, err
	}
	if len(s.List()) == 0 {
		return nil, nil, fmt.Errorf("no secrets found for %s/%s; this project is not set up locally for this env (run 'lsm set' / 'lsm import' first)", app, env)
	}

	return cfg, s, nil
}

// ghResolveEnv resolves the environment without touching app resolution:
// --env flag, then .lsm.yaml's env, then the global config's default env.
func ghResolveEnv(cwd string, globalCfg *config.GlobalConfig, dir string) (string, error) {
	if flagEnv != "" {
		return flagEnv, nil
	}
	if data, err := os.ReadFile(filepath.Join(cwd, ".lsm.yaml")); err == nil {
		var projCfg config.ProjectConfig
		if yerr := yaml.Unmarshal(data, &projCfg); yerr == nil && projCfg.Env != "" {
			return projCfg.Env, nil
		}
	}
	if globalCfg.Env != "" {
		return globalCfg.Env, nil
	}
	return "", fmt.Errorf("cannot determine environment: use --env, set env in .lsm.yaml, or set a default env in %s", filepath.Join(dir, "config.yaml"))
}

// ghPreflight verifies the gh CLI is installed and authenticated. It runs
// before any mutating or listing gh call so users get one clear, actionable
// error instead of a cryptic downstream failure.
func ghPreflight(ctx context.Context) error {
	if _, err := ghLookPath("gh"); err != nil {
		return fmt.Errorf("the GitHub CLI ('gh') was not found on PATH; install it from https://cli.github.com/ and run 'gh auth login'")
	}
	if _, err := ghRun(ctx, "", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not authenticated; run 'gh auth login' first (%w)", err)
	}
	return nil
}

// resolveGhRepo determines the OWNER/REPO target. An explicit --repo wins;
// otherwise the cwd's `origin` remote is parsed.
func resolveGhRepo(repoFlag string) (string, error) {
	if repoFlag != "" {
		return repoFlag, nil
	}
	url, err := gitOriginURL()
	if err != nil {
		return "", fmt.Errorf("could not determine the GitHub repo: %w; pass --repo OWNER/REPO", err)
	}
	owner, repo, ok := parseGitHubRemote(url)
	if !ok {
		return "", fmt.Errorf("could not parse a GitHub OWNER/REPO from origin remote %q; pass --repo OWNER/REPO", url)
	}
	return owner + "/" + repo, nil
}

// gitOriginURL returns the fetch URL of the `origin` remote in the cwd.
var gitOriginURL = func() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin failed (is this a git repo with an 'origin' remote?)")
	}
	return strings.TrimSpace(string(out)), nil
}

// parseGitHubRemote extracts OWNER and REPO from the common GitHub remote URL
// forms:
//
//	git@github.com:OWNER/REPO.git
//	ssh://git@github.com/OWNER/REPO.git
//	https://github.com/OWNER/REPO.git
//	https://github.com/OWNER/REPO
//
// The trailing ".git" is optional. Returns ok=false for anything that is not
// recognizably a github.com remote with both an owner and repo segment.
func parseGitHubRemote(url string) (owner, repo string, ok bool) {
	u := strings.TrimSpace(url)

	var path string
	switch {
	case strings.HasPrefix(u, "git@github.com:"):
		path = strings.TrimPrefix(u, "git@github.com:")
	case strings.HasPrefix(u, "ssh://git@github.com/"):
		path = strings.TrimPrefix(u, "ssh://git@github.com/")
	case strings.HasPrefix(u, "https://github.com/"):
		path = strings.TrimPrefix(u, "https://github.com/")
	case strings.HasPrefix(u, "http://github.com/"):
		path = strings.TrimPrefix(u, "http://github.com/")
	default:
		return "", "", false
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ghSecret is one row of `gh secret list --json name,updatedAt`.
type ghSecret struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
}

// listRemoteSecrets returns the secrets currently on GitHub for the given repo
// (and optional GitHub environment), parsed from gh's JSON output.
func listRemoteSecrets(ctx context.Context, repo, ghEnv string) ([]ghSecret, error) {
	args := []string{"secret", "list", "--repo", repo, "--json", "name,updatedAt"}
	if ghEnv != "" {
		args = append(args, "--env", ghEnv)
	}
	out, err := ghRun(ctx, "", args...)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var secrets []ghSecret
	if err := json.Unmarshal([]byte(out), &secrets); err != nil {
		return nil, fmt.Errorf("parsing gh secret list output: %w", err)
	}
	return secrets, nil
}

// confirm prints prompt to stderr and reads a [y/N] answer from cmd's stdin.
// It centralizes the dump.go confirmation idiom: --force / -y skips the
// prompt entirely, and a non-terminal without force is a hard error rather
// than a silent default. Returns (proceed, error).
func confirm(cmd *cobra.Command, force bool, prompt string) (bool, error) {
	if force {
		return true, nil
	}
	if !isTerminal() {
		return false, fmt.Errorf("refusing to proceed without confirmation (not a terminal); re-run with --force")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), prompt)
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if scanner.Scan() {
		response := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if response == "y" || response == "yes" {
			return true, nil
		}
	}
	return false, nil
}

// emitGhPushEvent appends a single audit event recording the push. It follows
// the audit.Event + Sink pattern: construct the event with application-level
// fields and hand it to a FileSink (the single chain writer) targeting the
// same <dir>/audit.jsonl path the audit command reads. Only secret NAMES are
// recorded, never values. Audit emission is best-effort: a sink error does not
// fail the push (the secrets are already set on GitHub), but it is surfaced to
// stderr so the user notices.
func emitGhPushEvent(cmd *cobra.Command, cfg *config.Config, repo, ghEnv string, names []string, pruned []string) {
	path, err := resolveAuditPath("")
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: audit: %v\n", err)
		return
	}
	sink, err := audit.NewFileSink(path)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: audit: %v\n", err)
		return
	}
	defer func() { _ = sink.Close() }()

	target := "repo"
	if ghEnv != "" {
		target = "env:" + ghEnv
	}
	fields := map[string]any{
		"repo":   repo,
		"target": target,
		"names":  names,
		"count":  len(names),
	}
	if len(pruned) > 0 {
		fields["pruned"] = pruned
		fields["pruned_count"] = len(pruned)
	}

	e := audit.Event{
		Event:  "gh_push",
		App:    cfg.App,
		Env:    cfg.Env,
		Actor:  audit.CaptureActor(),
		Fields: fields,
	}
	if err := sink.Write(cmd.Context(), e); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: audit: %v\n", err)
	}
}

// sortedNames returns a sorted copy of the input for stable display/audit.
func sortedNames(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
