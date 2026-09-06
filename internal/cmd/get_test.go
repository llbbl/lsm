// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"bytes"
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
