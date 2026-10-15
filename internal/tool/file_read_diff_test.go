// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chunk"
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
	// Reads are bounded, so the provider is given a budget before use. The
	// legacy shape - no store, chunked on the fly - is the one scan and any
	// unconfigured caller get, so the cases below also pin that path.
	p.SetContextStore(nil, 4096)

	tests := []struct {
		name    string
		args    map[string]any
		wantSub string
		wantErr string
	}{
		{
			name:    "single existing path",
			args:    map[string]any{"path_array": []any{"a.go"}},
			wantSub: "==== CHUNK ",
		},
		{
			name:    "multiple paths",
			args:    map[string]any{"path_array": []any{"a.go", "b.go"}},
			wantSub: "==== CHUNK ",
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
	p.SetContextStore(nil, 4096)

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

// The bounded-read contract: a provider configured with a budget must never
// hand back more than that, and must name whatever it left out. These cases
// cover the shapes the review path uses - chunk_id, a single path listing, and
// a multi-path read - plus the one case the model cannot influence.
func boundedProvider(t *testing.T, budget int) (*FileReadDiffProvider, *chunk.Store) {
	t.Helper()
	dm := NewDiffMap(map[string]string{
		"a.go": largeDiff(6, 40),
		"b.go": largeDiff(3, 20),
	})
	store := chunk.NewStore(budget)
	store.Add("g1", "a.go", chunk.Split("a.go", dm.m["a.go"], chunk.Options{Budget: budget}))
	store.Add("g1", "b.go", chunk.Split("b.go", dm.m["b.go"], chunk.Options{Budget: budget}))
	p := NewFileReadDiff(dm)
	p.SetContextStore(store, budget)
	return p, store
}

func largeDiff(hunks, body int) string {
	var b strings.Builder
	b.WriteString("diff --git a/f.go b/f.go\n@@ -1,1 +1,1 @@\n")
	for h := 0; h < hunks; h++ {
		b.WriteString("@@ -1,1 +1,1 @@\n")
		for i := 0; i < body; i++ {
			b.WriteString("+a line of diff payload with a little more text in it\n")
		}
	}
	return b.String()
}

func TestFileReadDiff_ChunkIDServesExactlyThatChunk(t *testing.T) {
	p, store := boundedProvider(t, 400)
	id := store.ChunksOfGroup("g1")[0].ID

	got, err := p.Execute(context.Background(), map[string]any{"chunk_id": id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "==== CHUNK "+id) {
		t.Errorf("chunk_id must serve that chunk: %q", got)
	}
	if strings.Contains(got, "[context read bounded]") {
		t.Error("an exact chunk read defers nothing")
	}
}

func TestFileReadDiff_UnknownChunkIDIsReported(t *testing.T) {
	p, _ := boundedProvider(t, 400)
	got, _ := p.Execute(context.Background(), map[string]any{"chunk_id": "cnope"})
	if !strings.Contains(got, "unknown chunk id") {
		t.Errorf("got %q, want an unknown-id report the model can act on", got)
	}
}

func TestFileReadDiff_SinglePathListsChunksWithoutContent(t *testing.T) {
	p, store := boundedProvider(t, 400)
	got, _ := p.Execute(context.Background(), map[string]any{"path": "a.go"})
	if !strings.Contains(got, "<chunk_index path=\"a.go\"") {
		t.Errorf("got %q, want the chunk index of the path", got)
	}
	if strings.Contains(got, "a line of diff payload") {
		t.Error("the listing exists so the manifest stays small: it must not carry content")
	}
	if len(store.ChunksOfGroup("g1")) == 0 {
		t.Error("fixture broken")
	}
}

func TestFileReadDiff_MultiPathReadStaysWithinTheBudget(t *testing.T) {
	const budget = 500
	p, store := boundedProvider(t, budget)
	got, _ := p.Execute(context.Background(), map[string]any{"path_array": []any{"a.go", "b.go"}})

	if !strings.Contains(got, "The diff is NOT fully read") {
		t.Fatalf("a read that stopped at the budget must say so: %q", got)
	}
	for _, c := range store.ChunksOfGroup("g1") {
		if !strings.Contains(got, "==== CHUNK "+c.ID) {
			continue
		}
		if c.Tokens > budget && !c.Oversized {
			t.Errorf("served a %d-token chunk under a %d-token budget", c.Tokens, budget)
		}
	}
}

func TestFileReadDiff_MaxTokensOnlyLowersTheBudget(t *testing.T) {
	p, _ := boundedProvider(t, 2000)
	lower, _ := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"}, "max_tokens": 300,
	})
	if !strings.Contains(lower, "The diff is NOT fully read") {
		t.Error("a smaller max_tokens must actually bound the read")
	}

	// Asking for more than the run allows must not widen it.
	higher, _ := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"}, "max_tokens": 999999,
	})
	if higher != lower && strings.Contains(higher, "NOT fully read") && !strings.Contains(lower, "NOT fully read") {
		t.Error("max_tokens must never raise the run's read budget")
	}
	if got := int(p.readBudget.Load()); got != 2000 {
		t.Errorf("read budget = %d, want it unchanged at 2000", got)
	}
}

func TestFileReadDiff_RefusesWithoutABudget(t *testing.T) {
	// Fail closed: a provider nobody configured must not serve the unbounded
	// read it exists to prevent.
	p := NewFileReadDiff(NewDiffMap(map[string]string{"a.go": largeDiff(2, 10)}))
	got, _ := p.Execute(context.Background(), map[string]any{"path_array": []any{"a.go"}})
	if !strings.Contains(got, "Error: diff not found") {
		t.Errorf("an unconfigured provider must refuse, got %q", got)
	}
}

func TestFileReadDiff_LowerReadBudgetOnlyTightens(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(nil))
	p.LowerReadBudget(1000)
	if got := int(p.readBudget.Load()); got != 1000 {
		t.Fatalf("first budget must be set, got %d", got)
	}
	p.LowerReadBudget(400)
	if got := int(p.readBudget.Load()); got != 400 {
		t.Errorf("a smaller budget must win, got %d", got)
	}
	p.LowerReadBudget(900)
	if got := int(p.readBudget.Load()); got != 400 {
		t.Errorf("a larger budget must not widen an established one, got %d", got)
	}
}

func TestFileReadDiff_UnregisteredPathIsServedThroughTheFallback(t *testing.T) {
	// A path whose diff exists in the snapshot but was never registered as
	// chunks must still be readable: reporting it missing would be a lie.
	dm := NewDiffMap(map[string]string{"later.go": largeDiff(2, 30)})
	p := NewFileReadDiff(dm)
	p.SetContextStore(chunk.NewStore(400), 400)

	got, _ := p.Execute(context.Background(), map[string]any{"path_array": []any{"later.go"}})
	if !strings.Contains(got, "FILE: later.go") {
		t.Errorf("an unregistered but present path must be served: %q", got)
	}
}
