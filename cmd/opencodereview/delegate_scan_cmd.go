// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/delegate"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/spf13/cobra"
)

// delegateScanOptions mirrors the subset of scanOptions that survives without
// an LLM. Everything the scan runtime needs only to drive its own model
// (concurrency, timeouts, token budgets, resume) is absent by design: under
// delegation the host agent owns the execution, so those knobs would be
// promises this command cannot keep.
type delegateScanOptions struct {
	repoDir        string
	paths          string
	excludes       string
	rulePath       string
	background     string
	backgroundFile string
	maxGitProcs    int
	batch          string
	batchSize      int
	format         string
	noRules        bool
}

var delegateScanOpts delegateScanOptions

var delegateScanCmd = &cobra.Command{
	Use:   "scan [flags]",
	Short: "Output full-file scan plan (files + batches + rules) for host-agent delegation",
	Long: `Outputs the deterministic full-file scan plan for the host agent to execute.

Unlike 'delegate preview', which describes a diff, this enumerates whole files
the way 'ocr scan' does — for auditing an unfamiliar codebase or a directory
with no meaningful diff. Files are grouped into the same batches 'ocr scan'
would dispatch, so the host agent can review one batch per sub-agent.

Review rules are resolved and printed alongside the plan (disable with
--no-rules), making this a single command that yields everything needed to
start scanning.`,
	Args: cobra.NoArgs,
	Example: `  # Plan a scan of the whole repository
  ocr delegate scan

  # Plan a scan of one directory, grouped by the directory each file lives in
  ocr delegate scan --path internal/agent --batch by-directory

  # Machine-readable plan for a scripted host agent
  ocr delegate scan --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateDelegateScanOptions(&delegateScanOpts); err != nil {
			return err
		}
		return executeDelegateScan(delegateScanOpts)
	},
}

func init() {
	registerDelegateScanFlags(delegateScanCmd, &delegateScanOpts)
	delegateCmd.AddCommand(delegateScanCmd)
}

func executeDelegateScan(opts delegateScanOptions) error {
	// requireGit is false to match 'ocr scan', which accepts a plain
	// directory. 'delegate preview' requires git because a diff needs it;
	// a full-file scan does not.
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, 0, opts.maxGitProcs, false)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// scan owns scan_template.json, separate from the diff-review template
	// loadCommonContext just read. MaxFileSizeBytes, BatchStrategy and
	// BatchSize all come from it, so the plan matches what 'ocr scan' would
	// actually dispatch.
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		return fmt.Errorf("load scan template: %w", err)
	}
	if err := scanTpl.Validate(); err != nil {
		return fmt.Errorf("invalid scan template: %w", err)
	}
	if opts.batch != "" {
		scanTpl.BatchStrategy = opts.batch
	}
	if opts.batchSize > 0 {
		scanTpl.BatchSize = opts.batchSize
	}

	background := opts.background
	if opts.backgroundFile != "" {
		fileBackground, err := loadBackgroundFile(resolveBackgroundFilePath(cc.RepoDir, opts.backgroundFile))
		if err != nil {
			return err
		}
		background = mergeBackground(background, fileBackground)
	}

	plan, err := scan.BuildPlan(context.Background(), scan.Args{
		RepoDir:          cc.RepoDir,
		Paths:            splitPaths(opts.paths),
		FileFilter:       cc.FileFilter,
		GitRunner:        cc.GitRunner,
		MaxFileSizeBytes: scanTpl.MaxFileSizeBytes,
		Template:         *scanTpl,
	})
	if err != nil {
		return fmt.Errorf("scan plan failed: %w", err)
	}

	var groups []delegate.RuleGroup
	if !opts.noRules && plan.ScannableCount > 0 {
		groups = delegate.GroupRules(cc.Resolver, plan.Paths())
	}

	if opts.format == "json" {
		return writeDelegateJSON(delegateScanJSON{
			SchemaVersion:  delegateSchemaVersion,
			Mode:           "scan",
			Repository:     cc.RepoDir,
			Background:     background,
			BatchStrategy:  plan.Strategy,
			BatchSize:      plan.BatchSize,
			TotalFiles:     plan.TotalFiles,
			ScannableCount: plan.ScannableCount,
			ExcludedCount:  plan.ExcludedCount,
			TotalLines:     plan.TotalLines,
			Batches:        scanBatchesJSON(plan),
			ExcludedFiles:  scanExcludedJSON(plan),
			RuleGroups:     ruleGroupsJSON(groups),
		})
	}

	printDelegateScanText(cc.RepoDir, background, plan, groups, opts.noRules)
	return nil
}

func printDelegateScanText(repoDir, background string, plan *scan.Plan, groups []delegate.RuleGroup, noRules bool) {
	fmt.Printf("# Scan Plan (%d scannable / %d total)\n\n", plan.ScannableCount, plan.TotalFiles)
	fmt.Printf("- mode: scan\n")
	fmt.Printf("- repository: %s\n", repoDir)
	fmt.Printf("- batch_strategy: %s\n", plan.Strategy)
	if plan.BatchSize > 0 {
		fmt.Printf("- batch_size: %d\n", plan.BatchSize)
	}
	fmt.Printf("- batches: %d\n", len(plan.Batches))
	fmt.Printf("- total_lines: %d\n", plan.TotalLines)
	if background != "" {
		fmt.Printf("- background: %s\n", background)
	}
	fmt.Println()

	for _, batch := range plan.Batches {
		label := ""
		if batch.Key != "" {
			label = fmt.Sprintf(" — %s", batch.Key)
		}
		fmt.Printf("## Batch %d%s (%d file(s), %d lines)\n\n", batch.ID, label, len(batch.Files), batch.TotalLines)
		for _, f := range batch.Files {
			fmt.Printf("- `%s` [%d lines]\n", f.Path, f.LineCount)
		}
		fmt.Println()
	}

	if plan.ExcludedCount > 0 {
		fmt.Printf("## Excluded (%d)\n\n", plan.ExcludedCount)
		for _, e := range plan.Excluded {
			fmt.Printf("~~- `%s` (excluded: %s)~~\n", e.Path, e.Reason)
		}
		fmt.Println()
	}

	if noRules {
		return
	}
	if len(groups) == 0 {
		return
	}
	fmt.Print(delegate.RuleGroupsMarkdown(groups))
}

type delegateScanFileJSON struct {
	Path      string `json:"path"`
	LineCount int    `json:"line_count"`
}

type delegateScanBatchJSON struct {
	BatchID    int                    `json:"batch_id"`
	Key        string                 `json:"key,omitempty"`
	TotalLines int                    `json:"total_lines"`
	Files      []delegateScanFileJSON `json:"files"`
}

type delegateScanExcludedJSON struct {
	Path          string `json:"path"`
	ExcludeReason string `json:"exclude_reason"`
}

type delegateScanJSON struct {
	SchemaVersion  string                     `json:"schema_version"`
	Mode           string                     `json:"mode"`
	Repository     string                     `json:"repository"`
	Background     string                     `json:"background,omitempty"`
	BatchStrategy  string                     `json:"batch_strategy"`
	BatchSize      int                        `json:"batch_size,omitempty"`
	TotalFiles     int                        `json:"total_files"`
	ScannableCount int                        `json:"scannable_count"`
	ExcludedCount  int                        `json:"excluded_count"`
	TotalLines     int                        `json:"total_lines"`
	Batches        []delegateScanBatchJSON    `json:"batches"`
	ExcludedFiles  []delegateScanExcludedJSON `json:"excluded_files"`
	RuleGroups     []delegateRuleGroupJSON    `json:"rule_groups"`
}

func scanBatchesJSON(plan *scan.Plan) []delegateScanBatchJSON {
	out := make([]delegateScanBatchJSON, 0, len(plan.Batches))
	for _, batch := range plan.Batches {
		files := make([]delegateScanFileJSON, 0, len(batch.Files))
		for _, f := range batch.Files {
			files = append(files, delegateScanFileJSON{Path: f.Path, LineCount: f.LineCount})
		}
		out = append(out, delegateScanBatchJSON{
			BatchID:    batch.ID,
			Key:        batch.Key,
			TotalLines: batch.TotalLines,
			Files:      files,
		})
	}
	return out
}

func scanExcludedJSON(plan *scan.Plan) []delegateScanExcludedJSON {
	out := make([]delegateScanExcludedJSON, 0, len(plan.Excluded))
	for _, e := range plan.Excluded {
		out = append(out, delegateScanExcludedJSON{Path: e.Path, ExcludeReason: string(e.Reason)})
	}
	return out
}
