// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"unicode"

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
	if !listItem {
		inline = strings.ReplaceAll(inline, "\n", " ")
	}
	var indent string
	if listItem && node.PreviousSibling() == nil {
		var marker strings.Builder
		if err := terminal.NewElement(node.Parent(), source).Renderer.Render(&marker, context); err != nil {
			return "", err
		}
		indent = strings.Repeat(" ", ansi.StringWidth(marker.String()))
	}
	layout := wrapCJK(inline, max(1, width-len(indent)))
	if layout == inline {
		return "", nil
	}
	if listItem {
		return strings.ReplaceAll(layout, "\n", "\n"+indent), nil
	}
	if node.PreviousSibling() != nil {
		layout = "\n" + layout
	}
	return layout + "\n", nil
}

func hasCJK(value string) bool {
	return strings.ContainsFunc(value, func(char rune) bool {
		return char >= 0x1100 && unicode.In(char, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
	})
}

func wrapCJK(styled string, width int) string {
	if !hasCJK(styled) || ansi.StringWidth(styled) <= width {
		return styled
	}
	plain := ansi.Strip(styled)
	var wrapped strings.Builder
	var start, cursor, column, segmentStart int
	clusters := uniseg.NewGraphemes(plain)
	for clusters.Next() {
		if clusters.LineBreak() == uniseg.LineDontBreak {
			continue
		}
		_, end := clusters.Positions()
		segment := plain[segmentStart:end]
		segmentStart = end
		visible := ansi.StringWidth(segment)
		spaces := len(segment) - len(strings.TrimRight(segment, " "))
		if column > 0 && column+visible-spaces > width {
			wrapped.WriteString(strings.TrimRight(styled[start:cursor], " "))
			wrapped.WriteByte('\n')
			start, column = cursor, 0
		}
		for remaining := len(segment); remaining > 0; {
			if cursor >= len(styled) {
				return styled
			}
			if styled[cursor] == '\x1b' {
				_, _, length, _ := ansi.DecodeSequence(styled[cursor:], 0, nil)
				cursor += length
			} else {
				cursor++
				remaining--
			}
		}
		column += visible
		if clusters.LineBreak() == uniseg.LineMustBreak {
			column = 0
		}
	}
	if start == 0 {
		return styled
	}
	wrapped.WriteString(styled[start:])
	return wrapped.String()
}
