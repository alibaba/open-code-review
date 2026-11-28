// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLookupReviewEvidenceUsesPinnedOrWorkspaceSource(t *testing.T) {
	r := newDiscoveryRepo(t)
	const p = "dispatch/record.py"
	r.write(p, "def persist(parcel):\n    return parcel.weight\n")
	base := r.commit("store parcel weight")
	r.write(p, "def persist(parcel):\n    return parcel.volume\n")
	r.write("dispatch/disallowed.py", "def persist(internal): pass\n")
	r.write("dispatch/credentials.py", "# -----BEGIN PRIVATE KEY-----\ndef persist(secret): pass\n")
	r.write("dispatch/binary.py", "def persist(binary):\x00\n")
	const transcript = "dispatch/review-notes.json"
	r.write(transcript, `[{"id":"change","body":"persist approval","created_at":"2099-01-01T00:00:00Z"}]`)
	r.write("dispatch/other.py", "def persistent(): pass\n")
	head := r.commit("store parcel volume")
	r.write(p, "def persist(parcel):\n    return parcel.length\n")
	r.write("dispatch/local.py", "def persist_local(): pass\n")
	ctx := t.Context()
	workspace := head
	workspace.Workspace = true
	for _, tc := range []struct {
		tree                 Tree
		area, revision, want string
	}{
		{base, p, base.Commit, "parcel.weight"},
		{head, "dispatch", head.Commit, "parcel.volume"},
		{workspace, ".", "workspace", "parcel.length"},
	} {
		tc.tree.AllowPath = func(path string) bool { return path != "dispatch/disallowed.py" }
		tc.tree = tc.tree.WithoutDiscussionExports([]string{filepath.Join(r.dir, filepath.FromSlash(transcript)), "https://example.com/project/pull/7"})
		result, err := tc.tree.LookupReviewEvidence(ctx, tc.area, "persist")
		if err != nil || len(result) != 1 || result[0].Path != p || result[0].Revision != tc.revision ||
			result[0].StartLine != 1 || result[0].EndLine != 2 || !strings.Contains(result[0].Quote, tc.want) {
			t.Fatalf("lookup %s: %+v, %v", tc.area, result, err)
		}
	}
	if matches, err := workspace.LookupReviewEvidence(ctx, "dispatch/local.py", "persist_local"); err != nil || len(matches) != 1 {
		t.Fatalf("named workspace source: %+v, %v", matches, err)
	}
	for _, q := range [][2]string{{"../outside", "persist"}, {"dispatch", "-e"}, {"dispatch", ""}} {
		if matches, err := head.LookupReviewEvidence(ctx, q[0], q[1]); err == nil || len(matches) != 0 {
			t.Fatalf("accepted invalid lookup %v", q)
		}
	}
	if matches, err := head.LookupReviewEvidence(ctx, ".", "missing_identifier"); err != nil || len(matches) != 0 {
		t.Fatalf("absent lookup: %+v, %v", matches, err)
	}
}

func TestReviewLookupExcerptsStayBoundedAndVerbatim(t *testing.T) {
	source := strings.Repeat("dispatch()\n"+strings.Repeat("# padding\n", 10), 6)
	tree := Tree{Workspace: true}
	matches := tree.reviewLookupSamples("dispatch.py", "dispatch", []byte(source), 4)
	if len(matches) != 4 {
		t.Fatalf("sampled %d excerpts", len(matches))
	}
	lines := strings.Split(source, "\n")
	for _, c := range matches {
		if c.Quote != strings.Join(lines[c.StartLine-1:c.EndLine], "\n") {
			t.Fatal("excerpt no longer matches its source span")
		}
	}
	near := tree.reviewLookupSamples("dispatch.py", "dispatch", []byte(source), 2, 56)
	if len(near) != 2 || near[0].StartLine > 56 || near[0].EndLine < 56 || near[1].StartLine != 1 {
		t.Fatalf("earlier functions displaced the cited use on line 56: %+v", near)
	}
	large := strings.Repeat("x", 1100)
	source = large + "\ndispatch()\n" + large + "\ndispatch(" + large + ")\n"
	matches = tree.reviewLookupSamples("dispatch.py", "dispatch", []byte(source), 4)
	if len(matches) != 1 || matches[0].Quote != "dispatch()" || matches[0].StartLine != 2 {
		t.Fatalf("oversize context or partial line: %+v", matches)
	}
}
