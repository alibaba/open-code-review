// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveBinaryPriorityAndAbsolutePath(t *testing.T) {
	t.Setenv("OCR_BINARY", "")
	startup := t.TempDir()
	explicit := writeExecutable(t, startup, "explicit", "#!/bin/sh\necho ocr 1.2.3\n")
	environment := writeExecutable(t, startup, "environment", "#!/bin/sh\necho unused\n")
	lookedUp := false
	resolved, err := ResolveBinary(ResolveOptions{
		Explicit:    "explicit",
		Environment: environment,
		StartupCWD:  startup,
		LookupPath:  func(string) (string, error) { lookedUp = true; return "", nil },
		Version:     true,
	})
	if err != nil {
		t.Fatalf("ResolveBinary: %v", err)
	}
	if resolved.Source != "--ocr-binary" || resolved.Path != explicit || resolved.Version != "ocr 1.2.3" || lookedUp {
		t.Fatalf("resolved = %+v, lookup=%v", resolved, lookedUp)
	}
}

func TestResolveBinaryEnvironmentAndPATH(t *testing.T) {
	startup := t.TempDir()
	environment := writeExecutable(t, startup, "environment", "#!/bin/sh\necho env\n")
	t.Setenv("OCR_BINARY", environment)
	resolved, err := ResolveBinary(ResolveOptions{StartupCWD: startup})
	if err != nil || resolved.Source != "OCR_BINARY" || resolved.Path != environment {
		t.Fatalf("environment resolved=%+v err=%v", resolved, err)
	}
	t.Setenv("OCR_BINARY", "")
	pathBinary := writeExecutable(t, startup, "path-ocr", "#!/bin/sh\necho path\n")
	resolved, err = ResolveBinary(ResolveOptions{LookupPath: func(string) (string, error) { return pathBinary, nil }, StartupCWD: startup})
	if err != nil || resolved.Source != "PATH" || resolved.Path != pathBinary {
		t.Fatalf("PATH resolved=%+v err=%v", resolved, err)
	}
}

func TestResolveBinaryInvalidExplicitDoesNotFallBack(t *testing.T) {
	t.Setenv("OCR_BINARY", "")
	called := false
	_, err := ResolveBinary(ResolveOptions{
		Explicit:    filepath.Join(t.TempDir(), "missing"),
		Environment: writeExecutable(t, t.TempDir(), "ocr", "#!/bin/sh\n"),
		LookupPath:  func(string) (string, error) { called = true; return "", nil },
	})
	if err == nil || called || !strings.Contains(err.Error(), "--ocr-binary") {
		t.Fatalf("err = %v, lookup=%v", err, called)
	}
}

func TestResolveBinaryRejectsDirectoryAndNonExecutable(t *testing.T) {
	t.Setenv("OCR_BINARY", "")
	dir := t.TempDir()
	for _, path := range []string{dir, filepath.Join(dir, "plain")} {
		if path != dir {
			if err := os.WriteFile(path, []byte("not executable"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := ResolveBinary(ResolveOptions{Explicit: path}); err == nil {
			t.Fatalf("ResolveBinary(%q) succeeded", path)
		}
	}
}

func TestProbeVersionRejectsOutputLimitAndTimeout(t *testing.T) {
	large := writeExecutable(t, t.TempDir(), "large", "#!/bin/sh\nawk 'BEGIN { for (i = 0; i < 2048; i++) printf \"x\" }'\n")
	if _, err := probeVersion(large, time.Second); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("large version error = %v", err)
	}
	slow := writeExecutable(t, t.TempDir(), "slow", "#!/bin/sh\nsleep 10\n")
	if _, err := probeVersion(slow, 20*time.Millisecond); err == nil {
		t.Fatal("slow version probe succeeded")
	}
}

func writeExecutable(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}
