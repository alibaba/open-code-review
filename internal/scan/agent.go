package scan

import (
	"context"
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

// Args bundles all dependencies needed for one scan session. Mirrors the
// fields of internal/agent.Args that scan actually uses, minus diff-only
// concerns (From / To / Commit / PlanToolDefs).
type Args struct {
	RepoDir               string
	Paths                 []string // empty = whole repo
	Template              template.Template
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
}

// Agent orchestrates full-file code review. It delegates the per-file LLM
// tool-use loop to llmloop.Runner and owns only scan-specific concerns
// (file enumeration, FULL_SCAN_TASK rendering, per-file filtering).
type Agent struct {
	args          Args
	items         []model.ScanItem
	currentDate   string
	session       *session.SessionHistory
	subtaskFailed int64 // atomic
	runner        *llmloop.Runner
}

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
		Template:          args.Template,
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

func (a *Agent) recordWarning(warningType, file, message string) {
	a.runner.RecordWarning(warningType, file, message)
}

// Run executes the full-scan pipeline: enumerate → filter → token-filter →
// dispatch one subtask per file → collect comments.
func (a *Agent) Run(ctx context.Context) ([]model.LlmComment, error) {
	if a.args.Template.FullScanTask == nil || len(a.args.Template.FullScanTask.Messages) == 0 {
		return nil, fmt.Errorf("FULL_SCAN_TASK template is missing or empty")
	}

	ctx, scanSpan := telemetry.StartSpan(ctx, "scan.enumerate")
	provider := NewProvider(a.args.RepoDir, a.args.Paths, a.args.GitRunner)
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

	a.currentDate = time.Now().Format("2006-01-02 15:04")
	telemetry.Event(ctx, "scan.started",
		telemetry.AnyToAttr("file.count", totalDiscovered),
		telemetry.AnyToAttr("review.count", reviewable),
		telemetry.AnyToAttr("repo.dir", a.args.RepoDir))
	telemetry.RecordFilesReviewed(ctx, int64(reviewable))

	comments, err := a.dispatchSubtasks(ctx)
	if len(comments) > 0 {
		telemetry.RecordCommentsGenerated(ctx, int64(len(comments)))
	}
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

// dispatchSubtasks fans out one subtask per item with a bounded goroutine
// pool. Mirrors the agent's dispatchSubtasks structure but works on
// ScanItem and delegates the per-file LLM loop to llmloop.Runner.
func (a *Agent) dispatchSubtasks(ctx context.Context) ([]model.LlmComment, error) {
	startTime := time.Now()
	defer func() {
		telemetry.RecordReviewDuration(ctx, time.Since(startTime))
	}()

	if len(a.items) == 0 {
		return []model.LlmComment{}, nil
	}

	atomic.StoreInt64(&a.subtaskFailed, 0)

	concurrency := a.args.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	timeout := time.Duration(a.args.ConcurrentTaskTimeout) * time.Minute

	var (
		wg         sync.WaitGroup
		dispatched int64
	)

	for i := range a.items {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return a.args.CommentCollector.Comments(), ctx.Err()
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
				fmt.Fprintf(stdout.Writer(), "[ocr] Scan subtask error for %s: %v\n", it.Path, err)
				telemetry.ErrorEvent(fileCtx, "scan.subtask.error", err,
					telemetry.AnyToAttr("file.path", it.Path))
				a.recordWarning("scan_subtask_error", it.Path, err.Error())
			}
		}(a.items[i])
	}

	wg.Wait()

	if a.args.CommentWorkerPool != nil {
		a.args.CommentWorkerPool.Await()
	}

	failed := atomic.LoadInt64(&a.subtaskFailed)
	if failed > 0 && failed == dispatched {
		return nil, fmt.Errorf("all %d file scan(s) failed — check your LLM configuration and API key", dispatched)
	}
	return a.args.CommentCollector.Comments(), nil
}

// executeSubtask renders the FULL_SCAN_TASK template for one item and
// invokes the shared LLM loop. Plan phase is intentionally skipped.
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

	messages := a.renderMessages(it, rule)

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

// renderMessages substitutes placeholders in the FULL_SCAN_TASK template
// for a single scan item.
func (a *Agent) renderMessages(it model.ScanItem, rule string) []llm.Message {
	rawMsgs := a.args.Template.FullScanTask.Messages
	messages := make([]llm.Message, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		content := m.Content
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
