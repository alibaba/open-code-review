// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"

	"go.opentelemetry.io/otel/codes"
)

type reviewOptions struct {
	toolConfigPath     string
	rulePath           string
	repoDir            string
	from               string
	to                 string
	commit             string
	resume             string
	excludes           string
	outputFormat       string
	audience           string
	background         string
	backgroundFile     string
	provider           string
	model              string
	concurrency        int
	perFileTimeout     int
	perFileTimeoutSet  bool
	maxTools           int
	maxGitProcs        int
	maxTokens          int
	maxTokensBudget    int
	maxTokensBudgetSet bool
	preview            bool
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

  # Provide requirement/business context inline, from a Markdown file, or both
  ocr review --background "Adding rate limiting to the login API"
  ocr review --background-file ./docs/requirements.md
  ocr review --background "Focus on auth" --background-file ./docs/requirements.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := reviewOpts
		opts.maxTokensBudgetSet = cmd.Flags().Changed("max-tokens-budget")
		opts.perFileTimeoutSet = cmd.Flags().Changed("timeout")
		if err := validateReviewOptions(&opts); err != nil {
			return err
		}
		return executeReview(opts)
	},
}

func init() {
	registerReviewFlags(reviewCmd, &reviewOpts)
}

func executeReview(opts reviewOptions) error {
	return executeReviewContext(context.Background(), opts, os.Stdout, os.Stderr, nil, nil)
}

type reviewStageFunc func(stage, path string)

func executeReviewContext(ctx context.Context, opts reviewOptions, outputWriter, diagnosticWriter io.Writer, progress llmloop.ProgressFunc, watchdog *reviewWatchdog) error {
	return executeReviewContextWithStage(ctx, opts, outputWriter, diagnosticWriter, progress, watchdog, nil)
}

func executeReviewContextWithStage(ctx context.Context, opts reviewOptions, outputWriter, diagnosticWriter io.Writer, progress llmloop.ProgressFunc, watchdog *reviewWatchdog, stage reviewStageFunc) error {
	if outputWriter == nil {
		outputWriter = os.Stdout
	}
	if diagnosticWriter == nil {
		diagnosticWriter = os.Stderr
	}
	setReviewStage(stage, "load_common_context", "")
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, opts.maxTools, opts.maxGitProcs, true)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// Security (#112): reject ref-option injection before any git invocation.
	setReviewStage(stage, "validate_refs", "")
	if err := validateReviewRefs(cc.RepoDir, opts); err != nil {
		return err
	}

	setReviewStage(stage, "resolve_background", "")
	if opts.commit != "" && opts.background == "" {
		if msg, err := getCommitMessage(cc.RepoDir, opts.commit); err == nil && msg != "" {
			opts.background = msg
		}
	}

	// Only touch the background when --background-file is set, so the existing
	// --background behaviour (raw, unsanitised) is preserved for users who do
	// not opt into the file-based context.
	if opts.backgroundFile != "" {
		// Resolve relative paths against the git top-level (cc.RepoDir), matching
		// file_read semantics, so `-B ./docs/context.md` works from any directory.
		bgPath := resolveBackgroundFilePath(cc.RepoDir, opts.backgroundFile)
		fileBackground, err := loadBackgroundFile(bgPath)
		if err != nil {
			return err
		}
		opts.background = mergeBackground(opts.background, fileBackground)
	}

	if opts.preview {
		return runPreview(cc, opts)
	}

	setReviewStage(stage, "load_resume_state", "")
	resumeState, err := loadReviewResumeState(cc.RepoDir, opts)
	if err != nil {
		return err
	}
	setReviewStage(stage, "load_llm_runtime", "")
	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, llm.ResolveOptions{
		Provider: opts.provider,
		Model:    opts.model,
	})
	if err != nil {
		return err
	}
	cc.Template.MaxCompletionTokens = cc.Template.OutputTokens()
	cc.Template.MaxTokens, err = resolveMaxTokens(cc.Template.MaxTokens, rt.AppCfg, opts.maxTokens)
	if err != nil {
		return err
	}
	opts.perFileTimeout = applyModelPerFileTimeout(rt.Model, opts.perFileTimeout, opts.perFileTimeoutSet)
	cc.Template.MaxToolRequestTimes = applyModelMaxToolRequestTimes(rt.Model, cc.Template.MaxToolRequestTimes)
	if watchdog != nil {
		rt.Client = watchdogLLMClient{inner: rt.Client, watchdog: watchdog}
	}
	llmIdentity := &jsonLLMIdentity{
		Provider: rt.Provider,
		Model:    rt.Model,
	}

	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit)
	ref, _ := mode.RefValue(opts.to, opts.commit)
	fileReader := &tool.FileReader{
		RepoDir: cc.RepoDir,
		Mode:    mode,
		Ref:     ref,
		Runner:  cc.GitRunner,
	}
	tools := buildToolRegistry(rt.Collector, fileReader)

	setReviewStage(stage, "init_mcp_clients", "")
	mcpClients := initMCPClientsTo(ctx, rt.AppCfg, tools, cc.RepoDir, Version, diagnosticWriter)
	defer func() {
		for _, mc := range mcpClients {
			if err := mc.Close(); err != nil {
				fmt.Fprintf(diagnosticWriter, "[ocr] WARNING: failed to close MCP server %q: %v\n", mc.Name(), err)
			}
		}
	}()

	mcpToolDefs := mcp.CollectToolDefs(mcpClients, tools)
	rt.PlanToolDefs = append(rt.PlanToolDefs, mcpToolDefs...)
	rt.MainToolDefs = append(rt.MainToolDefs, mcpToolDefs...)
	mcpInstructions := codebaseMemoryInstructions(mcpClients)

	ag := agent.New(agent.Args{
		RepoDir:                 cc.RepoDir,
		From:                    opts.from,
		To:                      opts.to,
		Commit:                  opts.commit,
		ReviewMode:              reviewModeFromOptions(opts),
		Template:                *cc.Template,
		SystemRule:              cc.Resolver,
		FileFilter:              cc.FileFilter,
		LLMClient:               rt.Client,
		Tools:                   tools,
		PlanToolDefs:            rt.PlanToolDefs,
		MainToolDefs:            rt.MainToolDefs,
		CommentCollector:        rt.Collector,
		CommentWorkerPool:       agent.NewCommentWorkerPool(opts.concurrency),
		MaxConcurrency:          opts.concurrency,
		ConcurrentTaskTimeout:   opts.perFileTimeout,
		Model:                   rt.Model,
		Provider:                rt.Provider,
		Background:              opts.background,
		MCPInstructions:         mcpInstructions,
		GitRunner:               cc.GitRunner,
		Resume:                  resumeState,
		MaxTokensBudget:         int64(opts.maxTokensBudget),
		MaxTokensBudgetExplicit: opts.maxTokensBudgetSet,
		RuntimeConfig:           rt.RuntimeConfig,
		Progress:                progress,
	})

	// Silence progress output during execution; restored before the trace
	// summary in agent-text mode (and on function exit otherwise).
	q := newQuietHandle(opts.outputFormat, opts.audience)
	defer q.Restore()

	ctx, span := telemetry.StartSpan(telemetry.ContextWithTraceParentFromEnv(ctx), "review.run")
	defer span.End()
	telemetry.SetAttr(span, "review.repo", cc.RepoDir)
	telemetry.SetAttr(span, "review.from", opts.from)
	telemetry.SetAttr(span, "review.to", opts.to)
	telemetry.SetAttr(span, "review.model", rt.Model)
	var traceID string
	if telemetry.IsEnabled() {
		traceID = telemetry.TraceIDFromContext(ctx)
		if opts.outputFormat != "json" {
			fmt.Fprintf(diagnosticWriter, "[ocr] TraceID: %s\n", traceID)
		}
	}
	startTime := time.Now()

	setReviewStage(stage, "agent_run", "")
	comments, runErr := ag.Run(ctx)
	manifest := ag.RunManifest()
	resultErr := reviewResultError(runErr, manifest)
	if resultErr != nil {
		span.SetStatus(codes.Error, resultErr.Error())
		span.RecordError(resultErr)
	}

	// A successfully constructed manifest is publishable even when execution or
	// session delivery failed. Emit it first, then return the independent process
	// error so JSON consumers retain the complete coverage diagnosis.
	var emitErr error
	if manifest != nil || runErr == nil {
		setReviewStage(stage, "emit_result", "")
		emitErr = emitRunResultTo(ctx, ag, comments, startTime, opts.outputFormat, opts.audience, q, llmIdentity, outputWriter)
		if emitErr != nil {
			emitErr = fmt.Errorf("emit review result: %w", emitErr)
		}
	}
	if resultErr != nil {
		q.Restore()
		emitFailureUsageTo(diagnosticWriter, ag, time.Since(startTime), opts.outputFormat, llmIdentity)
		if id := ag.SessionID(); id != "" {
			fmt.Fprintf(diagnosticWriter, "[ocr] Session: %s (retry with: --resume %s)\n", id, id)
		}
		return errors.Join(resultErr, emitErr)
	}
	return emitErr
}

func setReviewStage(stage reviewStageFunc, name, path string) {
	if stage != nil {
		stage(name, path)
	}
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
	state, err := session.LoadResumeState(repoDir, opts.resume)
	if err != nil {
		return nil, fmt.Errorf("load resume session: %w (run 'ocr session list' to see available sessions)", err)
	}
	if err := state.ValidateOptions(current); err != nil {
		return nil, fmt.Errorf("%w (run 'ocr session list' to see available sessions)", err)
	}
	if state.CompletedCount() == 0 {
		return nil, fmt.Errorf("resume session %q has no completed review items (run 'ocr session list' to see available sessions)", opts.resume)
	}
	return state, nil
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

func runPreview(cc *commonContext, opts reviewOptions) error {
	preview, err := agent.Preview(context.Background(), agent.Args{
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

	return outputPreview(preview, opts.outputFormat)
}

func initMCPClients(ctx context.Context, cfg *Config, tools *tool.Registry, repoDir, version string) []*mcp.Client {
	return initMCPClientsTo(ctx, cfg, tools, repoDir, version, os.Stderr)
}

const codebaseMemoryInstructionsText = `## Structural and relationship queries
When you need callers, callees, dependencies, import graphs, call chains, or change-impact analysis:
1. Prefer the connected codebase-memory MCP tools.
2. Call index_status first.
3. If the index is missing or stale, call index_repository once, then retry.
4. Choose the relevant tools: search_graph, trace_path, get_code_snippet, query_graph, get_architecture, or detect_changes.
5. If structural tools are unavailable, use language-server, build-tool, or text-search approximations and state the limitation.

All connected codebase-memory tools are available; call only the tools needed for the current question. Use built-in code_search for literal review searches and codebase-memory search_code for graph-augmented code search; they are complementary.
Use MCP tools for context only. Keep findings anchored to the current diff, and do not invoke destructive or maintenance operations unless the user explicitly asks.`

func codebaseMemoryInstructions(clients []*mcp.Client) string {
	for _, client := range clients {
		toolNames := make([]string, 0, len(client.Tools()))
		for _, tool := range client.Tools() {
			toolNames = append(toolNames, tool.Name)
		}
		if isCodebaseMemoryServer(client.Name(), toolNames) {
			return codebaseMemoryInstructionsText
		}
	}
	return ""
}

func isCodebaseMemoryServer(serverName string, toolNames []string) bool {
	serverName = strings.ToLower(serverName)
	if strings.Contains(serverName, "codebase-memory") || strings.Contains(serverName, "codebase_memory") {
		return true
	}
	hasIndexStatus := false
	hasSearchGraph := false
	for _, toolName := range toolNames {
		switch toolName {
		case "index_status":
			hasIndexStatus = true
		case "search_graph":
			hasSearchGraph = true
		}
	}
	return hasIndexStatus && hasSearchGraph
}

func initMCPClientsTo(ctx context.Context, cfg *Config, tools *tool.Registry, repoDir, version string, diagnosticWriter io.Writer) []*mcp.Client {
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
				fmt.Fprintf(diagnosticWriter, "[ocr] WARNING: remote MCP server %q has no URL configured, skipping\n", name)
				continue
			}
			initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
			mc, err := mcp.NewRemoteClient(initCtx, name, serverCfg.URL, serverCfg.Headers, version)
			initCancel()
			if err != nil {
				fmt.Fprintf(diagnosticWriter, "[ocr] WARNING: failed to connect to remote MCP server %q: %v\n", name, err)
				continue
			}
			clients = append(clients, mc)
			mcp.RegisterAll(tools, mc, serverCfg.Tools)
			continue
		}

		if serverCfg.Command == "" {
			fmt.Fprintf(diagnosticWriter, "[ocr] WARNING: MCP server %q has no command configured, skipping\n", name)
			continue
		}
		if serverCfg.Setup != "" {
			fmt.Fprintf(diagnosticWriter, "[ocr] Running setup for MCP server %q: %s\n", name, serverCfg.Setup)
			setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
			setupCmd := shellCommand(setupCtx, serverCfg.Setup)
			setupCmd.Dir = repoDir
			configureProcessGroup(setupCmd)
			output, err := setupCmd.CombinedOutput()
			setupCancel()
			if err != nil {
				fmt.Fprintf(diagnosticWriter, "[ocr] ERROR: MCP server %q setup command failed.\n", name)
				fmt.Fprintf(diagnosticWriter, "[ocr]   Command: %s\n", serverCfg.Setup)
				fmt.Fprintf(diagnosticWriter, "[ocr]   Working directory: %s\n", repoDir)
				fmt.Fprintf(diagnosticWriter, "[ocr]   Error: %v\n", err)
				if len(output) > 0 {
					fmt.Fprintf(diagnosticWriter, "[ocr]   Output:\n%s\n", string(output))
				}
				fmt.Fprintf(diagnosticWriter, "[ocr]   Skipping MCP server %q — review will proceed without it.\n", name)
				continue
			}
		}

		initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
		mc, err := mcp.NewClient(initCtx, name, serverCfg.Command, serverCfg.Args, serverCfg.Env, repoDir, version)
		initCancel()
		if err != nil {
			fmt.Fprintf(diagnosticWriter, "[ocr] WARNING: failed to start MCP server %q: %v\n", name, err)
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
