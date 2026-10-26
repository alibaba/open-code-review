// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
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

func TestVersionTooOldError_Error(t *testing.T) {
	err := &VersionTooOldError{Current: GitVersion{2, 30, 0}, Minimum: GitVersion{2, 41, 0}}
	msg := err.Error()
	for _, want := range []string{"2.30.0", "2.41.0", "upgrading git"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestCheckGitVersion(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		getVersion := func() ([]byte, error) {
			return []byte("git version 2.45.2\n"), nil
		}
		if err := checkGitVersion(getVersion); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("old git returns typed error", func(t *testing.T) {
		getVersion := func() ([]byte, error) {
			return []byte("git version 2.30.0\n"), nil
		}
		err := checkGitVersion(getVersion)
		var tooOld *VersionTooOldError
		if !errors.As(err, &tooOld) {
			t.Fatalf("expected *VersionTooOldError, got %v", err)
		}
		if tooOld.Current != (GitVersion{2, 30, 0}) {
			t.Errorf("Current = %v, want 2.30.0", tooOld.Current)
		}
		if tooOld.Minimum != gitVersionMin {
			t.Errorf("Minimum = %v, want %v", tooOld.Minimum, gitVersionMin)
		}
	})

	t.Run("getVersion error", func(t *testing.T) {
		getVersion := func() ([]byte, error) {
			return nil, errors.New("git not found")
		}
		if err := checkGitVersion(getVersion); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("garbage output", func(t *testing.T) {
		getVersion := func() ([]byte, error) {
			return []byte("banana\n"), nil
		}
		if err := checkGitVersion(getVersion); err == nil {
			t.Fatal("expected parse error, got nil")
		}
	})
}

func TestCheckGitVersion_Real(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	err := CheckGitVersion()
	if err == nil {
		return
	}
	var tooOld *VersionTooOldError
	if !errors.As(err, &tooOld) {
		t.Fatalf("CheckGitVersion() returned non-version error: %v", err)
	}
	if tooOld.Current == (GitVersion{}) || tooOld.Minimum == (GitVersion{}) {
		t.Errorf("VersionTooOldError has zero values: %+v", tooOld)
	}
}
