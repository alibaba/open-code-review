// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPathCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(t.TempDir(), "ocr data")
	for _, tc := range []struct {
		name string
		env  string
		want string
	}{
		{"default", "", filepath.Join(home, ".opencodereview", "config.json")},
		{"custom", custom, filepath.Join(custom, "config.json")},
		{"relative", "ocr-state", filepath.Join(cwd, "ocr-state", "config.json")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OCR_HOME", tc.env)
			for _, asJSON := range []bool{false, true} {
				cmd := newConfigPathCommand()
				var out bytes.Buffer
				cmd.SetOut(&out)
				args := []string{}
				if asJSON {
					args = append(args, "--json")
				}
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				got := strings.TrimSpace(out.String())
				if asJSON {
					var result struct {
						Path string `json:"path"`
					}
					if err := json.Unmarshal(out.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					got = result.Path
				}
				if got != tc.want {
					t.Fatalf("config path = %q, want %q", got, tc.want)
				}
			}
			if _, err := os.Stat(filepath.Dir(tc.want)); !os.IsNotExist(err) {
				t.Fatalf("path lookup created storage: %v", err)
			}
		})
	}
}

func TestConfigPathCommandWithoutUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OCR_HOME", "")
	cmd := newConfigPathCommand()
	cmd.SetArgs([]string{"--json"})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	if err := cmd.Execute(); err == nil {
		t.Fatal("path lookup succeeded without a storage root")
	}
	storage := t.TempDir()
	t.Setenv("OCR_HOME", storage)
	cmd = newConfigPathCommand()
	cmd.SetArgs([]string{"--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "config.json") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
