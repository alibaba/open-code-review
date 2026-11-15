// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import "strings"

// OldContent rebuilds the version of a file before rawDiff from its new
// content by reversing the diff's hunks. ok is false when the hunks do not
// match newContent, so a caller can treat an unverifiable previous version
// as unknown instead of trusting a wrong reconstruction.
func OldContent(newContent, rawDiff string) (old string, ok bool) {
	newLines := strings.Split(newContent, "\n")
	out := make([]string, 0, len(newLines))
	next := 0
	for _, h := range ParseHunks(rawDiff) {
		// A hunk that adds no lines names the line it follows, not the first
		// line it covers.
		start := h.NewStart - 1
		if h.NewCount == 0 {
			start = h.NewStart
		}
		if start < next || start > len(newLines) {
			return "", false
		}
		out = append(out, newLines[next:start]...)
		next = start
		// The header's counts bound the hunk: ParseHunks reads any trailing
		// text, such as the empty line after a final newline, as context.
		oldLeft, newLeft := h.OldCount, h.NewCount
		for _, l := range h.Lines {
			if oldLeft == 0 && newLeft == 0 {
				break
			}
			if l.Type != HunkAdded {
				oldLeft--
			}
			if l.Type != HunkDeleted {
				newLeft--
			}
			if oldLeft < 0 || newLeft < 0 {
				return "", false
			}
			if l.Type == HunkDeleted {
				out = append(out, l.Content)
				continue
			}
			if next >= len(newLines) || strings.TrimSuffix(newLines[next], "\r") != strings.TrimSuffix(l.Content, "\r") {
				return "", false
			}
			if l.Type == HunkContext {
				out = append(out, newLines[next])
			}
			next++
		}
		if oldLeft != 0 || newLeft != 0 {
			return "", false
		}
	}
	out = append(out, newLines[next:]...)
	return strings.Join(out, "\n"), true
}
