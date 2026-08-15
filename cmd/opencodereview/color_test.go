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

// setColorFlags sets the flag-backed variables for one test and restores them.
func setColorFlags(t *testing.T, mode string, never bool) {
	t.Helper()
	prevMode, prevNever := colorMode, colorNeverFl
	colorMode, colorNeverFl = mode, never
	t.Cleanup(func() { colorMode, colorNeverFl = prevMode, prevNever })
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
		name    string
		mode    string
		never   bool
		noColor string // NO_COLOR value; "" means unset
		term    string // TERM value; "" means unset
		want    bool
	}{
		{name: "auto into a pipe stays plain", mode: colorModeAuto, want: false},
		{name: "never", mode: colorModeNever, want: false},
		{name: "no-color flag", mode: colorModeAuto, never: true, want: false},
		{name: "always overrides pipe", mode: colorModeAlways, want: true},
		{name: "NO_COLOR disables auto", mode: colorModeAuto, noColor: "1", want: false},
		{name: "NO_COLOR any value disables", mode: colorModeAuto, noColor: "0", want: false},
		// A flag is a per-invocation decision, so it outranks the standing
		// NO_COLOR preference.
		{name: "always beats NO_COLOR", mode: colorModeAlways, noColor: "1", want: true},
		{name: "no-color beats always", mode: colorModeAlways, never: true, want: false},
		{name: "TERM=dumb disables auto", mode: colorModeAuto, term: "dumb", want: false},
		{name: "TERM=dumb case-insensitive", mode: colorModeAuto, term: "DUMB", want: false},
		{name: "always beats TERM=dumb", mode: colorModeAlways, term: "dumb", want: true},
		{name: "TERM=xterm still needs a TTY", mode: colorModeAuto, term: "xterm-256color", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setColorFlags(t, tt.mode, tt.never)
			t.Setenv("NO_COLOR", tt.noColor)
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
		{name: "no-color before the subcommand", args: []string{"--no-color", "version"}, wantOn: false},
		{name: "no-color after the subcommand", args: []string{"version", "--no-color"}, wantOn: false},
		{name: "color=never", args: []string{"version", "--color=never"}, wantOn: false},
		{name: "color=always forces color into a pipe", args: []string{"version", "--color=always"}, wantOn: true},
		{
			name:    "invalid value is rejected",
			args:    []string{"version", "--color=sometimes"},
			wantErr: `invalid --color value "sometimes"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setColorFlags(t, colorModeAuto, false)
			setColor(t, false)
			t.Setenv("NO_COLOR", "")
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
	// addColorFlags binds the shared flag variables and resets them to their
	// defaults, so restore them for the rest of the package.
	setColorFlags(t, colorMode, colorNeverFl)
	cmd := &cobra.Command{Use: "test"}
	addColorFlags(cmd)
	for _, name := range []string{"color", "no-color"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			// Persistent so `ocr --no-color review` and `ocr review --no-color`
			// behave identically.
			t.Errorf("--%s is not registered as a persistent flag", name)
		}
	}
	if def := cmd.PersistentFlags().Lookup("color").DefValue; def != colorModeAuto {
		t.Errorf("--color default = %q, want %q", def, colorModeAuto)
	}
}
