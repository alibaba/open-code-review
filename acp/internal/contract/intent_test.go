// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract

import (
	"reflect"
	"testing"
)

func TestBuildReviewArgs(t *testing.T) {
	tests := []struct {
		name    string
		intent  ReviewIntent
		want    []string
		wantErr bool
	}{
		{
			name:   "workspace review",
			intent: ReviewIntent{Type: ReviewTypeWorkspace},
			want:   []string{"review", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "range review",
			intent: ReviewIntent{
				Type: ReviewTypeRange,
				From: "main",
				To:   "feature",
			},
			want: []string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "commit review",
			intent: ReviewIntent{
				Type:   ReviewTypeCommit,
				Commit: "abc123",
			},
			want: []string{"review", "--commit", "abc123", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "range review missing from",
			intent: ReviewIntent{
				Type: ReviewTypeRange,
				To:   "feature",
			},
			wantErr: true,
		},
		{
			name: "commit review missing commit",
			intent: ReviewIntent{
				Type: ReviewTypeCommit,
			},
			wantErr: true,
		},
		{
			name: "workspace review with extra flags",
			intent: ReviewIntent{
				Type:  ReviewTypeWorkspace,
				Extra: []string{"--effort", "high"},
			},
			want: []string{"review", "--effort", "high", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "unknown review type",
			intent: ReviewIntent{
				Type: ReviewType("bogus"),
			},
			wantErr: true,
		},
		{
			name: "range review missing to only",
			intent: ReviewIntent{
				Type: ReviewTypeRange,
				From: "main",
			},
			wantErr: true,
		},
		{
			name: "range review missing from only",
			intent: ReviewIntent{
				Type: ReviewTypeRange,
				To:   "feature",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildReviewArgs(&tt.intent)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildReviewArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildReviewArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildScanArgs(t *testing.T) {
	tests := []struct {
		name    string
		intent  ScanIntent
		want    []string
		wantErr bool
	}{
		{
			name:   "scan root (no paths)",
			intent: ScanIntent{},
			want:   []string{"scan", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "scan single path",
			intent: ScanIntent{
				Paths: []string{"internal/agent"},
			},
			want: []string{"scan", "--path", "internal/agent", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "scan multiple paths are comma-joined into one --path",
			intent: ScanIntent{
				Paths: []string{"cmd", "internal"},
			},
			want: []string{"scan", "--path", "cmd,internal", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "reject absolute path",
			intent: ScanIntent{
				Paths: []string{"/absolute/path"},
			},
			wantErr: true,
		},
		{
			name: "reject parent traversal",
			intent: ScanIntent{
				Paths: []string{"../outside"},
			},
			wantErr: true,
		},
		{
			name: "clean path with dot",
			intent: ScanIntent{
				Paths: []string{"./internal/agent"},
			},
			want: []string{"scan", "--path", "internal/agent", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			name: "reject bare parent",
			intent: ScanIntent{
				Paths: []string{".."},
			},
			wantErr: true,
		},
		{
			name: "reject parent traversal after clean",
			intent: ScanIntent{
				Paths: []string{"foo/../../bar"},
			},
			wantErr: true,
		},
		{
			name: "reject backslash traversal",
			intent: ScanIntent{
				Paths: []string{`..\..\file`},
			},
			wantErr: true,
		},
		{
			name: "reject windows absolute path",
			intent: ScanIntent{
				Paths: []string{`C:\Windows\System32`},
			},
			wantErr: true,
		},
		{
			name: "reject windows drive absolute with forward slash",
			intent: ScanIntent{
				Paths: []string{"C:/Windows"},
			},
			wantErr: true,
		},
		{
			name: "reject comma in path",
			intent: ScanIntent{
				Paths: []string{"a,b"},
			},
			wantErr: true,
		},
		{
			name: "scan with extra flags",
			intent: ScanIntent{
				Extra: []string{"--batch", "by-language"},
			},
			want: []string{"scan", "--batch", "by-language", "--format", "json", "--audience", "human", "--color", "never"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildScanArgs(&tt.intent)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildScanArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildScanArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuildReviewArgsExtraWhitelist covers the flags that must not be
// forwardable from a caller, plus the value validation on the ones that may.
func TestBuildReviewArgsExtraWhitelist(t *testing.T) {
	tests := []struct {
		name    string
		extra   []string
		wantErr bool
	}{
		{"effort with valid value", []string{"--effort", "low"}, false},
		{"effort with invalid value", []string{"--effort", "extreme"}, true},
		{"effort with inline value", []string{"--effort=medium"}, false},
		{"effort missing value", []string{"--effort"}, true},
		{"boolean no-filter", []string{"--no-filter"}, false},
		{"boolean no-filter inline", []string{"--no-filter=true"}, false},
		{"boolean no-filter invalid inline", []string{"--no-filter=maybe"}, true},
		{"background with value", []string{"--background", "review the auth path"}, false},
		{"background-file with value", []string{"--background-file", "spec.md"}, false},
		// A value that looks like a flag is rejected rather than consumed, so a
		// typo'd invocation cannot silently swallow the following flag.
		{"effort value looks like flag", []string{"--effort", "--no-filter"}, true},
		{"inner flag name as value", []string{"--background", "--effort"}, true},
		{"effort inline value looks like flag", []string{"--effort=--no-filter"}, true},
		{"background inline value looks like flag", []string{"--background=--format"}, true},
		{"bare dash as value", []string{"--effort", "-"}, true},
		// Empty inline values: allowed for flags without an enum, where the OCR
		// CLI treats "" the same as omitting the flag, and rejected for enum
		// flags, where "" is simply not a valid value.
		{"empty inline background", []string{"--background="}, false},
		{"empty inline effort", []string{"--effort="}, true},
		// Integration flags are fixed by the adapter and must not be overridable.
		{"output path injection", []string{"--output", "/tmp/leak"}, true},
		{"format override", []string{"--format", "yaml"}, true},
		{"audience override", []string{"--audience", "agent"}, true},
		{"color override", []string{"--color", "always"}, true},
		{"repo override", []string{"--repo", "/etc"}, true},
		{"rule override", []string{"--rule", "evil.json"}, true},
		{"tools override", []string{"--tools", "evil.json"}, true},
		{"resume override", []string{"--resume", "other-session"}, true},
		{"model override", []string{"--model", "attacker-model"}, true},
		{"provider override", []string{"--provider", "evil"}, true},
		{"preview changes semantics", []string{"--preview"}, true},
		// scan-only flags are not registered on review.
		{"scan-only batch on review", []string{"--batch", "none"}, true},
		{"scan-only no-plan on review", []string{"--no-plan"}, true},
		{"scan-only no-dedup on review", []string{"--no-dedup"}, true},
		{"scan-only no-summary on review", []string{"--no-summary"}, true},
		// A bare word is not a flag and must not be forwarded.
		{"stray value", []string{"evil"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := ReviewIntent{Type: ReviewTypeWorkspace, Extra: tt.extra}
			_, err := BuildReviewArgs(&intent)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildReviewArgs(Extra=%v) error = %v, wantErr %v", tt.extra, err, tt.wantErr)
			}
		})
	}
}

// TestBuildScanArgsExtraWhitelist is the scan counterpart. The whitelists
// differ between commands, so --effort is rejected here and accepted on review.
func TestBuildScanArgsExtraWhitelist(t *testing.T) {
	tests := []struct {
		name    string
		extra   []string
		wantErr bool
	}{
		{"batch with valid value", []string{"--batch", "by-directory"}, false},
		{"batch with invalid value", []string{"--batch", "by-nonsense"}, true},
		{"batch with inline value", []string{"--batch=none"}, false},
		{"batch missing value", []string{"--batch"}, true},
		{"boolean no-plan", []string{"--no-plan"}, false},
		{"boolean no-dedup", []string{"--no-dedup"}, false},
		{"boolean no-summary", []string{"--no-summary"}, false},
		{"background with value", []string{"--background", "scan context"}, false},
		// Same flag-as-value guard as on review.
		{"batch value looks like flag", []string{"--batch", "--no-dedup"}, true},
		{"batch inline value looks like flag", []string{"--batch=--no-summary"}, true},
		{"empty inline background", []string{"--background="}, false},
		{"empty inline batch", []string{"--batch="}, true},
		// Integration flags are fixed by the adapter and must not be overridable.
		{"output path injection", []string{"--output", "/tmp/leak"}, true},
		{"format override", []string{"--format", "sarif"}, true},
		{"audience override", []string{"--audience", "agent"}, true},
		{"color override", []string{"--color", "always"}, true},
		{"repo override", []string{"--repo", "/etc"}, true},
		{"rule override", []string{"--rule", "evil.json"}, true},
		// review-only flags are not registered on scan.
		{"review-only effort on scan", []string{"--effort", "high"}, true},
		{"review-only no-filter on scan", []string{"--no-filter"}, true},
		{"review-only background-file on scan", []string{"--background-file", "spec.md"}, true},
		{"stray value", []string{"evil"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := ScanIntent{Extra: tt.extra}
			_, err := BuildScanArgs(&intent)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildScanArgs(Extra=%v) error = %v, wantErr %v", tt.extra, err, tt.wantErr)
			}
		})
	}
}

// TestBuildReviewArgsRejectsNilAndContradictory pins the mode exclusivity: a
// workspace intent must not carry range/commit fields, and vice versa. Silent
// tolerance here would hide a caller bug that changes the review scope.
func TestBuildReviewArgsRejectsNilAndContradictory(t *testing.T) {
	if _, err := BuildReviewArgs(nil); err == nil {
		t.Error("BuildReviewArgs(nil) = nil error, want error")
	}

	tests := []struct {
		name   string
		intent ReviewIntent
	}{
		{"workspace with commit", ReviewIntent{Type: ReviewTypeWorkspace, Commit: "abc"}},
		{"workspace with from", ReviewIntent{Type: ReviewTypeWorkspace, From: "main"}},
		{"range with commit", ReviewIntent{Type: ReviewTypeRange, From: "main", To: "feature", Commit: "abc"}},
		{"commit with from", ReviewIntent{Type: ReviewTypeCommit, Commit: "abc", From: "main"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildReviewArgs(&tt.intent); err == nil {
				t.Errorf("BuildReviewArgs(%+v) = nil error, want error", tt.intent)
			}
		})
	}
}

func TestBuildScanArgsRejectsNil(t *testing.T) {
	if _, err := BuildScanArgs(nil); err == nil {
		t.Error("BuildScanArgs(nil) = nil error, want error")
	}
}

// TestValidateExtraRejectsInvalidBooleanValues checks that an inline boolean
// value is only accepted when strconv.ParseBool accepts it, mirroring pflag.
func TestValidateExtraRejectsInvalidBooleanValues(t *testing.T) {
	if _, err := BuildReviewArgs(&ReviewIntent{Type: ReviewTypeWorkspace, Extra: []string{"--no-filter=not-a-bool"}}); err == nil {
		t.Error("review --no-filter=not-a-bool accepted, want error")
	}
	if _, err := BuildScanArgs(&ScanIntent{Extra: []string{"--no-plan=maybe"}}); err == nil {
		t.Error("scan --no-plan=maybe accepted, want error")
	}
	for _, ok := range []string{"--no-filter=true", "--no-filter=false", "--no-filter=1", "--no-filter=0"} {
		if _, err := BuildReviewArgs(&ReviewIntent{Type: ReviewTypeWorkspace, Extra: []string{ok}}); err != nil {
			t.Errorf("review %s rejected: %v", ok, err)
		}
	}
}
