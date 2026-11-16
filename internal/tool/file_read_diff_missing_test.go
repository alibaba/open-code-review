// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// During a review the snapshot holds the change set, and the model is asked to
// confirm cross-file concerns by reading other files' diffs. When it names a
// path that is not part of the change set, the old output was "diff not found
// for the requested paths" — indistinguishable from a diff that does not exist,
// and offering no way to recover. The model's only move was to re-read the same
// file under another spelling.
//
// These tests pin the answer that gives it a way out: name the paths that had no
// diff, and name the paths that do.

// A wrong path is named, and the paths that do have diffs are named with it.
func TestFileReadDiffNamesMissingPathAndAvailablePaths(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{
		"reviewed.go": "diff --git a/reviewed.go b/reviewed.go\n",
	}))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "a.go") {
		t.Errorf("message should name the path it could not serve: %q", got)
	}
	if !strings.Contains(got, "reviewed.go") {
		t.Errorf("message should name a path that does have a diff: %q", got)
	}
	if !strings.Contains(got, "diff not found") {
		t.Errorf("the original reason must survive: %q", got)
	}
	// Recovering is the point of the change: the non-empty case must point the
	// model at the paths that do have diffs and, for a file outside the change
	// set, at file_read.
	if !strings.Contains(got, "file_read for a file outside the reviewed set") {
		t.Errorf("message should tell the model how to reach a file outside the change set: %q", got)
	}
}

// Several paths in one call: the message must not pretend every path was
// missing, or the model stops looking for the one that was. The error line
// names only the paths that failed — a served path must never be reported as
// missing — and its diff is returned in full.
func TestFileReadDiffReportsPartialHits(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{
		"a.go": "@@ -1 +1 @@\n-old\n+new",
	}))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go", "b.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "==== FILE: a.go ====") {
		t.Fatalf("the available diff must still be returned: %q", got)
	}
	if !strings.Contains(got, "-old\n+new") {
		t.Fatalf("the available diff must be returned in full, not just its header: %q", got)
	}
	if !strings.Contains(got, "Error: diff not found for: b.go\n") {
		t.Errorf("the error line should name exactly the missing paths, not the served ones: %q", got)
	}
}

// Every requested path has a diff: no part of the missing-path message may
// appear at all.
func TestFileReadDiffAllPathsFoundHasNoError(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(map[string]string{
		"a.go": "@@ -1 +1 @@\n-old\n+new",
		"b.go": "@@ -1 +1 @@\n-old\n+new",
	}))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go", "b.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "diff not found") {
		t.Errorf("a fully served request must not carry the missing-path message: %q", got)
	}
	if strings.Contains(got, "Error:") {
		t.Errorf("a fully served request must carry no error line: %q", got)
	}
}

// The available-path list exists to be read in one glance, so it is capped and
// the remainder is counted rather than listed.
func TestFileReadDiffCapsAvailablePathList(t *testing.T) {
	m := make(map[string]string, diffNotFoundMaxListed+2)
	for i := 0; i < diffNotFoundMaxListed+2; i++ {
		m[fmt.Sprintf("f%02d.go", i)] = "diff"
	}
	p := NewFileReadDiff(NewDiffMap(m))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"missing.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "(and 2 more)") {
		t.Errorf("the paths beyond the cap should be counted: %q", got)
	}
	if strings.Contains(got, fmt.Sprintf("f%02d.go", diffNotFoundMaxListed)) {
		t.Errorf("a path past the cap should not be listed: %q", got)
	}
}

// An empty request stays a malformed call with its own message.
func TestFileReadDiffEmptyRequestUnchanged(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(nil))

	got, err := p.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Error: no files found" {
		t.Fatalf("empty request should keep its own message, got %q", got)
	}
}

// An empty snapshot has no paths to offer. Both review and scan reach this
// state only through their own initialization (scan injects full file contents,
// a review with no diffs never dispatches a round), so the tool cannot tell
// which mode it is in — and must not guess one in the message.
func TestFileReadDiffEmptySnapshotClaimsNoMode(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(nil))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"a.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "a.go") {
		t.Errorf("message should name the path it could not serve: %q", got)
	}
	if !strings.Contains(got, "file_read") {
		t.Errorf("with nothing to point at, file_read is the only recovery the message can offer: %q", got)
	}
	for _, unwanted := range []string{"scan", "reviewed set"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("message guesses at the mode (%q): %q", unwanted, got)
		}
	}
}
