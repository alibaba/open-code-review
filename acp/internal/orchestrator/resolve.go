// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ResolveOptions controls OCR binary discovery. Explicit and Environment are
// passed by the adapter; Environment defaults to OCR_BINARY when empty.
type ResolveOptions struct {
	Explicit       string
	Environment    string
	StartupCWD     string
	LookupPath     func(string) (string, error)
	Version        bool
	VersionTimeout time.Duration
}

// ResolvedBinary is a fixed absolute executable path and its optional version
// probe output. Source identifies which configuration won discovery. A probe
// failure records VersionWarning but does not make an otherwise runnable OCR
// binary unavailable.
type ResolvedBinary struct {
	Path           string
	Source         string
	Version        string
	VersionWarning string
}

// ResolveBinary discovers and validates OCR exactly once. An invalid explicit
// path is an error and never falls back to the environment or PATH.
func ResolveBinary(opts ResolveOptions) (ResolvedBinary, error) {
	lookup := opts.LookupPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	startup := opts.StartupCWD
	if startup == "" {
		var err error
		startup, err = os.Getwd()
		if err != nil {
			return ResolvedBinary{}, newError(ErrorStart, "get startup working directory: %v", err)
		}
	}
	path, source := opts.Explicit, "--ocr-binary"
	if path == "" {
		path, source = opts.Environment, "OCR_BINARY"
		if path == "" {
			path = os.Getenv("OCR_BINARY")
		}
	}
	if path == "" {
		var err error
		path, err = lookup("ocr")
		if err != nil {
			return ResolvedBinary{}, newError(ErrorStart, "find OCR binary from PATH: %v", err)
		}
		source = "PATH"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(startup, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return ResolvedBinary{}, newError(ErrorStart, "resolve OCR binary from %s (%q): %v", source, path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return ResolvedBinary{}, newError(ErrorStart, "OCR binary from %s (%q) is unavailable: %v", source, path, err)
	}
	if info.IsDir() {
		return ResolvedBinary{}, newError(ErrorStart, "OCR binary from %s (%q) is a directory", source, path)
	}
	if info.Mode()&0111 == 0 {
		return ResolvedBinary{}, newError(ErrorStart, "OCR binary from %s (%q) is not executable", source, path)
	}
	resolved := ResolvedBinary{Path: path, Source: source}
	if opts.Version || opts.VersionTimeout != 0 {
		timeout := opts.VersionTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		version, err := probeVersion(path, timeout)
		if err != nil {
			resolved.Version = "unknown"
			resolved.VersionWarning = fmt.Sprintf("OCR version probe for %s failed: %v", source, err)
			return resolved, nil
		}
		resolved.Version = version
	}
	return resolved, nil
}

func probeVersion(binary string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return "", err
	}
	defer stdin.Close()
	cmd.Stdin = stdin
	var output limitedBuffer
	output.limit = 1024
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Run()
	if output.exceeded || output.Len() >= output.limit {
		return "", fmt.Errorf("version output exceeded 1024 bytes")
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(output.String())
	if value == "" {
		return "", fmt.Errorf("version output is empty")
	}
	return value, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit-b.Len() <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	remaining := b.limit - b.Len()
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.exceeded = true
		return remaining, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}

func (b *limitedBuffer) String() string {
	return b.Buffer.String()
}

var _ io.Writer = (*limitedBuffer)(nil)
