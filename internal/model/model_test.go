package model

import (
	"encoding/json"
	"testing"
)

func TestDiff_JSONRoundTrip(t *testing.T) {
	d := Diff{
		OldPath:    "a.go",
		NewPath:    "b.go",
		Diff:       "@@ -1 +1 @@\n-old\n+new",
		IsBinary:   false,
		IsDeleted:  false,
		IsNew:      true,
		IsRenamed:  true,
		Insertions: 5,
		Deletions:  3,
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Diff
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != d {
		t.Errorf("roundtrip mismatch:\n  got  %+v\n  want %+v", got, d)
	}
}

func TestPreviewEntry_JSONRoundTrip(t *testing.T) {
	e := PreviewEntry{
		Path:          "main.go",
		Status:        "modified",
		Insertions:    10,
		Deletions:     2,
		WillReview:    true,
		ExcludeReason: ExcludeNone,
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PreviewEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != e {
		t.Errorf("roundtrip mismatch:\n  got  %+v\n  want %+v", got, e)
	}
}

func TestPreviewEntry_ExcludeReasonOmitEmpty(t *testing.T) {
	e := PreviewEntry{
		Path:          "a.go",
		ExcludeReason: ExcludeNone,
	}
	data, _ := json.Marshal(e)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["exclude_reason"]; ok {
		t.Error("expected exclude_reason to be omitted when empty")
	}

	e.ExcludeReason = ExcludeUserRule
	data, _ = json.Marshal(e)
	json.Unmarshal(data, &m)
	if m["exclude_reason"] != "user_exclude" {
		t.Errorf("expected exclude_reason=user_exclude, got %v", m["exclude_reason"])
	}
}

func TestExcludeReasonConstants(t *testing.T) {
	constants := map[ExcludeReason]string{
		ExcludeNone:        "",
		ExcludeUserRule:    "user_exclude",
		ExcludeExtension:   "unsupported_ext",
		ExcludeDefaultPath: "default_path",
		ExcludeDeleted:     "deleted",
		ExcludeBinary:      "binary",
	}
	for k, v := range constants {
		if string(k) != v {
			t.Errorf("ExcludeReason constant mismatch: got %q, want %q", string(k), v)
		}
	}
}

func TestScanItem_AsDiff(t *testing.T) {
	item := &ScanItem{
		Path:      "file.go",
		Content:   "package main\n",
		IsBinary:  false,
		LineCount: 1,
	}

	d := item.AsDiff()
	if d == nil {
		t.Fatal("expected non-nil Diff")
	}
	if d.OldPath != "file.go" {
		t.Errorf("OldPath = %q, want file.go", d.OldPath)
	}
	if d.NewPath != "file.go" {
		t.Errorf("NewPath = %q, want file.go", d.NewPath)
	}
	if d.NewFileContent != "package main\n" {
		t.Errorf("NewFileContent = %q", d.NewFileContent)
	}
	if d.IsBinary {
		t.Error("expected IsBinary=false")
	}
	if d.Insertions != 1 {
		t.Errorf("Insertions = %d, want 1", d.Insertions)
	}
}

func TestScanItem_AsDiff_Nil(t *testing.T) {
	var item *ScanItem
	d := item.AsDiff()
	if d != nil {
		t.Error("expected nil Diff for nil ScanItem")
	}
}

func TestScanItem_AsDiff_Binary(t *testing.T) {
	item := &ScanItem{
		Path:     "image.png",
		IsBinary: true,
	}
	d := item.AsDiff()
	if !d.IsBinary {
		t.Error("expected IsBinary=true")
	}
}

func TestLlmComment_JSON(t *testing.T) {
	c := LlmComment{
		Path:           "main.go",
		Content:        "fix this",
		SuggestionCode: "new code",
		ExistingCode:   "old code",
		StartLine:      10,
		EndLine:        15,
		Thinking:       "reasoning",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got LlmComment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != c {
		t.Errorf("roundtrip mismatch:\n  got  %+v\n  want %+v", got, c)
	}
}

func TestCodeReviewResult_JSON(t *testing.T) {
	r := CodeReviewResult{
		RelevantFile:      "api.go",
		SuggestionContent: "suggestion",
		ExistingCode:      "old",
		SuggestionCode:    "new",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got CodeReviewResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != r {
		t.Errorf("roundtrip mismatch:\n  got  %+v\n  want %+v", got, r)
	}
}
