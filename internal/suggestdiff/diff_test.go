// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package suggestdiff

import (
	"testing"
)

func TestComputeLineDiff(t *testing.T) {
	tests := []struct {
		name     string
		old      []string
		new      []string
		wantLen  int
		wantAdds int
		wantDels int
	}{
		{
			name:     "both empty",
			old:      nil,
			new:      nil,
			wantLen:  0,
			wantAdds: 0,
			wantDels: 0,
		},
		{
			name:     "identical single line",
			old:      []string{"hello"},
			new:      []string{"hello"},
			wantLen:  1,
			wantAdds: 0,
			wantDels: 0,
		},
		{
			name:     "add lines to empty",
			old:      []string{},
			new:      []string{"a", "b"},
			wantLen:  2,
			wantAdds: 2,
			wantDels: 0,
		},
		{
			name:     "delete all lines",
			old:      []string{"a", "b"},
			new:      []string{},
			wantLen:  2,
			wantAdds: 0,
			wantDels: 2,
		},
		{
			name:     "replace single line",
			old:      []string{"old"},
			new:      []string{"new"},
			wantLen:  2,
			wantAdds: 1,
			wantDels: 1,
		},
		{
			name:     "insert in middle",
			old:      []string{"a", "c"},
			new:      []string{"a", "b", "c"},
			wantLen:  3,
			wantAdds: 1,
			wantDels: 0,
		},
		{
			name:     "delete from middle",
			old:      []string{"a", "b", "c"},
			new:      []string{"a", "c"},
			wantLen:  3,
			wantAdds: 0,
			wantDels: 1,
		},
		{
			name:     "multi-line edit",
			old:      []string{"func main() {", "  fmt.Println(\"old\")", "}"},
			new:      []string{"func main() {", "  fmt.Println(\"new\")", "  return", "}"},
			wantLen:  5,
			wantAdds: 2,
			wantDels: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeLineDiff(tt.old, tt.new)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d; diff = %v", len(got), tt.wantLen, got)
			}
			var adds, dels int
			for _, l := range got {
				switch l.Type {
				case DiffAdded:
					adds++
				case DiffDeleted:
					dels++
				}
			}
			if adds != tt.wantAdds {
				t.Errorf("adds = %d, want %d", adds, tt.wantAdds)
			}
			if dels != tt.wantDels {
				t.Errorf("dels = %d, want %d", dels, tt.wantDels)
			}
		})
	}
}

func TestComputeLineDiff_FuzzyMatchPreservesRawChanges(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "case-only change",
			old:  "const maxRetries = 3",
			new:  "const MaxRetries = 3",
		},
		{
			name: "indentation-only change",
			old:  "\treturn nil",
			new:  "\t\treturn nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeLineDiff([]string{tt.old}, []string{tt.new})
			if len(got) != 2 {
				t.Fatalf("diff = %v, want delete + add", got)
			}
			if got[0].Type != DiffDeleted || got[0].Content != tt.old {
				t.Errorf("deleted line = %+v, want %q", got[0], tt.old)
			}
			if got[1].Type != DiffAdded || got[1].Content != tt.new {
				t.Errorf("added line = %+v, want %q", got[1], tt.new)
			}
		})
	}
}

func TestComputeLineDiff_FuzzyMatchPreservesRawChangesWithContext(t *testing.T) {
	old := []string{
		"func validate() error {",
		"\treturn nil",
		"}",
	}
	new := []string{
		"func validate() error {",
		"\t\treturn nil",
		"}",
	}

	got := ComputeLineDiff(old, new)
	want := []DiffLine{
		{Type: DiffContext, Content: "func validate() error {"},
		{Type: DiffDeleted, Content: "\treturn nil"},
		{Type: DiffAdded, Content: "\t\treturn nil"},
		{Type: DiffContext, Content: "}"},
	}

	if len(got) != len(want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("diff[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestComputeLineDiff_ContextContent(t *testing.T) {
	old := []string{"a", "b", "c"}
	new := []string{"a", "x", "c"}
	got := ComputeLineDiff(old, new)

	if got[0].Type != DiffContext || got[0].Content != "a" {
		t.Errorf("first line should be context 'a', got %+v", got[0])
	}
	last := got[len(got)-1]
	if last.Type != DiffContext || last.Content != "c" {
		t.Errorf("last line should be context 'c', got %+v", last)
	}
}
