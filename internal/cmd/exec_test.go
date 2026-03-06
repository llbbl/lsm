// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package cmd

import (
	"strings"
	"testing"
)

func TestExecCmd_MissingSeparator(t *testing.T) {
	dir := setupTestEnv(t)

	// No "--" separator: ArgsLenAtDash returns -1
	_, err := runCmd(t, "exec", "--dir", dir, "--app", "testapp", "--env", "dev", "echo", "hello")
	if err == nil {
		t.Fatal("expected error when -- separator is missing")
	}
}

func TestExecCmd_CommandNotFound(t *testing.T) {
	dir := setupTestEnv(t)

	// Set a secret so the store loads
	_, err := runCmd(t, "set", "--dir", dir, "--app", "testapp", "--env", "dev", "KEY", "val")
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	// Pass "--" through cobra so ArgsLenAtDash is set correctly
	_, err = runCmd(t, "exec", "--dir", dir, "--app", "testapp", "--env", "dev", "--", "nonexistent_command_xyz_12345")
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("error = %q, want it to contain 'command not found'", err.Error())
	}
}

func TestBuildEnvWithSecrets(t *testing.T) {
	t.Run("overrides existing var", func(t *testing.T) {
		environ := []string{"DB_HOST=old", "PATH=/usr/bin"}
		secrets := map[string]string{"DB_HOST": "new"}

		env := buildEnvWithSecrets(environ, secrets)

		lookup := envToMap(env)
		if lookup["DB_HOST"] != "new" {
			t.Errorf("DB_HOST = %q, want %q", lookup["DB_HOST"], "new")
		}
		if lookup["PATH"] != "/usr/bin" {
			t.Errorf("PATH = %q, want %q", lookup["PATH"], "/usr/bin")
		}
	})

	t.Run("adds new secret", func(t *testing.T) {
		environ := []string{"PATH=/usr/bin"}
		secrets := map[string]string{"API_KEY": "secret123"}

		env := buildEnvWithSecrets(environ, secrets)

		lookup := envToMap(env)
		if lookup["API_KEY"] != "secret123" {
			t.Errorf("API_KEY = %q, want %q", lookup["API_KEY"], "secret123")
		}
		if lookup["PATH"] != "/usr/bin" {
			t.Errorf("PATH = %q, want %q", lookup["PATH"], "/usr/bin")
		}
	})

	t.Run("passthrough when no secrets", func(t *testing.T) {
		environ := []string{"A=1", "B=2"}
		secrets := map[string]string{}

		env := buildEnvWithSecrets(environ, secrets)

		if len(env) != 2 {
			t.Fatalf("got %d vars, want 2", len(env))
		}
		lookup := envToMap(env)
		if lookup["A"] != "1" || lookup["B"] != "2" {
			t.Errorf("env = %v, want passthrough", env)
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		environ := []string{"KEY=old"}
		secrets := map[string]string{"KEY": "new"}

		env := buildEnvWithSecrets(environ, secrets)

		count := 0
		for _, e := range env {
			if strings.HasPrefix(e, "KEY=") {
				count++
			}
		}
		if count != 1 {
			t.Errorf("KEY appears %d times, want 1", count)
		}
	})

	t.Run("preserves value with equals sign", func(t *testing.T) {
		environ := []string{"CONN=host=localhost;port=5432"}
		secrets := map[string]string{"TOKEN": "abc=def"}

		env := buildEnvWithSecrets(environ, secrets)

		lookup := envToMap(env)
		if lookup["CONN"] != "host=localhost;port=5432" {
			t.Errorf("CONN = %q, want original value preserved", lookup["CONN"])
		}
		if lookup["TOKEN"] != "abc=def" {
			t.Errorf("TOKEN = %q, want %q", lookup["TOKEN"], "abc=def")
		}
	})
}

// envToMap converts a []string of "K=V" pairs to a map.
func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}
