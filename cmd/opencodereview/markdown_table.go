// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"html"
	"strings"

	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func formatMarkdownTables(document ast.Node, source []byte, options glamouransi.Options) ([]byte, error) {
	var tables []*extensionast.Table
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if table, ok := node.(*extensionast.Table); entering && ok {
			tables = append(tables, table)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if len(tables) == 0 {
		return source, nil
	}
	green, pink := "10", "13"
	options.Styles.Code.Color = &green
	options.Styles.Strong.Color = &pink
	inline := glamouransi.NewRenderer(options)
	context := glamouransi.NewRenderContext(options)
	for _, table := range tables {
		var rows [][]string
		for row := table.FirstChild(); row != nil; row = row.NextSibling() {
			var cells []string
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				var content strings.Builder
				for child := cell.FirstChild(); child != nil; child = child.NextSibling() {
					element := inline.NewElement(child, source)
					if element.Renderer != nil {
						if err := element.Renderer.Render(&content, context); err != nil {
							return nil, err
						}
					}
				}
				cells = append(cells, sanitizeRenderedMarkdown(content.String()))
			}
			rows = append(rows, cells)
		}
		width := options.WordWrap
		seenList := false
		for parent := table.Parent(); parent != nil; parent = parent.Parent() {
			switch parent.Kind() {
			case ast.KindBlockquote:
				width -= lipgloss.Width(*options.Styles.BlockQuote.IndentToken) * int(*options.Styles.BlockQuote.Indent)
			case ast.KindList:
				if seenList {
					width -= int(options.Styles.List.LevelIndent)
				}
				seenList = true
			}
		}
		layout := layoutMarkdownTable(rows, table.Alignments, max(1, width))
		start := len(source)
		source = append(source, '\n')
		source = append(source, html.EscapeString(strings.ReplaceAll(layout, `\`, `\\`))...)
		source = append(source, '\n')
		block := ast.NewTextBlock()
		block.AppendChild(block, ast.NewTextSegment(text.NewSegment(start, len(source))))
		table.Parent().ReplaceChild(table.Parent(), table, block)
	}
	return source, nil
}

func layoutMarkdownTable(rows [][]string, alignments []extensionast.Alignment, width int) string {
	const gutter = "  "
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	rule := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	columns := len(alignments)
	budget := width - (columns-1)*len(gutter)
	var blocks []string
	if budget < columns*2 {
		for _, row := range rows[1:] {
			for column, cell := range row {
				blocks = append(blocks, wrapMarkdownCell(header.Render(rows[0][column])+": "+cell, width))
			}
			blocks = append(blocks, "")
		}
		if len(blocks) == 0 {
			return wrapMarkdownCell(header.Render(strings.Join(rows[0], gutter)), width)
		}
		return strings.Join(blocks[:len(blocks)-1], "\n")
	}
	inset := 0
	if budget >= 8*columns {
		inset = 1
	}
	widths := markdownColumnWidths(rows, columns, budget-2*inset*columns)
	for rowIndex, row := range rows {
		var cells, rules []string
		for column, cell := range row {
			span := widths[column] + 2*inset
			cellStyle := lipgloss.NewStyle().Width(span).Padding(0, inset)
			switch alignments[column] {
			case extensionast.AlignCenter:
				cellStyle = cellStyle.Align(lipgloss.Center)
			case extensionast.AlignRight:
				cellStyle = cellStyle.Align(lipgloss.Right)
			}
			line := "─"
			if rowIndex == 0 {
				cell = header.Render(cell)
				line = "━"
			}
			if column > 0 {
				cells = append(cells, gutter)
			}
			cells = append(cells, cellStyle.Render(wrapMarkdownCell(cell, widths[column])))
			rules = append(rules, strings.Repeat(line, span))
		}
		blocks = append(blocks, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		if rowIndex == 0 || rowIndex < len(rows)-1 {
			blocks = append(blocks, rule.Render(strings.Join(rules, gutter)))
		}
	}
	return strings.Join(blocks, "\n")
}

func wrapMarkdownCell(cell string, width int) string {
	cell = strings.ReplaceAll(cell, "\t", "    ")
	var output, word strings.Builder
	used, spaces, wordWidth := 0, 0, 0
	flush := func() {
		if word.Len() == 0 {
			return
		}
		if used > 0 && used+spaces+wordWidth > width {
			output.WriteByte('\n')
			used, spaces = 0, 0
		}
		output.WriteString(strings.Repeat(" ", spaces))
		if wordWidth > width {
			wrapped := ansi.Wrap(word.String(), width, "/")
			output.WriteString(wrapped)
			used = ansi.StringWidth(wrapped[strings.LastIndexByte(wrapped, '\n')+1:])
		} else {
			output.WriteString(word.String())
			used += spaces + wordWidth
		}
		word.Reset()
		wordWidth, spaces = 0, 0
	}
	var state byte
	afterWide := false
	for len(cell) > 0 {
		sequence, size, consumed, nextState := ansi.DecodeSequence(cell, state, nil)
		cell, state = cell[consumed:], nextState
		switch sequence {
		case " ":
			flush()
			spaces++
		case "\n":
			flush()
			output.WriteByte('\n')
			used, spaces = 0, 0
		default:
			if size > 0 && (afterWide || size > 1) && !strings.Contains("\u3001\u3002\uff0c\uff1a\uff1b\uff01\uff1f\uff09", sequence) {
				flush()
			}
			word.WriteString(sequence)
			wordWidth += size
		}
		if size > 0 {
			afterWide = size > 1
		}
	}
	flush()
	return output.String()
}

func markdownColumnWidths(rows [][]string, columns, budget int) []int {
	natural := make([]int, columns)
	for _, row := range rows {
		for column, cell := range row {
			natural[column] = max(natural[column], lipgloss.Width(cell))
		}
	}
	widths := make([]int, columns)
	floor := min(12, budget/columns)
	for column, ideal := range natural {
		widths[column] = max(2, min(ideal, floor))
		budget -= widths[column]
	}
	for ; budget > 0; budget-- {
		best := -1
		for column, ideal := range natural {
			if widths[column] < ideal && (best < 0 || ideal*widths[best] > natural[best]*widths[column]) {
				best = column
			}
		}
		if best < 0 {
			break
		}
		widths[best]++
	}
	return widths
}
