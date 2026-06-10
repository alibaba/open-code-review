package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/llmloop"
	"github.com/open-code-review/open-code-review/internal/scan"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// scanOptions mirrors reviewOptions for the full-scan subcommand. The two
// types are kept separate so the scan flag set can evolve independently of
// the diff-based review flags (e.g. --from/--to/--commit make no sense here).
type scanOptions struct {
	toolConfigPath string
	rulePath       string
	repoDir        string
	all            bool
	paths          string // comma-separated relative paths
	outputFormat   string
	audience       string
	background     string
	concurrency    int
	perFileTimeout int
	maxTools       int
	maxGitProcs    int
	preview        bool
	showHelp       bool
}

func parseScanFlags(args []string) (scanOptions, error) {
	a := newOcrFlagSet("ocr scan")
	opts := scanOptions{}

	a.StringVar(&opts.toolConfigPath, "tools", "", "path to JSON tools config file (default: embedded)")
	a.StringVar(&opts.rulePath, "rule", "", "path to JSON file with system review rules")
	a.StringVar(&opts.repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.BoolVar(&opts.all, "all", false, "scan every reviewable file in the repository")
	a.StringVar(&opts.paths, "path", "", "comma-separated repo-relative directories or files to scan")
	a.StringVarP(&opts.outputFormat, "format", "f", "text", "output format: text or json")
	a.IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file scans")
	a.IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	a.StringVar(&opts.audience, "audience", "human", "output audience: human (show progress) or agent (summary only)")
	a.StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the scan")
	a.IntVar(&opts.maxTools, "max-tools", 0, "max tool call rounds per file; only takes effect when greater than template default")
	a.IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	a.BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be scanned without running the LLM")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.showHelp {
		return opts, nil
	}

	if !opts.all && strings.TrimSpace(opts.paths) == "" {
		return opts, fmt.Errorf("must specify --all or --path; run 'ocr scan -h' for usage")
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

	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, opts.maxTools, opts.maxGitProcs)
	if err != nil {
		return err
	}
	if cc.Template.FullScanTask == nil || len(cc.Template.FullScanTask.Messages) == 0 {
		return fmt.Errorf("FULL_SCAN_TASK is missing from the loaded template")
	}

	// Scan reviews whole files (often hundreds of lines with multiple
	// findings), so it needs a larger per-file tool-call budget than diff
	// review. Promote MaxToolRequestTimes to the scan-specific value. The
	// --max-tools flag (handled in loadCommonContext) can still raise this
	// further; we only raise, never lower, so an explicit --max-tools
	// override wins.
	if cc.Template.FullScanMaxToolRequestTimes > cc.Template.MaxToolRequestTimes {
		cc.Template.MaxToolRequestTimes = cc.Template.FullScanMaxToolRequestTimes
	}

	scanPaths := splitPaths(opts.paths)

	if opts.preview {
		return runScanPreview(cc, scanPaths)
	}

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath)
	if err != nil {
		return err
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
		Template:              *cc.Template,
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

func runScanPreview(cc *commonContext, scanPaths []string) error {
	ag := scan.NewAgent(scan.Args{
		RepoDir:    cc.RepoDir,
		Paths:      scanPaths,
		FileFilter: cc.FileFilter,
		GitRunner:  cc.GitRunner,
		// Template is unused by Preview but NewAgent inspects nothing in it.
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
  # Scan the entire repository
  ocr scan --all

  # Scan a single directory
  ocr scan --path internal/agent

  # Scan multiple files
  ocr scan --path internal/agent/agent.go,internal/diff/scan.go

  # Combine --all with --path to restrict the all-scan
  ocr scan --all --path internal/

  # Preview which files would be scanned without calling the LLM
  ocr scan --all --preview

Flags:
  --all                   scan every reviewable file in the repository
  --path string           comma-separated repo-relative dirs/files to scan
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
