// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDlogWiring_DebugToFile confirms PersistentPreRunE reads the log
// block from <dir>/config.yaml (the global config file lsm looks at)
// and that the initial Debug call lands in the destination file.
func TestDlogWiring_DebugToFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "lsm.log")

	cfg := fmt.Sprintf("env: dev\nlog:\n  level: debug\n  dest: file:%s\n  format: text\n", logPath)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// `apps --dir <tempdir>` is a safe no-op subcommand that exercises
	// the persistent pre/post-run wiring without needing any setup.
	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "command starting") {
		t.Errorf("log missing 'command starting' line; got: %q", s)
	}
	if !strings.Contains(s, "apps") {
		t.Errorf("log missing command name 'apps'; got: %q", s)
	}
}

// TestDlogWiring_DefaultOff confirms that without a `log:` block, the
// pre-run still succeeds (logger is Discard) and no file is written.
func TestDlogWiring_DefaultOff(t *testing.T) {
	dir := t.TempDir()
	// Config exists but has no log block — level defaults to off.
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("env: dev\n"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}
}

// TestDlogWiring_NoConfigFile confirms commands work when no config
// file exists yet (the chicken-and-egg case for `lsm init` itself).
func TestDlogWiring_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	// Intentionally no config.yaml written.

	if _, err := runCmd(t, "apps", "--dir", dir); err != nil {
		t.Fatalf("apps error: %v", err)
	}
}

func TestDataCommandsAnnotations(t *testing.T) {
	for _, name := range []string{"get", "list", "dump", "exec"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := NewRootCmd().Find([]string{name})
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Annotations["stdout"] != "data" {
				t.Error("command stdout is not annotated as data")
			}
		})
	}
}

func TestDlogWiring_DataStdout(t *testing.T) {
	for _, name := range []string{"get", "list"} {
		for _, dest := range []string{"stdout", " StDoUt "} {
			t.Run(name+"/"+dest, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Chdir(t.TempDir())
				dir := setupTestEnv(t)
				const sentinel = "fake-stdout-value"
				if _, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "TOKEN", sentinel); err != nil {
					t.Fatal(err)
				}
				cfg := fmt.Sprintf("env: dev\nlog:\n  level: debug\n  dest: %q\n", dest)
				if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600); err != nil {
					t.Fatal(err)
				}
				stdout, err := os.CreateTemp(t.TempDir(), "stdout")
				if err != nil {
					t.Fatal(err)
				}
				defer stdout.Close()
				stderr, err := os.CreateTemp(t.TempDir(), "stderr")
				if err != nil {
					t.Fatal(err)
				}
				defer stderr.Close()
				origOut, origErr := os.Stdout, os.Stderr
				os.Stdout, os.Stderr = stdout, stderr
				defer func() { os.Stdout, os.Stderr = origOut, origErr }()

				// Capture the process streams because dlog writes directly to them.
				root := NewRootCmd()
				args := []string{name, "--dir", dir, "--app", "testapp", "--env", "dev"}
				want := "TOKEN\n"
				if name == "get" {
					args = append(args, "TOKEN")
					want = sentinel
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				out, err := os.ReadFile(stdout.Name())
				if err != nil {
					t.Fatal(err)
				}
				if string(out) != want {
					t.Errorf("stdout = %q, want %q", out, want)
				}
				logs, err := os.ReadFile(stderr.Name())
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(logs), "command starting") {
					t.Errorf("stderr missing debug log: %q", logs)
				}
			})
		}
	}
}
