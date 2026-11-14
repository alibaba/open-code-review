// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/config/toolsconfig"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/pathutil"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/google/jsonschema-go/jsonschema"
)

type Deps struct {
	LLMClient       llm.LLMClient
	Model           string
	Language        string
	Template        *template.BehaviorTemplate
	ArtifactDir     string
	MaxTokensBudget int64
	NewRequestMeta  func(taskKey string, taskType session.TaskType, requestNo int) llm.RequestMeta
}

type Runner struct {
	loop             *llmloop.Runner
	code             *codeTools
	store            *observationStore
	contexts         []Context
	schemas          map[string]*jsonschema.Resolved
	analysis         *Analysis
	submissionStatus string
	deps             Deps
	history          *session.SessionHistory
	mu               sync.Mutex
	started          bool
}

func NewRunner(ctx context.Context, deps Deps, input Input) (*Runner, error) {
	if deps.LLMClient == nil || deps.Model == "" || deps.ArtifactDir == "" || deps.MaxTokensBudget < 0 {
		return nil, fmt.Errorf("behavior requires a client, model, fresh artifact directory, and nonnegative token budget")
	}
	if err := validateContexts(input.Contexts); err != nil {
		return nil, err
	}
	input.Contexts = slices.Clone(input.Contexts)
	for index := range input.Contexts {
		input.Contexts[index].ContentHash = contentHash(input.Contexts[index].Content)
	}
	var err error
	input.RepoDir, err = pathutil.CanonicalPath(input.RepoDir)
	if err != nil {
		return nil, err
	}
	artifactParent, err := pathutil.CanonicalPath(filepath.Dir(deps.ArtifactDir))
	if err != nil {
		return nil, err
	}
	deps.ArtifactDir = filepath.Join(artifactParent, filepath.Base(deps.ArtifactDir))
	if pathutil.WithinBase(input.RepoDir, deps.ArtifactDir) {
		return nil, fmt.Errorf("behavior artifacts must be outside the analyzed repository")
	}
	if deps.Template == nil {
		deps.Template, err = template.LoadBehaviorDefault()
		if err != nil {
			return nil, err
		}
	}
	if err := deps.Template.Validate(); err != nil {
		return nil, err
	}
	templateCopy := *deps.Template
	deps.Template = &templateCopy
	deps.Template.MainTask.Messages = slices.Clone(templateCopy.MainTask.Messages)
	deps.Template.MemoryCompressionTask.Messages = slices.Clone(templateCopy.MemoryCompressionTask.Messages)
	deps.Template.FinalRound.Messages = slices.Clone(templateCopy.FinalRound.Messages)
	deps.Template.ApplyLanguage(deps.Language)
	code, err := prepareCodeTools(ctx, input)
	if err != nil {
		return nil, err
	}
	catalog, err := toolsconfig.LoadDefault()
	if err != nil {
		return nil, err
	}
	definitions, err := toolsconfig.Resolve(catalog, deps.Template.MainTask.Tools)
	if err != nil {
		return nil, err
	}
	definitions, err = toolsconfig.WithComparisonSides(definitions)
	if err != nil {
		return nil, err
	}
	runner := &Runner{code: code, deps: deps, contexts: slices.Clone(input.Contexts), schemas: map[string]*jsonschema.Resolved{}, submissionStatus: "not_submitted"}
	var submit llm.ToolDef
	for _, definition := range definitions {
		data, err := json.Marshal(definition.Function.Parameters)
		if err != nil {
			return nil, err
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, err
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			return nil, err
		}
		runner.schemas[definition.Function.Name] = resolved
		if definition.Function.Name == "submit_analysis" {
			submit = definition
		}
	}
	if err := os.Mkdir(deps.ArtifactDir, 0700); err != nil {
		return nil, err
	}
	runner.store, err = newObservationStore(deps.ArtifactDir, code.comparison.ID)
	if err != nil {
		return nil, err
	}
	runner.history = session.New(input.RepoDir, "", deps.Model, session.SessionOptions{ReviewMode: "behavior", DiffFrom: code.comparison.Old, DiffTo: code.comparison.Head})
	compression := []llmloop.MessageTemplate{}
	for _, message := range deps.Template.MemoryCompressionTask.Messages {
		compression = append(compression, llmloop.MessageTemplate{Role: message.Role, Content: message.Content})
	}
	grace := []llm.Message{}
	for _, message := range deps.Template.FinalRound.Messages {
		grace = append(grace, llm.NewTextMessage(message.Role, message.Content))
	}
	runner.loop = llmloop.NewRunner(llmloop.Deps{LLMClient: deps.LLMClient, Model: deps.Model, Session: runner.history, NewRequestMeta: deps.NewRequestMeta, MaxTokensBudget: deps.MaxTokensBudget}, llmloop.Task{
		ToolDefs: definitions, ExecuteTool: runner.executeTool, MaxRounds: deps.Template.MaxToolRequestTimes,
		MaxContextTokens: deps.Template.MaxTokens, MaxCompletionTokens: deps.Template.MaxCompletionTokens,
		EmptyResponseMessage: "Use an available evidence tool or submit_analysis; prose alone does not complete the analysis.",
		Compression:          llmloop.CompressionPolicy{Messages: compression, SummaryTemplate: "\n\n<previous_behavior_summary>\n{{summary}}\n</previous_behavior_summary>"},
		Grace:                llmloop.GracePolicy{Tools: []llm.ToolDef{submit}, Messages: grace},
	})
	return runner, nil
}

func (runner *Runner) messages() ([]llm.Message, error) {
	values := map[string]any{"comparison_manifest": runner.code.comparison, "change_inventory": runner.code.scope, "context_index": contextIndex(runner.contexts)}
	background := []Context{}
	for _, captured := range runner.contexts {
		if captured.Kind == "background" {
			background = append(background, captured)
		}
	}
	values["background_context"] = background
	replacements := []string{}
	for name, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		replacements = append(replacements, "{{"+name+"}}", string(data))
	}
	messages := []llm.Message{}
	for _, message := range runner.deps.Template.MainTask.Messages {
		remaining := message.Content
		for name := range values {
			remaining = strings.ReplaceAll(remaining, "{{"+name+"}}", "")
		}
		if strings.Contains(remaining, "{{") {
			return nil, fmt.Errorf("unresolved behavior template variable")
		}
		content := strings.NewReplacer(replacements...).Replace(message.Content)
		if message.Role == "system" && len(messages) > 0 && messages[len(messages)-1].Role == "system" {
			messages[len(messages)-1] = llm.NewTextMessage("system", messages[len(messages)-1].ExtractText()+"\n\n"+content)
		} else {
			messages = append(messages, llm.NewTextMessage(message.Role, content))
		}
	}
	return messages, nil
}

func (runner *Runner) Run(ctx context.Context) (Result, error) {
	runner.mu.Lock()
	if runner.started {
		runner.mu.Unlock()
		return Result{}, fmt.Errorf("a behavior runner can run only once")
	}
	runner.started = true
	runner.mu.Unlock()
	messages, err := runner.messages()
	var outcome llmloop.RunResult
	if err == nil {
		outcome, err = runner.loop.Run(ctx, messages, runner.code.comparison.ID)
	}
	runner.loop.WaitBackground()
	if errors.Is(err, llm.ErrTokenBudget) {
		outcome.StopReason = llmloop.StopTokenBudget
	}
	err = errors.Join(err, ctx.Err(), runner.history.Finalize())
	if outcome.StopReason != llmloop.StopNone {
		runner.loop.RecordWarning("behavior_limit_reached", runner.code.comparison.ID, outcome.StopReason.Reason())
	}
	ids := []string{}
	for _, observation := range runner.store.items {
		ids = append(ids, observation.ID)
		if observation.Truncated {
			runner.loop.RecordWarning("observation_truncated", observation.ID, observation.Continuation)
		}
	}
	result := Result{SchemaVersion: SchemaVersion, Comparison: runner.code.comparison, Scope: runner.code.scope, Analysis: runner.analysis,
		SubmissionStatus: runner.submissionStatus, Completed: outcome.Completed, StopReason: outcome.StopReason.String(), Errors: []string{},
		Warnings: runner.loop.Warnings(), ToolFailures: runner.loop.ToolFailures(), Model: runner.deps.Model,
		TemplateVersion: runner.deps.Template.Version, ToolsVersion: ToolsVersion, SessionID: runner.history.SessionID, ObservationIDs: ids,
		ContextIndex: contextIndex(runner.contexts), MaxRounds: runner.deps.Template.MaxToolRequestTimes, MaxTokensBudget: runner.deps.MaxTokensBudget,
		Usage: Usage{runner.loop.TotalInputTokens(), runner.loop.TotalOutputTokens(), runner.loop.TotalCacheReadTokens(), runner.loop.TotalCacheWriteTokens()}}
	result.CitedObservationIDs = []string{}
	if runner.analysis != nil {
		result.CitedObservationIDs = citedObservations(runner.analysis.Markdown)
		err = errors.Join(err, runner.writeArtifact("report.md", []byte(runner.analysis.Markdown)))
	}
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	data, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return result, errors.Join(err, marshalErr)
	}
	writeErr := runner.writeArtifact("result.json", append(data, '\n'))
	if writeErr != nil {
		result.Errors = append(result.Errors, writeErr.Error())
	}
	return result, errors.Join(err, writeErr)
}

func (runner *Runner) Session() *session.SessionHistory { return runner.history }

func (runner *Runner) writeArtifact(name string, data []byte) error {
	handle, err := os.OpenFile(filepath.Join(runner.deps.ArtifactDir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = handle.Write(data)
	return errors.Join(err, handle.Close())
}

func (runner *Runner) executeTool(ctx context.Context, taskKey string, call llm.ToolCall, record *session.TaskRecord, _ string) llmloop.ToolOutcome {
	started := time.Now()
	name := call.Function.Name
	number := runner.loop.RecordToolCall(name)
	outcome, err := runner.dispatch(ctx, name, call.Function.Arguments)
	if err != nil {
		outcome.Content = "Error: " + err.Error()
		runner.loop.RecordToolFailure(number, name, taskKey, outcome.Content, record, call.Function.Arguments, time.Since(started))
	} else {
		var observation Observation
		_ = json.Unmarshal([]byte(outcome.Content), &observation)
		if observation.ErrorClass != "" {
			runner.loop.RecordToolFailure(number, name, taskKey, outcome.Content, record, call.Function.Arguments, time.Since(started))
		} else if record != nil {
			record.AddToolResult(name, call.Function.Arguments, outcome.Content)
		}
	}
	return outcome
}

func (runner *Runner) dispatch(ctx context.Context, name, raw string) (llmloop.ToolOutcome, error) {
	if err := ctx.Err(); err != nil {
		return llmloop.ToolOutcome{}, err
	}
	if runner.analysis != nil {
		return llmloop.ToolOutcome{}, fmt.Errorf("the first accepted analysis is frozen")
	}
	schema, exists := runner.schemas[name]
	if !exists {
		return llmloop.ToolOutcome{}, fmt.Errorf("tool is unavailable")
	}
	if name == "submit_analysis" {
		runner.submissionStatus = "rejected"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return llmloop.ToolOutcome{}, err
	}
	for key := range args {
		if _, exists := schema.Schema().Properties[key]; !exists {
			return llmloop.ToolOutcome{}, fmt.Errorf("unknown tool argument %q", key)
		}
	}
	if err := schema.Validate(args); err != nil {
		return llmloop.ToolOutcome{}, err
	}
	if name == "submit_analysis" {
		var analysis Analysis
		if err := json.Unmarshal([]byte(raw), &analysis); err != nil {
			return llmloop.ToolOutcome{}, err
		}
		if analysis.DeclaredCompletion == "" {
			analysis.DeclaredCompletion = "unknown"
		}
		if err := runner.validateAnalysis(analysis); err != nil {
			return llmloop.ToolOutcome{}, err
		}
		runner.analysis = &analysis
		runner.submissionStatus = "accepted"
		return llmloop.ToolOutcome{Content: "Analysis accepted and frozen.", Action: llmloop.Complete}, nil
	}
	var observation Observation
	var err error
	if name == "context_read" {
		observation, err = runner.readContext(args)
	} else {
		observation, err = runner.code.execute(ctx, name, args)
	}
	if err != nil {
		if observation.ErrorClass == "" {
			observation.ErrorClass = "tool_error"
		}
		if errors.Is(err, context.Canceled) {
			observation.ErrorClass = "canceled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			observation.ErrorClass = "timeout"
		}
		observation.ReturnedContent = "Error: " + err.Error()
	}
	if name == "context_read" && args["kind"] == "observation" && err == nil {
		if observation.Truncated {
			runner.loop.RecordWarning("observation_replay_truncated", fmt.Sprint(args["id"]), "Observation replay returned a page; continue with context_read and its cursor.")
		}
		data, marshalErr := json.Marshal(observation)
		return llmloop.ToolOutcome{Content: string(data)}, marshalErr
	}
	content, persistErr := runner.store.append(observation)
	if persistErr != nil {
		return llmloop.ToolOutcome{Action: llmloop.Fail}, fmt.Errorf("persist observation: %w", persistErr)
	}
	if observation.ErrorClass != "" {
		runner.loop.RecordWarning("observation_error", runner.store.items[len(runner.store.items)-1].ID, observation.ErrorClass)
	}
	return llmloop.ToolOutcome{Content: content}, nil
}
