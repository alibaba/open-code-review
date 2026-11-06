// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTUIBrandEHasJoinedArms(t *testing.T) {
	glyph := tuiBrandGlyph('E')
	arms := 0
	previousArm := false
	for y, row := range glyph {
		if !strings.HasPrefix(row, "##") {
			t.Fatalf("E's two-pixel stem is broken at row %d: %q", y, row)
		}
		arm := row == "######" || row == "#####."
		if arm && !previousArm {
			arms++
		}
		if !arm && row != "##...." {
			t.Fatalf("E's counter is obstructed at row %d: %q", y, row)
		}
		previousArm = arm
	}
	if arms != 3 {
		t.Fatalf("E has %d separate horizontal arms, want 3", arms)
	}
	for y, row := range strings.Split(tuiBrandRenderLine("E", false), "\n")[:tuiBrandPixelHeight] {
		if !strings.HasPrefix(row, "##") {
			t.Fatalf("rendered E's stem is broken at row %d: %q", y, row)
		}
	}
}

func TestTUIWordmarkDoesNotDependOnBlockFonts(t *testing.T) {
	colored := renderTUIWordmark(80, true)
	if strings.Trim(ansi.Strip(colored), " \n") != "" {
		t.Fatal("colored pixels must be spaces painted with background colors")
	}
	for _, background := range []string{
		"\x1b[48;2;5;9;7m",       // Canvas.
		"\x1b[48;2;18;61;38m",    // Shadow.
		"\x1b[48;2;66;184;120m",  // Face.
		"\x1b[48;2;140;223;167m", // Highlight.
	} {
		if !strings.Contains(colored, background) {
			t.Fatalf("missing background color %q", background)
		}
	}
	plain := renderTUIWordmark(80, false)
	if strings.Trim(plain, " \n#.") != "" || !strings.Contains(plain, "######") {
		t.Fatal("monochrome wordmark must remain visible with basic ASCII characters")
	}
}

func TestTUIBrandGlyphsAreConnected(t *testing.T) {
	for _, letter := range "OPENC DRVIW" {
		if letter == ' ' {
			continue
		}
		t.Run(string(letter), func(t *testing.T) {
			glyph := tuiBrandGlyph(letter)
			type point struct{ x, y int }
			var pixels []point
			for y, row := range glyph {
				for x, pixel := range row {
					if pixel == '#' {
						pixels = append(pixels, point{x, y})
					}
				}
			}
			if len(pixels) == 0 {
				t.Fatal("empty glyph")
			}
			visited := map[point]bool{pixels[0]: true}
			queue := []point{pixels[0]}
			for len(queue) != 0 {
				current := queue[0]
				queue = queue[1:]
				for _, offset := range []point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					next := point{current.x + offset.x, current.y + offset.y}
					if next.y < 0 || next.y >= len(glyph) || next.x < 0 || next.x >= len(glyph[0]) {
						continue
					}
					if glyph[next.y][next.x] == '#' && !visited[next] {
						visited[next] = true
						queue = append(queue, next)
					}
				}
			}
			if len(visited) != len(pixels) {
				t.Fatalf("glyph has detached pixels: %d of %d are connected", len(visited), len(pixels))
			}
		})
	}
}

func TestTUIWordmarkFitsAvailableWidth(t *testing.T) {
	for _, width := range []int{-1, 0, 1, 2, 3, 8, 9, 40, 63, 64, 80, 120} {
		for _, colored := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/color=%t", width, colored), func(t *testing.T) {
				mark := renderTUIWordmark(width, colored)
				if width <= 0 {
					if mark != "" {
						t.Fatalf("nonpositive width produced %q", mark)
					}
					return
				}
				for y, row := range strings.Split(mark, "\n") {
					if got := ansi.StringWidth(row); got > width {
						t.Fatalf("row %d occupies %d columns with only %d available", y, got, width)
					}
				}
				if !colored && strings.Contains(mark, "\x1b") {
					t.Fatal("plain wordmark includes ANSI escapes")
				}
				if colored && !strings.Contains(mark, "\x1b[") {
					t.Fatal("colored wordmark has no color sequences")
				}
			})
		}
	}
}

func TestTUIWordmarkLayout(t *testing.T) {
	plain := renderTUIWordmark(80, false)
	rows := strings.Split(plain, "\n")
	if len(rows) != 15 || rows[7] != "" {
		t.Fatalf("expected two seven-row words with one gap, got %d rows", len(rows))
	}
	fullWidth := 0
	for _, row := range rows[:7] {
		fullWidth = max(fullWidth, ansi.StringWidth(row))
	}
	if fullWidth != tuiBrandFullWidth {
		t.Fatalf("first word occupies %d columns, want %d", fullWidth, tuiBrandFullWidth)
	}
	if strings.HasPrefix(rows[0], " ") || strings.HasPrefix(rows[8], " ") {
		t.Fatal("wordmark lines have an outer left margin")
	}
	colored := strings.Split(renderTUIWordmark(80, true), "\n")
	for y, row := range rows {
		if ansi.StringWidth(row) != ansi.StringWidth(colored[y]) {
			t.Fatalf("color changes row %d's visible width", y)
		}
		if strings.HasSuffix(row, " ") {
			t.Fatalf("row %d has trailing outer padding", y)
		}
	}
	if got := renderTUIWordmark(40, false); got != "OPEN CODE\nREVIEW" {
		t.Fatalf("narrow-terminal fallback = %q", got)
	}
	if got := renderTUIWordmark(8, false); got != "OCR" {
		t.Fatalf("tiny-terminal fallback = %q", got)
	}
}
