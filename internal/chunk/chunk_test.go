// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// diffText builds a unified diff with the given number of hunks, each of the
// given body size. A generated diff keeps the tests about splitting rather than
// about hand-written fixtures.
func diffText(hunks, bodyLines int) string {
	var b strings.Builder
	b.WriteString("diff --git a/pkg/x.go b/pkg/x.go\nindex 111..222 100644\n--- a/pkg/x.go\n+++ b/pkg/x.go\n")
	for h := 0; h < hunks; h++ {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@ func handler()\n", h*100+1, bodyLines, h*100+1, bodyLines)
		for i := 0; i < bodyLines; i++ {
			fmt.Fprintf(&b, "+changed line %d of hunk %d with some payload text\n", i, h)
		}
	}
	return b.String()
}

func TestSplit_SmallDiffStaysOneChunk(t *testing.T) {
	text := diffText(1, 5)
	chunks := Split("pkg/x.go", text, Options{Budget: 100000})

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for a diff inside the budget, got %d", len(chunks))
	}
	if chunks[0].Kind != KindFile {
		t.Errorf("Kind = %q, want %q", chunks[0].Kind, KindFile)
	}
	if chunks[0].Text != text {
		t.Error("a whole-file chunk must carry the diff verbatim")
	}
	if chunks[0].Index != 0 || chunks[0].Total != 1 {
		t.Errorf("Index/Total = %d/%d, want 0/1", chunks[0].Index, chunks[0].Total)
	}
}

func TestSplit_IsDeterministic(t *testing.T) {
	text := diffText(6, 20)
	opts := Options{Budget: 400}

	first := Split("pkg/x.go", text, opts)
	if len(first) < 2 {
		t.Fatalf("fixture should split, got %d chunk(s)", len(first))
	}
	second := Split("pkg/x.go", text, opts)

	if len(first) != len(second) {
		t.Fatalf("chunk count differs between runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("chunk %d id differs between runs: %s vs %s", i, first[i].ID, second[i].ID)
		}
		if first[i].Text != second[i].Text {
			t.Errorf("chunk %d content differs between runs", i)
		}
	}
}

func TestSplit_IdentityTracksContentAndRange(t *testing.T) {
	text := diffText(4, 10)
	opts := Options{Budget: 300}
	base := Split("pkg/x.go", text, opts)
	if len(base) < 2 {
		t.Fatalf("fixture should split, got %d chunk(s)", len(base))
	}

	// A different path must not collide with the same range.
	other := Split("pkg/y.go", text, opts)
	if other[0].ID == base[0].ID {
		t.Error("chunk id must include the path")
	}

	// Same range, different content must not collide either.
	altered := strings.Replace(text, "changed line 0 of hunk 0", "CHANGED line 0 of hunk 0", 1)
	changed := Split("pkg/x.go", altered, opts)
	same := 0
	for i := range base {
		if i < len(changed) && base[i].ID == changed[i].ID {
			same++
		}
	}
	if same == len(base) {
		t.Error("chunk id must include the content: editing one line left every id unchanged")
	}
}

// covered is the coverage property: every line of the source diff appears in at
// least one chunk. It is the invariant the whole splitter exists to protect -
// a bounded split that loses a line is a silently truncated review.
func covered(t *testing.T, text string, chunks []Chunk) {
	t.Helper()
	src := strings.Split(text, "\n")
	if n := len(src); n > 1 && src[n-1] == "" {
		src = src[:n-1] // trailing newline terminator, not a line of the change
	}
	seen := make([]bool, len(src))
	for _, c := range chunks {
		for i := c.StartLine; i <= c.EndLine; i++ {
			if i >= 1 && i <= len(src) {
				seen[i-1] = true
			}
		}
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("line %d is not covered by any chunk", i+1)
		}
	}
}

func TestSplit_MultiHunkFileKeepsEveryLine(t *testing.T) {
	text := diffText(8, 15)
	chunks := Split("pkg/x.go", text, Options{Budget: 500})
	if len(chunks) < 2 {
		t.Fatalf("fixture should split, got %d chunk(s)", len(chunks))
	}
	covered(t, text, chunks)
	for i, c := range chunks {
		if c.Tokens > 500 && !c.Oversized {
			t.Errorf("chunk %d is %d tokens, over the 500 budget and not flagged", i, c.Tokens)
		}
	}
}

func TestSplit_GiantHunkFallsBackToWindows(t *testing.T) {
	// One hunk far larger than the budget: the fallback must window it.
	text := diffText(1, 400)
	chunks := Split("pkg/x.go", text, Options{Budget: 400})
	if len(chunks) < 2 {
		t.Fatalf("fixture should window, got %d chunk(s)", len(chunks))
	}
	// A leading git header may need its own chunk when merging it would push
	// the first window over budget; every window after it is one.
	windows := 0
	for i, c := range chunks {
		if c.Kind == KindWindow {
			windows++
			continue
		}
		if i != 0 {
			t.Errorf("chunk %d Kind = %q: only a leading header chunk may not be a window", i, c.Kind)
		}
	}
	if windows < 2 {
		t.Errorf("expected several windows, got %d", windows)
	}
	covered(t, text, chunks)

	// Consecutive windows overlap, which is what keeps a construct straddling a
	// border interpretable.
	overlapped := false
	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartLine <= chunks[i-1].EndLine {
			overlapped = true
		}
	}
	if !overlapped {
		t.Error("consecutive windows should overlap by the configured window")
	}
}

func TestSplit_SingleLineOverBudgetIsKeptWholeAndFlagged(t *testing.T) {
	// One line whose own token count exceeds the budget: overlap cannot help,
	// and cutting it would lose content. It is served whole and reported.
	huge := "+" + strings.Repeat("x ", 4000)
	text := "diff --git a/a.go b/a.go\n@@ -1,1 +1,1 @@\n" + huge + "\n"
	chunks := Split("a.go", text, Options{Budget: 256})

	oversized := 0
	var body *Chunk
	for i := range chunks {
		if chunks[i].Oversized {
			oversized++
			body = &chunks[i]
		}
	}
	if oversized != 1 {
		t.Fatalf("expected exactly 1 oversized chunk, got %d of %d", oversized, len(chunks))
	}
	if !strings.HasPrefix(body.Text, "+x x x") {
		t.Error("an oversized chunk must still carry its content: nothing may be truncated")
	}
	covered(t, text, chunks)
}

func TestSplit_EmptyAndDegenerateInputs(t *testing.T) {
	if got := Split("a.go", "", Options{Budget: 1000}); len(got) != 1 || got[0].Text != "" {
		t.Errorf("empty diff should produce one empty chunk, got %#v", got)
	}
	// No hunk header at all: the whole text is preamble, and it must still be
	// delivered rather than dropped.
	text := "diff --git a/a.go b/a.go\nBinary files differ\n"
	got := Split("a.go", text, Options{Budget: 256})
	if len(got) != 1 || !strings.Contains(got[0].Text, "Binary files differ") {
		t.Errorf("a hunk-less diff must be delivered whole, got %#v", got)
	}
}

func TestSplit_ChunkTokensAreMeasured(t *testing.T) {
	text := diffText(3, 10)
	chunks := Split("pkg/x.go", text, Options{Budget: 400})
	for i, c := range chunks {
		if want := llm.CountTokens(c.Text); c.Tokens != want {
			t.Errorf("chunk %d Tokens = %d, want measured %d", i, c.Tokens, want)
		}
	}
}
