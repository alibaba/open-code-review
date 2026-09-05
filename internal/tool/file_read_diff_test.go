// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
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
			if !strings.HasPrefix(got, "IS_TRUNCATED: false\n") {
				t.Errorf("got %q, want starting with 'IS_TRUNCATED: false\\n'", got)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("got %q, want containing %q", got, tt.wantSub)
			}
		})
	}
}

func TestFileReadDiffProvider_Execute_Truncation(t *testing.T) {
	makeDiff := func(lines int) string {
		var sb strings.Builder
		for i := 1; i <= lines; i++ {
			fmt.Fprintf(&sb, "line %d\n", i)
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	t.Run("single file >500 lines truncated to 500", func(t *testing.T) {
		dm := NewDiffMap(map[string]string{"large.go": makeDiff(501)})
		p := NewFileReadDiff(dm)
		got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"large.go"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "IS_TRUNCATED: true\n") {
			t.Errorf("expected IS_TRUNCATED: true prefix, got:\n%s", got)
		}
		if !strings.HasSuffix(got, "\nNote: Results truncated to 500 lines. Please request specific files or narrow scope.\n") {
			t.Errorf("expected footer note, got:\n%s", got)
		}
		lines := strings.Split(got, "\n")
		// IS_TRUNCATED: true (1) + FILE header (1) + 500 content lines + "" + Note line + "" -> 504 lines
		// Content lines are between header and trailing newline before Note
		var contentLines int
		for _, line := range lines {
			if strings.HasPrefix(line, "line ") {
				contentLines++
			}
		}
		if contentLines != 500 {
			t.Errorf("got %d diff lines, want 500", contentLines)
		}
	})

	t.Run("single file exactly 500 lines not truncated", func(t *testing.T) {
		dm := NewDiffMap(map[string]string{"exact.go": makeDiff(500)})
		p := NewFileReadDiff(dm)
		got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"exact.go"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "IS_TRUNCATED: false\n") {
			t.Errorf("expected IS_TRUNCATED: false prefix, got:\n%s", got)
		}
		if strings.Contains(got, "Note: Results truncated") {
			t.Errorf("unexpected footer note in exact 500 lines output")
		}
	})

	t.Run("multi-file truncation across files", func(t *testing.T) {
		dm := NewDiffMap(map[string]string{
			"a.go": makeDiff(300),
			"b.go": makeDiff(300),
		})
		p := NewFileReadDiff(dm)
		got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"a.go", "b.go"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "IS_TRUNCATED: true\n") {
			t.Errorf("expected IS_TRUNCATED: true prefix, got:\n%s", got)
		}
		if !strings.Contains(got, "==== FILE: a.go ====") || !strings.Contains(got, "==== FILE: b.go ====") {
			t.Errorf("expected both file headers present, got:\n%s", got)
		}
		var contentLines int
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "line ") {
				contentLines++
			}
		}
		if contentLines != 500 {
			t.Errorf("got %d total diff lines, want 500", contentLines)
		}
	})

	t.Run("empty diff string does not consume line budget", func(t *testing.T) {
		dm := NewDiffMap(map[string]string{
			"empty.go": "",
			"valid.go": makeDiff(10),
		})
		p := NewFileReadDiff(dm)
		got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"empty.go", "valid.go"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "IS_TRUNCATED: false\n") {
			t.Errorf("expected IS_TRUNCATED: false, got:\n%s", got)
		}
		if !strings.Contains(got, "==== FILE: valid.go ====") {
			t.Errorf("expected valid.go output present")
		}
	})
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
