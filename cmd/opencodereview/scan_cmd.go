package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llmloop"
	"github.com/open-code-review/open-code-review/internal/scan"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// scanOptions mirrors reviewOptions for the full-scan subcommand. The two
// types are kept separate so the scan flag set can evolve independently of
// the diff-based review flags (e.g. --from/--to/--commit make no sense here).
//
// Bare `ocr scan` (no --path) scans the entire repository; --path narrows.
type scanOptions struct {
	toolConfigPath  string
	rulePath        string
	repoDir         string
	paths           string // comma-separated relative paths; empty = whole repo
	excludes        string // comma-separated gitignore-style exclude patterns
	outputFormat    string
	audience        string
	background      string
	concurrency     int
	perFileTimeout  int
	maxTools        int
	maxGitProcs     int
	preview         bool
	noPlan          bool   // --no-plan: skip the PLAN_TASK pre-pass per file
	noDedup         bool   // --no-dedup: skip the per-batch DEDUP_TASK
	noSummary       bool   // --no-summary: skip the post-run PROJECT_SUMMARY_TASK
	batch           string // --batch: override scan template's BATCH_STRATEGY
	maxTokensBudget int    // --max-tokens-budget: cap total token usage; 0 = unlimited
	model           string // --model: override resolved LLM model for this scan
	showHelp        bool
}

func parseScanFlags(args []string) (scanOptions, error) {
	a := newOcrFlagSet("ocr scan")
	opts := scanOptions{}

	a.StringVar(&opts.toolConfigPath, "tools", "", "path to JSON tools config file (default: embedded)")
	a.StringVar(&opts.rulePath, "rule", "", "path to JSON file with system review rules")
	a.StringVar(&opts.repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.StringVar(&opts.paths, "path", "", "comma-separated repo-relative directories or files to scan (default: whole repo)")
	a.StringVar(&opts.excludes, "exclude", "", "comma-separated gitignore-style patterns to exclude; merged with rule.json excludes")
	a.StringVarP(&opts.outputFormat, "format", "f", "text", "output format: text or json")
	a.IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file scans")
	a.IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	a.StringVar(&opts.audience, "audience", "human", "output audience: human (show progress) or agent (summary only)")
	a.StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the scan")
	a.IntVar(&opts.maxTools, "max-tools", 0, "max tool call rounds per file; only takes effect when greater than template default")
	a.IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	a.BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be scanned without running the LLM")
	a.BoolVar(&opts.noPlan, "no-plan", false, "skip the per-file PLAN_TASK pre-pass (one fewer LLM call per file; may reduce review focus)")
	a.BoolVar(&opts.noDedup, "no-dedup", false, "skip the per-batch DEDUP_TASK (keeps raw comments; one fewer LLM call per batch)")
	a.BoolVar(&opts.noSummary, "no-summary", false, "skip the post-run PROJECT_SUMMARY_TASK (no project-level markdown summary)")
	a.StringVar(&opts.batch, "batch", "", "override BATCH_STRATEGY from scan template: none | by-language | by-directory")
	a.IntVar(&opts.maxTokensBudget, "max-tokens-budget", 0, "cap total token usage (input+output); dispatch stops once exceeded (0 = unlimited)")
	a.StringVar(&opts.model, "model", "", "override LLM model for this scan (e.g., claude-opus-4-6)")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.showHelp {
		return opts, nil
	}

	switch opts.audience {
	case "human", "agent":
	default:
		return opts, fmt.Errorf("invalid --audience value %q: must be 'human' or 'agent'", opts.audience)
	}

	if opts.maxTools < 0 {
		return opts, fmt.Errorf("--max-tools must be a non-negative integer (0 means use template default)")
	}
	if opts.maxGitProcs < 0 {
		return opts, fmt.Errorf("--max-git-procs must be a non-negative integer (0 means use default 16)")
	}
	if opts.maxTokensBudget < 0 {
		return opts, fmt.Errorf("--max-tokens-budget must be a non-negative integer (0 means unlimited)")
	}
	return opts, nil
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

func runScan(args []string) error {
	opts, err := parseScanFlags(args)
	if err != nil {
		// parseScanFlags already wraps with "parse flags: %w" — return as-is.
		return err
	}
	if opts.showHelp {
		printScanUsage()
		return nil
	}

	// scan path: git is preferred (more accurate .gitignore handling) but not required;
	// provider falls back to filepath.Walk when the dir is not a git repo.
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

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, opts.model)
	if err != nil {
		return err
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

	ctx, span := telemetry.StartSpan(context.Background(), "scan.run")
	defer span.End()
	startTime := time.Now()

	comments, err := ag.Run(ctx)
	if err != nil {
		telemetry.SetAttr(span, "error", err.Error())
		return fmt.Errorf("scan failed: %w", err)
	}

	return emitRunResult(ctx, ag, comments, startTime, opts.outputFormat, opts.audience, q)
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

func printScanUsage() {
	fmt.Println(`OpenCodeReview - Full-File Scan

Usage:
  ocr scan [flags]
  ocr s    [flags]                (alias)

Examples:
  # Scan the entire repository (default when no --path is given)
  ocr scan

  # Scan a single directory
  ocr scan --path internal/agent

  # Scan multiple files
  ocr scan --path internal/agent/agent.go,internal/diff/scan.go

  # Exclude generated files / fixtures
  ocr scan --exclude '**/generated/*,**/testdata/*'

  # Preview which files would be scanned without calling the LLM
  ocr scan --preview

  # Skip the per-file PLAN_TASK pre-pass (saves ~1 LLM call per file, may
  # reduce review focus)
  ocr scan --no-plan

Flags:
  --path string           comma-separated repo-relative dirs/files to scan (default: whole repo)
  --exclude string        comma-separated gitignore-style patterns to exclude (merged with rule.json)
  --no-plan               skip the per-file PLAN_TASK pre-pass (faster, less focused)
  --no-dedup              skip the per-batch DEDUP_TASK (keeps raw comments)
  --no-summary            skip the post-run PROJECT_SUMMARY_TASK
  --batch string          override BATCH_STRATEGY: none | by-language | by-directory
  --max-tokens-budget int cap total token usage; dispatch stops once exceeded (0 = unlimited)
  --model string          override LLM model for this scan (e.g., claude-opus-4-6)
  --audience string       output audience: human (show progress) or agent (summary only) (default "human")
  -b, --background string optional requirement/business context for the scan
  -f, --format string     output format: text or json (default "text")
  --concurrency int       max concurrent file scans (default 8)
  --max-git-procs int     max concurrent git subprocesses (default 16)
  --max-tools int         max tool call rounds per file; only takes effect when greater than template default
  -p, --preview           preview which files will be scanned without running the LLM
  --repo string           root directory of the git repository (default: current dir)
  --rule string           path to JSON file with system review rules
  --timeout int           concurrent task timeout in minutes (default 10)
  --tools string          path to JSON tools config file (default: embedded)`)
}
