// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

// TestDisplayMode covers the empty and non-empty branches.
func TestDisplayMode(t *testing.T) {
	if got := displayMode(""); got != "-" {
		t.Errorf("displayMode(\"\") = %q, want -", got)
	}
	if got := displayMode("range"); got != "range" {
		t.Errorf("displayMode(range) = %q, want range", got)
	}
}

// TestDescribeRange covers each review-mode branch plus the fallthrough.
func TestDescribeRange(t *testing.T) {
	tests := []struct {
		name    string
		summary session.Summary
		want    string
	}{
		{
			name:    "range with endpoints",
			summary: session.Summary{ReviewMode: session.ReviewModeRange, DiffFrom: "a", DiffTo: "b"},
			want:    "a..b",
		},
		{
			name:    "range without endpoints",
			summary: session.Summary{ReviewMode: session.ReviewModeRange},
			want:    "-",
		},
		{
			name:    "commit",
			summary: session.Summary{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"},
			want:    "abc123",
		},
		{
			name:    "commit without value",
			summary: session.Summary{ReviewMode: session.ReviewModeCommit},
			want:    "-",
		},
		{
			name:    "other mode",
			summary: session.Summary{},
			want:    "-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeRange(tt.summary); got != tt.want {
				t.Errorf("describeRange() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDescribeStart covers the zero-time and formatted branches.
func TestDescribeStart(t *testing.T) {
	if got := describeStart(session.Summary{}); got != "-" {
		t.Errorf("describeStart(zero) = %q, want -", got)
	}
	s := session.Summary{}
	s.StartTime = s.StartTime.AddDate(2024, 0, 0) // any non-zero time
	if got := describeStart(s); got == "-" {
		t.Error("describeStart(non-zero) should not be -")
	}
}

// TestDescribeFilesNoManifest covers the branch where RunManifest is nil.
func TestDescribeFilesNoManifest(t *testing.T) {
	s := session.Summary{CompletedFiles: 3}
	if got := describeFiles(s); got != "3" {
		t.Errorf("describeFiles(no manifest) = %q, want 3", got)
	}
	s.ReusedFiles = 2
	if got := describeFiles(s); got != "5 (reused 2)" {
		t.Errorf("describeFiles(reused) = %q, want 5 (reused 2)", got)
	}
}

// TestCompleteEnum verifies the closure returns the provided values with the
// no-file-completion directive.
func TestCompleteEnum(t *testing.T) {
	fn := completeEnum("a", "b", "c")
	values, directive := fn(nil, nil, "")
	if len(values) != 3 || values[0] != "a" {
		t.Errorf("completeEnum values = %v, want [a b c]", values)
	}
	if directive == 0 {
		t.Error("expected a non-zero shell completion directive")
	}
}
