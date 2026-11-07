// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
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

// interactiveTerminal reports whether the process is attached to a terminal a
// full-screen program can take over.
//
// Both ends are checked. A program that reads keys needs stdin to be a
// terminal, and one that draws needs stdout to be a terminal; a run with its
// input piped in would otherwise take the screen and then never hear the user.
//
// TERM=dumb means the terminal cannot do what a full-screen program needs —
// colours and cursor movement — so it counts as not interactive even when the
// file descriptors are a tty.
func interactiveTerminal() bool {
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// progressColorProfile resolves the colour profile the live dashboard renders
// with.
//
// It follows the precedence of --color: never means no colour at all, always
// means the widest profile, and the default asks the terminal what it
// supports. Detecting rather than assuming truecolor matters on a terminal
// that only speaks 256 colours: the dashboard's palette would otherwise emit
// sequences the terminal cannot show.
func progressColorProfile() colorprofile.Profile {
	switch colorMode {
	case colorModeNever:
		return colorprofile.Ascii
	case colorModeAlways:
		return colorprofile.TrueColor
	default:
		return colorprofile.Detect(os.Stdout, os.Environ())
	}
}

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
