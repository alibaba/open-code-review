// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/github"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"

	"go.opentelemetry.io/otel/codes"
)

type reviewOptions struct {
	toolConfigPath  string
	rulePath        string
	repoDir         string
	from            string
	to              string
	commit          string
	resume          string
	excludes        string
	outputFormat    string
	audience        string
	outputPath      string
	background      string
	backgroundFile  string
	provider        string
	model           string
	concurrency     int
	perFileTimeout  int
	maxTools        int
	maxGitProcs     int
	maxTokens       int
	maxTokensBudget int
	effort          string
	noFilter        bool
	preview         bool
	// GitHub integration flags
	githubToken string
	postToPR    bool
	// reviewedHeadSHA comes from the fully sealed range passed to the agent, so
	// PR posting cannot drift to a commit pushed while the review is in progress.
	reviewedHeadSHA string
}

var reviewOpts reviewOptions

var reviewCmd = &cobra.Command{
	Use:     "review [flags]",
	Aliases: []string{"r"},
	Short:   "Start a diff-based code review",
	Long:    "OpenCodeReview - AI-Powered Code Review CLI\n\nStart a diff-based code review using a configurable LLM.",
	Args:    cobra.NoArgs,
	Example: `  # Review staged + unstaged + untracked changes in current workspace
  ocr review

  # Review a branch against its base (merge-base mode)
  ocr review --from master --to dev-ref

  # Review a specific commit
  ocr review --commit abc123
  ocr review -c abc123

  # Resume a previous range review
  ocr review --from master --to dev-ref --resume <session-id>

  # Output JSON format
  ocr review --format json
  ocr review -f json

  # Select a configured provider and model for this run only
  ocr review --provider anthropic --model claude-opus-4-6 --format json

  # Agent mode (summary only, no progress lines)
  ocr review --audience agent

  # Preview which files will be reviewed
  ocr review --preview
  ocr review -c abc123 -p

  # Exclude generated files / fixtures
  ocr review --exclude '**/generated/*,**/testdata/*'

  # Provide requirement/business context inline or from a Markdown file
  ocr review --background "Adding rate limiting to the login API"
  ocr review --background-file ./docs/requirements.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateReviewOptions(&reviewOpts); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer stop()
		return executeReviewContext(ctx, reviewOpts)
	},
}

func init() {
	registerReviewFlags(reviewCmd, &reviewOpts)
}

func executeReviewContext(ctx context.Context, opts reviewOptions) (retErr error) {
	out, closeOut, err := resolveOutputWriter(opts.outputPath, opts.outputFormat)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := closeOut(); cerr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close output file: %w", cerr))
		}
	}()

	contentRef, _ := tool.ParseReviewMode(opts.from, opts.to, opts.commit).RefValue(opts.to, opts.commit)
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, contentRef, opts.maxTools, opts.maxGitProcs, true)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// Security (#112): reject ref-option injection before any git invocation.
	if err := validateReviewRefs(cc.RepoDir, opts); err != nil {
		return err
	}

	bg, err := resolveBackground(cc.RepoDir, opts.background, opts.backgroundFile, opts.commit)
	if err != nil {
		return err
	}
	opts.background = bg

	if opts.preview {
		return runPreviewContext(ctx, cc, opts, out)
	}

	resumeState, err := loadReviewResumeState(cc.RepoDir, opts)
	if err != nil {
		return err
	}

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, llm.ResolveOptions{
		Provider: opts.provider,
		Model:    opts.model,
	})
	if err != nil {
		return err
	}
	maxTokens, err := resolveMaxTokens(cc.Template.MaxTokens, rt.AppCfg, opts.maxTokens)
	if err != nil {
		return err
	}
	cc.Template.MaxTokens = maxTokens

	effort, err := resolveEffort(rt.AppCfg, opts.effort)
	if err != nil {
		return err
	}
	cc.Template.ApplyEffort(effort)

	// Strictly before agent.New, so a rejected resume persists nothing. The sealed
	// input it returns pins the run to the very commits this check passed on, so
	// the decision cannot be undone by a ref moving afterwards.
	sealed, err := validateResumeIdentity(ctx, cc, opts, rt, resumeState)
	if err != nil {
		return err
	}
	if opts.postToPR {
		if sealed == nil || sealed.Resolution.ResolvedBase == "" || sealed.Resolution.ResolvedHead == "" {
			return fmt.Errorf("resolve immutable range for GitHub posting")
		}
		opts.reviewedHeadSHA = sealed.Resolution.ResolvedHead
	}

	llmIdentity := &jsonLLMIdentity{
		Provider: rt.Provider,
		Model:    rt.Model,
	}

	var sealedInput *diff.InputResolution
	if sealed != nil {
		sealedInput = &sealed.Resolution
	}

	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit)
	fileReader := &tool.FileReader{
		RepoDir: cc.RepoDir,
		Mode:    mode,
		Ref:     fileReadRef(mode, opts, sealedInput),
		Runner:  cc.GitRunner,
	}
	tools := buildToolRegistry(rt.Collector, fileReader)

	mcpClients := initMCPClients(ctx, rt.AppCfg, tools, cc.RepoDir, Version)
	defer func() {
		for _, mc := range mcpClients {
			if err := mc.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to close MCP server %q: %v\n", mc.Name(), err)
			}
		}
	}()

	mcpToolDefs := mcp.CollectToolDefs(mcpClients, tools)
	rt.PlanToolDefs = append(rt.PlanToolDefs, mcpToolDefs...)
	rt.MainToolDefs = append(rt.MainToolDefs, mcpToolDefs...)

	ag := agent.New(agent.Args{
		RepoDir:               cc.RepoDir,
		From:                  opts.from,
		To:                    opts.to,
		Commit:                opts.commit,
		ReviewMode:            reviewModeFromOptions(opts),
		Template:              *cc.Template,
		SystemRule:            cc.Resolver,
		FileFilter:            cc.FileFilter,
		LLMClient:             rt.Client,
		Tools:                 tools,
		PlanToolDefs:          rt.PlanToolDefs,
		MainToolDefs:          rt.MainToolDefs,
		CommentCollector:      rt.Collector,
		CommentWorkerPool:     agent.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:        opts.concurrency,
		ConcurrentTaskTimeout: opts.perFileTimeout,
		Model:                 rt.Model,
		Provider:              rt.Provider,
		Background:            opts.background,
		GitRunner:             cc.GitRunner,
		Resume:                resumeState,
		SealedInput:           sealedInput,
		MaxTokensBudget:       int64(opts.maxTokensBudget),
		SkipFilter:            opts.noFilter,
		RuntimeConfig:         rt.RuntimeConfig,
	})

	// Silence progress output during execution; restored before the trace
	// summary in agent-text mode (and on function exit otherwise).
	q := newQuietHandle(opts.outputFormat, opts.audience)
	defer q.Restore()

	runCtx, span := telemetry.StartSpan(telemetry.ContextWithTraceParentFromEnv(ctx), "review.run")
	defer span.End()
	telemetry.SetAttr(span, "review.repo", cc.RepoDir)
	telemetry.SetAttr(span, "review.from", opts.from)
	telemetry.SetAttr(span, "review.to", opts.to)
	telemetry.SetAttr(span, "review.model", rt.Model)
	var traceID string
	if telemetry.IsEnabled() {
		traceID = telemetry.TraceIDFromContext(ctx)
		if !isMachineReadable(opts.outputFormat) {
			fmt.Fprintf(os.Stderr, "[ocr] TraceID: %s\n", traceID)
		}
	}
	startTime := time.Now()

	comments, runErr := ag.Run(runCtx)
	// Resolve once into the slice shared by every delivery path. emitRunResult
	// also resolves defensively for its other callers, but GitHub posting happens
	// after it returns and must receive the same line and side provenance.
	comments = diff.ResolveLineNumbers(comments, ag.Diffs())
	manifest := ag.RunManifest()

	// Freeze the retry report at the same boundary as the manifest: ag.Run has
	// returned and joined its background work, so every request this run made is
	// finalized and the report can no longer change. run_id is the session's
	// in-memory UUID (ag.Session().SessionID) rather than ag.SessionID(), which
	// returns "" when persistence failed — the report's logical_request_id must
	// stay stable and unique per run even for an unpersisted session.
	retryReport, freezeErr := rt.RetryCollector.Freeze(ag.Session().SessionID)
	if freezeErr != nil {
		// A construction error means the collector's invariants were violated, so
		// the report is self-contradictory and must not be published at all
		// (Freeze already returned nil). Retry reporting is observability only, so
		// its invariant failure must not change the review's exit status.
		fmt.Fprintf(os.Stderr, "[ocr] warning: freeze retry report: %v (retry report suppressed)\n", freezeErr)
	}

	resultErr := reviewResultError(runErr, manifest)
	if resultErr != nil {
		span.SetStatus(codes.Error, resultErr.Error())
		span.RecordError(resultErr)
	}

	// A successfully constructed manifest is publishable even when execution or
	// session delivery failed. Emit it first, then return the independent process
	// error so JSON consumers retain the complete coverage diagnosis.
	var emitErr error
	emitted := manifest != nil || runErr == nil
	if emitted {
		emitErr = emitRunResult(runCtx, ag, comments, startTime, opts.outputFormat, opts.audience, q, llmIdentity, out, retryReport)
		if emitErr != nil {
			emitErr = fmt.Errorf("emit review result: %w", emitErr)
		}
	}

	var postErr error
	if opts.postToPR && resultErr == nil && len(comments) > 0 {
		if err := validateGitHubPostingManifest(sealed, manifest); err != nil {
			postErr = fmt.Errorf("post review to GitHub: %w", err)
			fmt.Fprintf(os.Stderr, "[ocr] ERROR: %v\n", postErr)
		} else if err := postCommentsToGitHub(runCtx, cc.RepoDir, opts, comments, getGitHubToken(opts.githubToken)); err != nil {
			postErr = fmt.Errorf("post review to GitHub: %w", err)
			fmt.Fprintf(os.Stderr, "[ocr] ERROR: %v\n", postErr)
		}
	}
	if resultErr != nil {
		q.Restore()
		// The report has exactly one exit per run. emitRunResult already published
		// it whenever it ran (which it does even for a fully failed run, since a
		// failed manifest is still publishable), so the failure-usage path gets it
		// only when that call was skipped entirely.
		failureReport := retryReport
		if emitted {
			failureReport = nil
		}
		emitFailureUsage(ag, time.Since(startTime), opts.outputFormat, llmIdentity, failureReport)
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(os.Stderr, "[ocr] Session: %s (retry with: --resume %s)\n", id, id)
		}
		return errors.Join(resultErr, emitErr, postErr)
	}
	return errors.Join(emitErr, postErr)
}

func reviewResultError(runErr error, manifest *session.RunManifest) error {
	if runErr != nil {
		return fmt.Errorf("review failed: %w", runErr)
	}
	if manifest != nil && manifest.TerminalState == session.StateFailed {
		// The exit contract is: non-zero only for a run-level failure, or when
		// every selected item failed. Any usable coverage — even incomplete — exits
		// 0, so complete/partial/skipped all succeed and only failed lands here.
		// That makes a budget stop exit 0 whenever anything was covered (it is a
		// controlled truncation recording no run_failure) and non-zero only when
		// the cap left nothing covered at all. Partial results are published
		// regardless: runReview emits the frozen manifest before this error decides
		// the exit status.
		//
		// Reasons stored in the manifest already went through sanitizeReason, so
		// they are safe to echo on stderr.
		if rf := manifest.RunFailure; rf != nil {
			if rf.Reason != "" {
				return fmt.Errorf("review failed (%s): %s", rf.Classification, rf.Reason)
			}
			return fmt.Errorf("review failed (%s)", rf.Classification)
		}
		return fmt.Errorf("review failed: %d of %d selected item(s) failed",
			len(manifest.Coverage.Failed), len(manifest.Coverage.Selected))
	}
	return nil
}

func loadReviewResumeState(repoDir string, opts reviewOptions) (*session.ResumeState, error) {
	if opts.resume == "" {
		return nil, nil
	}
	current := session.SessionOptions{
		ReviewMode: reviewModeFromOptions(opts),
		DiffFrom:   opts.from,
		DiffTo:     opts.to,
		DiffCommit: opts.commit,
	}
	if current.ReviewMode == session.ReviewModeWorkspace {
		return nil, fmt.Errorf("resume requires --from/--to or --commit; workspace resume is not supported")
	}
	state, err := session.LoadReviewResumeState(repoDir, opts.resume)
	if err != nil {
		return nil, fmt.Errorf("load resume session: %w (run 'ocr session list' to see available sessions)", err)
	}
	if err := state.ValidateOptions(current); err != nil {
		return nil, fmt.Errorf("%w (run 'ocr session list' to see available sessions)", err)
	}
	// A parent whose every item failed is deliberately allowed through: it has a
	// verifiable manifest, so its whole selected set can simply be re-dispatched.
	// Whether the checkpoints may be reused at all is decided later, by
	// validateResumeIdentity, once the input identity is known.
	return state, nil
}

// validateResumeIdentity rejects a resume whose input, rules, provider or model
// no longer match the parent run.
//
// It must run before agent.New: agent.New creates the session, and session.New
// writes session_start immediately, so validating any later would leave an orphan
// session on disk behind every rejection. It must also run after max-tokens is
// resolved, because the per-file token ceiling decides which large diffs are
// dropped and therefore which files the input identity covers.
//
// provider and model are explicit exactly when their flag was passed on this
// command line: both default to the empty string and nothing else can set them,
// so a provider that changed via config file or environment stays implicit —
// which is the transition this check exists to reject.
func validateResumeIdentity(ctx context.Context, cc *commonContext, opts reviewOptions, rt *llmRuntime, state *session.ResumeState) (*agent.SealedInput, error) {
	if state == nil && !opts.postToPR {
		return nil, nil
	}
	sealed, err := agent.ResolveIdentity(ctx, agent.Args{
		RepoDir:    cc.RepoDir,
		From:       opts.from,
		To:         opts.to,
		Commit:     opts.commit,
		ReviewMode: reviewModeFromOptions(opts),
		Template:   *cc.Template,
		SystemRule: cc.Resolver,
		FileFilter: cc.FileFilter,
		GitRunner:  cc.GitRunner,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve current input identity: %w", err)
	}
	if state == nil {
		return sealed, nil
	}
	if err := state.ValidateResume(session.ResumeRequest{
		Identity:         sealed.Identity,
		Provider:         rt.Provider,
		Model:            rt.Model,
		ProviderExplicit: opts.provider != "",
		ModelExplicit:    opts.model != "",
	}); err != nil {
		return nil, err
	}
	return sealed, nil
}

func validateGitHubPostingManifest(sealed *agent.SealedInput, manifest *session.RunManifest) error {
	if sealed == nil {
		return fmt.Errorf("sealed review input is missing")
	}
	if manifest == nil {
		return fmt.Errorf("run manifest is missing")
	}
	expected := sealed.Resolution
	actual := manifest.Input
	if actual.Mode != session.InputModeRange ||
		actual.ResolvedBase != expected.ResolvedBase ||
		actual.ResolvedHead != expected.ResolvedHead ||
		actual.ExactRange != expected.ExactRange {
		return fmt.Errorf("run manifest input does not match the sealed range; refusing to post")
	}
	return nil
}

// fileReadRef picks the ref file_read resolves paths against.
//
// A sealed input replaces the ref the user typed with the commit that ref
// resolved to at admission. The diff under review is pinned to that same commit,
// so leaving the reader on a moving ref would let the model read one version of a
// file while reviewing the diff of another. Workspace mode has no ref at all, and
// keeps none: its content is the working tree, which is what the diff describes.
func fileReadRef(mode tool.ReviewMode, opts reviewOptions, sealed *diff.InputResolution) string {
	ref, ok := mode.RefValue(opts.to, opts.commit)
	if !ok {
		return ""
	}
	if sealed != nil && sealed.ResolvedHead != "" {
		return sealed.ResolvedHead
	}
	return ref
}

func reviewModeFromOptions(opts reviewOptions) string {
	if opts.commit != "" {
		return session.ReviewModeCommit
	}
	if opts.from != "" && opts.to != "" {
		return session.ReviewModeRange
	}
	return session.ReviewModeWorkspace
}

// resolveRepoDir resolves the repo dir for `ocr rules check`. It delegates to
// resolveWorkingDir(requireGit=true) so it anchors at the git top-level just
// like the review path — keeping rule resolution consistent when run from a
// monorepo subdirectory (#287).
func resolveRepoDir(input string) (string, error) {
	absPath, _, err := resolveWorkingDir(input, true)
	return absPath, err
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

// validateReviewRefs rejects ref-option injection (#112): any --from/--to/
// --commit value must be a real commit ref and must not start with '-'.
func validateReviewRefs(repoDir string, opts reviewOptions) error {
	refs := []struct {
		flag string
		ref  string
	}{
		{"--from", opts.from},
		{"--to", opts.to},
		{"--commit", opts.commit},
	}
	for _, item := range refs {
		if item.ref == "" {
			continue
		}
		if strings.HasPrefix(item.ref, "-") {
			return fmt.Errorf("%s value %q is not a valid git ref: refs must not start with '-'", item.flag, item.ref)
		}
		if out, err := runGitCmd(repoDir, "rev-parse", "--verify", "--end-of-options", item.ref+"^{commit}"); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("%s value %q is not a valid commit ref: %s", item.flag, item.ref, msg)
			}
			return fmt.Errorf("%s value %q is not a valid commit ref", item.flag, item.ref)
		}
	}
	return nil
}

func runPreviewContext(ctx context.Context, cc *commonContext, opts reviewOptions, out io.Writer) error {
	preview, err := agent.Preview(ctx, agent.Args{
		RepoDir:    cc.RepoDir,
		From:       opts.from,
		To:         opts.to,
		Commit:     opts.commit,
		FileFilter: cc.FileFilter,
		GitRunner:  cc.GitRunner,
	})
	if err != nil {
		return fmt.Errorf("preview failed: %w", err)
	}

	return outputPreview(preview, opts.outputFormat, out)
}

func initMCPClients(ctx context.Context, cfg *Config, tools *tool.Registry, repoDir, version string) []*mcp.Client {
	if cfg == nil || len(cfg.MCPServers) == 0 {
		return nil
	}

	mcpNames := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)

	var clients []*mcp.Client
	for _, name := range mcpNames {
		serverCfg := cfg.MCPServers[name]

		isRemote := serverCfg.Type == "remote"

		if isRemote {
			if serverCfg.URL == "" {
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: remote MCP server %q has no URL configured, skipping\n", name)
				continue
			}
			initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
			mc, err := mcp.NewRemoteClient(initCtx, name, serverCfg.URL, serverCfg.Headers, version)
			initCancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to connect to remote MCP server %q: %v\n", name, err)
				continue
			}
			clients = append(clients, mc)
			mcp.RegisterAll(tools, mc, serverCfg.Tools)
			continue
		}

		if serverCfg.Command == "" {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q has no command configured, skipping\n", name)
			continue
		}
		if serverCfg.Setup != "" {
			fmt.Fprintf(os.Stderr, "[ocr] Running setup for MCP server %q: %s\n", name, serverCfg.Setup)
			setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
			setupCmd := shellCommand(setupCtx, serverCfg.Setup)
			setupCmd.Dir = repoDir
			configureProcessGroup(setupCmd)
			output, err := setupCmd.CombinedOutput()
			setupCancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ocr] ERROR: MCP server %q setup command failed.\n", name)
				fmt.Fprintf(os.Stderr, "[ocr]   Command: %s\n", serverCfg.Setup)
				fmt.Fprintf(os.Stderr, "[ocr]   Working directory: %s\n", repoDir)
				fmt.Fprintf(os.Stderr, "[ocr]   Error: %v\n", err)
				if len(output) > 0 {
					fmt.Fprintf(os.Stderr, "[ocr]   Output:\n%s\n", string(output))
				}
				fmt.Fprintf(os.Stderr, "[ocr]   Skipping MCP server %q — review will proceed without it.\n", name)
				continue
			}
		}

		initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
		mc, err := mcp.NewClient(initCtx, name, serverCfg.Command, serverCfg.Args, serverCfg.Env, repoDir, version)
		initCancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to start MCP server %q: %v\n", name, err)
			continue
		}
		clients = append(clients, mc)
		mcp.RegisterAll(tools, mc, serverCfg.Tools)
	}
	return clients
}

func buildToolRegistry(collector *tool.CommentCollector, fr *tool.FileReader) *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Register(tool.NewFileFind(fr))
	reg.Register(tool.NewFileReadDiff(tool.DiffMap{}))
	reg.Register(tool.NewCodeSearch(fr))
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	return reg
}

const githubReviewBatchSize = 50

type githubRepository struct {
	remote string
	owner  string
	name   string
}

type githubPostingTarget struct {
	githubRepository
	prNumber int
}

type githubSummaryFinding struct {
	comment model.LlmComment
	reason  string
}

// postCommentsToGitHub posts review comments to the unique PR whose base and
// head SHA match the immutable range reviewed by this run.
func postCommentsToGitHub(ctx context.Context, repoDir string, opts reviewOptions, comments []model.LlmComment, ghToken string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reviewedHead := opts.reviewedHeadSHA
	if reviewedHead == "" {
		resolved, err := runGitCmdStdout(repoDir, "rev-parse", "--verify", "--end-of-options", opts.to+"^{commit}")
		if err != nil {
			return fmt.Errorf("resolve reviewed head %q: %w", opts.to, err)
		}
		reviewedHead = strings.TrimSpace(string(resolved))
	}
	if reviewedHead == "" {
		return fmt.Errorf("reviewed head SHA is empty")
	}

	repositories, remoteNames, err := getGitHubRepositories(repoDir)
	if err != nil {
		return err
	}
	baseBranch := normalizeGitHubBranch(opts.from, remoteNames)
	if baseBranch == "" {
		return fmt.Errorf("cannot derive GitHub base branch from %q", opts.from)
	}

	var target *githubPostingTarget
	for _, repository := range repositories {
		prNumber, findErr := github.FindPRByHead(ctx, ghToken, repository.owner, repository.name, baseBranch, reviewedHead)
		if errors.Is(findErr, github.ErrNoMatchingPullRequest) {
			continue
		}
		if findErr != nil {
			return fmt.Errorf("discover PR in %s/%s: %w", repository.owner, repository.name, findErr)
		}
		if target != nil {
			return fmt.Errorf(
				"multiple configured GitHub remotes have open PRs for base=%s and reviewed commit %s",
				baseBranch,
				reviewedHead,
			)
		}
		target = &githubPostingTarget{githubRepository: repository, prNumber: prNumber}
	}
	if target == nil {
		return fmt.Errorf(
			"no open PR found for base=%s and reviewed commit %s in configured GitHub remotes",
			baseBranch,
			reviewedHead,
		)
	}

	ghClient := github.NewClient(ghToken, target.owner, target.name, target.prNumber)
	if err := verifyGitHubPRHead(ctx, ghClient, reviewedHead); err != nil {
		return err
	}
	changedFiles, err := ghClient.ListChangedFiles(ctx)
	if err != nil {
		return fmt.Errorf("read PR diff: %w", err)
	}
	if err := verifyGitHubPRHead(ctx, ghClient, reviewedHead); err != nil {
		return err
	}
	inventory := buildGitHubDiffInventory(changedFiles)

	ghComments := make([]github.Comment, 0, len(comments))
	inlineSources := make([]model.LlmComment, 0, len(comments))
	summaryFindings := make([]githubSummaryFinding, 0)
	for _, comment := range comments {
		startLine, endLine, hasLocation := githubCommentLocation(comment)
		switch {
		case comment.Side != model.CommentSideRight:
			reason := "line side provenance is not RIGHT"
			if comment.Side == model.CommentSideLeft {
				reason = "resolved on the old side of the diff"
			}
			summaryFindings = append(summaryFindings, githubSummaryFinding{comment: comment, reason: reason})
		case !hasLocation:
			summaryFindings = append(summaryFindings, githubSummaryFinding{comment: comment, reason: "no valid line information"})
		case !inventory.contains(comment.Path, startLine, endLine):
			summaryFindings = append(summaryFindings, githubSummaryFinding{comment: comment, reason: "line range could not be verified in a complete right-side PR diff hunk"})
		default:
			ghComment := github.Comment{Path: comment.Path, Body: formatCommentBody(comment), Line: endLine, Side: model.CommentSideRight}
			if startLine != endLine {
				ghComment.StartLine = startLine
				ghComment.StartSide = model.CommentSideRight
			}
			ghComments = append(ghComments, ghComment)
			inlineSources = append(inlineSources, comment)
		}
	}

	summaryBody := buildSummaryBody(comments, len(ghComments), summaryFindings)
	if len(ghComments) == 0 {
		if err := verifyGitHubPRHead(ctx, ghClient, reviewedHead); err != nil {
			return err
		}
		commentURL, err := ghClient.CreateIssueComment(ctx, summaryBody)
		if err != nil {
			return fmt.Errorf("post review summary: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[ocr] No findings had a verified inline location; summary posted: %s\n", commentURL)
		return nil
	}

	reviewRunID, err := newGitHubReviewRunID()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[ocr] Posting %d verified inline comments to PR #%d in batches of at most %d\n", len(ghComments), target.prNumber, githubReviewBatchSize)
	for start := 0; start < len(ghComments); start += githubReviewBatchSize {
		end := min(start+githubReviewBatchSize, len(ghComments))
		if err := verifyGitHubPRHead(ctx, ghClient, reviewedHead); err != nil {
			return err
		}
		body := "OpenCodeReview inline findings (continued)."
		if start == 0 {
			body = summaryBody
		}
		marker := fmt.Sprintf("<!-- ocr-review-%s-batch-%d -->", reviewRunID, start/githubReviewBatchSize)
		body = marker + "\n" + body
		reviewResp, createErr := ghClient.CreateReview(ctx, reviewedHead, github.ReviewRequest{
			Body:     body,
			Event:    "COMMENT",
			Comments: ghComments[start:end],
		})
		if createErr != nil {
			if githubReviewWriteIsAmbiguous(createErr) {
				landed, reconcileErr := githubReviewExists(ctx, ghClient, reviewedHead, marker)
				if reconcileErr != nil {
					return errors.Join(
						fmt.Errorf("create inline review: %w", createErr),
						fmt.Errorf("reconcile ambiguous inline review write: %w", reconcileErr),
					)
				}
				if landed {
					fmt.Fprintln(os.Stderr, "[ocr] Review batch was accepted by GitHub despite an ambiguous response")
					continue
				}
			}

			fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create inline review batch, posting remaining findings as a summary instead: %v\n", createErr)
			fallbackComments := make([]model.LlmComment, 0, len(inlineSources)-start+len(summaryFindings))
			fallback := make([]githubSummaryFinding, 0, len(inlineSources)-start+len(summaryFindings))
			if start == 0 {
				for _, finding := range summaryFindings {
					fallbackComments = append(fallbackComments, finding.comment)
					fallback = append(fallback, finding)
				}
			}
			for _, comment := range inlineSources[start:] {
				fallbackComments = append(fallbackComments, comment)
				fallback = append(fallback, githubSummaryFinding{comment: comment, reason: "inline review could not be created"})
			}
			if err := verifyGitHubPRHead(ctx, ghClient, reviewedHead); err != nil {
				return errors.Join(fmt.Errorf("create inline review: %w", createErr), err)
			}
			commentURL, summaryErr := ghClient.CreateIssueComment(ctx, buildSummaryBody(fallbackComments, 0, fallback))
			if summaryErr != nil {
				return errors.Join(fmt.Errorf("create inline review: %w", createErr), fmt.Errorf("post fallback summary: %w", summaryErr))
			}
			fmt.Fprintf(os.Stderr, "[ocr] Summary comment posted: %s\n", commentURL)
			return nil
		}
		fmt.Fprintf(os.Stderr, "[ocr] Review batch posted successfully: %s\n", reviewResp.HTMLURL)
	}
	return nil
}

func newGitHubReviewRunID() (string, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate GitHub review reconciliation marker: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

func githubReviewWriteIsAmbiguous(err error) bool {
	var apiErr *github.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusRequestTimeout || apiErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

func githubReviewExists(ctx context.Context, client *github.Client, commitSHA, marker string) (bool, error) {
	reviews, err := client.ListReviews(ctx)
	if err != nil {
		return false, err
	}
	for _, review := range reviews {
		if review.CommitID == commitSHA && strings.Contains(review.Body, marker) {
			return true, nil
		}
	}
	return false, nil
}

func getGitHubRepositories(repoDir string) ([]githubRepository, []string, error) {
	output, err := runGitCmdStdout(repoDir, "remote")
	if err != nil {
		return nil, nil, fmt.Errorf("list git remotes: %w", err)
	}
	remoteNames := strings.Fields(string(output))
	if len(remoteNames) == 0 {
		return nil, nil, fmt.Errorf("repository has no Git remotes")
	}

	repositories := make([]githubRepository, 0, len(remoteNames))
	seen := make(map[string]struct{})
	for _, remote := range remoteNames {
		remoteURL, remoteErr := runGitCmdStdout(repoDir, "remote", "get-url", remote)
		if remoteErr != nil {
			return nil, nil, fmt.Errorf("read URL for git remote %q: %w", remote, remoteErr)
		}
		owner, name, parseErr := github.ParseRepoInfo(strings.TrimSpace(string(remoteURL)))
		if parseErr != nil {
			continue
		}
		key := strings.ToLower(owner + "/" + name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		repositories = append(repositories, githubRepository{remote: remote, owner: owner, name: name})
	}
	if len(repositories) == 0 {
		return nil, nil, fmt.Errorf("no parseable GitHub repository found in configured remotes")
	}
	return repositories, remoteNames, nil
}

func normalizeGitHubBranch(ref string, remoteNames []string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	for _, remote := range remoteNames {
		if strings.HasPrefix(ref, remote+"/") {
			return strings.TrimPrefix(ref, remote+"/")
		}
	}
	return ref
}

func verifyGitHubPRHead(ctx context.Context, client *github.Client, reviewedHead string) error {
	pull, err := client.GetPRInfo(ctx)
	if err != nil {
		return fmt.Errorf("read PR head: %w", err)
	}
	if pull.Head.SHA != reviewedHead {
		return fmt.Errorf("PR head changed from reviewed commit %s to %s; rerun the review before posting", reviewedHead, pull.Head.SHA)
	}
	return nil
}

func githubCommentLocation(comment model.LlmComment) (startLine, endLine int, ok bool) {
	endLine = comment.EndLine
	if endLine <= 0 {
		endLine = comment.StartLine
	}
	if endLine <= 0 {
		return 0, 0, false
	}
	startLine = endLine
	if comment.StartLine > 0 {
		startLine = comment.StartLine
	}
	if startLine > endLine {
		return 0, 0, false
	}
	return startLine, endLine, true
}

// formatCommentBody formats a finding as a structured GitHub review comment.
func formatCommentBody(comment model.LlmComment) string {
	var body strings.Builder
	if badge := buildBadge(comment); badge != "" {
		body.WriteString("**")
		body.WriteString(badge)
		body.WriteString("**\n\n")
	}
	body.WriteString(comment.Content)
	if comment.SuggestionCode != "" {
		body.WriteString("\n\n**Suggestion:**\n```suggestion\n")
		body.WriteString(comment.SuggestionCode)
		if !strings.HasSuffix(comment.SuggestionCode, "\n") {
			body.WriteByte('\n')
		}
		body.WriteString("```")
	}

	return body.String()
}

// buildSummaryBody builds the review body. Inline findings are represented by
// their attached comments; only findings that could not be anchored are
// expanded here, which avoids duplicating every finding in the summary.
func buildSummaryBody(comments []model.LlmComment, inlineCount int, summaryFindings []githubSummaryFinding) string {
	var body strings.Builder

	body.WriteString("## OpenCodeReview Summary\n\n")
	body.WriteString(fmt.Sprintf("%d comment(s) generated.\n\n", len(comments)))
	body.WriteString(fmt.Sprintf("- Inline findings: %d\n", inlineCount))
	body.WriteString(fmt.Sprintf("- Summary-only findings: %d\n\n", len(summaryFindings)))

	if len(summaryFindings) > 0 {
		body.WriteString("### Findings without a verified inline location\n\n")
		for _, finding := range summaryFindings {
			location := finding.comment.Path
			if startLine, endLine, ok := githubCommentLocation(finding.comment); ok {
				if startLine == endLine {
					location = fmt.Sprintf("%s (line %d)", location, endLine)
				} else {
					location = fmt.Sprintf("%s (lines %d-%d)", location, startLine, endLine)
				}
			}
			body.WriteString(fmt.Sprintf("#### `%s`\n\n", location))
			body.WriteString(fmt.Sprintf("_Summary reason: %s._\n\n", finding.reason))
			body.WriteString(formatCommentBody(finding.comment))
			body.WriteString("\n\n---\n\n")
		}
	}

	severityCount := make(map[string]int)
	for _, comment := range comments {
		if comment.Severity != "" {
			severityCount[comment.Severity]++
		}
	}

	if len(severityCount) > 0 {
		body.WriteString("### Severity breakdown:\n\n")
		severities := make([]string, 0, len(severityCount))
		for severity := range severityCount {
			severities = append(severities, severity)
		}
		sort.Strings(severities)
		for _, severity := range severities {
			count := severityCount[severity]
			body.WriteString(fmt.Sprintf("- **%s**: %d\n", severity, count))
		}
		body.WriteString("\n")
	}

	body.WriteString("---\n")
	body.WriteString("*Generated by OpenCodeReview*")

	return body.String()
}
