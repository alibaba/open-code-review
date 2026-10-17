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
			name:    "non-string entries are skipped",
			args:    map[string]any{"path_array": []any{float64(7), "a.go"}},
			wantSub: "==== FILE: a.go ====",
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

func diffLines(prefix string, n int) string {
	lines := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		lines = append(lines, fmt.Sprintf("%s line %03d", prefix, i))
	}
	return strings.Join(lines, "\n")
}

func TestFileReadDiffProvider_Execute_BoundsOutputAndPages(t *testing.T) {
	diffA := diffLines("-a/+a", 1200)
	diffB := "@@ -1 +1 @@\n-x\n+y"
	blockA := "==== FILE: a.go ====\n" + diffA + "\n"
	blockB := "==== FILE: b.go ====\n" + diffB + "\n"

	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": diffA, "b.go": diffB}))
	args := func(extra map[string]any) map[string]any {
		base := map[string]any{"path_array": []any{"a.go", "b.go"}}
		for k, v := range extra {
			base[k] = v
		}
		return base
	}

	page1, err := p.Execute(context.Background(), args(nil))
	if err != nil {
		t.Fatalf("page 1: unexpected error: %v", err)
	}
	wantPrefix := "IS_TRUNCATED: true\nNEXT_OFFSET: 500\n==== FILE: a.go ====\n"
	if !strings.HasPrefix(page1, wantPrefix) {
		t.Fatalf("page 1 = %q, want prefix %q", page1, wantPrefix)
	}
	content1 := strings.SplitN(page1, "\n", 3)[2]
	if got := strings.Count(content1, "\n"); got != 500 {
		t.Errorf("page 1 carried %d content lines, want 500", got)
	}

	page2, err := p.Execute(context.Background(), args(map[string]any{"offset": 500}))
	if err != nil {
		t.Fatalf("page 2: unexpected error: %v", err)
	}
	// Page 2 opens mid-file, so a.go's header is repeated as an anchor.
	wantPrefix = "IS_TRUNCATED: true\nNEXT_OFFSET: 1000\n==== FILE: a.go ====\n"
	if !strings.HasPrefix(page2, wantPrefix) {
		t.Fatalf("page 2 = %q, want prefix %q", page2, wantPrefix)
	}
	content2 := strings.TrimPrefix(strings.SplitN(page2, "\n", 3)[2], "==== FILE: a.go ====\n")
	if got := strings.Count(content2, "\n"); got != 500 {
		t.Errorf("page 2 carried %d content lines, want 500", got)
	}

	page3, err := p.Execute(context.Background(), args(map[string]any{"offset": 1000}))
	if err != nil {
		t.Fatalf("page 3: unexpected error: %v", err)
	}
	// The last page also opens mid-file and still ends inside b.go's block.
	wantPrefix = "IS_TRUNCATED: false\n==== FILE: a.go ====\n"
	if !strings.HasPrefix(page3, wantPrefix) {
		t.Fatalf("page 3 = %q, want prefix %q", page3, wantPrefix)
	}
	content3 := strings.TrimPrefix(strings.SplitN(page3, "\n", 3)[2], "==== FILE: a.go ====\n")
	if got := strings.Count(content3, "\n"); got != 205 {
		t.Errorf("page 3 carried %d content lines, want 205", got)
	}

	if content1+content2+content3 != blockA+blockB {
		t.Error("pages 1-3 do not reassemble into the unbounded output")
	}
}

func TestFileReadDiffProvider_Execute_MaxLines(t *testing.T) {
	diff := diffLines("-a/+a", 600)
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": diff}))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
		"max_lines":  float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "IS_TRUNCATED: true\nNEXT_OFFSET: 10\n") {
		t.Errorf("got %q, want IS_TRUNCATED true and NEXT_OFFSET 10", got)
	}
	if n := strings.Count(got, "\n") - 2; n != 10 {
		t.Errorf("carried %d content lines, want 10", n)
	}
}

func TestFileReadDiffProvider_Execute_MaxLinesClampedToCap(t *testing.T) {
	diff := diffLines("-a/+a", 6000)
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": diff}))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
		"max_lines":  float64(999999),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "IS_TRUNCATED: true\nNEXT_OFFSET: 5000\n") {
		t.Errorf("max_lines was not clamped to the cap: %q", got[:64])
	}
}

func TestFileReadDiffProvider_Execute_OffsetPastEnd(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": "@@ -1 +1 @@\n-x\n+y"}))

	_, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
		"offset":     float64(9999),
	})
	if err == nil || !strings.Contains(err.Error(), "past the end") {
		t.Errorf("offset past end: err = %v, want a past-the-end error", err)
	}
}

func TestFileReadDiffProvider_Execute_InvalidPagingArgs(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": "@@ -1 +1 @@\n-x\n+y"}))
	pathArray := []any{"a.go"}

	tests := []struct {
		name    string
		args    map[string]any
		wantSub string
	}{
		{
			name:    "negative offset",
			args:    map[string]any{"path_array": pathArray, "offset": float64(-1)},
			wantSub: "offset must be >= 0",
		},
		{
			name:    "zero max_lines",
			args:    map[string]any{"path_array": pathArray, "max_lines": float64(0)},
			wantSub: "max_lines must be > 0",
		},
		{
			name:    "non-integer offset",
			args:    map[string]any{"path_array": pathArray, "offset": "500"},
			wantSub: "offset must be an integer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Execute(context.Background(), tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %v, want containing %q", err, tt.wantSub)
			}
		})
	}
}

func TestFileReadDiffProvider_Execute_SingleLineByteBackstop(t *testing.T) {
	huge := strings.Repeat("x", fileReadDiffMaxBytes+1)
	p := NewFileReadDiff(NewDiffMap(map[string]string{"min.js": huge}))

	_, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"min.js"}})
	if err == nil || !strings.Contains(err.Error(), "per-call limit") {
		t.Errorf("err = %v, want a per-call byte limit error", err)
	}
}

func TestFileReadDiffProvider_Execute_ByteBackstopCutsBetweenLines(t *testing.T) {
	// Each line costs 4097 bytes, so the 1 MiB byte budget binds well before
	// the 500-line budget and must stop on a line boundary.
	line := strings.Repeat("y", 4096)
	diff := diffLines(line, 600)
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": diff}))

	got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"a.go"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "IS_TRUNCATED: true\nNEXT_OFFSET: 256\n") {
		t.Errorf("byte backstop did not bound the page: %q", got[:80])
	}
	content := strings.SplitN(got, "\n", 3)[2]
	if n := strings.Count(content, "\n"); n != 256 {
		t.Errorf("carried %d content lines, want 256", n)
	}
	if len(content) > fileReadDiffMaxBytes {
		t.Errorf("content is %d bytes, over the %d-byte budget", len(content), fileReadDiffMaxBytes)
	}
}

func TestFileReadDiffProvider_Execute_OversizedLineAfterFullPageKeepsPage(t *testing.T) {
	// The oversized-line check must not run before the page boundary: the
	// oversized line sits at offset 2, outside the requested two-line page,
	// and erroring on it would discard the collected page and its
	// continuation offset. The caller re-requests at NEXT_OFFSET and meets
	// the oversized-line error there, where that line is really the next
	// requested content.
	diff := "normal line\n" + strings.Repeat("x", fileReadDiffMaxBytes+1)
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": diff}))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
		"offset":     float64(0),
		"max_lines":  float64(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "IS_TRUNCATED: true\nNEXT_OFFSET: 2\n") {
		t.Errorf("full page was not preserved with a continuation offset: %q", got[:80])
	}
	content := strings.SplitN(got, "\n", 3)[2]
	if n := strings.Count(content, "\n"); n != 2 {
		t.Errorf("carried %d lines, want 2 (file header + normal line)", n)
	}
	if !strings.Contains(content, "normal line") {
		t.Errorf("collected page content was discarded: %q", content)
	}

	// The next request lands on the oversized line itself: there the error is
	// expected, because that line is now the next requested content.
	if _, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
		"offset":     float64(2),
		"max_lines":  float64(2),
	}); err == nil || !strings.Contains(err.Error(), "per-call limit") {
		t.Errorf("err = %v, want a per-call byte limit error at the oversized line", err)
	}
}
