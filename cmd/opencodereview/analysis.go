// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/pathutil"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/workflow"
)

func validateAnalysisOptions(opts reviewOptions) error {
	if opts.analysisMaxTokensBudget < 0 {
		return fmt.Errorf("--analysis-max-tokens-budget must be a non-negative integer (0 means unlimited)")
	}
	if !opts.analysis {
		return nil
	}
	if opts.resume != "" || opts.preview {
		return fmt.Errorf("--analysis cannot be combined with --resume or --preview")
	}
	if reviewModeFromOptions(opts) == session.ReviewModeWorkspace {
		return fmt.Errorf("--analysis is not available in workspace mode; omit --analysis to review uncommitted changes, or use --commit <sha> or --from <base> [--to <head>] to analyze committed changes")
	}
	format := strings.ToLower(strings.TrimSpace(opts.outputFormat))
	if format != "text" && format != "json" {
		return fmt.Errorf("--analysis supports only --format text or json")
	}
	return nil
}

type analysisRequest struct {
	input                               behaviortask.Input
	endpoint                            llm.ResolvedEndpoint
	model, provider, language, reviewID string
	maxTokens                           int
	budget                              int64
	timeout                             time.Duration
}

func analysisInputForReview(cc *commonContext, opts reviewOptions, runtime *llmRuntime, reviewID string, resolution diff.InputResolution) analysisRequest {
	input := behaviortask.Input{RepoDir: cc.RepoDir, Mode: reviewModeFromOptions(opts), Resolution: resolution, GitRunner: cc.GitRunner}
	if cc.FileFilter != nil {
		input.ExcludePatterns = append([]string(nil), cc.FileFilter.Exclude...)
	}
	if opts.background != "" {
		input.Contexts = []behaviortask.Context{{ID: "background", Kind: "background", Source: "review background input", Version: "v1", CollectedAt: time.Now().UTC().Format(time.RFC3339), Content: opts.background}}
	}
	return analysisRequest{
		input: input, endpoint: runtime.Endpoint, maxTokens: cc.Template.MaxTokens,
		model: runtime.Model, provider: runtime.Provider, language: runtime.RuntimeConfig.Language,
		reviewID: reviewID, budget: int64(opts.analysisMaxTokensBudget), timeout: time.Duration(opts.concurrentTaskTimeout) * time.Minute,
	}
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

func newAnalysisExecution(request analysisRequest) *analysisExecution {
	input := request.input
	execution := &analysisExecution{closeRaw: func() {}, timeout: request.timeout,
		fallback: behaviortask.Result{
			SchemaVersion: behaviortask.SchemaVersion, SubmissionStatus: "not_submitted", StopReason: "preparation", Model: request.model,
			Comparison: behaviortask.Comparison{Repository: input.RepoDir, Mode: input.Mode,
				Old: input.Resolution.ResolvedBase, Head: input.Resolution.ResolvedHead, BaseIsEmpty: input.Resolution.BaseIsEmpty},
		},
	}
	execution.initialize = func(ctx context.Context) error {
		return execution.prepare(ctx, request)
	}
	return execution
}

func (execution *analysisExecution) prepare(ctx context.Context, request analysisRequest) error {
	input, err := behaviortask.PrepareInput(ctx, request.input)
	if err != nil {
		return err
	}
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}
	parent := filepath.Join(filepath.Dir(configPath), "analysis")
	for ancestor := parent; ; ancestor = filepath.Dir(ancestor) {
		resolved, err := pathutil.CanonicalPath(ancestor)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if pathutil.WithinBase(input.RepoDir, resolved) {
			return behaviortask.ErrArtifactLocation
		}
		break
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	directory := filepath.Join(parent, request.reviewID)
	tpl, err := template.LoadBehaviorDefault()
	if err != nil {
		return err
	}
	tpl.MaxTokens = request.maxTokens
	execution.collector = llm.NewRetryCollector()
	var raw *llm.RawHolder
	if llm.RawLoggingEnabled() {
		raw = llm.NewRawHolder()
	}
	client := llm.NewLLMClient(request.endpoint, execution.collector, raw)
	execution.runner, err = behaviortask.NewRunner(ctx, behaviortask.Deps{LLMClient: client, Model: request.model, Language: request.language,
		Template: tpl, ArtifactDir: directory, MaxTokensBudget: request.budget,
		NewRequestMeta: func(key string, taskType session.TaskType, requestNo int) llm.RequestMeta {
			return llm.RequestMeta{Provider: request.provider, Model: request.model, FilePath: key, TaskType: string(taskType), RequestNo: requestNo}
		},
	}, input)
	if err != nil {
		return err
	}
	execution.directory = directory
	execution.closeRaw = bindRawWriter(raw, input.RepoDir, execution.runner.Session())
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
	report        *workflow.Report
	reviewErr     error
	reviewElapsed time.Duration
}

func reviewDuration(provider ResultProvider, start time.Time) time.Duration {
	if combined, ok := provider.(*combinedResult); ok {
		return combined.reviewElapsed
	}
	return time.Since(start)
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
