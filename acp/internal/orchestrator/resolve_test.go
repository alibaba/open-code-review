// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func TestResolveBinaryPriorityAndAbsolutePath(t *testing.T) {
	t.Setenv("OCR_BINARY", "")
	startup := t.TempDir()
	explicit := testutil.Install(t, startup, "explicit", &testutil.Config{VersionText: "ocr 1.2.3"})
	environment := testutil.Install(t, startup, "environment", &testutil.Config{VersionText: "unused"})
	lookedUp := false
	resolved, err := ResolveBinary(ResolveOptions{
		Explicit:    testutil.ExeName("explicit"),
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
	environment := testutil.Install(t, startup, "environment", &testutil.Config{VersionText: "env"})
	t.Setenv("OCR_BINARY", environment)
	resolved, err := ResolveBinary(ResolveOptions{StartupCWD: startup})
	if err != nil || resolved.Source != "OCR_BINARY" || resolved.Path != environment {
		t.Fatalf("environment resolved=%+v err=%v", resolved, err)
	}
	t.Setenv("OCR_BINARY", "")
	pathBinary := testutil.Install(t, startup, "path-ocr", &testutil.Config{VersionText: "path"})
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
		Environment: testutil.Install(t, t.TempDir(), "ocr", nil),
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
	large := testutil.Install(t, t.TempDir(), "large", &testutil.Config{VersionBytes: 2048})
	if _, err := probeVersion(large, time.Second); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("large version error = %v", err)
	}
	slow := testutil.Install(t, t.TempDir(), "slow", &testutil.Config{SleepMS: 10000})
	if _, err := probeVersion(slow, 20*time.Millisecond); err == nil {
		t.Fatal("slow version probe succeeded")
	}
}

func TestVersionCaptureBoundsChildOutput(t *testing.T) {
	binary := testutil.Install(t, t.TempDir(), "large", &testutil.Config{VersionBytes: 64 << 10})
	output := limitedBuffer{limit: 1024}
	cmd := exec.Command(binary, "--version")
	cmd.Stdout, cmd.Stderr = &output, &output
	// Closing the full capture's reader can also terminate the child with a
	// broken pipe. Either way, the stored bytes must remain bounded.
	var exitError *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.Is(err, io.ErrShortWrite) && !errors.As(err, &exitError) {
		t.Fatal(err)
	}
	if output.Len() != output.limit || !output.exceeded {
		t.Fatalf("capture length=%d limit=%d exceeded=%v", output.Len(), output.limit, output.exceeded)
	}
}

func TestProbeVersionAcceptsExactLimit(t *testing.T) {
	binary := testutil.Install(t, t.TempDir(), "exact", &testutil.Config{VersionBytes: 1024})
	version, err := probeVersion(binary, 5*time.Second)
	if err != nil || len(version) != 1024 {
		t.Fatalf("version length=%d err=%v", len(version), err)
	}
}

func TestResolveBinaryContinuesAfterVersionProbeFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		cfg  testutil.Config
	}{
		{name: "nonzero", cfg: testutil.Config{ExitCode: 7, VersionText: "unsupported"}},
		{name: "output limit", cfg: testutil.Config{VersionBytes: 2048}},
		{name: "timeout", cfg: testutil.Config{SleepMS: 10000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.cfg
			binary := testutil.Install(t, t.TempDir(), "ocr", &cfg)
			resolved, err := ResolveBinary(ResolveOptions{Explicit: binary, Version: true, VersionTimeout: 20 * time.Millisecond})
			if err != nil {
				t.Fatalf("ResolveBinary: %v", err)
			}
			if resolved.Path != binary || resolved.Version != "unknown" || resolved.VersionWarning == "" {
				t.Fatalf("resolved = %+v", resolved)
			}
		})
	}
}

func TestResolveBinaryWindowsExeSuffix(t *testing.T) {
	t.Setenv("OCR_BINARY", "")
	dir := t.TempDir()
	installed := testutil.Install(t, dir, "ocr", &testutil.Config{VersionText: "ocr 9.9.9"})
	withoutExt := strings.TrimSuffix(installed, ".exe")
	if withoutExt == installed {
		t.Skip("executable suffix is not added on this platform")
	}
	resolved, err := ResolveBinary(ResolveOptions{Explicit: withoutExt, Version: true})
	if err != nil || resolved.Path != installed || resolved.Version != "ocr 9.9.9" {
		t.Fatalf("resolved=%+v err=%v want %s", resolved, err, installed)
	}
}
