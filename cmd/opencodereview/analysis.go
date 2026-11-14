// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/workflow"
)

func validateAnalysisOptions(opts reviewOptions) error {
	if !opts.analysis {
		return nil
	}
	if err := validateDiffMode(opts.from, opts.to, opts.commit); err != nil {
		return err
	}
	if opts.resume != "" || opts.preview {
		return fmt.Errorf("--analysis cannot be combined with --resume or --preview")
	}
	format := strings.ToLower(strings.TrimSpace(opts.outputFormat))
	if format != "text" && format != "json" {
		return fmt.Errorf("--analysis supports only --format text or json")
	}
	return nil
}

func pinAnalysisOptions(ctx context.Context, opts reviewOptions) (reviewOptions, error) {
	if err := validateDiffMode(opts.from, opts.to, opts.commit); err != nil {
		return opts, err
	}
	repo, err := resolveRepoDir(opts.repoDir)
	if err != nil {
		return opts, err
	}
	if err := validateReviewRefs(repo, opts); err != nil {
		return opts, err
	}
	opts.repoDir = repo
	if opts.from == "" && opts.to == "" && opts.commit == "" {
		return opts, nil
	}
	resolution, err := agent.ResolveComparisonInput(ctx, agent.Args{RepoDir: repo, From: opts.from, To: opts.to, Commit: opts.commit, GitRunner: gitcmd.New(opts.maxGitProcs)})
	if err != nil {
		return opts, err
	}
	if opts.commit != "" {
		opts.commit = resolution.ResolvedHead
	} else {
		opts.from = resolution.ResolvedBase
		opts.to = resolution.ResolvedHead
	}
	return opts, nil
}

type analysisExecution struct {
	runner     *behaviortask.Runner
	initialize func(context.Context) error
	directory  string
	collector  *llm.RetryCollector
	closeRaw   func()
	timeout    time.Duration
	fallback   behaviortask.Result
}

func newAnalysisExecution(cc *commonContext, opts reviewOptions, runtime *llmRuntime, budget *workflow.Budget, reviewID string, resolution *diff.InputResolution) *analysisExecution {
	execution := &analysisExecution{closeRaw: func() {}, timeout: time.Duration(opts.concurrentTaskTimeout) * time.Minute,
		fallback: behaviortask.Result{SchemaVersion: behaviortask.SchemaVersion, SubmissionStatus: "not_submitted", StopReason: "preparation", Model: runtime.Model}}
	execution.initialize = func(ctx context.Context) error {
		return execution.prepare(ctx, cc, opts, runtime, budget, reviewID, resolution)
	}
	return execution
}

func (execution *analysisExecution) prepare(ctx context.Context, cc *commonContext, opts reviewOptions, runtime *llmRuntime, budget *workflow.Budget, reviewID string, resolution *diff.InputResolution) error {
	if resolution == nil {
		return fmt.Errorf("analysis requires resolved comparison endpoints")
	}
	execution.fallback.Comparison = behaviortask.Comparison{Repository: cc.RepoDir, Mode: reviewModeFromOptions(opts), Old: resolution.ResolvedBase, Head: resolution.ResolvedHead, BaseIsEmpty: resolution.BaseIsEmpty}
	input := behaviortask.Input{RepoDir: cc.RepoDir, Mode: reviewModeFromOptions(opts), Resolution: *resolution}
	if cc.FileFilter != nil {
		input.ExcludePatterns = append([]string(nil), cc.FileFilter.Exclude...)
	}
	if opts.background != "" {
		input.Contexts = []behaviortask.Context{{ID: "background", Kind: "background", Source: "review background input", Version: "v1", CollectedAt: time.Now().UTC().Format(time.RFC3339), Content: opts.background}}
	}
	input, err := behaviortask.PrepareInput(ctx, input)
	if err != nil {
		return err
	}
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}
	parent := filepath.Join(filepath.Dir(configPath), "analysis")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	execution.directory = filepath.Join(parent, reviewID)
	tpl, err := template.LoadBehaviorDefault()
	if err != nil {
		return err
	}
	tpl.MaxTokens = cc.Template.MaxTokens
	tpl.MaxToolRequestTimes = cc.Template.MaxToolRequestTimes
	execution.collector = llm.NewRetryCollector()
	var raw *llm.RawHolder
	if llm.RawLoggingEnabled() {
		raw = llm.NewRawHolder()
	}
	client := budget.Wrap(llm.NewLLMClient(runtime.Endpoint, execution.collector, raw))
	execution.runner, err = behaviortask.NewRunner(ctx, behaviortask.Deps{LLMClient: client, Model: runtime.Model, Language: runtime.RuntimeConfig.Language,
		Template: tpl, ArtifactDir: execution.directory, MaxTokensBudget: int64(opts.maxTokensBudget),
		NewRequestMeta: func(key string, taskType session.TaskType, requestNo int) llm.RequestMeta {
			return llm.RequestMeta{Provider: runtime.Provider, Model: runtime.Model, FilePath: key, TaskType: string(taskType), RequestNo: requestNo}
		},
	}, input)
	if err != nil {
		return err
	}
	execution.closeRaw = bindRawWriter(raw, cc.RepoDir, execution.runner.Session())
	return nil
}

func (execution *analysisExecution) run(ctx context.Context) (behaviortask.Result, error) {
	defer func() { execution.closeRaw() }()
	if execution.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, execution.timeout)
		defer cancel()
	}
	if err := execution.initialize(ctx); err != nil {
		return execution.fallback, err
	}
	return execution.runner.Run(ctx)
}

func (execution *analysisExecution) report(result behaviortask.Result, runErr error) *workflow.Report {
	report := workflow.NewReport(result, runErr, execution.directory)
	if execution.runner != nil {
		var err error
		report.RetryReport, err = execution.collector.Freeze(execution.runner.Session().SessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] warning: analysis retry report suppressed: %v\n", err)
		}
	}
	return report
}

type combinedResult struct {
	*agent.Agent
	report    *workflow.Report
	budget    *workflow.Budget
	reviewErr error
}

func (result *combinedResult) TotalInputTokens() int64 {
	usage, _ := result.budget.Snapshot()
	return usage.InputTokens
}
func (result *combinedResult) TotalOutputTokens() int64 {
	usage, _ := result.budget.Snapshot()
	return usage.OutputTokens
}
func (result *combinedResult) TotalTokensUsed() int64 {
	usage, _ := result.budget.Snapshot()
	return usage.InputTokens + usage.OutputTokens
}
func (result *combinedResult) TotalCacheReadTokens() int64 {
	usage, _ := result.budget.Snapshot()
	return usage.CacheReadTokens
}
func (result *combinedResult) TotalCacheWriteTokens() int64 {
	usage, _ := result.budget.Snapshot()
	return usage.CacheWriteTokens
}
func (result *combinedResult) BudgetExceeded() bool {
	_, exhausted := result.budget.Snapshot()
	return exhausted || result.Agent.BudgetExceeded()
}

type analysisOutput struct {
	Report       *workflow.Report
	ReviewStatus string
}

func (result *combinedResult) AnalysisOutput() *analysisOutput {
	status := "failed"
	if manifest := result.RunManifest(); manifest != nil {
		status = string(manifest.TerminalState)
	}
	if result.reviewErr != nil && (status == string(session.StateComplete) || status == string(session.StateSkipped)) {
		status = "partial"
	}
	return &analysisOutput{Report: result.report, ReviewStatus: status}
}

func analysisOutputOf(provider ResultProvider) *analysisOutput {
	if provider, ok := provider.(interface{ AnalysisOutput() *analysisOutput }); ok {
		return provider.AnalysisOutput()
	}
	return nil
}
