// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

func TestResolveReviewMaxTools(t *testing.T) {
	tests := []struct {
		name            string
		templateDefault int
		cliValue        int
		want            int
	}{
		{name: "template default", templateDefault: 100, cliValue: 0, want: 100},
		{name: "minimum override", templateDefault: 100, cliValue: 50, want: 50},
		{name: "lower override", templateDefault: 100, cliValue: 75, want: 75},
		{name: "equal override", templateDefault: 100, cliValue: 100, want: 100},
		{name: "higher override", templateDefault: 100, cliValue: 125, want: 125},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveReviewMaxTools(tt.templateDefault, tt.cliValue); got != tt.want {
				t.Errorf("resolveReviewMaxTools() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateReviewRefsRejectsOptionLikeCommit(t *testing.T) {
	err := validateReviewRefs(t.TempDir(), reviewOptions{commit: "-O./pwn.sh"})
	if err == nil {
		t.Fatal("expected option-like --commit ref to be rejected")
	}
	if !strings.Contains(err.Error(), "--commit") || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewResultErrorUsesManifestTerminalState(t *testing.T) {
	for _, state := range []session.TerminalState{session.StateComplete, session.StatePartial, session.StateSkipped} {
		if err := reviewResultError(nil, &session.RunManifest{TerminalState: state}); err != nil {
			t.Errorf("state %q returned error: %v", state, err)
		}
	}
	if err := reviewResultError(nil, &session.RunManifest{TerminalState: session.StateFailed}); err == nil {
		t.Fatal("failed manifest must produce a process error")
	}
	err := reviewResultError(nil, &session.RunManifest{
		TerminalState: session.StateFailed,
		RunFailure:    &session.RunFailure{Classification: session.RunFailureInput, Reason: "diff resolution failed"},
	})
	if err == nil || !strings.Contains(err.Error(), string(session.RunFailureInput)) || !strings.Contains(err.Error(), "diff resolution failed") {
		t.Fatalf("run failure detail missing from error: %v", err)
	}
	err = reviewResultError(nil, &session.RunManifest{
		TerminalState: session.StateFailed,
		Coverage: session.Coverage{
			Selected: []session.CoverageItem{{ItemID: "a"}, {ItemID: "b"}},
			Failed:   []session.CoverageItem{{ItemID: "a"}, {ItemID: "b"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "2 of 2 selected item(s) failed") {
		t.Fatalf("failed item counts missing from error: %v", err)
	}
	// A controlled budget stop records no run_failure, so coverage alone decides.
	// With anything covered the manifest is partial and must exit 0.
	budgetPartial := &session.RunManifest{
		TerminalState: session.StatePartial,
		Coverage: session.Coverage{
			Selected:  []session.CoverageItem{{ItemID: "a"}, {ItemID: "b"}},
			Completed: []session.CoverageItem{{ItemID: "a"}},
			Failed:    []session.CoverageItem{{ItemID: "b", Classification: session.FailureBudget}},
		},
	}
	if err := reviewResultError(nil, budgetPartial); err != nil {
		t.Fatalf("budget stop with usable coverage must not produce a process error: %v", err)
	}
	// When the cap stopped the run before any file completed, every selected item
	// is failed(budget): no usable coverage, so it must exit non-zero even though
	// no run_failure was recorded. This boundary is deliberate, not incidental —
	// one covered item is the difference between exit 0 and exit non-zero.
	budgetAllFailed := &session.RunManifest{
		TerminalState: session.StateFailed,
		Coverage: session.Coverage{
			Selected: []session.CoverageItem{{ItemID: "a"}, {ItemID: "b"}},
			Failed: []session.CoverageItem{
				{ItemID: "a", Classification: session.FailureBudget},
				{ItemID: "b", Classification: session.FailureBudget},
			},
		},
	}
	if err := reviewResultError(nil, budgetAllFailed); err == nil ||
		!strings.Contains(err.Error(), "2 of 2 selected item(s) failed") {
		t.Fatalf("budget stop that covered nothing must produce a process error: %v", err)
	}
	want := errors.New("dispatch failed")
	if err := reviewResultError(want, budgetPartial); !errors.Is(err, want) {
		t.Fatalf("run error not preserved: %v", err)
	}
}

func TestValidateReviewRefsRejectsOptionLikeRangeRef(t *testing.T) {
	err := validateReviewRefs(t.TempDir(), reviewOptions{to: "-O./pwn.sh"})
	if err == nil {
		t.Fatal("expected option-like --to ref to be rejected")
	}
	if !strings.Contains(err.Error(), "--to") || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsRejectsToWithoutFrom(t *testing.T) {
	_, err := parseReviewFlags([]string{"--to", "HEAD"})
	if err == nil {
		t.Fatal("expected --to without --from to fail")
	}
	if !strings.Contains(err.Error(), "--from is required when --to is specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsRejectsFromWithoutTo(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main"})
	if err == nil {
		t.Fatal("expected --from without --to to fail")
	}
	if !strings.Contains(err.Error(), "--to is required when --from is specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A review that fails flag validation must exit before any session is created,
// so nothing is persisted under $HOME/.opencodereview (scenario 5: no artifacts).
func TestRunReviewFlagValidationWritesNoArtifacts(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	// --to without --from is rejected in parseReviewFlags, before loadCommonContext,
	// git resolution, session.New, or any manifest work.
	if err := runReview([]string{"--to", "HEAD"}); err == nil {
		t.Fatal("expected --to without --from to fail")
	}

	if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
		t.Fatalf("validation failure left artifacts under $HOME/.opencodereview (stat err = %v)", err)
	}
}

func TestParseReviewFlagsAllowsFromAndTo(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "HEAD"})
	if err != nil {
		t.Fatalf("expected --from/--to to pass, got: %v", err)
	}
	if opts.from != "main" || opts.to != "HEAD" {
		t.Fatalf("unexpected opts: from=%q to=%q", opts.from, opts.to)
	}
}
