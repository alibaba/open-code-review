package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/config/toolsconfig"
	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/stdout"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
)

func runReview(args []string) error {
	opts, err := parseReviewFlags(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if opts.showHelp {
		printReviewUsage()
		return nil
	}

	// Resolve GitLab options early for clear/publish operations.
	if hasGitLabOperation(opts) {
		token := firstNonEmpty(os.Getenv("GITLAB_TOKEN"), os.Getenv("OCR_GITLAB_TOKEN"))
		pubOpts, err := resolveGitLabReviewOptions(opts, token)
		if err != nil {
			return fmt.Errorf("gitlab options: %w", err)
		}

		// Clear operations exit early without running review.
		if opts.clearInline || opts.clearSummary {
			return runGitLabClear(pubOpts)
		}
	}

	if err := requireGitRepo(opts.repoDir); err != nil {
		return err
	}

	tpl, err := template.LoadDefault()
	if err != nil {
		return fmt.Errorf("load default template: %w", err)
	}
	if err := tpl.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	repoDir, err := resolveRepoDir(opts.repoDir)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}

	resolver, fileFilter, err := rules.NewResolver(repoDir, opts.rulePath)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	if opts.preview {
		return runPreview(repoDir, opts, fileFilter)
	}

	toolEntries, err := toolsconfig.Load(opts.toolConfigPath)
	if err != nil {
		return fmt.Errorf("load tools: %w", err)
	}
	planToolDefs := agent.BuildToolDefs(toolEntries, true)
	mainToolDefs := agent.BuildToolDefs(toolEntries, false)

	cfgPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	appCfg, err := LoadAppConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load app config: %w", err)
	}
	if appCfg != nil {
		tpl.ApplyLanguage(appCfg.Language)
	}

	ep, err := llm.ResolveEndpoint(cfgPath)
	if err != nil {
		return fmt.Errorf("resolve LLM endpoint: %w", err)
	}

	llmClient := llm.NewLLMClient(ep)
	model := ep.Model

	collector := tool.NewCommentCollector()
	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit)
	ref, _ := mode.RefValue(opts.to, opts.commit)
	diffMap := make(map[string]string)
	fileReader := &tool.FileReader{
		RepoDir: repoDir,
		Mode:    mode,
		Ref:     ref,
	}
	tools := buildToolRegistry(collector, fileReader, diffMap)

	ag := agent.New(agent.Args{
		RepoDir:               repoDir,
		From:                  opts.from,
		To:                    opts.to,
		Commit:                opts.commit,
		DiffMap:               diffMap,
		Template:              *tpl,
		SystemRule:            resolver,
		FileFilter:            fileFilter,
		LLMClient:             llmClient,
		Tools:                 tools,
		PlanToolDefs:          planToolDefs,
		MainToolDefs:          mainToolDefs,
		CommentCollector:      collector,
		CommentWorkerPool:     agent.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:        opts.concurrency,
		ConcurrentTaskTimeout: opts.perFileTimeout,
		Model:                 model,
		Background:            opts.background,
	})

	// Silence progress output during execution; restore before Summary in agent mode.
	var unsilence func()
	if opts.outputFormat == "json" || opts.audience == "agent" {
		unsilence = stdout.Quiet()
		defer func() {
			if unsilence != nil {
				unsilence()
			}
		}()
	}

	ctx, span := telemetry.StartSpan(context.Background(), "review.run")
	defer span.End()
	startTime := time.Now()

	comments, err := ag.Run(ctx)
	if err != nil {
		telemetry.SetAttr(span, "error", err.Error())
		return fmt.Errorf("review failed: %w", err)
	}

	// Resolve line numbers by matching existing_code against diff hunks.
	comments = diff.ResolveLineNumbers(comments, ag.Diffs())

	// Publish to GitLab if requested.
	if opts.publish {
		token := firstNonEmpty(os.Getenv("GITLAB_TOKEN"), os.Getenv("OCR_GITLAB_TOKEN"))
		pubOpts, err := resolveGitLabReviewOptions(opts, token)
		if err != nil {
			return fmt.Errorf("gitlab options: %w", err)
		}
		pub := newGitLabPublisher(pubOpts)
		result, err := pub.Publish(comments)
		if err != nil {
			return fmt.Errorf("gitlab publish: %w", err)
		}
		printPublishResult(opts, result)
	}

	// Record summary metrics (files_reviewed is refined by agent.Run).
	duration := time.Since(startTime)
	telemetry.RecordReviewDuration(ctx, duration)
	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}

	// If no files were reviewed (e.g. workspace has no changes), inform the caller in JSON mode.
	if opts.outputFormat == "json" && len(comments) == 0 && ag.FilesReviewed() == 0 {
		return outputJSONNoFiles()
	}

	// In agent mode, restore stdout so Summary reaches the terminal.
	if opts.audience == "agent" && unsilence != nil {
		unsilence()
		unsilence = nil
	}

	telemetry.PrintTraceSummary(ag.FilesReviewed(), int64(len(comments)), ag.TotalInputTokens(), ag.TotalOutputTokens(), ag.TotalTokensUsed(), duration)

	if opts.outputFormat == "json" {
		return outputJSONWithWarnings(comments, ag.Warnings())
	}
	if opts.audience == "agent" {
		outputTextWithWarnings(comments, ag.Warnings())
		return nil
	}
	outputTextWithWarnings(comments, ag.Warnings())

	return nil
}

func resolveRepoDir(input string) (string, error) {
	if input == "" {
		var err error
		input, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
	}
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	out, err := runGitCmd(absPath, "rev-parse", "--git-dir")
	if err != nil || len(out) == 0 {
		return "", fmt.Errorf("%s is not a git repository", absPath)
	}
	return absPath, nil
}

// requireGitRepo validates that the given directory is part of a git repository.
func requireGitRepo(dir string) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	out, err := runGitCmd(repoDir, "rev-parse", "--git-dir")
	if err != nil || len(out) == 0 {
		return fmt.Errorf("%s is not a git repository, code review requires a valid git repository", repoDir)
	}
	return nil
}

func runPreview(repoDir string, opts reviewOptions, fileFilter *rules.FileFilter) error {
	ag := agent.New(agent.Args{
		RepoDir:    repoDir,
		From:       opts.from,
		To:         opts.to,
		Commit:     opts.commit,
		FileFilter: fileFilter,
	})

	preview, err := ag.Preview()
	if err != nil {
		return fmt.Errorf("preview failed: %w", err)
	}

	outputPreviewText(preview)
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func buildToolRegistry(collector *tool.CommentCollector, fr *tool.FileReader, diffMap map[string]string) tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Register(tool.NewFileFind(fr))
	reg.Register(tool.NewFileReadDiff())
	reg.Register(tool.NewCodeSearch(fr))
	reg.Register(&tool.CodeCommentProvider{Collector: collector})

	// Wire up FileReadDiffProvider with shared diffMap pointer so Agent's loadDiffs populates it.
	if p, ok := reg[tool.FileReadDiff.Name()].(*tool.FileReadDiffProvider); ok {
		p.DiffMap = diffMap
	}

	return reg
}
