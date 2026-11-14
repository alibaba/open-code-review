// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOCRHomeDefaultsToHomeDir(t *testing.T) {
	t.Setenv(HomeEnvVar, "")
	got, err := OCRHome()
	if err != nil {
		t.Fatalf("OCRHome: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	want := filepath.Join(home, ".opencodereview")
	if got != want {
		t.Errorf("OCRHome() = %q, want %q — existing installs must keep reading the same dir", got, want)
	}
}

func TestOCRHomeUsesEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(HomeEnvVar, dir)
	got, err := OCRHome()
	if err != nil {
		t.Fatalf("OCRHome: %v", err)
	}
	if got != dir {
		t.Errorf("OCRHome() = %q, want %q", got, dir)
	}
}

func TestOCRHomeIgnoresBlankEnvVar(t *testing.T) {
	// An empty or whitespace value means "unset", not "use the current dir".
	for _, v := range []string{"", "   "} {
		t.Setenv(HomeEnvVar, v)
		got, err := OCRHome()
		if err != nil {
			t.Fatalf("OCRHome: %v", err)
		}
		if got == "" || got == "." {
			t.Errorf("OCRHome() = %q for env %q, want the default home dir", got, v)
		}
	}
}

func TestOCRHomeIsAbsolute(t *testing.T) {
	// Sessions and config are written here, so a relative value would put them
	// wherever the process happens to be running.
	t.Setenv(HomeEnvVar, "relative/ocr")
	got, err := OCRHome()
	if err != nil {
		t.Fatalf("OCRHome: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("OCRHome() = %q, want an absolute path", got)
	}
}
