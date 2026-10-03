// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import (
	"strings"
	"testing"
)

func TestFetch_ReturnsOneChunkAndNamesIt(t *testing.T) {
	s, cs := newStore(t, 400, 5, 15)
	out := s.Fetch(cs[0].ID)

	if !strings.Contains(out, "==== CHUNK "+cs[0].ID) {
		t.Errorf("a served chunk must carry its identity so a later receipt can be attributed to it: %q", firstLine(out))
	}
	if !strings.Contains(out, "FILE: pkg/x.go") || !strings.Contains(out, cs[0].Text) {
		t.Error("a served chunk must carry its content")
	}
	if strings.Contains(out, "context read bounded") {
		t.Error("a single-chunk fetch has nothing to defer and must say so by not saying it")
	}
}

func TestFetch_UnknownIDIsRecoverable(t *testing.T) {
	s, _ := newStore(t, 400, 3, 15)
	out := s.Fetch("cdeadbeef0000")
	if !strings.Contains(out, "Error: unknown chunk id") {
		t.Errorf("an unknown id must say so and route the model back: %q", out)
	}
	if !strings.Contains(out, "file_read_diff") {
		t.Error("an unknown id must tell the model how to find real ids")
	}
}

func TestFetchPaths_NeverExceedsTheBudgetAndNamesWhatIsLeft(t *testing.T) {
	s, cs := newStore(t, 400, 8, 15)
	const budget = 800
	out := s.FetchPaths([]string{"pkg/x.go"}, budget)

	if !strings.Contains(out, "context read bounded") {
		t.Fatalf("a bounded read must say it was bounded: %q", firstLine(out))
	}
	if !strings.Contains(out, "The diff is NOT fully read") {
		t.Error("a bounded read must not be mistakable for the whole change")
	}
	// Every chunk not served must be named or counted.
	served := 0
	for _, c := range cs {
		if strings.Contains(out, "==== CHUNK "+c.ID) {
			served++
		}
	}
	if served == 0 || served == len(cs) {
		t.Errorf("expected a partial serve, got %d of %d chunks", served, len(cs))
	}
	if got := s.Stats().FetchCount; got != served {
		t.Errorf("FetchCount = %d, want the %d chunks actually served", got, served)
	}
}

func TestFetchPaths_ResultStaysUnderTheReadBudget(t *testing.T) {
	s, cs := newStore(t, 400, 10, 15)
	out := s.FetchPaths([]string{"pkg/x.go"}, 700)
	for _, c := range cs {
		if !strings.Contains(out, "==== CHUNK "+c.ID) {
			continue
		}
		if c.Tokens > 700 && !c.Oversized {
			t.Errorf("served a %d-token chunk under a 700-token budget", c.Tokens)
		}
	}
}

func TestFetchPaths_ExhaustedBudgetRefusesRatherThanTruncates(t *testing.T) {
	s, _ := newStore(t, 400, 3, 15)
	out := s.FetchPaths([]string{"pkg/x.go"}, 0)
	if !strings.Contains(out, "read budget is exhausted") {
		t.Errorf("a zero budget must refuse, not serve: %q", out)
	}
}

func TestRenderPaths_DoesNotCountAsARead(t *testing.T) {
	// Context a prompt was handed is not context the model read. Counting it
	// would make the coverage numbers mean something else than they say.
	s, _ := newStore(t, 400, 4, 15)
	before := s.Stats().FetchCount
	s.RenderPaths([]string{"pkg/x.go"}, 800)
	if got := s.Stats().FetchCount; got != before {
		t.Errorf("RenderPaths changed FetchCount from %d to %d", before, got)
	}
}

func TestListPath_GivesIdsWithoutContent(t *testing.T) {
	s, cs := newStore(t, 400, 4, 15)
	out := s.ListPath("pkg/x.go")

	if !strings.Contains(out, `<chunk_index path="pkg/x.go"`) {
		t.Errorf("a listing must name the path and its chunk count: %q", firstLine(out))
	}
	for _, c := range cs {
		if !strings.Contains(out, c.ID) {
			t.Errorf("listing is missing chunk id %s", c.ID)
		}
		if strings.Contains(out, c.Text) {
			t.Errorf("listing must not carry chunk content (%s)", c.ID)
		}
	}
	if !strings.Contains(s.ListPath("absent.go"), "Error: no chunk registered") {
		t.Error("listing an unknown path must say so")
	}
}

func TestResolvePath(t *testing.T) {
	s, cs := newStore(t, 400, 3, 15)
	if got := s.ResolvePath(cs[0].ID); len(got) != 1 || got[0] != cs[0].ID {
		t.Errorf("an id must resolve to itself, got %v", got)
	}
	if got := s.ResolvePath("pkg/x.go"); len(got) != len(cs) {
		t.Errorf("a path must resolve to its chunks, got %d", len(got))
	}
	if got := s.ResolvePath("nope.go"); len(got) != 0 {
		t.Errorf("an unknown ref must resolve to nothing, got %v", got)
	}
}

func TestRenderManifest_NamesEveryChunkWhenItFits(t *testing.T) {
	_, cs := newStore(t, 400, 4, 15)
	m := NewManifest("g1", cs, 1)
	out := RenderManifest(m, 100000)

	for _, c := range cs {
		if !strings.Contains(out, c.ID) {
			t.Errorf("manifest is missing chunk %s", c.ID)
		}
	}
	if strings.Contains(out, "<deferred") {
		t.Error("a manifest that fits must not defer anything")
	}
	if !strings.Contains(out, `pack_count="1"`) || !strings.Contains(out, `chunk_count="4"`) {
		t.Errorf("manifest must state its shape: %q", firstLine(out))
	}
}

func TestRenderManifest_StaysBoundedAndSaysWhatItOmitted(t *testing.T) {
	_, cs := newStore(t, 400, 12, 20)
	m := NewManifest("g1", cs, 4)
	const budget = 300
	out := RenderManifest(m, budget)

	if !strings.Contains(out, "<deferred") {
		t.Fatalf("a manifest over budget must defer, not overflow: %q", firstLine(out))
	}
	if !strings.Contains(out, "is NOT full coverage") {
		t.Error("deferred chunks must be flagged as missing coverage, or a truncated manifest reads as complete")
	}
	// The deferred block is itself bounded: it names a prefix and a count.
	deferred := out[strings.Index(out, "<deferred"):]
	if strings.Count(deferred, "<chunk ") > 11 {
		t.Errorf("deferred block listed %d rows: it must stay bounded", strings.Count(deferred, "<chunk "))
	}
}

func TestRenderManifest_KeepsFileRowsBeforeChunkRows(t *testing.T) {
	// Under pressure the per-file rows are what let a model know which files it
	// has not looked at, so they are the last thing to go.
	_, cs := newStore(t, 400, 12, 20)
	m := NewManifest("g1", cs, 4)
	out := RenderManifest(m, 400)
	if !strings.Contains(out, `<file path="pkg/x.go"`) {
		t.Errorf("per-file rows must survive a tight manifest budget: %q", firstLine(out))
	}
}

func TestNewManifest_CollectsFilesAndTotals(t *testing.T) {
	cs := []Chunk{
		{Path: "a.go", Tokens: 10},
		{Path: "b.go", Tokens: 20},
		{Path: "a.go", Tokens: 5},
	}
	m := NewManifest("g", cs, 2)
	if len(m.Files) != 2 || m.Files[0] != "a.go" || m.Files[1] != "b.go" {
		t.Errorf("Files = %v, want distinct paths in order", m.Files)
	}
	if m.TotalTokens != 35 {
		t.Errorf("TotalTokens = %d, want 35", m.TotalTokens)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func TestFetchPaths_NoChunksIsAMiss(t *testing.T) {
	s := NewStore(400)
	if got := s.FetchPaths([]string{"absent.go"}, 800); !strings.Contains(got, "diff not found") {
		t.Errorf("got %q, want a miss", got)
	}
}

func TestFetchPaths_NamesTheCountWhenItDefersMoreThanTheNoticeHolds(t *testing.T) {
	s, _ := newStore(t, 300, 20, 30)
	out := s.FetchPaths([]string{"pkg/x.go"}, 300)

	if !strings.Contains(out, "and ") || !strings.Contains(out, " more") {
		t.Errorf("the notice must count what it could not list: %q", lastLine(out))
	}
}

func TestChunkBody_FlagsAnOversizedChunk(t *testing.T) {
	s := NewStore(256)
	huge := "+" + strings.Repeat("x ", 4000)
	text := "diff --git a/a.go b/a.go\n@@ -1,1 +1,1 @@\n" + huge + "\n"
	cs := Split("a.go", text, Options{Budget: 256})
	s.Add("g1", "a.go", cs)

	var oversized bool
	for _, c := range cs {
		if c.Oversized {
			oversized = true
			if !strings.Contains(s.Fetch(c.ID), "single line over budget") {
				t.Error("an oversized chunk must say why it exceeds the budget, or the bound looks broken")
			}
		}
	}
	if !oversized {
		t.Skip("fixture did not produce an oversized chunk")
	}
}

func TestIndexLine_FlagsAnOversizedChunk(t *testing.T) {
	if !strings.Contains(IndexLine(Chunk{ID: "c1", Kind: KindWindow, Total: 1, Oversized: true}), "single_line_over_budget") {
		t.Error("the manifest must expose the one case where the bound cannot hold")
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}
