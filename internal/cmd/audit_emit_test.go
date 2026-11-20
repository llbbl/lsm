// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/llbbl/lsm/internal/audit"
)

const auditSentinel = "synthetic-audit-secret-value"

type recordingAuditSink struct {
	events []audit.Event
	closed int
	err    error
}

func (s *recordingAuditSink) Write(_ context.Context, event audit.Event) error {
	s.events = append(s.events, event)
	return s.err
}

func (s *recordingAuditSink) Close() error {
	s.closed++
	return nil
}

func stubAuditSink(t *testing.T, dir string) *recordingAuditSink {
	t.Helper()
	sink := new(recordingAuditSink)
	original := openAuditSink
	openAuditSink = func(path string) (audit.Sink, error) {
		if want := filepath.Join(dir, "audit.jsonl"); path != want {
			t.Fatalf("audit path = %q, want %q", path, want)
		}
		return sink, nil
	}
	t.Cleanup(func() { openAuditSink = original })
	return sink
}

func TestSecretCommandAuditEvents(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		fields map[string]any
	}{
		{"get", []string{"get", "BASE"}, map[string]any{"key": "BASE"}},
		{"set", []string{"set", "NEW", auditSentinel}, map[string]any{"key": "NEW"}},
		{"delete", []string{"delete", "BASE"}, map[string]any{"key": "BASE"}},
		{"dump", []string{"dump", "--output", "export.env"}, map[string]any{
			"path": "export.env", "names": []string{"BASE"}, "count": 1,
		}},
		{"exec", []string{"exec", "--", "/bin/sh", auditSentinel}, map[string]any{
			"command": "sh", "count": 1,
		}},
		{"edit", []string{"edit"}, map[string]any{"names": []string{"BASE", "EDITED"}, "count": 2}},
		{"import", []string{"import", "./input.env"}, map[string]any{"names": []string{"IMPORTED"}, "count": 1}},
		{"clean", []string{"clean", "--force"}, map[string]any{"names": []string{"BASE"}, "count": 1, "file_count": 2}},
		{"gh_push", []string{"gh", "push", "--repo", "example/repo", "--gh-env", "testing", "--prune", "--force"}, map[string]any{
			"repo": "example/repo", "target": "env:testing", "names": []string{"BASE"}, "count": 1,
			"pruned": []string{"STALE"}, "pruned_count": 1,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupLinkedProject(t, "testapp", map[string]string{"BASE": auditSentinel})
			sink := stubAuditSink(t, dir)
			writeFixture := func(path, content string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			switch tt.name {
			case "exec":
				original := execProcess
				t.Cleanup(func() { execProcess = original })
				execProcess = func(_ string, _, _ []string) error {
					if len(sink.events) != 1 || sink.closed != 1 {
						t.Fatal("audit event must be written and the sink closed before exec")
					}
					return nil
				}
			case "edit":
				path := filepath.Join(t.TempDir(), "editor.sh")
				writeFixture(path, "#!/bin/sh\nprintf '%s\\n' 'EDITED="+auditSentinel+"' > \"$1\"\n", 0700)
				t.Setenv("EDITOR", path)
			case "import":
				writeFixture("input.env", "IMPORTED="+auditSentinel+"\nIMPORTED="+auditSentinel+"\n", 0600)
			case "clean":
				writeFixture(".env", "BASE="+auditSentinel+"\n", 0600)
				writeFixture(".env.local", "BASE="+auditSentinel+"\n", 0600)
				writeFixture(".env.unsafe", "MISSING="+auditSentinel+"\n", 0600)
			case "gh_push":
				fakeGh(t, map[string]string{"secret list": `[{"name":"STALE"}]`})
			}
			if _, err := runCmd(t, append([]string{"--dir", dir}, tt.args...)...); err != nil {
				t.Fatal(err)
			}
			if len(sink.events) != 1 || sink.closed != 1 {
				t.Fatalf("got %d audit events and %d closes, want one of each", len(sink.events), sink.closed)
			}
			event := sink.events[0]
			body, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(body, []byte(auditSentinel)) {
				t.Fatal("found a synthetic secret value in the audit event")
			}
			if event.Event != tt.name || event.App != "testapp" || event.Env != "dev" {
				t.Errorf("unexpected event, app, or environment: %q %q %q", event.Event, event.App, event.Env)
			}
			if event.Actor.PPID != os.Getppid() || event.Actor.UID != os.Getuid() || event.Actor.CWD == "" {
				t.Error("audit actor metadata was not captured")
			}
			if !reflect.DeepEqual(event.Fields, tt.fields) {
				t.Error("audit fields differ from expected metadata")
			}
		})
	}
}

func TestSecretCommandAuditAfterEditorFailure(t *testing.T) {
	dir := setupLinkedProject(t, "testapp", map[string]string{"BASE": auditSentinel})
	sink := stubAuditSink(t, dir)
	t.Setenv("EDITOR", "/usr/bin/false")
	if _, err := runCmd(t, "--dir", dir, "edit"); err == nil {
		t.Fatal("expected the editor to fail")
	}
	if len(sink.events) != 1 || sink.events[0].Event != "edit" ||
		!reflect.DeepEqual(sink.events[0].Fields["names"], []string{"BASE"}) {
		t.Fatal("expected one audit event for the keys exposed to the failed editor")
	}
}

func TestAuditWarningPreservesGetOutput(t *testing.T) {
	dir := setupLinkedProject(t, "testapp", map[string]string{"BASE": auditSentinel})
	sink := stubAuditSink(t, dir)
	sink.err = errors.New("synthetic audit write failure")
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--dir", dir, "get", "BASE"})
	if err := root.Execute(); err != nil || stdout.String() != auditSentinel {
		t.Fatal("get did not preserve successful output when audit failed")
	}
	if !strings.Contains(stderr.String(), "warning: audit: synthetic audit write failure") || sink.closed != 1 {
		t.Fatal("expected an audit warning on stderr and a closed sink")
	}
}
