// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func makeTransformer(t *testing.T) Transformer {
	t.Helper()
	return Transformer{
		Salt:     []byte("test-salt-32-bytes-long-fixed-AA"),
		HostName: "test-host",
	}
}

func expectedHash(salt []byte, v string) string {
	if v == "" {
		return ""
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(v))
	return hex.EncodeToString(mac.Sum(nil))[:12]
}

func TestProject_LocalOnlyEventDropped(t *testing.T) {
	tr := makeTransformer(t)
	e := Event{Event: "set.success", LocalOnly: true}
	got, ok := tr.Project(e)
	if ok || got != nil {
		t.Fatalf("Project(LocalOnly) = (%v, %v); want (nil, false)", got, ok)
	}
}

func TestProject_AuditEventsDropped(t *testing.T) {
	tr := makeTransformer(t)
	for _, name := range []string{
		"audit.verify.failed",
		"audit.suspicious.flagged",
		"audit.anything.else",
	} {
		e := Event{Event: name}
		got, ok := tr.Project(e)
		if ok || got != nil {
			t.Errorf("Project(%q) = (%v, %v); want (nil, false)", name, got, ok)
		}
	}
}

func TestProject_FullEventLabelsAndBody(t *testing.T) {
	tr := makeTransformer(t)
	ts := time.Date(2026, 5, 14, 10, 30, 45, 0, time.UTC)
	e := Event{
		Seq:       42,
		Timestamp: ts,
		Event:     "set.success",
		App:       "webapp",
		Env:       "production",
		Actor: Actor{
			PPID:        1234,
			ParentComm:  "zsh",
			TTY:         "/dev/ttys001",
			CWD:         "/home/user/projects/webapp",
			AgentMarker: "claude-code",
			UID:         501,
		},
		Fields: map[string]any{
			"key": "DATABASE_URL",
		},
	}

	got, ok := tr.Project(e)
	if !ok || got == nil {
		t.Fatalf("Project() = (%v, %v); want non-nil and true", got, ok)
	}

	wantApp := expectedHash(tr.Salt, "webapp")
	wantEnv := expectedHash(tr.Salt, "production")
	wantLabels := map[string]string{
		"event":        "set.success",
		"app":          wantApp,
		"env":          wantEnv,
		"host":         "test-host",
		"tty_present":  "true",
		"agent_marker": "claude-code",
	}
	for k, v := range wantLabels {
		if got.Labels[k] != v {
			t.Errorf("Labels[%q] = %q, want %q", k, got.Labels[k], v)
		}
	}

	// Body must contain these.
	for _, k := range []string{
		"seq", "ts", "event", "actor.parent_comm", "actor.ppid",
		"actor.uid", "actor.agent_marker",
	} {
		if _, ok := got.Body[k]; !ok {
			t.Errorf("Body missing required key %q; got %v", k, got.Body)
		}
	}

	// Body must NOT contain these.
	for _, k := range []string{"actor.tty", "actor.cwd", "hash", "prev"} {
		if _, ok := got.Body[k]; ok {
			t.Errorf("Body should not contain %q; got %v", k, got.Body[k])
		}
	}

	// ts must be RFC3339 (no nanos).
	wantTS := ts.Format(time.RFC3339)
	if got.Body["ts"] != wantTS {
		t.Errorf("Body[ts] = %v, want %q", got.Body["ts"], wantTS)
	}

	// Sensitive field name replaced with _present marker.
	if _, ok := got.Body["fields.key_present"]; !ok {
		t.Errorf("Body should contain fields.key_present; got %v", got.Body)
	}
	if v, ok := got.Body["fields.key"]; ok {
		t.Errorf("Body should NOT contain fields.key (sensitive); got %v", v)
	}
}

func TestProject_TTYPresentLabel(t *testing.T) {
	tr := makeTransformer(t)
	cases := []struct {
		tty  string
		want string
	}{
		{"/dev/ttys001", "true"},
		{"", "false"},
	}
	for _, c := range cases {
		e := Event{Event: "x", Actor: Actor{TTY: c.tty}}
		got, ok := tr.Project(e)
		if !ok {
			t.Fatalf("Project unexpectedly dropped event for tty=%q", c.tty)
		}
		if got.Labels["tty_present"] != c.want {
			t.Errorf("tty=%q: tty_present = %q, want %q", c.tty, got.Labels["tty_present"], c.want)
		}
	}
}

func TestProject_AgentMarkerNoneDefault(t *testing.T) {
	tr := makeTransformer(t)
	e := Event{Event: "x", Actor: Actor{AgentMarker: ""}}
	got, _ := tr.Project(e)
	if got.Labels["agent_marker"] != "none" {
		t.Errorf("agent_marker = %q, want %q", got.Labels["agent_marker"], "none")
	}
}

func TestProject_EmptyAppEnvNotHashed(t *testing.T) {
	tr := makeTransformer(t)
	e := Event{Event: "x", App: "", Env: ""}
	got, _ := tr.Project(e)
	if got.Labels["app"] != "" {
		t.Errorf("empty app should remain empty, got %q", got.Labels["app"])
	}
	if got.Labels["env"] != "" {
		t.Errorf("empty env should remain empty, got %q", got.Labels["env"])
	}
}

func TestProject_HashLengthIs12Hex(t *testing.T) {
	tr := makeTransformer(t)
	e := Event{Event: "x", App: "foo", Env: "bar"}
	got, _ := tr.Project(e)
	if len(got.Labels["app"]) != 12 {
		t.Errorf("hashed app length = %d, want 12", len(got.Labels["app"]))
	}
	if len(got.Labels["env"]) != 12 {
		t.Errorf("hashed env length = %d, want 12", len(got.Labels["env"]))
	}
}

func TestProject_MultipleFieldsAllPresent(t *testing.T) {
	tr := makeTransformer(t)
	e := Event{
		Event: "set.success",
		Fields: map[string]any{
			"key":    "DATABASE_URL",
			"source": "cli",
			"reason": "manual",
		},
	}
	got, _ := tr.Project(e)
	for _, k := range []string{"key", "source", "reason"} {
		present := "fields." + k + "_present"
		if v, ok := got.Body[present]; !ok || v != true {
			t.Errorf("expected Body[%q] = true; got %v (ok=%v)", present, v, ok)
		}
		raw := "fields." + k
		if v, ok := got.Body[raw]; ok {
			t.Errorf("Body should not contain raw value %q = %v", raw, v)
		}
	}
}
