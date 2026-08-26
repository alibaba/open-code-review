// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// Color mode values accepted by --color.
const (
	colorModeAuto   = "auto"
	colorModeAlways = "always"
	colorModeNever  = "never"
)

var (
	// colorMode holds the raw --color value.
	colorMode = colorModeAuto

	// colorEnabled is the resolved decision, assigned once flags are parsed.
	// Its zero value is false, so any path that renders without going through a
	// command (unit tests, direct helper calls) stays plain text deterministically.
	colorEnabled bool
)

// addColorFlags registers the color control. It is persistent on the root
// command so every subcommand inherits identical behavior, and so
// `ocr --color=never review` and `ocr review --color=never` are equivalent.
func addColorFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&colorMode, "color", colorModeAuto,
		"when to emit ANSI color: auto (only when stdout is a terminal), always, or never")
	cmd.RegisterFlagCompletionFunc("color", completeEnum(colorModeAuto, colorModeAlways, colorModeNever))
}

// validateColorMode rejects unknown --color values instead of silently treating
// them as auto, so a typo like --color=yes is reported rather than ignored.
func validateColorMode(mode string) error {
	switch mode {
	case colorModeAuto, colorModeAlways, colorModeNever:
		return nil
	default:
		return fmt.Errorf("invalid --color value %q: must be one of auto, always, never", mode)
	}
}

// resolveColor decides whether ANSI sequences may be written to stdout.
//
// Precedence, highest first:
//  1. --color=never         → off
//  2. --color=always        → on, even into a pipe (for `| less -R`)
//  3. TERM=dumb              → off
//  4. stdout is a terminal  → on, otherwise off
func resolveColor() bool {
	if colorMode == colorModeNever {
		return false
	}
	if colorMode == colorModeAlways {
		return true
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	return term.IsTerminal(os.Stdout.Fd())
}

// colorOn reports whether ANSI color should be emitted.
func colorOn() bool { return colorEnabled }

// colorize wraps s in seq and a reset, or returns s untouched when color is off.
// Every escape sequence in the text output path is gated through here or through
// an explicit colorOn() branch.
func colorize(seq, s string) string {
	if !colorOn() {
		return s
	}
	return seq + s + ansiReset
}

// colorf formats then colorizes, so call sites need not nest fmt.Sprintf.
func colorf(seq, format string, a ...any) string {
	return colorize(seq, fmt.Sprintf(format, a...))
}

const ansiReset = "\033[0m"
