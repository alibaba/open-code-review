// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkParseDiffLines(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		body := strings.Repeat("-const previous = value;\n", count)
		raw := fmt.Sprintf("diff --git a/removed.go b/removed.go\ndeleted file mode 100644\n--- a/removed.go\n+++ /dev/null\n@@ -1,%d +0,0 @@\n", count) + body
		b.Run(fmt.Sprintf("files/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				diffs, err := ParseDiffText(b.Context(), raw, "", "", nil)
				if err != nil || len(diffs) != 1 || !diffs[0].IsDeleted || diffs[0].Deletions != int64(count) || diffs[0].NewPath != "/dev/null" || diffs[0].Diff != raw {
					b.Fatal("parsed diff changed")
				}
			}
		})
		b.Run(fmt.Sprintf("hunks/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				hunks := ParseHunks(raw)
				if len(hunks) != 1 || hunks[0].OldCount != count || len(hunks[0].Lines) != count+1 || hunks[0].Lines[count-1] != (HunkLine{Type: HunkDeleted, Content: "const previous = value;"}) || hunks[0].Lines[count] != (HunkLine{Type: HunkContext, Content: ""}) {
					b.Fatal("parsed hunk changed")
				}
			}
		})
	}
}
