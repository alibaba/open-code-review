// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/model"
)

// PlanFile is one scannable file in a batch.
type PlanFile struct {
	Path      string
	LineCount int
}

// PlanBatch is one dispatch unit: the set of files `ocr scan` would hand to a
// single sub-agent with its own isolated context. Delegation hosts should
// review one batch per sub-agent for the same reason — the batch is the unit
// the file grouping was designed around.
type PlanBatch struct {
	// ID is the 1-based batch number, stable for a given plan.
	ID int
	// Key is the grouping key that produced this batch (an extension for
	// by-language, the parent directory for by-directory). Empty under the
	// "none" strategy, where the key is just the file's own path and
	// carries no information.
	Key        string
	Files      []PlanFile
	TotalLines int
}

// ExcludedFile records one file the scan filter dropped, with the reason.
type ExcludedFile struct {
	Path   string
	Reason model.ExcludeReason
}

// Plan is the deterministic scan dispatch plan: which files `ocr scan` would
// read, and how it would group them for dispatch.
type Plan struct {
	Strategy       string
	BatchSize      int
	TotalFiles     int
	ScannableCount int
	ExcludedCount  int
	TotalLines     int
	Batches        []PlanBatch
	Excluded       []ExcludedFile
}

// BuildPlan enumerates files, applies the same reviewability filter as
// Preview, and groups the survivors into batches exactly as Agent.Run does.
//
// It dispatches no LLM call and builds no scan runtime — no session, no
// runner — so delegation hosts can call it freely without opening session
// persistence (see Preview for why going through NewAgent would not be).
//
// The per-file token ceiling (filterLargeScans) is deliberately NOT applied.
// That filter exists to keep a file's content inside the prompt budget of
// OCR's own LLM; under delegation the host agent reads the files itself, so
// dropping large files here would hide them from a host with room to spare.
// Preview makes the same choice, and the two stay consistent.
func BuildPlan(ctx context.Context, args Args) (*Plan, error) {
	a := &Agent{args: args}

	provider := NewProvider(args.RepoDir, args.Paths, args.GitRunner, args.MaxFileSizeBytes)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		return nil, fmt.Errorf("enumerate files: %w", err)
	}

	// Batches and Excluded are pre-allocated to non-nil empty slices so JSON
	// marshalling emits `[]` rather than `null` on an empty repo — downstream
	// delegation hosts iterate these without a nil check.
	plan := &Plan{
		TotalFiles: len(items),
		Batches:    make([]PlanBatch, 0),
		Excluded:   make([]ExcludedFile, 0),
	}

	scannable := make([]model.ScanItem, 0, len(items))
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if reason := a.whyExcluded(it); reason != model.ExcludeNone {
			plan.Excluded = append(plan.Excluded, ExcludedFile{Path: it.Path, Reason: reason})
			continue
		}
		scannable = append(scannable, it)
	}
	plan.ScannableCount = len(scannable)
	plan.ExcludedCount = len(plan.Excluded)

	strategy := parseBatchStrategy(args.Template.BatchStrategy)
	plan.Strategy = string(strategy)
	plan.BatchSize = args.Template.BatchSize

	keyOf := batchKeyFunc(strategy)
	for i, group := range groupBatches(scannable, strategy, args.Template.BatchSize) {
		batch := PlanBatch{ID: i + 1, Files: make([]PlanFile, 0, len(group))}
		if strategy != BatchNone && len(group) > 0 {
			batch.Key = keyOf(group[0])
		}
		for _, it := range group {
			batch.Files = append(batch.Files, PlanFile{Path: it.Path, LineCount: it.LineCount})
			batch.TotalLines += it.LineCount
		}
		plan.TotalLines += batch.TotalLines
		plan.Batches = append(plan.Batches, batch)
	}

	return plan, nil
}

// Paths returns every scannable path in batch order. Delegation hosts pass
// these straight to `ocr delegate rule`.
func (p *Plan) Paths() []string {
	out := make([]string, 0, p.ScannableCount)
	for _, batch := range p.Batches {
		for _, f := range batch.Files {
			out = append(out, f.Path)
		}
	}
	return out
}
