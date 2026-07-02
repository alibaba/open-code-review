package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/learn"
	"github.com/open-code-review/open-code-review/internal/mcp"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
)

func runReview(args []string) error {
	opts, err := parseReviewFlags(args)
	if err != nil {
		// parseReviewFlags already wraps with "parse flags: %w" — return as-is.
		return err
	}
	if opts.showHelp {
		printReviewUsage()
		return nil
	}

	// review path: git repo is required (diff concepts depend on it).
	cc, err := loadCommonContext(opts.repoDir, opts.rulePath, opts.maxTools, opts.maxGitProcs, true)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(opts.excludes))

	// Security (#112): reject ref-option injection before any git invocation.
	if err := validateReviewRefs(cc.RepoDir, opts); err != nil {
		return err
	}

	if opts.commit != "" && opts.background == "" {
		if msg, err := getCommitMessage(cc.RepoDir, opts.commit); err == nil && msg != "" {
			opts.background = msg
		}
	}

	if opts.preview {
		return runPreview(cc, opts)
	}

	rt, err := loadLLMRuntime(cc.Template, opts.toolConfigPath, opts.model, cc.RepoDir)
	if err != nil {
		return err
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

	mcpClients := initMCPClients(context.Background(), rt.AppCfg, tools, cc.RepoDir, Version)
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

	suppressor := setupLearnings(context.Background(), cc.RepoDir, rt.Endpoint.Token, cc.GitRunner)

	ag := agent.New(agent.Args{
		RepoDir:               cc.RepoDir,
		From:                  opts.from,
		To:                    opts.to,
		Commit:                opts.commit,
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
		Background:            opts.background,
		GitRunner:             cc.GitRunner,
	})

	// Silence progress output during execution; restored before the trace
	// summary in agent-text mode (and on function exit otherwise).
	q := newQuietHandle(opts.outputFormat, opts.audience)
	defer q.Restore()

	ctx, span := telemetry.StartSpan(context.Background(), "review.run")
	defer span.End()
	startTime := time.Now()

	comments, err := ag.Run(ctx)
	if err != nil {
		telemetry.SetAttr(span, "error", err.Error())
		return fmt.Errorf("review failed: %w", err)
	}

	// Suppress low-severity / low-confidence comments to improve signal-to-noise.
	// The drop count is reported (never silently truncated); tune or disable via
	// OCR_MIN_SEVERITY / OCR_MIN_CONFIDENCE / OCR_DISABLE_SEVERITY_FILTER.
	if cf := loadCommentFilter(); cf.enabled {
		var dropped int
		comments, dropped = cf.apply(comments)
		if dropped > 0 {
			fmt.Fprintf(os.Stderr, "[ocr] severity filter dropped %d comment(s) below min-severity=%s / min-confidence=%.2f (set OCR_DISABLE_SEVERITY_FILTER=1 to disable)\n",
				dropped, cf.minSeverityLabel, cf.minConfidence)
		}
	}

	// Suppress comments that repeat a previously human-rejected finding (the
	// multi-round re-flag problem). No-op unless cross-PR learnings are
	// configured and the store holds rejected verdicts. Never silently
	// truncated; set OCR_REFLAG_SUPPRESS=off to disable.
	if suppressor.enabled {
		var reflagged int
		comments, reflagged = suppressor.apply(ctx, comments)
		if reflagged > 0 {
			fmt.Fprintf(os.Stderr, "[ocr] reflag suppressor dropped %d comment(s) matching prior rejected findings (cosine>=%.2f; set OCR_REFLAG_SUPPRESS=off to disable)\n",
				reflagged, suppressor.threshold)
		}
	}

	return emitRunResult(ctx, ag, comments, startTime, opts.outputFormat, opts.audience, q)
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
	ag := agent.New(agent.Args{
		RepoDir:    cc.RepoDir,
		From:       opts.from,
		To:         opts.to,
		Commit:     opts.commit,
		FileFilter: cc.FileFilter,
		GitRunner:  cc.GitRunner,
	})

	preview, err := ag.Preview(context.Background())
	if err != nil {
		return fmt.Errorf("preview failed: %w", err)
	}

	outputPreviewText(preview)
	return nil
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

// setupLearnings ingests PR feedback (if configured) into the local store and
// returns a re-flag suppressor backed by that same store + embedder. Best-effort:
// every failure path warns and returns a disabled suppressor (zero value) so the
// review proceeds unaffected.
func setupLearnings(ctx context.Context, repoDir, token string, gitRunner *gitcmd.Runner) reflagSuppressor {
	cfg := learn.LoadConfig()
	if !cfg.Enabled {
		return reflagSuppressor{} // disabled
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "[ocr] learnings: no LLM token; skipping")
		return reflagSuppressor{}
	}
	remote, _ := gitRunner.Run(ctx, repoDir, "remote", "get-url", "origin")
	remote = strings.TrimSpace(remote)
	if remote == "" {
		remote = repoDir // fall back to repo path as the store key
	}
	storePath, err := learn.RepoStorePath(remote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] learnings: store path: %v (skipped)\n", err)
		return reflagSuppressor{}
	}
	store, err := learn.OpenStore(storePath, learn.DefaultSoftCap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] learnings: open store: %v (skipped)\n", err)
		return reflagSuppressor{}
	}
	emb := learn.NewBigModelEmbedder(cfg.EmbedURL, token, cfg.EmbedModel)

	// Ingestion only runs when the workflow supplied a feedback file; absent
	// one, we still build a suppressor from whatever the store already holds.
	if cfg.FeedbackPath != "" {
		added, err := learn.Ingest(ctx, store, emb, cfg.FeedbackPath, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] learnings: ingest: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[ocr] learnings: ingested %d new feedback item(s); store now has %d\n", added, store.Len())
		}
	}
	return newReflagSuppressor(true, emb, store)
}
