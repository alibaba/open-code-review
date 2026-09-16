// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"os"
	"testing"

	"github.com/dop251/goja"
)

// TestReposJS_SearchAndPagination executes the shipped static/repos.js (the
// same file the viewer embeds and serves) and then runs testdata/repos_js_assert.js
// against the ocrRepos helpers that file installs. The assertions cover
// substring search, paging of the filtered set, and resetting to page 1 when
// the query changes — they call those helpers, they do not reimplement them.
//
// This file is named repos_client_test.go rather than repos_js_test.go because
// Go treats a _js suffix as GOOS=js (WebAssembly) and would exclude it.
func TestReposJS_SearchAndPagination(t *testing.T) {
	src, err := assets.ReadFile("static/repos.js")
	if err != nil {
		t.Fatalf("read shipped repos.js: %v", err)
	}
	assertSrc, err := os.ReadFile("testdata/repos_js_assert.js")
	if err != nil {
		t.Fatalf("read repos.js assertions: %v", err)
	}

	vm := goja.New()
	if _, err := vm.RunString(string(src)); err != nil {
		t.Fatalf("execute shipped repos.js: %v", err)
	}
	if _, err := vm.RunString(string(assertSrc)); err != nil {
		t.Fatalf("shipped repos.js behavior: %v", err)
	}
}
