// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"testing"
)

// The not-found message is derived from the snapshot the provider currently
// holds, so its correctness rests on two properties of that snapshot: it is
// replaced rather than merged when a run swaps in a new one, and it reports its
// paths in a stable order. A stale path surviving a replacement would be named
// as available and then served; an unstable order would make the message differ
// run to run for the same input.

// A re-injection replaces the previous snapshot; a removed path must not
// survive it.
func TestDiffMapReplacementDropsEarlierPaths(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{
		"NOTES.md": "not reviewable",
		"main.go":  "package main",
	}))

	p.SetDiffMap(NewDiffMap(map[string]string{"main.go": "package main"}))

	if _, ok := p.diffMap.Get("NOTES.md"); ok {
		t.Fatalf("SetDiffMap must replace the snapshot, not merge it; paths: %v", p.diffMap.Paths())
	}
	if _, ok := p.diffMap.Get("main.go"); !ok {
		t.Fatalf("main.go must survive the replacement; paths: %v", p.diffMap.Paths())
	}

	// And the tool output agrees with the snapshot.
	got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"NOTES.md"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "not reviewable") {
		t.Fatalf("a dropped path must not be served from the old snapshot: %q", got)
	}
}

// Paths are reported sorted so a message built from them is stable.
func TestDiffMapPathsSorted(t *testing.T) {
	dm := NewDiffMap(map[string]string{"c.go": "", "a.go": "", "b.go": ""})
	got := strings.Join(dm.Paths(), ",")
	if got != "a.go,b.go,c.go" {
		t.Fatalf("Paths() = %q, want sorted", got)
	}
}

// A path that holds an empty diff is still a path the message can offer.
func TestDiffMapPathsKeepsEmptyDiff(t *testing.T) {
	dm := NewDiffMap(map[string]string{"empty.go": ""})
	if _, ok := dm.Get("empty.go"); !ok {
		t.Fatalf("an empty diff is still a present path; paths: %v", dm.Paths())
	}
}
