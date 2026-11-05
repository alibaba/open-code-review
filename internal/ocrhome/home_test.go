// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ocrhome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"unset", "empty"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("OCR_HOME", "")
			if name == "unset" {
				if err := os.Unsetenv("OCR_HOME"); err != nil {
					t.Fatal(err)
				}
			}
			want := filepath.Join(home, ".opencodereview")
			got, err := Dir()
			if err != nil || got != want {
				t.Fatalf("Dir() = %q, %v; want %q", got, err, want)
			}
			if _, err := os.Stat(got); !os.IsNotExist(err) {
				t.Fatalf("Dir created storage: %v", err)
			}
		})
	}
}

func TestDirOverrideWithoutUserHome(t *testing.T) {
	want := filepath.Join(t.TempDir(), "ocr data")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("OCR_HOME", want)
	got, err := Dir()
	if err != nil || got != want {
		t.Fatalf("Dir() = %q, %v; want %q", got, err, want)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("Dir created storage: %v", err)
	}
}

func TestDirRelativeOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("OCR_HOME", filepath.Join("state", "ocr"))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, "state", "ocr")
	got, err := Dir()
	if err != nil || got != want {
		t.Fatalf("Dir() = %q, %v; want %q", got, err, want)
	}
}

func TestDirMissingUserHome(t *testing.T) {
	t.Setenv("OCR_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, err := Dir(); err == nil {
		t.Fatal("Dir succeeded without OCR_HOME or a user home")
	}
}
