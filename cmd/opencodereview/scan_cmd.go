package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"

	"go.opentelemetry.io/otel/codes"
)

type scanOptions struct {
	toolConfigPath  string
	rulePath        string
	repoDir         string
	paths           string
	excludes        string
	outputFormat    string
	audience        string
	background      string
	concurrency     int
	perFileTimeout  int
	maxTools        int
	maxGitProcs     int
	preview         bool
	noPlan          bool
	noDedup         bool
	noSummary       bool
	batch           string
	maxTokensBudget int
	provider        string
	model           string
}

var scanOpts scanOptions

var scanCmd = &cobra.Command{
	Use:     "scan [flags]",
	Aliases: []string{"s"},
	Short:   "Scan entire files (no diff required)",
	Long:    "OpenCodeReview - Full-File Scan\n\nScan entire files for code review without requiring a diff.",
	Args:    cobra.NoArgs,
	Example: `  # Scan the entire repository
  ocr scan

  # Scan a single directory
  ocr scan --path internal/agent

  # Scan multiple files
  ocr scan --path internal/agent/agent.go,internal/diff/scan.go

  # Select a configured provider and model for this run only
  ocr scan --provider openai --model gpt-5.4 --format json

  # Exclude generated files / fixtures
  ocr scan --exclude '**/generated/*,**/testdata/*'

  # Preview which files would be scanned without calling the LLM
  ocr scan --preview

  # Skip the per-file PLAN_TASK pre-pass
  ocr scan --no-plan`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateScanOptions(&scanOpts); err != nil {
			return err
		}
		return executeScan(scanOpts)
	},
}

func init() {
	registerScanFlags(scanCmd, &scanOpts)
}

func splitPaths(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func executeScan(opts scanOptions) error {
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, opts.maxTools, opts.maxGitProcs, false)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// scan owns its own template (scan_template.json) independent from the
	// diff-review template loaded by loadCommonContext above. Apply --max-tools
	// as an "only raise" override to the scan template's per-file budget.
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		return fmt.Errorf("load scan template: %w", err)
	}
	if err := scanTpl.Validate(); err != nil {
		return fmt.Errorf("invalid scan template: %w", err)
	}
	if opts.maxTools > scanTpl.MaxToolRequestTimes {
		scanTpl.MaxToolRequestTimes = opts.maxTools
	}
	if opts.batch != "" {
		// CLI override of BATCH_STRATEGY; validated downstream by parseBatchStrategy
		// (unknown values silently fall back to "none").
		scanTpl.BatchStrategy = opts.batch
	}
	// Token budget: --max-tokens-budget overrides the template value when set.
	budget := scanTpl.MaxTokensBudget
	if opts.maxTokensBudget > 0 {
		budget = int64(opts.maxTokensBudget)
	}

	scanPaths := splitPaths(opts.paths)

	if opts.preview {
		return runScanPreview(cc, scanTpl, scanPaths)
	}

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, llm.ResolveOptions{
		Provider: opts.provider,
		Model:    opts.model,
	})
	if err != nil {
		return err
	}
	llmIdentity := &jsonLLMIdentity{
		Provider: rt.Provider,
		Model:    rt.Model,
	}
	// Apply language to the scan template too (loadLLMRuntime only mutates
	// the diff-review template it was handed).
	if rt.AppCfg != nil {
		scanTpl.ApplyLanguage(rt.AppCfg.Language)
	}

	// file_read_diff is meaningless in scan mode (no diff exists). Hiding it
	// from MainToolDefs stops the LLM from burning tool-call rounds probing
	// for diff content that does not exist.
	scanToolDefs := excludeToolDef(rt.MainToolDefs, "file_read_diff")

	// Scan mode always reads file contents from the working tree.
	fileReader := &tool.FileReader{
		RepoDir: cc.RepoDir,
		Mode:    tool.ModeWorkspace,
		Runner:  cc.GitRunner,
	}
	tools := buildToolRegistry(rt.Collector, fileReader)

	ag := scan.NewAgent(scan.Args{
		RepoDir:               cc.RepoDir,
		Paths:                 scanPaths,
		Template:              *scanTpl,
		SystemRule:            cc.Resolver,
		FileFilter:            cc.FileFilter,
		LLMClient:             rt.Client,
		Tools:                 tools,
		MainToolDefs:          scanToolDefs,
		CommentCollector:      rt.Collector,
		CommentWorkerPool:     llmloop.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:        opts.concurrency,
		ConcurrentTaskTimeout: opts.perFileTimeout,
		Model:                 rt.Model,
		Background:            opts.background,
		GitRunner:             cc.GitRunner,
		MaxFileSizeBytes:      scanTpl.MaxFileSizeBytes,
		MaxTokensBudget:       budget,
		SkipPlan:              opts.noPlan,
		SkipDedup:             opts.noDedup,
		SkipSummary:           opts.noSummary,
	})

	q := newQuietHandle(opts.outputFormat, opts.audience)
	defer q.Restore()

	ctx, span := telemetry.StartSpan(telemetry.ContextWithTraceParentFromEnv(context.Background()), "scan.run")
	defer span.End()
	var traceID string
	if telemetry.IsEnabled() {
		traceID = telemetry.TraceIDFromContext(ctx)
		if opts.outputFormat != "json" {
			fmt.Fprintf(os.Stderr, "[ocr] TraceID: %s\n", traceID)
		}
	}
	startTime := time.Now()

	comments, err := ag.Run(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] Session: %s\n", id)
		}
		return fmt.Errorf("scan failed: %w", err)
	}

	return emitRunResult(ctx, ag, comments, startTime, opts.outputFormat, opts.audience, q, llmIdentity)
}

func runScanPreview(cc *commonContext, scanTpl *template.ScanTemplate, scanPaths []string) error {
	ag := scan.NewAgent(scan.Args{
		RepoDir:          cc.RepoDir,
		Paths:            scanPaths,
		FileFilter:       cc.FileFilter,
		GitRunner:        cc.GitRunner,
		MaxFileSizeBytes: scanTpl.MaxFileSizeBytes,
		// Template's prompt fields are unused by Preview; pass the same
		// value so MaxFileSizeBytes is consistent.
		Template: *scanTpl,
	})

	preview, err := ag.Preview(context.Background())
	if err != nil {
		return fmt.Errorf("scan preview failed: %w", err)
	}
	outputPreviewText(preview)
	return nil
}
