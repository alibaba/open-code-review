package tool

import (
	"context"
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
