package main

import (
	"testing"
	"time"
)

func TestParseReviewFlagsBackgroundFile(t *testing.T) {
	for _, flag := range []string{"--background-file", "-B"} {
		t.Run(flag, func(t *testing.T) {
			opts, err := parseReviewFlags([]string{flag, "./docs/req.md"})
			if err != nil {
				t.Fatalf("parseReviewFlags: %v", err)
			}
			if opts.backgroundFile != "./docs/req.md" {
				t.Errorf("backgroundFile = %q, want %q", opts.backgroundFile, "./docs/req.md")
			}
		})
	}
}

func TestParseReviewFlagsModelOverride(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--model", "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}

	if opts.model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", opts.model, "claude-opus-4-6")
	}
	if opts.outputFormat != "text" {
		t.Errorf("outputFormat = %q, want %q", opts.outputFormat, "text")
	}
	if opts.audience != "human" {
		t.Errorf("audience = %q, want %q", opts.audience, "human")
	}
}

func TestParseReviewFlagsResume(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "feature", "--resume", "session-123"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}
	if opts.resume != "session-123" {
		t.Errorf("resume = %q, want session-123", opts.resume)
	}
}

func TestParseReviewFlags_PreviewWithResume(t *testing.T) {
	_, err := parseReviewFlags([]string{"--commit", "abc123", "--preview", "--resume", "session-123"})
	if err == nil {
		t.Fatal("expected error for --preview with --resume")
	}
}

func TestParseReviewFlags_InvalidAudience(t *testing.T) {
	_, err := parseReviewFlags([]string{"--audience", "robot"})
	if err == nil {
		t.Fatal("expected error for invalid audience")
	}
}

func TestParseReviewFlags_NegativeMaxTools(t *testing.T) {
	_, err := parseReviewFlags([]string{"--max-tools", "-1"})
	if err == nil {
		t.Fatal("expected error for negative max-tools")
	}
}

func TestParseReviewFlags_MaxToolsBelowMin(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--max-tools", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.maxTools != 10 {
		t.Errorf("maxTools = %d, want 10 (clamped to min)", opts.maxTools)
	}
}

func TestParseReviewFlags_NegativeMaxGitProcs(t *testing.T) {
	_, err := parseReviewFlags([]string{"--max-git-procs", "-1"})
	if err == nil {
		t.Fatal("expected error for negative max-git-procs")
	}
}

func TestParseReviewFlags_ConflictingModes(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main", "--to", "dev", "--commit", "abc"})
	if err == nil {
		t.Fatal("expected error for conflicting modes")
	}
}

func TestParseReviewFlags_FromWithoutTo(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main"})
	if err == nil {
		t.Fatal("expected error for --from without --to")
	}
}

func TestParseReviewFlags_ToWithoutFrom(t *testing.T) {
	_, err := parseReviewFlags([]string{"--to", "dev"})
	if err == nil {
		t.Fatal("expected error for --to without --from")
	}
}

func TestParseReviewFlags_Help(t *testing.T) {
	opts, err := parseReviewFlags([]string{"-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.showHelp {
		t.Error("expected showHelp=true")
	}
}

func TestParseReviewFlags_ShortFlags(t *testing.T) {
	opts, err := parseReviewFlags([]string{"-c", "abc123", "-f", "json", "-p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.commit != "abc123" {
		t.Errorf("commit = %q, want abc123", opts.commit)
	}
	if opts.outputFormat != "json" {
		t.Errorf("outputFormat = %q, want json", opts.outputFormat)
	}
	if !opts.preview {
		t.Error("expected preview=true")
	}
}

func TestParseConfigArgs_Empty(t *testing.T) {
	_, err := parseConfigArgs(nil)
	if err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestParseConfigArgs_Set(t *testing.T) {
	act, err := parseConfigArgs([]string{"set", "llm.model", "gpt-4"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.subCmd != "set" || act.key != "llm.model" || act.value != "gpt-4" {
		t.Errorf("got %+v", act)
	}
}

func TestParseConfigArgs_SetMissingValue(t *testing.T) {
	_, err := parseConfigArgs([]string{"set", "llm.model"})
	if err == nil {
		t.Fatal("expected error for missing value")
	}
}

func TestParseConfigArgs_Unset(t *testing.T) {
	act, err := parseConfigArgs([]string{"unset", "custom_providers.foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.subCmd != "unset" || act.key != "custom_providers.foo" {
		t.Errorf("got %+v", act)
	}
}

func TestParseConfigArgs_UnsetMissingKey(t *testing.T) {
	_, err := parseConfigArgs([]string{"unset"})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestParseConfigArgs_UnknownSubCmd(t *testing.T) {
	_, err := parseConfigArgs([]string{"delete", "foo"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestDurationVar(t *testing.T) {
	fs := newOcrFlagSet("test")
	var d time.Duration
	fs.DurationVar(&d, "timeout", 5*time.Second, "max duration")
	if err := fs.Parse([]string{"--timeout", "10s"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d != 10*time.Second {
		t.Errorf("d = %v, want 10s", d)
	}
}

func TestPrintDefaults(t *testing.T) {
	fs := newOcrFlagSet("test")
	var s string
	fs.StringVar(&s, "name", "default", "a name")
	fs.PrintDefaults()
}

func TestExpandShortFlags(t *testing.T) {
	m := map[string]string{"c": "commit", "f": "format"}
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"expands short", []string{"-c", "abc"}, []string{"--commit", "abc"}},
		{"keeps long", []string{"--format", "json"}, []string{"--format", "json"}},
		{"unknown short kept", []string{"-x", "val"}, []string{"-x", "val"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandShortFlags(tc.args, m)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
