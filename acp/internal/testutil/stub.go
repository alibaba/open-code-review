// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package testutil builds and installs the cross-platform OCR stub used by ACP tests.
package testutil

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Config is written beside an installed stub as <binary>.json.
type Config struct {
	VersionText   string   `json:"version_text,omitempty"`
	VersionPrefix string   `json:"version_prefix,omitempty"`
	VersionSuffix string   `json:"version_suffix,omitempty"`
	VersionBytes  int      `json:"version_bytes,omitempty"`
	SleepMS       int      `json:"sleep_ms,omitempty"`
	ExitCode      int      `json:"exit_code,omitempty"`
	HoldPIDFile   string   `json:"hold_pid_file,omitempty"`
	HoldInherit   bool     `json:"hold_inherit,omitempty"`
	HoldMS        int      `json:"hold_ms,omitempty"`
	FlushMS       int      `json:"flush_ms,omitempty"`
	FlushStdout   string   `json:"flush_stdout,omitempty"`
	FlushStderr   string   `json:"flush_stderr,omitempty"`
	MarkerFile    string   `json:"marker_file,omitempty"`
	StderrLines   []string `json:"stderr_lines,omitempty"`
	Stdout        string   `json:"stdout,omitempty"`
	Block         bool     `json:"block,omitempty"`
	CleanupFile   string   `json:"cleanup_file,omitempty"`
	HugeMessage   int      `json:"huge_message,omitempty"`
}

var (
	buildOnce sync.Once
	builtPath string
	buildErr  error
)

// ExeName returns name with a Windows .exe suffix when needed.
func ExeName(name string) string {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		return name + ".exe"
	}
	return name
}

// Build compiles the stub once per test binary.
func Build(t testing.TB) string {
	t.Helper()
	if err := ensureBuilt(); err != nil {
		t.Fatalf("build ocr-stub: %v", err)
	}
	return builtPath
}

// Cleanup removes the cached build, including any partial build artifacts.
// Call it from TestMain only after m.Run returns and all stub users have stopped.
func Cleanup() error {
	if builtPath == "" {
		return nil
	}
	return os.RemoveAll(filepath.Dir(builtPath))
}

func ensureBuilt() error {
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ocr-acp-stub")
		if err != nil {
			buildErr = err
			return
		}
		builtPath = filepath.Join(dir, ExeName("ocr-stub"))
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			buildErr = errCaller()
			return
		}
		src := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "ocr-stub")
		cmd := exec.Command("go", "build", "-o", builtPath, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = errOutput(err, out)
		}
	})
	return buildErr
}

type callerError struct{}

func errCaller() error { return callerError{} }

func (callerError) Error() string { return "cannot locate testutil source" }

type outputError struct {
	err error
	out []byte
}

func errOutput(err error, out []byte) error { return outputError{err: err, out: out} }

func (e outputError) Error() string {
	return e.err.Error() + "\n" + string(e.out)
}

// Install copies the stub into directory as name and optionally writes a sidecar config.
func Install(t testing.TB, directory, name string, cfg *Config) string {
	t.Helper()
	src := Build(t)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(directory, ExeName(name))
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst+".json", data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}
