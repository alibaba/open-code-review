// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/session"
)

// TestWarningsForOutputKeepsRecoverySignals pins the #1635 visibility
// contract: group_retry/group_split are recovery events, not coverage
// diagnostics — the manifest's coverage.failed set cannot express them, so
// they must survive the filter that drops subtask_error duplicates.
func TestWarningsForOutputKeepsRecoverySignals(t *testing.T) {
	warnings := []agent.AgentWarning{
		{Type: "subtask_error", File: "grp", Message: "context deadline exceeded"},
		{Type: "group_retry", File: "grp", Message: "group exceeded its deadline; retrying once (2 files)"},
		{Type: "group_split", File: "grp", Message: "group still exceeded its deadline after retry; splitting 2 files into single-file groups"},
		{Type: "comment_refiled", File: "a.go", Message: "re-filed"},
	}
	got := warningsForOutput(warnings, &session.RunManifest{})
	if len(got) != 3 {
		t.Fatalf("filtered = %d warnings (%v), want 3", len(got), got)
	}
	for _, w := range got {
		if w.Type == "subtask_error" {
			t.Fatalf("subtask_error should have been filtered, got %v", got)
		}
	}
}

// TestWarningsForOutputKeepsRecoverySignalsWithoutManifest covers the legacy
// path: with no manifest every warning passes through unchanged.
func TestWarningsForOutputKeepsRecoverySignalsWithoutManifest(t *testing.T) {
	warnings := []agent.AgentWarning{
		{Type: "group_retry", File: "grp", Message: "retrying"},
	}
	got := warningsForOutput(warnings, nil)
	if len(got) != 1 || got[0].Type != "group_retry" {
		t.Fatalf("got %v, want the group_retry warning unchanged", got)
	}
}
