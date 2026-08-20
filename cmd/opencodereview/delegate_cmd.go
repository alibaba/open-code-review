// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/delegate"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/spf13/cobra"
)

type delegateOptions struct {
	repoDir        string
	from           string
	to             string
	commit         string
	excludes       string
	rulePath       string
	background     string
	backgroundFile string
	maxGitProcs    int
	format         string
}

var delegatePreviewOpts delegateOptions
var delegateRuleOpts delegateOptions

var delegateCmd = &cobra.Command{
	Use:     "delegate",
	Aliases: []string{"d"},
	Short:   "Output review spec for host-agent delegation (no LLM required)",
	Long: `OpenCodeReview - Delegation Mode

Output review spec for host-agent delegation (no LLM required).`,
	Example: `  # Preview which files will be reviewed
  ocr delegate preview --from main --to feature

  # Preview workspace changes
  ocr delegate preview

  # Get rules for multiple files (grouped by content)
  ocr delegate rule internal/agent/agent.go internal/llm/client.go`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var delegatePreviewCmd = &cobra.Command{
	Use:   "preview [flags]",
	Short: "Preview reviewable files with mode/ref metadata",
	Long:  "Outputs reviewable file list with mode/ref metadata for the host agent to construct git commands.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateDelegateOptions(&delegatePreviewOpts); err != nil {
			return err
		}
		return executeDelegatePreview(delegatePreviewOpts)
	},
}

var delegateRuleCmd = &cobra.Command{
	Use:   "rule [flags] <path...>",
	Short: "Output resolved review rules grouped by content",
	Long:  "Outputs resolved review rules grouped by content. Accepts multiple paths.",
	Args:  minimumArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateDelegateOptions(&delegateRuleOpts); err != nil {
			return err
		}
		return executeDelegateRule(delegateRuleOpts, args)
	},
}

func init() {
	registerDelegateFlags(delegatePreviewCmd, &delegatePreviewOpts)
	registerDelegateFlags(delegateRuleCmd, &delegateRuleOpts)
	delegateCmd.AddCommand(delegatePreviewCmd)
	delegateCmd.AddCommand(delegateRuleCmd)
}

// delegateContext holds the shared state for delegate sub-commands.
type delegateContext struct {
	cc   *commonContext
	opts delegateOptions
}

func loadDelegateContext(opts delegateOptions) (*delegateContext, error) {
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, 0, opts.maxGitProcs, true)
	if err != nil {
		return nil, err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// Security: reject ref-option injection.
	reviewOpts := reviewOptions{from: opts.from, to: opts.to, commit: opts.commit}
	if err := validateReviewRefs(cc.RepoDir, reviewOpts); err != nil {
		return nil, err
	}

	bg, err := resolveBackground(cc.RepoDir, opts.background, opts.backgroundFile, opts.commit)
	if err != nil {
		return nil, err
	}
	opts.background = bg

	return &delegateContext{cc: cc, opts: opts}, nil
}

// preview runs the agent's file-selection logic and returns the preview result.
func (dc *delegateContext) preview(ctx context.Context) (*agent.DiffPreview, error) {
	return agent.Preview(ctx, agent.Args{
		RepoDir:    dc.cc.RepoDir,
		From:       dc.opts.from,
		To:         dc.opts.to,
		Commit:     dc.opts.commit,
		FileFilter: dc.cc.FileFilter,
		GitRunner:  dc.cc.GitRunner,
	})
}

// mergeBase computes the merge-base for range mode. Returns "" for other modes.
func (dc *delegateContext) mergeBase(ctx context.Context) string {
	if dc.opts.from == "" || dc.opts.to == "" {
		return ""
	}
	provider := diff.NewProvider(dc.cc.RepoDir, dc.opts.from, dc.opts.to, dc.cc.GitRunner)
	return provider.MergeBase(ctx)
}

// reviewMode returns the string mode identifier.
func (dc *delegateContext) reviewMode() string {
	switch {
	case dc.opts.commit != "":
		return "commit"
	case dc.opts.from != "" && dc.opts.to != "":
		return "range"
	default:
		return "workspace"
	}
}

// resolver returns the rules Resolver, asserting DetailResolver if available.
func (dc *delegateContext) resolver() rules.Resolver {
	return dc.cc.Resolver
}

func executeDelegatePreview(opts delegateOptions) error {
	dc, err := loadDelegateContext(opts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	preview, err := dc.preview(ctx)
	if err != nil {
		return fmt.Errorf("preview failed: %w", err)
	}
	mergeBase := dc.mergeBase(ctx)
	if opts.format == "json" {
		return writeDelegateJSON(delegatePreviewJSON{
			SchemaVersion:   delegateSchemaVersion,
			Mode:            dc.reviewMode(),
			Repository:      dc.cc.RepoDir,
			From:            dc.opts.from,
			To:              dc.opts.to,
			Commit:          dc.opts.commit,
			MergeBase:       mergeBase,
			Background:      dc.opts.background,
			TotalFiles:      preview.TotalFiles,
			ReviewableCount: preview.ReviewableCount,
			ExcludedCount:   preview.ExcludedCount,
			TotalInsertions: preview.TotalInsertions,
			TotalDeletions:  preview.TotalDeletions,
			ReviewableFiles: previewFiles(preview, true),
			ExcludedFiles:   previewFiles(preview, false),
		})
	}

	fmt.Printf("# Files (%d reviewable / %d total)\n\n", preview.ReviewableCount, preview.TotalFiles)
	fmt.Printf("- mode: %s\n", dc.reviewMode())
	if dc.opts.from != "" {
		fmt.Printf("- from: %s\n", dc.opts.from)
	}
	if dc.opts.to != "" {
		fmt.Printf("- to: %s\n", dc.opts.to)
	}
	if dc.opts.commit != "" {
		fmt.Printf("- commit: %s\n", dc.opts.commit)
	}
	if mergeBase != "" {
		fmt.Printf("- merge_base: %s\n", mergeBase)
	}
	if dc.opts.background != "" {
		fmt.Printf("- background: %s\n", dc.opts.background)
	}
	fmt.Printf("- total_insertions: %d\n", preview.TotalInsertions)
	fmt.Printf("- total_deletions: %d\n\n", preview.TotalDeletions)

	for _, entry := range preview.Entries {
		marker := "  "
		if !entry.WillReview {
			marker = "~~"
		}
		fmt.Printf("%s- `%s` [%s] +%d/-%d", marker, entry.Path, entry.Status, entry.Insertions, entry.Deletions)
		if !entry.WillReview {
			fmt.Printf(" (excluded: %s)", entry.ExcludeReason)
			fmt.Print("~~")
		}
		fmt.Println()
	}

	return nil
}

func executeDelegateRule(opts delegateOptions, paths []string) error {
	dc, err := loadDelegateContext(opts)
	if err != nil {
		return err
	}

	groups := delegate.GroupRules(dc.resolver(), paths)
	if opts.format == "json" {
		return writeDelegateJSON(delegateRulesJSON{
			SchemaVersion: delegateSchemaVersion,
			Groups:        ruleGroupsJSON(groups),
		})
	}
	fmt.Print(delegate.RuleGroupsMarkdown(groups))
	return nil
}

const delegateSchemaVersion = "1"

type delegatePreviewFileJSON struct {
	Path          string `json:"path"`
	Status        string `json:"status"`
	Insertions    int64  `json:"insertions"`
	Deletions     int64  `json:"deletions"`
	ExcludeReason string `json:"exclude_reason,omitempty"`
}

type delegatePreviewJSON struct {
	SchemaVersion   string                    `json:"schema_version"`
	Mode            string                    `json:"mode"`
	Repository      string                    `json:"repository"`
	From            string                    `json:"from,omitempty"`
	To              string                    `json:"to,omitempty"`
	Commit          string                    `json:"commit,omitempty"`
	MergeBase       string                    `json:"merge_base,omitempty"`
	Background      string                    `json:"background,omitempty"`
	TotalFiles      int                       `json:"total_files"`
	ReviewableCount int                       `json:"reviewable_count"`
	ExcludedCount   int                       `json:"excluded_count"`
	TotalInsertions int64                     `json:"total_insertions"`
	TotalDeletions  int64                     `json:"total_deletions"`
	ReviewableFiles []delegatePreviewFileJSON `json:"reviewable_files"`
	ExcludedFiles   []delegatePreviewFileJSON `json:"excluded_files"`
}

type delegateRuleGroupJSON struct {
	GroupID int      `json:"group_id"`
	Source  string   `json:"source"`
	Pattern string   `json:"pattern"`
	Files   []string `json:"files"`
	Rule    string   `json:"rule"`
}

type delegateRulesJSON struct {
	SchemaVersion string                  `json:"schema_version"`
	Groups        []delegateRuleGroupJSON `json:"groups"`
}

func previewFiles(preview *agent.DiffPreview, reviewable bool) []delegatePreviewFileJSON {
	files := make([]delegatePreviewFileJSON, 0)
	for _, entry := range preview.Entries {
		if entry.WillReview != reviewable {
			continue
		}
		files = append(files, delegatePreviewFileJSON{
			Path:          entry.Path,
			Status:        entry.Status,
			Insertions:    entry.Insertions,
			Deletions:     entry.Deletions,
			ExcludeReason: string(entry.ExcludeReason),
		})
	}
	return files
}

func ruleGroupsJSON(groups []delegate.RuleGroup) []delegateRuleGroupJSON {
	out := make([]delegateRuleGroupJSON, 0, len(groups))
	for _, group := range groups {
		files := make([]string, 0, len(group.Files))
		files = append(files, group.Files...)
		out = append(out, delegateRuleGroupJSON{
			GroupID: group.ID,
			Source:  group.Source,
			Pattern: group.Pattern,
			Files:   files,
			Rule:    group.Text,
		})
	}
	return out
}

func writeDelegateJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
