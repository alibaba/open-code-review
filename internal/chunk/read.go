// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import "strings"

// Read rendering. Every read returns a body bounded by the budget and names
// whatever it did not serve, so a bounded read is never mistaken for the whole
// file. Nothing here truncates content: a chunk that is not served is not
// served at all.

const readNoticeLimit = 10

// Fetch serves one chunk by id and marks it read. An unknown id returns a
// message naming the miss rather than an error, matching how the other read
// tools report: the model can correct itself from the text.
func (s *Store) Fetch(id string) string {
	c, ok := s.Chunk(id)
	if !ok {
		return "Error: unknown chunk id \"" + id + "\". Use file_read_diff {\"path\": \"<file>\"} to list the chunk ids of a file."
	}
	s.RecordFetch(id)
	return chunkBody(c)
}

// FetchPaths serves the chunks of several paths, in order, stopping when the
// next chunk would exceed budget. Remaining ids are named at the end. Each
// served chunk is recorded as read.
func (s *Store) FetchPaths(paths []string, budget int) string {
	return s.servePaths(paths, budget, true)
}

// RenderPaths serves the same bounded content as FetchPaths without recording
// the reads. It is how a prompt is handed context it did not ask for - the
// review filter reading the chunks its candidate comments point at - so the
// run's coverage accounting keeps meaning "the model read this".
func (s *Store) RenderPaths(paths []string, budget int) string {
	return s.servePaths(paths, budget, false)
}

func (s *Store) servePaths(paths []string, budget int, record bool) string {
	if budget <= 0 {
		return "Error: the context read budget is exhausted; no diff content was served."
	}
	var b strings.Builder
	served, remaining := 0, 0
	var firstRemaining []string
	used := 0
	seen := make(map[string]bool)

	for _, p := range paths {
		for _, c := range s.ChunksForPath(p) {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			body := chunkBody(c)
			// An oversized chunk is served whole: it is the one unit that
			// cannot be shrunk, and cutting it would lose content.
			if served > 0 && used+c.Tokens > budget {
				remaining++
				firstRemaining = appendBounded(firstRemaining, c.ID)
				continue
			}
			if record {
				s.RecordFetch(c.ID)
			}
			b.WriteString(body)
			used += c.Tokens
			served++
		}
	}

	if remaining == 0 {
		if served == 0 {
			return "Error: diff not found for the requested paths"
		}
		return b.String()
	}

	if served == 0 {
		return "Error: the requested diff exceeds the read budget of " + itoa(budget) +
			" tokens; fetch it with file_read_diff {\"chunk_id\": \"...\"}. First chunk ids: " +
			strings.Join(firstRemaining, ", ")
	}

	b.WriteString(renderReadNotice(served, remaining, firstRemaining, budget))
	return b.String()
}

// ListPath renders the chunk index of one path: identity, placement and cost,
// without content. It is how the model discovers the ids of a file whose chunks
// the manifest could not afford to list.
func (s *Store) ListPath(path string) string {
	chunks := s.ChunksForPath(path)
	if len(chunks) == 0 {
		return "Error: no chunk registered for path \"" + path + "\""
	}
	var b strings.Builder
	b.WriteString("<chunk_index path=\"" + escapeAttr(path) + "\" count=\"" + itoa(len(chunks)) + "\">\n")
	for _, c := range chunks {
		b.WriteString(IndexLine(c))
		b.WriteString("\n")
	}
	b.WriteString("</chunk_index>\n")
	return b.String()
}

// chunkBody renders one chunk with its identity attached, so a payload that is
// later replaced by a receipt can still be attributed to a unit of work.
func chunkBody(c Chunk) string {
	var b strings.Builder
	b.WriteString("==== CHUNK ")
	b.WriteString(c.ID)
	b.WriteString(" FILE: ")
	b.WriteString(c.Path)
	b.WriteString(" LINES: ")
	b.WriteString(itoa(c.StartLine))
	b.WriteString("-")
	b.WriteString(itoa(c.EndLine))
	b.WriteString(" TOKENS: ")
	b.WriteString(itoa(c.Tokens))
	if c.Oversized {
		b.WriteString(" NOTE: single line over budget, served whole")
	}
	b.WriteString(" ====\n")
	b.WriteString(c.Text)
	b.WriteString("\n")
	return b.String()
}

// renderReadNotice states what was served and what was not. It names a bounded
// prefix of the omitted ids and the total count, because an unbounded list of
// omitted ids would reintroduce the very blow-up the budget exists to prevent.
func renderReadNotice(served, remaining int, firstRemaining []string, budget int) string {
	var b strings.Builder
	b.WriteString("\n[context read bounded] ")
	b.WriteString(itoa(served))
	b.WriteString(" chunk(s) served, ")
	b.WriteString(itoa(remaining))
	b.WriteString(" not served within the ")
	b.WriteString(itoa(budget))
	b.WriteString("-token read budget.")
	if len(firstRemaining) > 0 {
		b.WriteString(" Not served: ")
		b.WriteString(strings.Join(firstRemaining, ", "))
		if remaining > len(firstRemaining) {
			b.WriteString(", and ")
			b.WriteString(itoa(remaining - len(firstRemaining)))
			b.WriteString(" more")
		}
		b.WriteString(".")
	}
	b.WriteString(" The diff is NOT fully read; fetch the rest with file_read_diff {\"chunk_id\": \"...\"}.")
	b.WriteString("\n")
	return b.String()
}

func appendBounded(list []string, id string) []string {
	if len(list) < readNoticeLimit {
		return append(list, id)
	}
	return list
}
