// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"html"
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
	extensionast "github.com/yuin/goldmark/extension/ast"
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
	source = escapeCodeBlockBackslashes(document, source)
	source = sanitizeMarkdownText(document, source)
	var rendered bytes.Buffer
	if err := markdownRenderer.Render(&rendered, source, document); err != nil {
		return markdown
	}
	return strings.TrimRight(sanitizeRenderedMarkdown(rendered.String()), "\n")
}

func sanitizeMarkdownText(document ast.Node, source []byte) []byte {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var decoded string
		switch node.(type) {
		case *ast.Text, *extensionast.Strikethrough:
			decoded = html.UnescapeString(string(node.Text(source)))
		case *ast.RawHTML, *ast.HTMLBlock:
			context := glamouransi.NewRenderContext(glamouransi.Options{})
			decoded = context.SanitizeHTML(string(node.Text(source)), true)
		default:
			return ast.WalkContinue, nil
		}
		sanitized := sanitizeTerminal(ansi.Strip(decoded))
		if sanitized == decoded {
			return ast.WalkContinue, nil
		}
		var textSiblings []*ast.Text
		if first, ok := node.(*ast.Text); ok {
			var content strings.Builder
			for current := first; current != nil; {
				textSiblings = append(textSiblings, current)
				content.Write(current.Value(source))
				next, ok := current.NextSibling().(*ast.Text)
				if !ok {
					break
				}
				if current.HardLineBreak() || current.SoftLineBreak() {
					content.WriteByte('\n')
				}
				current = next
			}
			decoded = html.UnescapeString(content.String())
			sanitized = sanitizeTerminal(ansi.Strip(decoded))
		}
		start := len(source)
		source = append(source, html.EscapeString(sanitized)...)
		segment := text.NewSegment(start, len(source))
		switch typed := node.(type) {
		case *ast.Text:
			typed.Segment = segment
			last := textSiblings[len(textSiblings)-1]
			typed.SetHardLineBreak(last.HardLineBreak())
			typed.SetSoftLineBreak(last.SoftLineBreak())
			for _, sibling := range textSiblings[1:] {
				sibling.Parent().RemoveChild(sibling.Parent(), sibling)
			}
		case *extensionast.Strikethrough:
			typed.RemoveChildren(typed)
			typed.AppendChild(typed, ast.NewTextSegment(segment))
		case *ast.RawHTML:
			typed.Segments = text.NewSegments()
			typed.Segments.Append(segment)
		case *ast.HTMLBlock:
			segments := text.NewSegments()
			segments.Append(segment)
			typed.SetLines(segments)
			typed.ClosureLine = text.NewSegment(-1, -1)
		}
		return ast.WalkSkipChildren, nil
	})
	return source
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

func escapeCodeBlockBackslashes(document ast.Node, source []byte) []byte {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || (node.Kind() != ast.KindCodeBlock && node.Kind() != ast.KindFencedCodeBlock) {
			return ast.WalkContinue, nil
		}
		lines := node.Lines()
		for index := 0; index < lines.Len(); index++ {
			segment := lines.At(index)
			literal := segment.Value(source)
			if !bytes.ContainsRune(literal, '\\') {
				continue
			}
			start := len(source)
			source = append(source, bytes.ReplaceAll(literal, []byte("\\"), []byte("\\\\"))...)
			lines.Set(index, text.NewSegment(start, len(source)))
		}
		return ast.WalkSkipChildren, nil
	})
	return source
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
