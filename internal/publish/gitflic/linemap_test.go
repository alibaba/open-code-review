package gitflic

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/diff"
)

// sampleDiff: old file lines 1..10; line 3 modified, a line inserted after
// old line 5, old line 8 deleted.
const sampleDiff = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,10 +1,10 @@
 line1
 line2
-line3 old
+line3 new
 line4
 line5
+inserted after5
 line6
 line7
-line8
 line9
 line10
`

func TestOldLineFor(t *testing.T) {
	hunks := diff.ParseHunks(sampleDiff)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	cases := []struct {
		name    string
		newLine int
		want    int
	}{
		{"context before changes", 1, 1},
		{"modified line maps to deleted position anchor", 3, 3},
		{"context after modification", 4, 4},
		{"added line anchors to preceding old line", 6, 5},
		{"context shifted by insertion", 7, 6},
		{"context after deletion", 9, 9},
		{"last context line", 10, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oldLineFor(hunks, tc.newLine); got != tc.want {
				t.Errorf("oldLineFor(%d) = %d, want %d", tc.newLine, got, tc.want)
			}
		})
	}
}

func TestOldLineForOutsideHunks(t *testing.T) {
	hunks := diff.ParseHunks(sampleDiff)

	// Lines after the hunk shift by the cumulative delta (here: 0, since
	// one line was added and one deleted).
	if got := oldLineFor(hunks, 42); got != 42 {
		t.Errorf("oldLineFor(42) = %d, want 42", got)
	}
}

func TestOldLineForMultipleHunks(t *testing.T) {
	const multiHunk = `@@ -1,2 +1,4 @@
 line1
+added2
+added3
 line2
@@ -10,3 +12,3 @@
 line10
-line11 old
+line11 new
 line12
`
	hunks := diff.ParseHunks(multiHunk)
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}

	// Between hunks: new line 8 = old line 6 (two lines added by hunk 1).
	if got := oldLineFor(hunks, 8); got != 6 {
		t.Errorf("oldLineFor(8) = %d, want 6", got)
	}
	// Inside second hunk: modified new line 13 anchors to old line 11.
	if got := oldLineFor(hunks, 13); got != 11 {
		t.Errorf("oldLineFor(13) = %d, want 11", got)
	}
}

func TestOldLineForPureAdditionAtTop(t *testing.T) {
	const topAddition = `@@ -0,0 +1,2 @@
+first
+second
`
	hunks := diff.ParseHunks(topAddition)
	// No preceding old line exists; result is clamped to 1.
	if got := oldLineFor(hunks, 1); got != 1 {
		t.Errorf("oldLineFor(1) = %d, want 1", got)
	}
}
