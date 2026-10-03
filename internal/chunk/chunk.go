// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package chunk splits review context into deterministic, bounded units and
// accounts for how often each unit is actually handed to a provider.
//
// The unit of work is a Chunk: a stable identifier plus the raw text of a
// contiguous slice of one file's diff. Identity is derived from path, line
// range and content, so the same diff always yields the same ids in the same
// order - no persistence, no index, no external store.
//
// Splitting is hierarchical and never lossy. A file that fits the budget stays
// whole; a file that does not is cut at diff hunk boundaries; a single hunk
// larger than the budget is cut into line windows with a small deterministic
// overlap. Every source line is covered by at least one chunk.
package chunk

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
)

// Chunk kinds, in the order the splitter tries them. They are part of the
// chunk index the model reads, so they are stable strings.
const (
	// KindFile marks a whole file that fit the budget on its own.
	KindFile = "file"
	// KindHunk marks a run of consecutive diff hunks that fit the budget.
	KindHunk = "hunk"
	// KindWindow marks a bounded line window inside one oversized hunk.
	KindWindow = "window"
)

// DefaultOverlapLines is the window overlap used when Options.OverlapLines is
// not set. Overlap re-sends a few already-sent lines at the head of the next
// window so a construct straddling a window border stays interpretable. It is a
// constant rather than a fraction so the split stays reproducible from the
// budget alone.
const DefaultOverlapLines = 8

// minUsableBudget is the floor below which chunking cannot make progress on any
// realistic diff. A budget at or under it is treated as "context must be
// fetched in pieces as small as the budget allows", not as a licence to drop
// content.
const minUsableBudget = 256

// Chunk is one bounded unit of review context. Text holds the raw diff slice
// and is the only field that is large.
type Chunk struct {
	// ID is the stable identity: a truncated SHA-256 over path, line range and
	// content. Two runs over the same diff produce the same ID.
	ID string
	// Path is the repository-relative file path the chunk belongs to.
	Path string
	// Kind is one of KindFile, KindHunk, KindWindow.
	Kind string
	// Index and Total place the chunk within its file, 0-based.
	Index int
	Total int
	// StartLine and EndLine are 1-based, inclusive bounds inside the file's
	// diff text - they address the diff text, not the source file.
	StartLine int
	EndLine   int
	// HunkHeader is the diff hunk header the chunk opens with, when it opens
	// on one. Empty for whole-file and pure-window chunks.
	HunkHeader string
	// Tokens is the measured token count of Text.
	Tokens int
	// Oversized marks the single case a line window cannot shrink: a chunk
	// holding one source line whose own token count exceeds the budget. Such a
	// chunk is returned whole rather than cut, and is counted in the run
	// statistics. It exists so that oversized input is reported instead of
	// silently truncated.
	Oversized bool
	// Text is the raw diff slice.
	Text string
}

// Options tunes Split.
type Options struct {
	// Budget is the maximum token count of a single chunk, excluding the
	// per-chunk index overhead. Non-positive values fall back to
	// minUsableBudget.
	Budget int
	// OverlapLines is the window overlap; zero selects DefaultOverlapLines.
	OverlapLines int
}

func (o Options) budget() int {
	if o.Budget <= 0 {
		return minUsableBudget
	}
	return o.Budget
}

func (o Options) overlap() int {
	if o.OverlapLines <= 0 {
		return DefaultOverlapLines
	}
	return o.OverlapLines
}

// span is a half-open line range [start,end) of one file's diff text, plus the
// hunk header it opens with.
type span struct {
	start, end int
	header     string
	// kind is empty for a hunk run and KindWindow for a line window; a window
	// is the only span the line fallback produces, so the flag is what
	// separates it.
	kind string
	// oversized marks a window that could not be shrunk because its single
	// line already exceeds the budget.
	oversized bool
}

// Split cuts one file's diff text into chunks that each fit opts.Budget.
//
// The order is: whole file, then hunk boundaries, then bounded line windows.
// Chunks come back in ascending line order, and every line of diffText appears
// in at least one of them.
func Split(path, diffText string, opts Options) []Chunk {
	budget := opts.budget()
	lines := strings.Split(diffText, "\n")

	if countTokens(diffText) <= budget {
		return []Chunk{newChunk(path, KindFile, lines, span{start: 0, end: len(lines)}, 1, false)}
	}

	spans := splitIntoSpans(lines, budget, opts.overlap())
	chunks := make([]Chunk, 0, len(spans))
	for _, s := range spans {
		kind := KindHunk
		if s.kind != "" {
			kind = s.kind
		} else if s.header == "" {
			kind = KindFile
		}
		chunks = append(chunks, newChunk(path, kind, lines, s, len(spans), s.oversized))
	}
	// A diff ends with a newline, so splitting it yields a final empty line.
	// That is a terminator, not a line of the change: keeping it would add an
	// empty chunk to every manifest.
	if len(chunks) > 1 && chunks[len(chunks)-1].Text == "" {
		chunks = chunks[:len(chunks)-1]
	}
	for i := range chunks {
		chunks[i].Index = i
	}
	return chunks
}

// splitIntoSpans partitions a diff into budget-sized spans, preferring hunk
// boundaries and falling back to line windows for a hunk that cannot fit.
func splitIntoSpans(lines []string, budget, overlap int) []span {
	preambleEnd := hunkStart(lines)
	hunks := hunkSpans(lines)

	var spans []span
	var pending *span
	pendingTokens := 0

	flush := func() {
		if pending != nil {
			spans = append(spans, *pending)
			pending = nil
			pendingTokens = 0
		}
	}

	for _, h := range hunks {
		text := joinLines(lines, h.start, h.end)
		tokens := countTokens(text)
		if tokens > budget {
			flush()
			spans = append(spans, windowSpans(lines, h, budget, overlap)...)
			continue
		}
		if pending != nil && pendingTokens+tokens <= budget {
			pending.end = h.end
			pendingTokens += tokens
			continue
		}
		flush()
		pending = &span{start: h.start, end: h.end, header: h.header}
		pendingTokens = tokens
	}
	flush()

	// The preamble carries the "diff --git" header a model needs to interpret
	// the hunks, so it rides with the first chunk. When merging it would push
	// that chunk over budget it becomes its own chunk instead - either way no
	// line of the diff is left without a home, which is the property the whole
	// splitter is judged on.
	if preambleEnd > 0 {
		preTokens := countTokens(joinLines(lines, 0, preambleEnd))
		switch {
		case len(spans) == 0:
			spans = append(spans, span{start: 0, end: preambleEnd})
		case preTokens+countTokens(joinLines(lines, spans[0].start, spans[0].end)) <= budget:
			spans[0].start = 0
		default:
			spans = append([]span{{start: 0, end: preambleEnd}}, spans...)
		}
	}
	return spans
}

// hunkStart returns the line index of the first diff hunk header, or the end of
// the text when the diff carries no hunk at all.
func hunkStart(lines []string) int {
	for i, l := range lines {
		if isHunkHeader(l) {
			return i
		}
	}
	return len(lines)
}

func isHunkHeader(line string) bool {
	return strings.HasPrefix(line, "@@")
}

// hunkSpans splits the diff into one span per hunk. Everything before the first
// hunk header (the "diff --git" / "index" / "---" / "+++" preamble) is dropped
// from the hunk spans because splitIntoSpans attaches it to the first chunk.
func hunkSpans(lines []string) []span {
	var out []span
	start := -1
	header := ""
	for i, l := range lines {
		if isHunkHeader(l) {
			if start >= 0 {
				out = append(out, span{start: start, end: i, header: header})
			}
			start, header = i, strings.TrimSpace(l)
		}
	}
	if start >= 0 {
		out = append(out, span{start: start, end: len(lines), header: header})
	}
	return out
}

// windowSpans cuts one oversized hunk into bounded line windows with a
// deterministic overlap. A window that cannot shrink further because it holds a
// single line over the budget is marked through span.kind.
func windowSpans(lines []string, h span, budget, overlap int) []span {
	var out []span
	pos := h.start
	for pos < h.end {
		tokens := 0
		end := pos
		for end < h.end {
			lineTokens := countTokens(lines[end])
			if end > pos && tokens+lineTokens > budget {
				break
			}
			tokens += lineTokens
			end++
		}
		s := span{start: pos, end: end, kind: KindWindow}
		if pos == 0 {
			s.header = h.header
		}
		if end == pos+1 && tokens > budget {
			s.oversized = true
		}
		out = append(out, s)
		if end >= h.end {
			break
		}
		next := end - overlap
		if next <= pos {
			next = pos + 1
		}
		pos = next
	}
	return out
}

// newChunk materializes a Chunk from a span over lines.
func newChunk(path, kind string, lines []string, s span, total int, oversized bool) Chunk {
	text := joinLines(lines, s.start, s.end)
	return Chunk{
		ID:         chunkID(path, s.start, s.end, text),
		Path:       path,
		Kind:       kind,
		StartLine:  s.start + 1,
		EndLine:    s.end,
		HunkHeader: s.header,
		Tokens:     countTokens(text),
		Oversized:  oversized,
		Text:       text,
		Total:      total,
	}
}

func joinLines(lines []string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// chunkID derives the stable identity of a chunk. Content is part of the
// digest on purpose: the same range holding different code is a different
// unit of work.
func chunkID(path string, start, end int, text string) string {
	h := sha256.New()
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write([]byte(itoa(start)))
	h.Write([]byte{0})
	h.Write([]byte(itoa(end)))
	h.Write([]byte{0})
	h.Write([]byte(text))
	return "c" + hex.EncodeToString(h.Sum(nil))[:12]
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func countTokens(text string) int { return llm.CountTokens(text) }
