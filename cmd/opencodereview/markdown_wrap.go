// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"

	glamouransi "charm.land/glamour/v2/ansi"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
	"github.com/yuin/goldmark/ast"
)

func layoutMarkdownProse(node ast.Node, source []byte, terminal *glamouransi.ANSIRenderer, context glamouransi.RenderContext, width int) (string, error) {
	inline, err := renderMarkdownInlines(node, source, terminal, context)
	if err != nil {
		return "", err
	}
	listItem := node.Parent().Kind() == ast.KindListItem
	var indent string
	if listItem {
		var marker strings.Builder
		if err := terminal.NewElement(node.Parent(), source).Renderer.Render(&marker, context); err != nil {
			return "", err
		}
		indent = strings.Repeat(" ", ansi.StringWidth(marker.String()))
	}
	layout := inline.wrap(max(1, width-len(indent)))
	if listItem {
		layout = strings.ReplaceAll(layout, "\n", "\n"+indent)
		if node.Kind() == ast.KindParagraph && node.PreviousSibling() != nil {
			layout = "\n\n" + indent + layout
		}
		return layout, nil
	}
	if layout == inline.styled {
		return "", nil
	}
	if node.PreviousSibling() != nil {
		layout = "\n" + layout
	}
	return layout + "\n", nil
}

const codeSpanBoundary = "\x00"

type byteRange struct{ start, end int }

type markdownInline struct {
	styled string
	code   []byteRange
}

func parseMarkdownInline(marked string) markdownInline {
	if !strings.Contains(marked, codeSpanBoundary) {
		return markdownInline{styled: marked}
	}
	var inline markdownInline
	var styled strings.Builder
	var offset int
	var literal bool
	for part := range strings.SplitSeq(marked, codeSpanBoundary) {
		end := offset + len(ansi.Strip(part))
		if literal {
			inline.code = append(inline.code, byteRange{offset, end})
		}
		styled.WriteString(part)
		offset, literal = end, !literal
	}
	inline.styled = styled.String()
	return inline
}

func (inline markdownInline) wrap(width int) string {
	if ansi.StringWidth(inline.styled) <= width {
		return inline.styled
	}
	plain := ansi.Strip(inline.styled)
	var breaks []byteRange
	var column, segmentStart, pendingSpaces, protectedEnd int
	code := inline.code
	clusters := uniseg.NewGraphemes(plain)
	for clusters.Next() {
		_, end := clusters.Positions()
		for len(code) > 0 && code[0].end <= end {
			protectedEnd, code = code[0].end, code[1:]
		}
		if clusters.LineBreak() == uniseg.LineDontBreak || (len(code) > 0 && code[0].start < end) {
			continue
		}
		segment := strings.TrimSuffix(plain[segmentStart:end], "\n")
		wordEnd := max(min(protectedEnd-segmentStart, len(segment)), len(strings.TrimRight(segment, " ")))
		word := segment[:wordEnd]
		visible := ansi.StringWidth(word)
		if column > 0 && column+pendingSpaces+visible > width {
			breaks = append(breaks, byteRange{segmentStart - pendingSpaces, segmentStart})
			column, pendingSpaces = 0, 0
		}
		column += pendingSpaces
		if visible > width {
			letters := uniseg.NewGraphemes(word)
			for letters.Next() {
				size := ansi.StringWidth(letters.Str())
				if column > 0 && column+size > width {
					start, _ := letters.Positions()
					offset := segmentStart + start
					breaks = append(breaks, byteRange{offset, offset})
					column = 0
				}
				column += size
			}
		} else {
			column += visible
		}
		pendingSpaces, segmentStart = len(segment)-wordEnd, end
		if clusters.LineBreak() == uniseg.LineMustBreak {
			column, pendingSpaces = 0, 0
		}
	}
	return applyMarkdownBreaks(inline.styled, breaks)
}

func applyMarkdownBreaks(styled string, breaks []byteRange) string {
	if len(breaks) == 0 {
		return styled
	}
	var wrapped strings.Builder
	var offset int
	remaining := styled
	for _, gap := range breaks {
		for offset < gap.end {
			if len(remaining) == 0 {
				return styled
			}
			if remaining[0] == '\x1b' {
				seq, _, size, _ := ansi.DecodeSequence(remaining, 0, nil)
				wrapped.WriteString(seq)
				remaining = remaining[size:]
				continue
			}
			if offset < gap.start {
				wrapped.WriteByte(remaining[0])
			}
			remaining = remaining[1:]
			offset++
		}
		wrapped.WriteByte('\n')
	}
	wrapped.WriteString(remaining)
	return wrapped.String()
}
