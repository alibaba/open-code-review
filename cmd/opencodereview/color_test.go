// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// setColor pins the resolved color decision for one test and restores the
// previous state afterwards. Tests capture stdout through a pipe, which is not a
// terminal, so auto-detection would otherwise disable color for every case.
func setColor(t *testing.T, on bool) {
	t.Helper()
	prev := colorEnabled
	colorEnabled = on
	t.Cleanup(func() { colorEnabled = prev })
}

// setColorFlags sets the flag-backed variable for one test and restores it.
func setColorFlags(t *testing.T, mode string) {
	t.Helper()
	prevMode := colorMode
	colorMode = mode
	t.Cleanup(func() { colorMode = prevMode })
}

func TestValidateColorMode(t *testing.T) {
	for _, mode := range []string{colorModeAuto, colorModeAlways, colorModeNever} {
		if err := validateColorMode(mode); err != nil {
			t.Errorf("validateColorMode(%q) = %v, want nil", mode, err)
		}
	}
	for _, mode := range []string{"yes", "no", "", "Auto", "1"} {
		err := validateColorMode(mode)
		if err == nil {
			t.Errorf("validateColorMode(%q) = nil, want error", mode)
			continue
		}
		// The message must name the offending value and the valid set so the
		// user can fix the invocation without consulting --help.
		if !strings.Contains(err.Error(), mode) && mode != "" {
			t.Errorf("error %q does not mention %q", err, mode)
		}
		if !strings.Contains(err.Error(), "auto, always, never") {
			t.Errorf("error %q does not list valid values", err)
		}
	}
}

// TestResolveColor covers the precedence chain. stdout under `go test` is not a
// terminal, so the auto cases assert the not-a-TTY branch — exactly the
// regression from issue #682.
func TestResolveColor(t *testing.T) {
	tests := []struct {
		name string
		mode string
		term string // TERM value; "" means unset
		want bool
	}{
		{name: "auto into a pipe stays plain", mode: colorModeAuto, want: false},
		{name: "never", mode: colorModeNever, want: false},
		{name: "always overrides pipe", mode: colorModeAlways, want: true},
		{name: "TERM=dumb disables auto", mode: colorModeAuto, term: "dumb", want: false},
		{name: "TERM=dumb case-insensitive", mode: colorModeAuto, term: "DUMB", want: false},
		{name: "always beats TERM=dumb", mode: colorModeAlways, term: "dumb", want: true},
		{name: "TERM=xterm still needs a TTY", mode: colorModeAuto, term: "xterm-256color", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setColorFlags(t, tt.mode)
			t.Setenv("TERM", tt.term)
			if got := resolveColor(); got != tt.want {
				t.Errorf("resolveColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColorize(t *testing.T) {
	t.Run("color on wraps in the sequence and a reset", func(t *testing.T) {
		setColor(t, true)
		if got := colorize("\033[31m", "boom"); got != "\033[31mboom\033[0m" {
			t.Errorf("colorize() = %q, want wrapped", got)
		}
	})
	t.Run("color off returns the text untouched", func(t *testing.T) {
		setColor(t, false)
		if got := colorize("\033[31m", "boom"); got != "boom" {
			t.Errorf("colorize() = %q, want bare text", got)
		}
	})
}

// TestColorFlagsThroughRootCmd drives the real command tree so the flags and the
// PersistentPreRunE validation and resolution are exercised together.
func TestColorFlagsThroughRootCmd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		wantOn  bool // expected colorOn() after the run; stdout is not a TTY here
	}{
		{name: "default auto stays plain off a TTY", args: []string{"version"}, wantOn: false},
		{name: "color=never before the subcommand", args: []string{"--color=never", "version"}, wantOn: false},
		{name: "color=never after the subcommand", args: []string{"version", "--color=never"}, wantOn: false},
		{name: "color=always forces color into a pipe", args: []string{"version", "--color=always"}, wantOn: true},
		{
			name:    "invalid value is rejected",
			args:    []string{"version", "--color=sometimes"},
			wantErr: `invalid --color value "sometimes"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setColorFlags(t, colorModeAuto)
			setColor(t, false)
			rootCmd.SetArgs(tt.args)
			t.Cleanup(func() { rootCmd.SetArgs(nil) })

			out := captureStdout(t, func() {
				err := rootCmd.Execute()
				switch {
				case tt.wantErr != "" && err == nil:
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
					t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
				case tt.wantErr == "" && err != nil:
					t.Errorf("unexpected error: %v", err)
				}
			})

			if tt.wantErr != "" {
				return
			}
			if got := colorOn(); got != tt.wantOn {
				t.Errorf("colorOn() = %v after %v, want %v", got, tt.args, tt.wantOn)
			}
			// The version banner itself is never colorized, so it must stay clean.
			if strings.Contains(out, "\033") {
				t.Errorf("version output contains an escape sequence: %q", out)
			}
		})
	}
}

func TestAddColorFlags(t *testing.T) {
	// addColorFlags binds the shared flag variable and resets it to its
	// default, so restore it for the rest of the package.
	setColorFlags(t, colorMode)
	cmd := &cobra.Command{Use: "test"}
	addColorFlags(cmd)
	// Persistent so `ocr --color=never review` and `ocr review --color=never`
	// behave identically.
	if cmd.PersistentFlags().Lookup("color") == nil {
		t.Error("--color is not registered as a persistent flag")
	}
	if def := cmd.PersistentFlags().Lookup("color").DefValue; def != colorModeAuto {
		t.Errorf("--color default = %q, want %q", def, colorModeAuto)
	}
}
