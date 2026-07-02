package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	allowedext "github.com/open-code-review/open-code-review/internal/config/allowlist"
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/llmloop"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/stdout"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// changeFilesScanLiteral substitutes for the {{change_files}} placeholder.
// Full-scan has no "other changed files" concept; using a fixed sentinel is
// less misleading than leaving the placeholder empty.
const changeFilesScanLiteral = "(not applicable in full-scan mode)"

// Args bundles all dependencies needed for one scan session.
//
// Note: Template is the scan-specific template.ScanTemplate (loaded from
// scan_template.json), not the diff-review template.Template. The two are
// intentionally separate so review/scan prompts evolve independently.
//
// MaxFileSizeBytes overrides the default 2 MiB per-file size cap; it is
// usually populated from ScanTemplate.MaxFileSizeBytes via scan_cmd.
type Args struct {
	RepoDir               string
	Paths                 []string // empty = whole repo
	Template              template.ScanTemplate
	SystemRule            rules.Resolver
	FileFilter            *rules.FileFilter
	LLMClient             llm.LLMClient
	Tools                 *tool.Registry
	MainToolDefs          []llm.ToolDef
	CommentCollector      *tool.CommentCollector
	CommentWorkerPool     *llmloop.CommentWorkerPool
	MaxConcurrency        int
	ConcurrentTaskTimeout int
	Model                 string
	Background            string
	GitRunner             *gitcmd.Runner
	Session               *session.SessionHistory
	MaxFileSizeBytes      int64
	// SkipPlan disables the PLAN_TASK pre-pass even when the template
	// defines one. Set via the --no-plan CLI flag.
	SkipPlan bool
	// SkipDedup disables the per-batch DEDUP_TASK even when the template
	// defines one. Set via the --no-dedup CLI flag.
	SkipDedup bool
	// SkipSummary disables the post-run PROJECT_SUMMARY_TASK even when the
	// template defines one. Set via the --no-summary CLI flag.
	SkipSummary bool
	// MaxTokensBudget, when > 0, caps total token usage (input+output, as
	// reported by the API). Once the running total exceeds it, no further
	// batches are dispatched. 0 = unlimited. Set via --max-tokens-budget
	// or ScanTemplate.MaxTokensBudget.
	MaxTokensBudget int64
}

// planEnabled / dedupEnabled / summaryEnabled report whether each optional
// phase will actually run: template must define it AND the corresponding
// --no-* flag must not be set. Used by both cost estimation and dispatch.
func (a *Agent) planEnabled() bool {
	return !a.args.SkipPlan && a.args.Template.PlanTask != nil && len(a.args.Template.PlanTask.Messages) > 0
}

func (a *Agent) dedupEnabled() bool {
	return !a.args.SkipDedup && a.args.Template.DedupTask != nil && len(a.args.Template.DedupTask.Messages) > 0
}

func (a *Agent) summaryEnabled() bool {
	return !a.args.SkipSummary && a.args.Template.ProjectSummaryTask != nil && len(a.args.Template.ProjectSummaryTask.Messages) > 0
}

// Agent orchestrates full-file code review. It delegates the per-file LLM
// tool-use loop to llmloop.Runner and owns only scan-specific concerns
// (file enumeration, FULL_SCAN_TASK rendering, per-file filtering).
type Agent struct {
	args           Args
	items          []model.ScanItem
	currentDate    string
	session        *session.SessionHistory
	subtaskFailed  int64 // atomic
	runner         *llmloop.Runner
	projectSummary string // populated post-run by maybeRunProjectSummary
}

// ProjectSummary returns the markdown project-level summary produced after
// all batches finish. Empty when SkipSummary is set, PROJECT_SUMMARY_TASK
// is absent, no comments were collected, or the summary LLM call failed.
func (a *Agent) ProjectSummary() string { return a.projectSummary }

// NewAgent creates a scan Agent from the given args. The Session is
// auto-created (review_mode = full_scan) when not supplied.
func NewAgent(args Args) *Agent {
	if args.Tools == nil {
		args.Tools = tool.NewRegistry()
	}
	if args.CommentCollector == nil {
		args.CommentCollector = tool.NewCommentCollector()
	}
	if args.Session == nil {
		args.Session = session.New(args.RepoDir, "", args.Model, session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		})
	}
	a := &Agent{
		args:    args,
		session: args.Session,
	}
	a.runner = llmloop.NewRunner(llmloop.Deps{
		LLMClient:         args.LLMClient,
		Model:             args.Model,
		Template:          toLoopTemplate(args.Template),
		Tools:             args.Tools,
		MainToolDefs:      args.MainToolDefs,
		CommentCollector:  args.CommentCollector,
		CommentWorkerPool: args.CommentWorkerPool,
		Session:           args.Session,
		// DiffLookup returns a synthetic Diff so the code_comment tool's
		// line-number resolver (resolveFromFileContent) can match against
		// the full file content of the scanned file.
		DiffLookup: a.lookupDiff,
	})
	return a
}

// toLoopTemplate maps the scan-specific ScanTemplate onto the subset of
// fields llmloop.Runner reads from template.Template. llmloop only needs
// MaxTokens / MaxToolRequestTimes / MemoryCompressionTask / ReLocationTask,
// so we leave the diff-only fields (MainTask / PlanTask / ReviewFilterTask)
// at their zero values.
func toLoopTemplate(s template.ScanTemplate) template.Template {
	return template.Template{
		MemoryCompressionTask: s.MemoryCompressionTask,
		MaxTokens:             s.MaxTokens,
		MaxToolRequestTimes:   s.MaxToolRequestTimes,
		ReLocationTask:        s.ReLocationTask,
	}
}

// Session returns the session history associated with this Agent.
func (a *Agent) Session() *session.SessionHistory { return a.session }

// FilesReviewed returns the number of items included in this scan.
func (a *Agent) FilesReviewed() int64 { return int64(len(a.items)) }

// Diffs returns the scanned items adapted to model.Diff form so callers
// (e.g. cmd/opencodereview's outputJSON / ResolveLineNumbers) can treat
// both review and scan results uniformly.
func (a *Agent) Diffs() []model.Diff {
	out := make([]model.Diff, len(a.items))
	for i := range a.items {
		out[i] = *a.items[i].AsDiff()
	}
	return out
}

// TotalTokensUsed / TotalInputTokens / ... delegate to the underlying runner.
func (a *Agent) TotalTokensUsed() int64      { return a.runner.TotalTokensUsed() }
func (a *Agent) TotalInputTokens() int64     { return a.runner.TotalInputTokens() }
func (a *Agent) TotalOutputTokens() int64    { return a.runner.TotalOutputTokens() }
func (a *Agent) TotalCacheReadTokens() int64 { return a.runner.TotalCacheReadTokens() }
func (a *Agent) TotalCacheWriteTokens() int64 {
	return a.runner.TotalCacheWriteTokens()
}

// Warnings returns the warnings recorded by the LLM runner.
func (a *Agent) Warnings() []llmloop.AgentWarning { return a.runner.Warnings() }

// ToolCalls returns per-tool call counts accumulated during scan.
func (a *Agent) ToolCalls() map[string]int64 { return a.runner.ToolCalls() }

func (a *Agent) recordWarning(warningType, file, message string) {
	a.runner.RecordWarning(warningType, file, message)
}

// Run executes the full-scan pipeline: enumerate → filter → token-filter →
// dispatch one subtask per file → collect comments.
func (a *Agent) Run(ctx context.Context) ([]model.LlmComment, error) {
	if len(a.args.Template.MainTask.Messages) == 0 {
		return nil, fmt.Errorf("scan template MAIN_TASK is missing or empty")
	}

	ctx, scanSpan := telemetry.StartSpan(ctx, "scan.enumerate")
	provider := NewProvider(a.args.RepoDir, a.args.Paths, a.args.GitRunner, a.args.MaxFileSizeBytes)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		scanSpan.End()
		return nil, fmt.Errorf("enumerate files: %w", err)
	}
	telemetry.SetAttr(scanSpan, "files.enumerated", len(items))
	scanSpan.End()

	a.items = items
	a.injectScanContentMap()
	a.args.Tools.Freeze()

	totalDiscovered := len(a.items)
	a.items = a.filterScanItems(a.items)
	a.items = a.filterLargeScans(a.items)

	reviewable := len(a.items)
	fmt.Fprintf(stdout.Writer(), "[ocr] full-scan: %d file(s) discovered, reviewing %d in %s\n",
		totalDiscovered, reviewable, a.args.RepoDir)

	if reviewable == 0 {
		fmt.Fprintln(stdout.Writer(), "[ocr] No reviewable files. Skipping scan.")
		telemetry.Event(ctx, "scan.no.files")
		a.session.Finalize()
		return []model.LlmComment{}, nil
	}

	// Pre-run cost projection so users aren't surprised by a large scan.
	est := estimateCost(a.items, a.planEnabled(), a.dedupEnabled(), a.summaryEnabled())
	fmt.Fprintf(stdout.Writer(), "[ocr] estimated cost: %s\n", est)
	if a.args.MaxTokensBudget > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] token budget: %s (dispatch stops once exceeded)\n", humanTokens(a.args.MaxTokensBudget))
		if est.TotalTokens > a.args.MaxTokensBudget {
			fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: estimate (%s) exceeds budget (%s); scan will stop partway\n",
				humanTokens(est.TotalTokens), humanTokens(a.args.MaxTokensBudget))
		}
	}

	a.currentDate = time.Now().Format("2006-01-02 15:04")
	telemetry.Event(ctx, "scan.started",
		telemetry.AnyToAttr("file.count", totalDiscovered),
		telemetry.AnyToAttr("review.count", reviewable),
		telemetry.AnyToAttr("est.total.tokens", est.TotalTokens),
		telemetry.AnyToAttr("repo.dir", a.args.RepoDir))
	telemetry.RecordFilesReviewed(ctx, int64(reviewable))

	comments, err := a.dispatchSubtasks(ctx)
	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}

	// Project-level summary runs after all batches; never blocks return.
	a.maybeRunProjectSummary(ctx, comments)

	a.session.Finalize()
	return comments, err
}

// lookupDiff returns the synthetic Diff for a path, used by llmloop.Runner
// to resolve code_comment line numbers against the scanned file content.
func (a *Agent) lookupDiff(path string) *model.Diff {
	for i := range a.items {
		if a.items[i].Path == path {
			return a.items[i].AsDiff()
		}
	}
	return nil
}

// injectScanContentMap fills the file_read_diff tool's DiffMap with full
// file content keyed by path, so if the model calls it the tool returns
// the whole file rather than failing.
func (a *Agent) injectScanContentMap() {
	m := make(map[string]string, len(a.items))
	for i := range a.items {
		it := &a.items[i]
		if it.Path != "" {
			m[it.Path] = it.Content
		}
	}
	dm := tool.NewDiffMap(m)
	if p, ok := a.args.Tools.Get(tool.FileReadDiff.Name()); ok {
		if frd, ok := p.(*tool.FileReadDiffProvider); ok {
			frd.SetDiffMap(dm)
		}
	}
}

// filterScanItems drops items that should not be reviewed under the standard
// reviewability rules (binary, extension allowlist, user include/exclude,
// default excluded paths).
func (a *Agent) filterScanItems(items []model.ScanItem) []model.ScanItem {
	var kept []model.ScanItem
	skipped := 0
	for _, it := range items {
		if reason := a.whyExcluded(it); reason != model.ExcludeNone {
			if it.IsBinary {
				fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s — binary file\n", it.Path)
			} else {
				fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s — filtered by path/extension rules\n", it.Path)
			}
			skipped++
			continue
		}
		kept = append(kept, it)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] Filtered %d file(s) by include/exclude rules\n", skipped)
	}
	return kept
}

// filterLargeScans drops items whose content exceeds 80% of MaxTokens.
func (a *Agent) filterLargeScans(items []model.ScanItem) []model.ScanItem {
	limit := a.args.Template.MaxTokens * 4 / 5
	if limit <= 0 {
		return items
	}
	var kept []model.ScanItem
	skipped := 0
	for _, it := range items {
		tokens := llm.CountTokens(it.Content)
		if tokens > limit {
			fmt.Fprintf(stdout.Writer(), "[ocr] Skipping %s (~%d tokens exceeds 80%% of max_tokens(%d))\n",
				it.Path, tokens, a.args.Template.MaxTokens)
			skipped++
			continue
		}
		kept = append(kept, it)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] Pre-filtered %d file(s) exceeding 80%% of max_tokens\n", skipped)
	}
	return kept
}

// whyExcluded mirrors agent.whyExcluded but for ScanItem inputs.
func (a *Agent) whyExcluded(it model.ScanItem) model.ExcludeReason {
	if it.IsBinary {
		return model.ExcludeBinary
	}
	path := it.Path
	if a.args.FileFilter != nil && a.args.FileFilter.IsUserExcluded(path) {
		return model.ExcludeUserRule
	}
	ext := extFromPath(path)
	if ext != "" && !allowedext.IsAllowedExt(ext) {
		return model.ExcludeExtension
	}
	if a.args.FileFilter != nil && a.args.FileFilter.HasInclude() && a.args.FileFilter.IsUserIncluded(path) {
		return model.ExcludeNone
	}
	if allowedext.IsExcludedPath(path) {
		return model.ExcludeDefaultPath
	}
	return model.ExcludeNone
}

func extFromPath(path string) string {
	basename := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		basename = path[idx+1:]
	}
	dot := strings.LastIndex(basename, ".")
	if dot <= 0 {
		return ""
	}
	return strings.ToLower(basename[dot:])
}

// dispatchSubtasks groups items into batches per the configured strategy,
// then processes batches sequentially while running files within each
// batch concurrently up to MaxConcurrency. Sequential batches enable
// future per-batch hooks (e.g. Phase 6 dedup) and improve LLM prompt-cache
// hit rate by keeping same-language files adjacent in time.
func (a *Agent) dispatchSubtasks(ctx context.Context) ([]model.LlmComment, error) {
	startTime := time.Now()
	defer func() {
		telemetry.RecordReviewDuration(ctx, time.Since(startTime))
	}()

	if len(a.items) == 0 {
		return []model.LlmComment{}, nil
	}

	atomic.StoreInt64(&a.subtaskFailed, 0)

	strategy := a.resolveBatchStrategy()
	batches := groupBatches(a.items, strategy, a.args.Template.BatchSize)
	fmt.Fprintf(stdout.Writer(), "[ocr] scan dispatch: %d batch(es) by %s strategy\n", len(batches), strategy)

	var dispatched int64
	for bi, batch := range batches {
		if err := ctx.Err(); err != nil {
			return a.args.CommentCollector.Comments(), err
		}
		// Snapshot the collector so we can isolate comments added by *this*
		// batch and feed them into the per-batch dedup hook.
		batchStart := a.args.CommentCollector.Snapshot()

		n, budgetHit, err := a.dispatchBatch(ctx, bi, batch)
		dispatched += n
		if err != nil {
			// ctx cancelled mid-batch: stop scheduling further batches but
			// still return whatever we've collected so far.
			return a.args.CommentCollector.Comments(), err
		}

		// Drain async comment workers BEFORE dedup so all of this batch's
		// comments are visible. CommentWorkerPool.Await is cumulative
		// across batches — that's fine since batches are sequential here.
		if a.args.CommentWorkerPool != nil {
			a.args.CommentWorkerPool.Await()
		}

		a.maybeRunDedup(ctx, bi, batchStart)

		// The per-file budget gate inside dispatchBatch tripped — stop
		// scheduling any remaining batches.
		if budgetHit {
			break
		}
	}

	failed := atomic.LoadInt64(&a.subtaskFailed)
	if failed > 0 && failed == dispatched {
		return nil, fmt.Errorf("all %d file scan(s) failed — check your LLM configuration and API key", dispatched)
	}
	return a.args.CommentCollector.Comments(), nil
}

// resolveBatchStrategy reads the strategy from the scan template, defaulting
// to BatchNone for unrecognized / empty values.
func (a *Agent) resolveBatchStrategy() BatchStrategy {
	return parseBatchStrategy(a.args.Template.BatchStrategy)
}

// dispatchBatch fans out the files of a single batch concurrently and
// blocks until they all finish (or ctx is cancelled). Returns the number
// of files dispatched, whether the token budget was hit mid-batch, and
// ctx.Err() if cancelled.
//
// The budget gate is checked per file, right after acquiring the
// concurrency slot and before launching the subtask: if the tokens already
// spent PLUS a look-ahead estimate of this file's cost would exceed the
// budget, the file (and all remaining files in the batch) are skipped.
// This keeps overrun bounded by roughly one in-flight file per worker,
// instead of a whole batch as the coarse batch-level gate did.
func (a *Agent) dispatchBatch(ctx context.Context, batchIdx int, batch []model.ScanItem) (int64, bool, error) {
	concurrency := a.args.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	timeout := time.Duration(a.args.ConcurrentTaskTimeout) * time.Minute

	var (
		wg         sync.WaitGroup
		dispatched int64
		budgetHit  bool
	)

	for i := range batch {
		// Per-file budget look-ahead. Stop before acquiring a slot so we
		// don't even queue work that would blow the budget.
		if a.args.MaxTokensBudget > 0 {
			used := a.runner.TotalTokensUsed()
			projected := used + estimateFileTokens(batch[i], a.planEnabled())
			if projected > a.args.MaxTokensBudget {
				fmt.Fprintf(stdout.Writer(), "[ocr] token budget reached (used %s + next-file est ≈ %s > budget %s) — skipping %s and remaining files\n",
					humanTokens(used), humanTokens(projected), humanTokens(a.args.MaxTokensBudget), batch[i].Path)
				a.recordWarning("token_budget_reached", batch[i].Path,
					fmt.Sprintf("stopped in batch #%d: used %d tokens + next-file estimate exceeds budget %d", batchIdx, used, a.args.MaxTokensBudget))
				budgetHit = true
				break
			}
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return dispatched, budgetHit, ctx.Err()
		}

		dispatched++
		wg.Add(1)
		go func(it model.ScanItem) {
			defer wg.Done()
			defer func() { <-sem }()

			var fileCtx context.Context
			var cancel context.CancelFunc
			if timeout > 0 {
				fileCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			} else {
				fileCtx = ctx
			}

			if err := a.executeSubtask(fileCtx, it); err != nil {
				atomic.AddInt64(&a.subtaskFailed, 1)
				fmt.Fprintf(stdout.Writer(), "[ocr] Scan subtask error for %s (batch #%d): %v\n", it.Path, batchIdx, err)
				telemetry.ErrorEvent(fileCtx, "scan.subtask.error", err,
					telemetry.AnyToAttr("file.path", it.Path),
					telemetry.AnyToAttr("batch.index", batchIdx))
				a.recordWarning("scan_subtask_error", it.Path, err.Error())
			}
		}(batch[i])
	}

	wg.Wait()
	return dispatched, budgetHit, nil
}

// executeSubtask runs the scan pipeline for one file:
//  1. Optional PLAN_TASK: produce a JSON checklist of focus areas.
//  2. MAIN_TASK: review the file with the plan's checkpoints embedded as
//     {{plan_guidance}}.
//
// Plan phase is skipped (and {{plan_guidance}} is filled with a "no plan"
// sentinel) when Template.PlanTask is nil, args.SkipPlan is true, the file
// is small enough that planning overhead outweighs gain, or the plan call
// itself fails. Plan failure never blocks the main review — it falls back
// to v1 (plan-less) behavior.
func (a *Agent) executeSubtask(ctx context.Context, it model.ScanItem) error {
	ctx, span := telemetry.StartSpan(ctx, "scan.subtask."+it.Path)
	defer span.End()
	telemetry.SetAttr(span, "file.path", it.Path)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	rule := ""
	if a.args.SystemRule != nil {
		rule = a.args.SystemRule.Resolve(strings.ToLower(it.Path))
	}

	planGuidance := a.maybeRunPlan(ctx, it, rule)

	messages := a.renderMessages(it, rule, planGuidance)

	tokenCount := llmloop.CountMessagesTokens(messages)
	maxAllowed := a.args.Template.MaxTokens
	tokenLimit := maxAllowed * 4 / 5
	if tokenCount > tokenLimit {
		msg := fmt.Sprintf("prompt tokens (%d) exceed %d%% of max_tokens(%d)", tokenCount, 80, maxAllowed)
		fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: %s for %s\n", msg, it.Path)
		a.recordWarning("token_threshold_exceeded", it.Path, msg)
		telemetry.Event(ctx, "token.threshold.exceeded",
			telemetry.AnyToAttr("file.path", it.Path),
			telemetry.AnyToAttr("tokens", tokenCount),
			telemetry.AnyToAttr("max_tokens", maxAllowed))
		return nil
	}

	return a.runner.RunPerFile(ctx, messages, it.Path)
}

// maybeRunPlan invokes PLAN_TASK on the file and returns a human-readable
// guidance string suitable for {{plan_guidance}} substitution. Returns "(no
// pre-scan plan; review the entire file as usual)" when planning is
// disabled or fails — that sentinel is intentionally non-empty so the
// surrounding "### Pre-scan Focus Areas" header in MAIN_TASK has content
// instead of dangling.
func (a *Agent) maybeRunPlan(ctx context.Context, it model.ScanItem, rule string) string {
	const noPlan = "(no pre-scan plan; review the entire file as usual)"

	if !a.planEnabled() {
		return noPlan
	}
	pt := a.args.Template.PlanTask

	// Render plan messages.
	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{current_file_path}}", it.Path)
		content = strings.ReplaceAll(content, "{{system_rule}}", rule)
		content = strings.ReplaceAll(content, "{{file_content}}", it.Content)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	fs := a.session.GetOrCreateFileSession(it.Path)
	rec := fs.AppendTaskRecord(session.PlanTask, messages)
	startTime := time.Now()

	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		rec.SetError(err, time.Since(startTime))
		fmt.Fprintf(stdout.Writer(), "[ocr] scan plan failed for %s: %v (falling back to plan-less)\n", it.Path, err)
		return noPlan
	}
	rec.SetResponse(resp, time.Since(startTime))
	a.runner.RecordUsage(resp.Usage)

	guidance := formatPlanGuidance(resp.Content())
	if guidance == "" {
		return noPlan
	}
	return guidance
}

// maybeRunProjectSummary runs the post-batch PROJECT_SUMMARY_TASK over the
// union of all collected comments. Best-effort: any error / empty input
// / no-template silently leaves projectSummary unset.
func (a *Agent) maybeRunProjectSummary(ctx context.Context, comments []model.LlmComment) {
	if !a.summaryEnabled() {
		return
	}
	pt := a.args.Template.ProjectSummaryTask
	if len(comments) == 0 {
		return
	}

	// Distinct file count for header context.
	fileSet := make(map[string]struct{}, len(comments))
	for _, c := range comments {
		fileSet[c.Path] = struct{}{}
	}
	payload := buildSummaryCommentsList(comments)

	messages := make([]llm.Message, 0, len(pt.Messages))
	for _, m := range pt.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{{comment_count}}", fmt.Sprintf("%d", len(comments)))
		content = strings.ReplaceAll(content, "{{file_count}}", fmt.Sprintf("%d", len(fileSet)))
		content = strings.ReplaceAll(content, "{{all_comments}}", payload)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	const pathKey = "__scan_project_summary__"
	fs := a.session.GetOrCreateFileSession(pathKey)
	rec := fs.AppendTaskRecord(session.MemoryCompressionTask, messages) // reuse existing task type
	startTime := time.Now()

	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		rec.SetError(err, time.Since(startTime))
		fmt.Fprintf(stdout.Writer(), "[ocr] scan project summary failed: %v\n", err)
		return
	}
	rec.SetResponse(resp, time.Since(startTime))
	a.runner.RecordUsage(resp.Usage)

	body := strings.TrimSpace(llmloop.StripMarkdownFences(resp.Content()))
	if body == "" {
		return
	}
	a.projectSummary = body
}

// buildSummaryCommentsList renders comments as a compact path-anchored
// markdown list suitable for embedding in the PROJECT_SUMMARY_TASK prompt.
// Format: "- `path/to/file.go`: <one-line content (truncated)>".
// Content is truncated to ~280 chars to bound prompt growth on large scans.
func buildSummaryCommentsList(comments []model.LlmComment) string {
	const maxLine = 280
	var sb strings.Builder
	for _, c := range comments {
		sb.WriteString("- `")
		sb.WriteString(c.Path)
		sb.WriteString("`: ")
		oneLine := strings.ReplaceAll(c.Content, "\n", " ")
		if len(oneLine) > maxLine {
			oneLine = oneLine[:maxLine] + "..."
		}
		sb.WriteString(oneLine)
		sb.WriteString("\n")
	}
	return sb.String()
}

// maybeRunDedup, when the template has a DedupTask and the batch produced
// at least DedupMinComments comments, invokes the DEDUP_TASK LLM to merge
// near-duplicate findings. On any failure (LLM error / malformed JSON /
// invalid grouping) the original batch comments are kept unchanged — dedup
// is a best-effort optimization, never a correctness gate.
func (a *Agent) maybeRunDedup(ctx context.Context, batchIdx, batchStart int) {
	if !a.dedupEnabled() {
		return
	}
	dt := a.args.Template.DedupTask
	minN := a.args.Template.DedupMinComments
	if minN <= 0 {
		minN = 2
	}

	batchComments := a.args.CommentCollector.Since(batchStart)
	if len(batchComments) < minN {
		return
	}

	payload := buildDedupCommentsJSON(batchComments)
	messages := make([]llm.Message, 0, len(dt.Messages))
	for _, m := range dt.Messages {
		content := strings.ReplaceAll(m.Content, "{{batch_comments}}", payload)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	// Use a synthetic file path keyed by batch index so the session JSONL
	// keeps dedup records distinct from per-file plan/main records.
	pathKey := fmt.Sprintf("__scan_dedup_batch_%d__", batchIdx)
	fs := a.session.GetOrCreateFileSession(pathKey)
	rec := fs.AppendTaskRecord(session.MemoryCompressionTask, messages) // reuse existing task type; no scan-specific type to invent
	startTime := time.Now()

	resp, err := a.args.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     a.args.Model,
		Messages:  messages,
		MaxTokens: a.args.Template.MaxTokens,
	})
	if err != nil {
		rec.SetError(err, time.Since(startTime))
		fmt.Fprintf(stdout.Writer(), "[ocr] scan dedup failed for batch #%d: %v (keeping originals)\n", batchIdx, err)
		return
	}
	rec.SetResponse(resp, time.Since(startTime))
	a.runner.RecordUsage(resp.Usage)

	deduped, ok := applyDedupGroups(resp.Content(), batchComments)
	if !ok {
		fmt.Fprintf(stdout.Writer(), "[ocr] scan dedup batch #%d: malformed groups, keeping originals\n", batchIdx)
		return
	}
	if len(deduped) == len(batchComments) {
		// No-op result — don't bother rewriting the collector.
		return
	}
	a.args.CommentCollector.ReplaceSince(batchStart, deduped)
	fmt.Fprintf(stdout.Writer(), "[ocr] scan dedup batch #%d: %d → %d comments\n", batchIdx, len(batchComments), len(deduped))
}

// buildDedupCommentsJSON renders the batch comments as a JSON list with
// stable c-N ids that the LLM groups by. Only fields the LLM needs to
// judge similarity are included (path / content / existing_code), keeping
// the prompt compact.
func buildDedupCommentsJSON(comments []model.LlmComment) string {
	type wire struct {
		ID           string `json:"id"`
		Path         string `json:"path"`
		Content      string `json:"content"`
		ExistingCode string `json:"existing_code,omitempty"`
	}
	items := make([]wire, len(comments))
	for i, cm := range comments {
		items[i] = wire{
			ID:           fmt.Sprintf("c-%d", i),
			Path:         cm.Path,
			Content:      cm.Content,
			ExistingCode: cm.ExistingCode,
		}
	}
	data, _ := json.Marshal(items)
	return string(data)
}

// applyDedupGroups parses the DEDUP_TASK output and returns the deduped
// comment slice. Returns (nil, false) when the response is malformed OR
// when the groups don't cover every input id exactly once (safety: we
// refuse to silently drop comments we can't account for).
func applyDedupGroups(rawJSON string, originals []model.LlmComment) ([]model.LlmComment, bool) {
	stripped := llmloop.StripMarkdownFences(rawJSON)
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return nil, false
	}
	var parsed struct {
		Groups []struct {
			Members       []string `json:"members"`
			MergedContent string   `json:"merged_content,omitempty"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(stripped), &parsed); err != nil {
		return nil, false
	}

	idToIdx := make(map[string]int, len(originals))
	for i := range originals {
		idToIdx[fmt.Sprintf("c-%d", i)] = i
	}

	seen := make(map[string]bool, len(originals))
	var out []model.LlmComment
	for _, g := range parsed.Groups {
		if len(g.Members) == 0 {
			return nil, false
		}
		canonicalIdx, ok := idToIdx[g.Members[0]]
		if !ok {
			return nil, false
		}
		for _, id := range g.Members {
			if _, exists := idToIdx[id]; !exists {
				return nil, false // unknown id
			}
			if seen[id] {
				return nil, false // duplicate assignment
			}
			seen[id] = true
		}
		canonical := originals[canonicalIdx]
		if len(g.Members) > 1 && g.MergedContent != "" {
			canonical.Content = g.MergedContent
		}
		out = append(out, canonical)
	}

	if len(seen) != len(originals) {
		return nil, false // some id missing
	}
	return out, true
}

// formatPlanGuidance parses the PLAN_TASK JSON output into a markdown
// snippet suitable for embedding in MAIN_TASK. On parse failure it returns
// the raw content so the model still gets *something* (better than the
// "no plan" fallback when the LLM did say something useful but in the
// wrong shape).
func formatPlanGuidance(raw string) string {
	stripped := llmloop.StripMarkdownFences(raw)
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return ""
	}

	var plan struct {
		Summary     string `json:"summary"`
		Checkpoints []struct {
			Focus string `json:"focus"`
			Lines string `json:"lines,omitempty"`
			Why   string `json:"why,omitempty"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal([]byte(stripped), &plan); err != nil {
		// Fallback: hand the raw text to the main task; it's better than
		// nothing and lets us debug bad outputs from session JSONL.
		return stripped
	}

	var sb strings.Builder
	if plan.Summary != "" {
		sb.WriteString("**Summary**: ")
		sb.WriteString(plan.Summary)
		sb.WriteString("\n\n")
	}
	if len(plan.Checkpoints) == 0 {
		// Summary-only plan still has value as orientation.
		return strings.TrimRight(sb.String(), "\n")
	}
	sb.WriteString("**Focus areas (give these extra attention; not exhaustive):**\n")
	for i, cp := range plan.Checkpoints {
		fmt.Fprintf(&sb, "%d. `%s`", i+1, cp.Focus)
		if cp.Lines != "" {
			fmt.Fprintf(&sb, " (lines %s)", cp.Lines)
		}
		if cp.Why != "" {
			fmt.Fprintf(&sb, " — %s", cp.Why)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderMessages substitutes placeholders in the scan template's MainTask
// for a single scan item. planGuidance is the output of maybeRunPlan and
// gets substituted into {{plan_guidance}}; callers should pass a non-empty
// sentinel when planning is disabled so the surrounding section header in
// the prompt template doesn't dangle.
func (a *Agent) renderMessages(it model.ScanItem, rule, planGuidance string) []llm.Message {
	rawMsgs := a.args.Template.MainTask.Messages
	messages := make([]llm.Message, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		content := m.Content
		content = strings.ReplaceAll(content, "{{plan_guidance}}", planGuidance)
		content = strings.ReplaceAll(content, "{{current_system_date_time}}", a.currentDate)
		content = strings.ReplaceAll(content, "{{current_file_path}}", it.Path)
		content = strings.ReplaceAll(content, "{{system_rule}}", rule)
		content = strings.ReplaceAll(content, "{{change_files}}", changeFilesScanLiteral)
		content = strings.ReplaceAll(content, "{{file_content}}", it.Content)
		content = strings.ReplaceAll(content, "{{requirement_background}}", a.args.Background)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}
	return messages
}
