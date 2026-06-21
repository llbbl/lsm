// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// ghCall records one invocation of the stubbed gh runner.
type ghCall struct {
	args  []string
	stdin string
}

// fakeGh installs a stub ghRun that records calls and returns canned output
// per first-two-args key. It also stubs ghLookPath (gh present) and forces
// `auth status` to succeed. The returned *[]ghCall is the recorded call log.
//
// responses maps a space-joined arg prefix (e.g. "secret list") to the JSON or
// text stdout to return. "auth status" always succeeds with empty output
// unless overridden.
func fakeGh(t *testing.T, responses map[string]string) *[]ghCall {
	t.Helper()
	var mu sync.Mutex
	calls := make([]ghCall, 0)

	origRun := ghRun
	origLook := ghLookPath
	ghLookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	ghRun = func(_ context.Context, stdin string, args ...string) (string, error) {
		mu.Lock()
		calls = append(calls, ghCall{args: append([]string(nil), args...), stdin: stdin})
		mu.Unlock()

		key := strings.Join(args, " ")
		// Longest matching prefix wins.
		var best string
		for k := range responses {
			if strings.HasPrefix(key, k) && len(k) > len(best) {
				best = k
			}
		}
		if best != "" {
			return responses[best], nil
		}
		if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
			return "", nil
		}
		// Default: succeed silently (e.g. secret set / delete).
		return "", nil
	}
	t.Cleanup(func() {
		ghRun = origRun
		ghLookPath = origLook
	})
	return &calls
}

// fakeGhFailOn installs a stub ghRun that behaves like fakeGh but returns an
// error for the `gh secret <verb> <name>` call matching failVerb/failName. This
// lets tests exercise the non-atomic mid-loop failure paths (set and delete)
// where some secrets are already live on the remote. auth status and
// secret list (canned via responses) still succeed.
func fakeGhFailOn(t *testing.T, responses map[string]string, failVerb, failName string) *[]ghCall {
	t.Helper()
	var mu sync.Mutex
	calls := make([]ghCall, 0)

	origRun := ghRun
	origLook := ghLookPath
	ghLookPath = func(string) (string, error) { return "/usr/bin/gh", nil }
	ghRun = func(_ context.Context, stdin string, args ...string) (string, error) {
		mu.Lock()
		calls = append(calls, ghCall{args: append([]string(nil), args...), stdin: stdin})
		mu.Unlock()

		if len(args) >= 3 && args[0] == "secret" && args[1] == failVerb && args[2] == failName {
			return "", errors.New("simulated gh failure")
		}

		key := strings.Join(args, " ")
		var best string
		for k := range responses {
			if strings.HasPrefix(key, k) && len(k) > len(best) {
				best = k
			}
		}
		if best != "" {
			return responses[best], nil
		}
		if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
			return "", nil
		}
		return "", nil
	}
	t.Cleanup(func() {
		ghRun = origRun
		ghLookPath = origLook
	})
	return &calls
}

// fakeOrigin stubs the git origin URL resolver.
func fakeOrigin(t *testing.T, url string) {
	t.Helper()
	orig := gitOriginURL
	gitOriginURL = func() (string, error) { return url, nil }
	t.Cleanup(func() { gitOriginURL = orig })
}

// forceInteractive overrides isTerminal to report a terminal stdin.
func forceInteractive(t *testing.T) {
	t.Helper()
	orig := isTerminal
	isTerminal = func() bool { return true }
	t.Cleanup(func() { isTerminal = orig })
}

// setupLinkedProject creates an lsm dir, a project dir registered as `app`,
// chdirs into the project, and seeds the given secrets into app/env=dev.
// Returns the lsm dir.
func setupLinkedProject(t *testing.T, app string, secrets map[string]string) string {
	t.Helper()
	dir := setupTestEnv(t)
	projDir := t.TempDir()
	// Resolve symlinks so the registered path matches ghResolve's EvalSymlinks.
	if resolved, err := filepath.EvalSymlinks(projDir); err == nil {
		projDir = resolved
	}
	t.Chdir(projDir)

	if _, err := runCmd(t, "link", "--dir", dir, app); err != nil {
		t.Fatalf("link error: %v", err)
	}
	for k, v := range secrets {
		if _, err := runCmd(t, "set", "--dir", dir, "--app", app, "--env", "dev", k, v); err != nil {
			t.Fatalf("set %s error: %v", k, err)
		}
	}
	// link changed cwd's config; chdir again to be safe (set may not chdir).
	t.Chdir(projDir)
	return dir
}

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{"ssh scp form", "git@github.com:llbbl/lsm.git", "llbbl", "lsm", true},
		{"ssh scp no .git", "git@github.com:llbbl/lsm", "llbbl", "lsm", true},
		{"ssh url form", "ssh://git@github.com/llbbl/lsm.git", "llbbl", "lsm", true},
		{"https with .git", "https://github.com/llbbl/lsm.git", "llbbl", "lsm", true},
		{"https no .git", "https://github.com/llbbl/lsm", "llbbl", "lsm", true},
		{"https trailing slash", "https://github.com/llbbl/lsm/", "llbbl", "lsm", true},
		{"not github", "git@gitlab.com:llbbl/lsm.git", "", "", false},
		{"missing repo", "https://github.com/llbbl", "", "", false},
		{"junk", "not a url", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubRemote(tt.url)
			if ok != tt.wantOK || owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("parseGitHubRemote(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tt.url, owner, repo, ok, tt.wantOwner, tt.wantRepo, tt.wantOK)
			}
		})
	}
}

func TestGhPush_RejectsAppFlag(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--app", "other", "--force")
	if err == nil {
		t.Fatal("expected error when --app is passed to gh push")
	}
	if !strings.Contains(err.Error(), "--app") {
		t.Errorf("error should mention --app, got: %v", err)
	}
}

func TestGhPush_AppNotRegistered(t *testing.T) {
	dir := setupTestEnv(t)
	projDir := t.TempDir()
	t.Chdir(projDir)
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--force")
	if err == nil {
		t.Fatal("expected error when cwd is not a registered project")
	}
	if !strings.Contains(err.Error(), "lsm link") {
		t.Errorf("error should point to 'lsm link', got: %v", err)
	}
}

func TestGhPush_EmptyStoreRefused(t *testing.T) {
	dir := setupTestEnv(t)
	projDir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(projDir); err == nil {
		projDir = resolved
	}
	t.Chdir(projDir)
	if _, err := runCmd(t, "link", "--dir", dir, "emptyapp"); err != nil {
		t.Fatalf("link error: %v", err)
	}
	t.Chdir(projDir)
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--force")
	if err == nil {
		t.Fatal("expected error for empty/missing store")
	}
	if !strings.Contains(err.Error(), "not set up locally") {
		t.Errorf("error should mention local setup, got: %v", err)
	}
}

func TestGhPush_HappyPath_ValuesViaStdin(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"DB_URL":  "postgres://localhost",
		"API_KEY": "sk-secret-value",
	})
	calls := fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	out, err := runCmd(t, "gh", "push", "--dir", dir, "--force")
	if err != nil {
		t.Fatalf("gh push error: %v", err)
	}
	if !strings.Contains(out, "Set 2 secret(s) on llbbl/lsm") {
		t.Errorf("missing summary: %s", out)
	}

	// Verify each value went via stdin and never appeared in argv.
	setByName := map[string]ghCall{}
	for _, c := range *calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "set" {
			setByName[c.args[2]] = c
		}
	}
	if len(setByName) != 2 {
		t.Fatalf("expected 2 secret set calls, got %d (%v)", len(setByName), *calls)
	}
	wantStdin := map[string]string{"DB_URL": "postgres://localhost", "API_KEY": "sk-secret-value"}
	for name, want := range wantStdin {
		c, ok := setByName[name]
		if !ok {
			t.Errorf("no set call for %s", name)
			continue
		}
		if c.stdin != want {
			t.Errorf("%s stdin = %q, want %q", name, c.stdin, want)
		}
		joined := strings.Join(c.args, " ")
		if strings.Contains(joined, want) {
			t.Errorf("secret VALUE leaked into argv for %s: %v", name, c.args)
		}
		// repo flag must be present.
		if !strings.Contains(joined, "--repo llbbl/lsm") {
			t.Errorf("set call missing --repo: %v", c.args)
		}
	}
}

func TestGhPush_RepoFlagOverride(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	calls := fakeGh(t, nil)
	// Origin points elsewhere; --repo must win.
	fakeOrigin(t, "git@github.com:someone/other.git")
	forceInteractive(t)

	out, err := runCmd(t, "gh", "push", "--dir", dir, "--repo", "acme/widget", "--force")
	if err != nil {
		t.Fatalf("gh push error: %v", err)
	}
	if !strings.Contains(out, "acme/widget") {
		t.Errorf("summary should reference --repo target: %s", out)
	}
	found := false
	for _, c := range *calls {
		if len(c.args) >= 2 && c.args[0] == "secret" && c.args[1] == "set" {
			if strings.Contains(strings.Join(c.args, " "), "--repo acme/widget") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no set call used --repo acme/widget: %v", *calls)
	}
}

func TestGhPush_GhEnvPassthrough(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	calls := fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	if _, err := runCmd(t, "gh", "push", "--dir", dir, "--gh-env", "production", "--force"); err != nil {
		t.Fatalf("gh push error: %v", err)
	}
	found := false
	for _, c := range *calls {
		if len(c.args) >= 2 && c.args[0] == "secret" && c.args[1] == "set" {
			if strings.Contains(strings.Join(c.args, " "), "--env production") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("set call missing --env production: %v", *calls)
	}
}

func TestGhPush_ConfirmDecline(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	calls := fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	rootCmd := NewRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetIn(strings.NewReader("n\n"))
	rootCmd.SetArgs([]string{"gh", "push", "--dir", dir})
	flagDir, flagApp, flagEnv = "", "", ""

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("gh push error: %v", err)
	}
	if !strings.Contains(buf.String(), "Aborted") {
		t.Errorf("expected Aborted on decline: %s", buf.String())
	}
	for _, c := range *calls {
		if len(c.args) >= 2 && c.args[0] == "secret" && c.args[1] == "set" {
			t.Errorf("no secret should be set after decline, got: %v", c.args)
		}
	}
}

func TestGhPush_ConfirmAccept(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	calls := fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	rootCmd := NewRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetIn(strings.NewReader("y\n"))
	rootCmd.SetArgs([]string{"gh", "push", "--dir", dir})
	flagDir, flagApp, flagEnv = "", "", ""

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("gh push error: %v", err)
	}
	setCount := 0
	for _, c := range *calls {
		if len(c.args) >= 2 && c.args[0] == "secret" && c.args[1] == "set" {
			setCount++
		}
	}
	if setCount != 1 {
		t.Errorf("expected 1 secret set after accept, got %d", setCount)
	}
}

func TestGhPush_NonTerminalWithoutForceRefused(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceNonInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir)
	if err == nil {
		t.Fatal("expected refusal in non-terminal without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}
}

func TestGhPush_Prune_DeletesRemoteOnly(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"KEEP_ME": "v1",
	})
	// Remote has KEEP_ME (local) and STALE_ONE / STALE_TWO (not local).
	resp := map[string]string{
		"secret list": `[{"name":"KEEP_ME","updatedAt":"2026-01-01T00:00:00Z"},` +
			`{"name":"STALE_ONE","updatedAt":"2026-01-02T00:00:00Z"},` +
			`{"name":"STALE_TWO","updatedAt":"2026-01-03T00:00:00Z"}]`,
	}
	calls := fakeGh(t, resp)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	out, err := runCmd(t, "gh", "push", "--dir", dir, "--prune", "--force")
	if err != nil {
		t.Fatalf("gh push --prune error: %v", err)
	}
	if !strings.Contains(out, "Pruned 2 secret(s)") {
		t.Errorf("expected prune summary of 2: %s", out)
	}

	var deleted []string
	for _, c := range *calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "delete" {
			deleted = append(deleted, c.args[2])
		}
	}
	sort.Strings(deleted)
	want := []string{"STALE_ONE", "STALE_TWO"}
	if strings.Join(deleted, ",") != strings.Join(want, ",") {
		t.Errorf("deleted = %v, want %v", deleted, want)
	}
}

func TestGhPush_PreflightGhMissing(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	origLook := ghLookPath
	ghLookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { ghLookPath = origLook })

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--force")
	if err == nil {
		t.Fatal("expected error when gh is not on PATH")
	}
	if !strings.Contains(err.Error(), "GitHub CLI") {
		t.Errorf("error should mention GitHub CLI, got: %v", err)
	}
}

func TestGhPush_PartialFailure_ReportsKofN(t *testing.T) {
	// Secrets are pushed in sorted name order: AAA, MMM, ZZZ. Failing on MMM
	// means AAA (1) was set before the failure on the 3-secret push.
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"AAA": "v1",
		"MMM": "v2",
		"ZZZ": "v3",
	})
	calls := fakeGhFailOn(t, nil, "set", "MMM")
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--force")
	if err == nil {
		t.Fatal("expected error when a mid-loop secret set fails")
	}
	if !strings.Contains(err.Error(), "1 of 3 secrets were set before failure on MMM") {
		t.Errorf("error should report K-of-N count and failing name, got: %v", err)
	}

	// ZZZ must not have been attempted after the failure on MMM.
	for _, c := range *calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "set" && c.args[2] == "ZZZ" {
			t.Errorf("ZZZ should not be set after MMM fails: %v", c.args)
		}
	}
}

func TestGhPush_PartialPrune_ReportsKofN(t *testing.T) {
	// All sets succeed; pruning deletes remote-only secrets in sorted order:
	// STALE_A, STALE_B, STALE_C. Failing the delete of STALE_B means 1 was
	// deleted (STALE_A) before the failure.
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"KEEP_ME": "v1",
	})
	resp := map[string]string{
		"secret list": `[{"name":"KEEP_ME","updatedAt":"2026-01-01T00:00:00Z"},` +
			`{"name":"STALE_A","updatedAt":"2026-01-02T00:00:00Z"},` +
			`{"name":"STALE_B","updatedAt":"2026-01-03T00:00:00Z"},` +
			`{"name":"STALE_C","updatedAt":"2026-01-04T00:00:00Z"}]`,
	}
	calls := fakeGhFailOn(t, resp, "delete", "STALE_B")
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--prune", "--force")
	if err == nil {
		t.Fatal("expected error when a mid-loop prune delete fails")
	}
	if !strings.Contains(err.Error(), "1 of 3 remote secrets were deleted before failure on STALE_B") {
		t.Errorf("error should report K-of-N delete count and failing name, got: %v", err)
	}

	// STALE_C must not have been attempted after STALE_B failed.
	for _, c := range *calls {
		if len(c.args) >= 3 && c.args[0] == "secret" && c.args[1] == "delete" && c.args[2] == "STALE_C" {
			t.Errorf("STALE_C should not be deleted after STALE_B fails: %v", c.args)
		}
	}
}

func TestGhPush_RejectsPositionalArg(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--force", "myapp")
	if err == nil {
		t.Fatal("expected error for stray positional arg to gh push")
	}
	if !strings.Contains(err.Error(), "unknown command") &&
		!strings.Contains(err.Error(), "arg") {
		t.Errorf("error should reject the positional arg, got: %v", err)
	}
}

func TestGhStatus_RejectsPositionalArg(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "status", "--dir", dir, "myapp")
	if err == nil {
		t.Fatal("expected error for stray positional arg to gh status")
	}
	if !strings.Contains(err.Error(), "unknown command") &&
		!strings.Contains(err.Error(), "arg") {
		t.Errorf("error should reject the positional arg, got: %v", err)
	}
}

func TestGhStatus_Buckets(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"BOTH":       "v1",
		"LOCAL_ONLY": "v2",
	})
	resp := map[string]string{
		"secret list": `[{"name":"BOTH","updatedAt":"2026-05-05T10:00:00Z"},` +
			`{"name":"REMOTE_ONLY","updatedAt":"2026-05-06T10:00:00Z"}]`,
	}
	fakeGh(t, resp)
	fakeOrigin(t, "https://github.com/llbbl/lsm.git")
	forceInteractive(t)

	out, err := runCmd(t, "gh", "status", "--dir", dir)
	if err != nil {
		t.Fatalf("gh status error: %v", err)
	}

	if !strings.Contains(out, "In sync (1)") {
		t.Errorf("missing in-sync bucket: %s", out)
	}
	if !strings.Contains(out, "Local only — would be pushed (1)") {
		t.Errorf("missing local-only bucket: %s", out)
	}
	if !strings.Contains(out, "Remote only — on GitHub, not local (1)") {
		t.Errorf("missing remote-only bucket: %s", out)
	}
	if !strings.Contains(out, "BOTH") || !strings.Contains(out, "LOCAL_ONLY") || !strings.Contains(out, "REMOTE_ONLY") {
		t.Errorf("output missing expected names: %s", out)
	}
	// updatedAt timestamps must surface for remote entries.
	if !strings.Contains(out, "2026-05-05T10:00:00Z") || !strings.Contains(out, "2026-05-06T10:00:00Z") {
		t.Errorf("output missing updatedAt timestamps: %s", out)
	}
}
