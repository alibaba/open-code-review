// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import (
	"sort"
	"strings"
	"sync"
)

// State is the coverage state of one chunk. The four states answer the only
// question the run can honestly answer: was this unit of context put in front
// of the model, and if not, why not.
type State int

const (
	// StateUnfetched means the chunk was offered in the manifest and never
	// read. It is not a failure of the run - the model chose its order - but it
	// is reported, because a chunk nobody read is a chunk nobody reviewed.
	StateUnfetched State = iota
	// StateFetched means the chunk content was served to the model at least
	// once. It does not claim the model reasoned about it.
	StateFetched
	// StateFailed means the chunk was requested but could not be served.
	StateFailed
	// StateSkipped means the model declared the chunk intentionally skipped,
	// with a structured cause.
	StateSkipped
)

// String names the state for reports and prompts.
func (s State) String() string {
	switch s {
	case StateFetched:
		return "fetched"
	case StateFailed:
		return "failed"
	case StateSkipped:
		return "skipped"
	default:
		return "unfetched"
	}
}

// entry is one registered chunk plus its coverage bookkeeping. GroupKey scopes
// coverage to the review unit the chunk belongs to, so two groups reviewing
// concurrently never mark each other's context.
type entry struct {
	chunk     Chunk
	group     string
	state     State
	fetches   int
	skipCause string
}

// Pack is a set of chunks that fit together in one read budget. Packing is what
// keeps small, strongly related files together instead of shredding them one
// chunk per tool call.
type Pack struct {
	Index  int
	Tokens int
	Chunks []int // indices into the store's registration order
}

// Store holds the chunks of one run and their coverage. It is the only shared
// state between the agent (which registers the manifest) and the read tool
// (which serves and counts the reads), and it is safe for concurrent use
// because groups are dispatched in parallel.
type Store struct {
	budget int

	mu       sync.RWMutex
	order    []string // chunk ids, in registration order
	entries  map[string]*entry
	byPath   map[string][]string
	byGroup  map[string][]string
	groupPks map[string]int
	fetched  int
	refetch  int
	// fallback resolves a path that was never registered, used to chunk a
	// diff on first read instead of reporting it as missing. Nil disables it.
	fallback func(path string) (string, bool)
}

// NewStore returns an empty Store bounded by budget. A non-positive budget is
// stored as-is; Options.Budget normalizes it at split time.
func NewStore(budget int) *Store {
	return &Store{
		budget:   budget,
		entries:  make(map[string]*entry),
		byPath:   make(map[string][]string),
		byGroup:  make(map[string][]string),
		groupPks: make(map[string]int),
	}
}

// SetFallback installs the resolver used for paths the run never registered as
// chunks - typically a file whose diff exists in the run's snapshot but whose
// group was delivered inline, or a file outside every reviewed group.
//
// The alternative is reporting such a path as missing, which would be a lie:
// the diff is there, and the only reason it has no chunks is that nobody
// needed them until now.
func (s *Store) SetFallback(fn func(path string) (string, bool)) {
	s.mu.Lock()
	s.fallback = fn
	s.mu.Unlock()
}

// Add registers one file's chunks under a group and returns the ids in order.
// Registering the same id twice is a no-op, so a group replayed after a
// resume does not double-count.
func (s *Store) Add(group, path string, chunks []Chunk) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.addLocked(group, path, chunks)
	s.groupPks[group] = s.packIndicesLocked(s.byGroup[group])
	return ids
}

// addLocked is Add without the lock and without the pack recount.
func (s *Store) addLocked(group, path string, chunks []Chunk) []string {
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if _, seen := s.entries[c.ID]; seen {
			ids = append(ids, c.ID)
			continue
		}
		s.entries[c.ID] = &entry{chunk: c, group: group}
		s.order = append(s.order, c.ID)
		s.byPath[path] = append(s.byPath[path], c.ID)
		s.byGroup[group] = append(s.byGroup[group], c.ID)
		ids = append(ids, c.ID)
	}
	return ids
}

// packIndicesLocked counts the packs a group's chunks fall into, under the
// store budget. Called with the write lock held; Packs is the reader-facing
// form of the same arithmetic.
func (s *Store) packIndicesLocked(ids []string) int {
	packs := 0
	cur := 0
	for _, id := range ids {
		t := s.entries[id].chunk.Tokens
		if cur > 0 && cur+t > s.budget {
			packs++
			cur = 0
		}
		cur += t
	}
	if cur > 0 || len(ids) == 0 {
		packs++
	}
	return packs
}

// PackCount returns how many packs a group falls into.
func (s *Store) PackCount(group string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n, ok := s.groupPks[group]; ok {
		return n
	}
	return 0
}

// Packs returns the packs of a group, in order. Pack membership is derived, not
// stored: the same budget and the same chunk list always produce the same
// packs.
func (s *Store) Packs(group string) []Pack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.byGroup[group]
	out := make([]Pack, 0, 1)
	cur := Pack{Index: len(out)}
	idx := make(map[string]int, len(ids))
	for i, id := range ids {
		idx[id] = i
	}
	for _, id := range ids {
		t := s.entries[id].chunk.Tokens
		if len(cur.Chunks) > 0 && cur.Tokens+t > s.budget {
			out = append(out, cur)
			cur = Pack{Index: len(out)}
		}
		cur.Tokens += t
		cur.Chunks = append(cur.Chunks, idx[id])
	}
	if len(cur.Chunks) > 0 || len(ids) == 0 {
		out = append(out, cur)
	}
	return out
}

// Chunk returns a registered chunk by id.
func (s *Store) Chunk(id string) (Chunk, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[id]
	if !ok {
		return Chunk{}, false
	}
	return e.chunk, true
}

// ChunksForPath returns a path's chunks in line order, chunking the path
// through the fallback on first access when the run never registered it.
func (s *Store) ChunksForPath(path string) []Chunk {
	ids, chunks := s.resolvePath(path)
	if chunks != nil {
		return chunks
	}
	return s.chunksOfIDs(ids)
}

// resolvePath returns the registered ids of a path, chunking it through the
// fallback when it has none. The second return value is non-nil when the answer
// came from a cache the caller can use directly.
func (s *Store) resolvePath(path string) ([]string, []Chunk) {
	s.mu.RLock()
	ids, ok := s.byPath[path]
	fallback := s.fallback
	s.mu.RUnlock()
	if ok && len(ids) > 0 {
		return ids, nil
	}
	if fallback == nil {
		return ids, nil
	}
	text, found := fallback(path)
	if !found {
		return nil, nil
	}
	// Chunking happens outside the lock: it tokenizes the whole diff, and
	// holding the write lock across that would serialize every concurrent
	// reader in the run behind it.
	cs := Split(path, text, Options{Budget: s.budget})
	s.mu.Lock()
	if existing, ok := s.byPath[path]; ok && len(existing) > 0 {
		// Another reader got here first; its ids are the same ones, because
		// chunk identity is derived from path, range and content.
		s.mu.Unlock()
		return existing, nil
	}
	s.addLocked("", path, cs)
	ids = s.byPath[path]
	s.mu.Unlock()
	return ids, nil
}

// chunksOfIDs materializes the chunks of a known id list.
func (s *Store) chunksOfIDs(ids []string) []Chunk {
	if len(ids) == 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Chunk, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.entries[id].chunk)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartLine < out[j].StartLine })
	return out
}

// RecordFetch marks a chunk as served and returns whether this was the first
// read. A second read of the same chunk is an explicit refetch: visible, and
// counted separately from the first send.
func (s *Store) RecordFetch(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return false
	}
	e.fetches++
	s.fetched++
	if e.fetches > 1 {
		s.refetch++
	}
	if e.state != StateSkipped {
		e.state = StateFetched
	}
	return e.fetches == 1
}

// MarkFetchFailed records that a chunk could not be served. It never demotes a
// chunk that was already served: a chunk the model has read is read, whatever
// a later read of it returned.
func (s *Store) MarkFetchFailed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[id]; ok && e.state == StateUnfetched {
		e.state = StateFailed
	}
}

// Stats is the run's context accounting: what was offered, what was sent, and
// what never was.
type Stats struct {
	// UniqueChunks is the number of distinct chunks in the manifest.
	UniqueChunks int
	// FetchCount is the number of chunk reads served, first and repeat.
	FetchCount int
	// RefetchCount is the number of reads beyond the first for a chunk.
	RefetchCount int
	// RawTokens is the total token count of all registered chunk content,
	// counted once per chunk however often it was read.
	RawTokens int
	// ResendTokens is the token count of chunks that were read more than once.
	// A nominal run leaves it at zero: the ticket's economic criterion is one
	// raw send per chunk, and anything above zero is an explicit refetch the
	// model asked for.
	ResendTokens int
	// Uninspected lists the chunks that were never fetched, in registration
	// order. Empty means every chunk was at least put in front of the model.
	Uninspected []string
	// Oversized lists the chunks that exceed the budget on their own, so the
	// one case where the bound cannot hold is named rather than hidden.
	Oversized []string
}

// Stats returns the run's context accounting.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{FetchCount: s.fetched, RefetchCount: s.refetch, UniqueChunks: len(s.order)}
	for _, id := range s.order {
		e := s.entries[id]
		st.RawTokens += e.chunk.Tokens
		if e.fetches > 1 {
			st.ResendTokens += e.chunk.Tokens
		}
		if e.chunk.Oversized {
			st.Oversized = append(st.Oversized, id)
		}
		if e.state == StateUnfetched {
			st.Uninspected = append(st.Uninspected, id)
		}
	}
	return st
}

// Coverage reports the state of one chunk.
func (s *Store) Coverage(id string) (State, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[id]
	if !ok {
		return StateUnfetched, false
	}
	return e.state, true
}

// GroupStats is Stats restricted to one group, so a caller can fold each
// group's slice into run-wide counters exactly once.
func (s *Store) GroupStats(group string) Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{}
	for _, id := range s.byGroup[group] {
		e := s.entries[id]
		st.UniqueChunks++
		st.FetchCount += e.fetches
		if e.fetches > 1 {
			st.RefetchCount += e.fetches - 1
			st.ResendTokens += e.chunk.Tokens
		}
		st.RawTokens += e.chunk.Tokens
		if e.chunk.Oversized {
			st.Oversized = append(st.Oversized, id)
		}
		if e.state == StateUnfetched {
			st.Uninspected = append(st.Uninspected, id)
		}
	}
	return st
}

// ChunksOfGroup returns a group's chunks in registration order.
func (s *Store) ChunksOfGroup(group string) []Chunk {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.byGroup[group]
	out := make([]Chunk, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.entries[id].chunk)
	}
	return out
}

// MarkSkippedInGroup records chunks a model declared skipped, matching each
// declaration to a chunk that group owns. A declaration naming an id from
// another group is ignored: groups are reviewed concurrently and each model
// only ever saw its own manifest.
func (s *Store) MarkSkippedInGroup(group string, ids []string, cause string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owned := make(map[string]bool, len(s.byGroup[group]))
	for _, id := range s.byGroup[group] {
		owned[id] = true
	}
	for _, id := range ids {
		if !owned[id] {
			continue
		}
		if e, ok := s.entries[id]; ok && e.state == StateUnfetched {
			e.state = StateSkipped
			e.skipCause = cause
		}
	}
}

// ResolvePath accepts either a repository-relative path or a chunk id and
// returns the chunk ids it addresses. A bare path addresses every chunk of that
// file; an id addresses exactly one. Ambiguity is impossible because ids are
// prefixed and paths are not, but a path that also happens to be a registered
// id is treated as a path - the model reaches for a path first.
func (s *Store) ResolvePath(ref string) []string {
	if _, ok := s.entries[ref]; ok {
		return []string{ref}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.byPath[ref]
	out := make([]string, len(ids))
	copy(out, ids)
	return out
}

// IndexLine renders one chunk as a bounded index row - identity and placement
// without its content. It is the unit the manifest and the path listing are
// built from.
func IndexLine(c Chunk) string {
	var b strings.Builder
	b.WriteString("    <chunk id=\"")
	b.WriteString(c.ID)
	b.WriteString("\" kind=\"")
	b.WriteString(c.Kind)
	b.WriteString("\" part=\"")
	b.WriteString(itoa(c.Index + 1))
	b.WriteString("/")
	b.WriteString(itoa(c.Total))
	b.WriteString("\" lines=\"")
	b.WriteString(itoa(c.StartLine))
	b.WriteString("-")
	b.WriteString(itoa(c.EndLine))
	b.WriteString("\" tokens=\"")
	b.WriteString(itoa(c.Tokens))
	b.WriteString("\"")
	if c.Oversized {
		b.WriteString(" note=\"single_line_over_budget\"")
	}
	if c.HunkHeader != "" {
		b.WriteString(" hunk=\"")
		b.WriteString(escapeAttr(c.HunkHeader))
		b.WriteString("\"")
	}
	b.WriteString("/>")
	return b.String()
}

func escapeAttr(v string) string {
	r := strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;")
	return r.Replace(v)
}
