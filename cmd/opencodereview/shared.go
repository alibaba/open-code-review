// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/config/toolsconfig"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
)

// commonContext bundles the state that both `ocr review` and `ocr scan`
// need to load *before* deciding whether to dispatch a preview or a real
// LLM session: a validated template, the resolved repo path, review rules,
// and a shared git subprocess limiter.
type commonContext struct {
	Template   *template.Template
	RepoDir    string
	Resolver   rules.Resolver
	FileFilter *rules.FileFilter
	GitRunner  *gitcmd.Runner
	// IsGitRepo reports whether RepoDir is inside a git repository. Always
	// true when requireGit was set; may be false when scan accepts non-git
	// directories.
	IsGitRepo bool
}

// resolveMaxTokens applies the per-run CLI override, then the saved setting,
// and finally the embedded task-template default.
func resolveMaxTokens(templateDefault int, cfg *Config, cliOverride int) (int, error) {
	if cliOverride < 0 {
		return 0, fmt.Errorf("--max-tokens must be a non-negative integer")
	}
	if cliOverride > 0 {
		return cliOverride, nil
	}
	if cfg == nil || cfg.MaxTokens == 0 {
		return templateDefault, nil
	}
	if cfg.MaxTokens < 0 {
		return 0, fmt.Errorf("invalid max_tokens in app config: must be a positive integer")
	}
	return cfg.MaxTokens, nil
}

// loadCommonContext validates the working directory, loads the embedded
// template, raises MaxToolRequestTimes when maxTools exceeds the default,
// resolves the absolute repo path, loads system review rules, and creates
// the global git subprocess limiter. Both review and scan callers go
// through this so the startup sequence stays consistent.
//
// requireGit=true fails fast when the directory is not a git repo (review
// path: diff concept requires git). requireGit=false allows non-git
// directories (scan path: provider falls back to filepath.Walk).
func loadCommonContext(repoDirInput, rulePath string, maxTools, maxGitProcs int, requireGit bool) (*commonContext, error) {
	tpl, err := template.LoadDefault()
	if err != nil {
		return nil, fmt.Errorf("load default template: %w", err)
	}
	if maxTools > tpl.MaxToolRequestTimes {
		tpl.MaxToolRequestTimes = maxTools
	}
	if err := tpl.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	repoDir, isGit, err := resolveWorkingDir(repoDirInput, requireGit)
	if err != nil {
		return nil, err
	}

	resolver, fileFilter, err := rules.NewResolver(repoDir, rulePath)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}

	return &commonContext{
		Template:   tpl,
		RepoDir:    repoDir,
		Resolver:   resolver,
		FileFilter: fileFilter,
		GitRunner:  gitcmd.New(maxGitProcs),
		IsGitRepo:  isGit,
	}, nil
}

// resolveWorkingDir returns (absPath, isGitRepo, err). When requireGit is
// true, returns an error if the directory is not a git repo. When false,
// returns IsGitRepo=false instead of erroring (scan path uses this).
func resolveWorkingDir(input string, requireGit bool) (string, bool, error) {
	if input == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", false, fmt.Errorf("get working directory: %w", err)
		}
		input = wd
	}
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "", false, fmt.Errorf("resolve absolute path: %w", err)
	}
	if _, statErr := os.Stat(absPath); statErr != nil {
		return "", false, fmt.Errorf("stat %s: %w", absPath, statErr)
	}
	out, err := runGitCmd(absPath, "rev-parse", "--git-dir")
	isGit := err == nil && len(out) > 0
	if !isGit && requireGit {
		return "", false, fmt.Errorf("%s is not a git repository", absPath)
	}
	// #287: git reports diff and `git show HEAD:<path>` paths relative to the
	// repository root, not the current directory. When `ocr review` runs from a
	// subdirectory of a monorepo, anchor RepoDir at the git top-level so those
	// root-relative paths resolve for both disk reads and git-show reads.
	// requireGit is true only for the review path; scan (requireGit=false) keeps
	// the CWD so its `git ls-files` walk stays scoped to the subdirectory.
	if isGit && requireGit {
		// runGitCmdStdout captures stdout only so git stderr notices can't
		// pollute the resolved path. --show-toplevel fails (or is empty) when
		// there is no work tree — e.g. a bare repo, where --git-dir succeeds so
		// isGit is true. Fail loudly there instead of silently reusing the
		// subdir, which would reproduce the #287 root-relative-path bug.
		top, topErr := runGitCmdStdout(absPath, "rev-parse", "--show-toplevel")
		t := strings.TrimSpace(string(top))
		if topErr != nil || t == "" {
			return "", false, fmt.Errorf("%s is a git repository without a work tree (bare repo?); cannot resolve its top level for review", absPath)
		}
		absPath = t
	}
	return absPath, isGit, nil
}

// llmRuntime bundles the LLM-side state both subcommands need once they've
// decided to actually run a session: tool definitions, an app-language
// adjusted template (mutated in place via ApplyLanguage), the LLM client,
// the resolved model name, and a fresh comment collector.
type llmRuntime struct {
	Client       llm.LLMClient
	Model        string
	Provider     string // resolved provider name (non-secret label; empty for non-provider endpoints)
	PlanToolDefs []llm.ToolDef
	MainToolDefs []llm.ToolDef
	Collector    *tool.CommentCollector
	// RetryCollector observes every LLM HTTP attempt this run makes. It is
	// created here rather than on the session or the agent because the client is
	// built before either exists, and it is per-run rather than package-level so
	// two runs in one process cannot share data. scan gets one too; its requests
	// carry no RequestMeta, so every attempt is dropped and the frozen report is
	// nil.
	RetryCollector *llm.RetryCollector
	AppCfg         *Config
	// RuntimeConfig holds the allowlisted, non-secret runtime settings (protocol,
	// sanitized endpoint host, language, timeout) derived from the resolved
	// endpoint and app config, for the run manifest's runtime_config_sha256. It
	// never carries the token or full URL.
	RuntimeConfig agent.RuntimeConfig
}

// newRetryCollector builds the per-run retry collector. It is a variable so a
// test can hand back a collector whose invariants are already violated, which is
// the only way to exercise the Freeze construction-error branch from the
// outside: every production path finalizes every logical request on every exit,
// so a well-behaved run can never produce one.
var newRetryCollector = llm.NewRetryCollector

// loadLLMRuntime loads tool defs from toolConfigPath, reads the app config
// from the user's default config path (applying the configured language to
// tpl — defaulting when the config file is absent), resolves the LLM
// endpoint (honoring resolveOpts), and
// returns the runtime bundle. tpl is mutated in place.
func loadLLMRuntime(tpl *template.Template, toolConfigPath string, resolveOpts llm.ResolveOptions) (*llmRuntime, error) {
	toolEntries, err := toolsconfig.Load(toolConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load tools: %w", err)
	}
	planToolDefs := agent.BuildToolDefs(toolEntries, true)
	mainToolDefs := agent.BuildToolDefs(toolEntries, false)

	cfgPath, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}
	appCfg, err := LoadAppConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load app config: %w", err)
	}
	// Apply the language directive even when the config file is missing
	// (upstream #fix: ApplyLanguage with empty lang falls back to default).
	var lang string
	if appCfg != nil {
		lang = appCfg.Language
	}
	tpl.ApplyLanguage(lang)

	ep, err := llm.ResolveEndpointWithOptions(cfgPath, resolveOpts)
	if err != nil {
		return nil, fmt.Errorf("resolve LLM endpoint: %w", err)
	}

	retryCollector := newRetryCollector()

	return &llmRuntime{
		Client:         llm.NewLLMClient(ep, retryCollector),
		Model:          ep.Model,
		Provider:       ep.Provider,
		PlanToolDefs:   planToolDefs,
		MainToolDefs:   mainToolDefs,
		Collector:      tool.NewCommentCollector(),
		RetryCollector: retryCollector,
		AppCfg:         appCfg,
		RuntimeConfig: agent.RuntimeConfig{
			Protocol:     ep.Protocol,
			EndpointHost: sanitizeEndpointHost(ep.URL),
			Language:     lang,
			Timeout:      ep.Timeout,
		},
	}, nil
}

// sanitizeEndpointHost extracts the credential-free host[:port] from a full LLM
// endpoint URL, dropping scheme, any embedded userinfo, path, query and fragment
// so no secret material survives into the manifest's runtime_config hash. The
// host is lowercased for a stable identity (DNS is case-insensitive). An empty
// or unparseable URL, or one without a host, yields "".
func sanitizeEndpointHost(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host) // u.Host is host[:port]; userinfo lives in u.User
}

// applyCLIExcludes appends user-supplied --exclude patterns (already split
// into a []string) onto cc.FileFilter.Exclude. Creates the FileFilter if
// none was returned by rule.json layers. Idempotent on empty input.
func applyCLIExcludes(cc *commonContext, patterns []string) {
	if len(patterns) == 0 {
		return
	}
	if cc.FileFilter == nil {
		cc.FileFilter = &rules.FileFilter{}
	}
	cc.FileFilter.Exclude = append(cc.FileFilter.Exclude, patterns...)
}

// excludeToolDef returns a copy of defs with any entries whose function name
// matches name removed. Used by `ocr scan` to hide tools that don't make
// sense in full-scan mode (e.g. file_read_diff).
func excludeToolDef(defs []llm.ToolDef, name string) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(defs))
	for _, d := range defs {
		if d.Function.Name == name {
			continue
		}
		out = append(out, d)
	}
	return out
}

// quietHandle wraps the restorer returned by whichever stdout redirection
// newQuietHandle chose, so callers can `defer q.Restore()` for safety while
// emitRunResult restores it early when the agent-text audience needs the trace
// summary on the user's terminal. Restore is idempotent.
type quietHandle struct {
	fn func()
}

// isMachineReadable reports whether the output format writes a structured
// document to stdout that must not be interleaved with progress text. Both
// json and sarif move [ocr] progress lines off stdout and suppress the trace
// summary, which is already carried inside the document.
func isMachineReadable(outputFormat string) bool {
	return outputFormat == "json" || outputFormat == "sarif"
}

// newQuietHandle routes [ocr] progress lines away from stdout so they cannot
// corrupt a structured output document. What it does depends on why stdout
// needs protecting:
//
//   - audience=="agent": the caller wants no progress at all, so progress is
//     discarded regardless of format.
//   - machine-readable format with a human audience: the human still asked to
//     watch the run, so progress is redirected to stderr rather than dropped.
//     Every result document (json, sarif, text) is written straight to
//     os.Stdout and never through stdout.Writer(), so stdout stays a single
//     parseable document while stderr carries the live progress.
//   - otherwise: no-op, progress stays on stdout.
func newQuietHandle(outputFormat, audience string) *quietHandle {
	h := &quietHandle{}
	switch {
	case audience == "agent":
		h.fn = stdout.Quiet()
	case isMachineReadable(outputFormat):
		h.fn = stdout.Swap(os.Stderr)
	}
	return h
}

// Restore re-enables stdout. Safe to call multiple times.
func (h *quietHandle) Restore() {
	if h == nil || h.fn == nil {
		return
	}
	h.fn()
	h.fn = nil
}

// ResultProvider abstracts the metadata both internal/agent.Agent and
// internal/scan.Agent expose post-run, so emitRunResult can finalize
// either without knowing which kind it has.
type ResultProvider interface {
	Diffs() []model.Diff
	FilesReviewed() int64
	TotalInputTokens() int64
	TotalOutputTokens() int64
	TotalTokensUsed() int64
	TotalCacheReadTokens() int64
	TotalCacheWriteTokens() int64
	Warnings() []agent.AgentWarning
	// ProjectSummary is the markdown project-level summary produced by
	// scan's PROJECT_SUMMARY_TASK or review's --summary flag. Empty when
	// the summary phase was skipped, disabled, or failed.
	ProjectSummary() string
	ToolCalls() map[string]int64
	// SessionID returns the persisted session identifier so callers can show it
	// in JSON output or failure diagnostics. Returns "" when no session was
	// created.
	SessionID() string
	// BudgetExceeded reports whether the aggregate token budget gate stopped the
	// run before all files were reviewed. It is a diagnostic signal only — it
	// feeds summary.budget_exceeded and the failure usage record, and never
	// decides the run's terminal state. The terminal state comes solely from the
	// manifest's coverage: the stop marks the undispatched items
	// failed(budget) without recording a run_failure, so it reads as partial
	// whenever anything was covered.
	BudgetExceeded() bool
	// RunManifest returns the frozen v1 coverage result for review runs. Scan
	// remains legacy and returns nil.
	RunManifest() *session.RunManifest
}

type resumeInfoProvider interface {
	ResumeInfo() *agent.ResumeInfo
}

// emitRunResult is the post-LLM-run finalization shared by `ocr review` and
// `ocr scan`: resolves comment line numbers, records telemetry, restores
// stdout early for agent-text audiences so the summary is visible, prints
// the trace summary, and writes the result in the requested format.
//
// q is the silencing handle returned by newQuietHandle; pass nil if no
// silencing was set up (in which case the early restore is a no-op).
//
// retryReport is the frozen LLM retry report, or nil when there is nothing to
// report (a clean run, or a caller that produces no report at all — `ocr scan`
// never freezes one). It is passed as a parameter rather than added to
// ResultProvider because the collector belongs to llmRuntime, not to the
// agent; putting it on the interface would force internal/scan.Agent to
// implement a method that is always nil.
func emitRunResult(
	ctx context.Context,
	ag ResultProvider,
	comments []model.LlmComment,
	startTime time.Time,
	outputFormat, audience string,
	q *quietHandle,
	llmIdentity *jsonLLMIdentity,
	retryReport *llm.RetryReport,
) error {
	comments = diff.ResolveLineNumbers(comments, ag.Diffs())

	duration := time.Since(startTime)
	telemetry.RecordReviewDuration(ctx, duration)
	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}

	traceID := telemetry.TraceIDFromContext(ctx)
	manifest := ag.RunManifest()

	// JSON and SARIF are machine-readable formats written to stdout; they
	// share the same suppression of trace summaries and early stdout restore.
	machineReadable := isMachineReadable(outputFormat)

	if machineReadable && manifest == nil && len(comments) == 0 && ag.FilesReviewed() == 0 {
		if outputFormat == "json" {
			return outputJSONNoFiles(traceID, llmIdentity)
		}
		return outputSARIF(nil, Version, ag.Warnings(), manifest)
	}

	// Agent-text audiences need stdout back before PrintTraceSummary so the
	// summary line lands on their terminal.
	if audience == "agent" && !machineReadable {
		q.Restore()
	}

	if !machineReadable {
		telemetry.PrintTraceSummary(telemetry.TraceSummary{
			FilesReviewed:     ag.FilesReviewed(),
			CommentsGenerated: int64(len(comments)),
			InputTokens:       ag.TotalInputTokens(),
			OutputTokens:      ag.TotalOutputTokens(),
			TotalTokens:       ag.TotalTokensUsed(),
			CacheReadTokens:   ag.TotalCacheReadTokens(),
			CacheWriteTokens:  ag.TotalCacheWriteTokens(),
			Duration:          duration,
			SessionID:         ag.SessionID(),
		})
	}

	if outputFormat == "json" {
		var resumeInfo *agent.ResumeInfo
		if p, ok := ag.(resumeInfoProvider); ok {
			resumeInfo = p.ResumeInfo()
		}
		return outputJSONWithWarnings(comments, ag.Warnings(), ag.FilesReviewed(),
			ag.TotalInputTokens(), ag.TotalOutputTokens(), ag.TotalTokensUsed(),
			ag.TotalCacheReadTokens(), ag.TotalCacheWriteTokens(), duration,
			ag.ProjectSummary(), ag.ToolCalls(), traceID, resumeInfo, ag.SessionID(), manifest, ag.BudgetExceeded(), llmIdentity, retryReport)
	}
	if outputFormat == "sarif" {
		return outputSARIF(comments, Version, ag.Warnings(), manifest)
	}
	outputTextWithWarnings(comments, ag.Warnings(), manifest)
	// Between the comments/warnings block and the project summary: the report is
	// run-level diagnostics about how the comments were obtained, so it reads
	// after them but must not separate the summary from the end of output.
	outputRetryReportText(os.Stdout, retryReport)
	if summary := ag.ProjectSummary(); summary != "" {
		fmt.Printf("\n\n──────── Project Summary ────────\n\n%s\n", summary)
	}
	return nil
}
