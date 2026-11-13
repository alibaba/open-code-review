// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
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
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
	)
	if err != nil {
		return markdown
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return strings.TrimRight(sanitizeRenderedMarkdown(rendered), "\n")
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
