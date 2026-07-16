package main

import (
	"context"
	"fmt"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/delegate"
	"github.com/open-code-review/open-code-review/internal/diff"
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
	showHelp       bool
}

func runDelegate(args []string) error {
	if len(args) == 0 {
		printDelegateUsage()
		return nil
	}

	sub := args[0]
	switch sub {
	case "-h", "--help":
		printDelegateUsage()
		return nil
	case "preview":
		return runDelegatePreview(args[1:])
	case "rule":
		return runDelegateRule(args[1:])
	default:
		return fmt.Errorf("unknown delegate sub-command: %s\nRun 'ocr delegate -h' for usage", sub)
	}
}

func parseDelegateFlags(args []string) (delegateOptions, []string, error) {
	a := newOcrFlagSet("ocr delegate")

	opts := delegateOptions{}
	a.StringVar(&opts.repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.StringVar(&opts.from, "from", "", "source ref to start diff from (e.g., 'main')")
	a.StringVar(&opts.to, "to", "", "target ref to end diff at (e.g., 'feature-branch')")
	a.StringVarP(&opts.commit, "commit", "c", "", "single commit hash or tag to review (vs its parent)")
	a.StringVar(&opts.excludes, "exclude", "", "comma-separated gitignore-style patterns to exclude")
	a.StringVar(&opts.rulePath, "rule", "", "path to JSON file with system review rules")
	a.StringVarP(&opts.background, "background", "b", "", "optional requirement/business context")
	a.StringVarP(&opts.backgroundFile, "background-file", "B", "", "path to a Markdown file used as background")
	a.IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")

	if err := a.Parse(args); err != nil {
		return opts, nil, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.showHelp {
		return opts, nil, nil
	}

	// Validate mode exclusivity
	modeCount := 0
	if opts.from != "" || opts.to != "" {
		modeCount++
	}
	if opts.commit != "" {
		modeCount++
	}
	if modeCount > 1 {
		return opts, nil, fmt.Errorf("only one review mode allowed (--from/--to or --commit)")
	}
	if opts.from != "" && opts.to == "" {
		return opts, nil, fmt.Errorf("--to is required when --from is specified")
	}
	if opts.to != "" && opts.from == "" {
		return opts, nil, fmt.Errorf("--from is required when --to is specified")
	}

	remaining := a.fs.Args()
	return opts, remaining, nil
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

	// Resolve background from --background-file if set.
	if opts.backgroundFile != "" {
		bgPath := resolveBackgroundFilePath(cc.RepoDir, opts.backgroundFile)
		fileBackground, err := loadBackgroundFile(bgPath)
		if err != nil {
			return nil, err
		}
		opts.background = mergeBackground(opts.background, fileBackground)
	}

	// Auto-fill background from commit message when reviewing a single commit.
	if opts.commit != "" && opts.background == "" {
		if msg, err := getCommitMessage(cc.RepoDir, opts.commit); err == nil && msg != "" {
			opts.background = msg
		}
	}

	return &delegateContext{cc: cc, opts: opts}, nil
}

// preview runs the agent's file-selection logic and returns the preview result.
func (dc *delegateContext) preview(ctx context.Context) (*agent.DiffPreview, error) {
	ag := agent.New(agent.Args{
		RepoDir:    dc.cc.RepoDir,
		From:       dc.opts.from,
		To:         dc.opts.to,
		Commit:     dc.opts.commit,
		FileFilter: dc.cc.FileFilter,
		GitRunner:  dc.cc.GitRunner,
	})
	return ag.Preview(ctx)
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

func runDelegatePreview(args []string) error {
	opts, _, err := parseDelegateFlags(args)
	if err != nil {
		return err
	}
	if opts.showHelp {
		fmt.Println("Usage: ocr delegate preview [flags]")
		fmt.Println("\nOutputs reviewable file list with mode/ref metadata for the host agent to construct git commands.")
		return nil
	}

	dc, err := loadDelegateContext(opts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	preview, err := dc.preview(ctx)
	if err != nil {
		return fmt.Errorf("preview failed: %w", err)
	}

	// Header with mode info
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
	if mergeBase := dc.mergeBase(ctx); mergeBase != "" {
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

func runDelegateRule(args []string) error {
	opts, remaining, err := parseDelegateFlags(args)
	if err != nil {
		return err
	}
	if opts.showHelp {
		fmt.Println("Usage: ocr delegate rule [flags] <path...>")
		fmt.Println("\nOutputs resolved review rules grouped by content. Accepts multiple paths.")
		return nil
	}
	if len(remaining) == 0 {
		return fmt.Errorf("at least one file path is required\nUsage: ocr delegate rule [flags] <path...>")
	}

	dc, err := loadDelegateContext(opts)
	if err != nil {
		return err
	}

	groups := delegate.GroupRules(dc.resolver(), remaining)
	fmt.Print(delegate.RuleGroupsMarkdown(groups))
	return nil
}

func printDelegateUsage() {
	fmt.Println(`OpenCodeReview - Delegation Mode

Usage:
  ocr delegate <sub-command> [flags]
  ocr d <sub-command> [flags]       (alias)

Sub-commands:
  preview       Preview reviewable files with mode/ref metadata
  rule          Output resolved review rules grouped by content

Shared Flags:
  --from string           source ref to start diff from (e.g., 'main')
  --to string             target ref to end diff at (e.g., 'feature-branch')
  -c, --commit string     single commit hash or tag to review
  --repo string           root directory of the git repository (default: current dir)
  --rule string           path to JSON file with system review rules
  --exclude string        comma-separated gitignore-style patterns to exclude
  -b, --background string optional requirement/business context
  -B, --background-file   path to a Markdown file used as background
  --max-git-procs int     max concurrent git subprocesses (default 16)

Examples:
  # Preview which files will be reviewed
  ocr delegate preview --from main --to feature

  # Preview workspace changes
  ocr delegate preview

  # Get rules for multiple files (grouped by content)
  ocr delegate rule internal/agent/agent.go internal/llm/client.go`)
}
