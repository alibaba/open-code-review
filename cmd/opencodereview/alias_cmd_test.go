// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidAliasName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"cp", true},
		{"rq", true},
		{"review-agents", true},
		{"review_agents", true},
		{"abc123", true},
		{"", false},
		{"-cp", false},
		{"_cp", false},
		{"cp name", false},
		{"cp=config", false},
		{"cp!", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validAliasName(tt.name); got != tt.want {
				t.Errorf("validAliasName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestValidateAliasNameRejectsReservedCommands(t *testing.T) {
	for _, name := range []string{"review", "r", "config", "alias", "help", "completion"} {
		t.Run(name, func(t *testing.T) {
			if err := validateAliasName(name); err == nil {
				t.Fatalf("expected reserved alias name %q to be rejected", name)
			}
		})
	}
}

func TestParseAliasTarget(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
		wantErr bool
	}{
		{
			name:    "plain command",
			command: "config provider",
			want:    []string{"config", "provider"},
		},
		{
			name:    "flags and double-quoted value",
			command: `review --audience agent --background "Focus on auth"`,
			want:    []string{"review", "--audience", "agent", "--background", "Focus on auth"},
		},
		{
			name:    "single-quoted value",
			command: `session show 'session with spaces'`,
			want:    []string{"session", "show", "session with spaces"},
		},
		{
			name:    "escaped double quote inside double quotes",
			command: `review --background "say \"hello\""`,
			want:    []string{"review", "--background", `say "hello"`},
		},
		{
			name:    "trailing backslash",
			command: `review \`,
			wantErr: true,
		},
		{
			name:    "unclosed quote",
			command: `review --background "oops`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAliasTarget(tt.command)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.command)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAliasTarget(%q): %v", tt.command, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAliasTarget(%q) = %#v, want %#v", tt.command, got, tt.want)
			}
		})
	}
}

func TestValidateAliasTarget(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{name: "valid nested command", command: "config provider"},
		{name: "valid command with flags", command: `review --audience agent --background "Focus on auth"`},
		{name: "built-in alias target", command: "r --format json"},
		{name: "unknown command", command: "bogus command", wantErr: true},
		{name: "leading ocr", command: "ocr config provider", wantErr: true},
		{name: "starts with flag", command: "--help", wantErr: true},
		{name: "empty quoted argument", command: "config ''", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAliasTarget(tt.command)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.command)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateAliasTarget(%q): %v", tt.command, err)
			}
		})
	}
}

func TestExpandAliases(t *testing.T) {
	aliases := map[string]string{
		"cp": "config provider",
		"rq": `review --audience agent --background "Focus on auth"`,
	}

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "non-alias passes through",
			args: []string{"version"},
			want: []string{"version"},
		},
		{
			name: "alias expands",
			args: []string{"cp"},
			want: []string{"config", "provider"},
		},
		{
			name: "alias appends extra args",
			args: []string{"cp", "extra"},
			want: []string{"config", "provider", "extra"},
		},
		{
			name: "alias with quoted target",
			args: []string{"rq", "--format", "json"},
			want: []string{"review", "--audience", "agent", "--background", "Focus on auth", "--format", "json"},
		},
		{
			name:    "malformed target",
			args:    []string{"bad"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "malformed target" {
				got, err := expandAliases(tt.args, map[string]string{"bad": `review --background "oops`})
				if err == nil {
					t.Fatal("expected error for malformed alias target")
				}
				_ = got
				return
			}
			got, err := expandAliases(tt.args, aliases)
			if err != nil {
				t.Fatalf("expandAliases(%v): %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandAliases(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestAliasCommandsPersistListAndRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	out := captureStdout(t, func() {
		if err := runAliasSet("cp", "config provider"); err != nil {
			t.Fatalf("runAliasSet: %v", err)
		}
	})
	if !strings.Contains(out, `Alias "cp" set to "config provider"`) {
		t.Errorf("stdout = %q", out)
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Aliases["cp"] != "config provider" {
		t.Errorf("saved alias = %q", cfg.Aliases["cp"])
	}

	out = captureStdout(t, func() {
		if err := runAliasList(); err != nil {
			t.Fatalf("runAliasList: %v", err)
		}
	})
	if !strings.Contains(out, "cp = config provider") {
		t.Errorf("list output = %q", out)
	}

	out = captureStdout(t, func() {
		if err := runAliasRm("cp"); err != nil {
			t.Fatalf("runAliasRm: %v", err)
		}
	})
	if !strings.Contains(out, `Removed alias "cp"`) {
		t.Errorf("stdout = %q", out)
	}

	cfg, err = loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if _, ok := cfg.Aliases["cp"]; ok {
		t.Fatal("alias was not removed")
	}
	if err := runAliasRm("cp"); err == nil {
		t.Fatal("expected error when removing missing alias")
	}
}

func TestExpandConfiguredAliases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := runAliasSet("cp", "config provider"); err != nil {
		t.Fatalf("runAliasSet: %v", err)
	}

	got, err := expandConfiguredAliases([]string{"cp", "extra"})
	if err != nil {
		t.Fatalf("expandConfiguredAliases: %v", err)
	}
	want := []string{"config", "provider", "extra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandConfiguredAliases = %#v, want %#v", got, want)
	}
}

func TestExpandConfiguredAliasesIgnoresMalformedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".opencodereview", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := expandConfiguredAliases([]string{"cp"})
	if err != nil {
		t.Fatalf("expandConfiguredAliases: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"cp"}) {
		t.Errorf("expandConfiguredAliases = %#v, want unchanged args", got)
	}
}
