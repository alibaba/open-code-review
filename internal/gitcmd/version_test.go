// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    GitVersion
		wantErr bool
	}{
		{name: "plain", input: "git version 2.41.0\n", want: GitVersion{2, 41, 0}},
		{name: "apple suffix", input: "git version 2.39.2 (Apple Git-145)\n", want: GitVersion{2, 39, 2}},
		{name: "windows suffix", input: "git version 2.41.0.windows.1\n", want: GitVersion{2, 41, 0}},
		{name: "no trailing newline", input: "git version 2.45.2", want: GitVersion{2, 45, 2}},
		{name: "no prefix", input: "2.41.0", want: GitVersion{2, 41, 0}},
		{name: "minor 10", input: "git version 2.10.0\n", want: GitVersion{2, 10, 0}},
		{name: "newer major", input: "git version 3.0.0\n", want: GitVersion{3, 0, 0}},
		{name: "empty", input: "", wantErr: true},
		{name: "garbage", input: "banana", wantErr: true},
		{name: "two component", input: "git version 2.41\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGitVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitVersion_AtLeast(t *testing.T) {
	min := GitVersion{2, 41, 0}
	tests := []struct {
		v    GitVersion
		want bool
	}{
		{GitVersion{2, 41, 0}, true},
		{GitVersion{2, 41, 1}, true},
		{GitVersion{2, 42, 0}, true},
		{GitVersion{3, 0, 0}, true},
		{GitVersion{2, 40, 0}, false},
		{GitVersion{2, 39, 99}, false},
		{GitVersion{1, 99, 99}, false},
		{GitVersion{2, 40, 1}, false},
		{GitVersion{2, 41, 0}, true}, // duplicate to be explicit
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s>=%s=%v", tt.v, min, tt.want), func(t *testing.T) {
			if got := tt.v.AtLeast(min); got != tt.want {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", tt.v, min, got, tt.want)
			}
		})
	}
}

func TestVersionWarning(t *testing.T) {
	tests := []struct {
		v       GitVersion
		wantOk  bool
		contain []string
	}{
		{GitVersion{2, 30, 0}, true, []string{"warning:", "2.30.0", "2.41.0", "upgrading git"}},
		{GitVersion{2, 41, 0}, false, nil},
		{GitVersion{2, 41, 1}, false, nil},
		{GitVersion{3, 0, 0}, false, nil},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("v=%s", tt.v), func(t *testing.T) {
			msg, ok := versionWarning(tt.v)
			if ok != tt.wantOk {
				t.Errorf("versionWarning(%s) ok=%v, want %v", tt.v, ok, tt.wantOk)
			}
			if tt.wantOk {
				for _, want := range tt.contain {
					if !strings.Contains(msg, want) {
						t.Errorf("message %q does not contain %q", msg, want)
					}
				}
			} else if msg != "" {
				t.Errorf("expected empty message, got %q", msg)
			}
		})
	}
}

func TestCheckGitVersion(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		var buf bytes.Buffer
		getVersion := func() ([]byte, error) {
			return []byte("git version 2.45.2\n"), nil
		}
		ver, err := checkGitVersion(&buf, getVersion)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ver != "2.45.2" {
			t.Errorf("version = %q, want %q", ver, "2.45.2")
		}
		if buf.Len() != 0 {
			t.Errorf("expected no warning, got %q", buf.String())
		}
	})

	t.Run("old git warns", func(t *testing.T) {
		var buf bytes.Buffer
		getVersion := func() ([]byte, error) {
			return []byte("git version 2.30.0\n"), nil
		}
		ver, err := checkGitVersion(&buf, getVersion)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ver != "2.30.0" {
			t.Errorf("version = %q, want %q", ver, "2.30.0")
		}
		msg := buf.String()
		if !strings.Contains(msg, "warning:") || !strings.Contains(msg, "2.41.0") {
			t.Errorf("expected warning with version, got %q", msg)
		}
	})

	t.Run("getVersion error", func(t *testing.T) {
		var buf bytes.Buffer
		getVersion := func() ([]byte, error) {
			return nil, errors.New("git not found")
		}
		_, err := checkGitVersion(&buf, getVersion)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if buf.Len() != 0 {
			t.Errorf("expected no warning, got %q", buf.String())
		}
	})

	t.Run("garbage output", func(t *testing.T) {
		var buf bytes.Buffer
		getVersion := func() ([]byte, error) {
			return []byte("banana\n"), nil
		}
		_, err := checkGitVersion(&buf, getVersion)
		if err == nil {
			t.Fatal("expected parse error, got nil")
		}
		if buf.Len() != 0 {
			t.Errorf("expected no warning, got %q", buf.String())
		}
	})
}

func TestCheckGitVersion_Real(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	ver, err := CheckGitVersion()
	if err != nil {
		t.Fatalf("CheckGitVersion() error: %v", err)
	}
	if ver == "" {
		t.Error("expected non-empty version string")
	}
	if !strings.Contains(ver, ".") {
		t.Errorf("version %q does not look like a semver", ver)
	}
}
