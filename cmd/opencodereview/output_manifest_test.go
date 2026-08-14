// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/session"
)

// TestWarningsForOutput covers warningsForOutput: the early pass-through when
// there is no manifest, the all-filtered case that collapses to nil, and the
// mixed case that keeps only non-subtask warnings.
func TestWarningsForOutput(t *testing.T) {
	warns := []agent.AgentWarning{
		{Type: "subtask_error"},
		{Type: "scan_subtask_error"},
		{Type: "token_budget_reached"},
	}
	manifest := &session.RunManifest{TerminalState: session.StateComplete}

	t.Run("nil manifest passes through unchanged", func(t *testing.T) {
		got := warningsForOutput(warns, nil)
		if len(got) != len(warns) {
			t.Errorf("got %d warnings, want %d", len(got), len(warns))
		}
	})

	t.Run("all subtask errors collapse to nil", func(t *testing.T) {
		only := []agent.AgentWarning{{Type: "subtask_error"}, {Type: "scan_subtask_error"}}
		if got := warningsForOutput(only, manifest); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("mixed keeps only non-subtask warnings", func(t *testing.T) {
		got := warningsForOutput(warns, manifest)
		if len(got) != 1 || got[0].Type != "token_budget_reached" {
			t.Errorf("expected only token_budget_reached, got %v", got)
		}
	})
}

// TestManifestMessage covers every terminal-state branch of manifestMessage,
// including the waived-count variant of a complete run and the failed run with
// and without a recorded RunFailure classification.
func TestManifestMessage(t *testing.T) {
	items := func(n int) []session.CoverageItem {
		return make([]session.CoverageItem, n)
	}

	t.Run("nil manifest is empty", func(t *testing.T) {
		if got := manifestMessage(nil, 0); got != "" {
			t.Errorf("nil manifest = %q, want empty", got)
		}
	})

	cases := []struct {
		name     string
		manifest *session.RunManifest
		findings int
		want     string // substring the message must contain
	}{
		{
			name: "complete without waived",
			manifest: &session.RunManifest{
				TerminalState: session.StateComplete,
				Coverage:      session.Coverage{Selected: items(3)},
			},
			findings: 2,
			want:     "Review complete: 2 finding(s) across 3 selected item(s).",
		},
		{
			name: "complete with waived",
			manifest: &session.RunManifest{
				TerminalState: session.StateComplete,
				Coverage:      session.Coverage{Selected: items(3), Waived: items(1)},
			},
			findings: 2,
			want:     "including 1 waived",
		},
		{
			name: "partial",
			manifest: &session.RunManifest{
				TerminalState: session.StatePartial,
				Coverage:      session.Coverage{Selected: items(4), Failed: items(1)},
			},
			findings: 1,
			want:     "partially complete",
		},
		{
			name: "failed with classification",
			manifest: &session.RunManifest{
				TerminalState: session.StateFailed,
				Coverage:      session.Coverage{Selected: items(2), Failed: items(2)},
				RunFailure:    &session.RunFailure{Classification: session.RunFailureInput},
			},
			findings: 0,
			want:     "Review failed (input)",
		},
		{
			name: "failed without classification",
			manifest: &session.RunManifest{
				TerminalState: session.StateFailed,
				Coverage:      session.Coverage{Selected: items(2), Failed: items(2)},
			},
			findings: 0,
			want:     "Review failed: 0 finding(s)",
		},
		{
			name: "skipped",
			manifest: &session.RunManifest{
				TerminalState: session.StateSkipped,
			},
			want: "Review skipped",
		},
		{
			name: "unknown state falls through",
			manifest: &session.RunManifest{
				TerminalState: session.TerminalState("bogus"),
			},
			want: "unknown manifest state",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := manifestMessage(tc.manifest, tc.findings)
			if !strings.Contains(got, tc.want) {
				t.Errorf("manifestMessage = %q, want substring %q", got, tc.want)
			}
		})
	}
}
