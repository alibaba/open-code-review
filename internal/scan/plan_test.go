// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
)

// planTemplate returns a scan template with the batching knobs under test.
// MaxFileSizeBytes is set on Args by the caller; the provider reads it from
// there, not from the template.
func planTemplate(strategy string, size int) template.ScanTemplate {
	tpl := makeTemplateWithFullScan()
	tpl.BatchStrategy = strategy
	tpl.BatchSize = size
	return tpl
}

func planArgs(repo string, tpl template.ScanTemplate) Args {
	return Args{
		RepoDir:          repo,
		Template:         tpl,
		MaxFileSizeBytes: 1 << 20,
	}
}

func TestBuildPlan_GroupsByDirectoryAndRecordsExclusions(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "alpha/one.go", []byte("package alpha\n\nfunc One() {}\n"))
	writeFile(t, repo, "alpha/two.go", []byte("package alpha\n\nfunc Two() {}\n"))
	writeFile(t, repo, "beta/three.go", []byte("package beta\n"))
	// Binary content is excluded by whyExcluded, not by the enumerator.
	writeFile(t, repo, "beta/logo.png", []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02})
	gitCommit(t, repo, "init")

	plan, err := BuildPlan(t.Context(), planArgs(repo, planTemplate("by-directory", 0)))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if plan.Strategy != string(BatchByDirectory) {
		t.Errorf("Strategy = %q, want %q", plan.Strategy, BatchByDirectory)
	}
	if plan.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", plan.TotalFiles)
	}
	if plan.ScannableCount != 3 {
		t.Errorf("ScannableCount = %d, want 3", plan.ScannableCount)
	}
	if plan.ExcludedCount != 1 {
		t.Fatalf("ExcludedCount = %d, want 1", plan.ExcludedCount)
	}
	if got := plan.Excluded[0]; got.Path != "beta/logo.png" || got.Reason != model.ExcludeBinary {
		t.Errorf("Excluded[0] = %+v, want beta/logo.png/%s", got, model.ExcludeBinary)
	}

	// by-directory buckets on the first path segment, sorted by key.
	if len(plan.Batches) != 2 {
		t.Fatalf("len(Batches) = %d, want 2", len(plan.Batches))
	}
	if plan.Batches[0].Key != "alpha" || plan.Batches[1].Key != "beta" {
		t.Errorf("batch keys = %q/%q, want alpha/beta", plan.Batches[0].Key, plan.Batches[1].Key)
	}
	if plan.Batches[0].ID != 1 || plan.Batches[1].ID != 2 {
		t.Errorf("batch IDs = %d/%d, want 1/2", plan.Batches[0].ID, plan.Batches[1].ID)
	}
	if len(plan.Batches[0].Files) != 2 {
		t.Errorf("batch 1 files = %d, want 2", len(plan.Batches[0].Files))
	}

	// TotalLines is the sum over batches, which is the sum over files.
	var sum int
	for _, b := range plan.Batches {
		var batchSum int
		for _, f := range b.Files {
			if f.LineCount <= 0 {
				t.Errorf("file %s has LineCount %d, want > 0", f.Path, f.LineCount)
			}
			batchSum += f.LineCount
		}
		if b.TotalLines != batchSum {
			t.Errorf("batch %d TotalLines = %d, want %d", b.ID, b.TotalLines, batchSum)
		}
		sum += batchSum
	}
	if plan.TotalLines != sum {
		t.Errorf("TotalLines = %d, want %d", plan.TotalLines, sum)
	}
}

// The excluded set must never reach a batch: a delegation host iterates
// batches only, so a leak there would send a binary file to the LLM.
func TestBuildPlan_ExcludedFilesNeverAppearInBatches(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "keep.go", []byte("package main\n"))
	writeFile(t, repo, "drop.png", []byte{0x89, 'P', 'N', 'G', 0x00})
	gitCommit(t, repo, "init")

	plan, err := BuildPlan(t.Context(), planArgs(repo, planTemplate("by-language", 0)))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, path := range plan.Paths() {
		if path == "drop.png" {
			t.Fatalf("excluded file drop.png leaked into batches: %v", plan.Paths())
		}
	}
	if len(plan.Paths()) != plan.ScannableCount {
		t.Errorf("len(Paths()) = %d, want ScannableCount %d", len(plan.Paths()), plan.ScannableCount)
	}
}

func TestBuildPlan_BatchSizeChunksLargeGroups(t *testing.T) {
	repo := initTestRepo(t)
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go"} {
		writeFile(t, repo, name, []byte("package main\n"))
	}
	gitCommit(t, repo, "init")

	// All five share one by-language group; size 2 chunks it into 3 batches.
	plan, err := BuildPlan(t.Context(), planArgs(repo, planTemplate("by-language", 2)))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.BatchSize != 2 {
		t.Errorf("BatchSize = %d, want 2", plan.BatchSize)
	}
	if len(plan.Batches) != 3 {
		t.Fatalf("len(Batches) = %d, want 3", len(plan.Batches))
	}
	for _, b := range plan.Batches {
		if len(b.Files) > 2 {
			t.Errorf("batch %d has %d files, exceeds BatchSize 2", b.ID, len(b.Files))
		}
	}
	if len(plan.Paths()) != 5 {
		t.Errorf("Paths() = %d, want all 5 files preserved across chunks", len(plan.Paths()))
	}
}

// Under "none" every file is its own batch and the grouping key is just the
// path — carrying no information, so it is left empty rather than echoed.
func TestBuildPlan_NoneStrategyLeavesKeyEmpty(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package main\n"))
	writeFile(t, repo, "b.go", []byte("package main\n"))
	gitCommit(t, repo, "init")

	plan, err := BuildPlan(t.Context(), planArgs(repo, planTemplate("none", 0)))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Batches) != 2 {
		t.Fatalf("len(Batches) = %d, want 2 (one per file)", len(plan.Batches))
	}
	for _, b := range plan.Batches {
		if b.Key != "" {
			t.Errorf("batch %d Key = %q, want empty under the none strategy", b.ID, b.Key)
		}
	}
}

// Empty slices must survive JSON marshalling as [] rather than null, since
// delegation hosts iterate them without a nil check.
func TestBuildPlan_EmptyResultSlicesAreNonNil(t *testing.T) {
	repo := initTestRepo(t)

	plan, err := BuildPlan(t.Context(), planArgs(repo, planTemplate("by-language", 0)))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Batches == nil {
		t.Error("Batches must be non-nil even when empty (JSON would emit null)")
	}
	if plan.Excluded == nil {
		t.Error("Excluded must be non-nil even when empty (JSON would emit null)")
	}
	if plan.ScannableCount != 0 || plan.TotalFiles != 0 {
		t.Errorf("empty repo: ScannableCount=%d TotalFiles=%d, want 0/0", plan.ScannableCount, plan.TotalFiles)
	}
}

// BuildPlan must agree with Preview on what is scannable — they are the two
// no-LLM views of the same filter, and a delegation host that disagrees with
// `ocr scan --preview` would be reviewing a different file set than promised.
func TestBuildPlan_AgreesWithPreviewOnScannableSet(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "alpha/one.go", []byte("package alpha\n"))
	writeFile(t, repo, "beta/two.go", []byte("package beta\n"))
	writeFile(t, repo, "beta/logo.png", []byte{0x89, 'P', 'N', 'G', 0x00})
	gitCommit(t, repo, "init")

	args := planArgs(repo, planTemplate("by-directory", 0))
	plan, err := BuildPlan(t.Context(), args)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	preview, err := Preview(t.Context(), args)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if plan.TotalFiles != preview.TotalFiles {
		t.Errorf("TotalFiles: plan=%d preview=%d", plan.TotalFiles, preview.TotalFiles)
	}
	if plan.ScannableCount != preview.ReviewableCount {
		t.Errorf("scannable: plan=%d preview=%d", plan.ScannableCount, preview.ReviewableCount)
	}
	if plan.ExcludedCount != preview.ExcludedCount {
		t.Errorf("excluded: plan=%d preview=%d", plan.ExcludedCount, preview.ExcludedCount)
	}

	planned := make(map[string]bool, len(plan.Paths()))
	for _, p := range plan.Paths() {
		planned[p] = true
	}
	for _, entry := range preview.Entries {
		if entry.WillReview != planned[entry.Path] {
			t.Errorf("%s: preview.WillReview=%v but planned=%v", entry.Path, entry.WillReview, planned[entry.Path])
		}
	}
}
