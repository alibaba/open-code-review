// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package testutil

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildLifetime(t *testing.T) {
	const recordEnv = "OCR_ACP_STUB_LIFETIME_RECORD"
	if record := os.Getenv(recordEnv); record != "" {
		var built string
		t.Run("first user", func(t *testing.T) { built = Build(t) })
		t.Run("later user", func(t *testing.T) {
			if got := Build(t); got != built {
				t.Fatalf("cached build = %q, want %q", got, built)
			}
			if _, err := os.Stat(built); err != nil {
				t.Fatalf("cached stub did not survive first test: %v", err)
			}
		})
		if err := os.WriteFile(record, []byte(built), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	record := filepath.Join(t.TempDir(), "built-path")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestBuildLifetime$")
	cmd.Env = append(os.Environ(), recordEnv+"="+record)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stub lifetime subprocess: %v\n%s", err, out)
	}
	built, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(string(built))
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("stub directory remains after test process exit: %s (stat: %v)", directory, err)
	}
}

func TestExeName(t *testing.T) {
	want := "ocr-stub"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got := ExeName("ocr-stub"); got != want {
		t.Fatalf("ExeName(ocr-stub) = %q, want %q", got, want)
	}
	if got := ExeName("ocr-stub.exe"); got != "ocr-stub.exe" {
		t.Fatalf("ExeName(ocr-stub.exe) = %q, want it unchanged", got)
	}
}

func TestBuild(t *testing.T) {
	built := Build(t)
	if built == "" {
		t.Fatal("Build returned an empty path")
	}
	info, err := os.Stat(built)
	if err != nil {
		t.Fatalf("stat built stub: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("built stub %q is a directory", built)
	}
}

func TestInstallWithoutConfig(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "nested", "stubs")
	installed := Install(t, directory, "plain", nil)
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("stat installed stub: %v", err)
	}
	if _, err := os.Stat(installed + ".json"); !os.IsNotExist(err) {
		t.Fatalf("unexpected sidecar config for a nil config: %v", err)
	}
}

func TestInstallWithConfig(t *testing.T) {
	cfg := &Config{}
	cfg.VersionText = "ocr 9.9.9"
	cfg.ExitCode = 7
	cfg.Stdout = "stdout line"
	cfg.StderrLines = []string{"stderr line"}

	installed := Install(t, t.TempDir(), "configured", cfg)
	if runtime.GOOS != "windows" && !strings.HasSuffix(installed, "configured") {
		t.Fatalf("installed path %q does not end with the requested name", installed)
	}

	data, err := os.ReadFile(installed + ".json")
	if err != nil {
		t.Fatalf("read sidecar config: %v", err)
	}
	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode sidecar config: %v", err)
	}
	if decoded.VersionText != cfg.VersionText || decoded.ExitCode != cfg.ExitCode {
		t.Fatalf("sidecar config = %+v, want version %q and exit code %d", decoded, cfg.VersionText, cfg.ExitCode)
	}
	if len(decoded.StderrLines) != 1 || decoded.StderrLines[0] != "stderr line" {
		t.Fatalf("sidecar stderr lines = %v", decoded.StderrLines)
	}
}

func TestErrorFormatting(t *testing.T) {
	if got := errCaller().Error(); got != "cannot locate testutil source" {
		t.Fatalf("errCaller = %q", got)
	}
	if got := (callerError{}).Error(); got != "cannot locate testutil source" {
		t.Fatalf("callerError = %q", got)
	}
	if got := errOutput(os.ErrNotExist, []byte("build failure detail")).Error(); !strings.Contains(got, "build failure detail") {
		t.Fatalf("errOutput = %q", got)
	}
	direct := outputError{err: os.ErrNotExist, out: []byte("more detail")}
	if !strings.Contains(direct.Error(), "more detail") {
		t.Fatal("outputError dropped the command output")
	}
}
