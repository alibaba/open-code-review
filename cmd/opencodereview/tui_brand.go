// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	tuiBrandBackground = "#050907"
	tuiBrandShadow     = "#123D26"
	tuiBrandFace       = "#42B878"
	tuiBrandHighlight  = "#8CDFA7"
	tuiBrandText       = "#DCE8DF"
	tuiBrandMuted      = "#82988A"

	tuiBrandPixelHeight = 6
	tuiBrandFullWidth   = 64
)

// renderTUIWordmark renders a left-aligned, terminal-native two-line wordmark.
// The large mark occupies at most 64 columns and 15 rows. Callers can choose
// the text variant by passing a width below tuiBrandFullWidth on short screens.
func renderTUIWordmark(width int, colored bool) string {
	if width <= 0 {
		return ""
	}
	if width < tuiBrandFullWidth {
		text := "OPEN CODE\nREVIEW"
		if width < len("OPEN CODE") {
			text = "OCR"[:min(width, len("OCR"))]
		}
		if colored {
			return ansi.NewStyle().
				ForegroundColor(lipgloss.Color(tuiBrandFace)).
				BackgroundColor(lipgloss.Color(tuiBrandBackground)).
				Styled(text)
		}
		return text
	}
	return tuiBrandRenderLine("OPEN CODE", colored) + "\n\n" +
		tuiBrandRenderLine("REVIEW", colored)
}

// Each pixel occupies a whole terminal cell. Two-column stems balance the
// taller cell aspect ratio without depending on the terminal font's glyphs.
func tuiBrandGlyph(letter rune) [tuiBrandPixelHeight]string {
	switch letter {
	case 'O':
		return [tuiBrandPixelHeight]string{
			"######", "##..##", "##..##", "##..##", "##..##", "######",
		}
	case 'P':
		return [tuiBrandPixelHeight]string{
			"#####.", "##..##", "#####.", "##....", "##....", "##....",
		}
	case 'E':
		return [tuiBrandPixelHeight]string{
			"######", "##....", "#####.", "##....", "##....", "######",
		}
	case 'N':
		return [tuiBrandPixelHeight]string{
			"##..##", "###.##", "######", "##.###", "##..##", "##..##",
		}
	case 'C':
		return [tuiBrandPixelHeight]string{
			"######", "##....", "##....", "##....", "##....", "######",
		}
	case 'D':
		return [tuiBrandPixelHeight]string{
			"#####.", "##..##", "##..##", "##..##", "##..##", "#####.",
		}
	case 'R':
		return [tuiBrandPixelHeight]string{
			"#####.", "##..##", "#####.", "####..", "##.##.", "##..##",
		}
	case 'V':
		return [tuiBrandPixelHeight]string{
			"##..##", "##..##", "##..##", "##..##", ".####.", "..##..",
		}
	case 'I':
		return [tuiBrandPixelHeight]string{
			"####", ".##.", ".##.", ".##.", ".##.", "####",
		}
	case 'W':
		return [tuiBrandPixelHeight]string{
			"##....##", "##....##", "##....##", "##.##.##", "########", ".##..##.",
		}
	default:
		return [tuiBrandPixelHeight]string{}
	}
}

func tuiBrandRenderLine(word string, colored bool) string {
	var glyphs [][tuiBrandPixelHeight]string
	pixelWidth := 0
	for _, letter := range word {
		if letter == ' ' {
			glyphs = append(glyphs, [tuiBrandPixelHeight]string{})
			pixelWidth++
			continue
		}
		glyph := tuiBrandGlyph(letter)
		glyphs = append(glyphs, glyph)
		pixelWidth += len(glyph[0]) + 2
	}
	// One column separates letters after their shadow. The final glyph needs
	// only its shadow column, without a trailing inter-letter gap.
	pixelWidth--
	pixels := make([][]uint8, tuiBrandPixelHeight+1)
	for y := range pixels {
		pixels[y] = make([]uint8, pixelWidth)
	}
	x := 0
	for _, glyph := range glyphs {
		if glyph[0] == "" {
			x++
			continue
		}
		for y, row := range glyph {
			for column, pixel := range row {
				if pixel == '#' {
					pixels[y][x+column] = 2
				}
			}
		}
		x += len(glyph[0]) + 2
	}
	// Draw the shadow only into empty pixels, then bevel the exposed top edges.
	for y := tuiBrandPixelHeight - 1; y >= 0; y-- {
		for column := pixelWidth - 2; column >= 0; column-- {
			if pixels[y][column] == 2 && pixels[y+1][column+1] == 0 {
				pixels[y+1][column+1] = 1
			}
		}
	}
	for y := tuiBrandPixelHeight - 1; y >= 0; y-- {
		for column := range pixels[y] {
			if pixels[y][column] == 2 && (y == 0 || pixels[y-1][column] < 2) {
				pixels[y][column] = 3
			}
		}
	}
	colors := [...]color.Color{
		lipgloss.Color(tuiBrandBackground),
		lipgloss.Color(tuiBrandShadow),
		lipgloss.Color(tuiBrandFace),
		lipgloss.Color(tuiBrandHighlight),
	}
	var result strings.Builder
	for y, row := range pixels {
		if y > 0 {
			result.WriteByte('\n')
		}
		lastColumn := pixelWidth - 1
		for lastColumn >= 0 && row[lastColumn] == 0 {
			lastColumn--
		}
		lastStyle := ""
		for column := 0; column <= lastColumn; column++ {
			pixel := row[column]
			cell := byte(' ')
			if colored {
				// Background-colored spaces remain solid even when legacy console
				// fonts lack Unicode block characters or leave gaps around them.
				style := ansi.NewStyle().BackgroundColor(colors[pixel]).String()
				if style != lastStyle {
					result.WriteString(style)
					lastStyle = style
				}
			} else if pixel >= 2 {
				cell = '#'
			} else if pixel == 1 {
				cell = '.'
			}
			result.WriteByte(cell)
		}
		if colored {
			result.WriteString(ansi.ResetStyle)
		}
	}
	return result.String()
}
