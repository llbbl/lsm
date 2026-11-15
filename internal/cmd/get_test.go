// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSecretValue(t *testing.T) {
	tests := []struct {
		name string
		val  string
		tty  bool
		want string
	}{
		{"terminal appends newline", "s3cret", true, "s3cret\n"},
		{"pipe stays byte-exact", "s3cret", false, "s3cret"},
		{"terminal empty value", "", true, "\n"},
		{"pipe empty value", "", false, ""},
		{"pipe preserves an existing trailing newline", "s3cret\n", false, "s3cret\n"},
		{"terminal does not collapse an existing trailing newline", "s3cret\n", true, "s3cret\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeSecretValue(&buf, tt.val, tt.tty)
			if got := buf.String(); got != tt.want {
				t.Errorf("writeSecretValue(%q, tty=%v) = %q, want %q", tt.val, tt.tty, got, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	cause := errors.New("fake failure")
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, 0},
		{"ordinary failure", cause, 1},
		{"missing key", &exitCodeError{code: 2, err: cause}, 2},
		{"missing store", &exitCodeError{code: 3, err: cause}, 3},
		{"missing identity", &exitCodeError{code: 4, err: cause}, 4},
		{"unresolved config", &exitCodeError{code: 5, err: cause}, 5},
		{"wrapped", fmt.Errorf("outer: %w", &exitCodeError{code: 2, err: cause}), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode() = %d, want %d", got, tt.want)
			}
			if tt.want > 1 && !errors.Is(tt.err, cause) {
				t.Error("exit code error does not preserve its cause")
			}
			if coded, ok := tt.err.(*exitCodeError); ok && coded.Error() != cause.Error() {
				t.Errorf("error text = %q, want %q", coded.Error(), cause.Error())
			}
		})
	}
}

func TestGet_ExitCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"found", 0},
		{"missing key", 2},
		{"missing store", 3},
		{"missing identity", 4},
		{"unresolved app", 5},
		{"unresolved env", 5},
		{"corrupt store", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Chdir(t.TempDir())
			dir := setupTestEnv(t)
			const sentinel = "fake-program-value"
			if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "TOKEN", sentinel); err != nil {
				t.Fatal(err)
			}
			args := []string{"get", "--dir", dir, "--app", "testapp", "--env", "dev", "TOKEN"}
			wantOut, wantErr := "", ""
			switch tt.name {
			case "found":
				wantOut = sentinel
			case "missing key":
				args[len(args)-1] = "ABSENT"
				wantErr = `key "ABSENT" not found`
			case "missing store":
				if err := os.Remove(filepath.Join(dir, "testapp.dev.age")); err != nil {
					t.Fatal(err)
				}
				wantErr = `key "TOKEN" not found`
			case "missing identity":
				keyPath := filepath.Join(dir, "key.txt")
				if err := os.Remove(keyPath); err != nil {
					t.Fatal(err)
				}
				wantErr = fmt.Sprintf("key file not found: %s (run 'lsm init' to create one)", keyPath)
			case "unresolved app":
				args = []string{"get", "--dir", dir, "--env", "dev", "TOKEN"}
				wantErr = "cannot determine app name: run 'lsm link <app>' in this project, pass --app, or create .lsm.yaml"
			case "unresolved env":
				if err := os.Remove(filepath.Join(dir, "config.yaml")); err != nil {
					t.Fatal(err)
				}
				args = []string{"get", "--dir", dir, "--app", "testapp", "TOKEN"}
				wantErr = fmt.Sprintf("cannot determine environment: use --env flag, create .lsm.yaml, or set env in %s", filepath.Join(dir, "config.yaml"))
			case "corrupt store":
				if err := os.WriteFile(filepath.Join(dir, "testapp.dev.age"), []byte("fake corrupt store"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, err := runCmd(t, args...)
			if got := exitCode(err); got != tt.code {
				t.Errorf("get exit code = %d, want %d (err: %v)", got, tt.code, err)
			}
			if out != wantOut {
				t.Errorf("stdout = %q, want %q", out, wantOut)
			}
			if wantErr != "" && (err == nil || err.Error() != wantErr) {
				t.Errorf("error = %v, want %q", err, wantErr)
			}
		})
	}
}

// The cobra writer in tests is a bytes.Buffer rather than os.Stdout, so
// isStdoutTerminal reports false and `get` must stay byte-exact.
func TestGet_NonTerminalOutputHasNoTrailingNewline(t *testing.T) {
	dir := setupTestEnv(t)

	if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "TOKEN", "abc123"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	out, err := runCmd(t, "get", "--dir", dir, "--app", "testapp", "--env", "dev", "TOKEN")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if out != "abc123" {
		t.Errorf("get output = %q, want %q", out, "abc123")
	}
}
