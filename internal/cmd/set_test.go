// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestTrimTrailingNewline(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lf stripped", "tok\n", "tok"},
		{"crlf stripped", "tok\r\n", "tok"},
		{"no newline unchanged", "tok", "tok"},
		{"only one newline stripped", "tok\n\n", "tok\n"},
		{"trailing space preserved", "tok ", "tok "},
		{"interior newline preserved", "a\nb\n", "a\nb"},
		{"empty stays empty", "", ""},
		{"lone newline becomes empty", "\n", ""},
		{"lone crlf becomes empty", "\r\n", ""},
		{"bare cr not stripped", "tok\r", "tok\r"},
		{"trailing tab preserved", "tok\t", "tok\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimTrailingNewline(tt.in); got != tt.want {
				t.Errorf("trimTrailingNewline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSetTrailingCount(t *testing.T) {
	tests := []struct {
		name  string
		nArgs int
		want  int
	}{
		// VALUE is optional only when KEY is the single positional argument.
		{"key only prompts", 1, 1},
		{"key value", 2, 2},
		{"app key value", 3, 2},
		{"app env key value", 4, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setTrailingCount(tt.nArgs); got != tt.want {
				t.Errorf("setTrailingCount(%d) = %d, want %d", tt.nArgs, got, tt.want)
			}
		})
	}
}

func TestAcquireSecret_PipedTrimsNewline(t *testing.T) {
	got, err := acquireSecret("KEY", false, strings.NewReader("tok\n"), io.Discard)
	if err != nil {
		t.Fatalf("acquireSecret() error: %v", err)
	}
	if got != "tok" {
		t.Errorf("acquireSecret() = %q, want %q", got, "tok")
	}
}

func TestAcquireSecret_PipedPreservesInterior(t *testing.T) {
	got, err := acquireSecret("KEY", false, strings.NewReader("a\nb\n"), io.Discard)
	if err != nil {
		t.Fatalf("acquireSecret() error: %v", err)
	}
	if got != "a\nb" {
		t.Errorf("acquireSecret() = %q, want %q", got, "a\nb")
	}
}

func TestAcquireSecret_TTYEmptyRejected(t *testing.T) {
	orig := readSecretFromTerminal
	readSecretFromTerminal = func(key string, prompt io.Writer) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { readSecretFromTerminal = orig })

	_, err := acquireSecret("KEY", true, strings.NewReader(""), io.Discard)
	if err == nil {
		t.Fatal("expected error for empty interactive input")
	}
	if !strings.Contains(err.Error(), "no value entered") {
		t.Errorf("error = %v, want it to mention 'no value entered'", err)
	}
}

func TestAcquireSecret_TTYReturnsTypedValue(t *testing.T) {
	orig := readSecretFromTerminal
	readSecretFromTerminal = func(key string, prompt io.Writer) (string, error) {
		return "typed-secret", nil
	}
	t.Cleanup(func() { readSecretFromTerminal = orig })

	got, err := acquireSecret("KEY", true, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatalf("acquireSecret() error: %v", err)
	}
	if got != "typed-secret" {
		t.Errorf("acquireSecret() = %q, want %q", got, "typed-secret")
	}
}

// TestSetStdinNoValue_BehavesLikeDash verifies that "set KEY" with piped
// (non-terminal) stdin reads and stores the value like "set KEY -", with the
// trailing newline stripped.
func TestSetStdinNoValue_BehavesLikeDash(t *testing.T) {
	dir := setupTestEnv(t)
	forceNonInteractive(t)

	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("piped_no_value\n")
		_ = w.Close()
	}()

	// No VALUE argument, non-terminal stdin.
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "NOVAL_KEY")
	if err != nil {
		t.Fatalf("set with piped stdin error: %v", err)
	}

	os.Stdin = origStdin

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "NOVAL_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "piped_no_value" {
		t.Errorf("stored value = %q, want %q (trailing newline should be stripped)", out, "piped_no_value")
	}
}

// TestSetStdinDash_TrimsNewline verifies the explicit "set KEY -" form also
// strips a single trailing newline.
func TestSetStdinDash_TrimsNewline(t *testing.T) {
	dir := setupTestEnv(t)

	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("dash_value\n")
		_ = w.Close()
	}()

	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "DASH_KEY", "-")
	if err != nil {
		t.Fatalf("set from stdin error: %v", err)
	}

	os.Stdin = origStdin

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "DASH_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "dash_value" {
		t.Errorf("stored value = %q, want %q", out, "dash_value")
	}
}

// TestSetPromptWithFlagAppEnv verifies that the no-value form for a specific
// app/env is reached via the --app/--env flags (KEY is the single positional),
// not via positional "app env KEY". Value comes from piped stdin.
func TestSetPromptWithFlagAppEnv(t *testing.T) {
	dir := setupTestEnv(t)
	forceNonInteractive(t)

	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.WriteString("flag_value\n")
		_ = w.Close()
	}()

	// KEY is the only positional; app/env come from flags.
	_, err := runCmd(t, "set", "--dir", dir, "--app", "myapp", "--env", "prod", "FLAG_KEY")
	if err != nil {
		t.Fatalf("set with flag app/env error: %v", err)
	}

	os.Stdin = origStdin

	out, err := runCmd(t, "get", "--dir", dir, "--app", "myapp", "--env", "prod", "FLAG_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "flag_value" {
		t.Errorf("stored value = %q, want %q", out, "flag_value")
	}
}

// TestSetPositionalApp_KeyValue is the regression guard for the 3-bare-positional
// form "set app KEY VALUE": app resolves positionally and KEY/VALUE are stored
// correctly. It must NOT silently mis-store (app=KEY, key=VALUE, ...).
func TestSetPositionalApp_KeyValue(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "set", "--dir", dir, "--env", "dev", "myapp", "MYKEY", "myvalue")
	if err != nil {
		t.Fatalf("set app key value error: %v", err)
	}

	// Value must land under app=myapp, key=MYKEY.
	out, err := runCmd(t, "get", "--dir", dir, "--env", "dev", "myapp", "MYKEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "myvalue" {
		t.Errorf("stored value = %q, want %q (mis-store regression)", out, "myvalue")
	}

	// The mis-store interpretation would have created key "myvalue"; it must not exist.
	if _, err := runCmd(t, "get", "--dir", dir, "--env", "dev", "myapp", "myvalue"); err == nil {
		t.Error("unexpected key \"myvalue\" present — arguments were mis-stored")
	}
}

// TestSetTooManyArgs verifies that surplus positionals are rejected rather than
// silently dropped. "set a b c d e" leaves remaining=[c,d,e] after app/env are
// peeled, which must error and store nothing.
func TestSetTooManyArgs(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "set", "--dir", dir, "a", "b", "c", "d", "e")
	if err == nil {
		t.Fatal("expected error for too many arguments")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Errorf("error = %v, want it to mention 'too many arguments'", err)
	}

	// Nothing should have been stored under the mis-parsed key.
	if _, err := runCmd(t, "get", "--dir", dir, "a", "b", "c"); err == nil {
		t.Error("expected key \"c\" to be absent after rejected set")
	}
}

// TestSetPositionalAppEnv_KeyValue verifies the 4-positional form
// "set app env KEY VALUE" resolves app+env positionally and stores KEY/VALUE.
func TestSetPositionalAppEnv_KeyValue(t *testing.T) {
	dir := setupTestEnv(t)

	_, err := runCmd(t, "set", "--dir", dir, "myapp", "prod", "DB_URL", "postgres://localhost")
	if err != nil {
		t.Fatalf("set app env key value error: %v", err)
	}

	out, err := runCmd(t, "get", "--dir", dir, "myapp", "prod", "DB_URL")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "postgres://localhost" {
		t.Errorf("stored value = %q, want %q", out, "postgres://localhost")
	}
}

// TestSetInteractivePrompt_StoresTypedValue drives the full set command down
// the TTY path by faking both the terminal detection and the password reader.
func TestSetInteractivePrompt_StoresTypedValue(t *testing.T) {
	dir := setupTestEnv(t)

	origTerm := isTerminal
	isTerminal = func() bool { return true }
	t.Cleanup(func() { isTerminal = origTerm })

	origReader := readSecretFromTerminal
	readSecretFromTerminal = func(key string, prompt io.Writer) (string, error) {
		return "interactive_secret", nil
	}
	t.Cleanup(func() { readSecretFromTerminal = origReader })

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "PROMPT_KEY"); err != nil {
		t.Fatalf("set interactive error: %v", err)
	}

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "PROMPT_KEY")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "interactive_secret" {
		t.Errorf("stored value = %q, want %q", out, "interactive_secret")
	}
}

// TestSetInteractivePrompt_EmptyRejected verifies the command errors and stores
// nothing when the interactive prompt returns an empty value.
func TestSetInteractivePrompt_EmptyRejected(t *testing.T) {
	dir := setupTestEnv(t)

	origTerm := isTerminal
	isTerminal = func() bool { return true }
	t.Cleanup(func() { isTerminal = origTerm })

	origReader := readSecretFromTerminal
	readSecretFromTerminal = func(key string, prompt io.Writer) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { readSecretFromTerminal = origReader })

	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "EMPTY_KEY")
	if err == nil {
		t.Fatal("expected error for empty interactive input")
	}

	// Nothing should have been stored.
	if _, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "EMPTY_KEY"); err == nil {
		t.Error("expected EMPTY_KEY to be absent after rejected prompt")
	}
}
