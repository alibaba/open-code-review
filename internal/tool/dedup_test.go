// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func lc(path, content string, start, end int, severity string) model.LlmComment {
	return model.LlmComment{Path: path, Content: content, StartLine: start, EndLine: end, Severity: severity}
}

func TestDedupByLocation_MergesSameLocation(t *testing.T) {
	in := []model.LlmComment{
		lc("Models/asdf.m", "first phrasing", 20, 20, "high"),
		lc("Models/asdf.m", "second phrasing, same finding", 20, 20, "high"),
		lc("Models/qwertz.m", "unrelated", 230, 230, "low"),
	}
	got := DedupByLocation(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 comments after dedup, got %d: %+v", len(got), got)
	}
	if got[0].Path != "Models/asdf.m" || got[0].Content != "first phrasing" {
		t.Errorf("expected first-seen kept when severities tie, got %+v", got[0])
	}
}

func TestDedupByLocation_KeepsHigherSeverity(t *testing.T) {
	in := []model.LlmComment{
		lc("a.go", "low sev finding", 10, 12, "low"),
		lc("a.go", "critical sev finding, same spot", 11, 13, "critical"),
	}
	got := DedupByLocation(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(got))
	}
	if got[0].Severity != "critical" || got[0].Content != "critical sev finding, same spot" {
		t.Errorf("expected higher-severity comment kept, got %+v", got[0])
	}
}

func TestDedupByLocation_NonOverlappingLinesNotMerged(t *testing.T) {
	in := []model.LlmComment{
		lc("a.go", "issue near top", 1, 2, "medium"),
		lc("a.go", "issue near bottom", 50, 52, "medium"),
	}
	got := DedupByLocation(in)
	if len(got) != 2 {
		t.Errorf("expected non-overlapping ranges to stay separate, got %d", len(got))
	}
}

func TestDedupByLocation_DifferentPathsNotMerged(t *testing.T) {
	in := []model.LlmComment{
		lc("a.go", "same content", 10, 10, "high"),
		lc("b.go", "same content", 10, 10, "high"),
	}
	got := DedupByLocation(in)
	if len(got) != 2 {
		t.Errorf("expected different files to stay separate, got %d", len(got))
	}
}

func TestDedupByLocation_UnsetLineRangesNotMerged(t *testing.T) {
	in := []model.LlmComment{
		lc("a.go", "file-level comment 1", 0, 0, "low"),
		lc("a.go", "file-level comment 2", 0, 0, "low"),
	}
	got := DedupByLocation(in)
	if len(got) != 2 {
		t.Errorf("expected unset line ranges to never merge, got %d", len(got))
	}
}

func TestDedupByLocation_EmptyAndSingleInputPassThrough(t *testing.T) {
	if got := DedupByLocation(nil); len(got) != 0 {
		t.Errorf("expected empty input to pass through, got %+v", got)
	}
	single := []model.LlmComment{lc("a.go", "x", 1, 1, "")}
	got := DedupByLocation(single)
	if len(got) != 1 || got[0].Content != "x" {
		t.Errorf("expected single comment passthrough, got %+v", got)
	}
}