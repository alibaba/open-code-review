// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileURLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file with space.go")
	uri := fileURLString(path, nil)
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		t.Fatalf("uri=%q err=%v", uri, err)
	}
	native := nativePathFromFileURL(parsed)
	if filepath.Clean(native) != filepath.Clean(path) {
		t.Fatalf("native=%q path=%q uri=%q", native, path, uri)
	}
	if runtime.GOOS == "windows" {
		if parsed.Host != "" || parsed.Path == "" || parsed.Path[0] != '/' {
			t.Fatalf("windows file URI should be file:///C:/... got %q", uri)
		}
	}
	line := 4
	withLine := fileURLString(path, &line)
	if !filepath.IsAbs(path) {
		t.Fatal(path)
	}
	parsed, err = url.Parse(withLine)
	if err != nil || parsed.Fragment != "L4" {
		t.Fatalf("line uri=%q err=%v", withLine, err)
	}
}

func TestFileURLRoundTripNonASCII(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "评审 file.go") // allow-non-english: Windows file URI fixture
	uri := fileURLString(path, nil)
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	native := nativePathFromFileURL(parsed)
	if filepath.Clean(native) != filepath.Clean(path) {
		t.Fatalf("native=%q path=%q uri=%q", native, path, uri)
	}
}
