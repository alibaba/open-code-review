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

func lcCode(path, content, existingCode string, start, end int, severity string) model.LlmComment {
	c := lc(path, content, start, end, severity)
	c.ExistingCode = existingCode
	return c
}

func TestDedupByLocation_MergesSameLocation(t *testing.T) {
	// Two paraphrasings of the same real-world finding (see the bug report):
	// missing isfield guard before accessing config.asdf.skip_DB_connection.
	in := []model.LlmComment{
		lc("Models/asdf.m",
			"The code accesses config.asdf before verifying that the asdf field exists in config. "+
				"If config.asdf is missing, this will raise a runtime error before the later assert(isfield(config, 'asdf'), ...) "+
				"can catch the problem. Add an existence check or move the assert before the conditional.",
			20, 20, "high"),
		lc("Models/asdf.m",
			"The code accesses config.asdf.skip_DB_connection before confirming the asdf field exists, "+
				"which can cause a runtime error when the field is missing. Move the existence check before "+
				"the conditional or include an isfield guard.",
			20, 20, "high"),
		lc("Models/qwertz.m", "Using numel is clearer and safer than nnz for this loop bound.", 230, 230, "low"),
	}
	got := DedupByLocation(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 comments after dedup, got %d: %+v", len(got), got)
	}
	if got[0].Path != "Models/asdf.m" {
		t.Errorf("expected the asdf.m pair merged into one, got %+v", got[0])
	}
}

// TestDedupByLocation_DistinctFindingsSameRangeBothSurvive is the regression
// test requested in review: two independent, valid findings that happen to
// share a line range (unsafe SQL construction vs. a separately-ignored
// database error) must both survive — location overlap alone must never
// collapse them.
func TestDedupByLocation_DistinctFindingsSameRangeBothSurvive(t *testing.T) {
	in := []model.LlmComment{
		lc("db/query.go",
			"User input is concatenated directly into the SQL query string, allowing SQL injection. "+
				"Use a parameterized query instead of building the statement with fmt.Sprintf.",
			42, 45, "critical"),
		lc("db/query.go",
			"The error returned by db.Exec is discarded here. A failed write will be silently ignored, "+
				"which can mask data-loss or connectivity problems in production.",
			42, 45, "high"),
	}
	got := DedupByLocation(in)
	if len(got) != 2 {
		t.Fatalf("expected both distinct findings to survive, got %d: %+v", len(got), got)
	}
}

func TestDedupByLocation_KeepsHigherSeverity(t *testing.T) {
	in := []model.LlmComment{
		lc("a.go",
			"The nil check for cfg is missing before cfg.Timeout is read, which can panic.",
			10, 12, "low"),
		lc("a.go",
			"cfg.Timeout is read without a preceding nil check on cfg, causing a possible nil pointer panic.",
			11, 13, "critical"),
	}
	got := DedupByLocation(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 comment, got %d: %+v", len(got), got)
	}
	if got[0].Severity != "critical" {
		t.Errorf("expected higher-severity comment kept, got %+v", got[0])
	}
}

func TestDedupByLocation_MergesViaExistingCodeWhenWordingDiffers(t *testing.T) {
	// Deliberately low-vocabulary-overlap phrasing, but both comments quote
	// the exact same existing_code snippet — the fallback signal.
	snippet := "if ~config.asdf.skip_DB_connection"
	in := []model.LlmComment{
		lcCode("a.m", "Guard this.", snippet, 20, 20, "medium"),
		lcCode("a.m", "Needs a check first.", snippet, 20, 20, "medium"),
	}
	got := DedupByLocation(in)
	if len(got) != 1 {
		t.Errorf("expected existing_code match to merge despite differing wording, got %d: %+v", len(got), got)
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