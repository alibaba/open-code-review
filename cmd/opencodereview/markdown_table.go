// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"cmp"
	"html"
	"slices"
	"strings"

	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark/ast"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func formatMarkdownTables(document ast.Node, source []byte, terminal *glamouransi.ANSIRenderer, options glamouransi.Options) ([]byte, error) {
	var tables []*extensionast.Table
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if table, ok := node.(*extensionast.Table); ok && entering {
			tables = append(tables, table)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if len(tables) == 0 {
		return source, nil
	}
	context := glamouransi.NewRenderContext(options)
	for _, table := range tables {
		rows, err := markdownTableRows(table, source, terminal, context)
		if err != nil {
			return nil, err
		}
		layout := layoutMarkdownTable(rows, table.Alignments, markdownTableWidth(table, options))
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

func markdownTableRows(table *extensionast.Table, source []byte, terminal *glamouransi.ANSIRenderer, context glamouransi.RenderContext) ([][]string, error) {
	var rows [][]string
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			var content strings.Builder
			for inline := cell.FirstChild(); inline != nil; inline = inline.NextSibling() {
				if element := terminal.NewElement(inline, source).Renderer; element != nil {
					if err := element.Render(&content, context); err != nil {
						return nil, err
					}
				}
			}
			cells = append(cells, strings.ReplaceAll(content.String(), "\t", "    "))
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

func markdownTableWidth(table *extensionast.Table, options glamouransi.Options) int {
	width := options.WordWrap
	inList := false
	for parent := table.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case ast.KindBlockquote:
			quote := options.Styles.BlockQuote
			width -= lipgloss.Width(*quote.IndentToken) * int(*quote.Indent)
		case ast.KindList:
			if inList {
				width -= int(options.Styles.List.LevelIndent)
			}
			inList = true
		}
	}
	return max(1, width)
}

const tableGutter = "  "

func tableColumnWidths(rows [][]string, width int) []int {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for column, cell := range row {
			widths[column] = max(widths[column], lipgloss.Width(cell), 1)
		}
	}
	available := width - (len(widths)-1)*len(tableGutter)
	columns := make([]int, len(widths))
	for column := range columns {
		columns[column] = column
	}
	slices.SortFunc(columns, func(left, right int) int { return cmp.Compare(widths[left], widths[right]) })
	for index, column := range columns {
		share := available / (len(columns) - index)
		if share < min(2, widths[column]) {
			return nil
		}
		widths[column] = min(widths[column], share)
		available -= widths[column]
	}
	return widths
}

func layoutMarkdownTable(rows [][]string, alignments []extensionast.Alignment, width int) string {
	widths := tableColumnWidths(rows, width)
	var rule string
	if widths == nil {
		rule = strings.Repeat("─", width)
	} else {
		var segments []string
		for _, size := range widths {
			segments = append(segments, strings.Repeat("─", size))
		}
		rule = strings.Join(segments, tableGutter)
	}
	var lines []string
	for index, row := range rows {
		var cells []string
		for column, cell := range row {
			style := lipgloss.NewStyle().Width(width).Bold(index == 0)
			if widths != nil {
				style = style.Width(widths[column])
				switch alignments[column] {
				case extensionast.AlignCenter:
					style = style.Align(lipgloss.Center)
				case extensionast.AlignRight:
					style = style.Align(lipgloss.Right)
				}
				if column > 0 {
					cells = append(cells, tableGutter)
				}
			}
			cells = append(cells, style.Render(cell))
		}
		if widths == nil {
			lines = append(lines, strings.Join(cells, "\n"))
		} else {
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		}
	}
	return strings.Join(lines, "\n"+lipgloss.NewStyle().Faint(true).Render(rule)+"\n")
}
