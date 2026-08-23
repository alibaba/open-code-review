// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "json", "sarif":
		return true
	default:
		return false
	}
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

const (
	maxOSCSequenceLength = 4096
	maxCSISequenceLength = 256
)

// stripAnsiState is the ANSI escape parsing state for stripAnsiWriter.
type stripAnsiState int

const (
	ansiNormal stripAnsiState = iota
	ansiEsc
	ansiCSI
	ansiOSC
	ansiOSCEsc
)

// stripAnsiWriter removes ANSI escape sequences (colors, cursor moves, OSC
// control strings) from the byte stream it forwards, so terminal-only
// decoration never reaches --output files. It tolerates sequences split
// arbitrarily across Write calls: any in-progress sequence is buffered and
// carried to the next Write. Payload bytes (non-ESC) pass through unchanged,
// so stripping only discards decoration, never result content.
type stripAnsiWriter struct {
	dst     io.Writer
	state   stripAnsiState
	pending []byte // partial escape sequence carried across Write calls
}

func (w *stripAnsiWriter) Write(p []byte) (int, error) {
	// Only new bytes are fed to the state machine. w.pending holds bytes that
	// already entered an escape sequence in earlier calls — re-feeding them
	// would re-parse the sequence start (e.g. '[' would be mistaken for a CSI
	// final byte once the state is already ansiCSI) and leak the sequence into
	// the output. The pending buffer is discarded wholesale on completion.
	var out []byte
	for _, c := range p {
		switch w.state {
		case ansiNormal:
			if c == 0x1b {
				w.state = ansiEsc
				w.pending = append(w.pending, c)
			} else {
				out = append(out, c)
			}
		case ansiEsc:
			w.pending = append(w.pending, c)
			switch {
			case c == '[':
				w.state = ansiCSI
			case c == ']', c == 'P', c == '^', c == '_':
				// OSC, DCS, PM and APC strings all run until ST (or BEL for
				// OSC); treat them uniformly through the OSC state.
				w.state = ansiOSC
			case c >= 0x20 && c <= 0x2f:
				// Intermediate byte of a multi-byte escape (e.g. ESC ( B);
				// keep collecting so the whole sequence is discarded.
			default:
				// Single-byte escape sequence. Discard.
				w.state = ansiNormal
				w.pending = w.pending[:0]
			}
		case ansiCSI:
			w.pending = append(w.pending, c)
			if c >= 0x40 && c <= 0x7e {
				w.state = ansiNormal
				w.pending = w.pending[:0]
			} else if c < 0x20 || c >= 0x80 || len(w.pending) >= maxCSISequenceLength {
				out = append(out, w.pending...)
				w.pending = w.pending[:0]
				w.state = ansiNormal
			}
		case ansiOSC:
			w.pending = append(w.pending, c)
			switch c {
			case 0x07: // BEL terminates an OSC string
				w.state = ansiNormal
				w.pending = w.pending[:0]
			case 0x1b: // possible ST terminator (ESC \)
				w.state = ansiOSCEsc
			default:
				if len(w.pending) >= maxOSCSequenceLength {
					out = append(out, w.pending...)
					w.pending = w.pending[:0]
					w.state = ansiNormal
				}
			}
		case ansiOSCEsc:
			if c == '\\' { // ST terminates the OSC string
				w.state = ansiNormal
				w.pending = w.pending[:0]
			} else {
				// Not a valid ST. The OSC ends here without one (e.g. a bare
				// ESC terminator, which some terminals accept). The trailing
				// byte is not part of the sequence and must be re-parsed:
				// an ESC starts a new escape sequence, anything else is text.
				w.state = ansiNormal
				w.pending = w.pending[:0]
				if c == 0x1b {
					w.state = ansiEsc
					w.pending = append(w.pending, c)
				} else {
					out = append(out, c)
				}
			}
		}
	}

	if len(out) > 0 {
		n, err := w.dst.Write(out)
		if err != nil {
			// The state machine has already consumed p; report the error but
			// still return len(p) so a caller that retries on n < len(p) does
			// not feed the same bytes through the state machine a second time.
			return len(p), err
		}
		if n != len(out) {
			return len(p), io.ErrShortWrite
		}
	}
	return len(p), nil
}

// lazyFileWriter defers os.Create until the first Write so a run that never
// produces output (LLM failure, preview error, interruption) leaves an
// existing target file untouched instead of truncating it to zero bytes. The
// "Results written" hint is printed to stderr only after the first successful
// Write, so agents never see a path hint for a file that stayed empty or was
// never persisted.
type lazyFileWriter struct {
	path     string
	strip    bool // strip ANSI when the target format is text
	once     sync.Once
	file     *os.File
	stripper *stripAnsiWriter
	err      error // os.Create error
	writeErr error // first error from a Write
	hinted   bool  // hint already printed after a successful Write
}

func (w *lazyFileWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		f, err := os.Create(w.path)
		if err != nil {
			w.err = fmt.Errorf("create output file %s: %w", w.path, err)
			return
		}
		w.file = f
		if w.strip {
			w.stripper = &stripAnsiWriter{dst: f}
		}
	})
	if w.err != nil {
		return 0, w.err
	}
	var n int
	var err error
	if w.stripper != nil {
		n, err = w.stripper.Write(p)
	} else {
		n, err = w.file.Write(p)
	}
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	if err == nil && !w.hinted {
		w.hinted = true
		fmt.Fprintf(os.Stderr, "[ocr] Results written to %s\n", w.path)
	}
	return n, err
}

// Err returns the first error encountered while creating or writing the
// underlying file, or nil if none occurred. Text-mode rendering drops the
// per-write errors of fmt.Fprintf, so callers use this to surface write
// failures (e.g. permission denied on the first write) as a command error —
// matching JSON mode, where Encoder.Encode propagates the same failure.
func (w *lazyFileWriter) Err() error {
	if w.err != nil {
		return w.err
	}
	return w.writeErr
}

// writeOutError surfaces deferred write errors from writers that record them
// (lazyFileWriter); plain writers such as os.Stdout report nil.
func writeOutError(out io.Writer) error {
	if r, ok := out.(interface{ Err() error }); ok {
		return r.Err()
	}
	return nil
}

// Close closes the underlying file. It is a no-op when the file was never
// created (no output produced), so failure paths cannot leave a fresh empty
// file behind.
func (w *lazyFileWriter) Close() error {
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

// resolveOutputWriter resolves the --output target into a writer plus a
// cleanup function.
//   - "" or "-"      → os.Stdout with a no-op cleanup (colors preserved, no hint)
//   - otherwise      → a lazyFileWriter over os.Create(path), deferred until the
//     first Write; text format wraps the file in stripAnsiWriter so ANSI
//     colors never reach the result file.
//
// Fail-fast checks (directory target, missing parent) run here without
// creating or truncating anything; deeper errors (permissions, disk) surface
// on the first Write and fail the command non-zero.
func resolveOutputWriter(path, format string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return nil, nil, fmt.Errorf("--output %q is a directory", path)
	}
	parent := filepath.Dir(path)
	if st, err := os.Stat(parent); err != nil || !st.IsDir() {
		return nil, nil, fmt.Errorf("--output directory does not exist: %s", parent)
	}
	w := &lazyFileWriter{path: path, strip: !isMachineReadable(format)}
	return w, w.Close, nil
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
	// scan's PROJECT_SUMMARY_TASK. Empty for review mode and for scans
	// that skipped / failed the summary phase.
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
	out io.Writer,
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
			return outputJSONNoFiles(traceID, llmIdentity, out)
		}
		return outputSARIF(nil, Version, ag.Warnings(), manifest, out)
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
			ag.ProjectSummary(), ag.ToolCalls(), traceID, resumeInfo, ag.SessionID(), manifest, ag.BudgetExceeded(), llmIdentity, out, retryReport)
	}
	if outputFormat == "sarif" {
		return outputSARIF(comments, Version, ag.Warnings(), manifest, out)
	}
	outputTextWithWarnings(comments, ag.Warnings(), manifest, out)
	// Between the comments/warnings block and the project summary: the report is
	// run-level diagnostics about how the comments were obtained, so it reads
	// after them but must not separate the summary from the end of output.
	outputRetryReportText(out, retryReport)
	if summary := ag.ProjectSummary(); summary != "" {
		fmt.Fprintf(out, "\n\n──────── Project Summary ────────\n\n%s\n", sanitizeTerminal(summary))
	}
	// Text rendering ignores fmt.Fprintf write errors; surface them here so a
	// failed --output write (permission, disk full) fails the command non-zero
	// exactly like JSON mode does via Encoder.Encode.
	return writeOutError(out)
}
