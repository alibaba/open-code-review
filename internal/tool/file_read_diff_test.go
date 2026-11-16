// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestNewDiffMap_DefensiveCopy(t *testing.T) {
	orig := map[string]string{"a.go": "diff a"}
	dm := NewDiffMap(orig)
	orig["a.go"] = "mutated"
	if v, _ := dm.Get("a.go"); v != "diff a" {
		t.Error("NewDiffMap should make a defensive copy")
	}
}

func TestDiffMap_Get(t *testing.T) {
	dm := NewDiffMap(map[string]string{"x.go": "content"})

	v, ok := dm.Get("x.go")
	if !ok || v != "content" {
		t.Errorf("Get(x.go) = %q, %v; want 'content', true", v, ok)
	}

	_, ok = dm.Get("missing.go")
	if ok {
		t.Error("Get(missing.go) should return false")
	}
}

func TestFileReadDiffProvider_Execute(t *testing.T) {
	dm := NewDiffMap(map[string]string{
		"a.go": "@@ -1 +1 @@\n-old\n+new",
		"b.go": "@@ -5 +5 @@\n-foo\n+bar",
	})
	p := NewFileReadDiff(dm)

	tests := []struct {
		name    string
		args    map[string]any
		wantSub string
		wantErr string
	}{
		{
			name:    "single existing path",
			args:    map[string]any{"path_array": []any{"a.go"}},
			wantSub: "==== FILE: a.go ====",
		},
		{
			name:    "multiple paths",
			args:    map[string]any{"path_array": []any{"a.go", "b.go"}},
			wantSub: "==== FILE: b.go ====",
		},
		{
			name:    "missing path",
			args:    map[string]any{"path_array": []any{"missing.go"}},
			wantErr: "Error: diff not found",
		},
		{
			name:    "empty path_array",
			args:    map[string]any{"path_array": []any{}},
			wantErr: "Error: no files found",
		},
		{
			name:    "nil path_array",
			args:    map[string]any{},
			wantErr: "Error: no files found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Execute(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if !strings.Contains(got, tt.wantErr) {
					t.Errorf("got %q, want containing %q", got, tt.wantErr)
				}
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("got %q, want containing %q", got, tt.wantSub)
			}
		})
	}
}

func TestFileReadDiffProvider_SetDiffMap(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{"old.go": "v1"}))
	p.SetDiffMap(NewDiffMap(map[string]string{"new.go": "v2"}))

	got, _ := p.Execute(context.Background(), map[string]any{"path_array": []any{"new.go"}})
	if !strings.Contains(got, "new.go") {
		t.Errorf("SetDiffMap not applied: %q", got)
	}
}

func TestFileReadDiffProvider_Tool(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(nil))
	if p.Tool() != FileReadDiff {
		t.Errorf("Tool() = %v, want FileReadDiff", p.Tool())
	}
}

func TestDiffMap_Get_Normalized(t *testing.T) {
	dm := NewDiffMap(map[string]string{
		"pkg/util.go": "diff content",
	})

	for _, query := range []string{
		"pkg/util.go",
		`pkg\util.go`,
		"./pkg/util.go",
		`.\pkg\util.go`,
		"/pkg/util.go",
	} {
		v, ok := dm.Get(query)
		if !ok || v != "diff content" {
			t.Errorf("Get(%q) = (%q, %v), want ('diff content', true)", query, v, ok)
		}
	}

	// Preserve literal backslash keys verbatim without collapsing to forward slashes.
	dm2 := NewDiffMap(map[string]string{
		`dir\file.go`: "diff content 2",
	})
	if got := dm2.Paths(); !reflect.DeepEqual(got, []string{`dir\file.go`}) {
		t.Errorf("Paths() = %v, want [%q]", got, `dir\file.go`)
	}
	if v, ok := dm2.Get(`dir\file.go`); !ok || v != "diff content 2" {
		t.Errorf("Get(dir\\file.go) = (%q, %v), want ('diff content 2', true)", v, ok)
	}
	if v, ok := dm2.Get(`./dir\file.go`); !ok || v != "diff content 2" {
		t.Errorf("Get(./dir\\file.go) = (%q, %v), want ('diff content 2', true)", v, ok)
	}
	if runtime.GOOS != "windows" {
		if v, ok := dm2.Get("dir/file.go"); ok {
			t.Errorf("Get(dir/file.go) on POSIX backslash-only map = (%q, %v), want ('', false)", v, ok)
		}
	} else {
		if v, ok := dm2.Get("dir/file.go"); !ok || v != "diff content 2" {
			t.Errorf("Get(dir/file.go) on Windows = (%q, %v), want ('diff content 2', true)", v, ok)
		}
	}
}

func TestDiffMap_POSIXDualPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX literal backslashes cannot coexist with directory separators on Windows")
	}

	dm := NewDiffMap(map[string]string{
		"pkg/util.go": "diff slash",
		`pkg\util.go`: "diff backslash",
	})

	wantPaths := []string{"pkg/util.go", `pkg\util.go`}
	if got := dm.Paths(); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("Paths() = %q, want %q", got, wantPaths)
	}

	cases := []struct {
		query  string
		want   string
		wantOK bool
	}{
		{query: "pkg/util.go", want: "diff slash", wantOK: true},
		{query: "./pkg/util.go", want: "diff slash", wantOK: true},
		{query: "/pkg/util.go", want: "diff slash", wantOK: true},
		{query: `pkg\util.go`, want: "diff backslash", wantOK: true},
		{query: `./pkg\util.go`, want: "diff backslash", wantOK: true},
		{query: `/pkg\util.go`, want: "diff backslash", wantOK: true},
		{query: `.\pkg\util.go`, want: "", wantOK: false},
		{query: `pkg\.\util.go`, want: "", wantOK: false},
	}

	for _, tc := range cases {
		got, ok := dm.Get(tc.query)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("Get(%q) = (%q, %v), want (%q, %v)", tc.query, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestFileReadDiffProvider_Execute_WindowsPaths(t *testing.T) {
	dm := NewDiffMap(map[string]string{
		"pkg/util.go": "@@ -1 +1 @@\n-old\n+new",
	})
	p := NewFileReadDiff(dm)

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{`pkg\util.go`, `./pkg/util.go`},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "==== FILE: pkg/util.go ====") {
		t.Errorf("expected normalized file header in output, got: %s", got)
	}
	if strings.Contains(got, "Error: diff not found") {
		t.Errorf("did not expect missing error, got: %s", got)
	}
}

func TestFileReadDiffProvider_Execute_POSIXDualPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX literal backslashes cannot coexist with directory separators on Windows")
	}

	dm := NewDiffMap(map[string]string{
		"pkg/util.go": "@@ -1 +1 @@\n-slash-old\n+slash-new",
		`pkg\util.go`: "@@ -1 +1 @@\n-backslash-old\n+backslash-new",
	})
	p := NewFileReadDiff(dm)

	// Exact queries select their distinct canonical diffs without substitution.
	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"pkg/util.go", `pkg\util.go`},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "==== FILE: pkg/util.go ====\n@@ -1 +1 @@\n-slash-old\n+slash-new") {
		t.Errorf("missing slash diff in output: %s", got)
	}
	if !strings.Contains(got, "==== FILE: pkg\\util.go ====\n@@ -1 +1 @@\n-backslash-old\n+backslash-new") {
		t.Errorf("missing literal-backslash diff in output: %s", got)
	}

	// Ambiguous Windows-style alias fails explicitly without serving either file.
	amb, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{`.\pkg\util.go`},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(amb, "ambiguous diff path") || !strings.Contains(amb, "pkg/util.go") || !strings.Contains(amb, `pkg\util.go`) {
		t.Errorf("expected explicit ambiguity error listing candidates, got: %s", amb)
	}
	if strings.Contains(amb, "slash-new") || strings.Contains(amb, "backslash-new") {
		t.Errorf("ambiguous lookup must not serve diff content: %s", amb)
	}

	// Partial batch with exact hit + ambiguous alias + missing path handles each cleanly.
	mixed, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{`./pkg\util.go`, `.\pkg\util.go`, "missing.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(mixed, "==== FILE: pkg\\util.go ====\n@@ -1 +1 @@\n-backslash-old\n+backslash-new") {
		t.Errorf("expected unambiguous ./pkg\\util.go diff in mixed batch, got: %s", mixed)
	}
	if !strings.Contains(mixed, "Error: diff not found for: missing.go") {
		t.Errorf("expected missing path error in mixed batch, got: %s", mixed)
	}
	if !strings.Contains(mixed, "Error: ambiguous diff path") {
		t.Errorf("expected ambiguity error in mixed batch, got: %s", mixed)
	}
}
