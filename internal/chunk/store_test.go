// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import (
	"fmt"
	"strings"
	"testing"
)

func newStore(t *testing.T, budget, hunks, bodyLines int) (*Store, []Chunk) {
	t.Helper()
	text := diffText(hunks, bodyLines)
	cs := Split("pkg/x.go", text, Options{Budget: budget})
	if len(cs) < 2 {
		t.Fatalf("fixture should split into several chunks, got %d", len(cs))
	}
	s := NewStore(budget)
	s.Add("g1", "pkg/x.go", cs)
	return s, cs
}

func TestStore_FetchCountsFirstReadThenRefetch(t *testing.T) {
	s, cs := newStore(t, 400, 6, 15)

	if first := s.RecordFetch(cs[0].ID); !first {
		t.Error("the first read of a chunk must report first")
	}
	if second := s.RecordFetch(cs[0].ID); second {
		t.Error("a second read of the same chunk must not report first")
	}

	st := s.Stats()
	if st.FetchCount != 2 {
		t.Errorf("FetchCount = %d, want 2 (both reads counted)", st.FetchCount)
	}
	if st.RefetchCount != 1 {
		t.Errorf("RefetchCount = %d, want 1 (the repeat is the refetch)", st.RefetchCount)
	}
	if st.ResendTokens != cs[0].Tokens {
		t.Errorf("ResendTokens = %d, want the chunk's own cost %d: a re-read is a re-send", st.ResendTokens, cs[0].Tokens)
	}
}

func TestStore_CoverageStates(t *testing.T) {
	s, cs := newStore(t, 400, 5, 15)

	for i, c := range cs {
		want := StateUnfetched
		if i == 0 {
			s.RecordFetch(c.ID)
			want = StateFetched
		}
		if got, ok := s.Coverage(c.ID); !ok || got != want {
			t.Errorf("chunk %d coverage = %v,%v want %v", i, got, ok, want)
		}
	}

	s.MarkFetchFailed(cs[1].ID)
	if got, _ := s.Coverage(cs[1].ID); got != StateFailed {
		t.Errorf("failed read coverage = %v, want %v", got, StateFailed)
	}

	s.MarkSkippedInGroup("g1", []string{cs[2].ID}, "generated code")
	if got, _ := s.Coverage(cs[2].ID); got != StateSkipped {
		t.Errorf("declared skip coverage = %v, want %v", got, StateSkipped)
	}

	st := s.GroupStats("g1")
	if len(st.Uninspected) != len(cs)-3 {
		t.Errorf("Uninspected = %v, want %d entries", st.Uninspected, len(cs)-3)
	}
}

func TestStore_DeclaredSkipIsScopedToItsGroup(t *testing.T) {
	s, cs := newStore(t, 400, 4, 15)
	s.Add("g2", "pkg/y.go", []Chunk{cs[0]})

	// A group cannot declare chunks it never saw: groups run concurrently and
	// each model only ever read its own manifest, so a declaration naming
	// another group's chunk is a model error, not a coverage statement.
	s.MarkSkippedInGroup("g2", []string{cs[0].ID}, "not mine")
	if got, _ := s.Coverage(cs[0].ID); got == StateSkipped {
		t.Error("a declaration for another group's chunk must be ignored")
	}
}

func TestStore_DeclaredSkipCannotOverrideARead(t *testing.T) {
	s, cs := newStore(t, 400, 4, 15)
	s.RecordFetch(cs[0].ID)
	s.MarkSkippedInGroup("g1", []string{cs[0].ID}, "changed my mind")
	if got, _ := s.Coverage(cs[0].ID); got != StateFetched {
		t.Errorf("coverage = %v: a chunk that was read must not be re-declared as skipped", got)
	}
}

func TestStore_UnknownDeclarationIsIgnored(t *testing.T) {
	s, cs := newStore(t, 400, 3, 15)
	s.MarkSkippedInGroup("g1", []string{"cnosuchchunk"}, "typo")
	if len(s.Stats().Uninspected) != len(cs) {
		t.Error("an unknown id must not change coverage: a typo is not a coverage failure")
	}
}

func TestStore_PacksKeepSmallRelatedChunksTogether(t *testing.T) {
	// Four small files in one budget must land in one pack: packing is what
	// stops a group of strongly related small files from being shredded into
	// one tool call each.
	s := NewStore(100000)
	var chunks []Chunk
	for i := 0; i < 4; i++ {
		path := fmt.Sprintf("pkg/f%d.go", i)
		cs := Split(path, diffText(1, 3), Options{Budget: 100000})
		s.Add("g1", path, cs)
		chunks = append(chunks, cs...)
	}
	if got := s.PackCount("g1"); got != 1 {
		t.Errorf("PackCount = %d, want 1", got)
	}

	// And a tight budget splits them into several reads.
	tight := NewStore(40)
	for i := 0; i < 4; i++ {
		path := fmt.Sprintf("pkg/f%d.go", i)
		tight.Add("g1", path, Split(path, diffText(1, 3), Options{Budget: 40}))
	}
	if got := tight.PackCount("g1"); got < 2 {
		t.Errorf("PackCount under a tight budget = %d, want more than one read", got)
	}
}

func TestStore_PackMembershipIsDeterministic(t *testing.T) {
	build := func() *Store {
		s := NewStore(600)
		s.Add("g1", "pkg/a.go", Split("pkg/a.go", diffText(4, 12), Options{Budget: 400}))
		s.Add("g1", "pkg/b.go", Split("pkg/b.go", diffText(4, 12), Options{Budget: 400}))
		return s
	}
	first, second := build(), build()
	for i, p := range build().Packs("g1") {
		q := second.Packs("g1")[i]
		if p.Index != q.Index || p.Tokens != q.Tokens || len(p.Chunks) != len(q.Chunks) {
			t.Fatalf("pack %d differs between runs: %#v vs %#v", i, p, q)
		}
	}
	if build().PackCount("g1") != first.PackCount("g1") {
		t.Error("pack count must not depend on the run")
	}
}

func TestStore_FallbackChunksAPathOnFirstRead(t *testing.T) {
	// A path the run never registered still has a diff; reading it must chunk
	// and serve it rather than reporting it missing.
	s := NewStore(400)
	s.SetFallback(func(path string) (string, bool) {
		if path == "other.go" {
			return diffText(3, 20), true
		}
		return "", false
	})

	chunks := s.ChunksForPath("other.go")
	if len(chunks) == 0 {
		t.Fatal("fallback must produce chunks for an unregistered path")
	}
	// Second access is served from the registration the first one created.
	again := s.ChunksForPath("other.go")
	if len(again) != len(chunks) || again[0].ID != chunks[0].ID {
		t.Error("a second read must resolve to the same chunks")
	}
	if len(s.ChunksForPath("absent.go")) != 0 {
		t.Error("an unresolvable path must stay empty")
	}
}

func TestStore_ConcurrentFetchAccounting(t *testing.T) {
	s, cs := newStore(t, 400, 8, 15)
	done := make(chan struct{})
	for w := 0; w < 8; w++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < len(cs); i++ {
				s.RecordFetch(cs[i].ID)
				_ = s.ChunksForPath("pkg/x.go")
			}
		}()
	}
	for w := 0; w < 8; w++ {
		<-done
	}
	st := s.Stats()
	if st.FetchCount != 8*len(cs) {
		t.Errorf("FetchCount = %d, want %d", st.FetchCount, 8*len(cs))
	}
	if st.UniqueChunks != len(cs) {
		t.Errorf("UniqueChunks = %d, want %d", st.UniqueChunks, len(cs))
	}
}

func TestState_String(t *testing.T) {
	for state, want := range map[State]string{
		StateUnfetched: "unfetched",
		StateFetched:   "fetched",
		StateFailed:    "failed",
		StateSkipped:   "skipped",
	} {
		if got := state.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", state, got, want)
		}
	}
}

func TestIndexLine_EscapesAttributeValues(t *testing.T) {
	c := Chunk{ID: "c1", Path: "a.go", Kind: KindHunk, Total: 1, Tokens: 10,
		HunkHeader: `@@ -1 +1 @@ func "quoted" <t>`}
	line := IndexLine(c)
	if strings.Contains(line, `"quoted"`) {
		t.Error("a quote inside the hunk header must be escaped, or the manifest is not well formed")
	}
	if !strings.Contains(line, "id=\"c1\"") || !strings.Contains(line, "part=\"1/1\"") {
		t.Errorf("index row must carry identity and placement: %q", line)
	}
}

func TestOptions_FallBackToUsableDefaults(t *testing.T) {
	// A zero or negative budget must still make progress on some diff, or a
	// misconfiguration turns into an empty review.
	for _, budget := range []int{0, -1} {
		if got := (Options{Budget: budget}).budget(); got != minUsableBudget {
			t.Errorf("budget %d normalized to %d, want %d", budget, got, minUsableBudget)
		}
	}
	if got := (Options{}).overlap(); got != DefaultOverlapLines {
		t.Errorf("default overlap = %d, want %d", got, DefaultOverlapLines)
	}
	if got := (Options{OverlapLines: 3}).overlap(); got != 3 {
		t.Errorf("explicit overlap = %d, want 3", got)
	}
}

func TestStore_UnknownLookupsAreNotFailures(t *testing.T) {
	s := NewStore(400)
	if _, ok := s.Coverage("cnope"); ok {
		t.Error("Coverage of an unknown id must report absence, not a state")
	}
	if _, ok := s.Chunk("cnope"); ok {
		t.Error("Chunk of an unknown id must report absence")
	}
	if s.RecordFetch("cnope") {
		t.Error("fetching an unknown id must not report a first read")
	}
	s.MarkFetchFailed("cnope")
	if s.PackCount("nosuchgroup") != 0 {
		t.Error("an unknown group has no packs")
	}
	if got := s.Stats(); got.UniqueChunks != 0 {
		t.Errorf("an empty store reports %d chunks", got.UniqueChunks)
	}
}

func TestStore_AddIsIdempotent(t *testing.T) {
	// A group replayed after a resume re-registers its chunks; counting them
	// twice would inflate the run's context numbers.
	s := NewStore(400)
	cs := Split("a.go", diffText(3, 20), Options{Budget: 400})
	s.Add("g1", "a.go", cs)
	s.Add("g1", "a.go", cs)

	if got := s.Stats().UniqueChunks; got != len(cs) {
		t.Errorf("UniqueChunks = %d, want %d: re-registration must not double-count", got, len(cs))
	}
}

func TestStore_MarkFetchFailedDoesNotEraseAFetch(t *testing.T) {
	s, cs := newStore(t, 400, 3, 15)
	s.RecordFetch(cs[0].ID)
	s.MarkFetchFailed(cs[0].ID)
	if got, _ := s.Coverage(cs[0].ID); got != StateFetched {
		t.Errorf("coverage = %v: a chunk that was served is served, whatever a later failure says", got)
	}
}

func TestStore_PacksOfAnEmptyGroup(t *testing.T) {
	s := NewStore(400)
	packs := s.Packs("nothing")
	if len(packs) != 1 || len(packs[0].Chunks) != 0 {
		t.Errorf("an empty group must yield one empty pack, got %#v", packs)
	}
}

func TestStore_GroupStatsOfAnUnknownGroup(t *testing.T) {
	s := NewStore(400)
	if got := s.GroupStats("nothing"); got.UniqueChunks != 0 || len(got.Uninspected) != 0 {
		t.Errorf("GroupStats of an unknown group = %+v, want zeroes", got)
	}
}

func TestStore_ResolvePathWithoutAFallback(t *testing.T) {
	s := NewStore(400)
	if got := s.ResolvePath("absent.go"); len(got) != 0 {
		t.Errorf("with no fallback an unregistered path resolves to nothing, got %v", got)
	}
}
