// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/config"
)

// loadMarker reads the github marker for app from the lsm dir's config.yaml.
func loadMarker(t *testing.T, dir, app string) (config.GitHubLink, bool) {
	t.Helper()
	cfg, err := config.LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	link, ok := cfg.GitHub[app]
	return link, ok
}

func TestGhPush_WritesMarkerOnSuccess(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"DB_URL":  "postgres://localhost",
		"API_KEY": "sk-secret-value",
	})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	if _, err := runCmd(t, "gh", "push", "--dir", dir, "--force"); err != nil {
		t.Fatalf("gh push error: %v", err)
	}

	link, ok := loadMarker(t, dir, "myapp")
	if !ok {
		t.Fatalf("expected a github marker for myapp after a successful push")
	}
	if link.Repo != "llbbl/lsm" {
		t.Errorf("marker Repo = %q, want llbbl/lsm", link.Repo)
	}
	if link.Target != "actions" {
		t.Errorf("marker Target = %q, want actions", link.Target)
	}
	if link.LastCount != 2 {
		t.Errorf("marker LastCount = %d, want 2", link.LastCount)
	}
	if link.LastPushed == "" {
		t.Errorf("marker LastPushed should be set")
	}

	// The marker must never contain a secret value or name. Assert against
	// the raw config.yaml bytes.
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	content := string(data)
	for _, leak := range []string{"postgres://localhost", "sk-secret-value", "DB_URL", "API_KEY"} {
		if strings.Contains(content, leak) {
			t.Errorf("marker config.yaml leaked %q:\n%s", leak, content)
		}
	}
}

func TestGhPush_MarkerTargetForGhEnv(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"K": "v"})
	fakeGh(t, nil)
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	if _, err := runCmd(t, "gh", "push", "--dir", dir, "--gh-env", "production", "--force"); err != nil {
		t.Fatalf("gh push error: %v", err)
	}
	link, ok := loadMarker(t, dir, "myapp")
	if !ok {
		t.Fatalf("expected marker after push")
	}
	if link.Target != "env:production" {
		t.Errorf("marker Target = %q, want env:production", link.Target)
	}
}

func TestGhPush_NoMarkerWhenSetFailsBeforeAnySecret(t *testing.T) {
	// Failing on the FIRST secret (sorted order: AAA) means no secret was ever
	// set, so no durable marker should be written.
	dir := setupLinkedProject(t, "myapp", map[string]string{
		"AAA": "v1",
		"BBB": "v2",
	})
	fakeGhFailOn(t, nil, "set", "AAA")
	fakeOrigin(t, "git@github.com:llbbl/lsm.git")
	forceInteractive(t)

	_, err := runCmd(t, "gh", "push", "--dir", dir, "--force")
	if err == nil {
		t.Fatal("expected error when the first secret set fails")
	}
	if _, ok := loadMarker(t, dir, "myapp"); ok {
		t.Errorf("no marker should be written when the push fails before any secret is set")
	}
}

func TestGhStatus_PrintsMarkerNoDrift(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"BOTH": "v1"})
	resp := map[string]string{
		"secret list": `[{"name":"BOTH","updatedAt":"2026-05-05T10:00:00Z"}]`,
	}
	fakeGh(t, resp)
	fakeOrigin(t, "https://github.com/llbbl/lsm.git")
	forceInteractive(t)

	// Seed a marker that agrees with the live state (same repo/target, remote
	// non-empty).
	if err := config.SetGitHubLink(dir, "myapp", config.GitHubLink{
		Repo: "llbbl/lsm", Target: "actions", LastPushed: "2026-05-04T09:00:00Z", LastCount: 1,
	}); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	out, err := runCmd(t, "gh", "status", "--dir", dir)
	if err != nil {
		t.Fatalf("gh status error: %v", err)
	}
	if !strings.Contains(out, "lsm last pushed 1 secret(s) to llbbl/lsm actions at 2026-05-04T09:00:00Z") {
		t.Errorf("missing marker line in status output:\n%s", out)
	}
	if !strings.Contains(out, "authoritative") {
		t.Errorf("status should make clear live state is authoritative:\n%s", out)
	}
	if strings.Contains(out, "DRIFT") {
		t.Errorf("did not expect drift when marker agrees with live state:\n%s", out)
	}
}

func TestGhStatus_FlagsDriftOnRepoMismatch(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"BOTH": "v1"})
	resp := map[string]string{
		"secret list": `[{"name":"BOTH","updatedAt":"2026-05-05T10:00:00Z"}]`,
	}
	fakeGh(t, resp)
	fakeOrigin(t, "https://github.com/llbbl/lsm.git")
	forceInteractive(t)

	// Marker recorded against a DIFFERENT repo than the one being queried.
	if err := config.SetGitHubLink(dir, "myapp", config.GitHubLink{
		Repo: "someone/elsewhere", Target: "actions", LastPushed: "2026-05-04T09:00:00Z", LastCount: 1,
	}); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	out, err := runCmd(t, "gh", "status", "--dir", dir)
	if err != nil {
		t.Fatalf("gh status error: %v", err)
	}
	if !strings.Contains(out, "DRIFT") {
		t.Errorf("expected DRIFT flag on repo mismatch:\n%s", out)
	}
	if !strings.Contains(out, "someone/elsewhere") || !strings.Contains(out, "llbbl/lsm") {
		t.Errorf("drift message should name both repos:\n%s", out)
	}
}

func TestGhStatus_FlagsDriftWhenRemoteEmpty(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"LOCAL_ONLY": "v1"})
	// Live remote is empty while the marker claims a push happened.
	resp := map[string]string{
		"secret list": `[]`,
	}
	fakeGh(t, resp)
	fakeOrigin(t, "https://github.com/llbbl/lsm.git")
	forceInteractive(t)

	if err := config.SetGitHubLink(dir, "myapp", config.GitHubLink{
		Repo: "llbbl/lsm", Target: "actions", LastPushed: "2026-05-04T09:00:00Z", LastCount: 2,
	}); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	out, err := runCmd(t, "gh", "status", "--dir", dir)
	if err != nil {
		t.Fatalf("gh status error: %v", err)
	}
	if !strings.Contains(out, "DRIFT") {
		t.Errorf("expected DRIFT when marker claims a push but remote is empty:\n%s", out)
	}
	if !strings.Contains(out, "no secrets") {
		t.Errorf("drift message should explain the empty remote:\n%s", out)
	}
}

func TestGhStatus_NoMarker_NoMarkerSection(t *testing.T) {
	dir := setupLinkedProject(t, "myapp", map[string]string{"BOTH": "v1"})
	resp := map[string]string{
		"secret list": `[{"name":"BOTH","updatedAt":"2026-05-05T10:00:00Z"}]`,
	}
	fakeGh(t, resp)
	fakeOrigin(t, "https://github.com/llbbl/lsm.git")
	forceInteractive(t)

	out, err := runCmd(t, "gh", "status", "--dir", dir)
	if err != nil {
		t.Fatalf("gh status error: %v", err)
	}
	if strings.Contains(out, "Local marker") {
		t.Errorf("should not print a marker section when none exists:\n%s", out)
	}
}

func TestApps_ShowsGhIndicatorForMarkedApps(t *testing.T) {
	dir := setupTestEnv(t)

	// Two apps with stores; only app1 gets a github marker.
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "app1", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set app1: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "app2", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set app2: %v", err)
	}
	if err := config.SetGitHubLink(dir, "app1", config.GitHubLink{
		Repo: "llbbl/lsm", Target: "actions", LastPushed: "2026-05-04T09:00:00Z", LastCount: 1,
	}); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	out, err := runCmd(t, "apps", "--dir", dir)
	if err != nil {
		t.Fatalf("apps error: %v", err)
	}
	if !strings.Contains(out, "app1  → gh:llbbl/lsm") {
		t.Errorf("app1 should show gh indicator:\n%s", out)
	}
	// app2 has no marker: it must appear bare, with no gh suffix.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "app2") && strings.Contains(l, "gh:") {
			t.Errorf("app2 should NOT show a gh indicator: %q", l)
		}
	}
}

func TestApps_UnchangedWithoutAnyMarkers(t *testing.T) {
	dir := setupTestEnv(t)
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "app1", "--env", "dev", "K", "V"); err != nil {
		t.Fatalf("set app1: %v", err)
	}
	if _, err := runCmd(t, "set", "--dir", dir, "--app", "app2", "--env", "prod", "K", "V"); err != nil {
		t.Fatalf("set app2: %v", err)
	}

	out, err := runCmd(t, "apps", "--dir", dir)
	if err != nil {
		t.Fatalf("apps error: %v", err)
	}
	// No markers means plain "app1\napp2\n" output (sorted), unchanged from
	// the original format.
	if out != "app1\napp2\n" {
		t.Errorf("apps output = %q, want plain sorted list with no gh suffix", out)
	}
}
