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

func layoutMarkdownBlocks(document ast.Node, source []byte, options glamouransi.Options) ([]byte, error) {
	var blocks []ast.Node
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case extensionast.KindTable, ast.KindParagraph, ast.KindTextBlock:
			blocks = append(blocks, node)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if len(blocks) == 0 {
		return source, nil
	}
	options.Styles.Code.Prefix = codeSpanBoundary + options.Styles.Code.Prefix
	options.Styles.Code.Suffix += codeSpanBoundary
	terminal := glamouransi.NewRenderer(options)
	context := glamouransi.NewRenderContext(options)
	for _, node := range blocks {
		width := markdownBlockWidth(node, options)
		var layout string
		if table, ok := node.(*extensionast.Table); ok {
			rows, err := markdownTableRows(table, source, terminal, context)
			if err != nil {
				return nil, err
			}
			layout = "\n" + layoutMarkdownTable(rows, table.Alignments, width) + "\n"
		} else {
			wrapped, err := layoutMarkdownProse(node, source, terminal, context, width)
			if err != nil {
				return nil, err
			}
			if wrapped == "" {
				continue
			}
			layout = wrapped
		}
		start := len(source)
		source = append(source, html.EscapeString(strings.ReplaceAll(layout, `\`, `\\`))...)
		block := ast.NewTextBlock()
		if first := node.FirstChild(); first != nil && first.Kind() == extensionast.KindTaskCheckBox {
			block.AppendChild(block, first)
		}
		block.AppendChild(block, ast.NewTextSegment(text.NewSegment(start, len(source))))
		node.Parent().ReplaceChild(node.Parent(), node, block)
	}
	return source, nil
}

func renderMarkdownInlines(node ast.Node, source []byte, terminal *glamouransi.ANSIRenderer, context glamouransi.RenderContext) (markdownInline, error) {
	var content strings.Builder
	for inline := node.FirstChild(); inline != nil; inline = inline.NextSibling() {
		if element := terminal.NewElement(inline, source).Renderer; element != nil {
			if err := element.Render(&content, context); err != nil {
				return markdownInline{}, err
			}
		}
	}
	marked := content.String()
	if node.Kind() == extensionast.KindTableCell {
		marked = strings.ReplaceAll(marked, "\t", "    ")
	}
	return parseMarkdownInline(marked), nil
}

func markdownTableRows(table *extensionast.Table, source []byte, terminal *glamouransi.ANSIRenderer, context glamouransi.RenderContext) ([][]markdownInline, error) {
	var rows [][]markdownInline
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []markdownInline
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			content, err := renderMarkdownInlines(cell, source, terminal, context)
			if err != nil {
				return nil, err
			}
			cells = append(cells, content)
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

func markdownBlockWidth(node ast.Node, options glamouransi.Options) int {
	width := options.WordWrap
	inList := false
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
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

func tableColumnWidths(rows [][]markdownInline, width int) []int {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for column, cell := range row {
			widths[column] = max(widths[column], lipgloss.Width(cell.styled), 1)
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

func layoutMarkdownTable(rows [][]markdownInline, alignments []extensionast.Alignment, width int) string {
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
			cells = append(cells, style.Render(cell.wrap(style.GetWidth())))
		}
		if widths == nil {
			lines = append(lines, strings.Join(cells, "\n"))
		} else {
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		}
	}
	return strings.Join(lines, "\n"+lipgloss.NewStyle().Faint(true).Render(rule)+"\n")
}
