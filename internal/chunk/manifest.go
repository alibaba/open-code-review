// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

import "strings"

// Manifest is the review context of one group: the ordered chunks the model
// must read, plus how many packs they fall into. It replaces an inlined diff
// in the first user message, so the frozen prefix of the conversation stays
// small and its content independent of diff size.
type Manifest struct {
	// Group is the review unit key.
	Group string
	// PackCount is how many reads the chunks fall into at the current budget.
	// It is a shape hint for the model, not a schedule it must follow.
	PackCount int
	// Files are the distinct paths, in manifest order.
	Files []string
	// Chunks are the units of work, in reading order.
	Chunks []Chunk
	// TotalTokens is the token count of every chunk combined.
	TotalTokens int
}

// NewManifest builds a manifest from a group's registered chunks, keeping the
// order the store produced (path order, then line order within a path).
func NewManifest(group string, chunks []Chunk, packCount int) Manifest {
	m := Manifest{Group: group, PackCount: packCount, Chunks: chunks}
	seen := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		m.TotalTokens += c.Tokens
		if !seen[c.Path] {
			seen[c.Path] = true
			m.Files = append(m.Files, c.Path)
		}
	}
	return m
}

// Render produces the bounded manifest text placed in the first user message.
//
// The render degrades in two steps as it approaches budget, so its size is
// bounded without ever dropping a file silently: per-file rows survive the
// per-chunk rows, and anything not rendered is named in a closing note with
// the tool call that reaches it.
func RenderManifest(m Manifest, budget int) string {
	var b strings.Builder
	b.WriteString("<context_manifest")
	b.WriteString(` chunk_count="`)
	b.WriteString(itoa(len(m.Chunks)))
	b.WriteString(`" file_count="`)
	b.WriteString(itoa(len(m.Files)))
	b.WriteString(`" pack_count="`)
	b.WriteString(itoa(m.PackCount))
	b.WriteString(`" chunk_budget="`)
	b.WriteString(itoa(budget))
	b.WriteString("\">\n")

	byPath := make(map[string][]Chunk, len(m.Files))
	for _, c := range m.Chunks {
		byPath[c.Path] = append(byPath[c.Path], c)
	}

	used := countTokens(b.String())
	var deferred []Chunk
	for _, path := range m.Files {
		header := "  <file path=\"" + escapeAttr(path) + "\" chunks=\"" + itoa(len(byPath[path])) + "\""
		header += " tokens=\"" + itoa(pathTokens(byPath[path])) + "\">\n"
		if used+countTokens(header) > budget {
			deferred = append(deferred, byPath[path]...)
			continue
		}
		used += countTokens(header)
		b.WriteString(header)
		for _, c := range byPath[path] {
			line := IndexLine(c) + "\n"
			if used+countTokens(line) > budget {
				deferred = append(deferred, c)
				continue
			}
			used += countTokens(line)
			b.WriteString(line)
		}
		b.WriteString("  </file>\n")
	}

	b.WriteString(renderDeferred(deferred))
	b.WriteString("</context_manifest>\n")
	return b.String()
}

// renderDeferred names the chunks the budget kept out of the manifest. The
// model is told exactly what it is missing and how to reach it, because a
// manifest that silently omits work would read as full coverage.
func renderDeferred(deferred []Chunk) string {
	if len(deferred) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("  <deferred note=\"")
	b.WriteString(itoa(len(deferred)))
	b.WriteString(` chunk(s) omitted to stay within the manifest budget; this is NOT full coverage.">` + "\n")
	for i, c := range deferred {
		if i >= 10 {
			b.WriteString("    ... and ")
			b.WriteString(itoa(len(deferred) - i))
			b.WriteString(" more, listed per file by file_read_diff {\"path\": \"")
			b.WriteString(escapeAttr(c.Path))
			b.WriteString("\"}\n")
			break
		}
		b.WriteString(IndexLine(c))
		b.WriteString("\n")
	}
	b.WriteString("  </deferred>\n")
	return b.String()
}

func pathTokens(chunks []Chunk) int {
	n := 0
	for _, c := range chunks {
		n += c.Tokens
	}
	return n
}
