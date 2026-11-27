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
		"path_array": []any{"a.go", "b.go"},
	})
	assertDiagnosticError(t, got, err, "Error: diff not found for: a.go, b.go\n"+
		"Diffs exist for: reviewed.go\nUse one of those paths, or file_read for a file outside the reviewed set.\n")
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
	for i := range diffNotFoundMaxListed + 2 {
		m[fmt.Sprintf("f%02d.go", i)] = "diff"
	}
	p := NewFileReadDiff(NewDiffMap(m))

	got, err := p.Execute(context.Background(), map[string]any{
		"path_array": []any{"missing.go"},
	})
	diagnostic := requireDiagnosticError(t, got, err)
	if !strings.Contains(diagnostic, "(and 2 more)") {
		t.Errorf("the paths beyond the cap should be counted: %q", diagnostic)
	}
	if strings.Contains(diagnostic, fmt.Sprintf("f%02d.go", diffNotFoundMaxListed)) {
		t.Errorf("a path past the cap should not be listed: %q", diagnostic)
	}
}

// An empty request stays a malformed call with its own message.
func TestFileReadDiffEmptyRequestUnchanged(t *testing.T) {
	p := NewFileReadDiff(NewDiffMap(nil))

	got, err := p.Execute(context.Background(), map[string]any{})
	assertDiagnosticError(t, got, err, "Error: no files found")
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
	assertDiagnosticError(t, got, err, "Error: diff not found for: a.go\n"+
		"This run has no diffs. Read the file with file_read instead.\n")
}
