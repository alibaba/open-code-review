// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"io"
	"os"
	"strings"

	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func markdownForOutput(writer io.Writer, markdown string) string {
	if writer != os.Stdout || !colorOn() {
		return sanitizeTerminal(markdown)
	}
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		width = 0
	}
	return renderMarkdown(markdown, width)
}

func renderMarkdown(markdown string, width int) string {
	markdown = sanitizeTerminal(ansi.Strip(markdown))
	if width <= 0 {
		width = 80
	}
	style := styles.ASCIIStyleConfig
	style.Document.Margin = new(uint)
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	emphasis := true
	style.Heading.Bold = &emphasis
	for _, heading := range []*glamouransi.StyleBlock{&style.H1, &style.H2, &style.H3, &style.H4, &style.H5, &style.H6} {
		heading.Prefix = ""
	}
	style.Strong = glamouransi.StylePrimitive{Bold: &emphasis}
	style.Emph = glamouransi.StylePrimitive{Italic: &emphasis}
	style.Strikethrough = glamouransi.StylePrimitive{CrossedOut: &emphasis}
	codeColor := "6"
	style.Code.StylePrimitive = glamouransi.StylePrimitive{Color: &codeColor}
	style.Link.Underline = &emphasis
	wrapTables := true
	markdownParser := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	markdownRenderer := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(
		glamouransi.NewRenderer(glamouransi.Options{
			Styles:           style,
			WordWrap:         width,
			TableWrap:        &wrapTables,
			InlineTableLinks: true,
		}), 1000,
	)))
	source := []byte(markdown)
	document := markdownParser.Parser().Parse(text.NewReader(source))
	escapeCodeSpanEntities(document, source)
	var rendered bytes.Buffer
	if err := markdownRenderer.Render(&rendered, source, document); err != nil {
		return markdown
	}
	return strings.TrimRight(sanitizeRenderedMarkdown(rendered.String()), "\n")
}

func escapeCodeSpanEntities(document ast.Node, source []byte) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Kind() != ast.KindCodeSpan {
			return ast.WalkContinue, nil
		}
		for child := node.FirstChild(); child != nil; {
			next := child.NextSibling()
			literal := child.(*ast.Text).Value(source)
			escaped := bytes.ReplaceAll(literal, []byte("&"), []byte("&amp;"))
			node.ReplaceChild(node, child, ast.NewString(escaped))
			child = next
		}
		return ast.WalkSkipChildren, nil
	})
}

func sanitizeRenderedMarkdown(rendered string) string {
	var output strings.Builder
	var state byte
	for len(rendered) > 0 {
		sequence, _, consumed, nextState := ansi.DecodeSequence(rendered, state, nil)
		if strings.HasPrefix(sequence, "\x1b[") && strings.HasSuffix(sequence, "m") && strings.Trim(sequence[2:len(sequence)-1], "0123456789;:") == "" {
			output.WriteString(sequence)
		} else {
			output.WriteString(sanitizeTerminal(ansi.Strip(sequence)))
		}
		rendered = rendered[consumed:]
		state = nextState
	}
	return output.String()
}
